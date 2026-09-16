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
			"Source comment language gate",
			"make comment-language-check",
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
}
