package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	lock, err := Load(filepath.Join(dir, "nope.json"))
	if err != nil {
		t.Fatalf("missing file should not be an error: %v", err)
	}
	if len(lock.Skills) != 0 {
		t.Fatalf("expected empty skills map, got %d entries", len(lock.Skills))
	}
}

func TestLoadValidLockfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".skill-lock.json")
	body := `{
		"version": 3,
		"skills": {
			"web-design": {
				"source": "vercel-labs/agent-skills",
				"sourceType": "github",
				"sourceUrl": "https://github.com/vercel-labs/agent-skills.git",
				"skillPath": "skills/web-design/SKILL.md",
				"skillFolderHash": "abc123",
				"installedAt": "2026-04-01T00:00:00Z",
				"updatedAt": "2026-04-20T00:00:00Z"
			}
		},
		"dismissed": {}
	}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	lock, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if lock.Version != 3 {
		t.Errorf("expected version 3, got %d", lock.Version)
	}
	entry, ok := lock.Skills["web-design"]
	if !ok {
		t.Fatalf("expected web-design entry: %#v", lock.Skills)
	}
	if entry.Source != "vercel-labs/agent-skills" {
		t.Errorf("unexpected source: %s", entry.Source)
	}
}

func TestLoadMalformedReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected parse error on malformed lockfile")
	}
}

func TestPathForSourceRoot(t *testing.T) {
	got := PathForSourceRoot("/Users/x/.agents/skills")
	want := "/Users/x/.agents/.skill-lock.json"
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestIsManaged(t *testing.T) {
	lock := &Lock{Skills: map[string]Entry{"foo": {Source: "src"}}}
	if !IsManaged(lock, "foo") {
		t.Error("foo should be managed")
	}
	if IsManaged(lock, "bar") {
		t.Error("bar should not be managed")
	}
	if IsManaged(nil, "foo") {
		t.Error("nil lock should always be unmanaged")
	}
}
