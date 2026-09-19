package archcheck_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestGodotDesktopOnlyToolchainPin(t *testing.T) {
	root := repositoryRoot(t)
	envPath := filepath.Join(root, "scripts", "godot", "version.env")
	values := readGodotVersionEnvironment(t, envPath)
	want := map[string]string{
		"GODOT_VERSION":                 "4.7.2-stable",
		"GODOT_MACOS_UNIVERSAL_URL":     "https://github.com/godotengine/godot/releases/download/4.7.2-stable/Godot_v4.7.2-stable_macos.universal.zip",
		"GODOT_MACOS_UNIVERSAL_SHA256":  "c58a24e31d720be9d62f60cb5627c4e695fb72f21b0cfe1bc9ccaa9a3b3ba63e",
		"GODOT_EXPORT_TEMPLATES_URL":    "https://github.com/godotengine/godot/releases/download/4.7.2-stable/Godot_v4.7.2-stable_export_templates.tpz",
		"GODOT_EXPORT_TEMPLATES_SHA256": "f298490b8d44d934be425a5a65a51bf15f422428b229a06a6e11d9ffea248011",
	}
	for key, expected := range want {
		if got := values[key]; got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}
	for key, value := range values {
		upper := strings.ToUpper(key + "=" + value)
		for _, forbidden := range []string{"ANDROID", "IOS", "MOBILE", "WEB", "CONSOLE"} {
			if strings.Contains(upper, forbidden) {
				t.Errorf("desktop-only Godot pin contains forbidden target %q in %s", forbidden, key)
			}
		}
		if strings.HasSuffix(key, "_SHA256") && !sha256Pattern.MatchString(value) {
			t.Errorf("%s is not a lowercase SHA-256 digest", key)
		}
	}

	fetchPath := filepath.Join(root, "scripts", "godot", "fetch.sh")
	info, err := os.Stat(fetchPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s must be executable", fetchPath)
	}
	fetch := readBaselineDoc(t, root, filepath.Join("scripts", "godot", "fetch.sh"))
	for _, required := range []string{"darwin-universal", "--verify-only", "MORNLEA_GODOT_CACHE_DIR", "unsupported Godot desktop target"} {
		if !strings.Contains(fetch, required) {
			t.Errorf("fetch.sh is missing %q", required)
		}
	}
}

func TestGodotRustDependencyPin(t *testing.T) {
	root := repositoryRoot(t)
	workspace := readBaselineDoc(t, root, filepath.Join("packages", "engine", "Cargo.toml"))
	if !strings.Contains(workspace, `"crates/mornlea_godot"`) {
		t.Error("Rust workspace does not include crates/mornlea_godot")
	}
	manifest := readBaselineDoc(t, root, filepath.Join("packages", "engine", "crates", "mornlea_godot", "Cargo.toml"))
	for _, required := range []string{
		`name = "mornlea_godot"`,
		`crate-type = ["rlib", "cdylib"]`,
		`version = "=0.5.5"`,
		`features = ["api-4-7"]`,
	} {
		if !strings.Contains(manifest, required) {
			t.Errorf("mornlea_godot Cargo.toml is missing %q", required)
		}
	}
}

func TestGodotPythonRuntimePin(t *testing.T) {
	root := repositoryRoot(t)
	envPath := filepath.Join(root, "scripts", "godot", "python-version.env")
	values := readGodotVersionEnvironment(t, envPath)
	want := map[string]string{
		"PY4GODOT_VERSION":                     "4.7-alpha21",
		"PY4GODOT_SOURCE_REVISION":             "d8e17428deeb0428587349b663f6da26cd71ef3a",
		"PY4GODOT_RELEASE_URL":                 "https://github.com/niklas2902/py4godot/releases/download/4.7-alpha21/py4godot.zip",
		"PY4GODOT_RELEASE_SHA256":              "7fa28db6e5614523a4a9e092ca2e75fb76dfa1f4c385aaf6dd624eb33bf3eb4c",
		"PY4GODOT_ARCHIVE_ROOT":                "install_dir/addons/py4godot",
		"PY4GODOT_GDEXTENSION_VERSION":         "4.7-alpha-21",
		"PY4GODOT_GODOT_COMPATIBILITY_MINIMUM": "4.7.0",
		"PY4GODOT_ENTRY_SYMBOL":                "initialize_pythonscript",
		"PY4GODOT_CPYTHON_VERSION":             "3.14.4",
		"PY4GODOT_TARGET":                      "darwin-arm64",
	}
	for key, expected := range want {
		if got := values[key]; got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}
	for key, value := range values {
		upper := strings.ToUpper(key + "=" + value)
		for _, forbidden := range []string{"ANDROID", "IOS", "MOBILE", "WEB", "CONSOLE", "WINDOWS", "LINUX"} {
			if strings.Contains(upper, forbidden) {
				t.Errorf("current macOS Python pin contains forbidden target %q in %s", forbidden, key)
			}
		}
		if strings.HasSuffix(key, "_SHA256") && !sha256Pattern.MatchString(value) {
			t.Errorf("%s is not a lowercase SHA-256 digest", key)
		}
	}

	checkPath := filepath.Join(root, "scripts", "godot", "python-runtime-check.sh")
	info, err := os.Stat(checkPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s must be executable", checkPath)
	}
	check := readBaselineDoc(t, root, filepath.Join("scripts", "godot", "python-runtime-check.sh"))
	for _, required := range []string{
		"--qualify",
		"--exported",
		"--offline",
		"MORNLEA_PY4GODOT_CACHE_DIR",
		"PYTHONNOUSERSITE",
		"PYTHONPATH",
		"addons/py4godot",
		"unsupported Py4Godot desktop target",
	} {
		if !strings.Contains(check, required) {
			t.Errorf("python-runtime-check.sh is missing %q", required)
		}
	}

	ignore := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", ".gitignore"))
	if !strings.Contains(ignore, "addons/py4godot/") {
		t.Error("Godot project ignore rules must exclude the reproducible Py4Godot runtime")
	}
}

func readGodotVersionEnvironment(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" || value == "" {
			t.Fatalf("invalid Godot version environment line %q", line)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}
