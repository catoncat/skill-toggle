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
	short := Skill{Name: "short", Source: "agents", Status: "enabled", DescriptionChars: 1}
	long := Skill{Name: "long", Source: "agents", Status: "enabled", DescriptionChars: 5}

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
	enabled := Skill{Name: "b", Source: "agents", Status: "enabled"}
	disabled := Skill{Name: "a", Source: "agents", Status: "disabled"}

	result := SortSkills([]Skill{disabled, enabled}, SortByName)
	if result[0].Name != "b" || result[1].Name != "a" {
		t.Errorf("expected [b, a], got [%s, %s]", result[0].Name, result[1].Name)
	}
}

func TestFilterSkills(t *testing.T) {
	skills := []Skill{
		{Name: "alpha", Source: "agents", DisplayName: "Alpha", Description: "first", Status: "enabled"},
		{Name: "beta", Source: "claude", DisplayName: "Beta", Description: "second", Status: "disabled"},
		{Name: "gamma", Source: "agents", DisplayName: "Gamma", Description: "third beta", Status: "enabled"},
	}

	result := FilterSkills(skills, "beta", "all", SortByName)
	if len(result) != 2 {
		t.Fatalf("expected 2 matches for 'beta', got %d", len(result))
	}
	if result[0].Name != "gamma" {
		t.Errorf("expected gamma first (enabled), got %s", result[0].Name)
	}
	if result[1].Name != "beta" {
		t.Errorf("expected beta second, got %s", result[1].Name)
	}

	result = FilterSkills(skills, "", "enabled", SortByName)
	if len(result) != 2 {
		t.Fatalf("expected 2 enabled skills, got %d", len(result))
	}
}

func TestFilterSkillsBySourceText(t *testing.T) {
	skills := []Skill{
		{Name: "x", Source: "agents", Description: "x desc", Status: "enabled"},
		{Name: "y", Source: "claude", Description: "y desc", Status: "enabled"},
	}
	result := FilterSkills(skills, "claude", "all", SortByName)
	if len(result) != 1 || result[0].Source != "claude" {
		t.Fatalf("expected only claude skill via source-text search, got %#v", result)
	}
}

func TestScanAggregatesAcrossSources(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents-skills")
	claudeRoot := filepath.Join(dir, "claude-skills")
	off := filepath.Join(dir, "off")

	writeSkill(t, agentsRoot, "alpha", "agents alpha")
	writeSkill(t, claudeRoot, "beta", "claude beta")
	writeSkill(t, filepath.Join(off, "agents"), "gamma", "disabled agents")

	sources := []Source{
		{Name: "agents", Root: agentsRoot},
		{Name: "claude", Root: claudeRoot},
	}
	all, err := Scan(sources, off)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 skills, got %d: %#v", len(all), all)
	}

	bySource := map[string][]string{}
	for _, s := range all {
		bySource[s.Source] = append(bySource[s.Source], s.Name+":"+s.Status)
	}
	if len(bySource["agents"]) != 2 {
		t.Errorf("expected 2 agents skills, got %v", bySource["agents"])
	}
	if len(bySource["claude"]) != 1 {
		t.Errorf("expected 1 claude skill, got %v", bySource["claude"])
	}
}

func TestScanSameNameInTwoSourcesIsDistinct(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents-skills")
	claudeRoot := filepath.Join(dir, "claude-skills")
	off := filepath.Join(dir, "off")

	writeSkill(t, agentsRoot, "shared", "from agents")
	writeSkill(t, claudeRoot, "shared", "from claude")

	sources := []Source{
		{Name: "agents", Root: agentsRoot},
		{Name: "claude", Root: claudeRoot},
	}
	all, err := Scan(sources, off)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 distinct skills, got %d", len(all))
	}
	saw := map[string]bool{}
	for _, s := range all {
		saw[s.Source] = true
	}
	if !saw["agents"] || !saw["claude"] {
		t.Fatalf("expected both sources represented, got %#v", saw)
	}
}

