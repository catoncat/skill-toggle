#!/usr/bin/env python3
"""TUI for enabling and disabling local agent skills by moving folders."""

from __future__ import annotations

import argparse
import curses
import os
import subprocess
import sys
import textwrap
from dataclasses import dataclass
from pathlib import Path


DEFAULT_ROOT = Path("~/.agents/skills").expanduser()
DEFAULT_DISABLED_ROOT = Path("~/.agents/skills-disabled").expanduser()
PROTECTED_NAMES = {".system"}


@dataclass(frozen=True)
class Skill:
    name: str
    status: str
    path: Path
    display_name: str
    description: str
    description_chars: int
    is_link: bool
    protected: bool = False


def parse_frontmatter(skill_md: Path) -> tuple[str, str]:
    try:
        text = skill_md.read_text(encoding="utf-8", errors="replace")
    except OSError:
        return skill_md.parent.name, ""
    if not text.startswith("---\n"):
        return skill_md.parent.name, ""
    end = text.find("\n---", 4)
    if end == -1:
        return skill_md.parent.name, ""
    block = text[4:end]
    name = skill_md.parent.name
    description = ""
    lines = block.splitlines()
    in_description = False
    for line in lines:
        if line.startswith("name:"):
            name = line.split(":", 1)[1].strip().strip("\"'")
            in_description = False
            continue
        if line.startswith("description:"):
            value = line.split(":", 1)[1].strip()
            if value.startswith("|") or value.startswith(">"):
                in_description = True
                description = ""
            else:
                description = value.strip("\"'")
                in_description = False
            continue
        if in_description:
            if line and not line.startswith((" ", "\t")) and ":" in line:
                in_description = False
                continue
            description = f"{description} {line.strip()}".strip()
    return name, " ".join(description.split())


def has_skill_md(path: Path) -> bool:
    try:
        return path.is_dir() and (path / "SKILL.md").exists()
    except OSError:
        return False


def scan_one_root(root: Path, status: str) -> list[Skill]:
    if not root.exists():
        return []
    skills: list[Skill] = []
    for entry in sorted(root.iterdir(), key=lambda p: p.name.lower()):
        if entry.name.startswith(".") or not has_skill_md(entry):
            continue
        display_name, description = parse_frontmatter(entry / "SKILL.md")
        skills.append(
            Skill(
                name=entry.name,
                status=status,
                path=entry,
                display_name=display_name,
                description=description,
                description_chars=len(description),
                is_link=entry.is_symlink(),
                protected=entry.name in PROTECTED_NAMES,
            )
        )
    return skills


def scan(root: Path, disabled_root: Path) -> list[Skill]:
    return scan_one_root(root, "enabled") + scan_one_root(disabled_root, "disabled")


def move_skill(skill: Skill, root: Path, disabled_root: Path) -> str:
    if skill.protected:
        raise RuntimeError(f"{skill.name} is protected")
    target_root = disabled_root if skill.status == "enabled" else root
    target_root.mkdir(parents=True, exist_ok=True)
    target = target_root / skill.name
    if target.exists() or target.is_symlink():
        raise RuntimeError(f"target already exists: {target}")
    skill.path.rename(target)
    return f"{skill.name}: {skill.status} -> {'disabled' if skill.status == 'enabled' else 'enabled'}"


def disable_skill(name: str, root: Path, disabled_root: Path) -> str:
    matches = [s for s in scan_one_root(root, "enabled") if s.name == name or s.display_name == name]
    if not matches:
        raise RuntimeError(f"enabled skill not found: {name}")
    return move_skill(matches[0], root, disabled_root)


def enable_skill(name: str, root: Path, disabled_root: Path) -> str:
    matches = [s for s in scan_one_root(disabled_root, "disabled") if s.name == name or s.display_name == name]
    if not matches:
        raise RuntimeError(f"disabled skill not found: {name}")
    return move_skill(matches[0], root, disabled_root)


def sort_skills(skills: list[Skill], sort_mode: str) -> list[Skill]:
    if sort_mode == "desc-size-desc":
        return sorted(skills, key=lambda skill: (-skill.description_chars, skill.name.lower()))
    if sort_mode == "desc-size-asc":
        return sorted(skills, key=lambda skill: (skill.description_chars, skill.name.lower()))
    return sorted(skills, key=lambda skill: (skill.status != "enabled", skill.name.lower()))


