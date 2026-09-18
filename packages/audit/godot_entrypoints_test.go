package archcheck_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The Godot pilot entry points are opt-in gates: they may touch the
// Godot/Python runtime only when a developer or the dedicated CI job invokes
// them explicitly. This guard pins both halves of that boundary: the entry
// points must exist with the exact verified script invocations, and every
// legacy make/CI entry (build, test, run, companion-agent-check) must stay
// free of any scripts/godot probe or godot-* dependency edge.
var godotEntrypointTargets = []string{
	"godot-asset-check",
	"godot-project-check",
	"godot-python-check",
	"godot-input-check",
	"godot-camera-check",
	"godot-target-check",
	"godot-entity-check",
	"godot-environment-check",
	"godot-hud-check",
	"godot-disconnect-check",
	"godot-smoke",
	"godot-terrain-check",
	"godot-capability-check",
	"godot-playable-smoke",
}

// godotEntrypointRecipes pins the exact script invocation each gate must run;
// godot-smoke additionally requires the isolated Python flag so the smoke
// cycle can never silently fall back to a host interpreter.
var godotEntrypointRecipes = map[string][]string{
	"godot-asset-check":   {"scripts/godot/sync-assets.sh --check"},
	"godot-project-check": {"scripts/godot/validate-project.sh"},
	"godot-python-check":  {"scripts/godot/python-check.sh --locked"},
	"godot-input-check":   {"scripts/godot/input-check.sh"},
	"godot-camera-check":  {"scripts/godot/camera-check.sh"},
	"godot-target-check":  {"scripts/godot/target-check.sh"},
	"godot-entity-check": {
		"$(CARGO) test -p mornlea_godot entity_snapshot --locked",
		"scripts/godot/entity-check.sh",
	},
	"godot-environment-check": {"scripts/godot/environment-check.sh"},
	"godot-hud-check":         {"scripts/godot/hud-check.sh"},
	"godot-disconnect-check":  {"go test ./packages/client/runtime", "Disconnect|Overflow|Shutdown"},
	"godot-smoke":             {"scripts/godot/smoke.sh --iterations 100", "--isolated-python"},
	"godot-terrain-check":     {"scripts/godot/godot-terrain-check.sh"},
	"godot-capability-check":  {"scripts/godot/capability-check.sh"},
	"godot-playable-smoke":    {"scripts/godot/playable-smoke.sh --duration 300s"},
}

const godotEntrypointTestRootEnv = "MORNLEA_GODOT_ENTRYPOINT_TEST_ROOT"

func TestGodotIsOptionalForLegacyBuild(t *testing.T) {
	// Mutation re-runs redirect both baseline reads to a temporary root, so
	// each probe below verifies that this guard rejects one isolated
	// regression instead of re-reading the real repository.
	root := os.Getenv(godotEntrypointTestRootEnv)
	mutationRun := root != ""
	if !mutationRun {
		root = repositoryRoot(t)
	}
	makefile := readBaselineDoc(t, root, "Makefile")
	workflow := readBaselineDoc(t, root, filepath.Join(".github", "workflows", "ci.yml"))
	if violations := godotMakefileEntrypointViolations(t, makefile); len(violations) > 0 {
		t.Errorf("Godot Makefile entry-point contract has %d violations:\n%s", len(violations), strings.Join(violations, "\n"))
	}
	if violations := godotWorkflowEntrypointViolations(workflow); len(violations) > 0 {
		t.Errorf("Godot CI entry-point contract has %d violations:\n%s", len(violations), strings.Join(violations, "\n"))
	}
	if !mutationRun {
		testGodotEntrypointMutations(t, makefile, workflow)
	}
}

