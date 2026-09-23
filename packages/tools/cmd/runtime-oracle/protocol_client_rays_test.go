package main

// This file is the client ray action packet producer group: the five Play
// client-to-server ray families (`OpenContainer`, `TillSoil`, `BoneMeal`,
// `CollectWater` and `PlaceWater`) each register a decode and an encode route
// through the shared packet-case runner, so every case is executed by the real
// Go codec rather than restated here.
//
// The decode cases are the Go decoder's own bytes: the valid payload, a
// mutated payload at one angle boundary each. The encode cases carry canonical
// JSON fields and the reviewed wire the Go encoder has to publish, or a DTO the
// Go outbound validator has to refuse. The input values themselves move no
// world state: the ray-cast target, the held item, the container kind and the
// resulting world write stay authority-owned, so no case in this group declares
// a session, a deadline or a send.

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	// openContainerFamily is the play open container family this group registers.
	openContainerFamily = "protocol.client.OpenContainer"
	// tillSoilFamily is the play till soil family this group registers.
	tillSoilFamily = "protocol.client.TillSoil"
	// boneMealFamily is the play bone meal family this group registers.
	boneMealFamily = "protocol.client.BoneMeal"
	// collectWaterFamily is the play collect water family this group registers.
	collectWaterFamily = "protocol.client.CollectWater"
	// placeWaterFamily is the play place water family this group registers.
	placeWaterFamily = "protocol.client.PlaceWater"
	// clientRaysVersion is the protocol version every family is pinned to.
	clientRaysVersion = "45"
	// clientRaysProducerID is the exporter's producer identity for this group's
	// candidate assets and merged manifest.
	clientRaysProducerID = "runtime-oracle/protocol-client-rays"

	// openContainerCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	openContainerCorpusRelDir = corpusCasesRelDir + "/protocol/OpenContainer"
	// tillSoilCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	tillSoilCorpusRelDir = corpusCasesRelDir + "/protocol/TillSoil"
	// boneMealCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	boneMealCorpusRelDir = corpusCasesRelDir + "/protocol/BoneMeal"
	// collectWaterCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	collectWaterCorpusRelDir = corpusCasesRelDir + "/protocol/CollectWater"
	// placeWaterCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	placeWaterCorpusRelDir = corpusCasesRelDir + "/protocol/PlaceWater"
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	openContainerDecodeCaseID   = openContainerFamily + "/" + clientRaysVersion + "/decode-valid"
	openContainerEncodeCaseID   = openContainerFamily + "/" + clientRaysVersion + "/encode-valid"
	openContainerNaNDecodeID    = openContainerFamily + "/" + clientRaysVersion + "/decode-nan-yaw"
	openContainerInfPitchDecode = openContainerFamily + "/" + clientRaysVersion + "/decode-infinite-pitch"
	openContainerNaNEncodeID    = openContainerFamily + "/" + clientRaysVersion + "/encode-nan-yaw"
	tillSoilDecodeCaseID        = tillSoilFamily + "/" + clientRaysVersion + "/decode-valid"
	tillSoilEncodeCaseID        = tillSoilFamily + "/" + clientRaysVersion + "/encode-valid"
	tillSoilNaNDecodeID         = tillSoilFamily + "/" + clientRaysVersion + "/decode-nan-yaw"
	tillSoilInfPitchDecode      = tillSoilFamily + "/" + clientRaysVersion + "/decode-infinite-pitch"
	tillSoilNaNEncodeID         = tillSoilFamily + "/" + clientRaysVersion + "/encode-nan-yaw"
	boneMealDecodeCaseID        = boneMealFamily + "/" + clientRaysVersion + "/decode-valid"
	boneMealEncodeCaseID        = boneMealFamily + "/" + clientRaysVersion + "/encode-valid"
	boneMealNaNDecodeID         = boneMealFamily + "/" + clientRaysVersion + "/decode-nan-yaw"
	boneMealInfPitchDecode      = boneMealFamily + "/" + clientRaysVersion + "/decode-infinite-pitch"
	boneMealNaNEncodeID         = boneMealFamily + "/" + clientRaysVersion + "/encode-nan-yaw"
	collectWaterDecodeCaseID    = collectWaterFamily + "/" + clientRaysVersion + "/decode-valid"
	collectWaterEncodeCaseID    = collectWaterFamily + "/" + clientRaysVersion + "/encode-valid"
	collectWaterNaNDecodeID     = collectWaterFamily + "/" + clientRaysVersion + "/decode-nan-yaw"
	collectWaterInfPitchDecode  = collectWaterFamily + "/" + clientRaysVersion + "/decode-infinite-pitch"
	collectWaterNaNEncodeID     = collectWaterFamily + "/" + clientRaysVersion + "/encode-nan-yaw"
	placeWaterDecodeCaseID      = placeWaterFamily + "/" + clientRaysVersion + "/decode-valid"
	placeWaterEncodeCaseID      = placeWaterFamily + "/" + clientRaysVersion + "/encode-valid"
	placeWaterNaNDecodeID       = placeWaterFamily + "/" + clientRaysVersion + "/decode-nan-yaw"
	placeWaterInfPitchDecode    = placeWaterFamily + "/" + clientRaysVersion + "/decode-infinite-pitch"
	placeWaterNaNEncodeID       = placeWaterFamily + "/" + clientRaysVersion + "/encode-nan-yaw"
)

