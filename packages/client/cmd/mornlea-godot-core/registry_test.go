package main

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The registry tests pin the sole client-core feature-family registry: the
// seven pilot families with header-derived IDs, contract versions, and
// limits, plus the negotiation rules every consumer (the Rust bridge and
// later identity/feature exports) must agree on. The registry reports
// data-plane availability only; product assembly stays with the Godot-side
// feature catalog and is deliberately out of scope here.

// pinnedPilotDescriptors pins today's registry table with explicit literals.
// Registry construction derives every value from the cgo header constants, so
// the equalities below fail when any existing family ID, contract version, or
// limit changes in the header or in registry.go without a reviewed version
// bump, mirroring the identity pins in abi_test.go.
func pinnedPilotDescriptors() []RegistryDescriptor {
	return []RegistryDescriptor{
		{Family: 1, Version: 1, RecordLimit: 7, RecordBytes: 24},
		{Family: 2, Version: 1, RecordLimit: 256, RecordBytes: 0},
		{Family: 3, Version: 1, RecordLimit: 128, RecordBytes: 0},
		{Family: 4, Version: 1, RecordLimit: 1, RecordBytes: 24},
		{Family: 5, Version: 1, RecordLimit: 4096, RecordBytes: 0},
		{Family: 6, Version: 1, RecordLimit: 7, RecordBytes: 0},
		{Family: 7, Version: 1, RecordLimit: 64, RecordBytes: 0},
	}
}

// registryTestDescriptors rebuilds the pilot table from the cgo constants for
// mutation-based validation tests. Unlike the pinned literals, this table
// follows a reviewed header bump automatically, so duplicate, missing, and
// unknown-family coverage keeps exercising construction rules rather than
// exact pilot values.
func registryTestDescriptors() []RegistryDescriptor {
	return []RegistryDescriptor{
		{Family: FamilyIdentity, Version: IdentityVersion, RecordLimit: uint32(FamilyCount), RecordBytes: FamilyDescriptorBytes},
		{Family: FamilyConnection, Version: ConnectionVersion, RecordLimit: MaxConnectionAddressBytes, RecordBytes: 0},
		{Family: FamilyInput, Version: InputVersion, RecordLimit: MaxInputEvents, RecordBytes: 0},
		{Family: FamilyStep, Version: StepVersion, RecordLimit: 1, RecordBytes: StepRequestBytes},
		{Family: FamilyWorld, Version: WorldVersion, RecordLimit: MaxWorldBatchOperations, RecordBytes: 0},
		{Family: FamilyFrame, Version: FrameVersion, RecordLimit: MaxEntityRecords, RecordBytes: 0},
		{Family: FamilyStatus, Version: StatusVersion, RecordLimit: MaxStatusRecords, RecordBytes: 0},
	}
}

// TestFeatureFamilyRegistryPinsPilotDescriptorsToHeaderContract is the
// no-silent-layout-change gate: the sole registry must expose exactly the
// pinned pilot descriptor values in ascending family-ID order. A changed
// limit, version, or family ID in the header or in the registry construction
// fails here until the pin is consciously updated as part of a reviewed
// contract-version bump.
func TestFeatureFamilyRegistryPinsPilotDescriptorsToHeaderContract(t *testing.T) {
	registry := ClientCoreRegistry()
	got := make([]RegistryDescriptor, registry.Len())
	if copied := registry.Descriptors(got); copied != len(got) {
		t.Fatalf("enumerated %d descriptors into a buffer of %d", copied, len(got))
	}
	want := pinnedPilotDescriptors()
	if !slices.Equal(got, want) {
		t.Fatalf("registry descriptors = %+v, want pinned %+v", got, want)
	}
	if uint32(registry.Len()) != uint32(FamilyCount) {
		t.Fatalf("registry length = %d, want family count %d", registry.Len(), FamilyCount)
	}
}

