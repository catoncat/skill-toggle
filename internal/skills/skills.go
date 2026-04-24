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

type Skill struct {
	Name             string
	DisplayName      string
	Description      string
	DescriptionChars int
	Status           string // "enabled" or "disabled"
	Path             string
	IsSymlink        bool
	Protected        bool
}

type Operation struct {
	SkillName   string
	ProfileName string
	Direction   string // "enable" or "disable"
	SourcePath  string
	TargetPath  string
}

// ParseFrontmatter parses YAML frontmatter from a SKILL.md file.
// Returns (displayName, description). On read errors or missing frontmatter,
// the parent directory name is used as the display name, matching Python behavior.
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
			// Line that doesn't start with whitespace and contains ":"
			// signals the end of the folded description block.
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

// ScanRoot scans a single root directory for skills. Dot-prefixed entries are
// skipped. Skills are sorted alphabetically by name (case-insensitive).
func ScanRoot(root string, status string) ([]Skill, error) {
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

// Scan scans the live root and one or more disabled roots for skills.
func Scan(liveRoot string, offRoots ...string) ([]Skill, error) {
	enabled, err := ScanRoot(liveRoot, "enabled")
	if err != nil {
		return nil, err
	}

	disabled := []Skill{}
	seenDisabledNames := map[string]bool{}
	for _, offRoot := range offRoots {
		if offRoot == "" {
			continue
		}
		scanned, err := ScanRoot(offRoot, "disabled")
		if err != nil {
			return nil, err
		}
		for _, skill := range scanned {
			if seenDisabledNames[skill.Name] {
				continue
			}
			seenDisabledNames[skill.Name] = true
			disabled = append(disabled, skill)
		}
	}

	return append(enabled, disabled...), nil
}

// MoveSkill moves a skill directory between liveRoot and offRoot based on its
// current status. Protected skills are refused. Returns a message describing
// the transition.
func MoveSkill(skill *Skill, liveRoot string, offRoot string) (string, error) {
	if skill.Protected {
		return "", fmt.Errorf("%s is protected", skill.Name)
	}

	var targetRoot string
	if skill.Status == "enabled" {
		targetRoot = offRoot
	} else {
		targetRoot = liveRoot
	}

	if err := os.MkdirAll(targetRoot, 0755); err != nil {
		return "", err
	}

	target := filepath.Join(targetRoot, skill.Name)

	_, errStat := os.Stat(target)
	_, errLstat := os.Lstat(target)
	if errStat == nil || errLstat == nil {
		return "", fmt.Errorf("target already exists: %s", target)
	}

	if err := os.Rename(skill.Path, target); err != nil {
		return "", err
	}

	newStatus := "disabled"
	if skill.Status == "disabled" {
		newStatus = "enabled"
	}

	return fmt.Sprintf("%s: %s -> %s", skill.Name, skill.Status, newStatus), nil
}

func findSkillByName(root string, status string, name string) (*Skill, error) {
	skills, err := ScanRoot(root, status)
	if err != nil {
		return nil, err
	}

	for _, s := range skills {
		if s.Name == name || s.DisplayName == name {
			skill := s
			return &skill, nil
		}
	}

	return nil, fmt.Errorf("%s skill not found: %s", status, name)
}

// DisableSkill finds a skill by name or display_name in the live root and
// moves it to the disabled root.
func DisableSkill(name string, liveRoot string, offRoot string) (string, error) {
	skill, err := findSkillByName(liveRoot, "enabled", name)
	if err != nil {
		return "", err
	}
	return MoveSkill(skill, liveRoot, offRoot)
}

// EnableSkill finds a skill by name or display_name in the disabled roots and
// moves it to the live root.
func EnableSkill(name string, liveRoot string, offRoots ...string) (string, error) {
	for _, offRoot := range offRoots {
		if offRoot == "" {
			continue
		}
		skill, err := findSkillByName(offRoot, "disabled", name)
		if err == nil {
			return MoveSkill(skill, liveRoot, offRoot)
		}
	}
	return "", fmt.Errorf("disabled skill not found: %s", name)
}

// SortSkills sorts skills by the given mode:
// "name" — enabled first, then alphabetical by name (case-insensitive)
// "desc-size-desc" — largest description first, then alphabetical
// "desc-size-asc" — smallest description first, then alphabetical
func SortSkills(skills []Skill, sortMode string) []Skill {
	result := make([]Skill, len(skills))
	copy(result, skills)

	switch sortMode {
	case SortByDescSizeDesc:
		sort.Slice(result, func(i, j int) bool {
			if result[i].DescriptionChars != result[j].DescriptionChars {
				return result[i].DescriptionChars > result[j].DescriptionChars
			}
			return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
		})
	case SortByDescSizeAsc:
		sort.Slice(result, func(i, j int) bool {
			if result[i].DescriptionChars != result[j].DescriptionChars {
				return result[i].DescriptionChars < result[j].DescriptionChars
			}
			return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
		})
	default:
		sort.Slice(result, func(i, j int) bool {
			if result[i].Status == "enabled" && result[j].Status != "enabled" {
				return true
			}
			if result[i].Status != "enabled" && result[j].Status == "enabled" {
				return false
			}
			return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
		})
	}

	return result
}

// FilterSkills filters skills by query (case-insensitive match against name,
// display_name, and description) and status, then sorts by the given mode.
func FilterSkills(skills []Skill, query string, statusFilter string, sortMode string) []Skill {
	queryLower := strings.ToLower(query)

	var filtered []Skill
	for _, skill := range skills {
		if statusFilter != "all" && skill.Status != statusFilter {
			continue
		}
		if queryLower != "" {
			haystack := strings.ToLower(skill.Name + " " + skill.DisplayName + " " + skill.Description)
			if !strings.Contains(haystack, queryLower) {
				continue
			}
		}
		filtered = append(filtered, skill)
	}

	return SortSkills(filtered, sortMode)
}

func isProtected(name string) bool {
	for _, p := range ProtectedNames {
		if name == p {
			return true
		}
	}
	return false
}

// PlanOperations builds a list of move operations for the given skills. Each
// enabled skill is planned for "disable" and each disabled skill for "enable".
func PlanOperations(skills []Skill, liveRoot string, offRoot string) []Operation {
	ops := make([]Operation, 0, len(skills))
	for _, s := range skills {
		var direction, targetRoot string
		if s.Status == "enabled" {
			direction = "disable"
			targetRoot = offRoot
		} else {
			direction = "enable"
			targetRoot = liveRoot
		}
		ops = append(ops, Operation{
			SkillName:  s.Name,
			Direction:  direction,
			SourcePath: s.Path,
			TargetPath: filepath.Join(targetRoot, s.Name),
		})
	}
	return ops
}

// ApplyOperation executes a single planned move operation. It validates source
// existence, SKILL.md presence, protected names, and target availability
// before renaming the directory.
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
			return fmt.Errorf("operation failed for %s: %w", op.SkillName, err)
		}
	}
	return nil
}
