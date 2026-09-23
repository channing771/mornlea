package main

// This file is the container and view-addressed stack command packet producer
// group: the four Play client-to-server families that address a container
// (`MoveContainerStack`) or a unified view slot (`MoveStackPartial`,
// `QuickMoveStack` and `DropStack`) each register a decode and an encode route
// through the shared packet-case runner, so every case is executed by the real
// Go codec rather than restated here.
//
// The decode cases are the Go decoder's own bytes: the valid payload, the
// reference or index the validators refuse, and the enum values outside the
// closed sets. The encode cases carry canonical JSON fields and the reviewed
// wire the Go encoder has to publish, or a DTO the Go outbound validator has to
// refuse. Every negative carries exactly one violation, because the producer
// resolves the category at the boundary that owns it and a doubly invalid
// record would be classified differently by the Go kind-first reference gate
// and the Rust dimension-first conversion. The input values themselves move no
// world state: the moved count, the transfer destination and the world drop
// position stay authority-owned, so no case in this group declares a session, a
// deadline or a send.

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

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	// moveContainerStackFamily is the play container move family this group registers.
	moveContainerStackFamily = "protocol.client.MoveContainerStack"
	// moveStackPartialFamily is the play partial stack move family this group registers.
	moveStackPartialFamily = "protocol.client.MoveStackPartial"
	// quickMoveStackFamily is the play quick move family this group registers.
	quickMoveStackFamily = "protocol.client.QuickMoveStack"
	// dropStackFamily is the play stack drop family this group registers.
	dropStackFamily = "protocol.client.DropStack"
	// clientStackViewsVersion is the protocol version every family is pinned to.
	clientStackViewsVersion = "45"
	// clientStackViewsProducerID is the exporter's producer identity for this
	// group's candidate assets and merged manifest.
	clientStackViewsProducerID = "runtime-oracle/protocol-client-stack-views"

	// moveContainerStackCorpusRelDir is the repository-relative directory
	// holding this family's committed case assets.
	moveContainerStackCorpusRelDir = corpusCasesRelDir + "/protocol/MoveContainerStack"
	// moveStackPartialCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	moveStackPartialCorpusRelDir = corpusCasesRelDir + "/protocol/MoveStackPartial"
	// quickMoveStackCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	quickMoveStackCorpusRelDir = corpusCasesRelDir + "/protocol/QuickMoveStack"
	// dropStackCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	dropStackCorpusRelDir = corpusCasesRelDir + "/protocol/DropStack"
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	moveContainerDecodeCaseID   = moveContainerStackFamily + "/" + clientStackViewsVersion + "/decode-valid"
	moveContainerEncodeCaseID   = moveContainerStackFamily + "/" + clientStackViewsVersion + "/encode-valid"
	moveContainerOutputDecodeID = moveContainerStackFamily + "/" + clientStackViewsVersion + "/decode-output-target"
	moveContainerOutputEncodeID = moveContainerStackFamily + "/" + clientStackViewsVersion + "/encode-output-target"
	moveContainerSameSlotID     = moveContainerStackFamily + "/" + clientStackViewsVersion + "/decode-same-slot"
	moveContainerChestIndexID   = moveContainerStackFamily + "/" + clientStackViewsVersion + "/decode-chest-index-above"
	moveContainerUnknownKindID  = moveContainerStackFamily + "/" + clientStackViewsVersion + "/decode-unknown-kind"
	moveContainerForeignDimID   = moveContainerStackFamily + "/" + clientStackViewsVersion + "/decode-foreign-dimension"
	moveContainerZeroGenID      = moveContainerStackFamily + "/" + clientStackViewsVersion + "/decode-zero-generation"
	moveContainerRefSlotAboveID = moveContainerStackFamily + "/" + clientStackViewsVersion + "/decode-ref-slot-above-range"
	movePartialDecodeCaseID     = moveStackPartialFamily + "/" + clientStackViewsVersion + "/decode-valid"
	movePartialEncodeCaseID     = moveStackPartialFamily + "/" + clientStackViewsVersion + "/encode-valid"
	movePartialUnknownViewID    = moveStackPartialFamily + "/" + clientStackViewsVersion + "/decode-unknown-view"
	movePartialNonzeroRefID     = moveStackPartialFamily + "/" + clientStackViewsVersion + "/decode-nonzero-ref-inventory-view"
	movePartialNonzeroRefEncID  = moveStackPartialFamily + "/" + clientStackViewsVersion + "/encode-nonzero-ref-inventory-view"
	movePartialZeroGenViewID    = moveStackPartialFamily + "/" + clientStackViewsVersion + "/decode-zero-generation-container-view"
	movePartialSingleTagID      = moveStackPartialFamily + "/" + clientStackViewsVersion + "/decode-single-tag-two"
	movePartialSameSlotID       = moveStackPartialFamily + "/" + clientStackViewsVersion + "/decode-same-slot"
	movePartialFurnaceIndexID   = moveStackPartialFamily + "/" + clientStackViewsVersion + "/decode-furnace-index-above"
	quickMoveDecodeCaseID       = quickMoveStackFamily + "/" + clientStackViewsVersion + "/decode-valid"
	quickMoveEncodeCaseID       = quickMoveStackFamily + "/" + clientStackViewsVersion + "/encode-valid"
	quickMoveUnknownViewID      = quickMoveStackFamily + "/" + clientStackViewsVersion + "/decode-unknown-view"
	quickMoveNonzeroRefID       = quickMoveStackFamily + "/" + clientStackViewsVersion + "/decode-nonzero-ref-crafting-view"
	quickMoveCraftingIndexID    = quickMoveStackFamily + "/" + clientStackViewsVersion + "/decode-crafting-index-above"
	dropStackDecodeCaseID       = dropStackFamily + "/" + clientStackViewsVersion + "/decode-valid"
	dropStackEncodeCaseID       = dropStackFamily + "/" + clientStackViewsVersion + "/encode-valid"
	dropStackUnknownViewID      = dropStackFamily + "/" + clientStackViewsVersion + "/decode-unknown-view"
	dropStackNonzeroRefID       = dropStackFamily + "/" + clientStackViewsVersion + "/decode-nonzero-ref-inventory-view"
	dropStackInventoryIndexID   = dropStackFamily + "/" + clientStackViewsVersion + "/decode-inventory-index-above"
)

