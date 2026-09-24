package archcheck_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEnglishCommentGateIntegration(t *testing.T) {
	root := repositoryRoot(t)
	checks := map[string][]string{
		"Makefile": {
			"comment-language-check:",
			"EnglishCommentMigration|CodeCommentLanguage|CommentScanner",
		},
		filepath.Join("scripts", "agents", "gates.sh"): {
			"English source-comment language ratchet",
			"make comment-language-check",
		},
		filepath.Join(".github", "workflows", "ci.yml"): {
			"preflight:",
			"make ci-preflight",
		},
	}
	for path, required := range checks {
		text := readBaselineDoc(t, root, path)
		for _, fragment := range required {
			if !strings.Contains(text, fragment) {
				t.Errorf("%s is missing English-comment gate fragment %q", path, fragment)
			}
		}
	}
	makefile := readBaselineDoc(t, root, "Makefile")
	if !strings.Contains(makeTargetRecipe(t, makefile, "ci-preflight"), "$(MAKE) comment-language-check") {
		t.Error("ci-preflight must invoke the source comment language gate")
	}
}
