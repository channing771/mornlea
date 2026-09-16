package archcheck_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type godotAssetManifest struct {
	SchemaVersion int `json:"schema_version"`
	Generator     string
	InputRevision struct {
		AssetsGitTree string `json:"assets_git_tree"`
		FontsGitTree  string `json:"fonts_git_tree"`
	} `json:"input_revision"`
	InputChecksum string `json:"input_checksum"`
	Inputs        []struct {
		Path   string
		SHA256 string
		Bytes  int64
	}
	Atlas struct {
		Path       string
		Format     string
		Width      int
		Height     int
		Layers     int
		MipLevels  int `json:"mip_levels"`
		SHA256     string
		Bytes      int64
		StorageKey string `json:"storage_order"`
	}
	Outputs []struct {
		Path   string
		SHA256 string
		Bytes  int64
	}
}

func TestGodotAssetSyncIsDeterministicAndRejectsManualFiles(t *testing.T) {
	root := repositoryRoot(t)
	script := filepath.Join(root, "scripts", "godot", "sync-assets.sh")
	info, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("%s must be executable", script)
	}

	generatedRoot := filepath.Join(root, "apps", "mornlea-godot", "assets", "generated")
	manifest := readGodotAssetManifest(t, filepath.Join(generatedRoot, "manifest.json"))
	if manifest.SchemaVersion != 1 {
		t.Errorf("asset manifest schema_version = %d, want 1", manifest.SchemaVersion)
	}
	if manifest.Generator != "packages/client/cmd/mornlea-godot-assets" {
		t.Errorf("asset manifest generator = %q", manifest.Generator)
	}
	for name, value := range map[string]string{
		"assets_git_tree": manifest.InputRevision.AssetsGitTree,
		"fonts_git_tree":  manifest.InputRevision.FontsGitTree,
		"input_checksum":  manifest.InputChecksum,
	} {
		if !sha256Pattern.MatchString(value) && (name == "input_checksum" || len(value) != 40) {
			t.Errorf("asset manifest %s is not a recorded digest: %q", name, value)
		}
	}
	if manifest.Atlas.Path != "atlas.rgba8" || manifest.Atlas.Format != "RGBA8" ||
		manifest.Atlas.Width != 16 || manifest.Atlas.Height != 16 || manifest.Atlas.Layers <= 0 ||
		manifest.Atlas.MipLevels != 5 || manifest.Atlas.StorageKey != "layer-major,mip-major" ||
		!sha256Pattern.MatchString(manifest.Atlas.SHA256) || manifest.Atlas.Bytes <= 0 {
		t.Errorf("asset manifest has invalid atlas metadata: %+v", manifest.Atlas)
	}

	inputPaths := make([]string, 0, len(manifest.Inputs))
	for _, input := range manifest.Inputs {
		inputPaths = append(inputPaths, input.Path)
		if !sha256Pattern.MatchString(input.SHA256) || input.Bytes <= 0 {
			t.Errorf("asset input has invalid checksum or size: %+v", input)
		}
	}
	for _, required := range []string{
		"packages/client/assets/blocks.go",
		"packages/client/assets/packs/pastelcraft/PROVENANCE.json",
		"packages/client/render/assets/NotoSansCJKsc-Regular.otf",
		"packages/client/render/assets/NotoSansCJKsc-Regular.provenance.json",
		"packages/client/render/assets/OFL.txt",
	} {
		if !slices.Contains(inputPaths, required) {
			t.Errorf("asset manifest inputs are missing %s", required)
		}
	}
	outputPaths := make([]string, 0, len(manifest.Outputs))
	for _, output := range manifest.Outputs {
		outputPaths = append(outputPaths, output.Path)
	}
	if !slices.Contains(outputPaths, "fonts/NotoSansCJKsc-Regular.otf.import") {
		t.Error("asset manifest outputs are missing the deterministic Godot font import sidecar")
	}
	fontImport := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "assets", "generated", "fonts", "NotoSansCJKsc-Regular.otf.import"))
	for _, required := range []string{`uid="uid://`, `source_file="res://assets/generated/fonts/NotoSansCJKsc-Regular.otf"`, "allow_system_fallback=false"} {
		if !strings.Contains(fontImport, required) {
			t.Errorf("deterministic font import policy is missing %q", required)
		}
	}

	runGodotAssetSync(t, script, "", "--check", true)
	first := filepath.Join(t.TempDir(), "generated")
	second := filepath.Join(t.TempDir(), "generated")
	runGodotAssetSync(t, script, first, "", true)
	runGodotAssetSync(t, script, second, "", true)
	if diff := compareGodotAssetTrees(t, first, second); diff != "" {
		t.Fatalf("identical asset inputs are not byte-stable: %s", diff)
	}

	manualPath := filepath.Join(first, "hand-authored-source.txt")
	if err := os.WriteFile(manualPath, []byte("forbidden\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := runGodotAssetSync(t, script, first, "--check", false)
	if !strings.Contains(output, "unexpected generated asset") {
		t.Fatalf("manual generated file was not rejected with a stable diagnostic:\n%s", output)
	}

	if err := os.Remove(manualPath); err != nil {
		t.Fatal(err)
	}
	atlasPath := filepath.Join(first, "atlas.rgba8")
	atlas, err := os.ReadFile(atlasPath)
	if err != nil {
		t.Fatal(err)
	}
	atlas[0] ^= 0xff
	if err := os.WriteFile(atlasPath, atlas, 0o644); err != nil {
		t.Fatal(err)
	}
	output = runGodotAssetSync(t, script, first, "--check", false)
	if !strings.Contains(output, "generated asset content differs") {
		t.Fatalf("modified generated asset was not rejected with a stable diagnostic:\n%s", output)
	}
}

func readGodotAssetManifest(t *testing.T, path string) godotAssetManifest {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest godotAssetManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return manifest
}

func runGodotAssetSync(t *testing.T, script, generatedRoot, argument string, wantSuccess bool) string {
	t.Helper()
	arguments := []string(nil)
	if argument != "" {
		arguments = append(arguments, argument)
	}
	command := exec.Command(script, arguments...)
	command.Dir = repositoryRoot(t)
	command.Env = os.Environ()
	if generatedRoot != "" {
		command.Env = append(command.Env, "MORNLEA_GODOT_GENERATED_ROOT="+generatedRoot)
	}
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("%s %s failed: %v\n%s", script, argument, err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("%s %s unexpectedly succeeded:\n%s", script, argument, output)
	}
	return string(output)
}

func compareGodotAssetTrees(t *testing.T, leftRoot, rightRoot string) string {
	t.Helper()
	left := readGodotAssetTree(t, leftRoot)
	right := readGodotAssetTree(t, rightRoot)
	if len(left) != len(right) {
		return "file counts differ"
	}
	for path, leftData := range left {
		rightData, ok := right[path]
		if !ok {
			return "second tree is missing " + path
		}
		if !bytes.Equal(leftData, rightData) {
			return "content differs for " + path
		}
	}
	return ""
}

func readGodotAssetTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
