# skill-toggle

`skill-toggle` is a small terminal UI for enabling and disabling local agent skills by moving skill folders between a live root and a disabled root.

It was built for Codex/Claude-style skill directories where each skill lives in a folder containing `SKILL.md`.

## What It Does

- Lists enabled and disabled skills.
- Toggles a skill with `Space`.
- Previews the selected `SKILL.md`.
- Sorts by name or description size.
- Searches by name, display name, or description.
- Runs `npx skills update` for one enabled skill or all global skills.
- Avoids deletion: toggling only renames/moves folders or symlinks.

Default paths:

```text
enabled:  ~/.agents/skills
disabled: ~/.agents/skills-disabled
```

## Install

From a local checkout:

```bash
python3 -m pip install .
```

Editable install while developing:

```bash
python3 -m pip install -e .
```

Requires Python 3.9 or newer on macOS/Linux.

After the GitHub repo is published:

```bash
python3 -m pip install "git+https://github.com/catoncat/skill-toggle.git"
```

Direct one-file install is also fine:

```bash
install -m 755 skill_toggle/cli.py ~/.local/bin/skill-toggle
```

## Usage

Open the TUI:

```bash
skill-toggle
```

Useful keys:

```text
j/k or ↑/↓   move
Space        toggle selected skill
p            preview selected SKILL.md
s            cycle sort mode
/            search
a/e/d        show all / enabled / disabled
r            refresh
u            update selected enabled skill
U            update all global skills
q            quit
```

Non-interactive commands:

```bash
skill-toggle --list --sort desc-size-desc --limit 20
skill-toggle --disable ctf-web
skill-toggle --enable ctf-web
skill-toggle --update ctf-web
skill-toggle --update-all
```

Use alternate roots:

```bash
skill-toggle --root ~/.agents/skills --disabled-root ~/.agents/skills-disabled
```

Or through environment variables:

```bash
SKILL_TOGGLE_ROOT=~/.agents/skills \
SKILL_TOGGLE_DISABLED_ROOT=~/.agents/skills-disabled \
skill-toggle
```

## Codex Notes

Codex reads skills at session start and caches the visible skill metadata for the running session. After toggling skills, open a new Codex session to see the updated skills list and context-budget effect.

This tool manages folder-based user skills. Plugin-provided skills and bundled system skills are controlled by Codex configuration, not by moving folders from `~/.agents/skills`.

## Development

Run checks:

```bash
python3 -m py_compile skill_toggle/cli.py
python3 -m unittest discover -s tests -v
```

## Publishing

Create the GitHub repo and push:

```bash
git remote add origin git@github.com:catoncat/skill-toggle.git
git push -u origin main
```

If using GitHub CLI:

```bash
gh repo create catoncat/skill-toggle --public --source=. --remote=origin --push
```
