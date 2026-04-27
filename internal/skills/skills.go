package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	SortByName         = "name"
	SortByDescSizeDesc = "desc-size-desc"
	SortByDescSizeAsc  = "desc-size-asc"
)

var ProtectedNames = []string{".system"}

// Source identifies one of the live skill roots aggregated by the tool.
type Source struct {
	Name string
	Root string
}

type Skill struct {
	Name             string
	Source           string
	DisplayName      string
	Description      string
	DescriptionChars int
	Status           string // "enabled" or "disabled"
	Path             string
	IsSymlink        bool
	Protected        bool
	// IsDuplicate is true when this skill's canonical (resolved) path was
	// already produced by an earlier entry — i.e. one source root is a
	// symlink to another (e.g. ~/.claude/skills -> ~/.agents/skills).
	IsDuplicate bool
}

type Operation struct {
	SkillName  string
	Source     string
	Direction  string // "enable" or "disable"
	SourcePath string
	TargetPath string
}

// ParseFrontmatter parses YAML frontmatter from a SKILL.md file.
// Returns (displayName, description). On read errors or missing frontmatter,
// the parent directory name is used as the display name.
func ParseFrontmatter(skillMDPath string) (string, string, error) {
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		parentDir := filepath.Base(filepath.Dir(skillMDPath))
		return parentDir, "", nil
	}
	text := string(data)
	parentDir := filepath.Base(filepath.Dir(skillMDPath))

	if !strings.HasPrefix(text, "---\n") {
		return parentDir, "", nil
	}

	end := strings.Index(text[4:], "\n---")
	if end == -1 {
		return parentDir, "", nil
	}

	block := text[4 : 4+end]

	name := parentDir
	description := ""
	inDescription := false

	lines := strings.Split(strings.TrimRight(block, "\n\r"), "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "name:") {
			parts := strings.SplitN(line, ":", 2)
			name = strings.Trim(strings.TrimSpace(parts[1]), "'\"")
			inDescription = false
			continue
		}
		if strings.HasPrefix(line, "description:") {
			parts := strings.SplitN(line, ":", 2)
			value := strings.TrimSpace(parts[1])
			if strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">") {
				inDescription = true
				description = ""
			} else {
				description = strings.Trim(value, "'\"")
				inDescription = false
			}
			continue
		}
		if inDescription {
			if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") {
				inDescription = false
				continue
			}
			description = strings.TrimSpace(description + " " + strings.TrimSpace(line))
		}
	}

	description = strings.Join(strings.Fields(description), " ")

	return name, description, nil
}

// HasSkillMD checks whether path is a directory containing SKILL.md.
func HasSkillMD(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return false
	}
	_, err = os.Stat(filepath.Join(path, "SKILL.md"))
	return err == nil
}

// ScanRoot scans a single root directory for skills, tagging each entry with
// the given source name and status. Dot-prefixed entries are skipped. Skills
// are sorted alphabetically by name (case-insensitive).
func ScanRoot(root, source, status string) ([]Skill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	var skills []Skill
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		entryPath := filepath.Join(root, name)
		if !HasSkillMD(entryPath) {
			continue
		}

		displayName, description, _ := ParseFrontmatter(filepath.Join(entryPath, "SKILL.md"))

		isSymlink := false
		if fi, err := os.Lstat(entryPath); err == nil {
			isSymlink = fi.Mode()&os.ModeSymlink != 0
		}

		protected := false
		for _, p := range ProtectedNames {
			if name == p {
				protected = true
				break
			}
		}

		skills = append(skills, Skill{
			Name:             name,
			Source:           source,
			DisplayName:      displayName,
			Description:      description,
			DescriptionChars: len(description),
			Status:           status,
			Path:             entryPath,
			IsSymlink:        isSymlink,
			Protected:        protected,
		})
	}

	return skills, nil
}

// OffRootForSource returns the per-source disabled directory under offRoot.
func OffRootForSource(offRoot, source string) string {
	return filepath.Join(offRoot, source)
}

