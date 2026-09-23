package main

// This file is the unsequenced chat command packet producer group: the one
// Play client-to-server family whose text is variable-length
// (`ChatCommand`, a canonical uvarint length prefix plus UTF-8 bytes)
// registers a decode and an encode route through the shared packet-case
// runner, so every case is executed by the real Go codec rather than restated
// here.
//
// The decode cases are the Go decoder's own bytes: the valid payload, the
// text-bound and encoding refusals, the payload above the pre-parse wire
// ceiling, the noncanonical length prefix, one trailing byte and a proper
// truncation. The encode cases carry the canonical JSON text and either the
// reviewed wire the Go encoder has to publish or a text the outbound
// validator has to refuse. Every negative carries exactly one violation. The
// text itself is never interpreted: a leading `@` is not addressing, a
// leading `/` is not a warp, and no case in this group declares a session, a
// deadline, a FIFO entry or a send.

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	// chatCommandFamily is the play chat command family this group registers.
	chatCommandFamily = "protocol.client.ChatCommand"
	// clientChatVersion is the protocol version the family is pinned to.
	clientChatVersion = "45"
	// clientChatProducerID is the exporter's producer identity for this group's
	// candidate assets and merged manifest.
	clientChatProducerID = "runtime-oracle/protocol-client-chat"

	// chatCommandCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	chatCommandCorpusRelDir = corpusCasesRelDir + "/protocol/ChatCommand"
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	chatCommandDecodeCaseID     = chatCommandFamily + "/" + clientChatVersion + "/decode-valid"
	chatCommandEncodeCaseID     = chatCommandFamily + "/" + clientChatVersion + "/encode-valid"
	chatCommandMaxTextDecodeID  = chatCommandFamily + "/" + clientChatVersion + "/decode-max-text"
	chatCommandAboveBoundEncID  = chatCommandFamily + "/" + clientChatVersion + "/encode-text-above-bound"
	chatCommandCeilingDecodeID  = chatCommandFamily + "/" + clientChatVersion + "/decode-payload-above-wire-ceiling"
	chatCommandEmptyDecodeID    = chatCommandFamily + "/" + clientChatVersion + "/decode-empty-text"
	chatCommandEmptyEncodeID    = chatCommandFamily + "/" + clientChatVersion + "/encode-empty-text"
	chatCommandNBSPDecodeID     = chatCommandFamily + "/" + clientChatVersion + "/decode-untrimmed-leading-nbsp"
	chatCommandControlDecodeID  = chatCommandFamily + "/" + clientChatVersion + "/decode-control-text"
	chatCommandInvalidUTF8ID    = chatCommandFamily + "/" + clientChatVersion + "/decode-invalid-utf8"
	chatCommandNonCanonicalID   = chatCommandFamily + "/" + clientChatVersion + "/decode-noncanonical-length"
	chatCommandTruncatedID      = chatCommandFamily + "/" + clientChatVersion + "/decode-truncated"
	chatCommandTrailingByteID   = chatCommandFamily + "/" + clientChatVersion + "/decode-trailing-byte"
	clientChatWireTextMaxBytes  = 1024
	clientChatWirePayloadCeilin = clientChatWireTextMaxBytes + 2
)

// clientChatFamilyKeys pins the family's complete packet key. A case names its
// key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs.
var clientChatFamilyKeys = map[string]PacketKeySpec{
	chatCommandFamily: {Direction: packetDirectionClient, State: packetStatePlay, ID: 12},
}

// clientChatFamilies lists the family in registry order, with the corpus
// directory that holds its assets. The order is the review order, not a
// dispatch table.
func clientChatFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{chatCommandFamily, chatCommandCorpusRelDir},
	}
}

// clientChatFollowWire is the reviewed wire literal the ChatCommand canonical
// vector pins: the single-byte canonical uvarint length prefix 12 followed by
// the twelve bytes of "@mira follow".
var clientChatFollowWire = []byte{
	0x0c, 0x40, 0x6d, 0x69, 0x72, 0x61, 0x20, 0x66, 0x6f, 0x6c, 0x6c, 0x6f, 0x77,
}

