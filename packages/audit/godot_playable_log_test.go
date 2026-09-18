package archcheck_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGodotPlayableLogRejectsEveryUnapprovedError(t *testing.T) {
	t.Parallel()

	checker := filepath.Join(repositoryRoot(t), "scripts", "godot", "playable-log-check.sh")
	tests := []struct {
		name    string
		log     string
		wantErr bool
	}{
		{name: "clean", log: "Godot Engine v4.7.2\nPython playable smoke passed\n"},
		{
			name: "exact dummy renderer diagnostic",
			log: "Godot Engine v4.7.2\n" +
				"ERROR: Parameter \"material\" is null.\n" +
				"   at: material_get_instance_shader_parameters (servers/rendering/dummy/storage/material_storage.cpp:264)\n" +
				"Python playable smoke passed\n",
		},
		{
			name: "repeated exact dummy renderer diagnostic",
			log: "ERROR: Parameter \"material\" is null.\n" +
				"   at: material_get_instance_shader_parameters (servers/rendering/dummy/storage/material_storage.cpp:264)\n" +
				"ERROR: Parameter \"material\" is null.\n" +
				"   at: material_get_instance_shader_parameters (servers/rendering/dummy/storage/material_storage.cpp:264)\n",
		},
		{name: "unrelated engine error", log: "ERROR: failed to load feature scene\n", wantErr: true},
		{name: "script error", log: "SCRIPT ERROR: Invalid call.\n", wantErr: true},
		{
			name:    "missing dummy detail",
			log:     "ERROR: Parameter \"material\" is null.\n",
			wantErr: true,
		},
		{
			name: "changed dummy detail",
			log: "ERROR: Parameter \"material\" is null.\n" +
				"   at: material_get_instance_shader_parameters (servers/rendering/dummy/storage/material_storage.cpp:265)\n",
			wantErr: true,
		},
		{
			name: "non-adjacent dummy detail",
			log: "ERROR: Parameter \"material\" is null.\n\n" +
				"   at: material_get_instance_shader_parameters (servers/rendering/dummy/storage/material_storage.cpp:264)\n",
			wantErr: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			logPath := filepath.Join(t.TempDir(), "godot.log")
			if err := os.WriteFile(logPath, []byte(test.log), 0o600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(checker, logPath)
			output, err := command.CombinedOutput()
			if test.wantErr && err == nil {
				t.Fatalf("checker accepted unapproved error output:\n%s", output)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("checker rejected approved output: %v\n%s", err, output)
			}
		})
	}
}

func TestGodotPlayableSmokeOracleContracts(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	read := func(relative string) string {
		t.Helper()
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		return string(contents)
	}
	requireText := func(path, contents, fragment string) {
		t.Helper()
		if !strings.Contains(contents, fragment) {
			t.Errorf("%s does not pin %q", path, fragment)
		}
	}

	manifestPath := "apps/mornlea-godot/features/world/feature.tres"
	manifest := read(manifestPath)
	requireText(manifestPath, manifest, `PackedStringArray("5@1.0", "8@1.0")`)

	scriptPath := "scripts/godot/playable-smoke.sh"
	script := read(scriptPath)
	requireText(scriptPath, script, "POSIX::setpgid(0, 0)")
	requireText(scriptPath, script, "MORNLEA_PLAYABLE_SMOKE_REMOTE_PLAYER_ID")
	requireText(scriptPath, script, "Mornlea Godot smoke helper observed target:")
	requireText(scriptPath, script, "Mornlea Godot smoke helper observed Godot peer:")
	requireText(scriptPath, script, `kill -TERM -- "-${pgid}"`)
	requireText(scriptPath, script, "if (( ${#remaining[@]} )); then")
	if strings.Contains(script, `godot_output="$(cat -- "${godot_log}")"`) {
		t.Errorf("%s reads the complete Godot log into shell memory", scriptPath)
	}

	driverPath := "apps/mornlea-godot/tests/scripts/playable_smoke_check.py"
	driver := read(driverPath)
	for _, fragment := range []string{
		"MORNLEA_PLAYABLE_SMOKE_REMOTE_PLAYER_ID",
		"INITIAL_SESSION_EPOCH = 1",
		`_canonical_player_id(entity["player_id"]) == self._expected_remote_player_id`,
		`facts.get("uploads_applied")`,
		`bridge.call("session_status_typed")`,
		"if not self._movement_seen:",
		"_duration_samples",
		"_last_sample_revision",
	} {
		requireText(driverPath, driver, fragment)
	}
}
