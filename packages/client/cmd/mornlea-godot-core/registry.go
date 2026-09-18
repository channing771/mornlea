//go:build cgo

package main

import (
	"cmp"
	"fmt"
	"slices"
)

// The sole client-core feature-family registry. It reports data-plane
// availability only: which pilot families exist, their contract versions,
// and their bounded limits. Product assembly (profiles, catalogs, feature
// manifests) is a separate Godot-side concern and deliberately never reads
// or extends this registry. Every descriptor value is derived from the cgo
// header constants in abi.go, so the registry cannot drift from
// include/mornlea_client_core.h; registry_test.go pins today's table to
// explicit literals so changing any existing ID, version, or limit without a
// reviewed contract-version bump fails the gate.

// RegistryDescriptor is the Go view of one feature-family descriptor record.
// It carries the same identity fields as the family descriptor record
// declared in include/mornlea_client_core.h, without the reserved padding.
type RegistryDescriptor struct {
	Family      Family
	Version     uint32
	RecordLimit uint32
	RecordBytes uint32
}

// NegotiationReason classifies one feature-family negotiation outcome. These
// values are internal diagnostics for the registry and its tests, not stable
// wire codes; the ABI wire surface reports failures through the `Status`
// codes (for example `StatusABIMismatch`).
type NegotiationReason int

const (
	NegotiationCompatible    NegotiationReason = iota // requested version equals or is older than the registered family contract
	NegotiationUnknownFamily                          // requested family identifier is not registered
	NegotiationMajorMismatch                          // requested ABI major differs from the producer major
	NegotiationMinorTooNew                            // requested minor is newer than the registered contract version
)

// String returns a stable human-readable reason for diagnostics.
func (reason NegotiationReason) String() string {
	switch reason {
	case NegotiationCompatible:
		return "compatible"
	case NegotiationUnknownFamily:
		return "unknown family"
	case NegotiationMajorMismatch:
		return "ABI major mismatch"
	case NegotiationMinorTooNew:
		return "requested minor is newer than the registered contract version"
	default:
		return "unknown negotiation reason"
	}
}

// NegotiationDecision is the outcome of one feature-family version check.
// `Accepted` is true only for `NegotiationCompatible`.
type NegotiationDecision struct {
	Accepted bool
	Reason   NegotiationReason
}

// Registry is an immutable feature-family table in ascending family-ID
// order. Construction is deterministic and side-effect free: no clocks, no
// I/O, and the caller's input slice is copied, never retained or mutated. A
// `Registry` and its slices are immutable after construction, so any later
// export may publish descriptors across goroutines without copying.
type Registry struct {
	descriptors []RegistryDescriptor
}

// pilotFamilyIDs lists every family identifier the pilot contract defines,
// in ascending order; new families append, existing identifiers are never
// reused or reordered.
func pilotFamilyIDs() []Family {
	return []Family{
		FamilyIdentity,
		FamilyConnection,
		FamilyInput,
		FamilyStep,
		FamilyWorld,
		FamilyFrame,
		FamilyStatus,
		FamilyEnvironment,
	}
}

// NewRegistry validates a descriptor list and returns the registry it
// describes. Every descriptor must name a known pilot family with a nonzero
// contract version, identifiers must be unique, and all eight families are
// required by the pilot data plane, so a missing required family fails here
// before any consumer creates world state. Unknown identifiers are rejected
// before the presence check so a table that both smuggles an unknown family
// and drops a required one reports the smuggled family first. The returned
// table is normalized to ascending family-ID order regardless of input
// order. The shared cross-language contract is the accept/reject predicate;
// the precedence of rejection reasons is a per-language diagnostic detail and
// may legitimately differ from the Rust mirror's ordering.
func NewRegistry(descriptors []RegistryDescriptor) (Registry, error) {
	table := slices.Clone(descriptors)
	seen := make(map[Family]bool, len(pilotFamilyIDs()))
	for _, descriptor := range table {
		if !slices.Contains(pilotFamilyIDs(), descriptor.Family) {
			return Registry{}, fmt.Errorf("registry cannot register unknown family %d", descriptor.Family)
		}
		if descriptor.Version == 0 {
			return Registry{}, fmt.Errorf("registry family %d has zero contract version", descriptor.Family)
		}
		if seen[descriptor.Family] {
			return Registry{}, fmt.Errorf("registry cannot register duplicate family %d", descriptor.Family)
		}
		seen[descriptor.Family] = true
	}
	for _, family := range pilotFamilyIDs() {
		if !seen[family] {
			return Registry{}, fmt.Errorf("registry is missing required family %d", family)
		}
	}
	slices.SortFunc(table, func(a, b RegistryDescriptor) int {
		return cmp.Compare(a.Family, b.Family)
	})
	return Registry{descriptors: table}, nil
}

