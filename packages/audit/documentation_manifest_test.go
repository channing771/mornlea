package archcheck_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const documentationManifestPath = "docs/documentation-manifest.json"

type documentationManifest struct {
	SchemaVersion int                          `json:"schema_version"`
	Documents     []documentationManifestEntry `json:"documents"`
}

type documentationManifestEntry struct {
	ID             string   `json:"id"`
	Classification string   `json:"classification"`
	Paths          []string `json:"paths"`
	EnglishPath    string   `json:"english_path,omitempty"`
	ChinesePath    string   `json:"chinese_path,omitempty"`
	Revision       string   `json:"revision,omitempty"`
}

var documentationClassifications = map[string]bool{
	"bilingual":  true,
	"generated":  true,
	"historical": true,
	"legacy":     true,
	"license":    true,
	"machine":    true,
	"plan":       true,
}

func TestDocumentationManifestClassifiesCurrentMarkdown(t *testing.T) {
	root := repositoryRoot(t)
	manifest := readDocumentationManifest(t, root)
	current := currentGovernedMarkdown(t, root)
	for _, problem := range validateDocumentationManifestShape(manifest, current) {
		t.Error(problem)
	}

	for _, entry := range manifest.Documents {
		for _, path := range entry.Paths {
			path = filepath.ToSlash(filepath.Clean(path))
			if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil || info.IsDir() {
				t.Errorf("classified document path %q must exist as a file: %v", path, err)
			}
		}
	}
}

func TestDocumentationManifestValidationRejectsDuplicateAndUnclassifiedEntries(t *testing.T) {
	manifest := documentationManifest{
		SchemaVersion: 1,
		Documents: []documentationManifestEntry{
			{ID: "duplicate", Classification: "machine", Paths: []string{"AGENTS.md"}},
			{ID: "duplicate", Classification: "machine", Paths: []string{"AGENTS.md", "README.en.md"}},
		},
	}
	problems := strings.Join(validateDocumentationManifestShape(manifest, []string{"AGENTS.md", "README.md"}), "\n")
	for _, want := range []string{"duplicate document id", "classified by both", "README.en.md", "README.md"} {
		if !strings.Contains(problems, want) {
			t.Errorf("manifest problems must mention %q:\n%s", want, problems)
		}
	}
}

func validateDocumentationManifestShape(manifest documentationManifest, current []string) []string {
	var problems []string
	if manifest.SchemaVersion != 1 {
		problems = append(problems, fmt.Sprintf("documentation manifest schema_version = %d, want 1", manifest.SchemaVersion))
	}

	classified := make(map[string]string)
	ids := make(map[string]bool)
	for index, entry := range manifest.Documents {
		if entry.ID == "" {
			problems = append(problems, fmt.Sprintf("documents[%d] has an empty id", index))
		} else if ids[entry.ID] {
			problems = append(problems, fmt.Sprintf("duplicate document id %q", entry.ID))
		}
		ids[entry.ID] = true
		if !documentationClassifications[entry.Classification] {
			problems = append(problems, fmt.Sprintf("document %q has unknown classification %q", entry.ID, entry.Classification))
		}
		if len(entry.Paths) == 0 {
			problems = append(problems, fmt.Sprintf("document %q has no current paths", entry.ID))
		}
		if entry.Classification == "bilingual" {
			if entry.EnglishPath == "" || entry.ChinesePath == "" || entry.Revision == "" {
				problems = append(problems, fmt.Sprintf("bilingual document %q must declare english_path, chinese_path, and revision", entry.ID))
			}
			if !isEnglishCanonicalMarkdown(entry.EnglishPath) {
				problems = append(problems, fmt.Sprintf("bilingual document %q English path %q must be an unsuffixed *.md path", entry.ID, entry.EnglishPath))
			}
			if !strings.HasSuffix(entry.ChinesePath, ".zh.md") {
				problems = append(problems, fmt.Sprintf("bilingual document %q Chinese path %q must use the *.zh.md suffix", entry.ID, entry.ChinesePath))
			}
		}
		if entry.Classification == "legacy" && (entry.EnglishPath != "" || entry.ChinesePath != "" || entry.Revision != "") {
			problems = append(problems, fmt.Sprintf("legacy document %q must not declare bilingual path or revision metadata", entry.ID))
		}
		for _, rawPath := range entry.Paths {
			path := filepath.ToSlash(filepath.Clean(rawPath))
			if path == "." || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
				problems = append(problems, fmt.Sprintf("document %q has invalid repository-relative path %q", entry.ID, path))
				continue
			}
			if strings.HasSuffix(path, ".en.md") {
				problems = append(problems, fmt.Sprintf("English-suffixed documentation path %q is forbidden", path))
			}
			if previous, exists := classified[path]; exists {
				problems = append(problems, fmt.Sprintf("document path %q is classified by both %q and %q", path, previous, entry.ID))
				continue
			}
			classified[path] = entry.ID
		}
	}

	for _, path := range current {
		if _, ok := classified[path]; !ok {
			problems = append(problems, fmt.Sprintf("current Markdown document %q is not classified in %s", path, documentationManifestPath))
		}
	}
	for path, id := range classified {
		if !slices.Contains(current, path) {
			problems = append(problems, fmt.Sprintf("document %q classifies %q, which is outside the current governed Markdown set", id, path))
		}
	}
	return problems
}