// clientRayFamilyKeys pins each family's complete packet key. A case names its
// key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs.
var clientRayFamilyKeys = map[string]PacketKeySpec{
	openContainerFamily: {Direction: packetDirectionClient, State: packetStatePlay, ID: 8},
	tillSoilFamily:      {Direction: packetDirectionClient, State: packetStatePlay, ID: 13},
	boneMealFamily:      {Direction: packetDirectionClient, State: packetStatePlay, ID: 14},
	collectWaterFamily:  {Direction: packetDirectionClient, State: packetStatePlay, ID: 16},
	placeWaterFamily:    {Direction: packetDirectionClient, State: packetStatePlay, ID: 17},
}

// clientRayFamilies lists the five families in registry order, with the corpus
// directory that holds each family's assets. The order is the review order, not
// a dispatch table.
func clientRayFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{openContainerFamily, openContainerCorpusRelDir},
		{tillSoilFamily, tillSoilCorpusRelDir},
		{boneMealFamily, boneMealCorpusRelDir},
		{collectWaterFamily, collectWaterCorpusRelDir},
		{placeWaterFamily, placeWaterCorpusRelDir},
	}
}

// clientRaysWire is the reviewed wire literal every ray family's canonical
// vector pins: the Go encoder's own output for sequence 0, the -0.0 yaw bit
// pattern and the 1.5 pitch bit pattern. The five families share the payload
// shape, so the packet ID is the only thing that tells them apart on the wire:
// no target position, held item, container kind or result rides along.
var clientRaysWire = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x80,
	0x00, 0x00, 0xc0, 0x3f,
}

// clientRayRejectionCategory resolves the language-neutral rejection category
// for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and the
// outbound validators name, never over a sentinel identity, and a failure with
// no mapping is a hard error. The primitive answer precedes the validator
// message, because the decoder rejects a non-finite float while reading and
// only then hands the record to `Validate`.
func clientRayRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "invalid float32"),
		strings.Contains(message, "non-finite rotation"):
		return "invalid-value", true
	}
	return "", false
}

// clientRayPacketKey resolves one packet key to the Go state and numeric ID the
// codec dispatches on, and reports whether the packet travels client-to-server.
func clientRayPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := clientRayFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no client ray producer owns", c.ID, c.Family)
	}
	if *c.PacketKey != registered {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names key %+v, want %+v", c.ID, *c.PacketKey, registered)
	}
	if registered.State != packetStatePlay {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names state %q, want play", c.ID, registered.State)
	}
	if registered.Direction != packetDirectionClient {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names direction %q, want client-to-server", c.ID, registered.Direction)
	}
	return protocol.StatePlay, registered.ID, nil
}

