package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// This file is the shared test-only manifest-candidate helper for the
// `domain.event` corpus-registration chain. Each domain event producer
// exposes one selector with the same closed responsibility, and this helper
// merges those selections into the tracked manifest, validates the merged
// value against the production reconciliation, and publishes the complete
// candidate outside the repository. The tracked manifest is never written by
// this helper or by any ordinary test run: the candidate is reviewed first
// and then copied mechanically by the controller side.

const (
	// domainEventFamily is the shared family every event selection registers
	// its cases under. Exactly one family row with this identity may exist.
	domainEventFamily = "domain.event"
	// domainEventManifestProducerID is the exporter's producer identity for
	// the complete manifest candidate. The registration chain owns it; the
	// per-producer asset exporters keep their own identities.
	domainEventManifestProducerID = "runtime-oracle/domain-event-manifest"
)

// domainEventSelection is one producer's exact reviewed registration: its
// case specifications and its provenance paths. A selector returns only the
// values its own frozen corpus produced, so the merged candidate never
// synthesizes a case or a source the reviewed evidence does not carry.
type domainEventSelection struct {
	Cases   []CaseSpec
	Sources []string
}

// mergeDomainEventSelections merges the selections into a clone of the base
// manifest and returns the reconciled, round-tripped result.
//
// The merge is the only supported way to grow the shared `domain.event`
// registration: it keeps every unrelated family and case byte-semantically
// unchanged, unions provenance, and leaves the family's other fields alone.
// It fatals through `t` on any invalid input; the checked core returns the
// same verdicts as errors so the rejection tests can observe them.
func mergeDomainEventSelections(
	t *testing.T,
	root string,
	base Inventory,
	selections ...domainEventSelection,
) Inventory {
	t.Helper()
	merged, err := mergeDomainEventSelectionsChecked(root, base, selections...)
	if err != nil {
		t.Fatalf("runtime-oracle: merge domain event selections: %v", err)
	}
	families, live, err := Discover(root)
	if err != nil {
		t.Fatalf("runtime-oracle: discover registries: %v", err)
	}
	if _, err := ReconcileWorking(root, merged, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions()); err != nil {
		t.Fatalf("runtime-oracle: merged manifest does not reconcile: %v", err)
	}
	// The merged value must survive the same encode/load path the published
	// candidate takes, so a candidate can never differ from its in-memory
	// source by anything but the write itself.
	encoded, err := encodeInventory(merged)
	if err != nil {
		t.Fatalf("runtime-oracle: encode merged manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("runtime-oracle: write merged manifest copy: %v", err)
	}
	reloaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("runtime-oracle: reload merged manifest: %v", err)
	}
	if _, err := ReconcileWorking(root, reloaded, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions()); err != nil {
		t.Fatalf("runtime-oracle: reloaded merged manifest does not reconcile: %v", err)
	}
	return reloaded
}

