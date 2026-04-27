package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkillFile(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SKILL_TOGGLE_CONFIG_DIR", filepath.Join(home, ".config", "skill-toggle"))
	t.Setenv("SKILL_TOGGLE_OFF_ROOT", filepath.Join(home, ".config", "skill-toggle", "off"))
	return home
}

func TestListIncludesSourceColumn(t *testing.T) {
	home := setupHome(t)
	writeSkillFile(t, filepath.Join(home, ".agents", "skills"), "alpha", "alpha description")
	writeSkillFile(t, filepath.Join(home, ".claude", "skills"), "beta", "beta description")

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "agents") || !strings.Contains(text, "alpha") {
		t.Fatalf("expected agents/alpha in output, got %q", text)
	}
	if !strings.Contains(text, "claude") || !strings.Contains(text, "beta") {
		t.Fatalf("expected claude/beta in output, got %q", text)
	}
}

func TestListSourceFilter(t *testing.T) {
	home := setupHome(t)
	writeSkillFile(t, filepath.Join(home, ".agents", "skills"), "alpha", "alpha")
	writeSkillFile(t, filepath.Join(home, ".claude", "skills"), "beta", "beta")

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list", "--source", "agents"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "alpha") {
		t.Fatalf("expected alpha in output, got %q", text)
	}
	if strings.Contains(text, "beta") {
		t.Fatalf("did not expect beta when filtering by agents, got %q", text)
	}
}

func TestEnableMovesSkillBackToSourceRoot(t *testing.T) {
	home := setupHome(t)
	off := filepath.Join(home, ".config", "skill-toggle", "off", "agents")
	writeSkillFile(t, off, "demo", "demo description")

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"enable", "demo"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	live := filepath.Join(home, ".agents", "skills", "demo", "SKILL.md")
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("expected skill at %s, got error %v", live, err)
	}
	if _, err := os.Stat(filepath.Join(off, "demo")); !os.IsNotExist(err) {
		t.Fatalf("expected demo to be removed from off root, got err=%v", err)
	}
}

func TestDisableMovesSkillIntoOffRoot(t *testing.T) {
	home := setupHome(t)
	live := filepath.Join(home, ".claude", "skills")
	writeSkillFile(t, live, "ctf-web", "Web CTF helpers")

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"disable", "ctf-web"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(home, ".config", "skill-toggle", "off", "claude", "ctf-web", "SKILL.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected ctf-web at %s, got %v", target, err)
	}
}

func TestEnableAmbiguousNameRequiresSource(t *testing.T) {
	home := setupHome(t)
	writeSkillFile(t, filepath.Join(home, ".config", "skill-toggle", "off", "agents"), "shared", "from agents")
	writeSkillFile(t, filepath.Join(home, ".config", "skill-toggle", "off", "claude"), "shared", "from claude")

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"enable", "shared"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected ambiguity error when same name exists in two sources")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
}

func TestUnknownPositionalArgIsRejected(t *testing.T) {
	setupHome(t)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"totally-not-a-command"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown positional arg to fail")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestUnknownSourceFlagRejected(t *testing.T) {
	setupHome(t)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"list", "--source", "nonsense"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown source to fail")
	}
}

func TestLegacyOffRootSurfacesAsDisabled(t *testing.T) {
	home := setupHome(t)
	legacyOff := filepath.Join(home, ".config", "toggle-skills", "off", "agents")
	writeSkillFile(t, legacyOff, "legacy-skill", "from old layout")

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "legacy-skill") || !strings.Contains(text, "OFF") {
		t.Fatalf("expected legacy skill listed as OFF, got %q", text)
	}
}