// clientRayFields renders the semantic fields one decoded client ray packet
// publishes.
//
// The fields come from the DTO the production decoder returned, never from the
// case input. The sequence renders as a decimal string so the full u64 range
// stays lossless, and the two look angles render as their eight-digit
// hexadecimal bit strings so a negative zero survives the round trip: the
// canonical field encoding is the Rust consumer's too.
func clientRayFields(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.OpenContainer:
		return clientRayRotationFields(message.Sequence, message.Yaw, message.Pitch), nil
	case protocol.TillSoil:
		return clientRayRotationFields(message.Sequence, message.Yaw, message.Pitch), nil
	case protocol.BoneMeal:
		return clientRayRotationFields(message.Sequence, message.Yaw, message.Pitch), nil
	case protocol.CollectWater:
		return clientRayRotationFields(message.Sequence, message.Yaw, message.Pitch), nil
	case protocol.PlaceWater:
		return clientRayRotationFields(message.Sequence, message.Yaw, message.Pitch), nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// clientRayRotationFields renders the shared sequence-and-angles fields of one
// ray DTO.
func clientRayRotationFields(sequence uint64, yaw, pitch float32) map[string]any {
	return map[string]any{
		"sequence": strconv.FormatUint(sequence, 10),
		"yaw":      float32BitsHex(yaw),
		"pitch":    float32BitsHex(pitch),
	}
}

// newClientRayCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newClientRayCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// runClientRayDecode executes one client ray decode case through the real Go
// decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeClient` and classifies
// the failure it returns. It never reimplements the length-prefix, float or
// trailing-byte rules, so the recorded outcome is whatever the production codec
// decides about these exact bytes.
func runClientRayDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := clientRayPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newClientRayCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeClient(state, packetID, input)
	if err != nil {
		category, classified := clientRayRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := clientRayFields(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// clientRayEncodeRequest is the canonical JSON field input one ray encode case
// carries. The look angles are bit strings, and the sequence is a decimal JSON
// integer, which the full u64 range survives.
type clientRayEncodeRequest struct {
	Sequence uint64 `json:"sequence"`
	Yaw      string `json:"yaw"`
	Pitch    string `json:"pitch"`
}

// runClientRayEncode executes one client ray encode case through the real Go
// encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects must
// never be recorded as evidence.
func runClientRayEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := clientRayPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newClientRayCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var request clientRayEncodeRequest
	if err := decodeEncodeRequest(c, input, &request); err != nil {
		return Outcome{}, nil, err
	}
	yaw, err := float32FromBitsHex(c, request.Yaw)
	if err != nil {
		return Outcome{}, nil, err
	}
	pitch, err := float32FromBitsHex(c, request.Pitch)
	if err != nil {
		return Outcome{}, nil, err
	}
	var packet protocol.ClientPacket
	switch c.Family {
	case openContainerFamily:
		packet = protocol.OpenContainer{Sequence: request.Sequence, Yaw: yaw, Pitch: pitch}
	case tillSoilFamily:
		packet = protocol.TillSoil{Sequence: request.Sequence, Yaw: yaw, Pitch: pitch}
	case boneMealFamily:
		packet = protocol.BoneMeal{Sequence: request.Sequence, Yaw: yaw, Pitch: pitch}
	case collectWaterFamily:
		packet = protocol.CollectWater{Sequence: request.Sequence, Yaw: yaw, Pitch: pitch}
	case placeWaterFamily:
		packet = protocol.PlaceWater{Sequence: request.Sequence, Yaw: yaw, Pitch: pitch}
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}

	encodedID, payload, err := wireCodec.EncodeClient(state, packet)
	if err != nil {
		category, classified := clientRayRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified encode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	if encodedID != packetID {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoder published packet ID %d, want %d", c.ID, encodedID, packetID)
	}
	decoded, err := wireCodec.DecodeClient(state, encodedID, payload)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload does not decode back: %w", c.ID, err)
	}
	if !reflect.DeepEqual(decoded, packet) {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload decodes to %#v, want %#v", c.ID, decoded, packet)
	}
	fields, err := clientRayFields(c, decoded)
	if err != nil {
		return Outcome{}, nil, err
	}
	sum := sha256.Sum256(payload)
	return Outcome{
		Kind:                 "ok",
		Category:             packetOutcomeCategory,
		Fields:               fields,
		EncodedPayloadDigest: fmt.Sprintf("sha256:%x", sum),
	}, payload, nil
}

// clientRayCorpusRoutes is the closed route map the five families execute.
func clientRayCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range clientRayFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: clientRaysVersion, Operation: "decode"}] = runClientRayDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: clientRaysVersion, Operation: "encode"}] = runClientRayEncode
	}
	return routes
}

// clientRayCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer result.
type clientRayCaseDefinition struct {
	id     string
	family string
	relDir string
	op     string
	// input is the binary decode payload, already mutated where the case is a
	// malformed one.
	input []byte
	// request is the JSON encode payload, nil for a decode case.
	request any
	// wire is the expected encoded payload for an encode case.
	wire []byte
	// expect is the normalized outcome an execution has to reproduce.
	expect Outcome
}

// clientRayValidFields renders the normalized fields the five valid decode
// cases publish.
func clientRayValidFields() map[string]any {
	return map[string]any{
		"sequence": "0",
		"yaw":      "80000000",
		"pitch":    "3fc00000",
	}
}

// clientRayCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid cases
// use the canonical payload the Go encoder produces for these fields; the
// malformed decode cases mutate one angle of that payload at its boundary, and
// the invalid encode cases build a DTO the production validator refuses, so
// each rejection names the boundary that owns it.
func clientRayCaseDefinitions() []clientRayCaseDefinition {
	fields := clientRayValidFields()
	request := clientRayEncodeRequest{Sequence: 0, Yaw: "80000000", Pitch: "3fc00000"}

	mutatedWord := func(source []byte, offset int, value uint32) []byte {
		wire := append([]byte(nil), source...)
		copy(wire[offset:offset+4], []byte{
			byte(value),
			byte(value >> 8),
			byte(value >> 16),
			byte(value >> 24),
		})
		return wire
	}

	definitions := make([]clientRayCaseDefinition, 0, len(clientRayFamilies())*5)
	for _, family := range clientRayFamilies() {
		definitions = append(definitions,
			clientRayCaseDefinition{
				id:     family.id + "/" + clientRaysVersion + "/decode-valid",
				family: family.id,
				relDir: family.relDir,
				op:     "decode",
				input:  append([]byte(nil), clientRaysWire...),
				expect: Outcome{
					Kind:     "ok",
					Category: packetOutcomeCategory,
					Fields:   fields,
				},
			},
			clientRayCaseDefinition{
				id:      family.id + "/" + clientRaysVersion + "/encode-valid",
				family:  family.id,
				relDir:  family.relDir,
				op:      "encode",
				request: request,
				wire:    append([]byte(nil), clientRaysWire...),
				expect: Outcome{
					Kind:     "ok",
					Category: packetOutcomeCategory,
					Fields:   fields,
				},
			},
			clientRayCaseDefinition{
				id:     family.id + "/" + clientRaysVersion + "/decode-nan-yaw",
				family: family.id,
				relDir: family.relDir,
				op:     "decode",
				input:  mutatedWord(clientRaysWire, 8, 0x7fc0_0000),
				expect: Outcome{Kind: "error", Category: "invalid-value"},
			},
			clientRayCaseDefinition{
				id:     family.id + "/" + clientRaysVersion + "/decode-infinite-pitch",
				family: family.id,
				relDir: family.relDir,
				op:     "decode",
				input:  mutatedWord(clientRaysWire, 12, 0x7f80_0000),
				expect: Outcome{Kind: "error", Category: "invalid-value"},
			},
			clientRayCaseDefinition{
				id:     family.id + "/" + clientRaysVersion + "/encode-nan-yaw",
				family: family.id,
				relDir: family.relDir,
				op:     "encode",
				request: clientRayEncodeRequest{
					Sequence: 0,
					Yaw:      "7fc00000",
					Pitch:    "3fc00000",
				},
				expect: Outcome{Kind: "error", Category: "invalid-value"},
			},
		)
	}
	return definitions
}

