package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateRuntimeInventory = flag.Bool(
	"update-runtime-inventory",
	false,
	"rewrite testdata/runtime-migration/contracts.json from live registries",
)

func TestContractInventoryReconcilesFrozenCorpus(t *testing.T) {
	root, families, live := discoverLive(t)
	if *updateRuntimeInventory {
		encoded, err := encodeInventory(inventoryFrom(live, families))
		if err != nil {
			t.Fatalf("encode inventory: %v", err)
		}
		path := filepath.Join(root, filepath.FromSlash(InventoryRelPath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create inventory directory: %v", err)
		}
		if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
			t.Fatalf("write inventory: %v", err)
		}
	}

	inventory, err := LoadInventory(filepath.Join(root, filepath.FromSlash(InventoryRelPath)))
	if err != nil {
		t.Fatalf("load frozen inventory: %v", err)
	}
	if err := Reconcile(root, inventory, families, live); err != nil {
		t.Fatalf("frozen inventory drifted from current registries: %v", err)
	}
	assertRequiredCoverage(t, inventory.Families)
}

func TestContractInventoryRejectsMissingFamily(t *testing.T) {
	root, families, live := discoverLive(t)
	if len(families) == 0 {
		t.Fatal("discover produced no families")
	}
	inventory := inventoryFrom(live, families[1:])
	err := Reconcile(root, inventory, families, live)
	if err == nil || !strings.Contains(err.Error(), "uncovered family "+families[0].ID) {
		t.Fatalf("missing family %s: error=%v", families[0].ID, err)
	}

	inventory = inventoryFrom(live, families)
	inventory.Families = append(inventory.Families, Family{
		ID: "protocol.missing.Synthetic", Kind: "protocol", Role: "input",
		CurrentVersion: "1", SupportedVersions: []string{"1"},
		Source: "does-not-exist.go", EventualOwner: ownerProtocol,
		NumericSemantics: protocolLE, Fixtures: []string{"does-not-exist.go"},
	})
	err = Reconcile(root, inventory, families, live)
	if err == nil || !strings.Contains(err.Error(), "inventory family protocol.missing.Synthetic is not in current registries") {
		t.Fatalf("extra inventory family: error=%v", err)
	}
}

func TestContractInventoryRejectsVersionMismatch(t *testing.T) {
	root, families, live := discoverLive(t)
	inventory := inventoryFrom(live, families)
	inventory.Identities.Protocol++
	if inventory.Families[0].CurrentVersion == "" {
		t.Fatal("discovered family is missing a current version")
	}
	inventory.Families[0].CurrentVersion = "0"
	err := Reconcile(root, inventory, families, live)
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

func TestContractInventoryRejectsMissingCoverageFixture(t *testing.T) {
	root, families, live := discoverLive(t)
	inventory := inventoryFrom(live, families)
	inventory.Families[0].Fixtures = []string{"testdata/runtime-migration/missing-fixture.bin"}
	err := Reconcile(root, inventory, families, live)
	if err == nil || !strings.Contains(err.Error(), "fixture testdata/runtime-migration/missing-fixture.bin is missing") {
		t.Fatalf("missing fixture: error=%v", err)
	}

	inventory = inventoryFrom(live, families)
	inventory.Families[0].Fixtures = nil
	err = Reconcile(root, inventory, families, live)
	if err == nil || !strings.Contains(err.Error(), "family "+families[0].ID+" has no coverage fixture") {
		t.Fatalf("empty fixtures: error=%v", err)
	}
}

func TestContractInventoryRejectsIncompleteIdentity(t *testing.T) {
	root, families, live := discoverLive(t)
	inventory := inventoryFrom(live, families)
	inventory.Identities.AgentHTTP = ""
	inventory.Identities.EngineABI = 0
	err := Reconcile(root, inventory, families, live)
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

func inventoryFrom(live Identities, families []Family) Inventory {
	cloned := make([]Family, len(families))
	for i, family := range families {
		family.SupportedVersions = append([]string(nil), family.SupportedVersions...)
		family.Fixtures = append([]string(nil), family.Fixtures...)
		cloned[i] = family
	}
	return Inventory{
		SchemaVersion: inventorySchemaVersion,
		Identities:    live,
		Families:      cloned,
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
	for _, kind := range []string{"protocol", "save", "kernel", "agent"} {
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