// clientStackViewsFamilyKeys pins each family's complete packet key. A case
// names its key instead of trusting its family, so a case registered under the
// wrong family, state or ID fails before the codec runs.
var clientStackViewsFamilyKeys = map[string]PacketKeySpec{
	moveContainerStackFamily: {Direction: packetDirectionClient, State: packetStatePlay, ID: 9},
	moveStackPartialFamily:   {Direction: packetDirectionClient, State: packetStatePlay, ID: 19},
	quickMoveStackFamily:     {Direction: packetDirectionClient, State: packetStatePlay, ID: 20},
	dropStackFamily:          {Direction: packetDirectionClient, State: packetStatePlay, ID: 21},
}

// clientStackViewsFamilies lists the four families in registry order, with the
// corpus directory that holds each family's assets. The order is the review
// order, not a dispatch table.
func clientStackViewsFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{moveContainerStackFamily, moveContainerStackCorpusRelDir},
		{moveStackPartialFamily, moveStackPartialCorpusRelDir},
		{quickMoveStackFamily, quickMoveStackCorpusRelDir},
		{dropStackFamily, dropStackCorpusRelDir},
	}
}

// clientStackViewsZeroSequence is the reviewed 8-byte zero sequence every
// canonical payload leads with.
var clientStackViewsZeroSequence = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// clientStackViewsFurnaceRefBytes is the reviewed furnace reference: overworld
// dimension, chunk −1/2, kind 0, physical slot 31 and generation 1.
var clientStackViewsFurnaceRefBytes = []byte{
	0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff,
	0x02, 0x00, 0x00, 0x00, 0x00, 0x1f, 0x01, 0x00, 0x00, 0x00,
}

// clientStackViewsChestRefBytes is the reviewed chest reference: the same chunk
// column with kind 1, physical slot 15 and generation 1.
var clientStackViewsChestRefBytes = []byte{
	0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff,
	0x02, 0x00, 0x00, 0x00, 0x01, 0x0f, 0x01, 0x00, 0x00, 0x00,
}

// clientStackViewsNoneRefBytes is the exact all-zero absent reference the
// inventory and crafting views carry.
var clientStackViewsNoneRefBytes = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// clientStackViewsMoveWire is the reviewed wire literal the MoveContainerStack
// canonical vector pins: sequence 0, the furnace reference, source slot 0 and
// target slot 37, one slot below the furnace output slot it may not target.
var clientStackViewsMoveWire = append(
	append(append([]byte(nil), clientStackViewsZeroSequence...), clientStackViewsFurnaceRefBytes...),
	0x00, 0x25,
)

// clientStackViewsPartialWire is the reviewed wire literal the MoveStackPartial
// canonical vector pins: the chest reference in the container view, source slot
// 36, target slot 62 and the single-item flag set.
var clientStackViewsPartialWire = append(
	append(append([]byte(nil), clientStackViewsZeroSequence...), clientStackViewsChestRefBytes...),
	0x02, 0x24, 0x3e, 0x01,
)

// clientStackViewsQuickWire is the reviewed wire literal the QuickMoveStack
// canonical vector pins: the absent reference in the crafting view and source
// slot 44.
var clientStackViewsQuickWire = append(
	append(append([]byte(nil), clientStackViewsZeroSequence...), clientStackViewsNoneRefBytes...),
	0x01, 0x2c,
)

// clientStackViewsDropWire is the reviewed wire literal the DropStack canonical
// vector pins: the absent reference in the inventory view and slot 35.
var clientStackViewsDropWire = append(
	append(append([]byte(nil), clientStackViewsZeroSequence...), clientStackViewsNoneRefBytes...),
	0x00, 0x23,
)

// clientStackViewsRefWire renders one container reference in the wire's field
// order, so a negative case can replace the reference bytes of a valid payload
// without hand-writing the 18 bytes again.
func clientStackViewsRefWire(dimension, chunkX, chunkZ int32, kind, slot uint8, generation uint32) []byte {
	wire := make([]byte, 0, 18)
	for _, value := range []int32{dimension, chunkX, chunkZ} {
		wire = append(wire, byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
	}
	wire = append(wire, kind, slot)
	wire = append(wire,
		byte(generation), byte(generation>>8), byte(generation>>16), byte(generation>>24),
	)
	return wire
}

// clientStackViewsWithRef replaces the reference bytes of one payload, leaving
// every other field in place.
func clientStackViewsWithRef(source []byte, reference []byte) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[8:26], reference)
	return wire
}