// clientRayLabel renders one case's asset label from its identity.
func clientRayLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// buildClientRayCandidate builds one case's manifest entry and asset bytes from
// its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildClientRayCandidate(t *testing.T, definition clientRayCaseDefinition) clientRayCandidate {
	t.Helper()

	label := clientRayLabel(definition.id)
	inputPath := filepath.ToSlash(filepath.Join(definition.relDir, label+".input"+inputExtension(definition.op)))
	expectedPath := filepath.ToSlash(filepath.Join(definition.relDir, label+".expected.json"))

	var input []byte
	switch {
	case definition.op == "decode":
		input = append([]byte(nil), definition.input...)
	case definition.request != nil:
		rendered, err := json.MarshalIndent(definition.request, "", "  ")
		if err != nil {
			t.Fatalf("case %s: marshal encode input: %v", definition.id, err)
		}
		input = append(rendered, '\n')
	default:
		t.Fatalf("case %s: neither a decode payload nor an encode request", definition.id)
	}

	expect := definition.expect
	if definition.wire != nil {
		sum := sha256.Sum256(definition.wire)
		expect.EncodedPayloadDigest = fmt.Sprintf("sha256:%x", sum)
	}
	expected, err := marshalIndentedOutcome(expect)
	if err != nil {
		t.Fatalf("case %s: render expectation: %v", definition.id, err)
	}
	assets := map[string][]byte{
		inputPath:    input,
		expectedPath: append(expected, '\n'),
	}

	spec := CaseSpec{
		ID:           definition.id,
		Family:       definition.family,
		Version:      clientRaysVersion,
		Operation:    definition.op,
		PacketKey:    clientRayKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return clientRayCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// clientRayKeyPointer resolves one family's reviewed packet key for a case spec.
func clientRayKeyPointer(family string) *PacketKeySpec {
	key, owned := clientRayFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// clientRayCandidate is one reviewed case: its manifest specification, the
// exact asset bytes it publishes, and the expectation an independent execution
// has to reproduce.
type clientRayCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which the
	// producer's own bytes are compared against.
	Wire []byte
}

// clientRayCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func clientRayCandidates(t *testing.T, root string) []clientRayCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := clientRayCaseDefinitions()
	candidates := make([]clientRayCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildClientRayCandidate(t, definition)
		if candidate.Spec.PacketKey == nil {
			t.Fatalf("case %s names family %q, which has no reviewed packet key", candidate.Spec.ID, candidate.Spec.Family)
		}
		for relative, want := range candidate.Assets {
			tracked, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
			if readErr != nil {
				if !os.IsNotExist(readErr) {
					t.Fatalf("read tracked asset %s: %v", relative, readErr)
				}
				continue
			}
			if !bytes.Equal(tracked, want) {
				t.Fatalf("tracked asset %s differs from the reviewed candidate", relative)
			}
		}
		if existing, ok := registered[candidate.Spec.ID]; ok {
			if !reflect.DeepEqual(existing, candidate.Spec) {
				t.Fatalf("tracked case %s is %#v, want the reviewed candidate %#v", candidate.Spec.ID, existing, candidate.Spec)
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

// clientRayRegisteredCase reports whether the base manifest already carries one
// case identity, so a re-merge of an integrated candidate adds nothing.
func clientRayRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// clientRaySelection is this group's registration: its cases, the Go sources
// its rules are read from, and the routes the five families execute.
func clientRaySelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := clientRayCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range clientRayFamilies() {
		sources[family.id] = clientRayClientSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(clientRayFamilies())*2)
	for _, family := range clientRayFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: clientRaysVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: clientRaysVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  clientRaysProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// clientRayClientSources lists the Go sources one ray family reads its rules
// from: the shared codec dispatch plus the family's own message file.
func clientRayClientSources(family string) []string {
	if family == openContainerFamily {
		return []string{
			"packages/shared/network/codec/codec_client.go",
			"packages/shared/network/protocol/message_container.go",
		}
	}
	return []string{
		"packages/shared/network/codec/codec_client.go",
		"packages/shared/network/protocol/message_command.go",
	}
}

// clientRayManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func clientRayManifest(t *testing.T, root string, candidates []clientRayCandidate) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)

	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	cloned := Inventory{
		SchemaVersion:  frozen.SchemaVersion,
		SourceRevision: frozen.SourceRevision,
		Identities:     frozen.Identities,
		Families:       append([]Family(nil), frozen.Families...),
		Cases:          cases,
	}
	for index := range cloned.Families {
		family := cloned.Families[index].ID
		if !clientRayOwnsFamily(family) {
			cloned.Families[index].Cases = nil
			continue
		}
		var listed []string
		for _, c := range cloned.Cases {
			if c.Family == family {
				listed = append(listed, c.ID)
			}
		}
		sort.Strings(listed)
		cloned.Families[index].Cases = listed
	}

	encoded, err := encodeInventory(cloned)
	if err != nil {
		t.Fatalf("encode client ray working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write client ray working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load client ray working manifest: %v", err)
	}
	return loaded
}

// clientRayOwnsFamily reports whether this group registers cases for one family.
func clientRayOwnsFamily(family string) bool {
	_, owned := clientRayFamilyKeys[family]
	return owned
}

// clientRayScratchRoot stages this group's candidate assets in a harness-owned
// temporary directory, because a corpus case has to resolve under the root the
// runner is given.
func clientRayScratchRoot(t *testing.T, root string, candidates []clientRayCandidate) string {
	t.Helper()
	staged := t.TempDir()
	for _, candidate := range candidates {
		for relative, data := range candidate.Assets {
			target := filepath.Join(staged, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create staged parent for %s: %v", relative, err)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				t.Fatalf("stage candidate asset %s: %v", relative, err)
			}
		}
	}
	return staged
}

// clientRayCandidateByID resolves one candidate by its case identity.
func clientRayCandidateByID(t *testing.T, candidates []clientRayCandidate, id string) clientRayCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return clientRayCandidate{}
}

// clientRayObservation resolves one executed observation by its case identity.
func clientRayObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// clientRayExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var clientRayExportPublished bool

// clientRayCandidatesExport publishes the reviewed candidates and the complete
// merged manifest candidate through the existing external exporter and returns
// the published producer directory. An unset export variable publishes nothing
// and returns "", so an ordinary test run never writes outside its own temporary
// storage.
func clientRayCandidatesExport(t *testing.T, root string, candidates []clientRayCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if clientRayExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-client-rays")
	}
	clientRayExportPublished = true

	manifest, err := encodeInventory(merged)
	if err != nil {
		t.Fatalf("encode manifest candidate: %v", err)
	}
	assets := []generatedAsset{{RelativePath: "contracts.json", Data: append(manifest, '\n')}}
	relatives := make([]string, 0)
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		for relative := range candidate.Assets {
			if seen[relative] {
				continue
			}
			seen[relative] = true
			relatives = append(relatives, relative)
		}
	}
	sort.Strings(relatives)
	for _, relative := range relatives {
		for _, candidate := range candidates {
			if data, ok := candidate.Assets[relative]; ok {
				assets = append(assets, generatedAsset{RelativePath: relative, Data: data})
				break
			}
		}
	}
	published, err := exportGeneratedAssets(root, exportRoot, clientRaysProducerID, assets)
	if err != nil {
		t.Fatalf("export client ray candidates: %v", err)
	}
	return published
}

