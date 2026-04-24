package update

import (
	"errors"
	"os/exec"
)

// RunSkillsUpdate runs "npx -y skills update [name] -g -y"
// If name is empty, updates all global skills.
// Returns the combined stdout+stderr output and the exit code.
func RunSkillsUpdate(name string) (output string, exitCode int, err error) {
	cmd := RunSkillsUpdateCmd(name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return string(out), exitErr.ExitCode(), nil
		}
		return string(out), 0, err
	}
	return string(out), 0, nil
}

// RunSkillsUpdateCmd builds and returns the *exec.Cmd for running the update.
// This allows callers (like the TUI) to customize how they run it.
func RunSkillsUpdateCmd(name string) *exec.Cmd {
	args := []string{"-y", "skills", "update"}
	if name != "" {
		args = append(args, name)
	}
	args = append(args, "-g", "-y")
	return exec.Command("npx", args...)
}
