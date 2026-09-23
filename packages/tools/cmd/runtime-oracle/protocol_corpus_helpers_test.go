package main

import (
	"fmt"
	"strings"
	"testing"
)

// This file is the protocol-only corpus runner. Packet families register one
// executable route per `{family, version, operation}` triple, so a family that
// publishes both a decode and an encode case needs a route lookup the old
// single-operation-per-family map cannot express. The runner below takes that
// closed route map, and the frame producer supplies the first pair of routes.
//
// The evidence boundary is unchanged from `RunCases`: a producer receives only
// the case specification and the case input bytes, the recorded expectation is
// never read, and every observation is derived from what the producer returned.

// routeLookup resolves one case's family, version and operation to the producer
// that executes it. A lookup returns an error when the case names a route this
// run cannot execute, so an unrouted case fails the run instead of being
// silently dropped.
type routeLookup func(family, version, operation string) (GoOperation, error)

// RunProtocolCases executes a protocol corpus selection through its registered
// routes and returns one executed observation per declared checkpoint.
//
// A route map is closed: a case whose `{family, version, operation}` triple
// names no registered route fails before its input is read, so a case can never
// pass by declaring an operation no producer implements. The runner shares the
// loop, the input budget, the digest proof and the observation builder with
// `RunCases`, so both runners publish evidence under the same rules.
func RunProtocolCases(root string, selection Inventory, routes map[ConsumerRoute]GoOperation) ([]ExecutedObservation, error) {
	if len(selection.Cases) == 0 {
		return nil, fmt.Errorf("runtime-oracle: manifest selection is empty")
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("runtime-oracle: no registered protocol routes")
	}
	return runCasesByRoute(root, selection, func(family, version, operation string) (GoOperation, error) {
		route := ConsumerRoute{FamilyID: family, Version: version, Operation: operation}
		producer, registered := routes[route]
		if !registered {
			return nil, fmt.Errorf("route %s/%s/%s has no registered Go producer", family, version, operation)
		}
		return producer, nil
	})
}

// TestProtocolCorpusRoutesExecuteTwoOperationsForOneFamily pins that a family
// publishing a decode case and an encode case executes both through one run.
//
// The baseline `RunCases` binds every family to a single operation, so the
// framing encode case could never be executed beside the two decode cases: one
// of the two operations was always rejected as a family/operation mismatch. The
// route-keyed runner executes both operations from one selection.
func TestProtocolCorpusRoutesExecuteTwoOperationsForOneFamily(t *testing.T) {
	root := mustRepoRoot(t)
	candidate := frameProtocolCandidate(t, root)
	manifest := frameProtocolManifest(t, root, candidate.Spec)
	staged := frameProtocolScratchRoot(t, root, candidate)

	routes := frameCorpusRoutes()
	observations, err := RunProtocolCases(staged, manifest, routes)
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != 3 {
		t.Fatalf("produced %d observations, want 3 (one per case)", len(observations))
	}

	// The two decode cases must reproduce the outcomes the frozen corpus
	// records, so the new route does not change what the decode evidence
	// publishes.
	for _, id := range []string{frameValidCaseID, frameNoncanonicalCaseID} {
		c := frameCaseByID(t, manifest, id)
		expected := readExpectedOutcome(t, root, c)
		obs := frameObservation(t, observations, id)
		if !outcomesEqual(obs.Outcome, expected) {
			t.Fatalf("case %s produced %#v, want %#v", id, obs.Outcome, expected)
		}
	}

	encode := frameObservation(t, observations, frameEncodeCaseID)
	if !outcomesEqual(encode.Outcome, candidate.Expect) {
		t.Fatalf("encode case produced %#v, want %#v", encode.Outcome, candidate.Expect)
	}
}

// TestProtocolCorpusRunnerRejectsUnregisteredRoute pins that a case naming a
// route the candidate does not register fails before its producer runs, so a
// case cannot claim coverage from its name alone.
func TestProtocolCorpusRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidate := frameProtocolCandidate(t, root)
	manifest := frameProtocolManifest(t, root, candidate.Spec)
	staged := frameProtocolScratchRoot(t, root, candidate)

	decodeOnly := map[ConsumerRoute]GoOperation{
		{FamilyID: frameFamily, Version: frameVersion, Operation: "decode"}: runFrameDecode,
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	wantRoute := frameFamily + "/" + frameVersion + "/encode"
	if !strings.Contains(err.Error(), wantRoute) {
		t.Fatalf("rejection %v does not name route %s", err, wantRoute)
	}
}

// TestProtocolCorpusRunnerRejectsEmptySelection pins that a selection carrying
// no case is refused rather than reported as a passing run.
func TestProtocolCorpusRunnerRejectsEmptySelection(t *testing.T) {
	root := mustRepoRoot(t)
	candidate := frameProtocolCandidate(t, root)
	manifest := frameProtocolManifest(t, root, candidate.Spec)
	manifest.Cases = nil

	if _, err := RunProtocolCases(frameProtocolScratchRoot(t, root, candidate), manifest, frameCorpusRoutes()); err == nil ||
		!strings.Contains(err.Error(), "selection is empty") {
		t.Fatalf("expected an empty-selection rejection, got %v", err)
	}
}

// TestProtocolCorpusRunnerRejectsMissingRoutes pins that a run with no route
// map at all is refused.
func TestProtocolCorpusRunnerRejectsMissingRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	candidate := frameProtocolCandidate(t, root)
	manifest := frameProtocolManifest(t, root, candidate.Spec)

	if _, err := RunProtocolCases(frameProtocolScratchRoot(t, root, candidate), manifest, nil); err == nil ||
		!strings.Contains(err.Error(), "no registered protocol routes") {
		t.Fatalf("expected a missing-routes rejection, got %v", err)
	}
}

// TestProtocolCorpusRunnerRejectsDuplicateCheckpoints pins that one case
// declares each checkpoint once, so a duplicated tick cannot inflate the
// executed evidence count for a single observation.
func TestProtocolCorpusRunnerRejectsDuplicateCheckpoints(t *testing.T) {
	root := mustRepoRoot(t)
	candidate := frameProtocolCandidate(t, root)
	manifest := frameProtocolManifest(t, root, candidate.Spec)
	for index := range manifest.Cases {
		if manifest.Cases[index].ID == frameValidCaseID {
			manifest.Cases[index].Checkpoints = []string{"0", "0"}
		}
	}

	_, err := RunProtocolCases(frameProtocolScratchRoot(t, root, candidate), manifest, frameCorpusRoutes())
	if err == nil || !strings.Contains(err.Error(), "duplicate checkpoint") {
		t.Fatalf("expected a duplicate-checkpoint rejection, got %v", err)
	}
}