// clientRayDecodeCaseIDs lists the five valid decode cases the producer pins.
func clientRayDecodeCaseIDs() []string {
	ids := make([]string, 0, len(clientRayFamilies()))
	for _, family := range clientRayFamilies() {
		ids = append(ids, family.id+"/"+clientRaysVersion+"/decode-valid")
	}
	return ids
}

// clientRayEncodeCaseIDs lists the five valid encode cases the producer pins.
func clientRayEncodeCaseIDs() []string {
	ids := make([]string, 0, len(clientRayFamilies()))
	for _, family := range clientRayFamilies() {
		ids = append(ids, family.id+"/"+clientRaysVersion+"/encode-valid")
	}
	return ids
}

// clientRayRejectedDecodeCaseIDs lists the ten malformed decode cases.
func clientRayRejectedDecodeCaseIDs() []string {
	ids := make([]string, 0, len(clientRayFamilies())*2)
	for _, family := range clientRayFamilies() {
		ids = append(ids,
			family.id+"/"+clientRaysVersion+"/decode-nan-yaw",
			family.id+"/"+clientRaysVersion+"/decode-infinite-pitch",
		)
	}
	return ids
}

// clientRayRejectedEncodeCaseIDs lists the five invalid encode cases.
func clientRayRejectedEncodeCaseIDs() []string {
	ids := make([]string, 0, len(clientRayFamilies()))
	for _, family := range clientRayFamilies() {
		ids = append(ids, family.id+"/"+clientRaysVersion+"/encode-nan-yaw")
	}
	return ids
}

