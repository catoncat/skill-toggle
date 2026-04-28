package skills

import (
	"os"
	"path/filepath"
	"strings"
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
	// Stale off SKILL.md differs from live source — must refuse to avoid
	// silent data loss.
	os.WriteFile(filepath.Join(off, "dup", "SKILL.md"), []byte("---\nname: dup\ndescription: stale\n---\n"), 0644)

	op := Operation{
		SkillName:  "dup",
		Source:     "agents",
		Direction:  "disable",
		SourcePath: filepath.Join(root, "dup"),
		TargetPath: filepath.Join(off, "dup"),
	}
	err := ApplyOperation(op)
	if err == nil {
		t.Fatal("expected refusal when stale off SKILL.md differs from live source")
	}
	if !strings.Contains(err.Error(), "differs") {
		t.Fatalf("expected error to mention SKILL.md diff, got: %v", err)
	}
}

func TestApplyOperationReplacesIdenticalStaleOff(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	off := filepath.Join(dir, "off", "agents")

	// writeSkill produces deterministic SKILL.md content, so writing the
	// same skill into both root and off creates the "identical stale off"
	// scenario: live and off carry the same content but live should win.
	writeSkill(t, root, "dup", "shared content")
	if err := os.MkdirAll(off, 0755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, off, "dup", "shared content")

	op := Operation{
		SkillName:  "dup",
		Source:     "agents",
		Direction:  "disable",
		SourcePath: filepath.Join(root, "dup"),
		TargetPath: filepath.Join(off, "dup"),
	}
	if err := ApplyOperation(op); err != nil {
		t.Fatalf("expected stale-off replace to succeed, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dup")); !os.IsNotExist(err) {
		t.Error("live source should have been moved into off")
	}
	if _, err := os.Stat(filepath.Join(off, "dup", "SKILL.md")); err != nil {
		t.Errorf("off entry should exist after replace: %v", err)
	}
}

