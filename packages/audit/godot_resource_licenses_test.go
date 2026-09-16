package archcheck_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type godotResourceLicenseRow struct {
	Category                string
	Component               string
	Source                  string
	Version                 string
	License                 string
	DistributionObligations string
	PilotAuthorization      string
}

func TestGodotResourceLicenses(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "apps", "mornlea-godot", "assets", "provenance", "THIRD_PARTY.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parseGodotResourceLicenseRows(string(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range validateGodotResourceLicenseRows(rows) {
		t.Error(problem)
	}
}

func TestGodotResourceLicensesRejectUnapprovedComponent(t *testing.T) {
	rows := approvedGodotResourceLicenseRows()
	rows = append(rows, godotResourceLicenseRow{
		Category:                "Font",
		Component:               "Downloaded font",
		Source:                  "https://example.com/font.zip",
		Version:                 "1.0",
		License:                 "Unknown",
		DistributionObligations: "Unknown",
		PilotAuthorization:      "Allowed",
	})
	problems := strings.Join(validateGodotResourceLicenseRows(rows), "\n")
	if !strings.Contains(problems, "Downloaded font") || !strings.Contains(problems, "not approved") {
		t.Fatalf("unapproved component was not rejected:\n%s", problems)
	}
}

func parseGodotResourceLicenseRows(text string) ([]godotResourceLicenseRow, error) {
	const header = "| Category | Component | Source | Version | License | Distribution obligations | Pilot authorization |"
	lines := strings.Split(text, "\n")
	headerIndex := slices.Index(lines, header)
	if headerIndex < 0 || headerIndex+2 >= len(lines) {
		return nil, errorsForGodotLicenseTable("missing required seven-column table header")
	}
	var rows []godotResourceLicenseRow
	for _, line := range lines[headerIndex+2:] {
		if !strings.HasPrefix(line, "|") {
			break
		}
		fields := strings.Split(strings.Trim(line, "|"), "|")
		if len(fields) != 7 {
			return nil, errorsForGodotLicenseTable(fmt.Sprintf("row has %d columns, want 7: %s", len(fields), line))
		}
		for index := range fields {
			fields[index] = strings.Trim(strings.TrimSpace(fields[index]), "`")
		}
		rows = append(rows, godotResourceLicenseRow{
			Category:                fields[0],
			Component:               fields[1],
			Source:                  fields[2],
			Version:                 fields[3],
			License:                 fields[4],
			DistributionObligations: fields[5],
			PilotAuthorization:      fields[6],
		})
	}
	return rows, nil
}

func validateGodotResourceLicenseRows(rows []godotResourceLicenseRow) []string {
	approved := make(map[string]godotResourceLicenseRow)
	for _, row := range approvedGodotResourceLicenseRows() {
		approved[row.Component] = row
	}
	var problems []string
	seen := make(map[string]bool)
	for _, row := range rows {
		if seen[row.Component] {
			problems = append(problems, fmt.Sprintf("Godot resource component %q is duplicated", row.Component))
		}
		seen[row.Component] = true
		want, ok := approved[row.Component]
		if !ok {
			problems = append(problems, fmt.Sprintf("Godot resource component %q is not approved for the pilot", row.Component))
			continue
		}
		if row.Category != want.Category || row.Source != want.Source || row.License != want.License {
			problems = append(problems, fmt.Sprintf("Godot resource component %q provenance changed: got %+v, want category=%q source=%q license=%q", row.Component, row, want.Category, want.Source, want.License))
		}
		if strings.TrimSpace(row.Version) == "" || strings.TrimSpace(row.DistributionObligations) == "" || strings.TrimSpace(row.PilotAuthorization) == "" {
			problems = append(problems, fmt.Sprintf("Godot resource component %q has incomplete version, obligations, or authorization", row.Component))
		}
	}
	for component := range approved {
		if !seen[component] {
			problems = append(problems, fmt.Sprintf("approved Godot resource component %q is missing", component))
		}
	}
	slices.Sort(problems)
	return problems
}

func approvedGodotResourceLicenseRows() []godotResourceLicenseRow {
	return []godotResourceLicenseRow{
		{Category: "Engine", Component: "Godot Engine", Source: "https://github.com/godotengine/godot", License: "MIT"},
		{Category: "GDExtension binding", Component: "godot-rust", Source: "https://github.com/godot-rust/gdext", License: "MIT"},
		{Category: "Scripting runtime", Component: "Py4Godot", Source: "https://github.com/niklas2902/py4godot", License: "MIT"},
		{Category: "Interpreter", Component: "CPython runtime", Source: "https://github.com/python/cpython", License: "Python Software Foundation License Version 2"},
		{Category: "Font", Component: "No project-bundled font", Source: "Godot Engine built-in default", License: "No separate project asset"},
		{Category: "Material", Component: "Mornlea generated materials", Source: "packages/client/assets", License: "MIT (project-owned)"},
		{Category: "Shader", Component: "Mornlea project shaders", Source: "apps/mornlea-godot", License: "MIT (project-owned)"},
		{Category: "Plugin", Component: "mornlea_bridge", Source: "packages/engine/crates/mornlea_godot", License: "MIT (project-owned)"},
	}
}

func errorsForGodotLicenseTable(message string) error {
	return fmt.Errorf("Godot resource license table: %s", message)
}
