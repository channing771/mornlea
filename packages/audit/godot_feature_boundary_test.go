package archcheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGodotCapabilityRegistryKeepsPythonAsPresentationLanguage(t *testing.T) {
	root := repositoryRoot(t)
	registry := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "catalog", "capability_registry.tres"))
	checker := readBaselineDoc(t, root, filepath.Join("scripts", "godot", "capability_registry_check.py"))
	host := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "app", "host", "feature_host.py"))
	for _, required := range []string{
		`"menus"`, `"containers"`, `"chat"`, `"audio"`, `"lod"`, `"viewmodel"`,
		`"companions"`, `"hostile_mobs"`, `"passive_cows"`, `"projectiles"`,
		`"drops"`, `"name_tags"`, `"particles"`, `"full_weather"`,
	} {
		if !strings.Contains(checker, required) {
			t.Errorf("capability registry checker is missing reserved capability %s", required)
		}
	}
	if !strings.Contains(registry, "registry_version = 1") {
		t.Error("capability registry must stay versioned")
	}
	if !strings.Contains(host, `BUDGET_CLASSES = frozenset({"bootstrap", "light", "standard", "heavy"})`) {
		t.Error("Python host must keep a closed budget-class set")
	}
	if !strings.Contains(host, "has unknown budget class") {
		t.Error("Python host must reject unknown budget classes")
	}
	makefile := readBaselineDoc(t, root, "Makefile")
	if !strings.Contains(makeTargetRecipe(t, makefile, "godot-capability-check"), "scripts/godot/capability-check.sh") {
		t.Error("godot-capability-check must invoke the capability registry checker")
	}
	catalog := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "config", "feature_catalog.tres"))
	capability := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "catalog", "capability_registry.tres"))
	registered := catalog + "\n" + capability
	projectRoot := filepath.Join(root, "apps", "mornlea-godot")
	if err := filepath.WalkDir(filepath.Join(projectRoot, "features"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "feature.tres" {
			return nil
		}
		relative, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return err
		}
		slash := filepath.ToSlash(relative)
		if !strings.Contains(registered, "res://"+slash) {
			t.Errorf("feature manifest %s is not in the catalog or reserved capability registry", slash)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGodotFeatureBoundaryRejectsCatalogAndOwnershipEscapes(t *testing.T) {
	root := repositoryRoot(t)
	checker := readBaselineDoc(t, root, filepath.Join("scripts", "godot", "python_boundary_check.py"))
	for _, required := range []string{
		"forbidden-protocol-ownership",
		"forbidden-prediction",
		"forbidden-persistence",
		"forbidden-numerical-fallback",
		"unbounded-callback",
		"cross-feature-private-path",
		"unrestricted-dynamic-import",
	} {
		if !strings.Contains(checker, required) {
			t.Errorf("Python boundary checker is missing rule %s", required)
		}
	}
	contract := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "tests", "scripts", "feature_contract_check.py"))
	for _, required := range []string{
		`"duplicate"`, `"cycle"`, `"unknown_budget"`, `"required_dependent"`,
		`"required_bridge"`, "--extensibility-probe", "synthetic_optional",
	} {
		if !strings.Contains(contract, required) {
			t.Errorf("feature-contract check is missing catalog case %s", required)
		}
	}
	validator := readBaselineDoc(t, root, filepath.Join("scripts", "godot", "validate-project.sh"))
	if !strings.Contains(validator, "unregistered autoload is forbidden") {
		t.Error("project validator must reject shared mutable autoloads")
	}
	host := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "app", "host", "feature_host.py"))
	if !strings.Contains(host, "elapsed_ns = max(0, min(int(delta * 1_000_000_000), 100_000_000))") {
		t.Error("Python host must clamp per-frame callback work")
	}
}

func TestGodotFeatureBoundaryDetectsDrift(t *testing.T) {
	root := repositoryRoot(t)
	projectRoot := filepath.Join(root, "apps", "mornlea-godot")
	if leaked := scanGodotScriptDiscipline(projectRoot); len(findingsWithRule(leaked, "protocol-")) != 0 {
		t.Fatalf("production scripts already implement protocol details: %v", leaked)
	}

	mutations := []struct {
		name     string
		relative string
		source   string
		rule     string
	}{
		{
			name:     "protocol decoding",
			relative: filepath.Join("features", "world", "world_feature.py"),
			source:   "from __future__ import annotations\nMAGIC = \"MCW1\"\n",
			rule:     "protocol-magic-text",
		},
		{
			name:     "host names a concrete feature",
			relative: filepath.Join("app", "host", "feature_host.py"),
			source:   "from __future__ import annotations\nFEATURE = \"actors\"\n",
			rule:     "host-feature-reference",
		},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			fixture := newGodotHostDisciplineFixture(t)
			writeGodotFixtureFile(t, filepath.Join(fixture, filepath.FromSlash(mutation.relative)), mutation.source)
			findings := scanGodotScriptDiscipline(fixture)
			found := false
			for _, finding := range findings {
				if finding.rule == mutation.rule {
					found = true
				}
			}
			if !found {
				t.Fatalf("feature-boundary probe %s was not rejected: %v", mutation.name, findings)
			}
		})
	}
}
