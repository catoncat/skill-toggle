package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/catoncat/skill-toggle/internal/lockfile"
)

const (
	SortByName         = "name"
	SortByDescSizeDesc = "desc-size-desc"
	SortByDescSizeAsc  = "desc-size-asc"
)

// Presence states describe how a skill name surfaces under each live root:
// "real" means an actual directory, "link" means a symlink to another root,
// "missing" means no entry at all. Used by the TUI to render the source
// column as a 3-character bitmap (a/c/x).
const (
	PresenceReal    = "real"
	PresenceLink    = "link"
	PresenceMissing = "missing"
)

var ProtectedNames = []string{".system"}

var now = time.Now

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
	// Managed is true when this skill appears in the source's
	// .skill-lock.json — i.e. it was installed by `npx skills add` and
	// can therefore be updated by `npx skills update`. Hand-placed skill
	// folders are Managed=false and are not safe to feed to update.
	Managed bool
	// LockSource is the upstream `source` field from the lockfile
	// (e.g. "vercel-labs/agent-skills"), or "" when Managed is false.
	LockSource string
	// LockEntry is the full .skill-lock.json record for this skill
	// (sourceUrl, skillPath, skillFolderHash, …) when Managed=true. The
	// freshness checker needs sourceUrl + skillPath + skillFolderHash to
	// compare against the upstream folder SHA, so we surface the whole
	// entry instead of cherry-picking fields onto Skill.
	LockEntry *lockfile.Entry
	// Presence is the per-source visibility of this skill *name* across
	// every known live root. Keyed by source name (e.g. "agents"); values
	// are PresenceReal / PresenceLink / PresenceMissing. Every row scanned
	// for the same name shares the same map (filled by markPresence after
	// the live + off pass), so the TUI can render the 3-character a/c/x
	// bitmap from any single row without cross-referencing siblings.
	Presence map[string]string
}

type Operation struct {
	SkillName  string
	Source     string
	Direction  string // "enable" or "disable"
	SourcePath string
	TargetPath string
}

// LinkOp captures one symlink mutation between live roots — used to expose
// a skill that lives under one root (agents) to another (claude) per-skill,
// without copying the directory or moving anything. The action is "link"
// (create a symlink at TargetPath pointing at SourcePath) or "unlink"
// (remove the symlink at TargetPath, leaving any real directory elsewhere
// untouched). Kept separate from Operation because the Direction language
// of enable/disable does not extend cleanly to symlink lifecycle, and the
// staged-then-apply queue is intentionally toggle-only.
type LinkOp struct {
	SkillName    string
	Action       string // "link" or "unlink"
	SourcePath   string // resolved path of the real skill (link only)
	TargetPath   string // path where the symlink lives (or will live)
	TargetSource string // name of the target source, for UI feedback
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

	// A live skill and a stale off entry can coexist (e.g. user disabled a
	// project skill, then a project sync re-installed the live source). Live
	// is the active state, so we drop the off duplicate to keep the lists
	// honest. ApplyOperation will reconcile the stale off folder when the
	// user disables the live row again.
	enabledKeys := make(map[string]bool, len(enabled))
	for _, s := range enabled {
		enabledKeys[s.Source+"/"+s.Name] = true
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
				if seen[key] || enabledKeys[key] {
					continue
				}
				seen[key] = true
				disabled = append(disabled, skill)
			}
		}
	}

	result := append(enabled, disabled...)
	markDuplicates(result)
	markManaged(result, sources)
	markPresence(result, sources)
	return result, nil
}

// markPresence fills each scanned row's Presence map with the per-source
// visibility of that skill name. Built in two passes so a name surfaced
// only by a disabled-pool entry still gets a complete (all-missing) map,
// not a sparse one — the TUI then renders identical bitmaps for every
// row sharing a name without checking siblings.
//
// Only "enabled" rows contribute real/link information; the disabled pool
// describes "skills you turned off", which is orthogonal to live-root
// visibility and would otherwise misreport a real skill as link-or-missing.
func markPresence(scanned []Skill, sources []Source) {
	if len(scanned) == 0 {
		return
	}

	names := make(map[string]struct{}, len(scanned))
	for _, s := range scanned {
		names[s.Name] = struct{}{}
	}

	presence := make(map[string]map[string]string, len(names))
	for name := range names {
		m := make(map[string]string, len(sources))
		for _, src := range sources {
			m[src.Name] = PresenceMissing
		}
		presence[name] = m
	}

	for _, s := range scanned {
		if s.Status != "enabled" {
			continue
		}
		state := PresenceReal
		if s.IsSymlink {
			state = PresenceLink
		}
		presence[s.Name][s.Source] = state
	}

	for i := range scanned {
		base := presence[scanned[i].Name]
		cp := make(map[string]string, len(base))
		for k, v := range base {
			cp[k] = v
		}
		scanned[i].Presence = cp
	}
}

