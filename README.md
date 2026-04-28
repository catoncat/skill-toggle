# skill-toggle

`skill-toggle` is a small terminal UI for enabling and disabling local agent skills across the three live skill roots used by Codex/Claude/agents-style tooling.

It aggregates skills from `~/.agents/skills`, `~/.claude/skills`, and `~/.codex/skills` into one view, lets you stage toggles, and applies them as folder moves between each source root and a single global off directory.

## What It Does

- Aggregates skills across all three live roots — no profile to pick.
- Lazygit-style TUI: a single skill list on the left filterable by `a` (all) / `e` (enabled) / `d` (disabled), with a SKILL.md preview on the right.
- Stages toggles with `Space` and applies them in one batch with `A`. Files don't move until you commit.
- Searches by name, source, or description.
- Sorts by name, description size descending, or description size ascending.
- Runs `npx skills update` for one enabled skill or all global skills.
- Avoids deletion: toggling only renames/moves folders or symlinks.

## Install

Build from a local checkout:

```bash
make build
install -m 755 ./skill-toggle ~/.local/bin/skill-toggle
```

Or with `go install`:

```bash
go install github.com/catoncat/skill-toggle/cmd/skill-toggle@latest
```

Requires a recent Go toolchain to build (1.22+; the module currently pins 1.26.1).

## Usage

Open the TUI:

```bash
skill-toggle
```

Useful keys:

```text
a / e / d          filter list (all / enabled-only / disabled-only)
j/k or ↑/↓         move cursor
g / G              top / bottom of list
ctrl+d / ctrl+u    half page down / up
space              stage / unstage current skill
A                  apply all staged operations
C                  clear all staged operations
p / enter / tab    full-screen SKILL.md preview (toggle)
/                  search (matches name, source, description)
esc                clear search / dismiss message
.                  toggle symlinked-duplicate rows
s                  cycle sort (name → size↓ → size↑)
u                  update current enabled skill (live progress overlay)
U                  update all global skills (live progress overlay)
F                  fetch upstream sha for current skill (check, no install)
r                  rescan filesystem
?                  help overlay
q                  quit (confirms if changes are staged)
ctrl+c             hard quit
```

In the update progress overlay (modeUpdate):

```text
j/k or ↑/↓         scroll output up/down
g / G              jump to top / bottom (latest line)
esc / q            cancel & close (kills the npx process if still running)
```

Non-interactive commands:

```bash
skill-toggle list
skill-toggle list --source agents
skill-toggle list --status enabled --sort desc-size-desc --limit 20
skill-toggle list --show-linked              # include symlinked duplicates
skill-toggle enable cloudflare-global
skill-toggle disable ctf-web --source claude
skill-toggle update cloudflare-global
skill-toggle update --all
```

`--source` is required for `enable` / `disable` only when the same name exists in two or more sources. When the name is unique across sources, it can be omitted.

### Symlinked source roots

If `~/.claude/skills` is a symlink to `~/.agents/skills` (a common setup), every skill would otherwise show up twice — once per source. By default the tool resolves canonical paths and hides the duplicates, anchoring on the earliest source listed (agents → claude → codex). Pass `--show-linked` (CLI) or press `.` (TUI) to see every source's row.

### Managed vs hand-placed skills

`npx skills update` only knows how to refresh skills it installed itself. The
authoritative marker is `~/.<source>/.skill-lock.json` — anything listed
under `skills.<name>` in that file was added via `npx skills add`. Hand-placed
SKILL.md folders (e.g. ones you wrote or copied from another machine) won't
be in the lockfile and therefore can't be updated by the tool.

`skill-toggle` reads each source's lockfile during the scan and tags every
skill with a `Managed` flag. The `u` key in the TUI and `skill-toggle update
NAME` on the CLI both refuse to act on unmanaged skills with a `manual
update only` message. The bulk path (`U` / `skill-toggle update --all`)
delegates to `npx skills update -g` directly — vercel-labs/skills will only
touch its own managed entries there, so it's safe to use either way.

## Filesystem Model

Live skill roots:

```text
~/.agents/skills/<name>/SKILL.md
~/.claude/skills/<name>/SKILL.md
~/.codex/skills/<name>/SKILL.md
```

Disabled skills land under one global directory, partitioned by source so the tool can move each one back to the right root:

```text
~/.config/skill-toggle/off/<source>/<name>/SKILL.md
```

Toggle is always `os.Rename` between the live root and the off path. The tool refuses to overwrite an existing target and refuses protected names (`.system`).

`SKILL_TOGGLE_OFF_ROOT` and `SKILL_TOGGLE_CONFIG_DIR` are honored mainly for tests and isolated environments.

## Migrating from the Profile-Era Layout

Earlier versions stored disabled skills under per-profile paths:

```text
~/.config/toggle-skills/off/agents/<name>
~/.config/toggle-skills/off/claude/<name>
~/.config/toggle-skills/off/codex/<name>
~/.<source>/skills-disabled/<name>
```

These directories are still scanned read-only — anything sitting there shows up in the Disabled panel. Newly-disabled skills are written to the new layout (`~/.config/skill-toggle/off/<source>/<name>`). When you're ready, move the legacy contents over manually:

```bash
mkdir -p ~/.config/skill-toggle/off
for src in agents claude codex; do
  if [ -d ~/.config/toggle-skills/off/$src ]; then
    mv ~/.config/toggle-skills/off/$src ~/.config/skill-toggle/off/
  fi
done
```

The tool will not delete or rewrite the legacy directories on its own.

## Codex / Claude Notes

Codex and Claude read skills at session start and cache the visible skill metadata for the running session. After toggling skills, open a new session to see the updated list and the context-budget effect.

This tool manages folder-based user skills only. Plugin-provided skills and bundled system skills are controlled by Codex/Claude configuration, not by moving folders out of the live roots.

## Development

```bash
make build      # build binary
make test       # go test ./...
make vet        # go vet ./...
make run        # build + run TUI against your live roots
```

Design notes for the rewrite live in
`docs/superpowers/specs/2026-04-27-tui-redesign-aggregated-lazygit.md`.
