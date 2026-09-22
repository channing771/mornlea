package archcheck_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const projectOrchestrationSkillPath = "skills/mornlea-implementation-orchestration/SKILL.md"

func TestProjectOrchestrationSkillsMatch(t *testing.T) {
	root := repositoryRoot(t)
	codex := readOrchestrationPolicyFile(t, filepath.Join(root, ".codex", projectOrchestrationSkillPath))
	claude := readOrchestrationPolicyFile(t, filepath.Join(root, ".claude", projectOrchestrationSkillPath))
	if !bytes.Equal(codex, claude) {
		t.Fatal("project-owned Codex and Claude orchestration skills must be byte-identical")
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

func TestProjectOrchestrationRetrospectivePolicy(t *testing.T) {
	root := repositoryRoot(t)
	for _, test := range []struct {
		path      string
		fragments []string
	}{
		{
			path:      ".codex/skills/mornlea-implementation-orchestration/SKILL.md",
			fragments: projectOrchestrationRetrospectiveSkillFragments(),
		},
		{
			path:      ".claude/skills/mornlea-implementation-orchestration/SKILL.md",
			fragments: projectOrchestrationRetrospectiveSkillFragments(),
		},
		{
			path:      ".codex/skills/mornlea-implementation-orchestration/references/worker-planning.md",
			fragments: projectOrchestrationRetrospectiveReferenceFragments(),
		},
		{
			path:      ".claude/skills/mornlea-implementation-orchestration/references/worker-planning.md",
			fragments: projectOrchestrationRetrospectiveReferenceFragments(),
		},
	} {
		t.Run(test.path, func(t *testing.T) {
			source := string(readOrchestrationPolicyFile(t, filepath.Join(root, filepath.FromSlash(test.path))))
			if violations := missingOrchestrationRetrospectiveFragments(source, test.fragments); len(violations) > 0 {
				t.Fatalf("%s does not preserve retrospective policy:\n%s", test.path, strings.Join(violations, "\n"))
			}
		})
	}
}

func TestProjectOrchestrationRetrospectivePolicyGuardDetectsDrift(t *testing.T) {
	for name, fragments := range map[string][]string{
		"skill":     projectOrchestrationRetrospectiveSkillFragments(),
		"reference": projectOrchestrationRetrospectiveReferenceFragments(),
	} {
		t.Run(name, func(t *testing.T) {
			valid := strings.Join(fragments, "\n")
			if violations := missingOrchestrationRetrospectiveFragments(valid, fragments); len(violations) != 0 {
				t.Fatalf("valid %s fixture produced violations: %v", name, violations)
			}
			for _, fragment := range fragments {
				t.Run(fragment, func(t *testing.T) {
					mutated := strings.Replace(valid, fragment, "", 1)
					if violations := missingOrchestrationRetrospectiveFragments(mutated, fragments); len(violations) != 1 {
						t.Fatalf("expected one retrospective-policy violation after removing %q, got %v", fragment, violations)
					}
				})
			}
		})
	}
}

func projectOrchestrationRetrospectiveSkillFragments() []string {
	return []string{
		"`tasks.md` is the sole OpenSpec plan identity and status source",
		"Do not create or maintain a flat or packet-keyed `.superpowers/sdd` progress store",
		"A failed required closeout gate leaves its node open",
		"Revise the acceptance contract explicitly before archive",
		"repository-wide producer enumeration",
	}
}

func projectOrchestrationRetrospectiveReferenceFragments() []string {
	return []string{
		"`tasks.md` remains the sole OpenSpec plan identity and status source",
		"Append per-node implementation and status evidence to the change ledger before starting the next node",
		"Batched retroactive acceptance is a recorded deviation",
		"A failed required closeout gate keeps the node open",
		"explicit acceptance-contract revision before archive",
		"repository-wide producer enumeration",
	}
}

func missingOrchestrationRetrospectiveFragments(source string, required []string) []string {
	var violations []string
	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			violations = append(violations, "missing retrospective-policy fragment: "+fragment)
		}
	}
	return violations
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