def render_list(skills: list[Skill], query: str, status_filter: str, sort_mode: str) -> list[Skill]:
    query_l = query.lower()
    filtered = []
    for skill in skills:
        if status_filter != "all" and skill.status != status_filter:
            continue
        haystack = f"{skill.name} {skill.display_name} {skill.description}".lower()
        if query_l and query_l not in haystack:
            continue
        filtered.append(skill)
    return sort_skills(filtered, sort_mode)


def read_preview_lines(skill: Skill, width: int) -> list[str]:
    skill_md = skill.path / "SKILL.md"
    try:
        text = skill_md.read_text(encoding="utf-8", errors="replace")
    except OSError as exc:
        return [f"Could not read {skill_md}: {exc}"]
    wrap_width = max(30, width)
    lines: list[str] = []
    for raw_line in text.splitlines():
        if not raw_line:
            lines.append("")
            continue
        wrapped = textwrap.wrap(
            raw_line,
            width=wrap_width,
            replace_whitespace=False,
            drop_whitespace=False,
            break_long_words=False,
            break_on_hyphens=False,
        )
        lines.extend(wrapped or [""])
    return lines


def run_skills_update(skill_name: str | None = None) -> int:
    cmd = ["npx", "-y", "skills", "update"]
    if skill_name:
        cmd.append(skill_name)
    cmd.extend(["-g", "-y"])
    return subprocess.run(cmd, check=False).returncode


def run_update_interactive(stdscr: curses.window, skill_name: str | None) -> str:
    curses.def_prog_mode()
    curses.endwin()
    target = skill_name or "all global skills"
    print(f"Running: npx -y skills update {skill_name or ''} -g -y".strip())
    code = run_skills_update(skill_name)
    input(f"\nUpdate finished for {target} with exit code {code}. Press Enter to return.")
    curses.reset_prog_mode()
    stdscr.keypad(True)
    return f"update {target}: exit {code}"


def trim(text: str, width: int) -> str:
    if width <= 0:
        return ""
    if len(text) <= width:
        return text
    if width <= 1:
        return text[:width]
    return f"{text[: width - 1]}…"


def draw(stdscr: curses.window, state: dict) -> None:
    stdscr.erase()
    height, width = stdscr.getmaxyx()
    query = state["query"]
    mode = state["mode"]
    status_filter = state["filter"]
    sort_mode = state["sort"]
    visible = state["visible"]
    selected = state["selected"]
    offset = state["offset"]
    preview_offset = state["preview_offset"]
    message = state["message"]

    header = f"skill-toggle  root={state['root']}  disabled={state['disabled_root']}"
    stdscr.addstr(0, 0, trim(header, width - 1), curses.A_BOLD)
    status_line = (
        f"filter={status_filter}  sort={sort_mode}  search={query or '-'}  "
        "↑/↓ j/k move  space toggle  p preview  s sort  u update  a/e/d filter  / search  q quit"
    )
    if mode == "search":
        status_line = f"search: {query}  Enter/Esc finish  Backspace delete"
    elif mode == "preview":
        status_line = "preview  ↑/↓ j/k scroll  PgUp/PgDn page  p/Esc/q back"
    elif mode == "confirm_update":
        status_line = "Update selected enabled skill? y confirm, any other key cancel"
    elif mode == "confirm_update_all":
        status_line = "Update ALL global skills? y confirm, any other key cancel"
    stdscr.addstr(1, 0, trim(status_line, width - 1), curses.A_DIM)

    if mode == "preview" and visible:
        skill = visible[selected]
        title = f"{skill.name}  status={skill.status}  desc_chars={skill.description_chars}  path={skill.path}"
        stdscr.addstr(3, 0, trim(title, width - 1), curses.A_BOLD)
        lines = read_preview_lines(skill, width - 2)
        body_height = max(1, height - 6)
        if preview_offset >= len(lines):
            preview_offset = max(0, len(lines) - body_height)
        state["preview_offset"] = max(0, preview_offset)
        for row, line in enumerate(lines[state["preview_offset"] : state["preview_offset"] + body_height], start=4):
            stdscr.addstr(row, 0, trim(line, width - 1))
        footer = message or f"{skill.name}/SKILL.md  line {state['preview_offset'] + 1}/{max(1, len(lines))}"
        stdscr.addstr(height - 1, 0, trim(footer, width - 1), curses.A_DIM)
        stdscr.refresh()
        return

    list_height = max(1, height - 5)
    if selected < offset:
        offset = selected
    if selected >= offset + list_height:
        offset = selected - list_height + 1
    state["offset"] = max(0, offset)

    if not visible:
        stdscr.addstr(3, 0, trim("No skills match.", width - 1))
    else:
        for row, skill in enumerate(visible[offset : offset + list_height], start=3):
            idx = offset + row - 3
            marker = ">" if idx == selected else " "
            status = "ON " if skill.status == "enabled" else "OFF"
            link = "@" if skill.is_link else " "
            desc_width = max(0, width - 50)
            line = (
                f"{marker} [{status}] {link} {skill.name:<26} "
                f"{skill.description_chars:>4} {trim(skill.description, desc_width)}"
            )
            attr = curses.A_REVERSE if idx == selected else curses.A_NORMAL
            if skill.status == "disabled":
                attr |= curses.A_DIM
            stdscr.addstr(row, 0, trim(line, width - 1), attr)

    footer = message or "Changes affect new Codex/Claude sessions; current session skill metadata is already loaded."
    stdscr.addstr(height - 1, 0, trim(footer, width - 1), curses.A_DIM)
    stdscr.refresh()


