package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/catoncat/skill-toggle/internal/config"
	"github.com/catoncat/skill-toggle/internal/skills"
	"github.com/catoncat/skill-toggle/internal/tui"
	"github.com/catoncat/skill-toggle/internal/update"
)

type options struct {
	profileName     string
	showProfiles    bool
	configFile      string
	addRootFlag     bool
	setDefaultFlag  string
	removeRootFlag  string
	rootOverride    string
	offRootOverride string
	listFlag        bool
	sortMode        string
	limit           int
	enableName      string
	disableName     string
	updateName      string
	updateAll       bool
}

// Execute runs the root command and returns an exit code.
func Execute() int {
	cmd := NewRootCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "skill-toggle: %v\n", err)
		return 1
	}
	return 0
}

func NewRootCommand() *cobra.Command {
	opts := &options{sortMode: skills.SortByName}
	rootCmd := &cobra.Command{
		Use:           "skill-toggle",
		Short:         "Enable/disable local agent skills by moving skill folders.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoot(cmd, args, opts)
		},
	}

	addSharedFlags(rootCmd, opts)
	addLegacyRootFlags(rootCmd, opts)
	rootCmd.AddCommand(
		newListCommand(opts),
		newProfilesCommand(opts),
		newEnableCommand(opts),
		newDisableCommand(opts),
		newUpdateCommand(opts),
		newProfileCommand(opts),
	)
	return rootCmd
}

func addSharedFlags(cmd *cobra.Command, opts *options) {
	f := cmd.PersistentFlags()
	f.StringVarP(&opts.profileName, "profile", "p", "", "Named skill root profile")
	f.StringVar(&opts.configFile, "config", "", "Config file path")
	f.StringVarP(&opts.rootOverride, "root", "r", "", "One-off live root override")
	f.StringVarP(&opts.offRootOverride, "disabled-root", "d", "", "One-off disabled root override")
	f.StringVar(&opts.sortMode, "sort", skills.SortByName, "Sort mode: name, desc-size-desc, desc-size-asc")
	f.IntVar(&opts.limit, "limit", 0, "Limit list output (0 = unlimited)")
}

func addLegacyRootFlags(cmd *cobra.Command, opts *options) {
	f := cmd.Flags()
	f.BoolVar(&opts.showProfiles, "profiles", false, "List configured profiles")
	f.BoolVar(&opts.addRootFlag, "add-root", false, "Add custom profile (requires PROFILE and PATH positional arguments)")
	f.StringVar(&opts.setDefaultFlag, "set-default", "", "Set default profile")
	f.StringVar(&opts.removeRootFlag, "remove-root", "", "Remove custom profile")
	f.BoolVarP(&opts.listFlag, "list", "l", false, "Print skills without TUI")
	f.StringVar(&opts.enableName, "enable", "", "Enable skill by name")
	f.StringVar(&opts.disableName, "disable", "", "Disable skill by name")
	f.StringVar(&opts.updateName, "update", "", "Run npx skills update for one skill")
	f.BoolVar(&opts.updateAll, "update-all", false, "Run npx skills update for all")
}

func runRoot(cmd *cobra.Command, args []string, opts *options) error {
	cfg, err := loadAndValidate(opts)
	if err != nil {
		return err
	}

	if opts.addRootFlag {
		if len(args) != 2 {
			return fmt.Errorf("--add-root requires exactly 2 positional arguments: PROFILE and PATH")
		}
		return addProfile(cmd, cfg, args[0], args[1], opts.offRootOverride)
	}
	if len(args) > 0 {
		return fmt.Errorf("unknown command: %s", args[0])
	}
	if opts.setDefaultFlag != "" {
		return setDefaultProfile(cmd, cfg, opts.setDefaultFlag)
	}
	if opts.removeRootFlag != "" {
		return removeProfile(cmd, cfg, opts.removeRootFlag)
	}
	if opts.showProfiles {
		printProfiles(cmd.OutOrStdout(), cfg)
		return nil
	}

	profile, err := config.ResolveRoots(cfg, opts.profileName, opts.rootOverride, opts.offRootOverride)
	if err != nil {
		return err
	}
	if opts.enableName != "" {
		return enableSkill(cmd, profile, opts.enableName)
	}
	if opts.disableName != "" {
		return disableSkill(cmd, profile, opts.disableName)
	}
	if opts.updateName != "" {
		return runUpdate(cmd, opts.updateName)
	}
	if opts.updateAll {
		return runUpdate(cmd, "")
	}
	if opts.listFlag || !isTerminal() {
		return listSkills(cmd, opts, profile)
	}
	return tui.Run(cfg, profile)
}

func newListCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List enabled and disabled skills.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidate(opts)
			if err != nil {
				return err
			}
			profile, err := config.ResolveRoots(cfg, opts.profileName, opts.rootOverride, opts.offRootOverride)
			if err != nil {
				return err
			}
			return listSkills(cmd, opts, profile)
		},
	}
}

func newProfilesCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "profiles",
		Short: "List configured profiles.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidate(opts)
			if err != nil {
				return err
			}
			printProfiles(cmd.OutOrStdout(), cfg)
			return nil
		},
	}
}

func newEnableCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "enable NAME",
		Short: "Move a disabled skill back into the live root.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidate(opts)
			if err != nil {
				return err
			}
			profile, err := config.ResolveRoots(cfg, opts.profileName, opts.rootOverride, opts.offRootOverride)
			if err != nil {
				return err
			}
			return enableSkill(cmd, profile, args[0])
		},
	}
}

func newDisableCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "disable NAME",
		Short: "Move an enabled skill into the off root.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidate(opts)
			if err != nil {
				return err
			}
			profile, err := config.ResolveRoots(cfg, opts.profileName, opts.rootOverride, opts.offRootOverride)
			if err != nil {
				return err
			}
			return disableSkill(cmd, profile, args[0])
		},
	}
}

func newUpdateCommand(opts *options) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "update [NAME]",
		Short: "Run npx skills update.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				return runUpdate(cmd, "")
			}
			if len(args) == 0 {
				return fmt.Errorf("update requires NAME or --all")
			}
			return runUpdate(cmd, args[0])
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Update all global skills")
	return cmd
}

func newProfileCommand(opts *options) *cobra.Command {
	profileCmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage profile defaults and custom roots.",
	}

	profileCmd.AddCommand(&cobra.Command{
		Use:   "set-default NAME",
		Short: "Set the default profile.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidate(opts)
			if err != nil {
				return err
			}
			return setDefaultProfile(cmd, cfg, args[0])
		},
	})

	profileCmd.AddCommand(&cobra.Command{
		Use:   "add NAME PATH",
		Short: "Add or update a custom profile.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidate(opts)
			if err != nil {
				return err
			}
			return addProfile(cmd, cfg, args[0], args[1], opts.offRootOverride)
		},
	})

	profileCmd.AddCommand(&cobra.Command{
		Use:   "remove NAME",
		Short: "Remove a custom profile.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidate(opts)
			if err != nil {
				return err
			}
			return removeProfile(cmd, cfg, args[0])
		},
	})

	return profileCmd
}

func loadAndValidate(opts *options) (*config.Config, error) {
	cfg, err := config.LoadConfig(opts.configFile)
	if err != nil {
		return nil, err
	}
	switch opts.sortMode {
	case skills.SortByName, skills.SortByDescSizeDesc, skills.SortByDescSizeAsc:
		return cfg, nil
	default:
		return nil, fmt.Errorf("invalid --sort value %q (valid: name, desc-size-desc, desc-size-asc)", opts.sortMode)
	}
}

func listSkills(cmd *cobra.Command, opts *options, profile *config.Profile) error {
	skillList, err := skills.Scan(profile.Root, profile.OffRoots...)
	if err != nil {
		return err
	}
	skillList = skills.FilterSkills(skillList, "", "all", opts.sortMode)
	printList(cmd.OutOrStdout(), skillList, opts.limit)
	return nil
}

func enableSkill(cmd *cobra.Command, profile *config.Profile, name string) error {
	msg, err := skills.EnableSkill(name, profile.Root, profile.OffRoots...)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), msg)
	fmt.Fprintln(cmd.OutOrStdout(), "Restart Codex/Claude to load the enabled skill.")
	return nil
}

func disableSkill(cmd *cobra.Command, profile *config.Profile, name string) error {
	msg, err := skills.DisableSkill(name, profile.Root, profile.OffRoot)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), msg)
	fmt.Fprintln(cmd.OutOrStdout(), "Restart Codex/Claude to apply the disabled skill.")
	return nil
}

func runUpdate(cmd *cobra.Command, name string) error {
	output, exitCode, err := update.RunSkillsUpdate(name)
	fmt.Fprint(cmd.OutOrStdout(), output)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("npx skills update exited with code %d", exitCode)
	}
	return nil
}

func addProfile(cmd *cobra.Command, cfg *config.Config, name, root, offRoot string) error {
	if err := config.AddProfile(cfg, name, root, offRoot); err != nil {
		return err
	}
	p := cfg.Profiles[name]
	fmt.Fprintf(cmd.OutOrStdout(), "added custom profile %s: root=%s disabled=%s\n", name, p.Root, p.OffRoot)
	return nil
}

func setDefaultProfile(cmd *cobra.Command, cfg *config.Config, name string) error {
	if err := config.SetDefaultProfile(cfg, name); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "default profile set to %s\n", name)
	return nil
}

func removeProfile(cmd *cobra.Command, cfg *config.Config, name string) error {
	if err := config.RemoveProfile(cfg, name); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "removed profile %s\n", name)
	return nil
}

// truncate cuts a string to at most maxLen runes, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
