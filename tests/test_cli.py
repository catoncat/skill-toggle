import tempfile
import unittest
from pathlib import Path

from skill_toggle import cli


def write_skill(root: Path, name: str, description: str) -> None:
    skill_dir = root / name
    skill_dir.mkdir(parents=True)
    (skill_dir / "SKILL.md").write_text(
        f"---\nname: {name}\ndescription: {description}\n---\n# {name}\n",
        encoding="utf-8",
    )


class SkillToggleTests(unittest.TestCase):
    def test_enable_disable_moves_skill_between_roots(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "skills"
            disabled = Path(tmp) / "skills-disabled"
            write_skill(root, "demo", "Demo description.")

            self.assertIn("enabled -> disabled", cli.disable_skill("demo", root, disabled))
            self.assertFalse((root / "demo").exists())
            self.assertTrue((disabled / "demo" / "SKILL.md").exists())

            self.assertIn("disabled -> enabled", cli.enable_skill("demo", root, disabled))
            self.assertTrue((root / "demo" / "SKILL.md").exists())
            self.assertFalse((disabled / "demo").exists())

    def test_parse_folded_description(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            skill_dir = Path(tmp) / "folded"
            skill_dir.mkdir()
            skill_md = skill_dir / "SKILL.md"
            skill_md.write_text(
                "---\nname: folded\ndescription: >-\n  first line\n  second line\n---\n# Folded\n",
                encoding="utf-8",
            )

            name, description = cli.parse_frontmatter(skill_md)
            self.assertEqual(name, "folded")
            self.assertEqual(description, "first line second line")

    def test_sort_by_description_size(self) -> None:
        short = cli.Skill("short", "enabled", Path("/tmp/short"), "short", "x", 1, False)
        long = cli.Skill("long", "enabled", Path("/tmp/long"), "long", "xxxxx", 5, False)

        self.assertEqual([s.name for s in cli.sort_skills([short, long], "desc-size-desc")], ["long", "short"])
        self.assertEqual([s.name for s in cli.sort_skills([short, long], "desc-size-asc")], ["short", "long"])


if __name__ == "__main__":
    unittest.main()

