package main

// This file is the simple inventory and crafting command packet producer
// group: the six Play client-to-server command families (`MoveInventoryStack`,
// `MoveCraftingStack`, `CloseContainer`, `DropSelectedItem`, `EquipArmor` and
// `TakeCraftingOutput`) each register a decode and an encode route through the
// shared packet-case runner, so every case is executed by the real Go codec
// rather than restated here.
//
// The decode cases are the Go decoder's own bytes: the valid payload, a
// truncated or trailing-byte payload, and the slots or sequence each family
// rejects. The encode cases carry canonical JSON fields and the reviewed wire
// the Go encoder has to publish, or a DTO the Go outbound validator has to
// refuse. The input values themselves move no world state: inventory contents,
// the moved count, the crafting output recipe, the drop position and the
// equipped armor slot stay authority-owned, so no case in this group declares
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
	// moveInventoryFamily is the play inventory move family this group registers.
	moveInventoryFamily = "protocol.client.MoveInventoryStack"
	// moveCraftingFamily is the play crafting move family this group registers.
	moveCraftingFamily = "protocol.client.MoveCraftingStack"
	// closeContainerFamily is the play container close family this group registers.
	closeContainerFamily = "protocol.client.CloseContainer"
	// dropSelectedFamily is the play drop selected item family this group registers.
	dropSelectedFamily = "protocol.client.DropSelectedItem"
	// equipArmorFamily is the play equip armor family this group registers.
	equipArmorFamily = "protocol.client.EquipArmor"
	// takeCraftingOutputFamily is the play take crafting output family this group registers.
	takeCraftingOutputFamily = "protocol.client.TakeCraftingOutput"
	// clientInventoryVersion is the protocol version every family is pinned to.
	clientInventoryVersion = "45"
	// clientInventoryProducerID is the exporter's producer identity for this group's
	// candidate assets and merged manifest.
	clientInventoryProducerID = "runtime-oracle/protocol-client-inventory"

	// moveInventoryCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	moveInventoryCorpusRelDir = corpusCasesRelDir + "/protocol/MoveInventoryStack"
	// moveCraftingCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	moveCraftingCorpusRelDir = corpusCasesRelDir + "/protocol/MoveCraftingStack"
	// closeContainerCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	closeContainerCorpusRelDir = corpusCasesRelDir + "/protocol/CloseContainer"
	// dropSelectedCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	dropSelectedCorpusRelDir = corpusCasesRelDir + "/protocol/DropSelectedItem"
	// equipArmorCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	equipArmorCorpusRelDir = corpusCasesRelDir + "/protocol/EquipArmor"
	// takeCraftingOutputCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	takeCraftingOutputCorpusRelDir = corpusCasesRelDir + "/protocol/TakeCraftingOutput"
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	moveInventoryDecodeCaseID      = moveInventoryFamily + "/" + clientInventoryVersion + "/decode-valid"
	moveInventoryEncodeCaseID      = moveInventoryFamily + "/" + clientInventoryVersion + "/encode-valid"
	moveInventoryFromAboveID       = moveInventoryFamily + "/" + clientInventoryVersion + "/decode-from-above-range"
	moveInventoryToAboveID         = moveInventoryFamily + "/" + clientInventoryVersion + "/decode-to-above-range"
	moveInventorySameSlotDecodeID  = moveInventoryFamily + "/" + clientInventoryVersion + "/decode-same-slot"
	moveInventorySameSlotEncodeID  = moveInventoryFamily + "/" + clientInventoryVersion + "/encode-same-slot"
	moveCraftingDecodeCaseID       = moveCraftingFamily + "/" + clientInventoryVersion + "/decode-valid"
	moveCraftingEncodeCaseID       = moveCraftingFamily + "/" + clientInventoryVersion + "/encode-valid"
	moveCraftingBothInventoryID    = moveCraftingFamily + "/" + clientInventoryVersion + "/decode-inventory-to-inventory"
	moveCraftingSameSlotDecodeID   = moveCraftingFamily + "/" + clientInventoryVersion + "/decode-same-slot"
	moveCraftingAboveViewID        = moveCraftingFamily + "/" + clientInventoryVersion + "/decode-above-view"
	moveCraftingBothInventoryEncID = moveCraftingFamily + "/" + clientInventoryVersion + "/encode-inventory-to-inventory"
	closeContainerDecodeCaseID     = closeContainerFamily + "/" + clientInventoryVersion + "/decode-valid"
	closeContainerEncodeCaseID     = closeContainerFamily + "/" + clientInventoryVersion + "/encode-valid"
	closeContainerTruncatedID      = closeContainerFamily + "/" + clientInventoryVersion + "/decode-truncated-byte"
	closeContainerTrailingID       = closeContainerFamily + "/" + clientInventoryVersion + "/decode-trailing-byte"
	dropSelectedDecodeCaseID       = dropSelectedFamily + "/" + clientInventoryVersion + "/decode-valid"
	dropSelectedEncodeCaseID       = dropSelectedFamily + "/" + clientInventoryVersion + "/encode-valid"
	dropSelectedTruncatedID        = dropSelectedFamily + "/" + clientInventoryVersion + "/decode-truncated-byte"
	dropSelectedTrailingID         = dropSelectedFamily + "/" + clientInventoryVersion + "/decode-trailing-byte"
	equipArmorDecodeCaseID         = equipArmorFamily + "/" + clientInventoryVersion + "/decode-valid"
	equipArmorEncodeCaseID         = equipArmorFamily + "/" + clientInventoryVersion + "/encode-valid"
	equipArmorTruncatedID          = equipArmorFamily + "/" + clientInventoryVersion + "/decode-truncated-byte"
	equipArmorTrailingID           = equipArmorFamily + "/" + clientInventoryVersion + "/decode-trailing-byte"
	takeCraftingDecodeCaseID       = takeCraftingOutputFamily + "/" + clientInventoryVersion + "/decode-valid"
	takeCraftingEncodeCaseID       = takeCraftingOutputFamily + "/" + clientInventoryVersion + "/encode-valid"
	takeCraftingZeroDecodeID       = takeCraftingOutputFamily + "/" + clientInventoryVersion + "/decode-zero-sequence"
	takeCraftingZeroEncodeID       = takeCraftingOutputFamily + "/" + clientInventoryVersion + "/encode-zero-sequence"
)

