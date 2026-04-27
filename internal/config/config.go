package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/catoncat/skill-toggle/internal/skills"
)

const (
	configDirName       = "skill-toggle"
	legacyConfigDirName = "toggle-skills"
)

// SourceNames lists the three live skill roots, in display order.
var SourceNames = []string{"agents", "claude", "codex"}

// Sentinel errors.
var (
	ErrUnknownSource = errors.New("unknown source")
)

// expandPath replaces a leading "~/" (or standalone "~") with the user's
// home directory.
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

func liveRootFor(source string) string {
	return expandPath(fmt.Sprintf("~/.%s/skills", source))
}

// Sources returns the three built-in sources with expanded live roots.
func Sources() []skills.Source {
	out := make([]skills.Source, 0, len(SourceNames))
	for _, name := range SourceNames {
		out = append(out, skills.Source{Name: name, Root: liveRootFor(name)})
	}
	return out
}

// SourceRootMap returns a map of source name -> live root for use when
// planning operations.
func SourceRootMap() map[string]string {
	out := make(map[string]string, len(SourceNames))
	for _, name := range SourceNames {
		out[name] = liveRootFor(name)
	}
	return out
}

// IsKnownSource reports whether name is one of the built-in sources.
func IsKnownSource(name string) bool {
	for _, s := range SourceNames {
		if s == name {
			return true
		}
	}
	return false
}

// configDirEnv returns an explicit override for the config directory, or "".
func configDirEnv() string {
	return os.Getenv("SKILL_TOGGLE_CONFIG_DIR")
}

// xdgConfigHome returns $XDG_CONFIG_HOME or ~/.config.
func xdgConfigHome() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config")
}

// ConfigDir resolves the new config directory path
// (~/.config/skill-toggle, overridable via SKILL_TOGGLE_CONFIG_DIR).
func ConfigDir() string {
	if d := configDirEnv(); d != "" {
		return d
	}
	base := xdgConfigHome()
	if base == "" {
		return ""
	}
	return filepath.Join(base, configDirName)
}

// LegacyConfigDir resolves the pre-rewrite config directory
// (~/.config/toggle-skills) used by the profile-based version.
func LegacyConfigDir() string {
	base := xdgConfigHome()
	if base == "" {
		return ""
	}
	return filepath.Join(base, legacyConfigDirName)
}

// OffRoot returns the global disabled-skills root, honoring
// SKILL_TOGGLE_OFF_ROOT for tests and special setups.
func OffRoot() string {
	if v := os.Getenv("SKILL_TOGGLE_OFF_ROOT"); v != "" {
		return expandPath(v)
	}
	dir := ConfigDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "off")
}

// LegacyOffPerSource returns, in the same order as Sources(), additional
// off-root directories that should be scanned for backward compatibility.
// These cover:
//
//   - the previous global layout: ~/.config/toggle-skills/off/<source>/
//   - the original per-root convention: ~/.<source>/skills-disabled/
//
// Missing directories simply don't contribute anything; callers do not need
// to filter them.
func LegacyOffPerSource() [][]string {
	out := make([][]string, 0, len(SourceNames))
	legacyDir := LegacyConfigDir()
	for _, name := range SourceNames {
		var dirs []string
		if legacyDir != "" {
			dirs = append(dirs, filepath.Join(legacyDir, "off", name))
		}
		dirs = append(dirs, expandPath(fmt.Sprintf("~/.%s/skills-disabled", name)))
		out = append(out, dirs)
	}
	return out
}