// markManaged loads each source's .skill-lock.json (if present) and tags
// any matching scanned skill with Managed=true plus the upstream source.
// A missing lockfile means nothing in that source was installed via
// `npx skills add`, so every entry stays Managed=false — safe default.
func markManaged(skills []Skill, sources []Source) {
	locks := make(map[string]*lockfile.Lock, len(sources))
	for _, src := range sources {
		lock, err := lockfile.Load(lockfile.PathForSourceRoot(src.Root))
		if err != nil {
			continue
		}
		locks[src.Name] = lock
	}
	for i := range skills {
		lock, ok := locks[skills[i].Source]
		if !ok {
			continue
		}
		entry, ok := lock.Skills[skills[i].Name]
		if !ok {
			continue
		}
		skills[i].Managed = true
		skills[i].LockSource = entry.Source
		entryCopy := entry
		skills[i].LockEntry = &entryCopy
	}
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
		// A target collision on disable usually means a stale off entry from
		// a previous disable that got re-activated by an external process
		// (project sync, npx update). The live source is the truth, so if
		// the two SKILL.md files match we replace the stale off copy. If
		// they differ we refuse and surface the path so the user can pick.
		if op.Direction == "disable" {
			same, cmpErr := sameSkillMD(op.SourcePath, op.TargetPath)
			if cmpErr != nil {
				return fmt.Errorf("target already exists at %s and could not be compared: %w", op.TargetPath, cmpErr)
			}
			if same {
				if err := os.RemoveAll(op.TargetPath); err != nil {
					return fmt.Errorf("failed to remove stale off entry %s: %w", op.TargetPath, err)
				}
			} else {
				if _, err := quarantineOffConflict(op); err != nil {
					return err
				}
			}
		} else {
			return fmt.Errorf("target already exists: %s", op.TargetPath)
		}
	}

	if err := os.Rename(op.SourcePath, op.TargetPath); err != nil {
		return fmt.Errorf("failed to move %s to %s: %w", op.SourcePath, op.TargetPath, err)
	}

	return nil
}

func quarantineOffConflict(op Operation) (string, error) {
	// TargetPath is <offRoot>/<source>/<name>; keep conflicts beside offRoot
	// so the normal disabled scan does not keep treating them as state.
	offRoot := filepath.Dir(filepath.Dir(op.TargetPath))
	configDir := filepath.Dir(offRoot)
	stamp := now().Format("20060102-150405")
	base := filepath.Join(configDir, "off-conflicts-"+stamp, op.Source)

	var quarantinePath string
	for i := 0; i < 100; i++ {
		name := op.SkillName
		if i > 0 {
			name = fmt.Sprintf("%s-%02d", op.SkillName, i)
		}
		candidate := filepath.Join(base, name)
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			quarantinePath = candidate
			break
		}
	}
	if quarantinePath == "" {
		return "", fmt.Errorf("failed to choose conflict quarantine path for %s", op.TargetPath)
	}
	if err := os.MkdirAll(filepath.Dir(quarantinePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create conflict quarantine directory: %w", err)
	}
	if err := os.Rename(op.TargetPath, quarantinePath); err != nil {
		return "", fmt.Errorf("failed to quarantine stale off entry %s to %s: %w", op.TargetPath, quarantinePath, err)
	}
	return quarantinePath, nil
}

func sameSkillMD(aDir, bDir string) (bool, error) {
	a, err := os.ReadFile(filepath.Join(aDir, "SKILL.md"))
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(filepath.Join(bDir, "SKILL.md"))
	if err != nil {
		return false, err
	}
	return bytes.Equal(a, b), nil
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

// PlanLink builds a LinkOp that, when applied, creates a per-skill symlink
// at <targetRoot>/<name> pointing back at the real skill directory under
// the row's current source. Used to surface a skill in a sibling root
// (e.g. exposing agents/foo to ~/.claude/skills/foo) without moving or
// duplicating files.
func PlanLink(skill Skill, targetSource, targetRoot string) LinkOp {
	return LinkOp{
		SkillName:    skill.Name,
		Action:       "link",
		SourcePath:   skill.Path,
		TargetPath:   filepath.Join(targetRoot, skill.Name),
		TargetSource: targetSource,
	}
}

// PlanUnlink builds a LinkOp that removes a symlink at <atRoot>/<name>.
// SourcePath is empty because unlink does not need it — ApplyLinkOp only
// touches the symlink itself and leaves any real directory under another
// root alone.
func PlanUnlink(skill Skill, atSource, atRoot string) LinkOp {
	return LinkOp{
		SkillName:    skill.Name,
		Action:       "unlink",
		TargetPath:   filepath.Join(atRoot, skill.Name),
		TargetSource: atSource,
	}
}

// ApplyLinkOp executes a planned link or unlink. Protected names are
// refused, and unlink defends against deleting a real directory by
// requiring TargetPath to actually be a symlink before os.Remove.
func ApplyLinkOp(op LinkOp) error {
	if isProtected(op.SkillName) {
		return fmt.Errorf("%s is protected", op.SkillName)
	}
	switch op.Action {
	case "link":
		sourceInfo, err := os.Stat(op.SourcePath)
		if err != nil || !sourceInfo.IsDir() {
			return fmt.Errorf("source does not exist or is not a directory: %s", op.SourcePath)
		}
		skillMDPath := filepath.Join(op.SourcePath, "SKILL.md")
		if mdInfo, err := os.Stat(skillMDPath); err != nil || mdInfo.IsDir() {
			return fmt.Errorf("source does not contain SKILL.md: %s", op.SourcePath)
		}
		if _, err := os.Lstat(op.TargetPath); err == nil {
			return fmt.Errorf("target already exists: %s", op.TargetPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("target lstat failed: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(op.TargetPath), 0755); err != nil {
			return fmt.Errorf("failed to create target directory: %w", err)
		}
		if err := os.Symlink(op.SourcePath, op.TargetPath); err != nil {
			return fmt.Errorf("failed to create symlink: %w", err)
		}
		return nil
	case "unlink":
		fi, err := os.Lstat(op.TargetPath)
		if err != nil {
			return fmt.Errorf("target does not exist: %s", op.TargetPath)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refused: %s is not a symlink", op.TargetPath)
		}
		if err := os.Remove(op.TargetPath); err != nil {
			return fmt.Errorf("failed to remove symlink: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown link action: %s", op.Action)
	}
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
