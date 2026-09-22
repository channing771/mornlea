package archcheck_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGodotBuildExtensionUsesEffectiveCargoTargetDirectory(t *testing.T) {
	script := filepath.Join(repositoryRoot(t), "scripts", "godot", "build-extension.sh")
	absoluteTarget := filepath.Join(t.TempDir(), "cargo-target")

	for _, testCase := range []struct {
		name           string
		cargoTargetDir string
		expectedTarget func(t *testing.T, repository string) string
	}{
		{
			name:           "unset",
			cargoTargetDir: "",
			expectedTarget: func(t *testing.T, repository string) string {
				return filepath.Join(physicalGodotExtensionPath(t, repository), "packages", "engine", "target")
			},
		},
		{
			name:           "relative",
			cargoTargetDir: "build/cargo-target",
			expectedTarget: func(t *testing.T, repository string) string {
				return filepath.Join(physicalGodotExtensionPath(t, repository), "build", "cargo-target")
			},
		},
		{
			name:           "absolute",
			cargoTargetDir: absoluteTarget,
			expectedTarget: func(t *testing.T, _ string) string {
				return physicalGodotExtensionPath(t, absoluteTarget)
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := t.TempDir()
			projectRoot := createGodotExtensionFixture(t, repository)
			expectedTarget := testCase.expectedTarget(t, repository)
			fixturePayload := []byte("GDExtension target root " + testCase.name)
			command, cargoMarker := godotExtensionBuildCommand(t, script, repository, projectRoot, expectedTarget, fixturePayload)
			command.Env = append(command.Env, "CARGO_TARGET_DIR="+testCase.cargoTargetDir)

			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("build-extension failed: %v\n%s", err, output)
			}
			if _, err := os.Stat(cargoMarker); err != nil {
				t.Fatalf("fake Cargo was not invoked: %v", err)
			}

			destination := filepath.Join(projectRoot, "addons", "mornlea_bridge", "bin", "macos-universal", "debug", "libmornlea_godot.dylib")
			built, err := os.ReadFile(destination)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(built, fixturePayload) {
				t.Fatalf("destination payload = %q, want %q", built, fixturePayload)
			}
		})
	}

	t.Run("rejects relative traversal", func(t *testing.T) {
		repository := t.TempDir()
		projectRoot := createGodotExtensionFixture(t, repository)
		command, cargoMarker := godotExtensionBuildCommand(t, script, repository, projectRoot, "", nil)
		command.Env = append(command.Env, "CARGO_TARGET_DIR=../escape")

		output, err := command.CombinedOutput()
		if err == nil {
			t.Fatal("build-extension accepted a traversing relative CARGO_TARGET_DIR")
		}
		if exitError, ok := err.(*exec.ExitError); !ok || exitError.ExitCode() != 2 {
			t.Fatalf("build-extension exit = %v, want exit 2\n%s", err, output)
		}
		if !strings.Contains(string(output), "relative CARGO_TARGET_DIR must not contain '..'") {
			t.Fatalf("traversal diagnostic missing:\n%s", output)
		}
		if _, err := os.Stat(cargoMarker); !os.IsNotExist(err) {
			t.Fatalf("fake Cargo invocation marker exists or cannot be checked: %v", err)
		}
	})
}

func createGodotExtensionFixture(t *testing.T, repository string) string {
	t.Helper()
	engineRoot := filepath.Join(repository, "packages", "engine")
	if err := os.MkdirAll(engineRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(engineRoot, "Cargo.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(repository, "apps", "mornlea-godot")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	return projectRoot
}

func godotExtensionBuildCommand(t *testing.T, script, repository, projectRoot, expectedTarget string, payload []byte) (*exec.Cmd, string) {
	t.Helper()
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "cargo-invoked")
	writeGodotExtensionFakeCommand(t, filepath.Join(binDir, "uname"), "#!/usr/bin/env bash\nprintf '%s\\n' arm64\n")
	writeGodotExtensionFakeCommand(t, filepath.Join(binDir, "rustup"), "#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"$3\" == rustc ]]; then\n  printf '%s\\n' 'host: aarch64-apple-darwin'\n  exit 0\nfi\nif [[ \"$3\" != cargo || \"$4\" != build ]]; then\n  printf 'unexpected rustup invocation: %s\\n' \"$*\" >&2\n  exit 1\nfi\n[[ \"${CARGO_TARGET_DIR}\" == \"${MORNLEA_EXPECTED_CARGO_TARGET_DIR}\" ]] || { printf 'CARGO_TARGET_DIR=%s, want %s\\n' \"${CARGO_TARGET_DIR}\" \"${MORNLEA_EXPECTED_CARGO_TARGET_DIR}\" >&2; exit 1; }\ntouch \"${MORNLEA_CARGO_MARKER}\"\nmkdir -p \"${CARGO_TARGET_DIR}/aarch64-apple-darwin/debug\"\nprintf '%s' \"${MORNLEA_EXTENSION_PAYLOAD}\" > \"${CARGO_TARGET_DIR}/aarch64-apple-darwin/debug/libmornlea_godot.dylib\"\n")

	command := exec.Command(script)
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MORNLEA_REPOSITORY_ROOT="+repository,
		"MORNLEA_GODOT_PROJECT_ROOT="+projectRoot,
		"MORNLEA_EXPECTED_CARGO_TARGET_DIR="+expectedTarget,
		"MORNLEA_CARGO_MARKER="+marker,
		"MORNLEA_EXTENSION_PAYLOAD="+string(payload),
	)
	return command, marker
}

func writeGodotExtensionFakeCommand(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func physicalGodotExtensionPath(t *testing.T, path string) string {
	t.Helper()
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, filepath.Base(path))
}
