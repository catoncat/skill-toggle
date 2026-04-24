package cli

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/catoncat/skill-toggle/internal/config"
	"github.com/catoncat/skill-toggle/internal/skills"
)

// isTerminal reports whether stdout is a character device (TTY).
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// printProfiles prints all configured profiles in a column-aligned table.
//
//   - claude       builtin root=~/.claude/skills disabled=~/.config/toggle-skills/off/claude
func printProfiles(w io.Writer, cfg *config.Config) {
	if len(cfg.Profiles) == 0 {
		return
	}

	// Compute max profile name width for alignment.
	maxNameLen := 0
	for name := range cfg.Profiles {
		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}
	if maxNameLen < 12 {
		maxNameLen = 12
	}

	// Sort profile names for deterministic output.
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		p := cfg.Profiles[name]
		marker := " "
		if name == cfg.DefaultProfile {
			marker = "*"
		}
		fmt.Fprintf(w, "%s %-*s %-7s root=%s disabled=%s\n",
			marker, maxNameLen, name, p.Source, p.Root, p.OffRoot)
	}
}

// printList prints scanned skills in a table suitable for --list or pipe output.
//
//	ON  cloudflare-global         1234 Description text...
//	OFF link-name                  567 Short description
//	ON  @symlink-skill              89 Description
func printList(w io.Writer, skills []skills.Skill, limit int) {
	count := 0
	for _, s := range skills {
		if limit > 0 && count >= limit {
			break
		}

		status := "ON "
		if s.Status == "disabled" {
			status = "OFF"
		}

		link := "  "
		if s.IsSymlink {
			link = " @"
		}

		fmt.Fprintf(w, "%s%s %-32s %4d %s\n",
			status, link, s.Name, s.DescriptionChars, truncate(s.Description, 100))
		count++
	}
}
