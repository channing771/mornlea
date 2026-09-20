package archcheck_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

const projectArchitectureSkillPath = "skills/mornlea-architecture/SKILL.md"

func TestProjectArchitectureSkillsMatch(t *testing.T) {
	root := repositoryRoot(t)
	codex := readOrchestrationPolicyFile(t, filepath.Join(root, ".codex", projectArchitectureSkillPath))
	claude := readOrchestrationPolicyFile(t, filepath.Join(root, ".claude", projectArchitectureSkillPath))
	if !bytes.Equal(codex, claude) {
		t.Fatal("project-owned Codex and Claude architecture skills must be byte-identical")
	}
}

func TestArchitectureSkillRetrospective(t *testing.T) {
	root := repositoryRoot(t)
	text := string(readOrchestrationPolicyFile(t, filepath.Join(root, ".codex", projectArchitectureSkillPath)))
	for _, required := range []string{
		"code and tests, canonical `openspec/specs/`",
		"`apps/mornlea-godot/` is the stable Godot project root",
		"`platform/desktop/` is the only platform-adapter family",
		"Route visual evidence by observable semantics",
		"English `*.md` plus synchronized Chinese `*.zh.md`",
		"current code, tests, or canonical specifications verify it",
		"applies across future tasks",
		"not already stated more authoritatively elsewhere",
		"contains no volatile count or task history",
		"Architecture skill: no change",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("architecture skill is missing required convention or promotion criterion %q", required)
		}
	}
}

func TestDirectoryScopedGuidancePolicy(t *testing.T) {
	root := repositoryRoot(t)
	paths := []string{
		"AGENTS.md",
		filepath.Join("docs", "agents-md-style.md"),
		filepath.Join(".codex", projectArchitectureSkillPath),
		filepath.Join(".claude", projectArchitectureSkillPath),
	}
	required := []string{
		"important directory",
		"`AGENTS.md`",
		"independent ownership, dependency, lifecycle, or validation boundary",
		"purpose",
		"directory map",
		"entry points",
		"focused validation",
		"create or update",
		"inherit the parent",
	}
	for _, relative := range paths {
		text := string(readOrchestrationPolicyFile(t, filepath.Join(root, relative)))
		for _, fragment := range required {
			if !strings.Contains(strings.ToLower(text), strings.ToLower(fragment)) {
				t.Errorf("%s does not state directory-scoped guidance rule %q", relative, fragment)
			}
		}
	}
}

func TestOrchestrationCarriesDirectoryGuidancePolicy(t *testing.T) {
	root := repositoryRoot(t)
	paths := []string{
		filepath.Join("openspec", "config.yaml"),
		filepath.Join(".codex", "skills", "mornlea-implementation-orchestration", "SKILL.md"),
		filepath.Join(".claude", "skills", "mornlea-implementation-orchestration", "SKILL.md"),
	}
	for _, relative := range paths {
		text := string(readOrchestrationPolicyFile(t, filepath.Join(root, relative)))
		for _, fragment := range []string{
			"ancestor `AGENTS.md` chain",
			"important directory",
			"same change",
			"inheritance rationale",
		} {
			if !strings.Contains(text, fragment) {
				t.Errorf("%s does not carry directory-guidance orchestration rule %q", relative, fragment)
			}
		}
	}
}
