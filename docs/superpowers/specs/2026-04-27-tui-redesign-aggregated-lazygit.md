# TUI Redesign — Aggregated Skills + Lazygit Style

Status: active. Supersedes `2026-04-24-go-bubbletea-redesign-design.md`.

## Why this rewrite

The first Go cut kept the Python tool's mental model: pick a profile (agents / claude / codex), then toggle skills inside that profile. In daily use this is the wrong abstraction:

- A user does not care which root a skill lives under — they care that it's on or off.
- Skills are read from all three roots regardless (Codex/Claude session start scans them all). Splitting the UI by profile hides the actual surface area.
- Maintaining per-profile off roots scatters disabled skills across `~/.config/toggle-skills/off/agents`, `~/.config/toggle-skills/off/claude`, `~/.config/toggle-skills/off/codex` for no benefit; the tool still has to know each origin to send a skill back.

The first cut also leaned on Bubble Tea ornament — accent dialogs, metric blocks, badges with colored fills, a stats line, a search bar that pushes layout when active. The result reads as decorative rather than useful. Lazygit's discipline — bordered panels, an active panel indicator, a stable footer key strip, no chrome that doesn't earn its row — is the target.

## Goals

- One unified skill list across all three roots (`~/.agents/skills`, `~/.claude/skills`, `~/.codex/skills`).
- A single global off directory under `~/.config/skill-toggle/off/`.
- Toggling a skill sends it to off; enabling it sends it back to its origin root, with no extra state files.
- TUI styled after lazygit: two stacked panels on the left (Enabled / Disabled), one large preview panel on the right, a single key strip at the bottom.
- Staged-then-apply workflow stays. It's the safety story.
- CLI keeps `list / enable / disable / update / update-all` for scripting; profile commands are removed.

## Non-Goals

- No plugin or system-skill management.
- No SKILL.md editing.
- No package-manager replacement for `npx skills`.
- No runtime Python.
- No new config schema beyond what's needed to remember user-added source roots later (this rewrite ships with three hard-coded sources).

## Data Model

```go
type Source struct {
    Name string // "agents" | "claude" | "codex"
    Root string // expanded absolute path to ~/.<name>/skills
}

type Skill struct {
    Name             string
    Source           string // "agents" | "claude" | "codex"
    DisplayName      string
    Description      string
    DescriptionChars int
    Status           string // "enabled" | "disabled"
    Path             string
    IsSymlink        bool
    Protected        bool
}

type Operation struct {
    SkillName  string
    Source     string
    Direction  string // "enable" | "disable"
    SourcePath string
    TargetPath string
}
```

A skill's identity is `(Source, Name)`. Two skills with the same folder name in different roots are distinct — they show on separate rows with their source tag. This matches reality: `agents/edge-tts` and `codex/edge-tts` are different files.

## Filesystem Convention

```text
Live roots:
  ~/.agents/skills/<name>/SKILL.md
  ~/.claude/skills/<name>/SKILL.md
  ~/.codex/skills/<name>/SKILL.md

Disabled root (single, global):
  ~/.config/skill-toggle/off/<source>/<name>/SKILL.md
```

The path itself encodes origin. `enable cloudflare-global --source agents` looks up `~/.config/skill-toggle/off/agents/cloudflare-global` and moves it to `~/.agents/skills/cloudflare-global`. No manifest, no frontmatter mutation.

Toggle operations are still `os.Rename` — no copy, no delete. Refuse to overwrite an existing target. Refuse protected names (`.system`).

### Legacy off-root migration

The old layout lives under `~/.config/toggle-skills/off/<profile>/`. On startup, if that directory exists and the new `~/.config/skill-toggle/off/` does not, log a one-line notice and read disabled skills from the legacy path as if it were the new layout (the directory shape is identical: `off/<profile>/<name>` ≡ `off/<source>/<name>` once profile == source). On the first successful disable in the new layout, leave the old tree alone — the user can `mv` it themselves. We will not auto-rewrite the user's filesystem.

If both directories exist, prefer the new path; the legacy path is read for additional disabled skills not already present under the new path.

## TUI

```
┌─Enabled (18)──────────────────────────┐┌─SKILL.md / cloudflare-global────────┐
│> ON  cloudflare-global   agents  141  ││ name: cloudflare-global              │
│  ON  session-wrap        claude  130  ││ source: agents                       │
│  ON  edge-tts            codex   88   ││ status: enabled                      │
│  ON  network-diagnosis   agents  205  ││ path: ~/.agents/skills/cloudflare... │
│                                       ││                                      │
├─Disabled (42)─────────────────────────┤│ ## Description                       │
│  OFF ctf-web             claude  882  ││ Cloudflare global config helpers...  │
│ ~OFF memo                agents  74   ││                                      │
│  OFF browser-harness     agents  210  ││ ## Usage                             │
│                                       ││ ...                                  │
│                                       ││                                      │
└───────────────────────────────────────┘└──────────────────────────────────────┘
 tab: switch panel  j/k: move  space: stage  A: apply  /: search  p: full preview  ?: help  q: quit
```

### Panels

- **Enabled** (top-left): all enabled skills across the three sources, sorted by name (case-insensitive). Each row: `marker  ON  <name padded>  <source tag>  <desc-chars>`. The marker is `>` when this row is the cursor in the active panel, `~` when staged for disable.
- **Disabled** (bottom-left): all disabled skills under the off-root (and the legacy path), same row format with `OFF`. `~` here means staged for enable.
- **Preview** (right): selected skill's metadata header (name / source / status / path / desc_chars / symlink) followed by the full SKILL.md body. Scrollable independently. No markdown rendering in this rewrite — plain text only — but section headings are recognized (`#`/`##`/`###`) and rendered with a single accent so the text isn't a wall of gray.