// TestProtocolClientRaysOracleDecodesEveryValidCase pins that the real Go
// decoder publishes the canonical fields for every family's valid decode case,
// with the -0.0 look bits rendered as bit strings rather than numbers.
func TestProtocolClientRaysOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientRayCandidates(t, root)

	for _, id := range clientRayDecodeCaseIDs() {
		candidate := clientRayCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientRayDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientRayDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientRaysOracleEncodesCanonicalWire pins each encode producer
// against the reviewed wire literal and against the recorded digest.
func TestProtocolClientRaysOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientRayCandidates(t, root)

	for _, id := range clientRayEncodeCaseIDs() {
		candidate := clientRayCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientRayEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientRayEncode(%s): %v", id, err)
		}
		if !bytes.Equal(encoded, candidate.Wire) {
			t.Fatalf("case %s encoded %x, want %x", id, encoded, candidate.Wire)
		}
		sum := sha256.Sum256(candidate.Wire)
		if outcome.EncodedPayloadDigest != fmt.Sprintf("sha256:%x", sum) {
			t.Fatalf("case %s digest = %s, want sha256:%x", id, outcome.EncodedPayloadDigest, sum)
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientRaysOracleRejectsMalformedCasesAtTheirBoundary pins that
// every malformed decode case and invalid encode case is refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolClientRaysOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientRayCandidates(t, root)

	for _, id := range clientRayRejectedDecodeCaseIDs() {
		candidate := clientRayCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientRayDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientRayDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range clientRayRejectedEncodeCaseIDs() {
		candidate := clientRayCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientRayEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientRayEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientRaysOracleExpectedFieldMutationFailsComparison pins that the
// recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded, and replacing the encoded digest
// fails comparison against the reviewed wire.
func TestProtocolClientRaysOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientRayCandidates(t, root)

	decode := clientRayCandidateByID(t, candidates, openContainerDecodeCaseID)
	produced, _, err := runClientRayDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runClientRayDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	fields["yaw"] = "00000000"
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated yaw bit string compares equal to the produced outcome")
	}

	encode := clientRayCandidateByID(t, candidates, tillSoilEncodeCaseID)
	producedEncode, _, err := runClientRayEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runClientRayEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolClientRaysOracleRoutesExecuteEveryCase executes this group's
// complete case set through the shared packet-case runner, once per case, and
// compares every observation with the reviewed expectation.
func TestProtocolClientRaysOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := clientRayCandidates(t, root)
	manifest := clientRayManifest(t, root, candidates)
	staged := clientRayScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, clientRayCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := clientRayObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientRaysOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so a
// case cannot claim coverage from its name alone.
func TestProtocolClientRaysOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientRayCandidates(t, root)
	manifest := clientRayManifest(t, root, candidates)
	staged := clientRayScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range clientRayFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: clientRaysVersion, Operation: "decode"}] = runClientRayDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range clientRayFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+clientRaysVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolClientRaysOracleManifestMergeRegistersClientRayRoutes pins that
// the merged manifest registers all five families' routes and case lists, leaves
// the source revision alone, and records this group's provenance sources.
func TestProtocolClientRaysOracleManifestMergeRegistersClientRayRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, clientRaySelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range clientRayFamilies() {
		var row *Family
		for index := range merged.Families {
			if merged.Families[index].ID == family.id {
				row = &merged.Families[index]
				break
			}
		}
		if row == nil {
			t.Fatalf("merged manifest has no %s family", family.id)
		}
		var listed []string
		for _, c := range merged.Cases {
			if c.Family == family.id {
				listed = append(listed, c.ID)
			}
		}
		sort.Strings(listed)
		if !reflect.DeepEqual(row.Cases, listed) {
			t.Fatalf("%s family lists %v, want %v", family.id, row.Cases, listed)
		}
		if len(listed) == 0 {
			t.Fatalf("%s family registers no case", family.id)
		}
		sources := make(map[string]bool, len(row.Sources))
		for _, source := range row.Sources {
			hash, hashErr := hashFile(filepath.Join(root, filepath.FromSlash(source.Path)))
			if hashErr != nil {
				t.Fatalf("hash provenance source %s: %v", source.Path, hashErr)
			}
			if hash != source.SHA256 {
				t.Fatalf("provenance source %s records %s, disk has %s", source.Path, source.SHA256, hash)
			}
			sources[source.Path] = true
		}
		for _, want := range clientRayClientSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so a
	// re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range clientRayCandidates(t, root) {
		if !clientRayRegisteredCase(base, candidate.Spec.ID) {
			wantCases++
		}
	}
	if len(merged.Cases) != wantCases {
		t.Fatalf("merged carries %d cases, want %d", len(merged.Cases), wantCases)
	}
	for index := 1; index < len(merged.Cases); index++ {
		if merged.Cases[index].ID < merged.Cases[index-1].ID {
			t.Fatalf("merged case list is not sorted by id at %d", index)
		}
	}
}

