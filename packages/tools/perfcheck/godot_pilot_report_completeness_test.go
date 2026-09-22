package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGodotPilotReportCompleteness(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	english := filepath.Join(root, "docs", "notes", "godot-client-pilot-report.md")
	chinese := filepath.Join(root, "docs", "notes", "godot-client-pilot-report.zh.md")
	body, err := os.ReadFile(english)
	if os.IsNotExist(err) {
		t.Skip("the dual-client report document is produced with measured P7 evidence")
	}
	if err != nil {
		t.Fatalf("read English Godot pilot report: %v", err)
	}
	text := string(body)
	if _, err := os.Stat(chinese); err != nil {
		t.Fatalf("Chinese counterpart missing: %v", err)
	}
	if !regexp.MustCompile(`(?m)^Decision: (GO|NO-GO)$`).MatchString(text) {
		t.Fatal("report must contain a Decision: GO or Decision: NO-GO line")
	}
	for _, required := range []string{
		"## Visual evidence",
		"## Performance",
		"## Feature coverage",
		"## Difference classification",
		"## Run identity",
		"Py4Godot",
		"CPython",
		"protocol v45",
		"engine ABI v11",
		"benchmark scenario v23",
		"testdata/godot-pilot/godot-pilot-v23.json",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("English report is missing %q", required)
		}
	}
	reportPath := filepath.Join(root, "testdata", "godot-pilot", "godot-pilot-v23.json")
	if _, err := readGodotPilotReport(reportPath); err != nil {
		t.Fatalf("linked Godot pilot report JSON is incomplete: %v", err)
	}
}