func godotMakefileEntrypointViolations(t *testing.T, makefile string) []string {
	t.Helper()
	var violations []string
	rules := parseMakeRules(makefile)
	declared := make(map[string]bool)
	for _, rule := range rules {
		for _, target := range rule.targets {
			declared[target] = true
		}
	}
	for _, target := range godotEntrypointTargets {
		if !declared[target] {
			violations = append(violations, fmt.Sprintf("Makefile is missing the optional godot gate %s", target))
			continue
		}
		if !makeTargetIsPhony(makefile, target) {
			violations = append(violations, fmt.Sprintf("Makefile .PHONY does not register %s", target))
		}
		if !strings.Contains(makefile, "make "+target) {
			violations = append(violations, fmt.Sprintf("make help does not list %s", target))
		}
		recipe := makeTargetRecipe(t, makefile, target)
		for _, required := range godotEntrypointRecipes[target] {
			if !strings.Contains(recipe, required) {
				violations = append(violations, fmt.Sprintf("godot gate %s must invoke %q", target, required))
			}
		}
	}

	godotGate := make(map[string]bool, len(godotEntrypointTargets))
	for _, target := range godotEntrypointTargets {
		godotGate[target] = true
	}
	for _, rule := range rules {
		for _, target := range rule.targets {
			if strings.HasPrefix(target, ".") {
				// Special make targets such as .PHONY register bookkeeping
				// metadata, not build-order edges; the mandatory .PHONY
				// registration of the godot gates must not be read as a
				// dependency.
				continue
			}
			for _, prerequisite := range rule.prerequisites {
				if strings.HasPrefix(prerequisite, "godot-") {
					if godotGate[target] {
						violations = append(violations, fmt.Sprintf("godot gate %s must stay a leaf target, but depends on %s", target, prerequisite))
					} else {
						violations = append(violations, fmt.Sprintf("legacy target %s must not depend on the optional godot gate %s", target, prerequisite))
					}
				}
			}
			if godotGate[target] {
				continue
			}
			for _, recipeLine := range rule.recipe {
				if strings.Contains(recipeLine, "scripts/godot") {
					violations = append(violations, fmt.Sprintf("legacy target %s probes the Godot runtime in its recipe: %s", target, strings.TrimSpace(recipeLine)))
				}
			}
		}
	}
	violations = append(violations, godotMakefileScriptReferenceViolations(makefile)...)
	slices.Sort(violations)
	return violations
}

// godotMakefileScriptReferenceViolations is a whole-file invariant over every
// non-comment Makefile line: the substring scripts/godot may appear only in a
// rule line of exactly one registered godot gate target or in the
// tab-indented recipe lines directly following such a rule. It deliberately
// stays grammar-light instead of teaching the rule parser more make syntax —
// double-colon rules, inline semicolon recipes, and especially parse-time
// $(shell ...) assignments (which execute on every make invocation) must not
// open a silent probe path into the Godot runtime.
func godotMakefileScriptReferenceViolations(makefile string) []string {
	godotGate := make(map[string]bool, len(godotEntrypointTargets))
	for _, target := range godotEntrypointTargets {
		godotGate[target] = true
	}
	var violations []string
	inGodotRecipe := false
	for _, line := range strings.Split(makefile, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			// Comment lines are inert prose and never terminate a recipe.
			continue
		}
		if strings.HasPrefix(line, "\t") {
			if !inGodotRecipe && strings.Contains(line, "scripts/godot") {
				violations = append(violations, fmt.Sprintf("tab recipe line outside the godot gate recipes references scripts/godot: %s", strings.TrimSpace(line)))
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			// Blank lines do not terminate a recipe, mirroring
			// `makeTargetRecipe`.
			continue
		}
		targets, _, ok := splitMakeRuleLine(line)
		inGodotRecipe = ok && len(targets) == 1 && godotGate[targets[0]]
		if !inGodotRecipe && strings.Contains(line, "scripts/godot") {
			violations = append(violations, fmt.Sprintf("Makefile line references scripts/godot outside the registered godot gate rules: %s", strings.TrimSpace(line)))
		}
	}
	return violations
}

type makeRule struct {
	targets       []string
	prerequisites []string
	recipe        []string
}

// parseMakeRules extracts one entry per rule line: the target names (a rule
// may declare several targets), the raw prerequisite tokens, and the
// tab-indented recipe that follows it. Variable assignments ("name := value")
// are skipped. Recipe scanning mirrors `makeTargetRecipe`: blank and comment
// lines do not terminate a recipe.
func parseMakeRules(makefile string) []makeRule {
	lines := strings.Split(makefile, "\n")
	var rules []makeRule
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		targets, prerequisites, ok := splitMakeRuleLine(line)
		if !ok {
			continue
		}
		rule := makeRule{targets: targets, prerequisites: prerequisites}
		next := index + 1
		for next < len(lines) {
			candidate := lines[next]
			if strings.HasPrefix(candidate, "\t") {
				rule.recipe = append(rule.recipe, candidate)
				next++
				continue
			}
			if candidate == "" || strings.HasPrefix(candidate, "#") {
				next++
				continue
			}
			break
		}
		rules = append(rules, rule)
		index = next - 1
	}
	return rules
}

