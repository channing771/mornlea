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
