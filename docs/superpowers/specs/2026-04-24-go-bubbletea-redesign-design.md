# Go Bubble Tea Redesign Design

## Summary

Rebuild `skill-toggle` as a native-feeling terminal application with a single-file binary distribution. The current Python/curses version proved the workflow: scan skill roots, preview `SKILL.md`, toggle folders between enabled and off roots, manage profiles, and call `npx skills update`. The next version should keep that behavior while making the app feel like a deliberate skills control center instead of a thin folder-moving script.

The target stack is Go with Bubble Tea, Bubbles, Lip Gloss, and Cobra. Go gives straightforward cross-platform binaries and release artifacts. Bubble Tea provides the terminal interaction model and enough polish for profile tabs, searchable lists, preview panes, staged changes, status bars, and command-style workflows without taking on Rust-level implementation cost.

## Product Goals

- Ship a standalone `skill-toggle` binary for macOS and Linux.
- Keep existing user-facing workflows: list, enable, disable, profile selection, custom roots, preview, update.
- Make the TUI safer by staging changes before applying filesystem moves.
- Make profile/root management first-class, especially `agents`, `claude`, and `codex`.
- Preserve the normalized off-root convention: `~/.config/toggle-skills/off/<profile>`.
- Make context-budget impact visible through description length totals and heavy-skill sorting.
- Keep the CLI useful in scripts even if the TUI is the primary experience.

## Non-Goals

- Do not manage plugin-provided or bundled system skills directly.
- Do not edit `SKILL.md` contents in this version.
- Do not implement a package manager replacement for `npx skills`.
- Do not depend on Python at runtime.
- Do not require an external database or daemon.

## User Experience

The default command opens the TUI:

```bash
skill-toggle
```

Common non-interactive commands remain available:

```bash
skill-toggle list
skill-toggle list --profile claude
skill-toggle enable cloudflare-global
skill-toggle disable ctf-web
skill-toggle profiles
skill-toggle profile set-default claude
skill-toggle update cloudflare-global
skill-toggle update --all
```

The TUI layout is profile-first:

```text
Profiles:  agents  claude  codex  custom      Search: memory

Enabled 18  Off 42  Desc chars 12.4k  Staged 2

ON   cloudflare-global       141  Cloudflare global config...
OFF  ctf-web                 882  Web CTF workflow...
ON   session-wrap            130  Session closeout...

Preview
name: cloudflare-global
status: enabled
path: ~/.agents/skills/cloudflare-global
desc_chars: 141
symlink: false
```

Primary keys:

- `Tab` / `Shift+Tab`: switch profile.
- `/`: search skills by folder name, frontmatter name, or description.
- `Space`: stage enable/disable for the selected skill.
- `A`: apply staged changes.
- `Esc`: clear search or cancel the current mode.
- `Enter`: focus preview/details.
- `s`: cycle sort mode.
- `u`: update selected enabled skill.
- `U`: update all global skills.
- `:`: command palette for text commands such as `disable cloudflare-global` or `profile claude`.
- `q`: quit, with confirmation if changes are staged.

The important safety change is staged toggling. Pressing `Space` does not move files immediately. It marks a pending operation and updates the UI. Pressing `A` executes all staged operations in a deterministic order. If any move fails, the app stops, reports the failed operation, and leaves remaining staged changes unapplied.

## Data Model

Core types:

- `Skill`: folder name, display name, description, description char count, status, root path, `SKILL.md` path, symlink flag, protected flag, modified time.
- `Profile`: name, live root, off root, source (`builtin` or `custom`).
- `Operation`: skill name, profile name, direction (`enable` or `disable`), source path, target path.
- `Config`: default profile and custom profiles loaded from JSON.

Built-in profiles:

```text
agents -> ~/.agents/skills
claude -> ~/.claude/skills
codex  -> ~/.codex/skills
```

Custom profiles live in:

```text
~/.config/toggle-skills/roots.json
```

The off root for a profile defaults to:

```text
~/.config/toggle-skills/off/<profile>
```

## Architecture

The Go implementation should be split into focused packages:

```text
cmd/skill-toggle/main.go
internal/cli       Cobra command definitions and command wiring
internal/config    Config path resolution, JSON load/save, profile merging
internal/skills    Scan roots, parse SKILL.md frontmatter, validate and move skills
internal/update    npx skills update wrapper
internal/tui       Bubble Tea app, models, views, key bindings
internal/ui        Shared styles and formatting helpers
```

The CLI package should depend on the core packages, not on TUI internals. The TUI should call the same `internal/skills` and `internal/config` APIs as non-interactive commands. This keeps scripted behavior and interactive behavior consistent.

## Filesystem Behavior

Toggling remains move-based:

- Disable: move `<live-root>/<skill>` to `<off-root>/<skill>`.
- Enable: move `<off-root>/<skill>` to `<live-root>/<skill>`.

Before moving:

- Confirm the source exists and contains `SKILL.md`.
- Create the target parent directory.
- Refuse to overwrite an existing target.
- Refuse protected names such as `.system`.
- Treat symlinks as movable entries but show them clearly in the UI.

No operation should delete skill directories. Recovery should be possible by moving folders back manually.

## Error Handling

Expected errors should be presented as actionable messages:

- Unknown profile.
- Invalid config JSON.
- Missing root.
- Skill not found.
- Target already exists.
- Permission denied.
- `npx skills update` missing or failed.

The TUI should keep the user in place after recoverable errors. Non-interactive commands should return non-zero exit codes and write concise errors to stderr.

## Testing

Unit tests should cover:

- Frontmatter parsing, including folded descriptions.
- Profile resolution and default off-root behavior.
- Custom profile load/save.
- Skill scanning for enabled and off roots.
- Staged operation planning.
- Move behavior and overwrite protection.
- CLI command parsing for list/enable/disable/profile/update.

Integration-style tests can use temporary directories and real filesystem moves. TUI rendering tests should stay light: test model update logic and view smoke output rather than full terminal screenshots.

## Migration Plan

The Python implementation can remain in git history, but the main branch should become Go-first:

1. Add the Go module and core packages beside the current Python code.
2. Reimplement CLI behavior with compatibility commands where practical.
3. Implement the Bubble Tea TUI.
4. Update README to make binary installation primary.
5. Update GitHub Actions to build/test Go and publish release artifacts.
6. Remove or archive the Python package only after Go parity is verified.

The existing published Python binary on this machine can be replaced by the Go binary after local smoke tests pass.

## Acceptance Criteria

- `skill-toggle` builds to a standalone binary on macOS.
- `skill-toggle` opens a polished Bubble Tea TUI by default.
- `skill-toggle list --profile claude` works without Python.
- Built-in profiles `agents`, `claude`, and `codex` work.
- Custom profiles persist in `~/.config/toggle-skills/roots.json`.
- Off skills are stored under `~/.config/toggle-skills/off/<profile>`.
- The TUI supports search, preview, sorting, profile switching, staged toggles, and apply.
- CLI enable/disable operations are covered by tests using temporary directories.
- GitHub Actions builds and tests the Go implementation.
- README documents binary install and the profile/off-root model.