// clientStackViewsWithByte replaces one byte of a payload at offset.
func clientStackViewsWithByte(source []byte, offset int, value uint8) []byte {
	wire := append([]byte(nil), source...)
	wire[offset] = value
	return wire
}

// clientStackViewsRejectionCategory resolves the language-neutral rejection
// category for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and the
// outbound validators name, never over a sentinel identity, and a failure with
// no mapping is a hard error. The primitive answer precedes the validator
// message, because the decoder rejects a short or over-long payload while
// reading and only then hands the record to `Validate`. The two enum
// boundaries — an unknown container kind and an unknown stack split view, plus
// the decoder's boolean tag — resolve before the value boundaries, because the
// Go reference gate checks the kind before any other field.
func clientStackViewsRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "unknown container kind"),
		strings.Contains(message, "unknown stack split view"),
		strings.Contains(message, "invalid boolean"):
		return "invalid-enum", true
	case strings.Contains(message, "dimension is not overworld"),
		strings.Contains(message, "slot is outside"),
		strings.Contains(message, "generation is zero"),
		strings.Contains(message, "carries a container ref"),
		strings.Contains(message, "source equals target"),
		strings.Contains(message, "cannot be a move target"):
		return "invalid-value", true
	}
	return "", false
}

// clientStackViewsPacketKey resolves one packet key to the Go state and numeric
// ID the codec dispatches on, and reports whether the packet travels
// client-to-server.
func clientStackViewsPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := clientStackViewsFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no client stack view producer owns", c.ID, c.Family)
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

// clientStackViewsRefFields renders the nested container reference object every
// stack-view field set publishes: the raw wire integers, so a negative chunk
// coordinate and a dimension the authority refuses survive the round trip.
func clientStackViewsRefFields(ref core.ContainerRef) map[string]any {
	return map[string]any{
		"dimension":  int32(ref.Dimension),
		"chunk_x":    ref.Chunk.X,
		"chunk_z":    ref.Chunk.Z,
		"kind":       uint8(ref.Kind),
		"slot":       ref.Slot,
		"generation": ref.Generation,
	}
}

// clientStackViewsFields renders the semantic fields one container move DTO
// publishes.
func clientStackViewsFields(sequence uint64, container core.ContainerRef, from, to uint8) map[string]any {
	return map[string]any{
		"sequence":  strconv.FormatUint(sequence, 10),
		"container": clientStackViewsRefFields(container),
		"from":      from,
		"to":        to,
	}
}

// clientStackViewsPartialFields renders the semantic fields one partial move
// DTO publishes, which add the view byte and the single-item flag.
func clientStackViewsPartialFields(sequence uint64, container core.ContainerRef, view, from, to uint8, single bool) map[string]any {
	return map[string]any{
		"sequence":  strconv.FormatUint(sequence, 10),
		"container": clientStackViewsRefFields(container),
		"view":      view,
		"from":      from,
		"to":        to,
		"single":    single,
	}
}

// clientStackViewsIndexFields renders the semantic fields one view-addressed
// DTO with a single index publishes, naming that index with the field the
// family carries.
func clientStackViewsIndexFields(indexName string, sequence uint64, container core.ContainerRef, view, index uint8) map[string]any {
	return map[string]any{
		"sequence":  strconv.FormatUint(sequence, 10),
		"container": clientStackViewsRefFields(container),
		"view":      view,
		indexName:   index,
	}
}

// newClientStackViewsCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newClientStackViewsCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// clientStackViewsFieldsForPacket renders the semantic fields of one decoded
// stack-view packet, dispatching on the concrete DTO the production decoder
// returned.
func clientStackViewsFieldsForPacket(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.MoveContainerStack:
		return clientStackViewsFields(message.Sequence, message.Container, message.From, message.To), nil
	case protocol.MoveStackPartial:
		return clientStackViewsPartialFields(message.Sequence, message.Container, message.View, message.From, message.To, message.Single), nil
	case protocol.QuickMoveStack:
		return clientStackViewsIndexFields("from", message.Sequence, message.Container, message.View, message.From), nil
	case protocol.DropStack:
		return clientStackViewsIndexFields("slot", message.Sequence, message.Container, message.View, message.Slot), nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// runClientStackViewsDecode executes one stack-view command decode case through
// the real Go decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeClient` and classifies
// the failure it returns. It never reimplements the length-prefix, reference or
// index rules, so the recorded outcome is whatever the production codec decides
// about these exact bytes.
func runClientStackViewsDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := clientStackViewsPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newClientStackViewsCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeClient(state, packetID, input)
	if err != nil {
		category, classified := clientStackViewsRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := clientStackViewsFieldsForPacket(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// clientStackViewsRefRequest is the canonical JSON container reference one
// encode case carries. Every field is a plain JSON integer, and the dimension
// and the two chunk coordinates may be negative.
type clientStackViewsRefRequest struct {
	Dimension  int32  `json:"dimension"`
	ChunkX     int32  `json:"chunk_x"`
	ChunkZ     int32  `json:"chunk_z"`
	Kind       uint8  `json:"kind"`
	Slot       uint8  `json:"slot"`
	Generation uint32 `json:"generation"`
}

// clientStackViewsMoveRequest is the canonical JSON field input one container
// move encode case carries.
type clientStackViewsMoveRequest struct {
	Sequence  uint64                     `json:"sequence"`
	Container clientStackViewsRefRequest `json:"container"`
	From      uint8                      `json:"from"`
	To        uint8                      `json:"to"`
}

// clientStackViewsPartialRequest is the canonical JSON field input one partial
// move encode case carries.
type clientStackViewsPartialRequest struct {
	Sequence  uint64                     `json:"sequence"`
	Container clientStackViewsRefRequest `json:"container"`
	View      uint8                      `json:"view"`
	From      uint8                      `json:"from"`
	To        uint8                      `json:"to"`
	Single    bool                       `json:"single"`
}

// clientStackViewsQuickRequest is the canonical JSON field input one quick move
// encode case carries: the destination is a server-derived contract, so the
// wire names a source slot only.
type clientStackViewsQuickRequest struct {
	Sequence  uint64                     `json:"sequence"`
	Container clientStackViewsRefRequest `json:"container"`
	View      uint8                      `json:"view"`
	From      uint8                      `json:"from"`
}

// clientStackViewsDropRequest is the canonical JSON field input one stack drop
// encode case carries.
type clientStackViewsDropRequest struct {
	Sequence  uint64                     `json:"sequence"`
	Container clientStackViewsRefRequest `json:"container"`
	View      uint8                      `json:"view"`
	Slot      uint8                      `json:"slot"`
}

// clientStackViewsFurnaceRef is the reviewed furnace reference JSON fields.
func clientStackViewsFurnaceRef() clientStackViewsRefRequest {
	return clientStackViewsRefRequest{Dimension: 0, ChunkX: -1, ChunkZ: 2, Kind: 0, Slot: 31, Generation: 1}
}

// clientStackViewsChestRef is the reviewed chest reference JSON fields.
func clientStackViewsChestRef() clientStackViewsRefRequest {
	return clientStackViewsRefRequest{Dimension: 0, ChunkX: -1, ChunkZ: 2, Kind: 1, Slot: 15, Generation: 1}
}

// clientStackViewsNoneRef is the exact all-zero absent reference JSON fields.
func clientStackViewsNoneRef() clientStackViewsRefRequest {
	return clientStackViewsRefRequest{}
}

// runClientStackViewsEncode executes one stack-view command encode case through
// the real Go encoder named by the case's own packet key and reads the result
// back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects must
// never be recorded as evidence.
func runClientStackViewsEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := clientStackViewsPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newClientStackViewsCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet protocol.ClientPacket
	switch c.Family {
	case moveContainerStackFamily:
		var request clientStackViewsMoveRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.MoveContainerStack{
			Sequence: request.Sequence,
			Container: core.ContainerRef{
				Dimension:  core.DimensionID(request.Container.Dimension),
				Chunk:      core.ChunkPos{X: request.Container.ChunkX, Z: request.Container.ChunkZ},
				Kind:       core.ContainerKind(request.Container.Kind),
				Slot:       request.Container.Slot,
				Generation: request.Container.Generation,
			},
			From: request.From,
			To:   request.To,
		}
	case moveStackPartialFamily:
		var request clientStackViewsPartialRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.MoveStackPartial{
			Sequence:  request.Sequence,
			Container: clientStackViewsCoreRef(request.Container),
			View:      request.View,
			From:      request.From,
			To:        request.To,
			Single:    request.Single,
		}
	case quickMoveStackFamily:
		var request clientStackViewsQuickRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.QuickMoveStack{
			Sequence:  request.Sequence,
			Container: clientStackViewsCoreRef(request.Container),
			View:      request.View,
			From:      request.From,
		}
	case dropStackFamily:
		var request clientStackViewsDropRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.DropStack{
			Sequence:  request.Sequence,
			Container: clientStackViewsCoreRef(request.Container),
			View:      request.View,
			Slot:      request.Slot,
		}
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}

	encodedID, payload, err := wireCodec.EncodeClient(state, packet)
	if err != nil {
		category, classified := clientStackViewsRejectionCategory(err)
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
	fields, err := clientStackViewsFieldsForPacket(c, decoded)
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

// clientStackViewsCoreRef renders one JSON reference as the Go domain value the
// production DTO carries.
func clientStackViewsCoreRef(request clientStackViewsRefRequest) core.ContainerRef {
	return core.ContainerRef{
		Dimension:  core.DimensionID(request.Dimension),
		Chunk:      core.ChunkPos{X: request.ChunkX, Z: request.ChunkZ},
		Kind:       core.ContainerKind(request.Kind),
		Slot:       request.Slot,
		Generation: request.Generation,
	}
}

// clientStackViewsCorpusRoutes is the closed route map the four families
// execute.
func clientStackViewsCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range clientStackViewsFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: clientStackViewsVersion, Operation: "decode"}] = runClientStackViewsDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: clientStackViewsVersion, Operation: "encode"}] = runClientStackViewsEncode
	}
	return routes
}