def tui(root: Path, disabled_root: Path) -> None:
    def run(stdscr: curses.window) -> None:
        curses.curs_set(0)
        stdscr.keypad(True)
        state = {
            "root": str(root),
            "disabled_root": str(disabled_root),
            "skills": scan(root, disabled_root),
            "visible": [],
            "selected": 0,
            "offset": 0,
            "preview_offset": 0,
            "query": "",
            "filter": "all",
            "sort": "name",
            "mode": "normal",
            "message": "",
        }
        while True:
            state["visible"] = render_list(state["skills"], state["query"], state["filter"], state["sort"])
            if state["selected"] >= len(state["visible"]):
                state["selected"] = max(0, len(state["visible"]) - 1)
            draw(stdscr, state)
            key = stdscr.getch()
            state["message"] = ""

            if state["mode"] == "search":
                if key in (10, 13, 27):
                    state["mode"] = "normal"
                elif key in (curses.KEY_BACKSPACE, 127, 8):
                    state["query"] = state["query"][:-1]
                elif 32 <= key <= 126:
                    state["query"] += chr(key)
                    state["selected"] = 0
                    state["offset"] = 0
                continue

            if state["mode"] == "preview":
                if key in (ord("q"), ord("p"), 27):
                    state["mode"] = "normal"
                    state["preview_offset"] = 0
                elif key in (curses.KEY_DOWN, ord("j")):
                    state["preview_offset"] += 1
                elif key in (curses.KEY_UP, ord("k")):
                    state["preview_offset"] = max(0, state["preview_offset"] - 1)
                elif key == curses.KEY_NPAGE:
                    state["preview_offset"] += max(1, curses.LINES - 8)
                elif key == curses.KEY_PPAGE:
                    state["preview_offset"] = max(0, state["preview_offset"] - max(1, curses.LINES - 8))
                continue

            if state["mode"] == "confirm_update":
                if key in (ord("y"), ord("Y")) and state["visible"]:
                    skill = state["visible"][state["selected"]]
                    if skill.status != "enabled":
                        state["message"] = "enable the skill before updating it"
                    else:
                        state["message"] = run_update_interactive(stdscr, skill.name)
                        state["skills"] = scan(root, disabled_root)
                else:
                    state["message"] = "update cancelled"
                state["mode"] = "normal"
                continue

            if state["mode"] == "confirm_update_all":
                if key in (ord("y"), ord("Y")):
                    state["message"] = run_update_interactive(stdscr, None)
                    state["skills"] = scan(root, disabled_root)
                else:
                    state["message"] = "update cancelled"
                state["mode"] = "normal"
                continue

            if key in (ord("q"), 27):
                break
            if key in (curses.KEY_DOWN, ord("j")):
                state["selected"] = min(state["selected"] + 1, max(0, len(state["visible"]) - 1))
            elif key in (curses.KEY_UP, ord("k")):
                state["selected"] = max(0, state["selected"] - 1)
            elif key == curses.KEY_NPAGE:
                state["selected"] = min(state["selected"] + 10, max(0, len(state["visible"]) - 1))
            elif key == curses.KEY_PPAGE:
                state["selected"] = max(0, state["selected"] - 10)
            elif key == ord("/"):
                state["mode"] = "search"
            elif key == ord("p"):
                if state["visible"]:
                    state["mode"] = "preview"
                    state["preview_offset"] = 0
            elif key == ord("s"):
                modes = ["name", "desc-size-desc", "desc-size-asc"]
                state["sort"] = modes[(modes.index(state["sort"]) + 1) % len(modes)]
                state["selected"] = 0
                state["offset"] = 0
                state["message"] = f"sort={state['sort']}"
            elif key == ord("u"):
                if state["visible"]:
                    state["mode"] = "confirm_update"
            elif key == ord("U"):
                state["mode"] = "confirm_update_all"
            elif key == ord("a"):
                state["filter"] = "all"
                state["selected"] = 0
                state["offset"] = 0
            elif key == ord("e"):
                state["filter"] = "enabled"
                state["selected"] = 0
                state["offset"] = 0
            elif key == ord("d"):
                state["filter"] = "disabled"
                state["selected"] = 0
                state["offset"] = 0
            elif key == ord("r"):
                state["skills"] = scan(root, disabled_root)
                state["message"] = "refreshed"
            elif key == ord(" "):
                if not state["visible"]:
                    continue
                skill = state["visible"][state["selected"]]
                try:
                    state["message"] = move_skill(skill, root, disabled_root)
                    state["skills"] = scan(root, disabled_root)
                except RuntimeError as exc:
                    state["message"] = str(exc)

    curses.wrapper(run)


