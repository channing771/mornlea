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
		"scripts/zcode-agent.mjs",
		"scripts/zcode-agent.test.mjs",
		"scripts/zcode-live-agent.mjs",
		"scripts/zcode-live-agent.test.mjs",
		"scripts/opencode-agent.mjs",
		"scripts/opencode-agent.test.mjs",
		"scripts/opencode-live-agent.mjs",
		"scripts/opencode-live-agent.test.mjs",
	} {
		codex := readOrchestrationPolicyFile(t, filepath.Join(root, ".codex", projectModelRouterSkillPath, filepath.FromSlash(relative)))
		claude := readOrchestrationPolicyFile(t, filepath.Join(root, ".claude", projectModelRouterSkillPath, filepath.FromSlash(relative)))
		if !bytes.Equal(codex, claude) {
			t.Errorf("project-owned Codex and Claude model-router files must match: %s", relative)
		}
	}
}

func TestProjectRouterOwnsExternalBridgeRuntime(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{
		"zcode-agent.mjs",
		"zcode-agent.test.mjs",
		"zcode-live-agent.mjs",
		"zcode-live-agent.test.mjs",
		"opencode-agent.mjs",
		"opencode-agent.test.mjs",
		"opencode-live-agent.mjs",
		"opencode-live-agent.test.mjs",
	} {
		for _, skillRoot := range []string{".codex", ".claude"} {
			path := filepath.Join(root, skillRoot, projectModelRouterSkillPath, "scripts", relative)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("model-router skill runtime is missing %s: %v", path, err)
			}
		}
		legacy := filepath.Join(root, "scripts", "agents", relative)
		if _, err := os.Stat(legacy); !os.IsNotExist(err) {
			t.Errorf("external bridge runtime must not remain outside the skill: %s", legacy)
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

func TestProjectOrchestrationPrefersContextIsolation(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{
		"AGENTS.md",
		"openspec/config.yaml",
		".codex/skills/mornlea-implementation-orchestration/SKILL.md",
		".claude/skills/mornlea-implementation-orchestration/SKILL.md",
	} {
		source := string(readOrchestrationPolicyFile(t, filepath.Join(root, filepath.FromSlash(relative))))
		for _, fragment := range []string{
			"isolation-first",
			"main-context retention",
			"concise task brief",
		} {
			if !strings.Contains(source, fragment) {
				t.Errorf("%s does not state context-isolation rule %q", relative, fragment)
			}
		}
	}
}

func TestProjectRouterDefinesZCodeBridge(t *testing.T) {
	root := repositoryRoot(t)
	router := string(readOrchestrationPolicyFile(t, filepath.Join(root, ".codex", projectModelRouterSkillPath, "SKILL.md")))
	for _, fragment := range []string{
		"GLM-5.3",
		"scripts/zcode-agent.mjs",
		"router_skill_dir",
		"external isolated agent",
		"native delegation surface",
		"session ID",
		"live-probe --cwd",
		"start --cwd",
		"--reasoning high",
		"wait --worker",
		"steer --worker",
		"--command-id",
		"stable command ID",
		"`guide`",
		"`queue`",
		"`startNow`",
		"cannot inject an unsolicited turn",
		"OpenCode",
		"scripts/opencode-agent.mjs",
		"muse-spark-1.3-contributor",
		"--variant xhigh",
		"multi-turn",
		"steer",
		"queue",
		"Grok 4.6",
		"grok-4.6",
		"reasoning: xhigh",
		"Grok prior = 1.12",
		"Grok score = base score × Grok prior",
	} {
		if !strings.Contains(router, fragment) {
			t.Errorf("project model router does not define ZCode bridge rule %q", fragment)
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
		"task and capability fit: 35%",
		"validation strength: 20%",
		"lifecycle and intervention integration: 15%",
		"main-context isolation benefit: 15%",
		"total token or quota efficiency: 10%",
		"startup and expected completion latency: 5%",
		"ZCode allocation policy",
		"14:00–18:00",
		"MUST be disabled",
		"time factor = 1.00 outside the disabled window",
		"quota ratio = clamp(remaining / limit, 0, 1)",
		"remaining / limit",
		"stale quota",
		"unknown quota",
		"Z Code prior = 1.06",
		"score modifier",
		"ZCode score = base score × Z Code prior × quota factor × time factor",
		"quota factor = 0.90 + 0.20 × quota ratio",
		"older than 15 minutes",
		"confirmed zero `remaining`",
		"rate-limit response",
		"read-only account-scoped usage source",
		"High-level OpenAI design gate",
		"Before applying any quota or backend score",
		"highest eligible OpenAI model",
		"gpt-5.6-sol",
		"with `high` or `max` reasoning",
		"Z Code and every other non-OpenAI backend MUST NOT compete",
		"no compliant advanced OpenAI configuration",
		"MUST NOT silently substitute Z Code",
		"OpenCode allocation policy",
		"opencode-go",
		"muse-spark-1.3-contributor",
		"reasoning: xhigh",
		"OpenCode score",
		"quota ratio",
		"multi-turn",
		"steer",
		"queue",
		"Grok 4.6 allocation policy",
		"grok-4.6",
		"Grok prior = 1.12",
		"Grok score = base score × Grok prior",
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
