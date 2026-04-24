package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfilesSubcommandPrintsProfiles(t *testing.T) {
	dir := t.TempDir()
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", filepath.Join(dir, "roots.json"), "profiles"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	text := out.String()
	if !strings.Contains(text, "agents") || !strings.Contains(text, "claude") {
		t.Fatalf("expected profile list, got %q", text)
	}
}

func TestUnknownPositionalArgIsRejected(t *testing.T) {
	dir := t.TempDir()
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--config", filepath.Join(dir, "roots.json"), "totally-not-a-command"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown positional arg to fail")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}
