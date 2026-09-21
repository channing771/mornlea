package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestContractInventoryReconcilesFrozenCorpus(t *testing.T) {
	root, families, live := discoverLive(t)

	inventory, err := LoadInventory(filepath.Join(root, filepath.FromSlash(InventoryRelPath)))
	if err != nil {
		t.Fatalf("load frozen inventory: %v", err)
	}
	if _, err := ReconcileWorking(root, inventory, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions()); err != nil {
		t.Fatalf("frozen inventory drifted from current registries: %v", err)
	}
	digest, err := CanonicalCorpusDigest(inventory)
	if err != nil {
		t.Fatalf("compute canonical corpus digest: %v", err)
	}
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 {
		t.Fatalf("unexpected canonical corpus digest format: %q", digest)
	}
	assertRequiredCoverage(t, inventory.Families)
}

func TestContractInventoryRejectsMissingFamily(t *testing.T) {
	root, families, live := discoverLive(t)
	if len(families) == 0 {
		t.Fatal("discover produced no families")
	}
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}

	inventory := inventoryFrom(live, families[1:], cases)
	_, err = ReconcileWorking(root, inventory, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
	if err == nil || !strings.Contains(err.Error(), "uncovered family "+families[0].ID) {
		t.Fatalf("missing family %s: error=%v", families[0].ID, err)
	}

	inventory = inventoryFrom(live, families, cases)
	inventory.Families = append(inventory.Families, Family{
		ID: "protocol.missing.Synthetic", Kind: "protocol", Role: "input",
		CurrentVersion: "1", SupportedVersions: []string{"1"},
		Source: "does-not-exist.go", EventualOwner: ownerProtocol,
		NumericSemantics: protocolLE,
		Sources:          []SourceSpec{{Path: "packages/shared/network/codec/frame.go", SHA256: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}},
	})
	_, err = ReconcileWorking(root, inventory, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
	if err == nil || !strings.Contains(err.Error(), "inventory family protocol.missing.Synthetic is not in current registries") {
		t.Fatalf("extra inventory family: error=%v", err)
	}
}

func TestContractInventoryRejectsVersionMismatch(t *testing.T) {
	root, families, live := discoverLive(t)
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}

	inventory := inventoryFrom(live, families, cases)
	inventory.Identities.Protocol++
	if inventory.Families[0].CurrentVersion == "" {
		t.Fatal("discovered family is missing a current version")
	}
	inventory.Families[0].CurrentVersion = "0"
	_, err = ReconcileWorking(root, inventory, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
	if err == nil {
		t.Fatal("version mismatch was accepted")
	}
	text := err.Error()
	if !strings.Contains(text, "protocol version") {
		t.Fatalf("identity version mismatch missing from %v", err)
	}
	if !strings.Contains(text, "family "+families[0].ID+" version") {
		t.Fatalf("family version mismatch missing from %v", err)
	}
}

func TestContractInventoryRejectsMissingProvenanceSource(t *testing.T) {
	root, families, live := discoverLive(t)
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}

	inventory := inventoryFrom(live, families, cases)
	inventory.Families[0].Sources = []SourceSpec{{
		Path:   "testdata/runtime-migration/missing-source.go",
		SHA256: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}}
	_, err = ReconcileWorking(root, inventory, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing source: error=%v", err)
	}

	inventory = inventoryFrom(live, families, cases)
	inventory.Families[0].Sources = nil
	_, err = ReconcileWorking(root, inventory, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
	if err == nil || !strings.Contains(err.Error(), "has no provenance sources") {
		t.Fatalf("empty sources: error=%v", err)
	}
}

func TestContractInventoryRejectsIncompleteIdentity(t *testing.T) {
	root, families, live := discoverLive(t)
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}

	inventory := inventoryFrom(live, families, cases)
	inventory.Identities.AgentHTTP = ""
	inventory.Identities.EngineABI = 0
	_, err = ReconcileWorking(root, inventory, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
	if err == nil {
		t.Fatal("incomplete identity was accepted")
	}
	text := err.Error()
	if !strings.Contains(text, "incomplete inventory identity") {
		t.Fatalf("incomplete identity missing from %v", err)
	}
}

func discoverLive(t *testing.T) (string, []Family, Identities) {
	t.Helper()
	root, err := RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	families, live, err := Discover(root)
	if err != nil {
		t.Fatalf("discover registries: %v", err)
	}
	if live.Protocol == 0 || live.EngineABI == 0 || live.AgentHTTP == "" {
		t.Fatalf("incomplete live identity: %+v", live)
	}
	return root, families, live
}

func inventoryFrom(live Identities, families []Family, cases []CaseSpec) Inventory {
	cloned := make([]Family, len(families))
	for i, family := range families {
		family.SupportedVersions = append([]string(nil), family.SupportedVersions...)
		family.Sources = append([]SourceSpec(nil), family.Sources...)
		family.Cases = append([]string(nil), family.Cases...)
		cloned[i] = family
	}
	clonedCases := append([]CaseSpec(nil), cases...)
	return Inventory{
		SchemaVersion:  inventorySchemaVersion,
		SourceRevision: BaselineSourceRevision,
		Identities:     live,
		Families:       cloned,
		Cases:          clonedCases,
	}
}

func assertRequiredCoverage(t *testing.T, families []Family) {
	t.Helper()
	seen := map[string]int{}
	roles := map[string]int{}
	owners := map[string]int{}
	for _, family := range families {
		if family.ID == "" || family.Source == "" || family.EventualOwner == "" || family.NumericSemantics == "" {
			t.Fatalf("family %#v is missing provenance", family)
		}
		seen[family.Kind]++
		roles[family.Role]++
		owners[family.EventualOwner]++
	}
	for _, kind := range []string{"domain", "protocol", "save", "kernel", "agent"} {
		if seen[kind] == 0 {
			t.Fatalf("inventory is missing kind %s", kind)
		}
	}
	for _, role := range []string{"input", "event"} {
		if roles[role] == 0 {
			t.Fatalf("inventory is missing semantic role %s", role)
		}
	}
	for _, owner := range []string{ownerDomain, ownerProtocol, ownerStorage, ownerEngine, ownerAgent} {
		if owners[owner] == 0 {
			t.Fatalf("inventory is missing eventual owner %s", owner)
		}
	}
	if seen["kernel"] < 2 {
		t.Fatalf("kernel coverage %d does not include both ABI exports and Go-only pathfind", seen["kernel"])
	}
}

func TestContractInventoryWorkingReportsZeroCaseFamilies(t *testing.T) {
	root, families, live := discoverLive(t)
	inventory, err := LoadInventory(filepath.Join(root, filepath.FromSlash(InventoryRelPath)))
	if err != nil {
		t.Fatalf("load frozen inventory: %v", err)
	}

	report, err := ReconcileWorking(
		root,
		inventory,
		families,
		live,
		BaselineConsumerRegistry(),
		BaselineNegativeCoverageExceptions(),
	)
	if err != nil {
		t.Fatalf("ReconcileWorking failed: %v", err)
	}

	zeroCasePoint := CoveragePoint{
		FamilyID: "protocol.client.BoneMeal",
		Version:  "45",
	}
	foundUncovered := false
	for _, pt := range report.Uncovered {
		if pt == zeroCasePoint {
			foundUncovered = true
			break
		}
	}
	if !foundUncovered {
		t.Fatalf("expected zero-case family %v in Uncovered, got %v", zeroCasePoint, report.Uncovered)
	}
	for _, pt := range report.Covered {
		if pt == zeroCasePoint {
			t.Fatalf("expected zero-case family %v NOT in Covered", zeroCasePoint)
		}
	}

	assertCoveragePointsSorted(t, "Covered", report.Covered)
	assertCoveragePointsSorted(t, "Uncovered", report.Uncovered)
}

func TestContractInventoryCompleteRejectsZeroCaseFamilies(t *testing.T) {
	root, families, live := discoverLive(t)
	inventory, err := LoadInventory(filepath.Join(root, filepath.FromSlash(InventoryRelPath)))
	if err != nil {
		t.Fatalf("load frozen inventory: %v", err)
	}

	report, err := ReconcileComplete(
		root,
		inventory,
		families,
		live,
		BaselineConsumerRegistry(),
		BaselineNegativeCoverageExceptions(),
	)
	if err == nil {
		t.Fatal("ReconcileComplete expected error on zero-case families, got nil")
	}

	invErr, ok := err.(*InventoryError)
	if !ok {
		t.Fatalf("expected *InventoryError, got %T: %v", err, err)
	}

	zeroCasePoint := CoveragePoint{
		FamilyID: "protocol.client.BoneMeal",
		Version:  "45",
	}
	foundInError := false
	for _, prob := range invErr.Problems {
		if strings.Contains(prob, zeroCasePoint.FamilyID) && strings.Contains(prob, zeroCasePoint.Version) {
			foundInError = true
			break
		}
	}
	if !foundInError {
		t.Fatalf("expected uncovered point %v in error problems, got %v", zeroCasePoint, invErr.Problems)
	}

	foundUncovered := false
	for _, pt := range report.Uncovered {
		if pt == zeroCasePoint {
			foundUncovered = true
			break
		}
	}
	if !foundUncovered {
		t.Fatalf("expected zero-case family %v in report.Uncovered, got %v", zeroCasePoint, report.Uncovered)
	}

	assertCoveragePointsSorted(t, "Covered", report.Covered)
	assertCoveragePointsSorted(t, "Uncovered", report.Uncovered)
}

func TestContractInventoryRejectsUnknownConsumer(t *testing.T) {
	root, families, live := discoverLive(t)
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no cases discovered")
	}

	inv := inventoryFrom(live, families, cases)
	inv.Cases[0].RustConsumer = "nonexistent_consumer"

	_, err = ReconcileWorking(
		root,
		inv,
		families,
		live,
		BaselineConsumerRegistry(),
		BaselineNegativeCoverageExceptions(),
	)
	if err == nil || !strings.Contains(err.Error(), "unknown consumer") {
		t.Fatalf("ReconcileWorking: expected unknown consumer error, got %v", err)
	}

	_, err = ReconcileComplete(
		root,
		inv,
		families,
		live,
		BaselineConsumerRegistry(),
		BaselineNegativeCoverageExceptions(),
	)
	if err == nil || !strings.Contains(err.Error(), "unknown consumer") {
		t.Fatalf("ReconcileComplete: expected unknown consumer error, got %v", err)
	}
}