// Scan walks every source's live root plus the corresponding disabled
// directory under offRoot, then optionally folds in legacy off roots so
// previously-disabled skills are still surfaced. Each source's legacy off
// directories are matched by index via legacyOffPerSource[i].
//
// legacyOffPerSource may be empty or shorter than sources; missing entries
// simply mean "no legacy directory for that source".
//
// After collection Scan walks the result once more and tags any entry
// whose canonical (symlink-resolved) path was already produced by an
// earlier entry as IsDuplicate=true. This is the common case where one
// source root (often ~/.claude/skills) is itself a symlink to another
// (~/.agents/skills); without de-duplication every skill would surface
// twice. The first occurrence keeps IsDuplicate=false, so callers that
// filter on it land on a unique set anchored to the earliest source in
// `sources`.
func Scan(sources []Source, offRoot string, legacyOffPerSource ...[]string) ([]Skill, error) {
	var enabled []Skill
	for _, src := range sources {
		scanned, err := ScanRoot(src.Root, src.Name, "enabled")
		if err != nil {
			return nil, err
		}
		enabled = append(enabled, scanned...)
	}

	var disabled []Skill
	seen := make(map[string]bool)
	for i, src := range sources {
		dirs := []string{OffRootForSource(offRoot, src.Name)}
		if i < len(legacyOffPerSource) {
			dirs = append(dirs, legacyOffPerSource[i]...)
		}
		for _, dir := range dirs {
			if dir == "" {
				continue
			}
			scanned, err := ScanRoot(dir, src.Name, "disabled")
			if err != nil {
				return nil, err
			}
			for _, skill := range scanned {
				key := src.Name + "/" + skill.Name
				if seen[key] {
					continue
				}
				seen[key] = true
				disabled = append(disabled, skill)
			}
		}
	}

	result := append(enabled, disabled...)
	markDuplicates(result)
	return result, nil
}

func markDuplicates(skills []Skill) {
	seen := make(map[string]bool, len(skills))
	for i := range skills {
		canonical := canonicalPath(skills[i].Path)
		if seen[canonical] {
			skills[i].IsDuplicate = true
			continue
		}
		seen[canonical] = true
	}
}

func canonicalPath(p string) string {
	if abs, err := filepath.EvalSymlinks(p); err == nil {
		return abs
	}
	return p
}

func isProtected(name string) bool {
	for _, p := range ProtectedNames {
		if name == p {
			return true
		}
	}
	return false
}

// PlanOperation builds the move that would flip a skill's status.
// liveRoot is the skill's source root; offRoot is the global off directory.
func PlanOperation(skill Skill, liveRoot, offRoot string) Operation {
	if skill.Status == "enabled" {
		return Operation{
			SkillName:  skill.Name,
			Source:     skill.Source,
			Direction:  "disable",
			SourcePath: skill.Path,
			TargetPath: filepath.Join(OffRootForSource(offRoot, skill.Source), skill.Name),
		}
	}
	return Operation{
		SkillName:  skill.Name,
		Source:     skill.Source,
		Direction:  "enable",
		SourcePath: skill.Path,
		TargetPath: filepath.Join(liveRoot, skill.Name),
	}
}

// PlanOperations builds move operations for every skill, flipping each one's
// current status. sourceRoots maps source name -> live root.
func PlanOperations(skills []Skill, sourceRoots map[string]string, offRoot string) []Operation {
	ops := make([]Operation, 0, len(skills))
	for _, s := range skills {
		ops = append(ops, PlanOperation(s, sourceRoots[s.Source], offRoot))
	}
	return ops
}

