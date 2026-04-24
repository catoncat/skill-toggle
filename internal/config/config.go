package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const DefaultProfile = "agents"

const (
	configDirName  = "toggle-skills"
	configFileName = "roots.json"
)

var profileNameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// Sentinel errors for common config failures.
var (
	ErrUnknownProfile     = errors.New("unknown profile")
	ErrInvalidConfig      = errors.New("invalid config")
	ErrProfileNameInvalid = errors.New("invalid profile name")
	ErrBuiltinProfile     = errors.New("cannot modify builtin profile")
)

// Profile holds the resolved paths for a single skill root profile.
type Profile struct {
	Name     string
	Root     string   // expanded absolute path
	OffRoot  string   // primary expanded off path used for new disables
	OffRoots []string // all off paths scanned for disabled skills
	Source   string   // "builtin" or "custom"
}

// Config represents the merged configuration of builtin and custom profiles.
type Config struct {
	DefaultProfile string
	Profiles       map[string]Profile

	configFilePath string
	rawConfig      map[string]any
}

// expandPath replaces a leading "~/" (or standalone "~") with the user's
// home directory. Other paths are returned unchanged.
func expandPath(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// makeBuiltinProfiles returns the three built-in profiles with off-root
// paths anchored to the configured config directory.
func makeBuiltinProfiles() map[string]Profile {
	return map[string]Profile{
		"agents": makeBuiltinProfile("agents", "~/.agents/skills", "~/.agents/skills-disabled"),
		"claude": makeBuiltinProfile("claude", "~/.claude/skills", "~/.claude/skills-disabled"),
		"codex":  makeBuiltinProfile("codex", "~/.codex/skills", "~/.codex/skills-disabled"),
	}
}

func makeBuiltinProfile(name, root, legacyOffRoot string) Profile {
	offRoot := DefaultOffRoot(name)
	legacy := expandPath(legacyOffRoot)
	offRoots := []string{offRoot}
	if legacy != offRoot {
		offRoots = append(offRoots, legacy)
	}
	return Profile{
		Name:     name,
		Root:     expandPath(root),
		OffRoot:  offRoot,
		OffRoots: offRoots,
		Source:   "builtin",
	}
}

// ConfigDir resolves the configuration directory path.
// Priority: SKILL_TOGGLE_CONFIG_DIR env var > XDG_CONFIG_HOME > ~/.config,
// with "toggle-skills" appended.
func ConfigDir() (string, error) {
	if d := os.Getenv("SKILL_TOGGLE_CONFIG_DIR"); d != "" {
		return d, nil
	}

	var base string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		base = xdg
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, configDirName), nil
}

// DefaultOffRoot returns the default off-root path for a profile, which is
// <configDir>/off/<profile>.
func DefaultOffRoot(profile string) string {
	dir, err := ConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config", configDirName)
	}
	return filepath.Join(dir, "off", profile)
}

// ValidateProfileName checks that the name matches the allowed pattern
// ^[A-Za-z0-9_.-]+$. Returns ErrProfileNameInvalid if it does not.
func ValidateProfileName(name string) error {
	if !profileNameRE.MatchString(name) {
		return fmt.Errorf("%w: profile names may only contain letters, numbers, dots, underscores, and dashes", ErrProfileNameInvalid)
	}
	return nil
}

// ConfigFilePath resolves the config file path.
// Priority: explicitPath > SKILL_TOGGLE_CONFIG env var > configDir/roots.json.
func ConfigFilePath(explicitPath string) string {
	if explicitPath != "" {
		return explicitPath
	}
	if env := os.Getenv("SKILL_TOGGLE_CONFIG"); env != "" {
		return env
	}
	dir, err := ConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config", configDirName)
	}
	return filepath.Join(dir, configFileName)
}

// LoadConfig reads and parses the config file at the resolved path, then
// merges builtin and custom profiles into a single Config value.
//
// If the file does not exist, an empty config with only builtin profiles is
// returned — this is not an error.
func LoadConfig(configFile string) (*Config, error) {
	cfgPath := ConfigFilePath(configFile)

	rawData := make(map[string]any)

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("could not read config %s: %w", cfgPath, err)
		}
	} else {
		if err := json.Unmarshal(data, &rawData); err != nil {
			return nil, fmt.Errorf("%w: could not parse %s: %v", ErrInvalidConfig, cfgPath, err)
		}
	}

	defaultProfile := DefaultProfile
	if d, ok := rawData["default"].(string); ok && d != "" {
		defaultProfile = d
	}

	cfg := &Config{
		DefaultProfile: defaultProfile,
		Profiles:       makeBuiltinProfiles(),
		configFilePath: cfgPath,
		rawConfig:      rawData,
	}

	// Merge custom profiles on top of builtins.
	profilesRaw, _ := rawData["profiles"].(map[string]any)
	for name, value := range profilesRaw {
		if err := ValidateProfileName(name); err != nil {
			return nil, err
		}

		profileMap, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: profile %s must be an object", ErrInvalidConfig, name)
		}

		root, _ := profileMap["root"].(string)
		if root == "" {
			return nil, fmt.Errorf("%w: profile %s must define root", ErrInvalidConfig, name)
		}

		offRoot, _ := profileMap["disabled_root"].(string)
		if offRoot == "" {
			offRoot = DefaultOffRoot(name)
		}

		cfg.Profiles[name] = Profile{
			Name:     name,
			Root:     expandPath(root),
			OffRoot:  expandPath(offRoot),
			OffRoots: []string{expandPath(offRoot)},
			Source:   "custom",
		}
	}

	// Ensure the configured default profile actually exists.
	if _, ok := cfg.Profiles[defaultProfile]; !ok {
		cfg.DefaultProfile = DefaultProfile
	}

	return cfg, nil
}

