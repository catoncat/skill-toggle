// Package lockfile reads vercel-labs/skills' .skill-lock.json files. The
// lockfile is the single source of truth for "is this skill installed via
// `npx skills add`?" — without it on disk, `skills update` cannot reach
// the skill either, so we use it to gate the TUI/CLI update entrypoints.
//
// Layout (one per source root, written by `npx skills add -g`):
//
//	~/.agents/.skill-lock.json
//	~/.claude/.skill-lock.json
//	~/.codex/.skill-lock.json
//
// Schema observed against skills@1.5.1:
//
//	{
//	  "version": 3,
//	  "skills": {
//	    "<name>": {
//	      "source":          "vercel-labs/agent-skills",
//	      "sourceType":      "github",
//	      "sourceUrl":       "https://github.com/vercel-labs/agent-skills.git",
//	      "skillPath":       "skills/<name>/SKILL.md",
//	      "skillFolderHash": "<sha>",
//	      "installedAt":     "...",
//	      "updatedAt":       "..."
//	    }
//	  },
//	  "dismissed": {}
//	}
package lockfile

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Entry is one row of the lockfile's `skills` map.
type Entry struct {
	Source          string `json:"source"`
	SourceType      string `json:"sourceType"`
	SourceURL       string `json:"sourceUrl"`
	SkillPath       string `json:"skillPath"`
	SkillFolderHash string `json:"skillFolderHash"`
	InstalledAt     string `json:"installedAt"`
	UpdatedAt       string `json:"updatedAt"`
}

// Lock is the parsed top-level structure.
type Lock struct {
	Version int              `json:"version"`
	Skills  map[string]Entry `json:"skills"`
}

// Load reads a single lockfile. A missing file is not an error — it just
// means nothing under that source root was installed via `skills add`,
// which is the common case for handcrafted skill folders. A malformed
// file is reported so the caller can decide whether to surface it.
func Load(path string) (*Lock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Lock{Skills: map[string]Entry{}}, nil
		}
		return nil, err
	}
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	if lock.Skills == nil {
		lock.Skills = map[string]Entry{}
	}
	return &lock, nil
}

// PathForSourceRoot derives the lockfile path from a source's live root.
// vercel-labs/skills writes the lockfile one level above the skills
// directory (e.g. `~/.agents/skills` → `~/.agents/.skill-lock.json`).
func PathForSourceRoot(sourceRoot string) string {
	return filepath.Join(filepath.Dir(sourceRoot), ".skill-lock.json")
}

// IsManaged reports whether (lock != nil) and lock.Skills contains name.
// nil-safe so callers can skip the existence check.
func IsManaged(lock *Lock, name string) bool {
	if lock == nil {
		return false
	}
	_, ok := lock.Skills[name]
	return ok
}