// clientChatMaxTextWire builds the reviewed maximum-text literal: the two-byte
// canonical uvarint prefix for 1024 followed by 1024 "a" bytes, which is
// exactly the fixed wire ceiling.
func clientChatMaxTextWire() []byte {
	wire := make([]byte, 0, clientChatWirePayloadCeilin)
	wire = append(wire, clientChatUvarint(clientChatWireTextMaxBytes)...)
	wire = append(wire, bytes.Repeat([]byte{'a'}, clientChatWireTextMaxBytes)...)
	return wire
}

// clientChatUvarint renders the canonical uvarint of one value, matching the Go
// primitive's shortest form, so a reviewed wire literal is the encoder's own
// encoding of the value it names.
func clientChatUvarint(value uint32) []byte {
	var encoded []byte
	for value >= 1<<7 {
		encoded = append(encoded, byte(value)|0x80)
		value >>= 7
	}
	return append(encoded, byte(value))
}

// clientChatRejectionCategory resolves the language-neutral rejection category
// for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and
// the outbound validators name, never over a sentinel identity, and a failure
// with no mapping is a hard error. The payload ceiling answers before any
// field is read, so an oversized payload is the `capacity` category rather
// than a field error. The one boundary the Go sentinels cannot express is a
// declared string length the payload cannot complete: `byteDecoder.string`
// answers it with the same error as a malformed UTF-8 text, while the frozen
// boundary taxonomy resolves an incomplete payload as the truncated category.
// The case's own derivation base decides between the two, because a rejected
// proper prefix of the reviewed payload is exactly the incomplete condition.
// The Rust consumer maps the same condition to its own truncated decode error,
// so both implementations publish one category.
func clientChatRejectionCategory(err error, derivedFrom, input []byte) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "chat command payload exceeds"):
		return "capacity", true
	case strings.Contains(message, "invalid uvarint"):
		return "invalid-varint", true
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "invalid chat command text"),
		strings.Contains(message, "chat command contains control character"):
		return "invalid-value", true
	case strings.Contains(message, "invalid string"):
		if isRejectedPrefix(derivedFrom, input) {
			return "truncated", true
		}
		return "invalid-value", true
	}
	return "", false
}

// clientChatDerivedFrom lists the reviewed payload the truncated case was
// derived from, so the boundary resolver can tell an incomplete payload from a
// mutated one.
func clientChatDerivedFrom(caseID string) []byte {
	switch caseID {
	case chatCommandTruncatedID:
		return append([]byte(nil), clientChatFollowWire...)
	default:
		return nil
	}
}

// clientChatPacketKey resolves one packet key to the Go state and numeric ID
// the codec dispatches on, and reports whether the packet travels
// client-to-server.
func clientChatPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := clientChatFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no chat command producer owns", c.ID, c.Family)
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

// clientChatFields renders the semantic field one chat command publishes: the
// text verbatim, including a leading mention prefix, because the codec
// performs no addressing.
func clientChatFields(text string) map[string]any {
	return map[string]any{
		"text": text,
	}
}

// newClientChatCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which this family never
// touches; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newClientChatCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// runClientChatDecode executes one chat command decode case through the real
// Go decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeClient` and classifies
// the failure it returns. It never reimplements the length-prefix, text or
// ceiling rules, so the recorded outcome is whatever the production codec
// decides about these exact bytes.
func runClientChatDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := clientChatPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newClientChatCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeClient(state, packetID, input)
	if err != nil {
		category, classified := clientChatRejectionCategory(err, clientChatDerivedFrom(c.ID), input)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	command, owned := packet.(protocol.ChatCommand)
	if !owned {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: clientChatFields(command.Text)}, nil, nil
}