// clientInventoryFamilyKeys pins each family's complete packet key. A case names its
// key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs.
var clientInventoryFamilyKeys = map[string]PacketKeySpec{
	moveInventoryFamily:      {Direction: packetDirectionClient, State: packetStatePlay, ID: 6},
	moveCraftingFamily:       {Direction: packetDirectionClient, State: packetStatePlay, ID: 7},
	closeContainerFamily:     {Direction: packetDirectionClient, State: packetStatePlay, ID: 10},
	dropSelectedFamily:       {Direction: packetDirectionClient, State: packetStatePlay, ID: 11},
	takeCraftingOutputFamily: {Direction: packetDirectionClient, State: packetStatePlay, ID: 15},
	equipArmorFamily:         {Direction: packetDirectionClient, State: packetStatePlay, ID: 18},
}

// clientInventoryFamilies lists the six families in registry order, with the corpus
// directory that holds each family's assets. The order is the review order, not
// a dispatch table.
func clientInventoryFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{moveInventoryFamily, moveInventoryCorpusRelDir},
		{moveCraftingFamily, moveCraftingCorpusRelDir},
		{closeContainerFamily, closeContainerCorpusRelDir},
		{dropSelectedFamily, dropSelectedCorpusRelDir},
		{equipArmorFamily, equipArmorCorpusRelDir},
		{takeCraftingOutputFamily, takeCraftingOutputCorpusRelDir},
	}
}

// clientInventoryMoveWire is the reviewed wire literal the MoveInventoryStack
// canonical vector pins: sequence 0, source slot 0 and target slot 35, which is
// the Go encoder's own output for those fields.
var clientInventoryMoveWire = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x23,
}

// clientInventoryCraftingMoveWire is the reviewed wire literal the
// MoveCraftingStack canonical vector pins: sequence 0, source grid slot 8 and
// target inventory slot 44.
var clientInventoryCraftingMoveWire = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x08, 0x2c,
}