// clientStackViewsCaseDefinition declares one case from literals before any
// producer runs, so the expectation is the review contract rather than a
// producer result.
type clientStackViewsCaseDefinition struct {
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

// clientStackViewsCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. Each negative
// carries exactly one violation: a reference mutation changes a single field of
// a valid payload, an index mutation changes a single byte, and no case
// combines an unknown kind with a nonzero dimension, because Go's reference
// gate answers the kind first while the Rust conversion answers the dimension
// first and the two would publish different categories for the same bytes.
func clientStackViewsCaseDefinitions() []clientStackViewsCaseDefinition {
	moveFields := map[string]any{
		"sequence":  "0",
		"container": clientStackViewsRefFields(clientStackViewsCoreRef(clientStackViewsFurnaceRef())),
		"from":      uint8(0),
		"to":        uint8(37),
	}
	partialFields := map[string]any{
		"sequence":  "0",
		"container": clientStackViewsRefFields(clientStackViewsCoreRef(clientStackViewsChestRef())),
		"view":      uint8(2),
		"from":      uint8(36),
		"to":        uint8(62),
		"single":    true,
	}
	quickFields := map[string]any{
		"sequence":  "0",
		"container": clientStackViewsRefFields(clientStackViewsCoreRef(clientStackViewsNoneRef())),
		"view":      uint8(1),
		"from":      uint8(44),
	}
	dropFields := map[string]any{
		"sequence":  "0",
		"container": clientStackViewsRefFields(clientStackViewsCoreRef(clientStackViewsNoneRef())),
		"view":      uint8(0),
		"slot":      uint8(35),
	}
	invalid := Outcome{Kind: "error", Category: "invalid-value"}
	invalidEnum := Outcome{Kind: "error", Category: "invalid-enum"}

	definitions := make([]clientStackViewsCaseDefinition, 0, 29)
	definitions = append(definitions,
		clientStackViewsCaseDefinition{
			id:     moveContainerDecodeCaseID,
			family: moveContainerStackFamily,
			relDir: moveContainerStackCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientStackViewsMoveWire...),
			expect: Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: moveFields},
		},
		clientStackViewsCaseDefinition{
			id:      moveContainerEncodeCaseID,
			family:  moveContainerStackFamily,
			relDir:  moveContainerStackCorpusRelDir,
			op:      "encode",
			request: clientStackViewsMoveRequest{Sequence: 0, Container: clientStackViewsFurnaceRef(), From: 0, To: 37},
			wire:    append([]byte(nil), clientStackViewsMoveWire...),
			expect:  Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: moveFields},
		},
		clientStackViewsCaseDefinition{
			id:     moveContainerOutputDecodeID,
			family: moveContainerStackFamily,
			relDir: moveContainerStackCorpusRelDir,
			op:     "decode",
			input:  clientStackViewsWithByte(clientStackViewsMoveWire, 27, 0x26),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:      moveContainerOutputEncodeID,
			family:  moveContainerStackFamily,
			relDir:  moveContainerStackCorpusRelDir,
			op:      "encode",
			request: clientStackViewsMoveRequest{Sequence: 0, Container: clientStackViewsFurnaceRef(), From: 0, To: 38},
			expect:  invalid,
		},
		clientStackViewsCaseDefinition{
			id:     moveContainerSameSlotID,
			family: moveContainerStackFamily,
			relDir: moveContainerStackCorpusRelDir,
			op:     "decode",
			input:  clientStackViewsWithByte(clientStackViewsMoveWire, 27, 0x00),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:     moveContainerChestIndexID,
			family: moveContainerStackFamily,
			relDir: moveContainerStackCorpusRelDir,
			op:     "decode",
			input: clientStackViewsWithByte(
				clientStackViewsWithRef(clientStackViewsMoveWire, clientStackViewsChestRefBytes), 27, 0x3f,
			),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:     moveContainerUnknownKindID,
			family: moveContainerStackFamily,
			relDir: moveContainerStackCorpusRelDir,
			op:     "decode",
			input: clientStackViewsWithRef(clientStackViewsMoveWire,
				clientStackViewsRefWire(0, -1, 2, 0x02, 0, 1)),
			expect: invalidEnum,
		},
		clientStackViewsCaseDefinition{
			id:     moveContainerForeignDimID,
			family: moveContainerStackFamily,
			relDir: moveContainerStackCorpusRelDir,
			op:     "decode",
			input: clientStackViewsWithRef(clientStackViewsMoveWire,
				clientStackViewsRefWire(-1, -1, 2, 0x00, 31, 1)),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:     moveContainerZeroGenID,
			family: moveContainerStackFamily,
			relDir: moveContainerStackCorpusRelDir,
			op:     "decode",
			input: clientStackViewsWithRef(clientStackViewsMoveWire,
				clientStackViewsRefWire(0, -1, 2, 0x00, 31, 0)),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:     moveContainerRefSlotAboveID,
			family: moveContainerStackFamily,
			relDir: moveContainerStackCorpusRelDir,
			op:     "decode",
			input: clientStackViewsWithRef(clientStackViewsMoveWire,
				clientStackViewsRefWire(0, -1, 2, 0x00, 0x20, 1)),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:     movePartialDecodeCaseID,
			family: moveStackPartialFamily,
			relDir: moveStackPartialCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientStackViewsPartialWire...),
			expect: Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: partialFields},
		},
		clientStackViewsCaseDefinition{
			id:      movePartialEncodeCaseID,
			family:  moveStackPartialFamily,
			relDir:  moveStackPartialCorpusRelDir,
			op:      "encode",
			request: clientStackViewsPartialRequest{Sequence: 0, Container: clientStackViewsChestRef(), View: 2, From: 36, To: 62, Single: true},
			wire:    append([]byte(nil), clientStackViewsPartialWire...),
			expect:  Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: partialFields},
		},
		clientStackViewsCaseDefinition{
			id:     movePartialUnknownViewID,
			family: moveStackPartialFamily,
			relDir: moveStackPartialCorpusRelDir,
			op:     "decode",
			input:  clientStackViewsWithByte(clientStackViewsPartialWire, 26, 0x03),
			expect: invalidEnum,
		},
		clientStackViewsCaseDefinition{
			// The inventory view names the zero reference, so a real reference
			// beside it is the single violation; both indices stay inside the
			// inventory bound so no second rule fires.
			id:     movePartialNonzeroRefID,
			family: moveStackPartialFamily,
			relDir: moveStackPartialCorpusRelDir,
			op:     "decode",
			input: clientStackViewsWithByte(
				clientStackViewsWithRef(clientStackViewsPartialWire, clientStackViewsChestRefBytes),
				26, 0x00,
			),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:      movePartialNonzeroRefEncID,
			family:  moveStackPartialFamily,
			relDir:  moveStackPartialCorpusRelDir,
			op:      "encode",
			request: clientStackViewsPartialRequest{Sequence: 0, Container: clientStackViewsChestRef(), View: 0, From: 0, To: 35, Single: true},
			expect:  invalid,
		},
		clientStackViewsCaseDefinition{
			id:     movePartialZeroGenViewID,
			family: moveStackPartialFamily,
			relDir: moveStackPartialCorpusRelDir,
			op:     "decode",
			input: clientStackViewsWithRef(clientStackViewsPartialWire,
				clientStackViewsRefWire(0, -1, 2, 0x01, 15, 0)),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:     movePartialSingleTagID,
			family: moveStackPartialFamily,
			relDir: moveStackPartialCorpusRelDir,
			op:     "decode",
			input:  clientStackViewsWithByte(clientStackViewsPartialWire, 29, 0x02),
			expect: invalidEnum,
		},
		clientStackViewsCaseDefinition{
			id:     movePartialSameSlotID,
			family: moveStackPartialFamily,
			relDir: moveStackPartialCorpusRelDir,
			op:     "decode",
			input:  clientStackViewsWithByte(clientStackViewsPartialWire, 28, 0x24),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			// The furnace view bounds the index at 39, so source slot 39 is the
			// single violation; the target stays inside the bound and the
			// reference is a valid furnace identity.
			id:     movePartialFurnaceIndexID,
			family: moveStackPartialFamily,
			relDir: moveStackPartialCorpusRelDir,
			op:     "decode",
			input: clientStackViewsWithByte(
				clientStackViewsWithRef(clientStackViewsPartialWire, clientStackViewsFurnaceRefBytes),
				27, 0x27,
			),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:     quickMoveDecodeCaseID,
			family: quickMoveStackFamily,
			relDir: quickMoveStackCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientStackViewsQuickWire...),
			expect: Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: quickFields},
		},
		clientStackViewsCaseDefinition{
			id:      quickMoveEncodeCaseID,
			family:  quickMoveStackFamily,
			relDir:  quickMoveStackCorpusRelDir,
			op:      "encode",
			request: clientStackViewsQuickRequest{Sequence: 0, Container: clientStackViewsNoneRef(), View: 1, From: 44},
			wire:    append([]byte(nil), clientStackViewsQuickWire...),
			expect:  Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: quickFields},
		},
		clientStackViewsCaseDefinition{
			id:     quickMoveUnknownViewID,
			family: quickMoveStackFamily,
			relDir: quickMoveStackCorpusRelDir,
			op:     "decode",
			input:  clientStackViewsWithByte(clientStackViewsQuickWire, 26, 0x03),
			expect: invalidEnum,
		},
		clientStackViewsCaseDefinition{
			// The crafting view names the zero reference; the source slot stays
			// at 44, inside that view's bound.
			id:     quickMoveNonzeroRefID,
			family: quickMoveStackFamily,
			relDir: quickMoveStackCorpusRelDir,
			op:     "decode",
			input:  clientStackViewsWithRef(clientStackViewsQuickWire, clientStackViewsChestRefBytes),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:     quickMoveCraftingIndexID,
			family: quickMoveStackFamily,
			relDir: quickMoveStackCorpusRelDir,
			op:     "decode",
			input:  clientStackViewsWithByte(clientStackViewsQuickWire, 27, 0x2d),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:     dropStackDecodeCaseID,
			family: dropStackFamily,
			relDir: dropStackCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientStackViewsDropWire...),
			expect: Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: dropFields},
		},
		clientStackViewsCaseDefinition{
			id:      dropStackEncodeCaseID,
			family:  dropStackFamily,
			relDir:  dropStackCorpusRelDir,
			op:      "encode",
			request: clientStackViewsDropRequest{Sequence: 0, Container: clientStackViewsNoneRef(), View: 0, Slot: 35},
			wire:    append([]byte(nil), clientStackViewsDropWire...),
			expect:  Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: dropFields},
		},
		clientStackViewsCaseDefinition{
			id:     dropStackUnknownViewID,
			family: dropStackFamily,
			relDir: dropStackCorpusRelDir,
			op:     "decode",
			input:  clientStackViewsWithByte(clientStackViewsDropWire, 26, 0x03),
			expect: invalidEnum,
		},
		clientStackViewsCaseDefinition{
			// The inventory view names the zero reference; the slot stays at 35,
			// inside that view's bound.
			id:     dropStackNonzeroRefID,
			family: dropStackFamily,
			relDir: dropStackCorpusRelDir,
			op:     "decode",
			input:  clientStackViewsWithRef(clientStackViewsDropWire, clientStackViewsChestRefBytes),
			expect: invalid,
		},
		clientStackViewsCaseDefinition{
			id:     dropStackInventoryIndexID,
			family: dropStackFamily,
			relDir: dropStackCorpusRelDir,
			op:     "decode",
			input:  clientStackViewsWithByte(clientStackViewsDropWire, 27, 0x24),
			expect: invalid,
		},
	)

	return definitions
}

