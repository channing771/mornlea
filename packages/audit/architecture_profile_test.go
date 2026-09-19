package archcheck_test

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// architectureProfile names one of the two architecture gates registered by
// the Godot pilot closeout. The pilot profile admits the existing Go
// client-core and the migration-only Bootstrap; the target profile is the
// growth and ownership gate for later work.
type architectureProfile string

const (
	architectureProfilePilot  architectureProfile = "pilot"
	architectureProfileTarget architectureProfile = "target"
)

const (
	pilotGoClientCorePackage = "packages/client/cmd/mornlea-godot-core"
	godotProjectRoot         = "apps/mornlea-godot"
	godotRustCrateRoot       = "packages/engine/crates/mornlea_godot"
)

// grandfatheredGoRealtimePackages is the closed inventory of current Go
// packages that own real-time session, authority, protocol, or numerical
// hot-path work. The target profile rejects any new package that matches
// the real-time heuristic and is absent from this list. The Go client-core
// is a named pilot transition exception, not a target owner.
var grandfatheredGoRealtimePackages = []string{
	"packages/client/client",
	"packages/client/cmd/mornlea-godot-core",
	"packages/client/lod",
	"packages/client/mesh",
	"packages/client/presentation",
	"packages/client/runtime",
	"packages/server/cmd/mornlea-server",
	"packages/server/fluid",
	"packages/server/server",
	"packages/server/server/persistence",
	"packages/server/sim/contract",
	"packages/server/sim/entity",
	"packages/server/sim/realm",
	"packages/server/sim/runtime",
	"packages/shared/core",
	"packages/shared/nativeabi",
	"packages/shared/network",
	"packages/shared/network/codec",
	"packages/shared/network/protocol",
	"packages/shared/network/tcp",
	"packages/shared/physics",
	"packages/shared/worldgen",
}

var pilotBootstrapGDScript = []string{
	"app/bootstrap/bootstrap.gd",
	"app/bootstrap/setup_required.gd",
}