def print_list(skills: list[Skill], limit: int | None = None) -> None:
    rows = skills[:limit] if limit else skills
    for skill in rows:
        status = "ON " if skill.status == "enabled" else "OFF"
        link = " @" if skill.is_link else "  "
        print(f"{status}{link} {skill.name:<32} {skill.description_chars:>4} {skill.description[:100]}")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Enable/disable local agent skills by moving skill folders.")
    parser.add_argument("--root", type=Path, default=Path(os.environ.get("SKILL_TOGGLE_ROOT", DEFAULT_ROOT)))
    parser.add_argument(
        "--disabled-root",
        type=Path,
        default=Path(os.environ.get("SKILL_TOGGLE_DISABLED_ROOT", DEFAULT_DISABLED_ROOT)),
    )
    parser.add_argument("--list", action="store_true", help="Print skills without opening the TUI.")
    parser.add_argument(
        "--sort",
        choices=["name", "desc-size-desc", "desc-size-asc"],
        default="name",
        help="Sort list output.",
    )
    parser.add_argument("--limit", type=int, help="Limit --list output.")
    parser.add_argument("--enable", metavar="NAME", help="Move NAME from disabled root back into the live root.")
    parser.add_argument("--disable", metavar="NAME", help="Move NAME from live root into the disabled root.")
    parser.add_argument("--update", metavar="NAME", help="Run npx -y skills update NAME -g -y.")
    parser.add_argument("--update-all", action="store_true", help="Run npx -y skills update -g -y.")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    root = args.root.expanduser()
    disabled_root = args.disabled_root.expanduser()
    try:
        if args.enable:
            print(enable_skill(args.enable, root, disabled_root))
            print("Restart Codex/Claude or open a new session for discovery changes to apply.")
            return 0
        if args.disable:
            print(disable_skill(args.disable, root, disabled_root))
            print("Restart Codex/Claude or open a new session for discovery changes to apply.")
            return 0
        if args.update:
            code = run_skills_update(args.update)
            return code
        if args.update_all:
            code = run_skills_update(None)
            return code
        skills = render_list(scan(root, disabled_root), "", "all", args.sort)
        if args.list or not sys.stdin.isatty() or not sys.stdout.isatty():
            print_list(skills, args.limit)
            return 0
        tui(root, disabled_root)
        return 0
    except KeyboardInterrupt:
        return 130
    except (OSError, RuntimeError) as exc:
        print(f"skill-toggle: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