// clientInventorySequenceWire is the reviewed wire literal the three total
// sequence-only families pin: sequence 0 and nothing else.
var clientInventorySequenceWire = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// clientInventoryTakeWire is the reviewed wire literal TakeCraftingOutput pins:
// sequence 1, because the family refuses sequence 0.
var clientInventoryTakeWire = []byte{
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// clientInventoryRejectionCategory resolves the language-neutral rejection category
// for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and the
// outbound validators name, never over a sentinel identity, and a failure with
// no mapping is a hard error. The primitive answer precedes the validator
// message, because the decoder rejects a short or over-long payload while
// reading and only then hands the record to `Validate`.
func clientInventoryRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "slot is outside"),
		strings.Contains(message, "source equals target"),
		strings.Contains(message, "both ends in the inventory region"),
		strings.Contains(message, "sequence is zero"):
		return "invalid-value", true
	}
	return "", false
}

// clientInventoryPacketKey resolves one packet key to the Go state and numeric ID the
// codec dispatches on, and reports whether the packet travels client-to-server.
func clientInventoryPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := clientInventoryFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no client inventory producer owns", c.ID, c.Family)
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

// clientInventoryFields renders the semantic fields one decoded inventory command
// packet publishes.
//
// The fields come from the DTO the production decoder returned, never from the
// case input. The sequence renders as a decimal string so the full u64 range
// stays lossless, and the two move slots render as JSON numbers, which is the
// canonical field encoding the Rust consumer publishes too.
func clientInventoryFields(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.MoveInventoryStack:
		return clientInventoryMoveFields(message.Sequence, message.From, message.To), nil
	case protocol.MoveCraftingStack:
		return clientInventoryMoveFields(message.Sequence, message.From, message.To), nil
	case protocol.CloseContainer:
		return clientInventorySequenceFields(message.Sequence), nil
	case protocol.DropSelectedItem:
		return clientInventorySequenceFields(message.Sequence), nil
	case protocol.EquipArmor:
		return clientInventorySequenceFields(message.Sequence), nil
	case protocol.TakeCraftingOutput:
		return clientInventorySequenceFields(message.Sequence), nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// clientInventoryMoveFields renders the shared sequence-and-slots fields of one
// move DTO.
func clientInventoryMoveFields(sequence uint64, from, to uint8) map[string]any {
	return map[string]any{
		"sequence": strconv.FormatUint(sequence, 10),
		"from":     from,
		"to":       to,
	}
}

// clientInventorySequenceFields renders the sequence-only fields the four
// total command DTOs publish.
func clientInventorySequenceFields(sequence uint64) map[string]any {
	return map[string]any{
		"sequence": strconv.FormatUint(sequence, 10),
	}
}

// newClientInventoryCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newClientInventoryCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// runClientInventoryDecode executes one inventory command decode case through the
// real Go decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeClient` and classifies
// the failure it returns. It never reimplements the length-prefix, slot or
// sequence rules, so the recorded outcome is whatever the production codec
// decides about these exact bytes.
func runClientInventoryDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := clientInventoryPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newClientInventoryCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeClient(state, packetID, input)
	if err != nil {
		category, classified := clientInventoryRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := clientInventoryFields(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// clientInventoryMoveRequest is the canonical JSON field input one move encode
// case carries. The sequence is a decimal JSON integer, which the full u64
// range survives, and the two slots are byte-range numbers.
type clientInventoryMoveRequest struct {
	Sequence uint64 `json:"sequence"`
	From     uint8  `json:"from"`
	To       uint8  `json:"to"`
}

// clientInventorySequenceRequest is the canonical JSON field input one
// sequence-only encode case carries.
type clientInventorySequenceRequest struct {
	Sequence uint64 `json:"sequence"`
}

// runClientInventoryEncode executes one inventory command encode case through the
// real Go encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects must
// never be recorded as evidence.
func runClientInventoryEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := clientInventoryPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newClientInventoryCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet protocol.ClientPacket
	switch c.Family {
	case moveInventoryFamily, moveCraftingFamily:
		var request clientInventoryMoveRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		if c.Family == moveInventoryFamily {
			packet = protocol.MoveInventoryStack{Sequence: request.Sequence, From: request.From, To: request.To}
		} else {
			packet = protocol.MoveCraftingStack{Sequence: request.Sequence, From: request.From, To: request.To}
		}
	default:
		var request clientInventorySequenceRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		switch c.Family {
		case closeContainerFamily:
			packet = protocol.CloseContainer{Sequence: request.Sequence}
		case dropSelectedFamily:
			packet = protocol.DropSelectedItem{Sequence: request.Sequence}
		case equipArmorFamily:
			packet = protocol.EquipArmor{Sequence: request.Sequence}
		case takeCraftingOutputFamily:
			packet = protocol.TakeCraftingOutput{Sequence: request.Sequence}
		default:
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
		}
	}

	encodedID, payload, err := wireCodec.EncodeClient(state, packet)
	if err != nil {
		category, classified := clientInventoryRejectionCategory(err)
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
	fields, err := clientInventoryFields(c, decoded)
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

// clientInventoryCorpusRoutes is the closed route map the six families execute.
func clientInventoryCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range clientInventoryFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: clientInventoryVersion, Operation: "decode"}] = runClientInventoryDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: clientInventoryVersion, Operation: "encode"}] = runClientInventoryEncode
	}
	return routes
}

// clientInventoryCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer result.
type clientInventoryCaseDefinition struct {
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

// clientInventoryValidMoveFields renders the normalized fields the two valid move
// decode cases publish.
func clientInventoryValidMoveFields(from, to uint8) map[string]any {
	return map[string]any{
		"sequence": "0",
		"from":     from,
		"to":       to,
	}
}

// clientInventoryValidSequenceFields renders the normalized fields the valid
// sequence-only decode cases publish.
func clientInventoryValidSequenceFields(sequence uint64) map[string]any {
	return map[string]any{
		"sequence": strconv.FormatUint(sequence, 10),
	}
}

// clientInventoryCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid cases
// use the canonical payload the Go encoder produces for these fields; the
// malformed decode cases truncate the payload, append one trailing byte, or
// mutate one slot byte or the sequence at its boundary, and the invalid encode
// cases build a DTO the production validator refuses, so each rejection names
// the boundary that owns it.
func clientInventoryCaseDefinitions() []clientInventoryCaseDefinition {
	moveFields := clientInventoryValidMoveFields(0, 35)
	craftingFields := clientInventoryValidMoveFields(8, 44)
	zeroSequenceFields := clientInventoryValidSequenceFields(0)
	takeFields := clientInventoryValidSequenceFields(1)
	invalid := Outcome{Kind: "error", Category: "invalid-value"}
	truncated := Outcome{Kind: "error", Category: "truncated"}
	trailing := Outcome{Kind: "error", Category: "trailing"}

	mutatedSlot := func(source []byte, offset int, value uint8) []byte {
		wire := append([]byte(nil), source...)
		wire[offset] = value
		return wire
	}

	definitions := make([]clientInventoryCaseDefinition, 0, len(clientInventoryFamilies())*5)
	definitions = append(definitions,
		clientInventoryCaseDefinition{
			id:     moveInventoryDecodeCaseID,
			family: moveInventoryFamily,
			relDir: moveInventoryCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientInventoryMoveWire...),
			expect: Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: moveFields},
		},
		clientInventoryCaseDefinition{
			id:      moveInventoryEncodeCaseID,
			family:  moveInventoryFamily,
			relDir:  moveInventoryCorpusRelDir,
			op:      "encode",
			request: clientInventoryMoveRequest{Sequence: 0, From: 0, To: 35},
			wire:    append([]byte(nil), clientInventoryMoveWire...),
			expect:  Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: moveFields},
		},
		clientInventoryCaseDefinition{
			id:     moveInventoryFromAboveID,
			family: moveInventoryFamily,
			relDir: moveInventoryCorpusRelDir,
			op:     "decode",
			input:  mutatedSlot(clientInventoryMoveWire, 8, 0x24),
			expect: invalid,
		},
		clientInventoryCaseDefinition{
			id:     moveInventoryToAboveID,
			family: moveInventoryFamily,
			relDir: moveInventoryCorpusRelDir,
			op:     "decode",
			input:  mutatedSlot(clientInventoryMoveWire, 9, 0x24),
			expect: invalid,
		},
		clientInventoryCaseDefinition{
			id:     moveInventorySameSlotDecodeID,
			family: moveInventoryFamily,
			relDir: moveInventoryCorpusRelDir,
			op:     "decode",
			input:  mutatedSlot(clientInventoryMoveWire, 9, 0x00),
			expect: invalid,
		},
		clientInventoryCaseDefinition{
			id:      moveInventorySameSlotEncodeID,
			family:  moveInventoryFamily,
			relDir:  moveInventoryCorpusRelDir,
			op:      "encode",
			request: clientInventoryMoveRequest{Sequence: 0, From: 0, To: 0},
			expect:  invalid,
		},
		clientInventoryCaseDefinition{
			id:     moveCraftingDecodeCaseID,
			family: moveCraftingFamily,
			relDir: moveCraftingCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientInventoryCraftingMoveWire...),
			expect: Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: craftingFields},
		},
		clientInventoryCaseDefinition{
			id:      moveCraftingEncodeCaseID,
			family:  moveCraftingFamily,
			relDir:  moveCraftingCorpusRelDir,
			op:      "encode",
			request: clientInventoryMoveRequest{Sequence: 0, From: 8, To: 44},
			wire:    append([]byte(nil), clientInventoryCraftingMoveWire...),
			expect:  Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: craftingFields},
		},
		clientInventoryCaseDefinition{
			id:     moveCraftingBothInventoryID,
			family: moveCraftingFamily,
			relDir: moveCraftingCorpusRelDir,
			op:     "decode",
			input:  mutatedSlot(clientInventoryCraftingMoveWire, 8, 0x09),
			expect: invalid,
		},
		clientInventoryCaseDefinition{
			id:     moveCraftingSameSlotDecodeID,
			family: moveCraftingFamily,
			relDir: moveCraftingCorpusRelDir,
			op:     "decode",
			input:  mutatedSlot(clientInventoryCraftingMoveWire, 9, 0x08),
			expect: invalid,
		},
		clientInventoryCaseDefinition{
			id:     moveCraftingAboveViewID,
			family: moveCraftingFamily,
			relDir: moveCraftingCorpusRelDir,
			op:     "decode",
			input:  mutatedSlot(clientInventoryCraftingMoveWire, 9, 0x2d),
			expect: invalid,
		},
		clientInventoryCaseDefinition{
			id:      moveCraftingBothInventoryEncID,
			family:  moveCraftingFamily,
			relDir:  moveCraftingCorpusRelDir,
			op:      "encode",
			request: clientInventoryMoveRequest{Sequence: 0, From: 9, To: 44},
			expect:  invalid,
		},
	)

	// The three total sequence-only families each publish a valid pair plus the
	// two structural boundaries: a 7-byte prefix of the canonical payload and
	// one trailing byte. Their field set is the sequence alone, and the payload
	// purity pin is the exact 8-byte literal.
	sequenceOnly := []struct {
		family   string
		relDir   string
		wire     []byte
		fields   map[string]any
		decodeID string
		encodeID string
		truncID  string
		trailID  string
	}{
		{
			closeContainerFamily,
			closeContainerCorpusRelDir,
			clientInventorySequenceWire,
			zeroSequenceFields,
			closeContainerDecodeCaseID,
			closeContainerEncodeCaseID,
			closeContainerTruncatedID,
			closeContainerTrailingID,
		},
		{
			dropSelectedFamily,
			dropSelectedCorpusRelDir,
			clientInventorySequenceWire,
			zeroSequenceFields,
			dropSelectedDecodeCaseID,
			dropSelectedEncodeCaseID,
			dropSelectedTruncatedID,
			dropSelectedTrailingID,
		},
		{
			equipArmorFamily,
			equipArmorCorpusRelDir,
			clientInventorySequenceWire,
			zeroSequenceFields,
			equipArmorDecodeCaseID,
			equipArmorEncodeCaseID,
			equipArmorTruncatedID,
			equipArmorTrailingID,
		},
	}
	for _, family := range sequenceOnly {
		definitions = append(definitions,
			clientInventoryCaseDefinition{
				id:     family.decodeID,
				family: family.family,
				relDir: family.relDir,
				op:     "decode",
				input:  append([]byte(nil), family.wire...),
				expect: Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: family.fields},
			},
			clientInventoryCaseDefinition{
				id:      family.encodeID,
				family:  family.family,
				relDir:  family.relDir,
				op:      "encode",
				request: clientInventorySequenceRequest{Sequence: 0},
				wire:    append([]byte(nil), family.wire...),
				expect:  Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: family.fields},
			},
			clientInventoryCaseDefinition{
				id:     family.truncID,
				family: family.family,
				relDir: family.relDir,
				op:     "decode",
				input:  family.wire[:len(family.wire)-1],
				expect: truncated,
			},
			clientInventoryCaseDefinition{
				id:     family.trailID,
				family: family.family,
				relDir: family.relDir,
				op:     "decode",
				input:  append(append([]byte(nil), family.wire...), 0x00),
				expect: trailing,
			},
		)
	}

	// TakeCraftingOutput carries the same sequence-only shape with one extra
	// boundary: its zero sequence is refused even though every other command
	// accepts it, so the canonical vector is sequence 1.
	definitions = append(definitions,
		clientInventoryCaseDefinition{
			id:     takeCraftingDecodeCaseID,
			family: takeCraftingOutputFamily,
			relDir: takeCraftingOutputCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientInventoryTakeWire...),
			expect: Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: takeFields},
		},
		clientInventoryCaseDefinition{
			id:      takeCraftingEncodeCaseID,
			family:  takeCraftingOutputFamily,
			relDir:  takeCraftingOutputCorpusRelDir,
			op:      "encode",
			request: clientInventorySequenceRequest{Sequence: 1},
			wire:    append([]byte(nil), clientInventoryTakeWire...),
			expect:  Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: takeFields},
		},
		clientInventoryCaseDefinition{
			id:     takeCraftingZeroDecodeID,
			family: takeCraftingOutputFamily,
			relDir: takeCraftingOutputCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientInventorySequenceWire...),
			expect: invalid,
		},
		clientInventoryCaseDefinition{
			id:      takeCraftingZeroEncodeID,
			family:  takeCraftingOutputFamily,
			relDir:  takeCraftingOutputCorpusRelDir,
			op:      "encode",
			request: clientInventorySequenceRequest{Sequence: 0},
			expect:  invalid,
		},
	)
	return definitions
}

