package archcheck_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitectureTargetDocumentsFoundationAndRollback(t *testing.T) {
	root := repositoryRoot(t)
	english := readBaselineDoc(t, root, filepath.Join("docs", "architecture-target.md"))
	chinese := readBaselineDoc(t, root, filepath.Join("docs", "architecture-target.zh.md"))
	for _, fragment := range []string{
		"status: target-not-current",
		"F1", "F2", "F3",
		"P8", "P9", "P10", "P11", "P12", "P13", "P14",
		"`apps/mornlea-godot/`",
		"`scripts/godot`",
		"No-Go rollback",
		"offline replay",
		"independently reversible",
		"never runs two online authorities",
	} {
		if !strings.Contains(english, fragment) {
			t.Errorf("architecture-target.md is missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		"status: target-not-current",
		"F1", "F2", "F3",
		"P8", "P14",
		"`apps/mornlea-godot/`",
		"`scripts/godot`",
		"No-Go rollback",
		"offline replay",
		"绝不运行两个在线权威",
	} {
		if !strings.Contains(chinese, fragment) {
			t.Errorf("architecture-target.zh.md is missing %q", fragment)
		}
	}
	if strings.Contains(english, "current default production") {
		t.Error("architecture-target.md must not present the pilot as current default production behavior")
	}

	crossLinks := map[string]string{
		"AGENTS.md":                                                            filepath.Join("docs", "architecture-target.md"),
		filepath.Join("docs", "architecture.md"):                               "architecture-target.md",
		filepath.Join("docs", "README.md"):                                     "architecture-target.md",
		filepath.Join("docs", "notes", "go-rust-division.md"):                  "../architecture-target.md",
		filepath.Join(".codex", "skills", "mornlea-architecture", "SKILL.md"):  "../../../docs/architecture-target.md",
		filepath.Join(".claude", "skills", "mornlea-architecture", "SKILL.md"): "../../../docs/architecture-target.md",
	}
	for relative, needle := range crossLinks {
		text := readBaselineDoc(t, root, relative)
		if !strings.Contains(text, needle) {
			t.Errorf("%s does not cross-link the target architecture document via %q", relative, needle)
		}
	}
}