// ApplyOperation executes a single planned move. It validates source existence,
// SKILL.md presence, protected names, and target availability before renaming.
func ApplyOperation(op Operation) error {
	if isProtected(op.SkillName) {
		return fmt.Errorf("%s is protected", op.SkillName)
	}

	sourceInfo, err := os.Stat(op.SourcePath)
	if err != nil || !sourceInfo.IsDir() {
		return fmt.Errorf("source does not exist or is not a directory: %s", op.SourcePath)
	}

	skillMDPath := filepath.Join(op.SourcePath, "SKILL.md")
	mdInfo, err := os.Stat(skillMDPath)
	if err != nil || mdInfo.IsDir() {
		return fmt.Errorf("source does not contain SKILL.md: %s", op.SourcePath)
	}

	targetDir := filepath.Dir(op.TargetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	_, errStat := os.Stat(op.TargetPath)
	_, errLstat := os.Lstat(op.TargetPath)
	if errStat == nil || errLstat == nil {
		return fmt.Errorf("target already exists: %s", op.TargetPath)
	}

	if err := os.Rename(op.SourcePath, op.TargetPath); err != nil {
		return fmt.Errorf("failed to move %s to %s: %w", op.SourcePath, op.TargetPath, err)
	}

	return nil
}

// ApplyOperations applies all operations in order, stopping at the first
// failure and reporting which operation failed.
func ApplyOperations(ops []Operation) error {
	for _, op := range ops {
		if err := ApplyOperation(op); err != nil {
			return fmt.Errorf("operation failed for %s/%s: %w", op.Source, op.SkillName, err)
		}
	}
	return nil
}

// FindSkill locates a skill in scanned results by name and (optional) source.
// If source is "" and the name is unique across sources it returns that skill;
// if ambiguous it returns an error listing the candidates.
func FindSkill(skills []Skill, name, source, status string) (Skill, error) {
	var matches []Skill
	for _, s := range skills {
		if status != "" && s.Status != status {
			continue
		}
		if source != "" && s.Source != source {
			continue
		}
		if s.Name == name || s.DisplayName == name {
			matches = append(matches, s)
		}
	}
	if len(matches) == 0 {
		if source != "" {
			return Skill{}, fmt.Errorf("skill not found: %s in source %s", name, source)
		}
		return Skill{}, fmt.Errorf("skill not found: %s", name)
	}
	if len(matches) > 1 {
		var sourceNames []string
		for _, m := range matches {
			sourceNames = append(sourceNames, m.Source)
		}
		return Skill{}, fmt.Errorf("skill %s is ambiguous across sources %s; pass --source to disambiguate", name, strings.Join(sourceNames, ", "))
	}
	return matches[0], nil
}

// SortSkills sorts skills by the given mode.
func SortSkills(skills []Skill, sortMode string) []Skill {
	result := make([]Skill, len(skills))
	copy(result, skills)

	byNameThenSource := func(i, j int) bool {
		ni, nj := strings.ToLower(result[i].Name), strings.ToLower(result[j].Name)
		if ni != nj {
			return ni < nj
		}
		return result[i].Source < result[j].Source
	}

	switch sortMode {
	case SortByDescSizeDesc:
		sort.SliceStable(result, func(i, j int) bool {
			if result[i].DescriptionChars != result[j].DescriptionChars {
				return result[i].DescriptionChars > result[j].DescriptionChars
			}
			return byNameThenSource(i, j)
		})
	case SortByDescSizeAsc:
		sort.SliceStable(result, func(i, j int) bool {
			if result[i].DescriptionChars != result[j].DescriptionChars {
				return result[i].DescriptionChars < result[j].DescriptionChars
			}
			return byNameThenSource(i, j)
		})
	default:
		sort.SliceStable(result, func(i, j int) bool {
			if (result[i].Status == "enabled") != (result[j].Status == "enabled") {
				return result[i].Status == "enabled"
			}
			return byNameThenSource(i, j)
		})
	}

	return result
}

// FilterSkills filters by query (case-insensitive against name, display name,
// description, and source) and status, then sorts.
func FilterSkills(skills []Skill, query, statusFilter, sortMode string) []Skill {
	queryLower := strings.ToLower(query)

	var filtered []Skill
	for _, skill := range skills {
		if statusFilter != "all" && statusFilter != "" && skill.Status != statusFilter {
			continue
		}
		if queryLower != "" {
			haystack := strings.ToLower(skill.Name + " " + skill.DisplayName + " " + skill.Description + " " + skill.Source)
			if !strings.Contains(haystack, queryLower) {
				continue
			}
		}
		filtered = append(filtered, skill)
	}

	return SortSkills(filtered, sortMode)
}
