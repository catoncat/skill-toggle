// Package update wraps `npx -y skills update`. The TUI uses the streaming
// helpers (Start) to surface live progress; the CLI prefers the simpler
// blocking call (Run) and prints output afterwards.
package update

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"sync"
)

// Line is one output line from the running npx process.
type Line struct {
	Text  string
	IsErr bool // true when the line came from stderr
}

// Result is sent once the process exits.
type Result struct {
	ExitCode int
	Err      error
}

// Run runs `npx -y skills update [name] -g -y` to completion and returns
// the combined output and the exit code. Suitable for non-interactive use.
func Run(name string) (output string, exitCode int, err error) {
	cmd := Cmd(name)
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

// RunSkillsUpdate is the legacy blocking entry point retained for the CLI.
// New callers should use Run / Start.
func RunSkillsUpdate(name string) (string, int, error) { return Run(name) }

// Cmd builds the *exec.Cmd that drives `npx -y skills update`.
func Cmd(name string) *exec.Cmd {
	args := []string{"-y", "skills", "update"}
	if name != "" {
		args = append(args, name)
	}
	args = append(args, "-g", "-y")
	return exec.Command("npx", args...)
}

// RunSkillsUpdateCmd is retained as an alias for legacy callers.
func RunSkillsUpdateCmd(name string) *exec.Cmd { return Cmd(name) }

// Start launches the update asynchronously. It returns:
//
//   - lines: stdout/stderr lines as they're read; closed when both pipes
//     EOF (i.e. the process is about to exit).
//   - result: receives exactly one Result once the process has been waited
//     on. Receive after lines closes.
//   - cancel: kill the underlying process. Calling cancel after exit is a
//     no-op.
//
// The caller is responsible for draining `lines` to completion (otherwise
// the goroutines feeding it will block on a full channel). For TUI usage
// the bubbletea event loop polls one line per Cmd invocation, which is
// sufficient because the channel buffer is sized for typical npx output.
func Start(name string) (lines <-chan Line, result <-chan Result, cancel func(), err error) {
	cmd := Cmd(name)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}

	linesCh := make(chan Line, 256)
	resultCh := make(chan Result, 1)

	var wg sync.WaitGroup
	wg.Add(2)
	go scanLines(stdout, false, linesCh, &wg)
	go scanLines(stderr, true, linesCh, &wg)

	go func() {
		wg.Wait()
		close(linesCh)
		runErr := cmd.Wait()
		res := Result{}
		if runErr != nil {
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				res.ExitCode = exitErr.ExitCode()
			} else {
				res.Err = runErr
			}
		}
		resultCh <- res
		close(resultCh)
	}()

	cancel = func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	return linesCh, resultCh, cancel, nil
}

func scanLines(r io.ReadCloser, isErr bool, out chan<- Line, wg *sync.WaitGroup) {
	defer wg.Done()
	defer r.Close()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		out <- Line{Text: scanner.Text(), IsErr: isErr}
	}
}