// mergeDomainEventSelectionsChecked performs the merge and reports every
// rejection as an error. It clones the base before any mutation, so a
// rejected merge leaves the caller's value untouched.
func mergeDomainEventSelectionsChecked(
	root string,
	base Inventory,
	selections ...domainEventSelection,
) (Inventory, error) {
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

	// 1. Reject a duplicate case ID in the base, inside one selection, or
	// across two selections. A selection case the base already registers is
	// accepted only as the byte-identical spec, so re-merging a registered
	// corpus is an idempotent no-op instead of a silent rewrite.
	baseByID := make(map[string]CaseSpec, len(base.Cases))
	for _, c := range base.Cases {
		if _, exists := baseByID[c.ID]; exists {
			return Inventory{}, fmt.Errorf("base manifest registers case %s twice", c.ID)
		}
		baseByID[c.ID] = c
	}
	pendingByID := make(map[string]CaseSpec)
	for index, selection := range selections {
		for _, relative := range selection.Sources {
			if err := validateCorpusPath(relative); err != nil {
				return Inventory{}, fmt.Errorf("selection %d provenance source %s: %w", index, relative, err)
			}
		}
		for _, c := range selection.Cases {
			if c.Family != domainEventFamily {
				return Inventory{}, fmt.Errorf("selection %d registers case %s under family %s, want %s", index, c.ID, c.Family, domainEventFamily)
			}
			if _, exists := pendingByID[c.ID]; exists {
				return Inventory{}, fmt.Errorf("selections register case %s twice", c.ID)
			}
			pendingByID[c.ID] = c
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

	// 2. The complete top-level case list is sorted by ID; every carried
	// case value stays byte-semantically what its producer registered.
	sort.Slice(merged.Cases, func(i, j int) bool { return merged.Cases[i].ID < merged.Cases[j].ID })

	familyIndex, familyCount := -1, 0
	for index := range merged.Families {
		if merged.Families[index].ID == domainEventFamily {
			familyIndex = index
			familyCount++
		}
	}
	if familyCount != 1 {
		return Inventory{}, fmt.Errorf("base manifest carries %d %s families, want exactly 1", familyCount, domainEventFamily)
	}

	// 3. The family's case list is replaced with the sorted union of every
	// top-level `domain.event` case. A case the family currently lists that
	// the top-level list no longer carries would be silently unregistered by
	// that replacement, so it is rejected instead.
	familyTopLevel := make([]string, 0)
	for _, c := range merged.Cases {
		if c.Family == domainEventFamily {
			familyTopLevel = append(familyTopLevel, c.ID)
		}
	}
	sort.Strings(familyTopLevel)
	registered := make(map[string]bool, len(familyTopLevel))
	for _, id := range familyTopLevel {
		registered[id] = true
	}
	for _, id := range merged.Families[familyIndex].Cases {
		if !registered[id] {
			return Inventory{}, fmt.Errorf("%s family lists case %s, which is absent from the top-level case list", domainEventFamily, id)
		}
	}
	merged.Families[familyIndex].Cases = familyTopLevel

	// 4. Provenance is unioned by repository-relative path, every source
	// digest is recomputed from `root`, and the set is sorted by path. Every
	// other family field keeps its registered value.
	sourcePaths := make(map[string]bool)
	for _, source := range merged.Families[familyIndex].Sources {
		sourcePaths[source.Path] = true
	}
	for _, selection := range selections {
		for _, relative := range selection.Sources {
			sourcePaths[relative] = true
		}
	}
	paths := make([]string, 0, len(sourcePaths))
	for relative := range sourcePaths {
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	sources := make([]SourceSpec, 0, len(paths))
	for _, relative := range paths {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return Inventory{}, fmt.Errorf("hash %s provenance source %s: %w", domainEventFamily, relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	merged.Families[familyIndex].Sources = sources

	// 5. The "source_revision" field is preserved verbatim; refreshing it is
	// a later node's explicit act.
	return merged, nil
}

// writeDomainEventManifestCandidate publishes one complete manifest
// candidate below `runtime-oracle/domain-event-manifest/contracts.json` in
// the named external export root and returns the written path.
//
// An empty export root writes nothing and returns "". Directory safety is
// delegated to the existing external-export helpers: a repository-contained
// root, a symlinked ancestor, a non-fresh producer child or any escaping
// relative path is rejected there, and a rejection reports no candidate
// rather than fatal, so the boundary tests can observe it. A written
// candidate is loaded back through `LoadInventory` and reconciled before the
// path is returned, so a published candidate always validates.
func writeDomainEventManifestCandidate(
	t *testing.T,
	root string,
	exportRoot string,
	inventory Inventory,
) string {
	t.Helper()
	exportRoot = strings.TrimSpace(exportRoot)
	if exportRoot == "" {
		return ""
	}
	encoded, err := encodeInventory(inventory)
	if err != nil {
		t.Fatalf("runtime-oracle: encode manifest candidate: %v", err)
	}
	payload := append(encoded, '\n')
	published, err := exportGeneratedAssets(root, exportRoot, domainEventManifestProducerID, []generatedAsset{
		{RelativePath: "contracts.json", Data: payload},
	})
	if err != nil {
		t.Logf("runtime-oracle: manifest candidate export rejected: %v", err)
		return ""
	}
	candidate := filepath.Join(published, "contracts.json")
	reloaded, err := LoadInventory(candidate)
	if err != nil {
		t.Fatalf("runtime-oracle: reload manifest candidate: %v", err)
	}
	families, live, err := Discover(root)
	if err != nil {
		t.Fatalf("runtime-oracle: discover registries: %v", err)
	}
	if _, err := ReconcileWorking(root, reloaded, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions()); err != nil {
		t.Fatalf("runtime-oracle: manifest candidate does not reconcile: %v", err)
	}
	written, err := os.ReadFile(candidate)
	if err != nil {
		t.Fatalf("runtime-oracle: read manifest candidate: %v", err)
	}
	if !bytes.Equal(written, payload) {
		t.Fatalf("runtime-oracle: manifest candidate %s does not carry the encoded inventory", candidate)
	}
	return candidate
}

// domainEventFamilySpec indexes a manifest's `domain.event` family row.
func domainEventFamilySpec(inventory Inventory) (Family, bool) {
	for _, family := range inventory.Families {
		if family.ID == domainEventFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainEventMergedForTest merges the current mobs selection into the tracked
// manifest, the shape every helper test below observes.
func domainEventMergedForTest(t *testing.T, root string) (Inventory, Inventory) {
	t.Helper()
	base := loadRealManifest(t, root)
	return base, mergeDomainEventSelections(t, root, base, domainEventMobsSelection(t, root))
}

// TestDomainEventManifestMergeRejectsDuplicateCase pins the duplicate gate: a
// selection listing one case ID twice is rejected before any merge effect,
// and a base that registers one case twice is rejected the same way.
func TestDomainEventManifestMergeRejectsDuplicateCase(t *testing.T) {
	root := mustRepoRoot(t)
	base := loadRealManifest(t, root)
	selection := domainEventMobsSelection(t, root)
	if len(selection.Cases) == 0 {
		t.Fatal("mobs selection carries no case")
	}
	duplicated := domainEventSelection{
		Cases:   append(append([]CaseSpec(nil), selection.Cases...), selection.Cases[0]),
		Sources: selection.Sources,
	}
	if _, err := mergeDomainEventSelectionsChecked(root, base, duplicated); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("expected a duplicate-case rejection, got: %v", err)
	}

	doubledBase := base
	doubledBase.Cases = append(append([]CaseSpec(nil), base.Cases...), base.Cases[0])
	if _, err := mergeDomainEventSelectionsChecked(root, doubledBase, selection); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("expected a base duplicate rejection, got: %v", err)
	}

	conflicting := domainEventSelection{
		Cases:   append([]CaseSpec(nil), selection.Cases...),
		Sources: selection.Sources,
	}
	// Re-register one already-tracked domain.event case with one field
	// rewritten, so the base and the selection name the same ID with two
	// different specs. The conflict branch is observable in both the
	// pre-registration and the already-registered corpus state.
	registered := base.Cases[0]
	for _, c := range base.Cases {
		if c.Family == domainEventFamily {
			registered = c
			break
		}
	}
	if registered.Family != domainEventFamily {
		t.Fatalf("tracked manifest registers no %s case", domainEventFamily)
	}
	rewritten := registered
	rewritten.Checkpoints = []string{"9"}
	conflicting.Cases = append(conflicting.Cases, rewritten)
	if _, err := mergeDomainEventSelectionsChecked(root, base, conflicting); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected a conflicting-reregistration rejection, got: %v", err)
	}
}

// TestDomainEventManifestMergeRejectsMissingFamilyCase pins that a family
// case the top-level list no longer carries is rejected instead of being
// silently unregistered by the family-list replacement.
func TestDomainEventManifestMergeRejectsMissingFamilyCase(t *testing.T) {
	root := mustRepoRoot(t)
	base := loadRealManifest(t, root)
	missing := -1
	for index, c := range base.Cases {
		if c.Family == domainEventFamily {
			missing = index
			break
		}
	}
	if missing < 0 {
		t.Fatalf("tracked manifest registers no %s case", domainEventFamily)
	}
	thinned := base
	thinned.Cases = append(append([]CaseSpec(nil), base.Cases[:missing]...), base.Cases[missing+1:]...)
	family, ok := domainEventFamilySpec(thinned)
	if !ok || len(family.Cases) == 0 {
		t.Fatalf("tracked manifest has no %s family cases", domainEventFamily)
	}
	_, err := mergeDomainEventSelectionsChecked(root, thinned, domainEventMobsSelection(t, root))
	if err == nil || !strings.Contains(err.Error(), "absent from the top-level case list") {
		t.Fatalf("expected a missing-family-case rejection, got: %v", err)
	}
}

// TestDomainEventManifestMergeSortsCasesSourcesAndFamilyCases pins the
// deterministic shape of every merged candidate: the complete top-level case
// list, the family's case list, and the family's provenance set are sorted,
// and the merged provenance carries both a base-only and a selection-only
// path.
func TestDomainEventManifestMergeSortsCasesSourcesAndFamilyCases(t *testing.T) {
	root := mustRepoRoot(t)
	_, merged := domainEventMergedForTest(t, root)

	for index := 1; index < len(merged.Cases); index++ {
		if merged.Cases[index].ID < merged.Cases[index-1].ID {
			t.Fatalf("merged case list is not sorted by id at %d: %s then %s", index, merged.Cases[index-1].ID, merged.Cases[index].ID)
		}
	}
	family, ok := domainEventFamilySpec(merged)
	if !ok {
		t.Fatalf("merged manifest has no %s family", domainEventFamily)
	}
	for index := 1; index < len(family.Cases); index++ {
		if family.Cases[index] < family.Cases[index-1] {
			t.Fatalf("%s case list is not sorted at %d", domainEventFamily, index)
		}
	}
	for index := 1; index < len(family.Sources); index++ {
		if family.Sources[index].Path < family.Sources[index-1].Path {
			t.Fatalf("%s provenance is not sorted at %d: %s then %s", domainEventFamily, index, family.Sources[index-1].Path, family.Sources[index].Path)
		}
	}
	paths := make(map[string]bool, len(family.Sources))
	for _, source := range family.Sources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(source.Path)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", source.Path, err)
		}
		if hash != source.SHA256 {
			t.Fatalf("provenance source %s records %s, disk has %s", source.Path, source.SHA256, hash)
		}
		paths[source.Path] = true
	}
	for _, required := range []string{
		"packages/shared/network/protocol/message_player.go",
		"packages/shared/network/protocol/message_hostile.go",
		"packages/shared/network/protocol/message_passive.go",
	} {
		if !paths[required] {
			t.Fatalf("%s provenance drops %s", domainEventFamily, required)
		}
	}

	familyTopLevel := make([]string, 0)
	for _, c := range merged.Cases {
		if c.Family == domainEventFamily {
			familyTopLevel = append(familyTopLevel, c.ID)
		}
	}
	sort.Strings(familyTopLevel)
	if len(familyTopLevel) != len(family.Cases) {
		t.Fatalf("%s lists %d cases, top-level carries %d", domainEventFamily, len(family.Cases), len(familyTopLevel))
	}
	for index := range familyTopLevel {
		if familyTopLevel[index] != family.Cases[index] {
			t.Fatalf("%s case list diverges from the top-level union at %d", domainEventFamily, index)
		}
	}
}

// TestDomainEventManifestMergePreservesUnrelatedFamilies pins that the merge
// touches nothing but the shared family registration: every other family row
// and every non-`domain.event` case is byte-semantically unchanged, and the
// schema, identities and source revision keep the base values.
func TestDomainEventManifestMergePreservesUnrelatedFamilies(t *testing.T) {
	root := mustRepoRoot(t)
	base, merged := domainEventMergedForTest(t, root)

	if merged.SchemaVersion != base.SchemaVersion {
		t.Fatalf("merged schema_version = %d, want %d", merged.SchemaVersion, base.SchemaVersion)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}
	if !reflect.DeepEqual(merged.Identities, base.Identities) {
		t.Fatal("merged manifest rewrites the identity matrix")
	}
	mergedFamilies := make(map[string]Family, len(merged.Families))
	for _, family := range merged.Families {
		mergedFamilies[family.ID] = family
	}
	for _, family := range base.Families {
		if family.ID == domainEventFamily {
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
	nonEvent := 0
	for _, c := range base.Cases {
		if c.Family == domainEventFamily {
			continue
		}
		nonEvent++
		if got, ok := mergedByID[c.ID]; !ok || !reflect.DeepEqual(got, c) {
			t.Fatalf("merged manifest rewrites non-event case %s", c.ID)
		}
	}
	mergedNonEvent := 0
	for _, c := range merged.Cases {
		if c.Family != domainEventFamily {
			mergedNonEvent++
		}
	}
	if mergedNonEvent != nonEvent {
		t.Fatalf("merged manifest carries %d non-event cases, base carries %d", mergedNonEvent, nonEvent)
	}
}

// TestDomainEventManifestCandidateRejectsRepositoryAndSymlinkTargets pins
// the publication boundary of the candidate writer: a repository-contained
// export root and a root reached through a symlinked ancestor are both
// refused with no candidate and no filesystem effect.
func TestDomainEventManifestCandidateRejectsRepositoryAndSymlinkTargets(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)
	_, merged := domainEventMergedForTest(t, root)

	contained := filepath.Join(root, "manifest-export-probe")
	if got := writeDomainEventManifestCandidate(t, root, contained, merged); got != "" {
		t.Fatalf("repository-contained root published %s", got)
	}
	if _, statErr := os.Lstat(contained); !os.IsNotExist(statErr) {
		t.Fatalf("export directory was created inside repository: %v", statErr)
	}

	parent := t.TempDir()
	realDir := filepath.Join(parent, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkDir := filepath.Join(parent, "symlink-ancestor")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Fatal(err)
	}
	if got := writeDomainEventManifestCandidate(t, root, filepath.Join(symlinkDir, "export"), merged); got != "" {
		t.Fatalf("symlinked ancestor published %s", got)
	}
	entries, readErr := os.ReadDir(realDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("candidate export wrote through the symlinked ancestor: %v", entries)
	}
}

// TestDomainEventManifestCandidateReloadsAndReconciles pins the happy
// publication path: a fresh external root receives exactly
// `runtime-oracle/domain-event-manifest/contracts.json`, the written bytes
// are the encoded merged inventory, and the reloaded candidate validates
// through the production reconciliation.
func TestDomainEventManifestCandidateReloadsAndReconciles(t *testing.T) {
	root := mustRepoRoot(t)
	_, merged := domainEventMergedForTest(t, root)

	exportRoot := t.TempDir()
	want := filepath.Join(exportRoot, "runtime-oracle", "domain-event-manifest", "contracts.json")
	got := writeDomainEventManifestCandidate(t, root, exportRoot, merged)
	if got != want {
		t.Fatalf("candidate path = %s, want %s", got, want)
	}
	reloaded, err := LoadInventory(got)
	if err != nil {
		t.Fatalf("reload candidate: %v", err)
	}
	families, live, err := Discover(root)
	if err != nil {
		t.Fatalf("discover registries: %v", err)
	}
	if _, err := ReconcileWorking(root, reloaded, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions()); err != nil {
		t.Fatalf("candidate does not reconcile: %v", err)
	}
	encoded, err := encodeInventory(merged)
	if err != nil {
		t.Fatalf("encode merged manifest: %v", err)
	}
	written, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read candidate: %v", err)
	}
	if !bytes.Equal(written, append(encoded, '\n')) {
		t.Fatal("candidate bytes are not the encoded merged inventory")
	}
}
