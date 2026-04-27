package config

import (
	"path/filepath"
	"testing"
)

func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SKILL_TOGGLE_CONFIG_DIR", "")
	t.Setenv("SKILL_TOGGLE_OFF_ROOT", "")
	return home
}

func TestSourcesReturnsThreeBuiltinRoots(t *testing.T) {
	home := setupHome(t)
	got := Sources()
	if len(got) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(got))
	}
	wantNames := []string{"agents", "claude", "codex"}
	for i, name := range wantNames {
		if got[i].Name != name {
			t.Errorf("source %d: expected %q, got %q", i, name, got[i].Name)
		}
		expectedRoot := filepath.Join(home, "."+name, "skills")
		if got[i].Root != expectedRoot {
			t.Errorf("source %s: expected root %s, got %s", name, expectedRoot, got[i].Root)
		}
	}
}

func TestOffRootDefaultsUnderConfigDir(t *testing.T) {
	home := setupHome(t)
	got := OffRoot()
	want := filepath.Join(home, ".config", "skill-toggle", "off")
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestOffRootEnvOverride(t *testing.T) {
	setupHome(t)
	t.Setenv("SKILL_TOGGLE_OFF_ROOT", "/tmp/custom-off")
	got := OffRoot()
	if got != "/tmp/custom-off" {
		t.Errorf("expected env override, got %s", got)
	}
}

func TestConfigDirEnvOverride(t *testing.T) {
	setupHome(t)
	t.Setenv("SKILL_TOGGLE_CONFIG_DIR", "/tmp/custom-cfg")
	got := ConfigDir()
	if got != "/tmp/custom-cfg" {
		t.Errorf("expected env override, got %s", got)
	}
}

func TestLegacyConfigDirIsTogglSkills(t *testing.T) {
	home := setupHome(t)
	got := LegacyConfigDir()
	want := filepath.Join(home, ".config", "toggle-skills")
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestLegacyOffPerSourceCoversBothPatterns(t *testing.T) {
	home := setupHome(t)
	got := LegacyOffPerSource()
	if len(got) != 3 {
		t.Fatalf("expected 3 entries (one per source), got %d", len(got))
	}
	for i, name := range SourceNames {
		legacyTSPath := filepath.Join(home, ".config", "toggle-skills", "off", name)
		legacySrcPath := filepath.Join(home, "."+name, "skills-disabled")
		var foundTS, foundSrc bool
		for _, p := range got[i] {
			if p == legacyTSPath {
				foundTS = true
			}
			if p == legacySrcPath {
				foundSrc = true
			}
		}
		if !foundTS {
			t.Errorf("source %s: missing legacy %s in %v", name, legacyTSPath, got[i])
		}
		if !foundSrc {
			t.Errorf("source %s: missing legacy %s in %v", name, legacySrcPath, got[i])
		}
	}
}

func TestIsKnownSource(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"agents", true},
		{"claude", true},
		{"codex", true},
		{"custom", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsKnownSource(c.name); got != c.want {
			t.Errorf("IsKnownSource(%q): expected %v, got %v", c.name, c.want, got)
		}
	}
}