// clientChatEncodeRequest is the canonical JSON field input one encode case
// carries: the text alone, with no sequence, session or addressing field.
type clientChatEncodeRequest struct {
	Text string `json:"text"`
}

// runClientChatEncode executes one chat command encode case through the real
// Go encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed field and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects
// must never be recorded as evidence.
func runClientChatEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := clientChatPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newClientChatCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var request clientChatEncodeRequest
	if err := decodeEncodeRequest(c, input, &request); err != nil {
		return Outcome{}, nil, err
	}
	packet := protocol.ChatCommand{Text: request.Text}

	encodedID, payload, err := wireCodec.EncodeClient(state, packet)
	if err != nil {
		category, classified := clientChatRejectionCategory(err, nil, nil)
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
	fields := clientChatFields(packet.Text)
	sum := sha256.Sum256(payload)
	return Outcome{
		Kind:                 "ok",
		Category:             packetOutcomeCategory,
		Fields:               fields,
		EncodedPayloadDigest: fmt.Sprintf("sha256:%x", sum),
	}, payload, nil
}

// clientChatCorpusRoutes is the closed route map the family executes.
func clientChatCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range clientChatFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: clientChatVersion, Operation: "decode"}] = runClientChatDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: clientChatVersion, Operation: "encode"}] = runClientChatEncode
	}
	return routes
}

// clientChatCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer
// result.
type clientChatCaseDefinition struct {
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

// clientChatCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. Each negative
// carries exactly one violation: the payload above the wire ceiling is one
// byte over the fixed bound, the text-above-bound case lives on the encode
// side only because the payload ceiling answers a length the decoder cannot
// reach first, and the truncated case is a proper prefix of the reviewed valid
// payload so the boundary resolver can tell an incomplete payload from a
// mutated one.
func clientChatCaseDefinitions() []clientChatCaseDefinition {
	valid := Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: clientChatFields("@mira follow")}
	maximum := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   clientChatFields(strings.Repeat("a", clientChatWireTextMaxBytes)),
	}
	invalid := Outcome{Kind: "error", Category: "invalid-value"}
	invalidVarint := Outcome{Kind: "error", Category: "invalid-varint"}
	capacity := Outcome{Kind: "error", Category: "capacity"}
	truncated := Outcome{Kind: "error", Category: "truncated"}
	trailing := Outcome{Kind: "error", Category: "trailing"}

	definitions := make([]clientChatCaseDefinition, 0, 13)
	definitions = append(definitions,
		clientChatCaseDefinition{
			id:     chatCommandDecodeCaseID,
			family: chatCommandFamily,
			relDir: chatCommandCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientChatFollowWire...),
			expect: valid,
		},
		clientChatCaseDefinition{
			id:      chatCommandEncodeCaseID,
			family:  chatCommandFamily,
			relDir:  chatCommandCorpusRelDir,
			op:      "encode",
			request: clientChatEncodeRequest{Text: "@mira follow"},
			wire:    append([]byte(nil), clientChatFollowWire...),
			expect:  valid,
		},
		clientChatCaseDefinition{
			id:     chatCommandMaxTextDecodeID,
			family: chatCommandFamily,
			relDir: chatCommandCorpusRelDir,
			op:     "decode",
			input:  clientChatMaxTextWire(),
			expect: maximum,
		},
		clientChatCaseDefinition{
			// The decode side cannot carry this case: the payload ceiling
			// answers a 1025-byte text before the string primitive reads the
			// declared length, so the text bound is reachable on the encode
			// side alone.
			id:      chatCommandAboveBoundEncID,
			family:  chatCommandFamily,
			relDir:  chatCommandCorpusRelDir,
			op:      "encode",
			request: clientChatEncodeRequest{Text: strings.Repeat("a", clientChatWireTextMaxBytes+1)},
			expect:  invalid,
		},
		clientChatCaseDefinition{
			id:     chatCommandCeilingDecodeID,
			family: chatCommandFamily,
			relDir: chatCommandCorpusRelDir,
			op:     "decode",
			input: append(
				clientChatUvarint(clientChatWireTextMaxBytes+1),
				bytes.Repeat([]byte{'a'}, clientChatWireTextMaxBytes+1)...,
			),
			expect: capacity,
		},
		clientChatCaseDefinition{
			id:     chatCommandEmptyDecodeID,
			family: chatCommandFamily,
			relDir: chatCommandCorpusRelDir,
			op:     "decode",
			input:  []byte{0x00},
			expect: invalid,
		},
		clientChatCaseDefinition{
			id:      chatCommandEmptyEncodeID,
			family:  chatCommandFamily,
			relDir:  chatCommandCorpusRelDir,
			op:      "encode",
			request: clientChatEncodeRequest{Text: ""},
			expect:  invalid,
		},
		clientChatCaseDefinition{
			id:     chatCommandNBSPDecodeID,
			family: chatCommandFamily,
			relDir: chatCommandCorpusRelDir,
			op:     "decode",
			input:  append(clientChatUvarint(5), 0xc2, 0xa0, 0x61, 0x62, 0x63),
			expect: invalid,
		},
		clientChatCaseDefinition{
			id:     chatCommandControlDecodeID,
			family: chatCommandFamily,
			relDir: chatCommandCorpusRelDir,
			op:     "decode",
			input:  []byte{0x03, 0x61, 0x01, 0x62},
			expect: invalid,
		},
		clientChatCaseDefinition{
			id:     chatCommandInvalidUTF8ID,
			family: chatCommandFamily,
			relDir: chatCommandCorpusRelDir,
			op:     "decode",
			input:  []byte{0x02, 0xc3, 0x28},
			expect: invalid,
		},
		clientChatCaseDefinition{
			id:     chatCommandNonCanonicalID,
			family: chatCommandFamily,
			relDir: chatCommandCorpusRelDir,
			op:     "decode",
			input:  []byte{0x80, 0x00, 0x61},
			expect: invalidVarint,
		},
		clientChatCaseDefinition{
			id:     chatCommandTruncatedID,
			family: chatCommandFamily,
			relDir: chatCommandCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientChatFollowWire[:6]...),
			expect: truncated,
		},
		clientChatCaseDefinition{
			id:     chatCommandTrailingByteID,
			family: chatCommandFamily,
			relDir: chatCommandCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), clientChatFollowWire...), 0x00),
			expect: trailing,
		},
	)

	return definitions
}