// TestFeatureFamilyRegistryEnumeratesInFamilyIDOrder verifies enumeration is
// strictly ascending by family ID so descriptor order is part of the stable
// registry contract, not an artifact of construction order.
func TestFeatureFamilyRegistryEnumeratesInFamilyIDOrder(t *testing.T) {
	registry := ClientCoreRegistry()
	view := make([]RegistryDescriptor, registry.Len())
	if copied := registry.Descriptors(view); copied != len(view) {
		t.Fatalf("enumerated %d descriptors into a buffer of %d", copied, len(view))
	}
	for index, descriptor := range view {
		if want := uint32(index + 1); uint32(descriptor.Family) != want {
			t.Fatalf("descriptor %d has family %d, want ascending ID %d",
				index, descriptor.Family, want)
		}
		if index > 0 && descriptor.Family <= view[index-1].Family {
			t.Fatalf("descriptor %d family %d does not ascend past %d",
				index, descriptor.Family, view[index-1].Family)
		}
	}
}

// TestFeatureFamilyRegistryEnumerationFollowsCapacityProtocol verifies the
// descriptor enumeration rehearses the ABI two-phase capacity protocol: a
// zero-capacity call reports the required count without writing, and a short
// buffer writes nothing rather than a partial descriptor set.
func TestFeatureFamilyRegistryEnumerationFollowsCapacityProtocol(t *testing.T) {
	registry := ClientCoreRegistry()
	if reported := registry.Descriptors(nil); reported != registry.Len() {
		t.Fatalf("zero-capacity query reported %d, want %d", reported, registry.Len())
	}
	short := make([]RegistryDescriptor, registry.Len()-1)
	poison := RegistryDescriptor{Family: 99, Version: 99, RecordLimit: 99, RecordBytes: 99}
	for index := range short {
		short[index] = poison
	}
	if reported := registry.Descriptors(short); reported != registry.Len() {
		t.Fatalf("short buffer reported %d, want the full count %d", reported, registry.Len())
	}
	for index := range short {
		if short[index] != poison {
			t.Fatalf("short buffer wrote descriptor %d despite insufficient capacity", index)
		}
	}
	full := make([]RegistryDescriptor, registry.Len()+2)
	for index := range full {
		full[index] = poison
	}
	if copied := registry.Descriptors(full); copied != registry.Len() {
		t.Fatalf("full buffer copied %d descriptors, want %d", copied, registry.Len())
	}
	for _, index := range []int{registry.Len(), registry.Len() + 1} {
		if full[index] != poison {
			t.Fatalf("enumeration wrote past the reported count at index %d", index)
		}
	}
}

// TestFeatureFamilyRegistryLookupFindsEachPilotFamily covers single-family
// lookup for every pilot family and the rejection of unregistered IDs.
func TestFeatureFamilyRegistryLookupFindsEachPilotFamily(t *testing.T) {
	registry := ClientCoreRegistry()
	for _, want := range pinnedPilotDescriptors() {
		descriptor, ok := registry.Descriptor(want.Family)
		if !ok {
			t.Fatalf("lookup of family %d failed", want.Family)
		}
		if descriptor != want {
			t.Fatalf("family %d descriptor = %+v, want %+v", want.Family, descriptor, want)
		}
	}
	for _, unknown := range []Family{0, FamilyCount + 1, 99} {
		if _, ok := registry.Descriptor(unknown); ok {
			t.Fatalf("lookup of unknown family %d succeeded", unknown)
		}
	}
}

// TestFeatureFamilyRegistryRejectsUnknownFamilyIDs proves construction
// rejects descriptor lists containing a family outside the header's pilot
// identifier range, in both the zero and the appended-past-count direction.
func TestFeatureFamilyRegistryRejectsUnknownFamilyIDs(t *testing.T) {
	for _, unknown := range []Family{0, FamilyCount + 1} {
		input := registryTestDescriptors()
		input[0].Family = unknown
		_, err := NewRegistry(input)
		if err == nil {
			t.Fatalf("registry with unknown family %d was accepted", unknown)
		}
		if !strings.Contains(err.Error(), strconv.FormatUint(uint64(unknown), 10)) {
			t.Fatalf("unknown-family error %q does not name family %d", err, unknown)
		}
	}
}