// clientInventoryLabel renders one case's asset label from its identity.
func clientInventoryLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// buildClientInventoryCandidate builds one case's manifest entry and asset bytes from
// its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildClientInventoryCandidate(t *testing.T, definition clientInventoryCaseDefinition) clientInventoryCandidate {
	t.Helper()

	label := clientInventoryLabel(definition.id)
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
		Version:      clientInventoryVersion,
		Operation:    definition.op,
		PacketKey:    clientInventoryKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return clientInventoryCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// clientInventoryKeyPointer resolves one family's reviewed packet key for a case spec.
func clientInventoryKeyPointer(family string) *PacketKeySpec {
	key, owned := clientInventoryFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// clientInventoryCandidate is one reviewed case: its manifest specification, the
// exact asset bytes it publishes, and the expectation an independent execution
// has to reproduce.
type clientInventoryCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which the
	// producer's own bytes are compared against.
	Wire []byte
}

// clientInventoryCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func clientInventoryCandidates(t *testing.T, root string) []clientInventoryCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := clientInventoryCaseDefinitions()
	candidates := make([]clientInventoryCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildClientInventoryCandidate(t, definition)
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

// clientInventoryRegisteredCase reports whether the base manifest already carries one
// case identity, so a re-merge of an integrated candidate adds nothing.
func clientInventoryRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// clientInventorySelection is this group's registration: its cases, the Go sources
// its rules are read from, and the routes the six families execute.
func clientInventorySelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := clientInventoryCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range clientInventoryFamilies() {
		sources[family.id] = clientInventoryClientSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(clientInventoryFamilies())*2)
	for _, family := range clientInventoryFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: clientInventoryVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: clientInventoryVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  clientInventoryProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// clientInventoryClientSources lists the Go sources one inventory command family
// reads its rules from: the shared codec dispatch plus the family's own message
// file.
func clientInventoryClientSources(family string) []string {
	switch family {
	case moveInventoryFamily, moveCraftingFamily, takeCraftingOutputFamily:
		return []string{
			"packages/shared/network/codec/codec_client.go",
			"packages/shared/network/protocol/message_inventory.go",
		}
	case closeContainerFamily:
		return []string{
			"packages/shared/network/codec/codec_client.go",
			"packages/shared/network/protocol/message_container.go",
		}
	default:
		return []string{
			"packages/shared/network/codec/codec_client.go",
			"packages/shared/network/protocol/message_command.go",
		}
	}
}

// clientInventoryManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func clientInventoryManifest(t *testing.T, root string, candidates []clientInventoryCandidate) Inventory {
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
		if !clientInventoryOwnsFamily(family) {
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
		t.Fatalf("encode client inventory working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write client inventory working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load client inventory working manifest: %v", err)
	}
	return loaded
}