// Save persists the current configuration to disk as JSON. Unknown
// top-level keys from the original file are preserved.
func (cfg *Config) Save() error {
	if cfg.rawConfig == nil {
		cfg.rawConfig = make(map[string]any)
	}

	cfg.rawConfig["default"] = cfg.DefaultProfile

	// Rebuild the custom profiles section from the merged state.
	customProfiles := make(map[string]any)
	for name, p := range cfg.Profiles {
		if p.Source == "custom" {
			customProfiles[name] = map[string]any{
				"root":          p.Root,
				"disabled_root": p.OffRoot,
			}
		}
	}
	if len(customProfiles) > 0 {
		cfg.rawConfig["profiles"] = customProfiles
	} else {
		delete(cfg.rawConfig, "profiles")
	}

	data, err := json.MarshalIndent(cfg.rawConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal config: %w", err)
	}
	// Append trailing newline to match Python json.dumps(…, sort_keys=True)
	data = append(data, '\n')

	dir := filepath.Dir(cfg.configFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("could not create config directory %s: %w", dir, err)
	}

	if err := os.WriteFile(cfg.configFilePath, data, 0644); err != nil {
		return fmt.Errorf("could not write config %s: %w", cfg.configFilePath, err)
	}

	return nil
}

// AddProfile adds or overrides a custom profile. If offRoot is empty the
// default off-root path is used. The config is saved on success.
func AddProfile(cfg *Config, name, root, offRoot string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}

	if offRoot == "" {
		offRoot = DefaultOffRoot(name)
	}

	cfg.Profiles[name] = Profile{
		Name:     name,
		Root:     expandPath(root),
		OffRoot:  expandPath(offRoot),
		OffRoots: []string{expandPath(offRoot)},
		Source:   "custom",
	}

	return cfg.Save()
}

// SetDefaultProfile sets the default profile name and persists the config.
// Returns ErrUnknownProfile if the name does not exist.
func SetDefaultProfile(cfg *Config, name string) error {
	if _, ok := cfg.Profiles[name]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknownProfile, name)
	}
	cfg.DefaultProfile = name
	return cfg.Save()
}

// RemoveProfile deletes a custom profile. Builtin profiles cannot be
// removed; attempting to do so returns ErrBuiltinProfile.
func RemoveProfile(cfg *Config, name string) error {
	p, ok := cfg.Profiles[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownProfile, name)
	}
	if p.Source == "builtin" {
		return fmt.Errorf("%w: %s", ErrBuiltinProfile, name)
	}

	delete(cfg.Profiles, name)
	return cfg.Save()
}

// ResolveRoots resolves the effective profile and paths, considering
// runtime overrides from arguments and environment variables.
//
// Resolution priority for the profile name:
//  1. profile parameter (non-empty)
//  2. SKILL_TOGGLE_PROFILE env var
//  3. cfg.DefaultProfile
//
// Resolution priority for root / off-root overrides:
//  1. explicit override parameter (non-empty)
//  2. SKILL_TOGGLE_ROOT / SKILL_TOGGLE_DISABLED_ROOT env var
//  3. profile's configured path
func ResolveRoots(cfg *Config, profile, rootOverride, offRootOverride string) (*Profile, error) {
	name := profile
	if name == "" {
		name = os.Getenv("SKILL_TOGGLE_PROFILE")
	}
	if name == "" {
		name = cfg.DefaultProfile
	}

	p, ok := cfg.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProfile, name)
	}

	root := p.Root
	if rootOverride != "" {
		root = expandPath(rootOverride)
	} else if env := os.Getenv("SKILL_TOGGLE_ROOT"); env != "" {
		root = expandPath(env)
	}

	offRoot := p.OffRoot
	offRoots := p.OffRoots
	if offRootOverride != "" {
		offRoot = expandPath(offRootOverride)
		offRoots = []string{offRoot}
	} else if env := os.Getenv("SKILL_TOGGLE_DISABLED_ROOT"); env != "" {
		offRoot = expandPath(env)
		offRoots = []string{offRoot}
	} else if len(offRoots) == 0 {
		offRoots = []string{offRoot}
	}

	return &Profile{
		Name:     name,
		Root:     root,
		OffRoot:  offRoot,
		OffRoots: offRoots,
		Source:   p.Source,
	}, nil
}
