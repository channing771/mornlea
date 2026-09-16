package archcheck_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVisualBaselineRouting(t *testing.T) {
	root := repositoryRoot(t)
	forbidden := filepath.Join(root, "testdata", "visual-golden", "godot")
	if _, err := os.Lstat(forbidden); err == nil {
		t.Fatalf("renderer-specific visual class must not exist: %s", forbidden)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect renderer-specific visual class: %v", err)
	}

	for _, relative := range []string{
		".codex/skills/visual-baseline/SKILL.md",
		".claude/skills/visual-baseline/SKILL.md",
	} {
		text := readBaselineDoc(t, root, filepath.FromSlash(relative))
		for _, required := range []string{
			"Route by the observable subject, never by renderer identity",
			"build/visual/godot-pilot/<run-id>/",
			"must not write tracked goldens or relax thresholds",
			"requires an approved feature change",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s is missing visual-routing rule %q", relative, required)
			}
		}
	}
}

func TestVisualProducerOwnership(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		pilot      bool
		update     bool
		hasHandoff bool
		wantError  bool
	}{
		{name: "pilot evidence", path: "build/visual/godot-pilot/run-1/frame.png", pilot: true},
		{name: "pilot writes tracked world", path: "testdata/visual-golden/world/frame.png", pilot: true, wantError: true},
		{name: "renderer class", path: "testdata/visual-golden/godot/frame.png", wantError: true},
		{name: "update without handoff", path: "testdata/visual-golden/ui/panel.png", update: true, wantError: true},
		{name: "approved handoff update", path: "testdata/visual-golden/ui/panel.png", update: true, hasHandoff: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateVisualEvidenceRoute(test.path, test.pilot, test.update, test.hasHandoff)
			if (err != nil) != test.wantError {
				t.Fatalf("validate route error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func validateVisualEvidenceRoute(path string, pilot, update, hasHandoff bool) error {
	path = filepath.ToSlash(filepath.Clean(path))
	const tracked = "testdata/visual-golden/"
	if strings.HasPrefix(path, tracked+"godot/") {
		return errors.New("renderer-specific tracked visual classes are forbidden")
	}
	if pilot && strings.HasPrefix(path, tracked) {
		return errors.New("pilot evidence must stay outside tracked visual baselines")
	}
	if pilot && !strings.HasPrefix(path, "build/visual/godot-pilot/") {
		return errors.New("Godot pilot evidence must use the dedicated build output")
	}
	if update && strings.HasPrefix(path, tracked) && !hasHandoff {
		return errors.New("tracked producer update requires an approved handoff")
	}
	return nil
}
