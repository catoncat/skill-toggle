package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// openaiYAMLContent is the standard agents/openai.yaml body that prevents
// Codex from implicitly invoking the skill while keeping explicit $skill
// invocation available.
const openaiYAMLContent = `policy:
  allow_implicit_invocation: false
`

// SetDisableModelInvocation adds or sets `disable-model-invocation: true`
// in the SKILL.md frontmatter. This flag is read by PI, Claude Code, Cursor
// and other tools that follow the Agent Skills spec extensions. It prevents
// the skill's description from being injected into the agent's system prompt,
// so the agent won't auto-trigger the skill — but explicit /skill or $skill
// invocation still works.
func SetDisableModelInvocation(skillMDPath string) error {
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		return err
	}
	text := string(data)

	// No frontmatter block at all — shouldn't happen for valid skills,
	// but guard against corruption.
	if !strings.HasPrefix(text, "---\n") {
		return nil
	}

	end := strings.Index(text[4:], "\n---")
	if end == -1 {
		return nil // malformed frontmatter
	}

	fmBlock := text[4 : 4+end]
	rest := text[4+end+4:] // everything after the closing ---

	lines := strings.Split(fmBlock, "\n")

	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "disable-model-invocation:") {
			lines[i] = "disable-model-invocation: true"
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, "disable-model-invocation: true")
	}

	newText := "---\n" + strings.Join(lines, "\n") + "\n---" + rest
	return os.WriteFile(skillMDPath, []byte(newText), 0644)
}

// RemoveDisableModelInvocation removes the `disable-model-invocation` line
// from SKILL.md frontmatter. If the line is absent this is a no-op.
func RemoveDisableModelInvocation(skillMDPath string) error {
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		return err
	}
	text := string(data)

	if !strings.HasPrefix(text, "---\n") {
		return nil
	}

	end := strings.Index(text[4:], "\n---")
	if end == -1 {
		return nil
	}

	fmBlock := text[4 : 4+end]
	rest := text[4+end+4:]

	lines := strings.Split(fmBlock, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "disable-model-invocation:") {
			continue
		}
		filtered = append(filtered, line)
	}

	newText := "---\n" + strings.Join(filtered, "\n") + "\n---" + rest
	return os.WriteFile(skillMDPath, []byte(newText), 0644)
}

// CreateOpenaiYAML creates agents/openai.yaml with
// `allow_implicit_invocation: false`. This is Codex's equivalent of
// `disable-model-invocation` — it prevents Codex from auto-triggering
// the skill while keeping explicit $skill invocation available.
func CreateOpenaiYAML(skillDir string) error {
	agentsDir := filepath.Join(skillDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(agentsDir, "openai.yaml"), []byte(openaiYAMLContent), 0644)
}

// RemoveOpenaiYAML removes agents/openai.yaml. If the agents/ directory
// is empty after removal, it is also removed. Missing files are not an error.
func RemoveOpenaiYAML(skillDir string) error {
	yamlPath := filepath.Join(skillDir, "agents", "openai.yaml")
	if err := os.Remove(yamlPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	// Best-effort cleanup of empty agents/ dir
	os.Remove(filepath.Join(skillDir, "agents"))
	return nil
}

// IsDisabledByFrontmatter reports whether SKILL.md contains
// `disable-model-invocation: true` in its frontmatter.
func IsDisabledByFrontmatter(skillMDPath string) bool {
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		return false
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return false
	}
	end := strings.Index(text[4:], "\n---")
	if end == -1 {
		return false
	}
	fmBlock := text[4 : 4+end]
	for _, line := range strings.Split(fmBlock, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "disable-model-invocation:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "disable-model-invocation:"))
			return val == "true"
		}
	}
	return false
}

// HasOpenaiYAMLDisabled reports whether agents/openai.yaml exists and
// contains `allow_implicit_invocation: false`.
func HasOpenaiYAMLDisabled(skillDir string) bool {
	data, err := os.ReadFile(filepath.Join(skillDir, "agents", "openai.yaml"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "allow_implicit_invocation: false")
}