// clientInventoryOwnsFamily reports whether this group registers cases for one family.
func clientInventoryOwnsFamily(family string) bool {
	_, owned := clientInventoryFamilyKeys[family]
	return owned
}

// clientInventoryScratchRoot stages this group's candidate assets in a harness-owned
// temporary directory, because a corpus case has to resolve under the root the
// runner is given.
func clientInventoryScratchRoot(t *testing.T, root string, candidates []clientInventoryCandidate) string {
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

// clientInventoryCandidateByID resolves one candidate by its case identity.
func clientInventoryCandidateByID(t *testing.T, candidates []clientInventoryCandidate, id string) clientInventoryCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return clientInventoryCandidate{}
}

// clientInventoryObservation resolves one executed observation by its case identity.
func clientInventoryObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// clientInventoryExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var clientInventoryExportPublished bool

// clientInventoryCandidatesExport publishes the reviewed candidates and the complete
// merged manifest candidate through the existing external exporter and returns
// the published producer directory. An unset export variable publishes nothing
// and returns "", so an ordinary test run never writes outside its own temporary
// storage.
func clientInventoryCandidatesExport(t *testing.T, root string, candidates []clientInventoryCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if clientInventoryExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-client-inventory")
	}
	clientInventoryExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, clientInventoryProducerID, assets)
	if err != nil {
		t.Fatalf("export client inventory candidates: %v", err)
	}
	return published
}