func TestDocumentationManifestLegacyEntriesCannotDeclarePartialPairs(t *testing.T) {
	manifest := documentationManifest{
		SchemaVersion: 1,
		Documents: []documentationManifestEntry{
			{
				ID:             "legacy-guide",
				Classification: "legacy",
				Paths:          []string{"docs/legacy.md"},
				EnglishPath:    "docs/legacy.md",
			},
		},
	}
	problems := strings.Join(validateDocumentationManifestShape(manifest, []string{"docs/legacy.md"}), "\n")
	if !strings.Contains(problems, "must not declare bilingual path or revision metadata") {
		t.Fatalf("legacy partial pair was accepted:\n%s", problems)
	}
}

func TestLegacyDocumentationRemainsUnchangedUntilPairMigration(t *testing.T) {
	root := repositoryRoot(t)
	manifest := readDocumentationManifest(t, root)
	changed := changedRepositoryPaths(t, root)
	for _, problem := range legacyDocumentationChangeProblems(manifest, changed) {
		t.Error(problem)
	}
}

func TestLegacyDocumentationChangeRequiresPairMigration(t *testing.T) {
	manifest := documentationManifest{
		SchemaVersion: 1,
		Documents: []documentationManifestEntry{
			{ID: "legacy-guide", Classification: "legacy", Paths: []string{"docs/legacy.md"}},
		},
	}
	problems := strings.Join(legacyDocumentationChangeProblems(manifest, map[string]bool{"docs/legacy.md": true}), "\n")
	if !strings.Contains(problems, "must migrate to an English *.md and Chinese *.zh.md pair") {
		t.Fatalf("changed legacy document was accepted:\n%s", problems)
	}
}

func legacyDocumentationChangeProblems(manifest documentationManifest, changed map[string]bool) []string {
	var problems []string
	for _, entry := range manifest.Documents {
		if entry.Classification != "legacy" {
			continue
		}
		for _, path := range entry.Paths {
			if changed[path] {
				problems = append(problems, fmt.Sprintf("legacy document %q at %s changed and must migrate to an English *.md and Chinese *.zh.md pair", entry.ID, path))
			}
		}
	}
	slices.Sort(problems)
	return problems
}

func isEnglishCanonicalMarkdown(path string) bool {
	return strings.HasSuffix(path, ".md") &&
		!strings.HasSuffix(path, ".zh.md") &&
		!strings.HasSuffix(path, ".en.md")
}

func readDocumentationManifest(t *testing.T, root string) documentationManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, documentationManifestPath))
	if err != nil {
		t.Fatalf("read %s: %v", documentationManifestPath, err)
	}
	var manifest documentationManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode %s: %v", documentationManifestPath, err)
	}
	return manifest
}

func currentGovernedMarkdown(t *testing.T, root string) []string {
	t.Helper()
	var current []string
	rootEntries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("enumerate root documentation: %v", err)
	}
	for _, entry := range rootEntries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			current = append(current, entry.Name())
		}
	}
	docsRoot := filepath.Join(root, "docs")
	if err := filepath.WalkDir(docsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != docsRoot && entry.Name() == "superpowers" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		current = append(current, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		t.Fatalf("enumerate governed Markdown: %v", err)
	}
	visualRoot := filepath.Join(root, "testdata", "visual-golden")
	entries, err := os.ReadDir(visualRoot)
	if err != nil {
		t.Fatalf("enumerate visual documentation: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		current = append(current, filepath.ToSlash(filepath.Join("testdata", "visual-golden", entry.Name())))
	}
	slices.Sort(current)
	return current
}
