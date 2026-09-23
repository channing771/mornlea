package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// This file is the test-only protocol manifest candidate helper. Packet
// producer groups register one family at a time, so the candidate manifest
// grows through the same reviewed merge the `domain.event` chain uses: the
// frozen manifest is cloned, only the selected protocol families change, and
// the complete candidate is reconciled through the production inventory gate
// before it is published for controller review. The tracked manifest is never
// written here.

// protocolManifestWorkspacePrefix names the harness-owned temporary directory
// that stages one candidate's assets, so a stray staging directory is
// recognizable and never mistaken for evidence.
const protocolManifestWorkspacePrefix = "mornlea-protocol-candidate-"

// ProtocolSelection is one producer's exact reviewed registration: its case
// specifications, the Go sources its rules are read from, and the executable
// routes it claims. A selection returns only values its own produced evidence
// carries, so the merged candidate never synthesizes a case, a source or a
// route the review did not see.
type ProtocolSelection struct {
	ProducerID  string
	Cases       []CaseSpec
	SourcePaths map[string][]string
	Routes      []ConsumerRoute
}

// mergeProtocolSelections merges the protocol selections into a clone of the
// base manifest, proves the merged value reconciles, and returns the
// round-tripped result.
//
// Reconciliation runs against the live repository. A candidate's own assets may
// not be tracked yet, so the verification accepts exactly one deviation: the
// case assets of a case the base manifest does not register, which are the
// assets this node exports for review and the controller integrates. Every
// other reconciliation failure is a rejection, so a candidate can never pass by
// disagreeing with the current sources.
func mergeProtocolSelections(root string, base Inventory, groups ...ProtocolSelection) (Inventory, error) {
	merged, err := mergeProtocolSelectionsChecked(root, base, groups...)
	if err != nil {
		return Inventory{}, err
	}
	if err := verifyProtocolManifest(root, base, merged); err != nil {
		return Inventory{}, fmt.Errorf("runtime-oracle: merged manifest does not reconcile: %w", err)
	}
	encoded, err := encodeInventory(merged)
	if err != nil {
		return Inventory{}, fmt.Errorf("runtime-oracle: encode merged manifest: %w", err)
	}
	path := filepath.Join(os.TempDir(), protocolManifestWorkspacePrefix+"manifest.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return Inventory{}, fmt.Errorf("runtime-oracle: write merged manifest copy: %w", err)
	}
	defer os.Remove(path)
	reloaded, err := LoadInventory(path)
	if err != nil {
		return Inventory{}, fmt.Errorf("runtime-oracle: reload merged manifest: %w", err)
	}
	if err := verifyProtocolManifest(root, base, reloaded); err != nil {
		return Inventory{}, fmt.Errorf("runtime-oracle: reloaded merged manifest does not reconcile: %w", err)
	}
	return reloaded, nil
}

// verifyProtocolManifest reconciles one manifest against the current registries
// and tolerates only the candidate assets awaiting integration.
//
// The tolerance is narrow on purpose: a problem counts as pending only when it
// names an asset path of a case the base manifest does not register and that
// path is absent from disk. A digest mismatch, a missing provenance source or a
// registration error keeps the file present or names a tracked value, so none of
// them can be classified as pending.
func verifyProtocolManifest(root string, base Inventory, inventory Inventory) error {
	families, live, err := Discover(root)
	if err != nil {
		return fmt.Errorf("runtime-oracle: discover registries: %w", err)
	}
	_, err = ReconcileWorking(root, inventory, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions())
	if err == nil {
		return nil
	}
	var inventoryErr *InventoryError
	if !errors.As(err, &inventoryErr) {
		return err
	}
	pending := pendingCandidateAssets(root, base, inventory)
	if len(pending) == 0 {
		return err
	}
	for _, problem := range inventoryErr.Problems {
		if !problemNamesPendingAsset(problem, pending) {
			return err
		}
	}
	return nil
}

// pendingCandidateAssets lists the asset paths of a newly registered case that
// are absent from disk, which is the reviewed-candidate state before the
// controller integrates the assets.
func pendingCandidateAssets(root string, base Inventory, inventory Inventory) map[string]bool {
	tracked := make(map[string]bool, len(base.Cases))
	for _, c := range base.Cases {
		tracked[c.ID] = true
	}
	pending := make(map[string]bool)
	for _, c := range inventory.Cases {
		if tracked[c.ID] {
			continue
		}
		refs := []AssetRef{c.Input, c.Expected}
		if c.Encoded != nil {
			refs = append(refs, *c.Encoded)
		}
		for _, ref := range refs {
			if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(ref.Path))); os.IsNotExist(statErr) {
				pending[ref.Path] = true
			}
		}
	}
	return pending
}