func TestContractInventoryRejectsUnsupportedCaseVersion(t *testing.T) {
	root, families, live := discoverLive(t)
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no cases discovered")
	}

	inv := inventoryFrom(live, families, cases)
	newID := inv.Cases[0].Family + "/999/unsupported_version_case"
	inv.Cases[0].Version = "999"
	inv.Cases[0].ID = newID
	for i, f := range inv.Families {
		if f.ID == inv.Cases[0].Family {
			for j, id := range f.Cases {
				if id == cases[0].ID {
					inv.Families[i].Cases[j] = newID
				}
			}
		}
	}

	_, err = ReconcileWorking(
		root,
		inv,
		families,
		live,
		BaselineConsumerRegistry(),
		BaselineNegativeCoverageExceptions(),
	)
	if err == nil || (!strings.Contains(err.Error(), "supported_versions") && !strings.Contains(err.Error(), "unsupported case version")) {
		t.Fatalf("ReconcileWorking: expected unsupported version error, got %v", err)
	}

	_, err = ReconcileComplete(
		root,
		inv,
		families,
		live,
		BaselineConsumerRegistry(),
		BaselineNegativeCoverageExceptions(),
	)
	if err == nil || (!strings.Contains(err.Error(), "supported_versions") && !strings.Contains(err.Error(), "unsupported case version")) {
		t.Fatalf("ReconcileComplete: expected unsupported version error, got %v", err)
	}
}