// clientStackViewsLabel renders one case's asset label from its identity.
func clientStackViewsLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// buildClientStackViewsCandidate builds one case's manifest entry and asset
// bytes from its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildClientStackViewsCandidate(t *testing.T, definition clientStackViewsCaseDefinition) clientStackViewsCandidate {
	t.Helper()

	label := clientStackViewsLabel(definition.id)
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
		Version:      clientStackViewsVersion,
		Operation:    definition.op,
		PacketKey:    clientStackViewsKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return clientStackViewsCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// clientStackViewsKeyPointer resolves one family's reviewed packet key for a
// case spec.
func clientStackViewsKeyPointer(family string) *PacketKeySpec {
	key, owned := clientStackViewsFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// clientStackViewsCandidate is one reviewed case: its manifest specification,
// the exact asset bytes it publishes, and the expectation an independent
// execution has to reproduce.
type clientStackViewsCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which the
	// producer's own bytes are compared against.
	Wire []byte
}

// clientStackViewsCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func clientStackViewsCandidates(t *testing.T, root string) []clientStackViewsCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := clientStackViewsCaseDefinitions()
	candidates := make([]clientStackViewsCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildClientStackViewsCandidate(t, definition)
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

// clientStackViewsRegisteredCase reports whether the base manifest already
// carries one case identity, so a re-merge of an integrated candidate adds
// nothing.
func clientStackViewsRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// clientStackViewsSelection is this group's registration: its cases, the Go
// sources its rules are read from, and the routes the four families execute.
func clientStackViewsSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := clientStackViewsCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range clientStackViewsFamilies() {
		sources[family.id] = clientStackViewsClientSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(clientStackViewsFamilies())*2)
	for _, family := range clientStackViewsFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: clientStackViewsVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: clientStackViewsVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  clientStackViewsProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// clientStackViewsClientSources lists the Go sources one stack-view command
// family reads its rules from: the shared codec dispatch, the shared reference
// validators, and the family's own message file.
func clientStackViewsClientSources(family string) []string {
	shared := []string{
		"packages/shared/network/codec/codec_client.go",
		"packages/shared/network/protocol/message_container.go",
	}
	switch family {
	case moveContainerStackFamily:
		return shared
	case moveStackPartialFamily, quickMoveStackFamily:
		return append(append([]string{}, shared...),
			"packages/shared/network/protocol/message_stack_splitting.go")
	case dropStackFamily:
		return append(append([]string{}, shared...),
			"packages/shared/network/protocol/message_drop_stack.go")
	default:
		return shared
	}
}

// clientStackViewsManifest assembles the family-scoped selection the route
// runner executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func clientStackViewsManifest(t *testing.T, root string, candidates []clientStackViewsCandidate) Inventory {
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
		if !clientStackViewsOwnsFamily(family) {
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
		t.Fatalf("encode client stack views working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write client stack views working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load client stack views working manifest: %v", err)
	}
	return loaded
}

