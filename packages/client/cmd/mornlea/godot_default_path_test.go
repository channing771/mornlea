//go:build darwin

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultClientDoesNotProbeGodot(t *testing.T) {
	var hits []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := strings.ToLower(string(data))
		for _, token := range []string{"godot", "py4godot", "mornlea-godot"} {
			if strings.Contains(source, token) {
				hits = append(hits, path+": "+token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("default client production sources probe Godot:\n%s", strings.Join(hits, "\n"))
	}
}

func TestLegacyClientABIRemainsV19(t *testing.T) {
	header := filepath.Join("..", "..", "..", "..", "packages", "engine", "include", "mornlea_client.h")
	data, err := os.ReadFile(header)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "#define MORNLEA_CLIENT_ABI_VERSION 19u") {
		t.Fatalf("legacy client ABI must remain v19 in %s", header)
	}
}

func TestDefaultAssemblyDoesNotCreateOnlineDualAuthority(t *testing.T) {
	options, err := parseMainOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if options.Application.Connect != "" {
		t.Fatalf("default startup must not auto-connect, got %q", options.Application.Connect)
	}
	flags := []string{
		"godot", "godot-pilot", "mornlea-godot", "py4godot",
	}
	optionsSource := readProductionFile(t, "options.go")
	for _, name := range flags {
		if strings.Contains(strings.ToLower(optionsSource), `"`+name+`"`) {
			t.Errorf("default CLI must not grow a %s flag", name)
		}
	}
	mainSource := readProductionFile(t, "main.go")
	for _, token := range []string{"mornlea_godot", "shadow writer", "dual-authorit", "dual authority"} {
		if strings.Contains(strings.ToLower(mainSource), token) {
			t.Errorf("default assembly must not introduce %q", token)
		}
	}
}

func readProductionFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
