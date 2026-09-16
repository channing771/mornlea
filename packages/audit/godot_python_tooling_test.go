package archcheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGodotPythonToolingIsLockedAndRuntimeFree(t *testing.T) {
	root := repositoryRoot(t)
	projectRoot := filepath.Join(root, "apps", "mornlea-godot")
	pyproject := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "pyproject.toml"))
	for _, required := range []string{
		`requires-python = "==3.14.*"`,
		"dependencies = []",
		"[dependency-groups]",
		"mypy",
		"ruff",
		"mypy_path = \"typing\"",
		"strict = true",
	} {
		if !strings.Contains(pyproject, required) {
			t.Errorf("Godot pyproject is missing %q", required)
		}
	}

	lock := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "uv.lock"))
	for _, required := range []string{"mypy", "ruff", `requires-python = "==3.14.*"`} {
		if !strings.Contains(lock, required) {
			t.Errorf("Godot uv.lock is missing %q", required)
		}
	}

	checkPath := filepath.Join(root, "scripts", "godot", "python-check.sh")
	info, err := os.Stat(checkPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s must be executable", checkPath)
	}
	check := readBaselineDoc(t, root, filepath.Join("scripts", "godot", "python-check.sh"))
	for _, required := range []string{
		"--locked",
		"--offline",
		"python_boundary_check.py",
		"python_boundary_check_test.py",
		"ruff format --check",
		"ruff check",
		"mypy",
		"PY4GODOT_CPYTHON_VERSION",
	} {
		if !strings.Contains(check, required) {
			t.Errorf("python-check.sh is missing %q", required)
		}
	}

	for _, relative := range []string{
		"typing/py4godot/classes/__init__.pyi",
		"typing/py4godot/classes/Node.pyi",
		"typing/py4godot/classes/Object.pyi",
		"typing/py4godot/classes/PackedScene.pyi",
		"typing/py4godot/classes/Resource.pyi",
		"typing/py4godot/classes/ResourceLoader.pyi",
		"typing/py4godot/classes/SceneTree.pyi",
		"typing/py4godot/py.typed",
	} {
		if info, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(relative))); err != nil || info.IsDir() {
			t.Errorf("local Py4Godot typing artifact %s is missing: %v", relative, err)
		}
	}
}