// clientStackViewsOwnsFamily reports whether this group registers cases for one
// family.
func clientStackViewsOwnsFamily(family string) bool {
	_, owned := clientStackViewsFamilyKeys[family]
	return owned
}

// clientStackViewsScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve under
// the root the runner is given.
func clientStackViewsScratchRoot(t *testing.T, root string, candidates []clientStackViewsCandidate) string {
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

// clientStackViewsCandidateByID resolves one candidate by its case identity.
func clientStackViewsCandidateByID(t *testing.T, candidates []clientStackViewsCandidate, id string) clientStackViewsCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return clientStackViewsCandidate{}
}

// clientStackViewsObservation resolves one executed observation by its case
// identity.
func clientStackViewsObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// clientStackViewsExportPublished guards the single publication per test
// process, because the exporter's producer child is create-exclusive and more
// than one test in this package observes the same candidates.
var clientStackViewsExportPublished bool

// clientStackViewsCandidatesExport publishes the reviewed candidates and the
// complete merged manifest candidate through the existing external exporter and
// returns the published producer directory. An unset export variable publishes
// nothing and returns "", so an ordinary test run never writes outside its own
// temporary storage.
func clientStackViewsCandidatesExport(t *testing.T, root string, candidates []clientStackViewsCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if clientStackViewsExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-client-stack-views")
	}
	clientStackViewsExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, clientStackViewsProducerID, assets)
	if err != nil {
		t.Fatalf("export client stack views candidates: %v", err)
	}
	return published
}

// clientStackViewsDecodeCaseIDs lists the four valid decode cases the producer
// pins.
func clientStackViewsDecodeCaseIDs() []string {
	return []string{
		moveContainerDecodeCaseID,
		movePartialDecodeCaseID,
		quickMoveDecodeCaseID,
		dropStackDecodeCaseID,
	}
}