// clientInventoryDecodeCaseIDs lists the six valid decode cases the producer pins.
func clientInventoryDecodeCaseIDs() []string {
	return []string{
		moveInventoryDecodeCaseID,
		moveCraftingDecodeCaseID,
		closeContainerDecodeCaseID,
		dropSelectedDecodeCaseID,
		equipArmorDecodeCaseID,
		takeCraftingDecodeCaseID,
	}
}

// clientInventoryEncodeCaseIDs lists the six valid encode cases the producer pins.
func clientInventoryEncodeCaseIDs() []string {
	return []string{
		moveInventoryEncodeCaseID,
		moveCraftingEncodeCaseID,
		closeContainerEncodeCaseID,
		dropSelectedEncodeCaseID,
		equipArmorEncodeCaseID,
		takeCraftingEncodeCaseID,
	}
}

// clientInventoryRejectedDecodeCaseIDs lists the malformed decode cases.
func clientInventoryRejectedDecodeCaseIDs() []string {
	return []string{
		moveInventoryFromAboveID,
		moveInventoryToAboveID,
		moveInventorySameSlotDecodeID,
		moveCraftingBothInventoryID,
		moveCraftingSameSlotDecodeID,
		moveCraftingAboveViewID,
		closeContainerTruncatedID,
		closeContainerTrailingID,
		dropSelectedTruncatedID,
		dropSelectedTrailingID,
		equipArmorTruncatedID,
		equipArmorTrailingID,
		takeCraftingZeroDecodeID,
	}
}

