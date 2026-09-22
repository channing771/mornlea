package main

import (
	"crypto/sha256"
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
	_, err = ReconcileWorking(root, inv, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
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

	_, err = ReconcileWorking(root, inv, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
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

	_, err = ReconcileWorking(root, inv, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
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
		_, err := ReconcileWorking(root, inv, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
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

	_, err = ReconcileWorking(root, inv, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
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

func TestLoadInventoryRejectsNonRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := LoadInventory(path)
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected manifest regular-file rejection, got %v", err)
	}
}

type inventoryAssetFixture struct {
	root      string
	inventory Inventory
	families  []Family
	live      Identities
}

func newInventoryAssetFixture(t *testing.T, inputFormat string, input []byte, withEncoded bool) inventoryAssetFixture {
	t.Helper()
	root := t.TempDir()
	source := writeFixtureAsset(t, root, "assets/source.txt", []byte("source\n"))
	inputExt := ".bin"
	if inputFormat == "json" {
		inputExt = ".json"
	}
	inputAsset := writeFixtureAsset(t, root, "assets/input"+inputExt, input)
	expected := writeFixtureAsset(t, root, "assets/expected.json", []byte(`{"kind":"ok"}`))
	var encoded *AssetRef
	if withEncoded {
		asset := writeFixtureAsset(t, root, "assets/encoded.bin", []byte{0x01})
		encoded = &asset
	}

	caseSpec := CaseSpec{
		ID:           "protocol.frame/45/budget",
		Family:       "protocol.frame",
		Version:      "45",
		Operation:    "decode",
		Input:        inputAsset,
		InputFormat:  inputFormat,
		Expected:     expected,
		Encoded:      encoded,
		Checkpoints:  []string{"0"},
		RustConsumer: "corpus_frame",
	}
	baseFamily := func(id, kind, role, version string) Family {
		return Family{
			ID:                id,
			Kind:              kind,
			Role:              role,
			CurrentVersion:    version,
			SupportedVersions: []string{version},
			Source:            "synthetic registry",
			EventualOwner:     "synthetic owner",
			NumericSemantics:  "synthetic semantics",
			Sources:           []SourceSpec{{Path: source.Path, SHA256: source.SHA256}},
		}
	}
	families := []Family{
		baseFamily("protocol.frame", "protocol", "input", "45"),
		baseFamily("domain.synthetic", "domain", "event", "1"),
		baseFamily("save.synthetic", "save", "input", "1"),
		baseFamily("kernel.synthetic", "kernel", "input", "1"),
		baseFamily("agent.synthetic", "agent", "input", "1"),
	}
	families[0].Cases = []string{caseSpec.ID}
	live := Identities{
		Protocol:           45,
		ChunkSchema:        9,
		PlayerSchema:       9,
		WorldMetadata:      6,
		CompanionsAISchema: 5,
		HostileMobsSchema:  2,
		PassiveMobsSchema:  1,
		EngineABI:          11,
		RegionFormat:       1,
		AgentHTTP:          "v1",
		AgentMCP:           "v1",
	}
	return inventoryAssetFixture{
		root: root,
		inventory: Inventory{
			SchemaVersion:  inventorySchemaVersion,
			SourceRevision: BaselineSourceRevision,
			Identities:     live,
			Families:       cloneFamilies(families),
			Cases:          []CaseSpec{caseSpec},
		},
		families: cloneFamilies(families),
		live:     live,
	}
}

func cloneFamilies(families []Family) []Family {
	cloned := make([]Family, len(families))
	for i, family := range families {
		family.SupportedVersions = append([]string(nil), family.SupportedVersions...)
		family.Sources = append([]SourceSpec(nil), family.Sources...)
		family.Cases = append([]string(nil), family.Cases...)
		cloned[i] = family
	}
	return cloned
}

func writeFixtureAsset(t *testing.T, root, rel string, data []byte) AssetRef {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return AssetRef{Path: rel, SHA256: fmt.Sprintf("sha256:%x", sum)}
}

func exactJSONObject(t *testing.T, size int64) []byte {
	t.Helper()
	const prefix = `{"padding":"`
	const suffix = `"}`
	padding := int(size) - len(prefix) - len(suffix)
	if padding < 0 {
		t.Fatalf("JSON size %d is below wrapper size", size)
	}
	return []byte(prefix + strings.Repeat("x", padding) + suffix)
}

func directoryAssetRef(path string) AssetRef {
	return AssetRef{Path: path, SHA256: "sha256:" + strings.Repeat("0", 64)}
}

func assetRefAsSource(path string) SourceSpec {
	asset := directoryAssetRef(path)
	return SourceSpec{Path: asset.Path, SHA256: asset.SHA256}
}
