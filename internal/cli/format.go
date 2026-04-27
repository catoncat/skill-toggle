package cli

import (
	"fmt"
	"io"
	"os"

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

// printList prints scanned skills in a table suitable for --list or piped
// output. Columns: STATUS  SOURCE  NAME (32w)  DESC_CHARS  DESCRIPTION
//
//	ON   agents  cloudflare-global         1234 Description text...
//	OFF  claude  ctf-web                    882 Web CTF workflow...
func printList(w io.Writer, list []skills.Skill, limit int) {
	count := 0
	for _, s := range list {
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

		fmt.Fprintf(w, "%s%s %-7s %-32s %4d %s\n",
			status, link, s.Source, s.Name, s.DescriptionChars, truncate(s.Description, 100))
		count++
	}
}