// problemNamesPendingAsset reports whether one reconciliation problem is about a
// pending candidate asset.
func problemNamesPendingAsset(problem string, pending map[string]bool) bool {
	for path := range pending {
		if strings.Contains(problem, path) {
			return true
		}
	}
	return false
}

// mergeProtocolSelectionsChecked performs the merge and reports every rejection
// as an error. It clones the base before any mutation, so a rejected merge
// leaves the caller's value untouched.
func mergeProtocolSelectionsChecked(root string, base Inventory, groups ...ProtocolSelection) (Inventory, error) {
	if len(groups) == 0 {
		return Inventory{}, fmt.Errorf("runtime-oracle: merge needs at least one protocol selection")
	}
	registry := BaselineConsumerRegistry()

	merged := Inventory{
		SchemaVersion:  base.SchemaVersion,
		SourceRevision: base.SourceRevision,
		Identities:     base.Identities,
		Families:       make([]Family, 0, len(base.Families)),
		Cases:          make([]CaseSpec, 0, len(base.Cases)),
	}
	for _, family := range base.Families {
		family.SupportedVersions = append([]string(nil), family.SupportedVersions...)
		family.Sources = append([]SourceSpec(nil), family.Sources...)
		family.Cases = append([]string(nil), family.Cases...)
		merged.Families = append(merged.Families, family)
	}
	merged.Cases = append(merged.Cases, base.Cases...)

	familiesByID := make(map[string]Family, len(merged.Families))
	for _, family := range merged.Families {
		if _, exists := familiesByID[family.ID]; exists {
			return Inventory{}, fmt.Errorf("base manifest registers family %s twice", family.ID)
		}
		familiesByID[family.ID] = family
	}
	baseByID := make(map[string]CaseSpec, len(base.Cases))
	for _, c := range base.Cases {
		if _, exists := baseByID[c.ID]; exists {
			return Inventory{}, fmt.Errorf("base manifest registers case %s twice", c.ID)
		}
		baseByID[c.ID] = c
	}

	selectedFamilies := make(map[string]bool)
	pendingByID := make(map[string]CaseSpec)
	for index, group := range groups {
		if _, err := validateProducerID(group.ProducerID); err != nil {
			return Inventory{}, fmt.Errorf("selection %d producer ID: %w", index, err)
		}
		if !validProducerIDs[group.ProducerID] {
			return Inventory{}, fmt.Errorf("selection %d names unrecognized producer ID %q", index, group.ProducerID)
		}
		for _, relative := range selectionSourcePaths(group) {
			if err := validateCorpusPath(relative); err != nil {
				return Inventory{}, fmt.Errorf("selection %d provenance source %s: %w", index, relative, err)
			}
			if _, err := requireRegularFile(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
				return Inventory{}, fmt.Errorf("selection %d provenance source %s: %w", index, relative, err)
			}
		}
		routes := make(map[ConsumerRoute]bool, len(group.Routes))
		for _, route := range group.Routes {
			if !protocolRouteRegistered(registry, route) {
				return Inventory{}, fmt.Errorf(
					"selection %d claims route %s/%s/%s, which no consumer registration carries",
					index, route.FamilyID, route.Version, route.Operation,
				)
			}
			if routes[route] {
				return Inventory{}, fmt.Errorf("selection %d claims route %s/%s/%s twice", index, route.FamilyID, route.Version, route.Operation)
			}
			routes[route] = true
		}
		if len(group.Cases) == 0 {
			return Inventory{}, fmt.Errorf("selection %d registers no case", index)
		}
		for _, c := range group.Cases {
			if err := validateProtocolCase(c, familiesByID); err != nil {
				return Inventory{}, fmt.Errorf("selection %d case %s: %w", index, c.ID, err)
			}
			route := ConsumerRoute{FamilyID: c.Family, Version: c.Version, Operation: c.Operation}
			if !routes[route] {
				return Inventory{}, fmt.Errorf(
					"selection %d case %s names route %s/%s/%s, which the selection does not claim",
					index, c.ID, route.FamilyID, route.Version, route.Operation,
				)
			}
			if !protocolRouteRegistered(registry, route) {
				return Inventory{}, fmt.Errorf(
					"selection %d case %s names route %s/%s/%s, which no consumer registration carries",
					index, c.ID, route.FamilyID, route.Version, route.Operation,
				)
			}
			if _, exists := pendingByID[c.ID]; exists {
				return Inventory{}, fmt.Errorf("selections register case %s twice", c.ID)
			}
			pendingByID[c.ID] = c
			selectedFamilies[c.Family] = true
		}
	}

	addedIDs := make([]string, 0, len(pendingByID))
	for id, c := range pendingByID {
		existing, exists := baseByID[id]
		if exists && !reflect.DeepEqual(existing, c) {
			return Inventory{}, fmt.Errorf("selection case %s conflicts with the registered case", id)
		}
		if !exists {
			addedIDs = append(addedIDs, id)
		}
	}
	sort.Strings(addedIDs)
	for _, id := range addedIDs {
		merged.Cases = append(merged.Cases, pendingByID[id])
	}
	sort.Slice(merged.Cases, func(i, j int) bool { return merged.Cases[i].ID < merged.Cases[j].ID })

	for index := range merged.Families {
		family := merged.Families[index]
		if !selectedFamilies[family.ID] {
			continue
		}
		if !strings.HasPrefix(family.ID, "protocol.") {
			return Inventory{}, fmt.Errorf("selection registers non-protocol family %s", family.ID)
		}

		// The family's case list is the sorted union of every top-level case
		// the merged manifest carries for it. A listed case the top-level list
		// no longer carries would be silently unregistered, so it is rejected.
		var listed []string
		for _, c := range merged.Cases {
			if c.Family == family.ID {
				listed = append(listed, c.ID)
			}
		}
		sort.Strings(listed)
		registered := make(map[string]bool, len(listed))
		for _, id := range listed {
			registered[id] = true
		}
		for _, id := range family.Cases {
			if !registered[id] {
				return Inventory{}, fmt.Errorf("family %s lists case %s, which is absent from the top-level case list", family.ID, id)
			}
		}
		merged.Families[index].Cases = listed

		// Provenance is the union of the registered sources and the selection's
		// own, rehashed from the live files, and sorted by path. Every other
		// family field keeps its registered value.
		paths := make(map[string]bool, len(family.Sources))
		for _, source := range family.Sources {
			paths[source.Path] = true
		}
		for _, group := range groups {
			for _, relative := range group.SourcePaths[family.ID] {
				paths[relative] = true
			}
		}
		relatives := make([]string, 0, len(paths))
		for relative := range paths {
			relatives = append(relatives, relative)
		}
		sort.Strings(relatives)
		sources := make([]SourceSpec, 0, len(relatives))
		for _, relative := range relatives {
			hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
			if err != nil {
				return Inventory{}, fmt.Errorf("hash provenance source %s: %w", relative, err)
			}
			sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
		}
		merged.Families[index].Sources = sources
	}

	if merged.SourceRevision != base.SourceRevision {
		return Inventory{}, fmt.Errorf("merged source_revision %s, want %s", merged.SourceRevision, base.SourceRevision)
	}
	return merged, nil
}