// TestProtocolClientRaysOracleCandidatesExportForReview publishes the reviewed
// candidates and the manifest candidate. An unset export variable publishes
// nothing, so the tracked corpus is never written by this package.
func TestProtocolClientRaysOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := clientRayCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), clientRaySelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := clientRayCandidatesExport(t, root, candidates, merged)
	if strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv)) == "" {
		if published != "" {
			t.Fatalf("published %s with the export variable unset", published)
		}
		return
	}
	if published == "" {
		t.Fatal("no candidate was published")
	}
	for _, candidate := range candidates {
		for relative := range candidate.Assets {
			path := filepath.Join(published, filepath.FromSlash(relative))
			if _, statErr := os.Lstat(path); statErr != nil {
				t.Fatalf("candidate asset %s is missing: %v", relative, statErr)
			}
		}
	}
	written, err := os.ReadFile(filepath.Join(published, "contracts.json"))
	if err != nil {
		t.Fatalf("read manifest candidate: %v", err)
	}
	encoded, err := encodeInventory(merged)
	if err != nil {
		t.Fatalf("encode merged manifest: %v", err)
	}
	if !bytes.Equal(written, append(encoded, '\n')) {
		t.Fatal("manifest candidate is not the encoded merged manifest")
	}
	reloaded, err := LoadInventory(filepath.Join(published, "contracts.json"))
	if err != nil {
		t.Fatalf("reload manifest candidate: %v", err)
	}
	if err := verifyProtocolManifest(root, loadRealManifest(t, root), reloaded); err != nil {
		t.Fatalf("manifest candidate does not reconcile: %v", err)
	}
}
