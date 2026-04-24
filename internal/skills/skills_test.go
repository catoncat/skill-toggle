package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkill(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeFoldedSkill(t *testing.T, root, name string, lines ...string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: >-\n"
	for _, line := range lines {
		content += "  " + line + "\n"
	}
	content += "---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestParseFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "demo", "Demo description.")

	name, desc, err := ParseFrontmatter(filepath.Join(dir, "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" {
		t.Errorf("expected name 'demo', got '%s'", name)
	}
	if desc != "Demo description." {
		t.Errorf("expected description 'Demo description.', got '%s'", desc)
	}
}

func TestParseFoldedDescription(t *testing.T) {
	dir := t.TempDir()
	writeFoldedSkill(t, dir, "folded", "first line", "second line")

	name, desc, err := ParseFrontmatter(filepath.Join(dir, "folded", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if name != "folded" {
		t.Errorf("expected name 'folded', got '%s'", name)
	}
	if desc != "first line second line" {
		t.Errorf("expected description 'first line second line', got '%s'", desc)
	}
}

func TestParseFrontmatterMissing(t *testing.T) {
	dir := t.TempDir()
	dirPath := filepath.Join(dir, "no-frontmatter")
	os.MkdirAll(dirPath, 0755)
	os.WriteFile(filepath.Join(dirPath, "SKILL.md"), []byte("# Just a heading\n"), 0644)

	name, _, err := ParseFrontmatter(filepath.Join(dirPath, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if name != "no-frontmatter" {
		t.Errorf("expected folder name 'no-frontmatter', got '%s'", name)
	}
}

func TestParseFrontmatterQuotedName(t *testing.T) {
	dir := t.TempDir()
	dirPath := filepath.Join(dir, "quoted")
	os.MkdirAll(dirPath, 0755)
	os.WriteFile(filepath.Join(dirPath, "SKILL.md"),
		[]byte("---\nname: \"Display Name\"\ndescription: A description\n---\n"), 0644)

	name, desc, err := ParseFrontmatter(filepath.Join(dirPath, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if name != "Display Name" {
		t.Errorf("expected name 'Display Name', got '%s'", name)
	}
	if desc != "A description" {
		t.Errorf("expected description 'A description', got '%s'", desc)
	}
}

func TestSortByDescriptionSize(t *testing.T) {
	short := Skill{Name: "short", Status: "enabled", Path: "/tmp/short", DescriptionChars: 1}
	long := Skill{Name: "long", Status: "enabled", Path: "/tmp/long", DescriptionChars: 5}

	names := func(skills []Skill) []string {
		out := make([]string, len(skills))
		for i, s := range skills {
			out[i] = s.Name
		}
		return out
	}

	result := SortSkills([]Skill{short, long}, SortByDescSizeDesc)
	if names(result)[0] != "long" || names(result)[1] != "short" {
		t.Errorf("desc-size-desc: expected [long, short], got %v", names(result))
	}

	result = SortSkills([]Skill{short, long}, SortByDescSizeAsc)
	if names(result)[0] != "short" || names(result)[1] != "long" {
		t.Errorf("desc-size-asc: expected [short, long], got %v", names(result))
	}
}

func TestSortByNameEnabledFirst(t *testing.T) {
	enabled := Skill{Name: "b", Status: "enabled", Path: "/tmp/b"}
	disabled := Skill{Name: "a", Status: "disabled", Path: "/tmp/a"}

	result := SortSkills([]Skill{disabled, enabled}, SortByName)
	if result[0].Name != "b" || result[1].Name != "a" {
		t.Errorf("expected [b, a], got [%s, %s]", result[0].Name, result[1].Name)
	}
}

func TestFilterSkills(t *testing.T) {
	skills := []Skill{
		{Name: "alpha", DisplayName: "Alpha", Description: "first", Status: "enabled"},
		{Name: "beta", DisplayName: "Beta", Description: "second", Status: "disabled"},
		{Name: "gamma", DisplayName: "Gamma", Description: "third beta", Status: "enabled"},
	}

	result := FilterSkills(skills, "beta", "all", SortByName)
	if len(result) != 2 {
		t.Fatalf("expected 2 matches for 'beta', got %d", len(result))
	}
	// SortByName puts enabled skills first, so gamma (enabled, contains "beta" in desc)
	// comes before beta (disabled, name match).
	if result[0].Name != "gamma" {
		t.Errorf("expected gamma first (enabled before disabled), got %s", result[0].Name)
	}
	if result[1].Name != "beta" {
		t.Errorf("expected beta second, got %s", result[1].Name)
	}

	result = FilterSkills(skills, "", "enabled", SortByName)
	if len(result) != 2 {
		t.Fatalf("expected 2 enabled skills, got %d", len(result))
	}
}

func TestEnableDisableMovesSkill(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	disabled := filepath.Join(dir, "skills-disabled")

	writeSkill(t, root, "demo", "Demo description.")

	msg, err := DisableSkill("demo", root, disabled)
	if err != nil {
		t.Fatal(err)
	}
	if msg != "demo: enabled -> disabled" {
		t.Errorf("unexpected message: %s", msg)
	}
	if _, err := os.Stat(filepath.Join(root, "demo")); !os.IsNotExist(err) {
		t.Error("demo dir should not exist in live root after disable")
	}
	if _, err := os.Stat(filepath.Join(disabled, "demo", "SKILL.md")); err != nil {
		t.Errorf("SKILL.md should exist in disabled root: %v", err)
	}

	msg, err = EnableSkill("demo", root, disabled)
	if err != nil {
		t.Fatal(err)
	}
	if msg != "demo: disabled -> enabled" {
		t.Errorf("unexpected message: %s", msg)
	}
	if _, err := os.Stat(filepath.Join(root, "demo", "SKILL.md")); err != nil {
		t.Errorf("SKILL.md should exist in live root after enable: %v", err)
	}
}

func TestEnableSkillNotFound(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	disabled := filepath.Join(dir, "skills-disabled")

	_, err := EnableSkill("nonexistent", root, disabled)
	if err == nil {
		t.Fatal("expected error for nonexistent skill")
	}
}

func TestDisableProtectedSkill(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	disabled := filepath.Join(dir, "skills-disabled")

	os.MkdirAll(filepath.Join(root, ".system"), 0755)
	os.WriteFile(filepath.Join(root, ".system", "SKILL.md"), []byte("---\n---\n"), 0644)

	_, err := DisableSkill(".system", root, disabled)
	if err == nil {
		t.Fatal("expected error for protected skill")
	}
}

func TestMoveSkillTargetExists(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	disabled := filepath.Join(dir, "skills-disabled")

	writeSkill(t, root, "dup", "Some skill.")
	os.MkdirAll(filepath.Join(disabled, "dup"), 0755)
	os.WriteFile(filepath.Join(disabled, "dup", "SKILL.md"), []byte("---\n---\n"), 0644)

	_, err := DisableSkill("dup", root, disabled)
	if err == nil {
		t.Fatal("expected error for target already existing")
	}
}

func TestPlanAndApplyOperations(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	disabled := filepath.Join(dir, "skills-disabled")

	writeSkill(t, root, "toggle-me", "A skill to toggle.")

	all, err := Scan(root, disabled)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(all))
	}

	ops := PlanOperations(all, root, disabled)
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Direction != "disable" {
		t.Errorf("expected disable direction, got %s", ops[0].Direction)
	}

	err = ApplyOperations(ops)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "toggle-me")); !os.IsNotExist(err) {
		t.Error("skill should have been moved out of root")
	}
	if _, err := os.Stat(filepath.Join(disabled, "toggle-me", "SKILL.md")); err != nil {
		t.Error("skill should exist in disabled root")
	}
}

func TestScanIncludesMultipleDisabledRoots(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	newOff := filepath.Join(dir, "off-new")
	legacyOff := filepath.Join(dir, "skills-disabled")

	writeSkill(t, root, "enabled-skill", "enabled")
	writeSkill(t, legacyOff, "legacy-off", "legacy")

	all, err := Scan(root, newOff, legacyOff)
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]Skill{}
	for _, skill := range all {
		byName[skill.Name] = skill
	}
	if byName["enabled-skill"].Status != "enabled" {
		t.Fatalf("enabled skill missing or wrong status: %#v", byName["enabled-skill"])
	}
	if byName["legacy-off"].Status != "disabled" {
		t.Fatalf("legacy off skill missing or wrong status: %#v", byName["legacy-off"])
	}
}

func TestHasSkillMD(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "valid", "desc")

	if !HasSkillMD(filepath.Join(dir, "valid")) {
		t.Error("expected HasSkillMD true for valid skill dir")
	}
	if HasSkillMD(filepath.Join(dir, "nonexistent")) {
		t.Error("expected HasSkillMD false for nonexistent dir")
	}
}

func TestScanRootSkipsDotFiles(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "visible", "desc")
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0755)
	os.WriteFile(filepath.Join(dir, ".hidden", "SKILL.md"), []byte("---\n---\n"), 0644)

	skills, err := ScanRoot(dir, "enabled")
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (visible only), got %d", len(skills))
	}
	if skills[0].Name != "visible" {
		t.Errorf("expected 'visible', got '%s'", skills[0].Name)
	}
}