// selectionSourcePaths lists every provenance path one selection names, in a
// deterministic order.
func selectionSourcePaths(group ProtocolSelection) []string {
	seen := make(map[string]bool)
	for _, paths := range group.SourcePaths {
		for _, relative := range paths {
			seen[relative] = true
		}
	}
	relatives := make([]string, 0, len(seen))
	for relative := range seen {
		relatives = append(relatives, relative)
	}
	sort.Strings(relatives)
	return relatives
}

// validateProtocolCase checks one selected case against its family without
// touching its assets: identity, version, operation, consumer, route and
// checkpoints. Asset existence and digests are the reconciliation's job,
// because a candidate's assets are reviewed before they are tracked.
func validateProtocolCase(c CaseSpec, families map[string]Family) error {
	family, ok := families[c.Family]
	if !ok {
		return fmt.Errorf("unknown family %s", c.Family)
	}
	if !strings.HasPrefix(c.Family, "protocol.") {
		return fmt.Errorf("family %s is not a protocol family", c.Family)
	}
	prefix := c.Family + "/" + c.Version + "/"
	if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
		return fmt.Errorf("id %q must match %s<label>", c.ID, prefix)
	}
	if !containsString(family.SupportedVersions, c.Version) {
		return fmt.Errorf("version %q is absent from family supported_versions", c.Version)
	}
	if strings.TrimSpace(c.RustConsumer) == "" {
		return fmt.Errorf("missing rust_consumer")
	}
	switch c.Operation {
	case "decode", "encode":
	default:
		return fmt.Errorf("invalid operation %q", c.Operation)
	}
	if strings.TrimSpace(c.InputFormat) != "binary" && strings.TrimSpace(c.InputFormat) != "json" {
		return fmt.Errorf("invalid input_format %q", c.InputFormat)
	}
	if len(c.Checkpoints) == 0 {
		return fmt.Errorf("empty checkpoints")
	}
	seen := make(map[string]bool, len(c.Checkpoints))
	for _, checkpoint := range c.Checkpoints {
		if seen[checkpoint] {
			return fmt.Errorf("duplicate checkpoint %q", checkpoint)
		}
		seen[checkpoint] = true
	}
	return nil
}