// TestFeatureFamilyRegistryRejectsDuplicateFamilyIDs proves the constructor
// rejects a duplicate identifier through the injection seam of building a
// synthetic registry, without mutating the sole registry.
func TestFeatureFamilyRegistryRejectsDuplicateFamilyIDs(t *testing.T) {
	input := registryTestDescriptors()
	input = append(input, input[0])
	_, err := NewRegistry(input)
	if err == nil {
		t.Fatal("registry with a duplicate family ID was accepted")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error %q does not classify as duplicate", err)
	}
	if !strings.Contains(err.Error(), strconv.FormatUint(uint64(input[0].Family), 10)) {
		t.Fatalf("duplicate error %q does not name the duplicated family", err)
	}
}

// TestFeatureFamilyRegistryRejectsMissingRequiredFamilies proves every pilot
// family is required: dropping any one of the seven from an otherwise valid
// table fails construction and names the missing family.
func TestFeatureFamilyRegistryRejectsMissingRequiredFamilies(t *testing.T) {
	for dropped := range registryTestDescriptors() {
		input := registryTestDescriptors()
		missing := input[dropped].Family
		input = slices.Delete(input, dropped, dropped+1)
		_, err := NewRegistry(input)
		if err == nil {
			t.Fatalf("registry without family %d was accepted", missing)
		}
		if !strings.Contains(err.Error(), strconv.FormatUint(uint64(missing), 10)) {
			t.Fatalf("missing-family error %q does not name family %d", err, missing)
		}
	}
}

// TestFeatureFamilyRegistryRejectsZeroContractVersions proves a descriptor
// with contract version zero cannot be registered; family versions start at
// one and only rise, so zero is an invalid table, not the oldest release.
func TestFeatureFamilyRegistryRejectsZeroContractVersions(t *testing.T) {
	input := registryTestDescriptors()
	input[0].Version = 0
	_, err := NewRegistry(input)
	if err == nil {
		t.Fatal("registry with a zero contract version was accepted")
	}
	if !strings.Contains(err.Error(), strconv.FormatUint(uint64(input[0].Family), 10)) {
		t.Fatalf("zero-version error %q does not name family %d", err, input[0].Family)
	}
}

// TestFeatureFamilyRegistryConstructionIsDeterministicAndSideEffectFree
// proves construction is a pure function: input order does not change the
// enumerated table, the input slice is neither retained nor mutated, and two
// constructions of the sole registry are equal.
func TestFeatureFamilyRegistryConstructionIsDeterministicAndSideEffectFree(t *testing.T) {
	forward := registryTestDescriptors()
	reverse := slices.Clone(forward)
	slices.Reverse(reverse)
	ordered, err := NewRegistry(forward)
	if err != nil {
		t.Fatalf("forward construction failed: %v", err)
	}
	shuffled, err := NewRegistry(reverse)
	if err != nil {
		t.Fatalf("reversed construction failed: %v", err)
	}
	orderedView := make([]RegistryDescriptor, ordered.Len())
	shuffledView := make([]RegistryDescriptor, shuffled.Len())
	ordered.Descriptors(orderedView)
	shuffled.Descriptors(shuffledView)
	if !slices.Equal(orderedView, shuffledView) {
		t.Fatalf("construction order changed the table: %+v vs %+v", orderedView, shuffledView)
	}

	mutated := registryTestDescriptors()
	registry, err := NewRegistry(mutated)
	if err != nil {
		t.Fatalf("construction failed: %v", err)
	}
	mutated[0].Version = 99
	mutated[1].RecordLimit = 99
	view := make([]RegistryDescriptor, registry.Len())
	registry.Descriptors(view)
	if !slices.Equal(view, pinnedPilotDescriptors()) {
		t.Fatalf("post-construction input mutation leaked into the registry: %+v", view)
	}

	first := ClientCoreRegistry()
	second := ClientCoreRegistry()
	firstView := make([]RegistryDescriptor, first.Len())
	secondView := make([]RegistryDescriptor, second.Len())
	first.Descriptors(firstView)
	second.Descriptors(secondView)
	if !slices.Equal(firstView, secondView) {
		t.Fatalf("two constructions of the sole registry differ: %+v vs %+v", firstView, secondView)
	}
}

