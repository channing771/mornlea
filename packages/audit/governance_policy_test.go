package archcheck_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderAwareOrchestration(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{"AGENTS.md", filepath.Join("openspec", "config.yaml")} {
		text := readBaselineDoc(t, root, path)
		for _, required := range []string{
			"verified ChatGPT or Codex controller using an OpenAI model",
			"standing project authorization",
			"direct, delegated, or mixed execution",
			"without a separate per-task user request",
			"non-OpenAI or unknown-provider controller",
			"strict `subagent-driven-development`",
			"explicit user prohibition",
			"higher-priority runtime restriction",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s does not state required provider-aware policy %q", path, required)
			}
		}
	}
}

func TestAgentGuidance(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{
		filepath.Join("docs", "AGENTS.md"),
		filepath.Join("packages", "engine", "AGENTS.md"),
	} {
		text := readBaselineDoc(t, root, path)
		if !strings.Contains(text, "The root provider-aware orchestration policy applies") {
			t.Errorf("%s must defer to the root provider-aware orchestration policy", path)
		}
	}
}

func TestCodeCommentLanguagePolicy(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{
		"AGENTS.md",
		filepath.Join("docs", "AGENTS.md"),
		filepath.Join("packages", "engine", "AGENTS.md"),
		filepath.Join("docs", "test-organization.md"),
	} {
		text := readBaselineDoc(t, root, path)
		if !strings.Contains(text, "comments") || !strings.Contains(text, "English") {
			t.Errorf("%s must require English comments", path)
		}
		for _, forbidden := range []string{"comments use Chinese", "Chinese comments", "注释使用中文", "中文注释", "中文 `///`", "中文 doc 注释"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s retains obsolete Chinese-comment requirement %q", path, forbidden)
			}
		}
	}
}

func TestDelegationBudgetAndAdaptiveModelRouting(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{
		"AGENTS.md",
		filepath.Join(".codex", "skills", "mornlea-implementation-orchestration", "SKILL.md"),
		filepath.Join(".claude", "skills", "mornlea-implementation-orchestration", "SKILL.md"),
	} {
		text := readBaselineDoc(t, root, path)
		if violations := delegationPolicyViolations(text); len(violations) > 0 {
			t.Errorf("%s does not enforce cost-aware context-isolation delegation:\n%s", path, strings.Join(violations, "\n"))
		}
	}
}

func TestDelegationBudgetGuardDetectsDrift(t *testing.T) {
	valid := strings.Join([]string{
		"Main-agent execution is the default",
		"At most two subagents may run concurrently",
		"material context isolation",
		"Do not delegate merely for parallel speed",
		"`adaptive-model-router`",
		"live host capability set",
		"lowest-cost model and reasoning effort credibly sufficient",
		"Escalate one eligible step only after observable insufficiency",
	}, "\n")
	if violations := delegationPolicyViolations(valid); len(violations) != 0 {
		t.Fatalf("valid delegation fixture produced violations: %v", violations)
	}

	for _, fragment := range delegationPolicyFragments() {
		mutated := strings.Replace(valid, fragment, "", 1)
		if violations := delegationPolicyViolations(mutated); len(violations) != 1 {
			t.Errorf("removing %q produced violations %v, want exactly one", fragment, violations)
		}
	}
}

func delegationPolicyViolations(text string) []string {
	var violations []string
	for _, fragment := range delegationPolicyFragments() {
		if !strings.Contains(text, fragment) {
			violations = append(violations, "missing delegation policy fragment: "+fragment)
		}
	}
	return violations
}

func delegationPolicyFragments() []string {
	return []string{
		"Main-agent execution is the default",
		"At most two subagents may run concurrently",
		"material context isolation",
		"Do not delegate merely for parallel speed",
		"`adaptive-model-router`",
		"live host capability set",
		"lowest-cost model and reasoning effort credibly sufficient",
		"Escalate one eligible step only after observable insufficiency",
	}
}
