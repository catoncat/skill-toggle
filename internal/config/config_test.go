package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateProfileName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"agents", false},
		{"claude", false},
		{"my-agent", false},
		{"my_agent", false},
		{"agent.v2", false},
		{"test name", true},
		{"path/traversal", true},
		{"", true},
		{"../../../etc", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProfileName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProfileName(%q) error = %v, wantErr = %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestDefaultOffRoot(t *testing.T) {
	off := DefaultOffRoot("claude")
	if !strings.Contains(off, "toggle-skills") {
		t.Errorf("off root should contain toggle-skills: %s", off)
	}
	if !strings.HasSuffix(off, filepath.Join("off", "claude")) {
		t.Errorf("off root should end with off/claude: %s", off)
	}
}

func TestBuiltinProfilesExist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKILL_TOGGLE_CONFIG_DIR", dir)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"agents", "claude", "codex"} {
		p, ok := cfg.Profiles[name]
		if !ok {
			t.Errorf("builtin profile %s missing", name)
		}
		if p.Source != "builtin" {
			t.Errorf("profile %s should be builtin, got %s", name, p.Source)
		}
	}
}

func TestBuiltinAgentsProfileIncludesLegacyOffRoot(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "toggle-skills")
	legacyOff := filepath.Join(home, ".agents", "skills-disabled")
	t.Setenv("HOME", home)
	t.Setenv("SKILL_TOGGLE_CONFIG_DIR", configDir)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}

	agents := cfg.Profiles["agents"]
	if agents.OffRoot != filepath.Join(configDir, "off", "agents") {
		t.Fatalf("primary off root should stay normalized, got %s", agents.OffRoot)
	}
	if len(agents.OffRoots) != 2 {
		t.Fatalf("expected normalized and legacy off roots, got %#v", agents.OffRoots)
	}
	if agents.OffRoots[1] != legacyOff {
		t.Fatalf("expected legacy off root %s, got %#v", legacyOff, agents.OffRoots)
	}
}

func TestDefaultProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKILL_TOGGLE_CONFIG_DIR", dir)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.DefaultProfile != DefaultProfile {
		t.Errorf("expected default profile %s, got %s", DefaultProfile, cfg.DefaultProfile)
	}
}

func TestCustomProfileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "roots.json")

	root := filepath.Join(dir, "lab-skills")
	disabled := filepath.Join(dir, "lab-off")

	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatal(err)
	}

	err = AddProfile(cfg, "lab", root, disabled)
	if err != nil {
		t.Fatal(err)
	}

	// Reload and verify persistence
	cfg2, err := LoadConfig(configFile)
	if err != nil {
		t.Fatal(err)
	}

	p, ok := cfg2.Profiles["lab"]
	if !ok {
		t.Fatal("custom profile lab not found after reload")
	}
	if p.Root != root {
		t.Errorf("expected root %s, got %s", root, p.Root)
	}
	if p.Source != "custom" {
		t.Errorf("expected source custom, got %s", p.Source)
	}
}

func TestSetDefaultProfile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "roots.json")

	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatal(err)
	}

	err = SetDefaultProfile(cfg, "claude")
	if err != nil {
		t.Fatal(err)
	}

	cfg2, err := LoadConfig(configFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.DefaultProfile != "claude" {
		t.Errorf("expected default claude, got %s", cfg2.DefaultProfile)
	}
}

func TestRemoveProfile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "roots.json")

	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatal(err)
	}

	err = AddProfile(cfg, "temp-profile", "/tmp/skills", "/tmp/off")
	if err != nil {
		t.Fatal(err)
	}

	err = RemoveProfile(cfg, "temp-profile")
	if err != nil {
		t.Fatal(err)
	}

	cfg2, err := LoadConfig(configFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg2.Profiles["temp-profile"]; ok {
		t.Error("profile should have been removed")
	}
}

func TestCannotRemoveBuiltinProfile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "roots.json")

	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatal(err)
	}

	err = RemoveProfile(cfg, "agents")
	if err == nil {
		t.Fatal("expected error when removing builtin profile")
	}
}

func TestResolveRoots(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKILL_TOGGLE_CONFIG_DIR", dir)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}

	p, err := ResolveRoots(cfg, "claude", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "claude" {
		t.Errorf("expected profile claude, got %s", p.Name)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".claude", "skills")
	if p.Root != expected {
		t.Errorf("expected root %s, got %s", expected, p.Root)
	}
}

func TestResolveRootsWithOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKILL_TOGGLE_CONFIG_DIR", dir)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}

	customRoot := filepath.Join(dir, "custom-skills")
	customOff := filepath.Join(dir, "custom-off")

	p, err := ResolveRoots(cfg, "agents", customRoot, customOff)
	if err != nil {
		t.Fatal(err)
	}
	if p.Root != customRoot {
		t.Errorf("expected overridden root %s, got %s", customRoot, p.Root)
	}
	if p.OffRoot != customOff {
		t.Errorf("expected overridden off root %s, got %s", customOff, p.OffRoot)
	}
}

func TestResolveRootsUnknownProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKILL_TOGGLE_CONFIG_DIR", dir)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}

	_, err = ResolveRoots(cfg, "nonexistent", "", "")
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestLoadConfigNonexistentFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "nonexistent.json")

	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for nonexistent file")
	}
	if cfg.DefaultProfile != DefaultProfile {
		t.Errorf("expected default profile %s, got %s", DefaultProfile, cfg.DefaultProfile)
	}
}

func TestConfigFilePath(t *testing.T) {
	t.Setenv("SKILL_TOGGLE_CONFIG", "/custom/path/config.json")
	path := ConfigFilePath("")
	if path != "/custom/path/config.json" {
		t.Errorf("expected env override path, got %s", path)
	}

	path = ConfigFilePath("/explicit/path.json")
	if path != "/explicit/path.json" {
		t.Errorf("expected explicit path, got %s", path)
	}
}

func TestAddProfileWithDefaultOffRoot(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "roots.json")

	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatal(err)
	}

	err = AddProfile(cfg, "no-off", "/tmp/my-skills", "")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Profiles["no-off"].OffRoot == "" {
		t.Error("off root should have been set to default")
	}
}