// MustNewRegistry builds a registry from a compile-time-constant descriptor
// table. It panics only when that table violates `NewRegistry` validation;
// the panic marks a build-time invariant break caught by the registry tests,
// never a runtime input path.
func MustNewRegistry(descriptors []RegistryDescriptor) Registry {
	registry, err := NewRegistry(descriptors)
	if err != nil {
		panic(fmt.Sprintf("constant registry table is invalid: %v", err))
	}
	return registry
}

// ClientCoreRegistry returns the sole pilot registry. All descriptor values
// come from the cgo header constants, and rebuilding on each call keeps the
// registry side-effect free and deterministic.
func ClientCoreRegistry() Registry {
	return MustNewRegistry(clientCoreFamilyTable())
}

// clientCoreFamilyTable derives the pilot descriptor table from the header
// constants. `RecordLimit` names each family's bounded batch capacity (for
// byte-payload families, the byte bound) and `RecordBytes` the fixed
// per-record wire size, zero marking variable-length records whose layout
// lands with that family's later export.
func clientCoreFamilyTable() []RegistryDescriptor {
	return []RegistryDescriptor{
		// Identity output is 1..`FamilyCount` fixed-size descriptor records.
		{Family: FamilyIdentity, Version: IdentityVersion, RecordLimit: uint32(FamilyCount), RecordBytes: FamilyDescriptorBytes},
		// One variable-length UTF-8 address bounded by its byte limit.
		{Family: FamilyConnection, Version: ConnectionVersion, RecordLimit: MaxConnectionAddressBytes},
		// Bounded event batch; the fixed event record layout arrives with the
		// input export, so record bytes stay variable in this generation.
		{Family: FamilyInput, Version: InputVersion, RecordLimit: MaxInputEvents},
		// Exactly one fixed request record per step, not a batch.
		{Family: FamilyStep, Version: StepVersion, RecordLimit: 1, RecordBytes: StepRequestBytes},
		// Bounded upsert/drop operation batch with variable packed-quad payload.
		{Family: FamilyWorld, Version: WorldVersion, RecordLimit: MaxWorldBatchOperations},
		// Bounded entity records plus the variable target-name payload.
		{Family: FamilyFrame, Version: FrameVersion, RecordLimit: MaxEntityRecords},
		// Bounded status pull; the fixed record layout now lives in status.go,
		// and the descriptor keeps zero `RecordBytes` this generation because
		// both language pin suites pin this table (see status.go).
		{Family: FamilyStatus, Version: StatusVersion, RecordLimit: MaxStatusRecords},
		{Family: FamilyEnvironment, Version: EnvironmentVersion, RecordLimit: 1, RecordBytes: EnvironmentBytes},
	}
}

// Len reports the number of registered families.
func (r Registry) Len() int {
	return len(r.descriptors)
}

// Descriptors copies every descriptor in ascending family-ID order into dst
// and returns the total descriptor count, rehearsing the ABI two-phase
// capacity protocol: a zero-capacity or too-small dst is never written; the
// caller reallocates to the returned count and calls again.
func (r Registry) Descriptors(dst []RegistryDescriptor) int {
	if len(dst) < len(r.descriptors) {
		return len(r.descriptors)
	}
	copy(dst, r.descriptors)
	return len(r.descriptors)
}

// Descriptor returns the registered descriptor of one family, reporting
// false for any unregistered identifier. The scan is linear over at most
// `FamilyCount` entries, so lookups stay bounded on hot paths.
func (r Registry) Descriptor(family Family) (RegistryDescriptor, bool) {
	for _, descriptor := range r.descriptors {
		if descriptor.Family == family {
			return descriptor, true
		}
	}
	return RegistryDescriptor{}, false
}

// Negotiate decides whether a consumer may use `family` at the requested
// `(major, minor)`. The registry major is `ABIMajor`, the producer-wide
// compatibility key that moves only for incompatible layout or semantic
// changes; the registry minor of a family is that family's contract version,
// which only rises compatibly. A request is accepted exactly when the family
// is registered, the major matches, and the requested minor is equal to or
// older than the registered version — a consumer needing a newer minor than
// the producer provides is rejected rather than partially served.
func (r Registry) Negotiate(family Family, major, minor uint32) NegotiationDecision {
	descriptor, ok := r.Descriptor(family)
	if !ok {
		return NegotiationDecision{Accepted: false, Reason: NegotiationUnknownFamily}
	}
	if major != ABIMajor {
		return NegotiationDecision{Accepted: false, Reason: NegotiationMajorMismatch}
	}
	if minor > descriptor.Version {
		return NegotiationDecision{Accepted: false, Reason: NegotiationMinorTooNew}
	}
	return NegotiationDecision{Accepted: true, Reason: NegotiationCompatible}
}