// clientChatLabel renders one case's asset label from its identity.
func clientChatLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// buildClientChatCandidate builds one case's manifest entry and asset bytes
// from its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildClientChatCandidate(t *testing.T, definition clientChatCaseDefinition) clientChatCandidate {
	t.Helper()

	label := clientChatLabel(definition.id)
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
		Version:      clientChatVersion,
		Operation:    definition.op,
		PacketKey:    clientChatKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return clientChatCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// clientChatKeyPointer resolves one family's reviewed packet key for a case
// spec.
func clientChatKeyPointer(family string) *PacketKeySpec {
	key, owned := clientChatFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// clientChatCandidate is one reviewed case: its manifest specification, the
// exact asset bytes it publishes, and the expectation an independent execution
// has to reproduce.
type clientChatCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which the
	// producer's own bytes are compared against.
	Wire []byte
}

// clientChatCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func clientChatCandidates(t *testing.T, root string) []clientChatCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := clientChatCaseDefinitions()
	candidates := make([]clientChatCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildClientChatCandidate(t, definition)
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

// clientChatRegisteredCase reports whether the base manifest already carries
// one case identity, so a re-merge of an integrated candidate adds nothing.
func clientChatRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// clientChatSelection is this group's registration: its cases, the Go sources
// its rules are read from, and the routes the family executes.
func clientChatSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := clientChatCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range clientChatFamilies() {
		sources[family.id] = clientChatClientSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(clientChatFamilies())*2)
	for _, family := range clientChatFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: clientChatVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: clientChatVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  clientChatProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// clientChatClientSources lists the Go sources the chat command family reads
// its rules from: the shared codec dispatch with its payload ceiling, and the
// family's own message file, which owns the text validator the shared planner
// instruction bound ties to.
func clientChatClientSources(family string) []string {
	shared := []string{
		"packages/shared/network/codec/codec_client.go",
		"packages/shared/network/protocol/message_companion.go",
	}
	if family != chatCommandFamily {
		return nil
	}
	return shared
}

// clientChatManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func clientChatManifest(t *testing.T, root string, candidates []clientChatCandidate) Inventory {
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
		if !clientChatOwnsFamily(family) {
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
		t.Fatalf("encode client chat working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write client chat working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load client chat working manifest: %v", err)
	}
	return loaded
}

