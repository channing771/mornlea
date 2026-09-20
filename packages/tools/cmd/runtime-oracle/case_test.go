package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractInventoryRejectsTamperedPayload(t *testing.T) {
	root, families, live := discoverLive(t)
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no cases discovered")
	}

	relTampered := "testdata/runtime-migration/cases/frame/valid_tampered.bin"
	fullTampered := filepath.Join(root, filepath.FromSlash(relTampered))
	// Original is [0x02, 0x00, 0x2d]; tampered is [0x02, 0x00, 0x2c]
	if err := os.WriteFile(fullTampered, []byte{0x02, 0x00, 0x2c}, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fullTampered)

	inv := inventoryFrom(live, families, cases)
	// Point case input to tampered file without updating SHA256
	inv.Cases[0].Input.Path = relTampered
	err = Reconcile(root, inv, families, live)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected sha256 mismatch for tampered payload, got: %v", err)
	}
}

func TestContractInventoryRejectsDuplicateCaseID(t *testing.T) {
	root, families, live := discoverLive(t)
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}

	inv := inventoryFrom(live, families, cases)
	dupCase := cases[0]
	inv.Cases = append(inv.Cases, dupCase)

	err = Reconcile(root, inv, families, live)
	if err == nil || !strings.Contains(err.Error(), "duplicate case "+dupCase.ID) {
		t.Fatalf("expected duplicate case error, got: %v", err)
	}
}

func TestContractInventoryRejectsAbsentRustConsumer(t *testing.T) {
	root, families, live := discoverLive(t)
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}

	inv := inventoryFrom(live, families, cases)
	inv.Cases[0].RustConsumer = "  "

	err = Reconcile(root, inv, families, live)
	if err == nil || !strings.Contains(err.Error(), "missing rust_consumer") {
		t.Fatalf("expected missing rust_consumer error, got: %v", err)
	}
}

func TestContractInventoryRejectsInvalidPaths(t *testing.T) {
	root, families, live := discoverLive(t)
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}

	for _, invalid := range []string{
		"/absolute/path/valid.bin",
		"testdata/../cases/frame/valid.bin",
		"cases\\frame\\valid.bin",
	} {
		inv := inventoryFrom(live, families, cases)
		inv.Cases[0].Input.Path = invalid
		err := Reconcile(root, inv, families, live)
		if err == nil {
			t.Fatalf("expected path error for %q, got nil", invalid)
		}
	}
}

func TestContractInventoryRejectsOversizedBudgets(t *testing.T) {
	root, families, live := discoverLive(t)
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}

	// 8193 cases
	inv := inventoryFrom(live, families, cases)
	hugeCases := make([]CaseSpec, 8193)
	for i := range hugeCases {
		hugeCases[i] = cases[0]
		hugeCases[i].ID = fmt.Sprintf("protocol.frame/45/valid_%d", i)
	}
	inv.Cases = hugeCases

	tmpManifest := filepath.Join(t.TempDir(), "huge_manifest.json")
	data, err := encodeInventory(inv)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmpManifest, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadInventory(tmpManifest)
	if err == nil || (!strings.Contains(err.Error(), "exceeds maximum") && !strings.Contains(err.Error(), "exceeds max")) {
		t.Fatalf("expected budget error, got: %v", err)
	}
}

func TestContractInventoryRejectsGoSourceAsBinaryAsset(t *testing.T) {
	root, families, live := discoverLive(t)
	cases, err := DiscoverCases(root)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}

	inv := inventoryFrom(live, families, cases)
	// A readable .go source path cannot satisfy a binary case
	inv.Cases[0].Input.Path = "packages/shared/network/codec/frame.go"
	inv.Cases[0].Input.SHA256, _ = hashFile(filepath.Join(root, "packages/shared/network/codec/frame.go"))
	inv.Cases[0].InputFormat = "binary"

	err = Reconcile(root, inv, families, live)
	if err == nil || !strings.Contains(err.Error(), "cannot be a Go source file") {
		t.Fatalf("expected rejection of .go file as binary asset, got: %v", err)
	}
}

func TestContractInventoryRejectsDuplicateJSONKeys(t *testing.T) {
	duplicateJSON := []byte(`{
		"schema_version": 2,
		"source_revision": "60c476645ee6dae1f6392336a7f3c593d2163ae3",
		"identities": {
			"protocol": 45,
			"protocol": 46
		}
	}`)
	tmp := filepath.Join(t.TempDir(), "dup.json")
	if err := os.WriteFile(tmp, duplicateJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadInventory(tmp)
	if err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("expected duplicate key error, got: %v", err)
	}
}