// clientInventoryRejectedEncodeCaseIDs lists the invalid encode cases.
func clientInventoryRejectedEncodeCaseIDs() []string {
	return []string{
		moveInventorySameSlotEncodeID,
		moveCraftingBothInventoryEncID,
		takeCraftingZeroEncodeID,
	}
}

// TestProtocolClientInventoryOracleDecodesEveryValidCase pins that the real Go
// decoder publishes the canonical fields for every family's valid decode case,
// with the sequence rendered as a decimal string rather than a number.
func TestProtocolClientInventoryOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientInventoryCandidates(t, root)

	for _, id := range clientInventoryDecodeCaseIDs() {
		candidate := clientInventoryCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientInventoryDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientInventoryDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientInventoryOracleEncodesCanonicalWire pins each encode producer
// against the reviewed wire literal and against the recorded digest.
func TestProtocolClientInventoryOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientInventoryCandidates(t, root)

	for _, id := range clientInventoryEncodeCaseIDs() {
		candidate := clientInventoryCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientInventoryEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientInventoryEncode(%s): %v", id, err)
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

// TestProtocolClientInventoryOracleRejectsMalformedCasesAtTheirBoundary pins that
// every malformed decode case and invalid encode case is refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolClientInventoryOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientInventoryCandidates(t, root)

	for _, id := range clientInventoryRejectedDecodeCaseIDs() {
		candidate := clientInventoryCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientInventoryDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientInventoryDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range clientInventoryRejectedEncodeCaseIDs() {
		candidate := clientInventoryCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientInventoryEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientInventoryEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientInventoryOracleExpectedFieldMutationFailsComparison pins that the
// recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded, and replacing the encoded digest
// fails comparison against the reviewed wire.
func TestProtocolClientInventoryOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientInventoryCandidates(t, root)

	decode := clientInventoryCandidateByID(t, candidates, moveInventoryDecodeCaseID)
	produced, _, err := runClientInventoryDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runClientInventoryDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	fields["to"] = uint8(34)
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated target slot compares equal to the produced outcome")
	}

	encode := clientInventoryCandidateByID(t, candidates, closeContainerEncodeCaseID)
	producedEncode, _, err := runClientInventoryEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runClientInventoryEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolClientInventoryOracleRoutesExecuteEveryCase executes this group's
// complete case set through the shared packet-case runner, once per case, and
// compares every observation with the reviewed expectation.
func TestProtocolClientInventoryOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := clientInventoryCandidates(t, root)
	manifest := clientInventoryManifest(t, root, candidates)
	staged := clientInventoryScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, clientInventoryCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := clientInventoryObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientInventoryOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so a
// case cannot claim coverage from its name alone.
func TestProtocolClientInventoryOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientInventoryCandidates(t, root)
	manifest := clientInventoryManifest(t, root, candidates)
	staged := clientInventoryScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range clientInventoryFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: clientInventoryVersion, Operation: "decode"}] = runClientInventoryDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range clientInventoryFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+clientInventoryVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolClientInventoryOracleManifestMergeRegistersClientInventoryRoutes pins that
// the merged manifest registers all six families' routes and case lists, leaves
// the source revision alone, and records this group's provenance sources.
func TestProtocolClientInventoryOracleManifestMergeRegistersClientInventoryRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, clientInventorySelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range clientInventoryFamilies() {
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
		for _, want := range clientInventoryClientSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so a
	// re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range clientInventoryCandidates(t, root) {
		if !clientInventoryRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolClientInventoryOracleCandidatesExportForReview publishes the reviewed
// candidates and the manifest candidate. An unset export variable publishes
// nothing, so the tracked corpus is never written by this package.
func TestProtocolClientInventoryOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := clientInventoryCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), clientInventorySelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := clientInventoryCandidatesExport(t, root, candidates, merged)
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