func splitMakeRuleLine(line string) ([]string, []string, bool) {
	if line == "" || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "#") {
		return nil, nil, false
	}
	colon := strings.Index(line, ":")
	if colon <= 0 {
		return nil, nil, false
	}
	if colon+1 < len(line) && (line[colon+1] == '=' || line[colon+1] == ':') {
		return nil, nil, false
	}
	targets := strings.Fields(line[:colon])
	if len(targets) == 0 {
		return nil, nil, false
	}
	return targets, strings.Fields(line[colon+1:]), true
}

var workflowJobHeader = regexp.MustCompile(`^ {2}[A-Za-z0-9][A-Za-z0-9_-]*:$`)

// parseWorkflowJobs splits the workflow into top-level job blocks keyed by job
// name. A job header is a two-space-indented "name:" line under `jobs:`; the
// block extends to the next job header or end of file. Only lines after the
// top-level `jobs:` key participate, so sibling keys such as `on: push:` are
// never mistaken for jobs.
func parseWorkflowJobs(workflow string) map[string]string {
	jobs := make(map[string]string)
	current := ""
	var block []string
	flush := func() {
		if current != "" {
			jobs[current] = strings.Join(block, "\n")
		}
	}
	inJobs := false
	for _, line := range strings.Split(workflow, "\n") {
		if !inJobs {
			inJobs = line == "jobs:"
			continue
		}
		if workflowJobHeader.MatchString(line) {
			flush()
			current = strings.TrimSpace(strings.TrimSuffix(line, ":"))
			block = nil
			continue
		}
		if current != "" {
			block = append(block, line)
		}
	}
	flush()
	return jobs
}

func godotWorkflowEntrypointViolations(workflow string) []string {
	var violations []string
	jobs := parseWorkflowJobs(workflow)
	godotJob, ok := jobs["godot"]
	if !ok {
		violations = append(violations, "CI workflow is missing the optional godot pilot job")
	} else {
		for _, invocation := range []string{
			"make godot-project-check",
			"make godot-python-check",
			"make godot-smoke",
		} {
			if !strings.Contains(godotJob, invocation) {
				violations = append(violations, fmt.Sprintf("godot CI job is missing the invocation %q", invocation))
			}
		}
		if total, scoped := strings.Count(workflow, "make godot-"), strings.Count(godotJob, "make godot-"); total != scoped {
			violations = append(violations, fmt.Sprintf("godot gate invocations must stay inside the godot job (workflow=%d, godot job=%d)", total, scoped))
		}
	}
	testJob, ok := jobs["test"]
	if !ok {
		violations = append(violations, "CI workflow is missing the final test job")
	} else {
		// Reject any godot mention on a non-comment line, not just a needs
		// key: routine YAML reformatting can turn a one-line needs list into
		// a block-style list, and the release gate must never silently
		// couple to the optional pilot job.
		for _, line := range strings.Split(testJob, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			if strings.Contains(line, "godot") {
				violations = append(violations, fmt.Sprintf("final test job must stay independent of the optional godot job: %s", strings.TrimSpace(line)))
			}
		}
	}
	jobNames := make([]string, 0, len(jobs))
	for name := range jobs {
		jobNames = append(jobNames, name)
	}
	slices.Sort(jobNames)
	for _, name := range jobNames {
		if name == "godot" {
			continue
		}
		if strings.Contains(jobs[name], "make godot-") {
			violations = append(violations, fmt.Sprintf("job %s invokes a godot gate; optional godot evidence must stay in the godot job", name))
		}
		if strings.Contains(jobs[name], "scripts/godot/") {
			violations = append(violations, fmt.Sprintf("job %s probes scripts/godot directly instead of going through the godot job", name))
		}
	}
	slices.Sort(violations)
	return violations
}

