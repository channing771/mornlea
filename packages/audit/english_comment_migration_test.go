package archcheck_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	englishCommentMigrationPath = "testdata/audit/english-comment-migration.json"
	updateEnglishCommentDebtEnv = "MORNLEA_UPDATE_ENGLISH_COMMENT_BASELINE"
)

type englishCommentMigration struct {
	SchemaVersion int                      `json:"schema_version"`
	TotalFiles    int                      `json:"total_files"`
	TotalComments int                      `json:"total_comments"`
	Files         []englishCommentDebtFile `json:"files"`
}

type englishCommentDebtFile struct {
	Path   string `json:"path"`
	Count  int    `json:"count"`
	Digest string `json:"digest"`
}

func TestEnglishCommentMigration(t *testing.T) {
	root := repositoryRoot(t)
	current := currentEnglishCommentDebt(t, root)
	path := filepath.Join(root, filepath.FromSlash(englishCommentMigrationPath))

	baseline, err := readEnglishCommentMigration(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if os.Getenv(updateEnglishCommentDebtEnv) == "1" {
		if err == nil {
			if problems := compareEnglishCommentDebt(baseline, current, true); len(problems) > 0 {
				t.Fatalf("refuse to increase or replace English-comment migration debt:\n%s", strings.Join(problems, "\n"))
			}
		}
		writeEnglishCommentMigration(t, path, current)
		return
	}
	if os.IsNotExist(err) {
		t.Fatalf("%s is missing; create the initial inventory only with %s=1", englishCommentMigrationPath, updateEnglishCommentDebtEnv)
	}
	if problems := compareEnglishCommentDebt(baseline, current, false); len(problems) > 0 {
		t.Fatalf("English-comment migration debt changed:\n%s\ntranslate comments or update a decreasing baseline with %s=1", strings.Join(problems, "\n"), updateEnglishCommentDebtEnv)
	}
}

func TestEnglishCommentMigrationRejectsGrowthAndReplacement(t *testing.T) {
	baseline := englishCommentMigration{
		SchemaVersion: 1,
		TotalFiles:    1,
		TotalComments: 2,
		Files: []englishCommentDebtFile{
			{Path: "packages/example.go", Count: 2, Digest: "old"},
		},
	}

	tests := []struct {
		name    string
		current englishCommentMigration
		update  bool
		want    string
	}{
		{
			name: "new debt path",
			current: englishCommentMigration{SchemaVersion: 1, TotalFiles: 2, TotalComments: 3, Files: []englishCommentDebtFile{
				{Path: "packages/example.go", Count: 2, Digest: "old"},
				{Path: "packages/new.go", Count: 1, Digest: "new"},
			}},
			update: true,
			want:   "new non-English comment debt",
		},
		{
			name: "higher count",
			current: englishCommentMigration{SchemaVersion: 1, TotalFiles: 1, TotalComments: 3, Files: []englishCommentDebtFile{
				{Path: "packages/example.go", Count: 3, Digest: "larger"},
			}},
			update: true,
			want:   "increased from 2 to 3",
		},
		{
			name: "same count replacement",
			current: englishCommentMigration{SchemaVersion: 1, TotalFiles: 1, TotalComments: 2, Files: []englishCommentDebtFile{
				{Path: "packages/example.go", Count: 2, Digest: "different"},
			}},
			update: true,
			want:   "changed without decreasing",
		},
		{
			name: "decrease requires baseline update",
			current: englishCommentMigration{SchemaVersion: 1, TotalFiles: 1, TotalComments: 1, Files: []englishCommentDebtFile{
				{Path: "packages/example.go", Count: 1, Digest: "smaller"},
			}},
			want: "decreased from 2 to 1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			problems := strings.Join(compareEnglishCommentDebt(baseline, test.current, test.update), "\n")
			if !strings.Contains(problems, test.want) {
				t.Fatalf("problems = %q, want fragment %q", problems, test.want)
			}
		})
	}

	decreased := englishCommentMigration{
		SchemaVersion: 1,
		TotalFiles:    1,
		TotalComments: 1,
		Files: []englishCommentDebtFile{
			{Path: "packages/example.go", Count: 1, Digest: "smaller"},
		},
	}
	if problems := compareEnglishCommentDebt(baseline, decreased, true); len(problems) != 0 {
		t.Fatalf("decreasing update rejected: %v", problems)
	}
}