func TestScanDedupsLiveAgainstStaleOff(t *testing.T) {
	dir := t.TempDir()
	codexRoot := filepath.Join(dir, "codex-skills")
	off := filepath.Join(dir, "off")

	// Live source plus a stale off entry under the same source/name.
	writeSkill(t, codexRoot, "yansu", "live copy")
	staleOff := filepath.Join(off, "codex")
	if err := os.MkdirAll(staleOff, 0755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, staleOff, "yansu", "stale copy")

	all, err := Scan([]Source{{Name: "codex", Root: codexRoot}}, off)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected stale off entry to be hidden, got %d rows: %#v", len(all), all)
	}
	if all[0].Status != "enabled" {
		t.Fatalf("expected the surviving row to be enabled (live), got %s", all[0].Status)
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

func TestScanMarksManagedFromLockfile(t *testing.T) {
	dir := t.TempDir()
	agentsHome := filepath.Join(dir, ".agents")
	agentsRoot := filepath.Join(agentsHome, "skills")
	off := filepath.Join(dir, "off")

	writeSkill(t, agentsRoot, "managed-one", "from skills add")
	writeSkill(t, agentsRoot, "handcrafted", "manually placed")

	lockBody := `{"version":3,"skills":{"managed-one":{"source":"vercel-labs/agent-skills","sourceType":"github","sourceUrl":"https://github.com/vercel-labs/agent-skills.git","skillPath":"skills/managed-one/SKILL.md","skillFolderHash":"abc"}}}`
	if err := os.WriteFile(filepath.Join(agentsHome, ".skill-lock.json"), []byte(lockBody), 0644); err != nil {
		t.Fatal(err)
	}

	all, err := Scan([]Source{{Name: "agents", Root: agentsRoot}}, off)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Skill{}
	for _, s := range all {
		byName[s.Name] = s
	}
	if !byName["managed-one"].Managed {
		t.Fatalf("managed-one should be Managed=true: %#v", byName["managed-one"])
	}
	if byName["managed-one"].LockSource != "vercel-labs/agent-skills" {
		t.Errorf("expected LockSource set, got %q", byName["managed-one"].LockSource)
	}
	if byName["handcrafted"].Managed {
		t.Fatalf("handcrafted should be Managed=false: %#v", byName["handcrafted"])
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

// linkSibling drops a per-skill symlink at <linkRoot>/<name> pointing at the
// real skill under realRoot, so tests can model the ~/.claude/skills/foo ->
// ~/.agents/skills/foo pattern that npx skills emits.
func linkSibling(t *testing.T, realRoot, linkRoot, name string) {
	t.Helper()
	if err := os.MkdirAll(linkRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(realRoot, name), filepath.Join(linkRoot, name)); err != nil {
		t.Fatal(err)
	}
}

func TestMarkPresenceAcrossThreeRoots(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents-skills")
	claudeRoot := filepath.Join(dir, "claude-skills")
	codexRoot := filepath.Join(dir, "codex-skills")
	off := filepath.Join(dir, "off")

	// agents-only skill.
	writeSkill(t, agentsRoot, "solo", "agents-only")
	// agents real + claude symlink (npx skills add pattern).
	writeSkill(t, agentsRoot, "linked", "agents real")
	linkSibling(t, agentsRoot, claudeRoot, "linked")
	// claude-only manual skill.
	writeSkill(t, claudeRoot, "claude-only", "manual claude")
	// codex disabled (lives only in off pool).
	writeSkill(t, filepath.Join(off, "codex"), "off-only", "disabled")

	sources := []Source{
		{Name: "agents", Root: agentsRoot},
		{Name: "claude", Root: claudeRoot},
		{Name: "codex", Root: codexRoot},
	}
	all, err := Scan(sources, off)
	if err != nil {
		t.Fatal(err)
	}

	// Find the canonical (non-duplicate) row for each name. Symlinked
	// duplicates may also be present but share the same Presence map.
	byName := map[string]Skill{}
	for _, s := range all {
		if s.IsDuplicate {
			continue
		}
		byName[s.Name] = s
	}

	cases := []struct {
		name string
		want map[string]string
	}{
		{"solo", map[string]string{"agents": PresenceReal, "claude": PresenceMissing, "codex": PresenceMissing}},
		{"linked", map[string]string{"agents": PresenceReal, "claude": PresenceLink, "codex": PresenceMissing}},
		{"claude-only", map[string]string{"agents": PresenceMissing, "claude": PresenceReal, "codex": PresenceMissing}},
		{"off-only", map[string]string{"agents": PresenceMissing, "claude": PresenceMissing, "codex": PresenceMissing}},
	}
	for _, c := range cases {
		got := byName[c.name].Presence
		if got == nil {
			t.Errorf("%s: missing Presence map", c.name)
			continue
		}
		for src, want := range c.want {
			if got[src] != want {
				t.Errorf("%s.Presence[%s] = %q, want %q", c.name, src, got[src], want)
			}
		}
	}
}

func TestMarkPresenceMultiRealIndependent(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents-skills")
	claudeRoot := filepath.Join(dir, "claude-skills")
	off := filepath.Join(dir, "off")

	// Two independent real copies of the same name (different SKILL.md).
	writeSkill(t, agentsRoot, "shared", "from agents")
	writeSkill(t, claudeRoot, "shared", "from claude")

	all, err := Scan([]Source{
		{Name: "agents", Root: agentsRoot},
		{Name: "claude", Root: claudeRoot},
	}, off)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 distinct rows, got %d", len(all))
	}
	for _, s := range all {
		if s.Presence["agents"] != PresenceReal {
			t.Errorf("%s/%s.Presence[agents] = %q, want real", s.Source, s.Name, s.Presence["agents"])
		}
		if s.Presence["claude"] != PresenceReal {
			t.Errorf("%s/%s.Presence[claude] = %q, want real", s.Source, s.Name, s.Presence["claude"])
		}
	}
}

func TestApplyLinkOpCreates(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents-skills")
	claudeRoot := filepath.Join(dir, "claude-skills")

	writeSkill(t, agentsRoot, "foo", "real")

	op := PlanLink(Skill{Name: "foo", Path: filepath.Join(agentsRoot, "foo")}, "claude", claudeRoot)
	if err := ApplyLinkOp(op); err != nil {
		t.Fatalf("apply link: %v", err)
	}

	target := filepath.Join(claudeRoot, "foo")
	fi, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", target, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", target)
	}
	resolved, err := os.Readlink(target)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(agentsRoot, "foo") {
		t.Errorf("symlink points at %q, want %q", resolved, filepath.Join(agentsRoot, "foo"))
	}
	// Source must still exist with SKILL.md intact.
	if _, err := os.Stat(filepath.Join(agentsRoot, "foo", "SKILL.md")); err != nil {
		t.Errorf("source SKILL.md should remain readable: %v", err)
	}
}

