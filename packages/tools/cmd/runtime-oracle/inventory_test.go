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
	if err := Reconcile(root, inventory, families, live); err != nil {
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
	err = Reconcile(root, inventory, families, live)
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
	err = Reconcile(root, inventory, families, live)
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
	err = Reconcile(root, inventory, families, live)
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
	err = Reconcile(root, inventory, families, live)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing source: error=%v", err)
	}

	inventory = inventoryFrom(live, families, cases)
	inventory.Families[0].Sources = nil
	err = Reconcile(root, inventory, families, live)
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
	err = Reconcile(root, inventory, families, live)
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