// clientChatOwnsFamily reports whether this group registers cases for one
// family.
func clientChatOwnsFamily(family string) bool {
	_, owned := clientChatFamilyKeys[family]
	return owned
}

// clientChatScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve
// under the root the runner is given.
func clientChatScratchRoot(t *testing.T, root string, candidates []clientChatCandidate) string {
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

// clientChatCandidateByID resolves one candidate by its case identity.
func clientChatCandidateByID(t *testing.T, candidates []clientChatCandidate, id string) clientChatCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return clientChatCandidate{}
}

// clientChatObservation resolves one executed observation by its case
// identity.
func clientChatObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// clientChatExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var clientChatExportPublished bool

// clientChatCandidatesExport publishes the reviewed candidates and the
// complete merged manifest candidate through the existing external exporter
// and returns the published producer directory. An unset export variable
// publishes nothing and returns "", so an ordinary test run never writes
// outside its own temporary storage.
func clientChatCandidatesExport(t *testing.T, root string, candidates []clientChatCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if clientChatExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-client-chat")
	}
	clientChatExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, clientChatProducerID, assets)
	if err != nil {
		t.Fatalf("export client chat candidates: %v", err)
	}
	return published
}

// clientChatDecodeCaseIDs lists the valid decode cases the producer pins.
func clientChatDecodeCaseIDs() []string {
	return []string{
		chatCommandDecodeCaseID,
		chatCommandMaxTextDecodeID,
	}
}

// clientChatEncodeCaseIDs lists the valid encode case the producer pins.
func clientChatEncodeCaseIDs() []string {
	return []string{
		chatCommandEncodeCaseID,
	}
}

// clientChatRejectedDecodeCaseIDs lists the malformed decode cases.
func clientChatRejectedDecodeCaseIDs() []string {
	return []string{
		chatCommandCeilingDecodeID,
		chatCommandEmptyDecodeID,
		chatCommandNBSPDecodeID,
		chatCommandControlDecodeID,
		chatCommandInvalidUTF8ID,
		chatCommandNonCanonicalID,
		chatCommandTruncatedID,
		chatCommandTrailingByteID,
	}
}

// clientChatRejectedEncodeCaseIDs lists the invalid encode cases.
func clientChatRejectedEncodeCaseIDs() []string {
	return []string{
		chatCommandAboveBoundEncID,
		chatCommandEmptyEncodeID,
	}
}