func TestContractInventoryWorkingAndCompleteCoverage(t *testing.T) {
	root, families, live := discoverLive(t)
	frozen, err := LoadInventory(filepath.Join(root, filepath.FromSlash(InventoryRelPath)))
	if err != nil {
		t.Fatalf("load frozen inventory: %v", err)
	}

	// Create an inventory where protocol.frame has only its kind: "ok" case.
	framePoint := CoveragePoint{
		FamilyID: "protocol.frame",
		Version:  "45",
	}
	var filteredCases []CaseSpec
	for _, c := range frozen.Cases {
		if c.Family == "protocol.frame" {
			if strings.HasSuffix(c.ID, "/valid") {
				filteredCases = append(filteredCases, c)
			}
		} else {
			filteredCases = append(filteredCases, c)
		}
	}
	inv := Inventory{
		SchemaVersion:  frozen.SchemaVersion,
		SourceRevision: frozen.SourceRevision,
		Identities:     live,
		Families:       make([]Family, len(frozen.Families)),
		Cases:          filteredCases,
	}
	for i, f := range frozen.Families {
		inv.Families[i] = f
		inv.Families[i].SupportedVersions = append([]string(nil), f.SupportedVersions...)
		inv.Families[i].Sources = append([]SourceSpec(nil), f.Sources...)
		if f.ID == "protocol.frame" {
			inv.Families[i].Cases = []string{"protocol.frame/45/valid"}
		} else {
			inv.Families[i].Cases = append([]string(nil), f.Cases...)
		}
	}

	tests := []struct {
		name               string
		negativeExceptions NegativeCoverageExceptions
		wantCovered        bool
		wantErrSubstring   string
	}{
		{
			name:               "ok_only_without_exception",
			negativeExceptions: NegativeCoverageExceptions{},
			wantCovered:        false,
		},
		{
			name: "ok_only_with_reviewed_exception",
			negativeExceptions: NegativeCoverageExceptions{
				framePoint: "reviewed: frame parsing synthetic valid-only profile has no error representation",
			},
			wantCovered: true,
		},
		{
			name: "exception_with_empty_rationale",
			negativeExceptions: NegativeCoverageExceptions{
				framePoint: "   ",
			},
			wantErrSubstring: "empty rationale",
		},
		{
			name: "exception_for_unknown_point",
			negativeExceptions: NegativeCoverageExceptions{
				CoveragePoint{FamilyID: "unknown.family", Version: "1"}: "some rationale",
			},
			wantErrSubstring: "unknown family/version",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workingReport, workingErr := ReconcileWorking(
				root,
				inv,
				families,
				live,
				BaselineConsumerRegistry(),
				tc.negativeExceptions,
			)

			if tc.wantErrSubstring != "" {
				if workingErr == nil || !strings.Contains(workingErr.Error(), tc.wantErrSubstring) {
					t.Fatalf("ReconcileWorking: want error containing %q, got %v", tc.wantErrSubstring, workingErr)
				}
				completeReport, completeErr := ReconcileComplete(
					root,
					inv,
					families,
					live,
					BaselineConsumerRegistry(),
					tc.negativeExceptions,
				)
				_ = completeReport
				if completeErr == nil || !strings.Contains(completeErr.Error(), tc.wantErrSubstring) {
					t.Fatalf("ReconcileComplete: want error containing %q, got %v", tc.wantErrSubstring, completeErr)
				}
				return
			}

			if workingErr != nil {
				t.Fatalf("ReconcileWorking unexpected error: %v", workingErr)
			}

			assertCoveragePointsSorted(t, "working Covered", workingReport.Covered)
			assertCoveragePointsSorted(t, "working Uncovered", workingReport.Uncovered)

			hasPoint := func(pts []CoveragePoint, target CoveragePoint) bool {
				for _, pt := range pts {
					if pt == target {
						return true
					}
				}
				return false
			}

			if tc.wantCovered {
				if !hasPoint(workingReport.Covered, framePoint) {
					t.Fatalf("expected %v in working Covered, got %v", framePoint, workingReport.Covered)
				}
				if hasPoint(workingReport.Uncovered, framePoint) {
					t.Fatalf("expected %v NOT in working Uncovered, got %v", framePoint, workingReport.Uncovered)
				}
			} else {
				if hasPoint(workingReport.Covered, framePoint) {
					t.Fatalf("expected %v NOT in working Covered, got %v", framePoint, workingReport.Covered)
				}
				if !hasPoint(workingReport.Uncovered, framePoint) {
					t.Fatalf("expected %v in working Uncovered, got %v", framePoint, workingReport.Uncovered)
				}

				// In complete mode, without the exception, protocol.frame must be rejected
				completeReport, completeErr := ReconcileComplete(
					root,
					inv,
					families,
					live,
					BaselineConsumerRegistry(),
					tc.negativeExceptions,
				)
				if completeErr == nil {
					t.Fatalf("ReconcileComplete expected rejection of uncovered point %v, got nil", framePoint)
				}
				if !strings.Contains(completeErr.Error(), framePoint.FamilyID) || !strings.Contains(completeErr.Error(), framePoint.Version) {
					t.Fatalf("ReconcileComplete error should identify uncovered point %v, got %v", framePoint, completeErr)
				}
				if !hasPoint(completeReport.Uncovered, framePoint) {
					t.Fatalf("expected %v in complete Uncovered, got %v", framePoint, completeReport.Uncovered)
				}
				assertCoveragePointsSorted(t, "complete Covered", completeReport.Covered)
				assertCoveragePointsSorted(t, "complete Uncovered", completeReport.Uncovered)
			}
		})
	}
}

func assertCoveragePointsSorted(t *testing.T, label string, points []CoveragePoint) {
	t.Helper()
	for i := 0; i < len(points)-1; i++ {
		curr, next := points[i], points[i+1]
		if curr.FamilyID > next.FamilyID || (curr.FamilyID == next.FamilyID && curr.Version >= next.Version) {
			t.Fatalf("%s points not sorted: points[%d]=%+v >= points[%d]=%+v", label, i, curr, i+1, next)
		}
	}
}
