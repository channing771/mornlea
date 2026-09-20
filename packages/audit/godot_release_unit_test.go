package archcheck_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGodotReleaseUnitSeparatesPythonEnvironments(t *testing.T) {
	root := repositoryRoot(t)
	godotProject := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "pyproject.toml"))
	agentProject := readBaselineDoc(t, root, filepath.Join("packages", "agent", "pyproject.toml"))
	if !strings.Contains(godotProject, "dependencies = []") {
		t.Error("Godot production Python dependencies must remain empty")
	}
	if strings.Contains(godotProject, "mornlea-agent") || strings.Contains(godotProject, "packages/agent") {
		t.Error("Godot Python must not depend on the standalone Agent service")
	}
	if strings.Contains(agentProject, "mornlea-godot") || strings.Contains(agentProject, "apps/mornlea-godot") {
		t.Error("standalone Agent Python must not depend on the Godot embedded runtime")
	}
	if !strings.Contains(godotProject, `requires-python = "==3.14.*"`) {
		t.Error("embedded Godot Python must stay pinned to CPython 3.14")
	}
	if !strings.Contains(agentProject, ">=3.12,<3.13") {
		t.Error("standalone Agent Python must keep its independent 3.12 pin")
	}

	godotLock := filepath.Join(root, "apps", "mornlea-godot", "uv.lock")
	agentLock := filepath.Join(root, "packages", "agent", "uv.lock")
	for _, path := range []string{godotLock, agentLock} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("Python lockfile missing: %s", path)
		}
	}

	provenance := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "assets", "provenance", "THIRD_PARTY.md"))
	for _, required := range []string{
		"CPython runtime",
		"Py4Godot",
		"Do not add Mojang",
		"runtime installers",
		"System Python",
	} {
		if !strings.Contains(provenance, required) {
			t.Errorf("Godot provenance table is missing %q", required)
		}
	}
}

func TestGodotReleaseUnitRejectsMojangAndSystemPathLibraries(t *testing.T) {
	root := repositoryRoot(t)
	projectRoot := filepath.Join(root, "apps", "mornlea-godot")
	err := filepath.WalkDir(projectRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".godot", ".venv", "addons", "__pycache__":
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(entry.Name())
		if strings.Contains(name, "mojang") {
			t.Errorf("Mojang-named asset is forbidden: %s", path)
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".py" || ext == ".gd" || ext == ".pyi" {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			source := string(data)
			if strings.Contains(source, "packages.agent") || strings.Contains(source, "packages/agent") {
				t.Errorf("%s imports the standalone Agent Python environment", path)
			}
			if strings.Contains(source, "/usr/bin/python") || strings.Contains(source, "/usr/local/lib/python") {
				t.Errorf("%s resolves a system Python path", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
