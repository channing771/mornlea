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
	chineseBody, err := os.ReadFile(chinese)
	if err != nil {
		t.Fatalf("read Chinese Godot pilot report: %v", err)
	}
	if !regexp.MustCompile(`(?m)^Decision: (GO|NO-GO)$`).MatchString(text) {
		t.Fatal("report must contain a Decision: GO or Decision: NO-GO line")
	}
	protocolIdentityPattern := regexp.MustCompile(`protocol v[0-9]+`)
	for _, report := range []struct {
		name string
		text string
	}{
		{name: "English", text: text},
		{name: "Chinese", text: string(chineseBody)},
	} {
		identities := protocolIdentityPattern.FindAllString(report.text, -1)
		if len(identities) != 3 {
			t.Errorf("%s report has %d protocol identities, want 3", report.name, len(identities))
		}
		for _, identity := range identities {
			if identity != "protocol v44" {
				t.Errorf("%s report has protocol identity %q, want protocol v44", report.name, identity)
			}
		}
	}
	for _, required := range []string{
		"## Visual evidence",
		"## Performance",
		"## Feature coverage",
		"## Difference classification",
		"## Run identity",
		"Py4Godot",
		"CPython",
		"engine ABI v11",
		"benchmark scenario v23",
		"testdata/godot-pilot/godot-pilot-v23.json",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("English report is missing %q", required)
		}
	}
	reportPath := filepath.Join(root, "testdata", "godot-pilot", "godot-pilot-v23.json")
	report, err := readGodotPilotReport(reportPath)
	if err != nil {
		t.Fatalf("linked Godot pilot report JSON is incomplete: %v", err)
	}
	if got := report.Identity.ProtocolVersion; got != 44 {
		t.Errorf("linked Godot pilot report protocol_version=%d, want 44", got)
	}
}
