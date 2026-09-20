package archcheck_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestGodotBuildCoreScriptPin pins the shared-library build script for the
// Go client core. The script is the producer half of the distribution
// contract: it must build the c-shared library into the ignored Godot bridge
// bin tree next to the GDExtension, colocate the Rust engine dynamic
// libraries the core links, and verify the loaded contract (exported symbol
// surface, canonical header, producer identity, dependency resolution)
// without writing outside the bin tree plus user-level caches.
func TestGodotBuildCoreScriptPin(t *testing.T) {
	root := repositoryRoot(t)
	scriptPath := filepath.Join(root, "scripts", "godot", "build-core.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s must be executable", scriptPath)
	}
	script := readBaselineDoc(t, root, filepath.Join("scripts", "godot", "build-core.sh"))
	for _, required := range []string{
		"go build",
		"-buildmode=c-shared",
		"addons/mornlea_bridge/bin",
		"macos-universal",
		"--verify",
		"--target",
		"--profile",
		"unsupported Godot desktop target",
		"make rust",
		"libmornlea_client_core.dylib",
		"libmornlea_engine.dylib",
		"libmornlea_client.dylib",
		"include/mornlea_client_core.h",
		"libmornlea_client_core.h",
		"nm -gU",
		"otool -L",
		"otool -D",
		"install_name_tool",
		"codesign",
		"@loader_path",
		"@rpath/libmornlea_engine.dylib",
		"dlopen",
		"mornlea_client_core_abi_version",
		"mktemp",
		"user-level Go build cache",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("build-core.sh is missing %q", required)
		}
	}

	// The deterministic symbol list inside the script must cover every
	// `//export` directive of the producer, so a new export cannot land
	// without the shared-library verification learning its name.
	exports := readBaselineDoc(t, root, filepath.Join("packages", "client", "cmd", "mornlea-godot-core", "exports.go"))
	exportPattern := regexp.MustCompile(`(?m)^//export ([A-Za-z0-9_]+)$`)
	matches := exportPattern.FindAllStringSubmatch(exports, -1)
	if len(matches) == 0 {
		t.Fatal("exports.go declares no //export symbols")
	}
	for _, match := range matches {
		if !strings.Contains(script, match[1]) {
			t.Errorf("build-core.sh symbol verification is missing export %q", match[1])
		}
	}

	// The build output tree must stay reproducible and untracked; the script
	// writes only into this ignored distribution directory.
	ignore := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", ".gitignore"))
	if !strings.Contains(ignore, "addons/mornlea_bridge/bin/") {
		t.Error("Godot project ignore rules must exclude the bridge bin distribution tree")
	}
}