func TestEnglishCommentGrandfatherAllowsUnchangedLegacyDebt(t *testing.T) {
	baseline := englishCommentMigration{
		SchemaVersion: 1,
		TotalFiles:    1,
		TotalComments: 2,
		Files: []englishCommentDebtFile{
			{Path: "packages/example.go", Count: 2, Digest: "unchanged-comments"},
		},
	}

	for _, allowDecrease := range []bool{false, true} {
		if problems := compareEnglishCommentDebt(baseline, baseline, allowDecrease); len(problems) != 0 {
			t.Fatalf("unchanged grandfathered debt rejected with allowDecrease=%t: %v", allowDecrease, problems)
		}
	}
}

func currentEnglishCommentDebt(t *testing.T, root string) englishCommentMigration {
	t.Helper()
	sources, err := collectFirstPartyCommentSources(root)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := scanCodeCommentLanguage(sources)
	if err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string][]string)
	for _, finding := range findings {
		byPath[finding.path] = append(byPath[finding.path], finding.text)
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	slices.Sort(paths)

	migration := englishCommentMigration{SchemaVersion: 1, TotalFiles: len(paths), TotalComments: len(findings)}
	for _, path := range paths {
		comments := byPath[path]
		slices.Sort(comments)
		digest := sha256.Sum256([]byte(strings.Join(comments, "\n")))
		migration.Files = append(migration.Files, englishCommentDebtFile{
			Path:   path,
			Count:  len(comments),
			Digest: hex.EncodeToString(digest[:]),
		})
	}
	return migration
}

func compareEnglishCommentDebt(baseline, current englishCommentMigration, allowDecrease bool) []string {
	if baseline.SchemaVersion != 1 {
		return []string{fmt.Sprintf("baseline schema_version = %d, want 1", baseline.SchemaVersion)}
	}
	if current.SchemaVersion != 1 {
		return []string{fmt.Sprintf("current schema_version = %d, want 1", current.SchemaVersion)}
	}
	baselineFiles := commentDebtByPath(baseline.Files)
	currentFiles := commentDebtByPath(current.Files)
	var problems []string
	for path, currentFile := range currentFiles {
		baselineFile, exists := baselineFiles[path]
		if !exists {
			problems = append(problems, fmt.Sprintf("%s has new non-English comment debt (%d comments)", path, currentFile.Count))
			continue
		}
		switch {
		case currentFile.Count > baselineFile.Count:
			problems = append(problems, fmt.Sprintf("%s increased from %d to %d comments", path, baselineFile.Count, currentFile.Count))
		case currentFile.Count == baselineFile.Count && currentFile.Digest != baselineFile.Digest:
			problems = append(problems, fmt.Sprintf("%s comment debt changed without decreasing", path))
		case currentFile.Count < baselineFile.Count && !allowDecrease:
			problems = append(problems, fmt.Sprintf("%s decreased from %d to %d comments but the checked-in baseline was not ratcheted", path, baselineFile.Count, currentFile.Count))
		}
	}
	for path, baselineFile := range baselineFiles {
		if _, exists := currentFiles[path]; !exists && !allowDecrease {
			problems = append(problems, fmt.Sprintf("%s decreased from %d to 0 comments but the checked-in baseline was not ratcheted", path, baselineFile.Count))
		}
	}
	if !allowDecrease {
		if baseline.TotalFiles != current.TotalFiles {
			problems = append(problems, fmt.Sprintf("total_files changed from %d to %d", baseline.TotalFiles, current.TotalFiles))
		}
		if baseline.TotalComments != current.TotalComments {
			problems = append(problems, fmt.Sprintf("total_comments changed from %d to %d", baseline.TotalComments, current.TotalComments))
		}
	}
	slices.Sort(problems)
	return problems
}

func commentDebtByPath(files []englishCommentDebtFile) map[string]englishCommentDebtFile {
	byPath := make(map[string]englishCommentDebtFile, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}
	return byPath
}

func readEnglishCommentMigration(path string) (englishCommentMigration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return englishCommentMigration{}, err
	}
	var migration englishCommentMigration
	if err := json.Unmarshal(data, &migration); err != nil {
		return englishCommentMigration{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return migration, nil
}

func writeEnglishCommentMigration(t *testing.T, path string, migration englishCommentMigration) {
	t.Helper()
	data, err := json.MarshalIndent(migration, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