// TestFeatureCompatibilityNegotiationAcceptsCompatibleMinor proves a consumer
// request is accepted when the ABI major matches and the requested minor is
// equal to or older than the registered family contract version; equal or
// older requests stay compatible because family versions only rise
// additively.
func TestFeatureCompatibilityNegotiationAcceptsCompatibleMinor(t *testing.T) {
	registry := ClientCoreRegistry()
	for _, descriptor := range pinnedPilotDescriptors() {
		requests := []uint32{descriptor.Version}
		if descriptor.Version > 1 {
			requests = append(requests, descriptor.Version-1)
		}
		requests = append(requests, 0)
		for _, minor := range requests {
			decision := registry.Negotiate(descriptor.Family, ABIMajor, minor)
			if !decision.Accepted {
				t.Fatalf("family %d request (major %d, minor %d) rejected: %s",
					descriptor.Family, ABIMajor, minor, decision.Reason)
			}
			if decision.Reason != NegotiationCompatible {
				t.Fatalf("accepted request for family %d reported reason %s",
					descriptor.Family, decision.Reason)
			}
		}
	}
}

// TestFeatureCompatibilityNegotiationRejectsIncompatibleMajor proves any ABI
// major mismatch is rejected in both directions, because the major only moves
// for incompatible layout or semantic changes that this producer cannot serve.
func TestFeatureCompatibilityNegotiationRejectsIncompatibleMajor(t *testing.T) {
	registry := ClientCoreRegistry()
	for _, descriptor := range pinnedPilotDescriptors() {
		for _, major := range []uint32{ABIMajor + 1, ABIMajor - 1} {
			decision := registry.Negotiate(descriptor.Family, major, descriptor.Version)
			if decision.Accepted {
				t.Fatalf("family %d request with major %d accepted", descriptor.Family, major)
			}
			if decision.Reason != NegotiationMajorMismatch {
				t.Fatalf("family %d major mismatch reported reason %s",
					descriptor.Family, decision.Reason)
			}
		}
	}
}

// TestFeatureCompatibilityNegotiationRejectsUnknownFamily proves negotiation
// rejects unregistered family identifiers with the unknown-family reason
// rather than accepting them as unbounded capability.
func TestFeatureCompatibilityNegotiationRejectsUnknownFamily(t *testing.T) {
	registry := ClientCoreRegistry()
	for _, unknown := range []Family{0, FamilyCount + 1, 99} {
		decision := registry.Negotiate(unknown, ABIMajor, 1)
		if decision.Accepted {
			t.Fatalf("unknown family %d accepted", unknown)
		}
		if decision.Reason != NegotiationUnknownFamily {
			t.Fatalf("unknown family %d reported reason %s", unknown, decision.Reason)
		}
	}
}

// TestFeatureCompatibilityNegotiationRejectsNewerMinor proves a request for a
// newer family contract version than the registry provides is rejected: the
// producer cannot serve records the consumer expects.
func TestFeatureCompatibilityNegotiationRejectsNewerMinor(t *testing.T) {
	registry := ClientCoreRegistry()
	for _, descriptor := range pinnedPilotDescriptors() {
		decision := registry.Negotiate(descriptor.Family, ABIMajor, descriptor.Version+1)
		if decision.Accepted {
			t.Fatalf("family %d newer minor %d accepted", descriptor.Family, descriptor.Version+1)
		}
		if decision.Reason != NegotiationMinorTooNew {
			t.Fatalf("family %d newer minor reported reason %s",
				descriptor.Family, decision.Reason)
		}
	}
}