func TestApplyLinkOpUnlinksSymlinkOnly(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents-skills")
	claudeRoot := filepath.Join(dir, "claude-skills")

	writeSkill(t, agentsRoot, "foo", "real")
	linkSibling(t, agentsRoot, claudeRoot, "foo")

	op := PlanUnlink(Skill{Name: "foo"}, "claude", claudeRoot)
	if err := ApplyLinkOp(op); err != nil {
		t.Fatalf("apply unlink: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(claudeRoot, "foo")); !os.IsNotExist(err) {
		t.Errorf("expected claude/foo symlink to be removed, lstat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(agentsRoot, "foo", "SKILL.md")); err != nil {
		t.Errorf("real source must survive unlink: %v", err)
	}
}

func TestApplyLinkOpRefusesUnlinkOfRealDir(t *testing.T) {
	dir := t.TempDir()
	claudeRoot := filepath.Join(dir, "claude-skills")
	writeSkill(t, claudeRoot, "manual", "real claude entry")

	op := PlanUnlink(Skill{Name: "manual"}, "claude", claudeRoot)
	err := ApplyLinkOp(op)
	if err == nil {
		t.Fatal("expected refusal for unlinking a real directory")
	}
	if !strings.Contains(err.Error(), "not a symlink") {
		t.Fatalf("expected 'not a symlink' error, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(claudeRoot, "manual", "SKILL.md")); err != nil {
		t.Errorf("real dir must remain after refused unlink: %v", err)
	}
}

func TestApplyLinkOpRefusesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents-skills")
	claudeRoot := filepath.Join(dir, "claude-skills")

	writeSkill(t, agentsRoot, "foo", "real")
	writeSkill(t, claudeRoot, "foo", "preexisting claude") // collision

	op := PlanLink(Skill{Name: "foo", Path: filepath.Join(agentsRoot, "foo")}, "claude", claudeRoot)
	err := ApplyLinkOp(op)
	if err == nil {
		t.Fatal("expected refusal when target already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected 'already exists', got: %v", err)
	}
}

func TestApplyLinkOpRefusesProtected(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents-skills")
	claudeRoot := filepath.Join(dir, "claude-skills")

	os.MkdirAll(filepath.Join(agentsRoot, ".system"), 0755)
	os.WriteFile(filepath.Join(agentsRoot, ".system", "SKILL.md"), []byte("---\n---\n"), 0644)

	op := PlanLink(Skill{Name: ".system", Path: filepath.Join(agentsRoot, ".system")}, "claude", claudeRoot)
	if err := ApplyLinkOp(op); err == nil {
		t.Fatal("expected protected refusal")
	}
}

func TestApplyLinkOpRefusesMissingSKILLMD(t *testing.T) {
	dir := t.TempDir()
	agentsRoot := filepath.Join(dir, "agents-skills")
	claudeRoot := filepath.Join(dir, "claude-skills")

	if err := os.MkdirAll(filepath.Join(agentsRoot, "bogus"), 0755); err != nil {
		t.Fatal(err)
	}
	op := PlanLink(Skill{Name: "bogus", Path: filepath.Join(agentsRoot, "bogus")}, "claude", claudeRoot)
	err := ApplyLinkOp(op)
	if err == nil {
		t.Fatal("expected error for source without SKILL.md")
	}
	if !strings.Contains(err.Error(), "SKILL.md") {
		t.Fatalf("expected SKILL.md error, got: %v", err)
	}
}