func testGodotEntrypointMutations(t *testing.T, makefile, workflow string) {
	t.Helper()
	mutations := []struct {
		name string
		// Each mutator returns the mutated copy or fails the subtest when its
		// anchor drifted, so a silent no-op mutation can never look like a
		// rejection.
		mutateMakefile func(*testing.T, string) string
		mutateWorkflow func(*testing.T, string) string
	}{
		{
			name: "legacy build recipe probes the godot runtime",
			mutateMakefile: func(t *testing.T, source string) string {
				return mutateGodotAnchorOnce(t, source,
					"\tcp $(PIXEL_PERFECTION_NOTICE_DIR)/PROVENANCE.json $(PIXEL_PERFECTION_NOTICE_DEST)/PROVENANCE.json\n",
					"\tcp $(PIXEL_PERFECTION_NOTICE_DIR)/PROVENANCE.json $(PIXEL_PERFECTION_NOTICE_DEST)/PROVENANCE.json\n\tscripts/godot/fetch.sh\n")
			},
		},
		{
			name: "legacy test target depends on a godot gate",
			mutateMakefile: func(t *testing.T, source string) string {
				return mutateGodotAnchorOnce(t, source, "\ntest:\n", "\ntest: godot-project-check\n")
			},
		},
		{
			name: "godot ci job deleted",
			mutateWorkflow: func(t *testing.T, source string) string {
				t.Helper()
				start := strings.Index(source, "\n  godot:\n")
				end := strings.Index(source, "\n  test:\n")
				if start < 0 || end < 0 || start >= end {
					t.Fatalf("mutation anchor not found: the godot job must sit directly before the final test job")
				}
				return source[:start+1] + source[end+1:]
			},
		},
		{
			name: "final test job gates on the godot job",
			mutateWorkflow: func(t *testing.T, source string) string {
				return mutateGodotAnchorOnce(t, source,
					"needs: [native-macos, quality, frontend, go-race, integration, linux-server]",
					"needs: [native-macos, quality, frontend, go-race, integration, linux-server, godot]")
			},
		},
		{
			name: "godot smoke loses the isolated python flag",
			mutateMakefile: func(t *testing.T, source string) string {
				return mutateGodotAnchorOnce(t, source,
					"scripts/godot/smoke.sh --iterations 100 --isolated-python",
					"scripts/godot/smoke.sh --iterations 100")
			},
		},
		{
			name: "double-colon legacy rule runs a godot script",
			mutateMakefile: func(t *testing.T, source string) string {
				return mutateGodotAnchorOnce(t, source,
					"\ngodot-smoke:\n\tscripts/godot/smoke.sh --iterations 100 --isolated-python\n",
					"\ngodot-smoke:\n\tscripts/godot/smoke.sh --iterations 100 --isolated-python\n\nbuild::\n\tscripts/godot/fetch.sh\n")
			},
		},
		{
			name: "make parse-time assignment probes the godot runtime",
			mutateMakefile: func(t *testing.T, source string) string {
				return mutateGodotAnchorOnce(t, source,
					"\ngodot-smoke:\n\tscripts/godot/smoke.sh --iterations 100 --isolated-python\n",
					"\ngodot-smoke:\n\tscripts/godot/smoke.sh --iterations 100 --isolated-python\n\nGODOT_PROBE := $(shell scripts/godot/fetch.sh)\n")
			},
		},
		{
			name: "block-style test job needs lists the godot job",
			mutateWorkflow: func(t *testing.T, source string) string {
				return mutateGodotAnchorOnce(t, source,
					"needs: [native-macos, quality, frontend, go-race, integration, linux-server]",
					"needs:\n      - native-macos\n      - quality\n      - frontend\n      - go-race\n      - integration\n      - linux-server\n      - godot")
			},
		},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			root := t.TempDir()
			mutatedMakefile, mutatedWorkflow := makefile, workflow
			if mutation.mutateMakefile != nil {
				mutatedMakefile = mutation.mutateMakefile(t, mutatedMakefile)
			}
			if mutation.mutateWorkflow != nil {
				mutatedWorkflow = mutation.mutateWorkflow(t, mutatedWorkflow)
			}
			writeGodotEntrypointMutationFile(t, root, "Makefile", mutatedMakefile)
			writeGodotEntrypointMutationFile(t, root, filepath.Join(".github", "workflows", "ci.yml"), mutatedWorkflow)
			command := exec.Command(os.Args[0], "-test.run=^TestGodotIsOptionalForLegacyBuild$")
			command.Env = append(os.Environ(), godotEntrypointTestRootEnv+"="+root)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Errorf("godot entry-point guard accepted the mutation\n%s", output)
			}
		})
	}
}

func mutateGodotAnchorOnce(t *testing.T, source, old, replacement string) string {
	t.Helper()
	if count := strings.Count(source, old); count != 1 {
		t.Fatalf("mutation anchor must occur exactly once, got %d: %q", count, old)
	}
	return strings.Replace(source, old, replacement, 1)
}

func writeGodotEntrypointMutationFile(t *testing.T, root, relative, source string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}