func TestArchitecturePilotProfile(t *testing.T) {
	root := repositoryRoot(t)
	packages := architectureListedPackages(t)
	if _, ok := packages[pilotGoClientCorePackage]; !ok {
		t.Fatalf("pilot profile must register the Go client-core transition package %s", pilotGoClientCorePackage)
	}
	if _, ok := packages["packages/client/runtime"]; !ok {
		t.Fatal("pilot profile must register packages/client/runtime")
	}
	if _, ok := packages["packages/client/presentation"]; !ok {
		t.Fatal("pilot profile must register packages/client/presentation")
	}
	if violations := architectureRealtimeGrowthViolations(packages, architectureProfilePilot); len(violations) > 0 {
		t.Errorf("pilot real-time inventory drifted, %d violations:\n%s", len(violations), strings.Join(violations, "\n"))
	}
	if violations := architectureGDScriptViolations(t, root, architectureProfilePilot); len(violations) > 0 {
		t.Errorf("pilot Bootstrap allowlist drifted, %d violations:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

func TestArchitectureTargetProfile(t *testing.T) {
	root := repositoryRoot(t)
	packages := architectureListedPackages(t)
	if violations := architectureRealtimeGrowthViolations(packages, architectureProfileTarget); len(violations) > 0 {
		t.Errorf("target profile found new Go real-time ownership, %d violations:\n%s", len(violations), strings.Join(violations, "\n"))
	}
	if violations := architectureGDScriptViolations(t, root, architectureProfileTarget); len(violations) > 0 {
		t.Errorf("target profile found production GDScript outside Bootstrap, %d violations:\n%s", len(violations), strings.Join(violations, "\n"))
	}
	if violations := architectureGodotEngineAccessViolations(t, root); len(violations) > 0 {
		t.Errorf("mornlea_godot must not call the engine ABI directly, %d violations:\n%s", len(violations), strings.Join(violations, "\n"))
	}
	if violations := architectureGoHostBoundaryViolations(t); len(violations) > 0 {
		t.Errorf("Go packages must not import Godot, Python, or GPU bindings, %d violations:\n%s", len(violations), strings.Join(violations, "\n"))
	}
	projectRoot := filepath.Join(root, godotProjectRoot)
	for _, finding := range scanGodotScriptDiscipline(projectRoot) {
		t.Errorf("Python/Godot feature code crossed a target ownership boundary: %s", finding)
	}
}

func TestArchitectureProfilesDetectDrift(t *testing.T) {
	contract := architectureListedPackages(t)
	if violations := architectureRealtimeGrowthViolations(contract, architectureProfileTarget); len(violations) != 0 {
		t.Fatalf("current tree must satisfy the target real-time inventory: %v", violations)
	}

	t.Run("new Go client-core is rejected", func(t *testing.T) {
		packages := cloneArchitecturePackages(contract)
		packages["packages/client/cmd/mornlea-godot-core-v2"] = []string{"packages/client/runtime", "packages/shared/network/protocol"}
		violations := architectureRealtimeGrowthViolations(packages, architectureProfileTarget)
		if len(violations) != 1 || !strings.Contains(violations[0], "packages/client/cmd/mornlea-godot-core-v2") {
			t.Fatalf("target profile must reject a new Go client-core: %v", violations)
		}
	})

	t.Run("new protocol-owning Go package is rejected", func(t *testing.T) {
		packages := cloneArchitecturePackages(contract)
		packages["packages/client/session2"] = []string{"packages/shared/network/protocol"}
		violations := architectureRealtimeGrowthViolations(packages, architectureProfileTarget)
		if len(violations) != 1 || !strings.Contains(violations[0], "packages/client/session2") {
			t.Fatalf("target profile must reject new Go real-time ownership: %v", violations)
		}
	})

	t.Run("pilot still admits the existing Go client-core", func(t *testing.T) {
		packages := cloneArchitecturePackages(contract)
		violations := architectureRealtimeGrowthViolations(packages, architectureProfilePilot)
		if len(violations) != 0 {
			t.Fatalf("pilot profile must admit the grandfathered Go client-core: %v", violations)
		}
	})

	t.Run("Python protocol access is rejected", func(t *testing.T) {
		fixture := newGodotHostDisciplineFixture(t)
		writeGodotFixtureFile(t, filepath.Join(fixture, "features", "world", "world_feature.py"), "from __future__ import annotations\nMAGIC = \"MCW1\"\n")
		findings := findingsWithRule(scanGodotScriptDiscipline(fixture), "protocol-")
		if len(findings) == 0 {
			t.Fatal("target profile must reject Python protocol decoding")
		}
	})

	t.Run("direct engine access is rejected", func(t *testing.T) {
		violations := architectureEngineAccessFromTexts(
			`mornlea_engine = { path = "../mornlea_engine" }`,
			[]string{"use mornlea_engine::mesh;"},
		)
		if len(violations) != 2 {
			t.Fatalf("engine-access checker must reject Cargo and source probes: %v", violations)
		}
	})

	t.Run("Go Godot import is rejected", func(t *testing.T) {
		violations := architectureGoHostBoundaryFromImports(map[string][]string{
			"packages/client/runtime": {"github.com/godot-rust/gdext"},
		})
		if len(violations) != 1 || !strings.Contains(violations[0], "Godot") {
			t.Fatalf("target profile must reject Go→Godot imports: %v", violations)
		}
	})
}

func architectureListedPackages(t *testing.T) map[string][]string {
	t.Helper()
	out := listWorkspacePackages(t, "{{.ImportPath}}|{{join .Imports \" \"}}", map[string][]string{
		"packages/client": {
			"./client/...", "./render/...", "./mesh/...", "./lod/...", "./audio/...", "./assets/...",
			"./runtime/...", "./presentation/...", "./cmd/mornlea-godot-core",
		},
		"packages/tools": {"./...", "./gfxspike"},
	})
	packages := make(map[string][]string)
	for _, line := range out {
		parts := strings.SplitN(line, "|", 2)
		pkg := localName(parts[0])
		var imports []string
		if len(parts) == 2 {
			for _, imported := range strings.Fields(parts[1]) {
				if local := localName(imported); local != imported {
					imports = append(imports, local)
				}
			}
		}
		packages[pkg] = imports
	}
	return packages
}

func architectureRealtimeGrowthViolations(packages map[string][]string, profile architectureProfile) []string {
	inventory := make(map[string]bool, len(grandfatheredGoRealtimePackages))
	for _, pkg := range grandfatheredGoRealtimePackages {
		inventory[pkg] = true
	}
	var violations []string
	for _, pkg := range slices.Sorted(maps.Keys(packages)) {
		if !packageOwnsGoRealtime(pkg, packages[pkg]) {
			continue
		}
		if inventory[pkg] {
			if profile == architectureProfileTarget && pkg == pilotGoClientCorePackage {
				continue
			}
			continue
		}
		violations = append(violations, "new Go real-time ownership "+pkg+" is not in the grandfathered inventory")
	}
	if profile == architectureProfilePilot && !inventory[pilotGoClientCorePackage] {
		violations = append(violations, "pilot profile lost the Go client-core transition exception")
	}
	return violations
}

func packageOwnsGoRealtime(pkg string, imports []string) bool {
	if strings.HasPrefix(pkg, "packages/tools") || pkg == "packages/audit" || strings.HasPrefix(pkg, "packages/contracts") {
		return false
	}
	if pkg == pilotGoClientCorePackage {
		return true
	}
	for _, imported := range imports {
		if isGoRealtimeImport(imported) {
			return true
		}
	}
	switch {
	case pkg == "packages/client/client",
		pkg == "packages/client/runtime",
		pkg == "packages/client/presentation",
		strings.HasPrefix(pkg, "packages/server/sim/"),
		pkg == "packages/server/server",
		pkg == "packages/server/server/persistence",
		pkg == "packages/shared/nativeabi",
		strings.HasPrefix(pkg, "packages/shared/network"):
		return true
	default:
		return false
	}
}

func isGoRealtimeImport(imported string) bool {
	switch {
	case strings.HasPrefix(imported, "packages/shared/network"),
		strings.HasPrefix(imported, "packages/server/sim"),
		imported == "packages/client/runtime",
		imported == "packages/shared/nativeabi":
		return true
	default:
		return false
	}
}

func architectureGDScriptViolations(t *testing.T, root string, profile architectureProfile) []string {
	t.Helper()
	allow := make(map[string]bool, len(pilotBootstrapGDScript))
	for _, relative := range pilotBootstrapGDScript {
		allow[relative] = true
	}
	var violations []string
	projectRoot := filepath.Join(root, godotProjectRoot)
	err := filepath.WalkDir(projectRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".godot", ".venv", "tests", "py4godot":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".gd" {
			return nil
		}
		relative, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return err
		}
		slash := filepath.ToSlash(relative)
		if allow[slash] {
			return nil
		}
		violations = append(violations, "production GDScript outside Bootstrap: "+slash)
		return nil
	})
	if err != nil {
		t.Fatalf("walk Godot scripts: %v", err)
	}
	if profile == architectureProfilePilot {
		for _, relative := range pilotBootstrapGDScript {
			if _, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(relative))); err != nil {
				violations = append(violations, "pilot Bootstrap script is missing: "+relative)
			}
		}
	}
	return violations
}

