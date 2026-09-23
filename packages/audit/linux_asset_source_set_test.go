package archcheck_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"testing"
)

type goListPackage struct {
	ImportPath     string
	GoFiles        []string
	IgnoredGoFiles []string
}

func TestGodotAssetGeneratorUsesPortableLinuxSourceSet(t *testing.T) {
	environment := append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=1")
	command := exec.Command("go", "list", "-json",
		"./packages/client/assets",
		"./packages/client/cmd/mornlea-godot-assets",
	)
	command.Dir = repositoryRoot(t)
	command.Env = environment
	output, err := command.Output()
	if err != nil {
		t.Fatalf("GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go list -json ./packages/client/assets ./packages/client/cmd/mornlea-godot-assets failed: %v", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	packages := make(map[string]goListPackage, 2)
	for decoder.More() {
		var listed goListPackage
		if err := decoder.Decode(&listed); err != nil {
			t.Fatalf("decode Linux go list output: %v", err)
		}
		packages[listed.ImportPath] = listed
	}
	assets, ok := packages["github.com/channing771/mornlea/packages/client/assets"]
	if !ok {
		t.Fatalf("Linux go list output did not include assets package: %v", packages)
	}
	if !slices.Contains(assets.GoFiles, "atlas.go") {
		t.Fatalf("Linux assets GoFiles = %v, IgnoredGoFiles = %v; want atlas.go in GoFiles", assets.GoFiles, assets.IgnoredGoFiles)
	}
	if _, ok := packages["github.com/channing771/mornlea/packages/client/cmd/mornlea-godot-assets"]; !ok {
		t.Fatalf("Linux go list output did not include Godot asset generator package: %v", packages)
	}
	// Linux quality compiles this source set only after verifying its native artifact;
	// preflight must inspect selection without linking against an unbuilt engine.
}