### Active panel

The active panel's border uses `BorderStrong`; inactive panels use `Border`. The cursor row is highlighted only in the active panel; the other panel shows its cursor as a dimmed `>`. This is the lazygit signal.

### Key strip

A single line at the bottom. No mode banners. No `NORMAL` / `PREVIEW` labels — the panels themselves show what's active.

### Modes

- **normal**: panel-driven navigation, default.
- **search**: `/` opens an inline filter at the top of whichever side panel is active. Filter is applied to both Enabled and Disabled simultaneously. `Esc` clears, `Enter` keeps the filter and returns to normal.
- **preview-fullscreen**: `p` (or `Enter`) expands the right preview to full width, hiding the left panels. `q` / `Esc` / `p` returns.
- **help**: `?` overlays a key reference, dismiss with any key.
- **confirm**: small inline prompt at the bottom for destructive actions (`A` apply, `u`/`U` update, `q` with staged ops). No centered modal box.

### Keymap

```
Global
  q              quit (confirm if staged)
  ?              toggle help overlay
  ctrl+c         hard quit
  /              search (filter both panels)
  esc            cancel search / dismiss overlay

Navigation
  j / down       cursor down in active panel
  k / up         cursor up in active panel
  g              top of panel
  G              bottom of panel
  ctrl+d         half page down
  ctrl+u         half page up
  tab            next panel (Enabled → Disabled → Enabled)
  shift+tab      prev panel
  H              focus Enabled panel
  L              focus Disabled panel

Skill actions
  space          stage current skill (toggle)
  A              apply all staged ops
  C              clear all staged ops
  p / enter      open full-screen preview
  u              update current enabled skill (npx skills update <name>)
  U              update all global skills
  r              rescan filesystem

Sort
  s              cycle sort: name → desc-size desc → desc-size asc → name
```

The command palette (`:`) is dropped. It was vestigial: `enable`/`disable`/`profile` are all reachable via the panels and the CLI is the right place for scripting.

## CLI

```bash
skill-toggle                       # opens TUI
skill-toggle list                  # all sources, all statuses
skill-toggle list --source agents
skill-toggle list --status enabled
skill-toggle list --sort desc-size-desc --limit 20
skill-toggle enable cloudflare-global --source agents
skill-toggle disable ctf-web --source claude
skill-toggle update cloudflare-global
skill-toggle update --all
```

Rules:

- `--source` is required for `enable` / `disable` only when the name is ambiguous across sources. If the name is unique across all sources, `--source` may be omitted.
- The legacy flags `--profile / --profiles / --add-root / --set-default / --remove-root / --root / --disabled-root` are removed entirely. They were paths into a model that no longer exists. The CHANGELOG/README will call this out.
- `SKILL_TOGGLE_OFF_ROOT` env var overrides the global off path (mostly for tests).

## Architecture

```text
cmd/skill-toggle/main.go
internal/cli       cobra command surface (no profile commands)
internal/config    config dir resolution, source list, off-root resolution, legacy off-root detection
internal/skills    Skill / Operation, Scan(sources, offRoot), frontmatter, ApplyOperation
internal/update    npx skills update wrapper (unchanged)
internal/tui
  model.go         Model, NewModel, Init
  update.go        Update loop, message handling
  view.go          View(), top-level layout
  view_panels.go   panel rendering (enabled/disabled/preview)
  view_help.go     help overlay
  keymap.go        key bindings (using bubbles/key)
internal/ui        lipgloss styles, trim/pad helpers — slimmed down
```

`internal/tui/tui.go` is split. The 1419-line single file is the main code-quality complaint and won't survive this rewrite.

## Testing

Unit tests cover:

- Frontmatter parsing (existing).
- Source-aware Scan: three temp source roots + one temp off-root, expect aggregated output with correct `Source` tags and statuses.
- Same-name across sources: two skills named `edge-tts` in different sources both surface, both keep their identity.
- Operation planning and apply: enable/disable round-trips through `~/.config/skill-toggle/off/<source>/<name>`.
- Legacy off-root migration: temp dir mimicking `~/.config/toggle-skills/off/agents/<name>`, expect those skills surfaced as `Source: agents, Status: disabled`.
- TUI: smoke test of Update on a fixed-size WindowSizeMsg, ensure panels render without panic; assert key bindings produce the expected mode/cursor changes. No screenshot diffing.

## Acceptance

- `skill-toggle` opens the lazygit-style TUI by default.
- Enabled and Disabled panels show aggregated content from all three sources.
- Disabled skills land in `~/.config/skill-toggle/off/<source>/<name>`; enabling sends them back.
- Legacy `~/.config/toggle-skills/off/<profile>/<name>` skills appear in the Disabled panel without manual migration.
- `skill-toggle list` works without TUI; profile flags are gone and produce a clear error if used.
- `go vet ./...` clean. `go test ./...` passes. `make build` produces a working binary.
- README documents the unified model, the off-root path, the keymap, and the legacy migration note.

## Out of scope (future)

- Markdown rendering in the preview panel (glamour or similar).
- User-defined extra source roots beyond the three built-ins.
- Bulk multi-select with visual selection range (lazygit-style `v` mode).
- A real command palette if it earns the line.