// TestProtocolClientChatOracleDecodesEveryValidCase pins that the real Go
// decoder publishes the canonical text for both valid decode cases, with the
// text carried verbatim.
func TestProtocolClientChatOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientChatCandidates(t, root)

	for _, id := range clientChatDecodeCaseIDs() {
		candidate := clientChatCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientChatDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientChatDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientChatOracleEncodesCanonicalWire pins the encode producer
// against the reviewed wire literal and against the recorded digest.
func TestProtocolClientChatOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientChatCandidates(t, root)

	for _, id := range clientChatEncodeCaseIDs() {
		candidate := clientChatCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientChatEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientChatEncode(%s): %v", id, err)
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

// TestProtocolClientChatOracleRejectsMalformedCasesAtTheirBoundary pins that
// every malformed decode case and invalid encode case is refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolClientChatOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientChatCandidates(t, root)

	for _, id := range clientChatRejectedDecodeCaseIDs() {
		candidate := clientChatCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientChatDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientChatDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range clientChatRejectedEncodeCaseIDs() {
		candidate := clientChatCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientChatEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientChatEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientChatOracleIncompletePayloadPrefixIsTruncated pins the
// controller ruling this group's string boundary rests on: a declared length
// the payload cannot complete is an incomplete payload, so the producer
// classifies it as the truncated category even though the Go string primitive
// answers it with the same sentinel as a malformed UTF-8 text, and the Rust
// reader reports the same category through its own truncated decode error.
func TestProtocolClientChatOracleIncompletePayloadPrefixIsTruncated(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientChatCandidates(t, root)

	candidate := clientChatCandidateByID(t, candidates, chatCommandTruncatedID)
	input := candidate.Assets[candidate.Spec.Input.Path]
	if !isRejectedPrefix(clientChatDerivedFrom(candidate.Spec.ID), input) {
		t.Fatalf("case %s input is not a proper prefix of the reviewed payload", candidate.Spec.ID)
	}
	outcome, _, err := runClientChatDecode(candidate.Spec, input)
	if err != nil {
		t.Fatalf("runClientChatDecode(%s): %v", candidate.Spec.ID, err)
	}
	if outcome.Kind != "error" || outcome.Category != "truncated" {
		t.Fatalf("case %s produced %#v, want the truncated category", candidate.Spec.ID, outcome)
	}
}

// TestProtocolClientChatOracleExpectedFieldMutationFailsComparison pins that
// the recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded, and replacing the encoded
// digest fails comparison against the reviewed wire.
func TestProtocolClientChatOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientChatCandidates(t, root)

	decode := clientChatCandidateByID(t, candidates, chatCommandDecodeCaseID)
	produced, _, err := runClientChatDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runClientChatDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	fields["text"] = "chop oak"
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated text compares equal to the produced outcome")
	}

	encode := clientChatCandidateByID(t, candidates, chatCommandEncodeCaseID)
	producedEncode, _, err := runClientChatEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runClientChatEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolClientChatOracleRoutesExecuteEveryCase executes this group's
// complete case set through the shared packet-case runner, once per case, and
// compares every observation with the reviewed expectation.
func TestProtocolClientChatOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := clientChatCandidates(t, root)
	manifest := clientChatManifest(t, root, candidates)
	staged := clientChatScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, clientChatCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := clientChatObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientChatOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so
// a case cannot claim coverage from its name alone.
func TestProtocolClientChatOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientChatCandidates(t, root)
	manifest := clientChatManifest(t, root, candidates)
	staged := clientChatScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{
		{FamilyID: chatCommandFamily, Version: clientChatVersion, Operation: "decode"}: runClientChatDecode,
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	if !strings.Contains(err.Error(), chatCommandFamily+"/"+clientChatVersion+"/encode") {
		t.Fatalf("rejection %v does not name the family's encode route", err)
	}
}

// TestProtocolClientChatOracleManifestMergeRegistersChatRoutes pins that the
// merged manifest registers the family's routes and case list, leaves the
// source revision alone, and records this group's provenance sources.
func TestProtocolClientChatOracleManifestMergeRegistersChatRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, clientChatSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range clientChatFamilies() {
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
		for _, want := range clientChatClientSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so a
	// re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range clientChatCandidates(t, root) {
		if !clientChatRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolClientChatOracleCandidatesExportForReview publishes the reviewed
// candidates and the manifest candidate. An unset export variable publishes
// nothing, so the tracked corpus is never written by this package.
func TestProtocolClientChatOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := clientChatCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), clientChatSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := clientChatCandidatesExport(t, root, candidates, merged)
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
