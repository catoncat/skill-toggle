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
	listFlag    bool
	sortMode    string
	limit       int
	source      string
	status      string
	enableName  string
	disableName string
	updateName  string
	updateAll   bool
	// showLinked includes skills whose canonical path duplicates an
	// earlier source's entry (e.g. ~/.claude/skills -> ~/.agents/skills).
	// Off by default so a single physical skill doesn't show up twice.
	showLinked bool
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
		Short:         "Toggle local agent skills across agents/claude/codex roots.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoot(cmd, args, opts)
		},
	}

	addSharedFlags(rootCmd, opts)
	addLegacyTopLevelFlags(rootCmd, opts)
	rootCmd.AddCommand(
		newListCommand(opts),
		newEnableCommand(opts),
		newDisableCommand(opts),
		newUpdateCommand(opts),
	)
	return rootCmd
}

func addSharedFlags(cmd *cobra.Command, opts *options) {
	f := cmd.PersistentFlags()
	f.StringVar(&opts.sortMode, "sort", skills.SortByName, "Sort mode: name, desc-size-desc, desc-size-asc")
	f.IntVar(&opts.limit, "limit", 0, "Limit list output (0 = unlimited)")
	f.StringVarP(&opts.source, "source", "s", "", "Restrict to one source: agents, claude, codex")
	f.StringVar(&opts.status, "status", "", "Filter list by status: enabled, disabled")
	f.BoolVar(&opts.showLinked, "show-linked", false, "Include skills whose source root is a symlink to another (hidden by default)")
}

func addLegacyTopLevelFlags(cmd *cobra.Command, opts *options) {
	f := cmd.Flags()
	f.BoolVarP(&opts.listFlag, "list", "l", false, "Print skills without TUI")
	f.StringVar(&opts.enableName, "enable", "", "Enable skill by name")
	f.StringVar(&opts.disableName, "disable", "", "Disable skill by name")
	f.StringVar(&opts.updateName, "update", "", "Run npx skills update for one skill")
	f.BoolVar(&opts.updateAll, "update-all", false, "Run npx skills update for all global skills")
}

func runRoot(cmd *cobra.Command, args []string, opts *options) error {
	if err := validateOpts(opts); err != nil {
		return err
	}
	if len(args) > 0 {
		return fmt.Errorf("unknown command: %s", args[0])
	}

	if opts.enableName != "" {
		return enableSkill(cmd, opts.enableName, opts.source)
	}
	if opts.disableName != "" {
		return disableSkill(cmd, opts.disableName, opts.source)
	}
	if opts.updateName != "" {
		return runUpdate(cmd, opts.updateName)
	}
	if opts.updateAll {
		return runUpdate(cmd, "")
	}
	if opts.listFlag || !isTerminal() {
		return listSkills(cmd, opts)
	}
	return tui.Run()
}

func newListCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List enabled and disabled skills across all sources.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOpts(opts); err != nil {
				return err
			}
			return listSkills(cmd, opts)
		},
	}
}

func newEnableCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "enable NAME",
		Short: "Move a disabled skill back into its source root.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOpts(opts); err != nil {
				return err
			}
			return enableSkill(cmd, args[0], opts.source)
		},
	}
}

func newDisableCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "disable NAME",
		Short: "Move an enabled skill into the global off directory.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOpts(opts); err != nil {
				return err
			}
			return disableSkill(cmd, args[0], opts.source)
		},
	}
}

func newUpdateCommand(opts *options) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "update [NAME]",
		Short: "Run npx skills update for one or all skills.",
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

func validateOpts(opts *options) error {
	switch opts.sortMode {
	case skills.SortByName, skills.SortByDescSizeDesc, skills.SortByDescSizeAsc:
	default:
		return fmt.Errorf("invalid --sort value %q (valid: name, desc-size-desc, desc-size-asc)", opts.sortMode)
	}
	if opts.source != "" && !config.IsKnownSource(opts.source) {
		return fmt.Errorf("unknown --source %q (valid: agents, claude, codex)", opts.source)
	}
	switch opts.status {
	case "", "all", "enabled", "disabled":
	default:
		return fmt.Errorf("invalid --status value %q (valid: enabled, disabled)", opts.status)
	}
	return nil
}

func scanAll() ([]skills.Skill, error) {
	return skills.Scan(config.Sources(), config.OffRoot(), config.LegacyOffPerSource()...)
}

func listSkills(cmd *cobra.Command, opts *options) error {
	skillList, err := scanAll()
	if err != nil {
		return err
	}
	// By default hide rows that are duplicates of another via symlinked
	// source roots (e.g. ~/.claude/skills -> ~/.agents/skills). Pass
	// --show-linked to surface every entry.
	if !opts.showLinked {
		filtered := skillList[:0]
		for _, s := range skillList {
			if s.IsDuplicate {
				continue
			}
			filtered = append(filtered, s)
		}
		skillList = filtered
	}
	if opts.source != "" {
		filtered := skillList[:0]
		for _, s := range skillList {
			if s.Source == opts.source {
				filtered = append(filtered, s)
			}
		}
		skillList = filtered
	}
	status := opts.status
	if status == "" {
		status = "all"
	}
	skillList = skills.FilterSkills(skillList, "", status, opts.sortMode)
	printList(cmd.OutOrStdout(), skillList, opts.limit)
	return nil
}

func enableSkill(cmd *cobra.Command, name, source string) error {
	all, err := scanAll()
	if err != nil {
		return err
	}
	skill, err := skills.FindSkill(all, name, source, "disabled")
	if err != nil {
		return err
	}
	op := skills.PlanOperation(skill, config.SourceRootMap()[skill.Source], config.OffRoot())
	if err := skills.ApplyOperation(op); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s/%s: disabled -> enabled\n", skill.Source, skill.Name)
	fmt.Fprintln(cmd.OutOrStdout(), "Restart Codex/Claude to load the enabled skill.")
	return nil
}

func disableSkill(cmd *cobra.Command, name, source string) error {
	all, err := scanAll()
	if err != nil {
		return err
	}
	skill, err := skills.FindSkill(all, name, source, "enabled")
	if err != nil {
		return err
	}
	op := skills.PlanOperation(skill, config.SourceRootMap()[skill.Source], config.OffRoot())
	if err := skills.ApplyOperation(op); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s/%s: enabled -> disabled\n", skill.Source, skill.Name)
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

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