// protocolRouteRegistered reports whether the closed consumer registry carries
// the route on any consumer, so a selection cannot claim an executable route no
// consumer publishes.
func protocolRouteRegistered(registry ConsumerRegistry, route ConsumerRoute) bool {
	for _, registration := range registry {
		if _, ok := registration.Routes[route]; ok {
			return true
		}
	}
	return false
}

// TestProtocolCorpusManifestMergeRegistersFrameEncodeRoute pins that the merge
// adds exactly the encode case and its route-bearing family registration while
// every unrelated family, case and digest keeps its frozen value.
func TestProtocolCorpusManifestMergeRegistersFrameEncodeRoute(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	candidate := frameProtocolCandidate(t, root)
	merged, err := mergeProtocolSelections(root, base, frameProtocolSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}
	if merged.SchemaVersion != base.SchemaVersion || !reflect.DeepEqual(merged.Identities, base.Identities) {
		t.Fatal("merged manifest rewrites the schema or the identity matrix")
	}
	// The merged case list is the union of the base and the selection, so a
	// re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	if !frameCaseRegistered(base, frameEncodeCaseID) {
		wantCases++
	}
	if len(merged.Cases) != wantCases {
		t.Fatalf("merged carries %d cases, want %d", len(merged.Cases), wantCases)
	}
	for index := 1; index < len(merged.Cases); index++ {
		if merged.Cases[index].ID < merged.Cases[index-1].ID {
			t.Fatalf("merged case list is not sorted by id at %d", index)
		}
	}

	var frame *Family
	for index := range merged.Families {
		if merged.Families[index].ID == frameFamily {
			frame = &merged.Families[index]
			break
		}
	}
	if frame == nil {
		t.Fatalf("merged manifest has no %s family", frameFamily)
	}
	want := []string{frameEncodeCaseID, frameNoncanonicalCaseID, frameValidCaseID}
	if !reflect.DeepEqual(frame.Cases, want) {
		t.Fatalf("%s family lists %v, want %v", frameFamily, frame.Cases, want)
	}
	sources := make(map[string]bool, len(frame.Sources))
	for _, source := range frame.Sources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(source.Path)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", source.Path, err)
		}
		if hash != source.SHA256 {
			t.Fatalf("provenance source %s records %s, disk has %s", source.Path, source.SHA256, hash)
		}
		sources[source.Path] = true
	}
	for _, required := range []string{
		"packages/shared/network/codec/frame.go",
		"packages/shared/network/codec/frame_test.go",
	} {
		if !sources[required] {
			t.Fatalf("%s provenance drops %s", frameFamily, required)
		}
	}

	found := false
	for _, c := range merged.Cases {
		if c.ID == frameEncodeCaseID {
			found = true
			if !reflect.DeepEqual(c, candidate.Spec) {
				t.Fatalf("merged case %s is %#v, want %#v", c.ID, c, candidate.Spec)
			}
		}
	}
	if !found {
		t.Fatalf("merged manifest does not register %s", frameEncodeCaseID)
	}
}

// frameCaseRegistered reports whether a manifest already registers one case
// identity, so a re-merge of an integrated candidate is an idempotent no-op.
func frameCaseRegistered(inventory Inventory, id string) bool {
	for _, c := range inventory.Cases {
		if c.ID == id {
			return true
		}
	}
	return false
}

