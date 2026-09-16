package archcheck_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const projectOrchestrationSkillPath = "skills/mornlea-implementation-orchestration/SKILL.md"

const projectModelRouterSkillPath = "skills/adaptive-model-router"

func TestProjectOrchestrationSkillsMatch(t *testing.T) {
	root := repositoryRoot(t)
	codex := readOrchestrationPolicyFile(t, filepath.Join(root, ".codex", projectOrchestrationSkillPath))
	claude := readOrchestrationPolicyFile(t, filepath.Join(root, ".claude", projectOrchestrationSkillPath))
	if !bytes.Equal(codex, claude) {
		t.Fatal("project-owned Codex and Claude orchestration skills must be byte-identical")
	}
}

func TestProjectAdaptiveModelRouterSkillsMatch(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{
		"SKILL.md",
		"agents/openai.yaml",
		"references/capability-discovery.md",
	} {
		codex := readOrchestrationPolicyFile(t, filepath.Join(root, ".codex", projectModelRouterSkillPath, filepath.FromSlash(relative)))
		claude := readOrchestrationPolicyFile(t, filepath.Join(root, ".claude", projectModelRouterSkillPath, filepath.FromSlash(relative)))
		if !bytes.Equal(codex, claude) {
			t.Errorf("project-owned Codex and Claude model-router files must match: %s", relative)
		}
	}
}

func TestProjectAdaptiveModelRouterPolicy(t *testing.T) {
	root := repositoryRoot(t)
	router := string(readOrchestrationPolicyFile(t, filepath.Join(root, ".codex", projectModelRouterSkillPath, "SKILL.md")))
	if violations := modelRouterPolicyViolations(router); len(violations) > 0 {
		t.Fatalf("project model router does not preserve quality, token, and retrospective policy:\n%s", strings.Join(violations, "\n"))
	}

	for _, relative := range []string{
		"AGENTS.md",
		"openspec/config.yaml",
		".codex/skills/mornlea-implementation-orchestration/SKILL.md",
		".claude/skills/mornlea-implementation-orchestration/SKILL.md",
	} {
		source := string(readOrchestrationPolicyFile(t, filepath.Join(root, filepath.FromSlash(relative))))
		for _, fragment := range []string{
			"project-owned `adaptive-model-router`",
			"code quality and token efficiency",
			"Model router: no change",
		} {
			if !strings.Contains(source, fragment) {
				t.Errorf("%s does not state project router rule %q", relative, fragment)
			}
		}
	}
}

func TestProjectAdaptiveModelRouterGuardDetectsDrift(t *testing.T) {
	valid := strings.Join(modelRouterPolicyFragments(), "\n")
	if violations := modelRouterPolicyViolations(valid); len(violations) != 0 {
		t.Fatalf("valid router fixture produced violations: %v", violations)
	}

	for _, fragment := range modelRouterPolicyFragments() {
		mutated := strings.Replace(valid, fragment, "", 1)
		if violations := modelRouterPolicyViolations(mutated); len(violations) != 1 {
			t.Errorf("removing %q produced violations %v, want exactly one", fragment, violations)
		}
	}
}

func modelRouterPolicyViolations(source string) []string {
	var violations []string
	for _, fragment := range modelRouterPolicyFragments() {
		if !strings.Contains(source, fragment) {
			violations = append(violations, "missing model-router policy fragment: "+fragment)
		}
	}
	return violations
}

func modelRouterPolicyFragments() []string {
	return []string{
		"code quality and token efficiency",
		"independent axes",
		"live host capability set",
		"lowest-cost configuration that is credibly sufficient",
		"observable insufficiency",
		"over-routing",
		"under-routing",
		"Model router: no change",
	}
}

func TestProjectOpenSpecApplySkillsRouteOrchestration(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{
		".codex/skills/openspec-apply-change/SKILL.md",
		".claude/skills/openspec-apply-change/SKILL.md",
	} {
		t.Run(relative, func(t *testing.T) {
			source := string(readOrchestrationPolicyFile(t, filepath.Join(root, filepath.FromSlash(relative))))
			if violations := openSpecApplyRoutingViolations(source); len(violations) > 0 {
				t.Fatalf("%s does not preserve the project orchestration route and OpenSpec state contract:\n%s", relative, strings.Join(violations, "\n"))
			}
		})
	}
}

func TestOpenSpecApplyRoutingGuardDetectsDrift(t *testing.T) {
	valid := strings.Join([]string{
		"../mornlea-implementation-orchestration/SKILL.md",
		"openspec status --change",
		"openspec instructions apply --change",
		`state: "blocked"`,
		`state: "all_done"`,
		"contextFiles",
		"Preserve CLI-controlled `blocked`, `ready`, and `all_done` states",
	}, "\n")
	if violations := openSpecApplyRoutingViolations(valid); len(violations) != 0 {
		t.Fatalf("valid apply fixture produced violations: %v", violations)
	}

	for name, fragment := range map[string]string{
		"missing project orchestration route": "../mornlea-implementation-orchestration/SKILL.md",
		"missing status command":              "openspec status --change",
		"missing apply instructions":          "openspec instructions apply --change",
		"missing blocked state":               `state: "blocked"`,
		"missing all-done state":              `state: "all_done"`,
		"missing context files":               "contextFiles",
		"missing ready-state preservation":    "Preserve CLI-controlled `blocked`, `ready`, and `all_done` states",
	} {
		t.Run(name, func(t *testing.T) {
			mutated := strings.Replace(valid, fragment, "", 1)
			if violations := openSpecApplyRoutingViolations(mutated); len(violations) != 1 {
				t.Fatalf("expected one routing violation after removing %q, got %v", fragment, violations)
			}
		})
	}
}

func openSpecApplyRoutingViolations(source string) []string {
	required := []string{
		"../mornlea-implementation-orchestration/SKILL.md",
		"openspec status --change",
		"openspec instructions apply --change",
		`state: "blocked"`,
		`state: "all_done"`,
		"contextFiles",
		"Preserve CLI-controlled `blocked`, `ready`, and `all_done` states",
	}
	var violations []string
	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			violations = append(violations, "missing required apply-workflow fragment: "+fragment)
		}
	}
	return violations
}

func readOrchestrationPolicyFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