// clientStackViewsEncodeCaseIDs lists the four valid encode cases the producer
// pins.
func clientStackViewsEncodeCaseIDs() []string {
	return []string{
		moveContainerEncodeCaseID,
		movePartialEncodeCaseID,
		quickMoveEncodeCaseID,
		dropStackEncodeCaseID,
	}
}

// clientStackViewsRejectedDecodeCaseIDs lists the malformed decode cases.
func clientStackViewsRejectedDecodeCaseIDs() []string {
	return []string{
		moveContainerOutputDecodeID,
		moveContainerSameSlotID,
		moveContainerChestIndexID,
		moveContainerUnknownKindID,
		moveContainerForeignDimID,
		moveContainerZeroGenID,
		moveContainerRefSlotAboveID,
		movePartialUnknownViewID,
		movePartialNonzeroRefID,
		movePartialZeroGenViewID,
		movePartialSingleTagID,
		movePartialSameSlotID,
		movePartialFurnaceIndexID,
		quickMoveUnknownViewID,
		quickMoveNonzeroRefID,
		quickMoveCraftingIndexID,
		dropStackUnknownViewID,
		dropStackNonzeroRefID,
		dropStackInventoryIndexID,
	}
}

// clientStackViewsRejectedEncodeCaseIDs lists the invalid encode cases.
func clientStackViewsRejectedEncodeCaseIDs() []string {
	return []string{
		moveContainerOutputEncodeID,
		movePartialNonzeroRefEncID,
	}
}

// TestProtocolClientStackViewsOracleDecodesEveryValidCase pins that the real Go
// decoder publishes the canonical fields for every family's valid decode case,
// with the sequence rendered as a decimal string and the container reference as
// the nested raw object.
func TestProtocolClientStackViewsOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientStackViewsCandidates(t, root)

	for _, id := range clientStackViewsDecodeCaseIDs() {
		candidate := clientStackViewsCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientStackViewsDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientStackViewsDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientStackViewsOracleEncodesCanonicalWire pins each encode
// producer against the reviewed wire literal and against the recorded digest.
func TestProtocolClientStackViewsOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientStackViewsCandidates(t, root)

	for _, id := range clientStackViewsEncodeCaseIDs() {
		candidate := clientStackViewsCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientStackViewsEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientStackViewsEncode(%s): %v", id, err)
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

// TestProtocolClientStackViewsOracleRejectsMalformedCasesAtTheirBoundary pins
// that every malformed decode case and invalid encode case is refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolClientStackViewsOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientStackViewsCandidates(t, root)

	for _, id := range clientStackViewsRejectedDecodeCaseIDs() {
		candidate := clientStackViewsCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientStackViewsDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientStackViewsDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range clientStackViewsRejectedEncodeCaseIDs() {
		candidate := clientStackViewsCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientStackViewsEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientStackViewsEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientStackViewsOracleExpectedFieldMutationFailsComparison pins
// that the recorded expectation is a commitment: replacing an expected field
// fails comparison against what the producer decoded, and replacing the encoded
// digest fails comparison against the reviewed wire.
func TestProtocolClientStackViewsOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientStackViewsCandidates(t, root)

	decode := clientStackViewsCandidateByID(t, candidates, moveContainerDecodeCaseID)
	produced, _, err := runClientStackViewsDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runClientStackViewsDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	fields["to"] = uint8(36)
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated target slot compares equal to the produced outcome")
	}

	encode := clientStackViewsCandidateByID(t, candidates, moveContainerEncodeCaseID)
	producedEncode, _, err := runClientStackViewsEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runClientStackViewsEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolClientStackViewsOracleRoutesExecuteEveryCase executes this
// group's complete case set through the shared packet-case runner, once per
// case, and compares every observation with the reviewed expectation.
func TestProtocolClientStackViewsOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := clientStackViewsCandidates(t, root)
	manifest := clientStackViewsManifest(t, root, candidates)
	staged := clientStackViewsScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, clientStackViewsCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := clientStackViewsObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientStackViewsOracleRunnerRejectsUnregisteredRoute pins that a
// case naming a route this group does not claim fails before its producer runs,
// so a case cannot claim coverage from its name alone.
func TestProtocolClientStackViewsOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientStackViewsCandidates(t, root)
	manifest := clientStackViewsManifest(t, root, candidates)
	staged := clientStackViewsScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range clientStackViewsFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: clientStackViewsVersion, Operation: "decode"}] = runClientStackViewsDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range clientStackViewsFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+clientStackViewsVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolClientStackViewsOracleManifestMergeRegistersStackViewRoutes pins
// that the merged manifest registers all four families' routes and case lists,
// leaves the source revision alone, and records this group's provenance
// sources.
func TestProtocolClientStackViewsOracleManifestMergeRegistersStackViewRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, clientStackViewsSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range clientStackViewsFamilies() {
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
		for _, want := range clientStackViewsClientSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so a
	// re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range clientStackViewsCandidates(t, root) {
		if !clientStackViewsRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolClientStackViewsOracleCandidatesExportForReview publishes the
// reviewed candidates and the manifest candidate. An unset export variable
// publishes nothing, so the tracked corpus is never written by this package.
func TestProtocolClientStackViewsOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := clientStackViewsCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), clientStackViewsSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := clientStackViewsCandidatesExport(t, root, candidates, merged)
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