// TestProtocolCorpusManifestMergePreservesUnrelatedFamilies pins that the merge
// touches nothing but the selected protocol families: every other family row
// and every unrelated case is byte-semantically unchanged.
func TestProtocolCorpusManifestMergePreservesUnrelatedFamilies(t *testing.T) {
	root := mustRepoRoot(t)
	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, frameProtocolSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	mergedFamilies := make(map[string]Family, len(merged.Families))
	for _, family := range merged.Families {
		mergedFamilies[family.ID] = family
	}
	for _, family := range base.Families {
		if family.ID == frameFamily {
			continue
		}
		if !reflect.DeepEqual(mergedFamilies[family.ID], family) {
			t.Fatalf("merged manifest rewrites unrelated family %s", family.ID)
		}
	}
	mergedByID := make(map[string]CaseSpec, len(merged.Cases))
	for _, c := range merged.Cases {
		mergedByID[c.ID] = c
	}
	unrelated := 0
	for _, c := range base.Cases {
		if c.Family == frameFamily {
			continue
		}
		unrelated++
		if got, ok := mergedByID[c.ID]; !ok || !reflect.DeepEqual(got, c) {
			t.Fatalf("merged manifest rewrites unrelated case %s", c.ID)
		}
	}
	mergedUnrelated := 0
	for _, c := range merged.Cases {
		if c.Family != frameFamily {
			mergedUnrelated++
		}
	}
	if mergedUnrelated != unrelated {
		t.Fatalf("merged manifest carries %d unrelated cases, base carries %d", mergedUnrelated, unrelated)
	}
}

// TestProtocolCorpusManifestMergeRejectsDuplicateAndConflictingCases pins that
// a case registered twice, or re-registered with one field rewritten, is
// rejected before any merge effect.
func TestProtocolCorpusManifestMergeRejectsDuplicateAndConflictingCases(t *testing.T) {
	root := mustRepoRoot(t)
	base := loadRealManifest(t, root)
	selection := frameProtocolSelection(t, root)

	duplicated := selection
	duplicated.Cases = append(append([]CaseSpec(nil), selection.Cases...), selection.Cases[0])
	if _, err := mergeProtocolSelectionsChecked(root, base, duplicated); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("expected a duplicate-case rejection, got %v", err)
	}

	registered := base.Cases[0]
	for _, c := range base.Cases {
		if c.ID == frameValidCaseID {
			registered = c
			break
		}
	}
	if registered.ID != frameValidCaseID {
		t.Fatalf("tracked manifest registers no %s case", frameValidCaseID)
	}
	rewritten := registered
	rewritten.Checkpoints = []string{"9"}
	conflicting := selection
	conflicting.Cases = append(append([]CaseSpec(nil), selection.Cases...), rewritten)
	if _, err := mergeProtocolSelectionsChecked(root, base, conflicting); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected a conflicting-registration rejection, got %v", err)
	}
}

// TestProtocolCorpusManifestMergeRejectsUnregisteredRoute pins that a selection
// cannot claim a route no consumer registration carries, so the candidate
// manifest can never name an unexecutable operation.
func TestProtocolCorpusManifestMergeRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	base := loadRealManifest(t, root)
	selection := frameProtocolSelection(t, root)
	selection.Routes = append(selection.Routes, ConsumerRoute{
		FamilyID:  "protocol.client.ClientHello",
		Version:   "45",
		Operation: "admit",
	})

	if _, err := mergeProtocolSelectionsChecked(root, base, selection); err == nil ||
		!strings.Contains(err.Error(), "no consumer registration carries") {
		t.Fatalf("expected an unregistered-route rejection, got %v", err)
	}
}

// TestProtocolCorpusManifestMergeRejectsUnclaimedCaseRoute pins that a case
// naming a route its own selection does not claim is rejected, so a case can
// never travel with a family registration it does not belong to.
func TestProtocolCorpusManifestMergeRejectsUnclaimedCaseRoute(t *testing.T) {
	root := mustRepoRoot(t)
	base := loadRealManifest(t, root)
	selection := frameProtocolSelection(t, root)
	selection.Routes = []ConsumerRoute{
		{FamilyID: frameFamily, Version: frameVersion, Operation: "decode"},
	}

	if _, err := mergeProtocolSelectionsChecked(root, base, selection); err == nil ||
		!strings.Contains(err.Error(), "does not claim") {
		t.Fatalf("expected an unclaimed-case-route rejection, got %v", err)
	}
}