func TestScanIncludesLegacyOffRoots(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents-skills")
	off := filepath.Join(dir, "off")
	legacyOff := filepath.Join(dir, "legacy-off-agents")

	writeSkill(t, legacyOff, "legacy", "legacy disabled")

	sources := []Source{{Name: "agents", Root: agentsRoot}}
	all, err := Scan(sources, off, []string{legacyOff})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 legacy skill, got %d", len(all))
	}
	if all[0].Status != "disabled" || all[0].Name != "legacy" {
		t.Fatalf("unexpected skill: %#v", all[0])
	}
}

func TestPlanAndApplyOperations(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents-skills")
	off := filepath.Join(dir, "off")

	writeSkill(t, agentsRoot, "toggle-me", "A skill to toggle.")

	sources := []Source{{Name: "agents", Root: agentsRoot}}
	all, err := Scan(sources, off)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(all))
	}

	roots := map[string]string{"agents": agentsRoot}
	ops := PlanOperations(all, roots, off)
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Direction != "disable" {
		t.Errorf("expected disable direction, got %s", ops[0].Direction)
	}
	if ops[0].Source != "agents" {
		t.Errorf("expected source agents, got %s", ops[0].Source)
	}

	if err := ApplyOperations(ops); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(agentsRoot, "toggle-me")); !os.IsNotExist(err) {
		t.Error("skill should have been moved out of agents root")
	}
	target := filepath.Join(off, "agents", "toggle-me", "SKILL.md")
	if _, err := os.Stat(target); err != nil {
		t.Errorf("skill should exist at %s: %v", target, err)
	}
}

func TestApplyOperationRefusesProtected(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	off := filepath.Join(dir, "off", "agents")
	os.MkdirAll(filepath.Join(root, ".system"), 0755)
	os.WriteFile(filepath.Join(root, ".system", "SKILL.md"), []byte("---\n---\n"), 0644)

	op := Operation{
		SkillName:  ".system",
		Source:     "agents",
		Direction:  "disable",
		SourcePath: filepath.Join(root, ".system"),
		TargetPath: filepath.Join(off, ".system"),
	}
	if err := ApplyOperation(op); err == nil {
		t.Fatal("expected protected refusal")
	}
}

func TestApplyOperationRefusesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	off := filepath.Join(dir, "off", "agents")
	writeSkill(t, root, "dup", "original")
	os.MkdirAll(filepath.Join(off, "dup"), 0755)
	os.WriteFile(filepath.Join(off, "dup", "SKILL.md"), []byte("---\n---\n"), 0644)

	op := Operation{
		SkillName:  "dup",
		Source:     "agents",
		Direction:  "disable",
		SourcePath: filepath.Join(root, "dup"),
		TargetPath: filepath.Join(off, "dup"),
	}
	if err := ApplyOperation(op); err == nil {
		t.Fatal("expected target-exists refusal")
	}
}

func TestFindSkillExactSource(t *testing.T) {
	skills := []Skill{
		{Name: "shared", Source: "agents", Status: "disabled"},
		{Name: "shared", Source: "claude", Status: "disabled"},
		{Name: "uniq", Source: "claude", Status: "enabled"},
	}

	if _, err := FindSkill(skills, "shared", "", "disabled"); err == nil {
		t.Fatal("expected ambiguity error for shared without source")
	}

	got, err := FindSkill(skills, "shared", "agents", "disabled")
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "agents" {
		t.Errorf("expected agents/shared, got %s/%s", got.Source, got.Name)
	}

	got, err = FindSkill(skills, "uniq", "", "enabled")
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "claude" {
		t.Errorf("expected unique skill, got %s/%s", got.Source, got.Name)
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

	scanned, err := ScanRoot(dir, "agents", "enabled")
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned) != 1 {
		t.Fatalf("expected 1 skill (visible only), got %d", len(scanned))
	}
	if scanned[0].Name != "visible" || scanned[0].Source != "agents" {
		t.Errorf("expected visible/agents, got %s/%s", scanned[0].Name, scanned[0].Source)
	}
}