func architectureGodotEngineAccessViolations(t *testing.T, root string) []string {
	t.Helper()
	cargo := readBaselineDoc(t, root, filepath.Join(godotRustCrateRoot, "Cargo.toml"))
	var sources []string
	srcRoot := filepath.Join(root, godotRustCrateRoot, "src")
	err := filepath.WalkDir(srcRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".rs" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sources = append(sources, string(data))
		return nil
	})
	if err != nil {
		t.Fatalf("walk mornlea_godot sources: %v", err)
	}
	return architectureEngineAccessFromTexts(cargo, sources)
}

func architectureEngineAccessFromTexts(cargo string, sources []string) []string {
	var violations []string
	for _, marker := range []string{"mornlea_engine", "mornlea_engine.h", "nativeabi"} {
		if strings.Contains(cargo, marker) {
			violations = append(violations, "mornlea_godot Cargo.toml references "+marker)
		}
	}
	for _, source := range sources {
		for _, marker := range []string{"mornlea_engine", "mornlea_engine.h", "FluidEvalBatch", "nativeabi"} {
			if strings.Contains(source, marker) {
				violations = append(violations, "mornlea_godot source references "+marker)
			}
		}
	}
	return violations
}

func architectureGoHostBoundaryViolations(t *testing.T) []string {
	t.Helper()
	out := listWorkspacePackages(t, "{{.ImportPath}}|{{join .Imports \" \"}}", nil)
	imports := make(map[string][]string)
	for _, line := range out {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		imports[localName(parts[0])] = strings.Fields(parts[1])
	}
	return architectureGoHostBoundaryFromImports(imports)
}

func architectureGoHostBoundaryFromImports(imports map[string][]string) []string {
	var violations []string
	for _, pkg := range slices.Sorted(maps.Keys(imports)) {
		if pkg == "packages/audit" {
			continue
		}
		for _, imported := range imports[pkg] {
			lower := strings.ToLower(imported)
			switch {
			case strings.Contains(lower, "godot"):
				violations = append(violations, pkg+" imports Godot binding "+imported)
			case strings.Contains(lower, "py4godot"), strings.Contains(lower, "cpython"):
				violations = append(violations, pkg+" imports Python runtime binding "+imported)
			case strings.Contains(lower, "webgpu"), strings.Contains(lower, "vulkan"):
				violations = append(violations, pkg+" imports GPU binding "+imported)
			}
		}
	}
	return violations
}

func cloneArchitecturePackages(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for pkg, imports := range in {
		copied := make([]string, len(imports))
		copy(copied, imports)
		out[pkg] = copied
	}
	return out
}
