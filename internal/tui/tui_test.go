package tui

import (
	"path/filepath"
	"testing"

	"github.com/catoncat/skill-toggle/internal/config"
	"github.com/catoncat/skill-toggle/internal/skills"
)

func TestCommandPaletteStagesDisableInsteadOfMoving(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	off := filepath.Join(t.TempDir(), "off")
	cfg := &config.Config{
		DefaultProfile: "agents",
		Profiles: map[string]config.Profile{
			"agents": {
				Name:     "agents",
				Root:     root,
				OffRoot:  off,
				OffRoots: []string{off},
				Source:   "builtin",
			},
		},
	}
	profile := cfg.Profiles["agents"]
	model := NewModel(cfg, &profile)
	model.allSkills = []skills.Skill{{
		Name:   "demo",
		Status: "enabled",
		Path:   filepath.Join(root, "demo"),
	}}
	model.commandInput = "disable demo"

	nextModel, cmd := model.executeCommand()
	next := nextModel.(Model)

	if cmd != nil {
		t.Fatal("disable command should only stage, not trigger async scan")
	}
	if len(next.stagedOps) != 1 {
		t.Fatalf("expected one staged operation, got %#v", next.stagedOps)
	}
	if next.stagedOps[0].Direction != "disable" {
		t.Fatalf("expected staged disable, got %#v", next.stagedOps[0])
	}
	if next.stagedOps[0].TargetPath != filepath.Join(off, "demo") {
		t.Fatalf("unexpected target path: %s", next.stagedOps[0].TargetPath)
	}
}

func TestNewModelKeepsCustomProfilesFromConfig(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "lab",
		Profiles: map[string]config.Profile{
			"lab": {
				Name:     "lab",
				Root:     "/tmp/lab-skills",
				OffRoot:  "/tmp/lab-off",
				OffRoots: []string{"/tmp/lab-off"},
				Source:   "custom",
			},
		},
	}
	profile := cfg.Profiles["lab"]

	model := NewModel(cfg, &profile)

	if len(model.profiles) != 1 {
		t.Fatalf("expected custom profile to be present, got %#v", model.profiles)
	}
	if model.currentProfile().Name != "lab" {
		t.Fatalf("expected active custom profile, got %#v", model.currentProfile())
	}
}
