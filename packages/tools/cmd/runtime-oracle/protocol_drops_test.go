package main

// This file is the item drop packet producer group: the two Play
// server-to-client families that publish the authoritative drops one
// subscriber can see (`ItemDropUpserts` and `ItemDropRemoves`) each register a
// decode and an encode route through the shared packet-case runner, so every
// case is executed by the real Go codec rather than restated here.
//
// The decode cases are the Go decoder's own bytes: the canonical payloads, the
// block-index, identity, stack and count boundaries, and one structural
// mutation each. The encode cases carry canonical JSON fields and either the
// reviewed wire the Go encoder has to publish or a DTO the outbound validator
// has to refuse. Every negative carries exactly one violation.
//
// The identity is the Go `core.DropID` and its wire layout is kept exactly:
// the dimension is the raw `i32` the validity rule never narrows, so the
// reviewed vectors carry dimensions −1 and 256 and both round-trip verbatim.
// The batch ordering compares the full identity key with the dimension first,
// so a −1-dimension record sorts before a 256-dimension one.
//
// The Go `ItemDrop.validate` rule is folded: an invalid identity, an
// out-of-range block index and an invalid stack each answer with their own
// message, but the stack message covers an unregistered item number, a count
// above the item's stack limit and a durability violation alike. The frozen
// corpus therefore records the count and stack-limit boundaries at the value
// boundary both sides publish, and the unregistered item number stays a Rust
// group-test pin because the folded message cannot publish a distinct
// category for it. The batch count message, the two order messages and the
// block-index message resolve at the value boundary; the identity messages
// resolve at the identity boundary; the end-of-payload check resolves at the
// trailing boundary, because both drop decoders apply the
// minimum-records rule and leave the remainder to it.

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
	// itemDropUpsertsFamily is the play item drop upsert family this group registers.
	itemDropUpsertsFamily = "protocol.server.ItemDropUpserts"
	// itemDropRemovesFamily is the play item drop remove family this group registers.
	itemDropRemovesFamily = "protocol.server.ItemDropRemoves"
	// itemDropsVersion is the protocol version both families are pinned to.
	itemDropsVersion = "45"
	// itemDropsProducerID is the exporter's producer identity for this
	// group's candidate assets and merged manifest.
	itemDropsProducerID = "runtime-oracle/protocol-drops"

	// itemDropUpsertsCorpusRelDir is the repository-relative directory
	// holding this family's committed case assets.
	itemDropUpsertsCorpusRelDir = corpusCasesRelDir + "/protocol/ItemDropUpserts"
	// itemDropRemovesCorpusRelDir is the repository-relative directory
	// holding this family's committed case assets.
	itemDropRemovesCorpusRelDir = corpusCasesRelDir + "/protocol/ItemDropRemoves"

	// dropIDWireBytes is the fixed identity stride.
	dropIDWireBytes = 17
	// itemDropWireBytes is the fixed record stride of one upsert: the
	// identity, the block index and the five-byte item stack.
	itemDropWireBytes = dropIDWireBytes + 4 + 5
	// itemDropsMaxRecords is the bounded drop set one batch may carry.
	itemDropsMaxRecords = protocol.MaxItemDropBatch
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	itemDropsUpsertsDecodeCaseID   = itemDropUpsertsFamily + "/" + itemDropsVersion + "/decode-valid"
	itemDropsUpsertsEncodeCaseID   = itemDropUpsertsFamily + "/" + itemDropsVersion + "/encode-valid"
	itemDropsUpsertsBlockIndexID   = itemDropUpsertsFamily + "/" + itemDropsVersion + "/decode-block-index-above"
	itemDropsUpsertsGenerationID   = itemDropUpsertsFamily + "/" + itemDropsVersion + "/decode-generation-zero"
	itemDropsUpsertsSlotAboveID    = itemDropUpsertsFamily + "/" + itemDropsVersion + "/decode-slot-above"
	itemDropsUpsertsCountLimitID   = itemDropUpsertsFamily + "/" + itemDropsVersion + "/decode-count-above-stack-limit"
	itemDropsUpsertsDuplicateID    = itemDropUpsertsFamily + "/" + itemDropsVersion + "/decode-duplicate-ids"
	itemDropsUpsertsReversedID     = itemDropUpsertsFamily + "/" + itemDropsVersion + "/decode-reversed-ids"
	itemDropsUpsertsEncodeBlockID  = itemDropUpsertsFamily + "/" + itemDropsVersion + "/encode-block-index-above"
	itemDropsUpsertsTrailingID     = itemDropUpsertsFamily + "/" + itemDropsVersion + "/decode-trailing-byte"
	itemDropsRemovesDecodeCaseID   = itemDropRemovesFamily + "/" + itemDropsVersion + "/decode-valid"
	itemDropsRemovesEncodeCaseID   = itemDropRemovesFamily + "/" + itemDropsVersion + "/encode-valid"
	itemDropsRemovesCountOneID     = itemDropRemovesFamily + "/" + itemDropsVersion + "/decode-count-one"
	itemDropsRemovesCountThirtyID  = itemDropRemovesFamily + "/" + itemDropsVersion + "/decode-count-thirty-two"
	itemDropsRemovesCountZeroID    = itemDropRemovesFamily + "/" + itemDropsVersion + "/decode-count-zero"
	itemDropsRemovesCountOverID    = itemDropRemovesFamily + "/" + itemDropsVersion + "/decode-count-thirty-three"
	itemDropsRemovesDuplicateID    = itemDropRemovesFamily + "/" + itemDropsVersion + "/decode-duplicate-ids"
	itemDropsRemovesReversedID     = itemDropRemovesFamily + "/" + itemDropsVersion + "/decode-reversed-ids"
	itemDropsRemovesBadGenerationI = itemDropRemovesFamily + "/" + itemDropsVersion + "/decode-bad-generation"
)

// itemDropFamilyKeys pins each family's complete packet key. A case names its
// key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs. Both families are
// server-to-client play packets.
var itemDropFamilyKeys = map[string]PacketKeySpec{
	itemDropUpsertsFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 11},
	itemDropRemovesFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 12},
}

// itemDropFamilies lists the two families in registry order, with the corpus
// directory that holds each family's assets. The order is the review order,
// not a dispatch table.
func itemDropFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{itemDropUpsertsFamily, itemDropUpsertsCorpusRelDir},
		{itemDropRemovesFamily, itemDropRemovesCorpusRelDir},
	}
}

// itemDropsU64 renders one little-endian u64 field.
func itemDropsU64(value uint64) []byte {
	wire := make([]byte, 8)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// itemDropsU32 renders one little-endian u32 field.
func itemDropsU32(value uint32) []byte {
	wire := make([]byte, 4)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// itemDropsI32 renders one little-endian i32 field, preserving the raw
// dimension verbatim so a −1 or 256 dimension survives the wire round trip.
func itemDropsI32(value int32) []byte {
	return itemDropsU32(uint32(value))
}

// itemDropsUvarint renders one canonical uvarint count prefix.
func itemDropsUvarint(value uint32) []byte {
	wire := make([]byte, 0, 5)
	for value >= 1<<7 {
		wire = append(wire, byte(value)|0x80)
		value >>= 7
	}
	return append(wire, byte(value))
}

// itemDropsIDWire renders one 17-byte identity in the wire field order: the
// raw dimension, the two chunk coordinates, the slot and the generation.
func itemDropsIDWire(id core.DropID) []byte {
	wire := make([]byte, 0, dropIDWireBytes)
	wire = append(wire, itemDropsI32(int32(id.Dimension))...)
	wire = append(wire, itemDropsI32(id.Chunk.X)...)
	wire = append(wire, itemDropsI32(id.Chunk.Z)...)
	wire = append(wire, id.Slot)
	return append(wire, itemDropsU32(id.Generation)...)
}

// itemDropsStackWire renders one five-byte item stack: the item, the count and
// the durability.
func itemDropsStackWire(item core.ItemID, count uint8, durability uint16) []byte {
	wire := make([]byte, 0, 5)
	wire = append(wire, byte(item), byte(uint16(item)>>8))
	wire = append(wire, count)
	return append(wire, byte(durability), byte(durability>>8))
}

// itemDropsUpsertWire renders one 26-byte upsert record.
func itemDropsUpsertWire(drop protocol.ItemDrop) []byte {
	wire := make([]byte, 0, itemDropWireBytes)
	wire = append(wire, itemDropsIDWire(drop.ID)...)
	wire = append(wire, itemDropsU32(drop.BlockIndex)...)
	return append(wire, itemDropsStackWire(drop.Item, drop.Count, drop.Durability)...)
}

// itemDropsUpsertsWire renders one upsert payload: the tick, the canonical
// uvarint count and the ordered records.
func itemDropsUpsertsWire(tick uint64, declared uint32, drops []protocol.ItemDrop) []byte {
	wire := append([]byte(nil), itemDropsU64(tick)...)
	wire = append(wire, itemDropsUvarint(declared)...)
	for _, drop := range drops {
		wire = append(wire, itemDropsUpsertWire(drop)...)
	}
	return wire
}

// itemDropsRemovesWire renders one remove payload: the tick, the canonical
// uvarint count and the ordered identities.
func itemDropsRemovesWire(tick uint64, declared uint32, ids []core.DropID) []byte {
	wire := append([]byte(nil), itemDropsU64(tick)...)
	wire = append(wire, itemDropsUvarint(declared)...)
	for _, id := range ids {
		wire = append(wire, itemDropsIDWire(id)...)
	}
	return wire
}

// itemDropsID builds one drop identity for the wire builders.
func itemDropsID(dimension int32, chunkX, chunkZ int32, slot uint8, generation uint32) core.DropID {
	return core.DropID{
		Dimension:  core.DimensionID(dimension),
		Chunk:      core.ChunkPos{X: chunkX, Z: chunkZ},
		Slot:       slot,
		Generation: generation,
	}
}

// itemDropsCanonicalUpsert is the reviewed first record: the raw dimension −1
// with the highest slot and generation the wire can carry, block index 0 and
// the ordinary stone stack.
func itemDropsCanonicalUpsert() protocol.ItemDrop {
	return protocol.ItemDrop{
		ID: itemDropsID(-1, 7, -3, 31, 0xFFFFFFFF), BlockIndex: 0,
		Item: core.ItemStone, Count: 4, Durability: 0,
	}
}

// itemDropsEmptyUpsert is the reviewed second record: dimension 256, block
// index 98303 (the inclusive upper bound of the chunk-ordered block index
// minus one) and the exact empty stack triple.
func itemDropsEmptyUpsert() protocol.ItemDrop {
	return protocol.ItemDrop{
		ID: itemDropsID(256, -4, 9, 0, 1), BlockIndex: 98303,
		Item: core.ItemNone, Count: 0, Durability: 0,
	}
}

// itemDropsCanonicalUpserts is the reviewed two-record canonical batch.
func itemDropsCanonicalUpserts() protocol.ItemDropUpserts {
	return protocol.ItemDropUpserts{
		ServerTick: 0,
		Drops: []protocol.ItemDrop{
			itemDropsCanonicalUpsert(),
			itemDropsEmptyUpsert(),
		},
	}
}

// itemDropsCanonicalRemoves is the reviewed one-record canonical batch.
func itemDropsCanonicalRemoves() protocol.ItemDropRemoves {
	return protocol.ItemDropRemoves{
		ServerTick: 0,
		IDs:        []core.DropID{itemDropsID(0, 1, -2, 3, 7)},
	}
}

// itemDropsFullRemoves is the reviewed full batch of thirty-two identities
// ordered by ascending raw dimension.
func itemDropsFullRemoves() protocol.ItemDropRemoves {
	ids := make([]core.DropID, 0, itemDropsMaxRecords)
	for dimension := int32(0); dimension < itemDropsMaxRecords; dimension++ {
		ids = append(ids, itemDropsID(dimension, 0, 0, 0, 1))
	}
	return protocol.ItemDropRemoves{ServerTick: 0, IDs: ids}
}

// itemDropsIDFields renders one drop identity as its ordered five-field
// object. The dimension publishes as the plain wire integer, verbatim and
// never narrowed.
func itemDropsIDFields(id core.DropID) map[string]any {
	return map[string]any{
		"dimension":  int32(id.Dimension),
		"chunk_x":    id.Chunk.X,
		"chunk_z":    id.Chunk.Z,
		"slot":       id.Slot,
		"generation": id.Generation,
	}
}

// itemDropsUpsertFields renders one drop record's semantic fields.
func itemDropsUpsertFields(drop protocol.ItemDrop) map[string]any {
	return map[string]any{
		"id": itemDropsIDFields(drop.ID),
		"stack": map[string]any{
			"item":       int32(drop.Item),
			"count":      drop.Count,
			"durability": drop.Durability,
		},
		"block_index": drop.BlockIndex,
	}
}

// itemDropsUpsertsFields renders one upsert batch's semantic fields.
//
// The records publish in wire order, never sorted, so a batch the authority
// ordered is observed in the order it carried.
func itemDropsUpsertsFields(upserts protocol.ItemDropUpserts) map[string]any {
	drops := make([]map[string]any, 0, len(upserts.Drops))
	for _, drop := range upserts.Drops {
		drops = append(drops, itemDropsUpsertFields(drop))
	}
	return map[string]any{
		"server_tick": strconv.FormatUint(upserts.ServerTick, 10),
		"drops":       drops,
	}
}

// itemDropsRemovesFields renders one remove batch's semantic fields.
func itemDropsRemovesFields(removes protocol.ItemDropRemoves) map[string]any {
	ids := make([]map[string]any, 0, len(removes.IDs))
	for _, id := range removes.IDs {
		ids = append(ids, itemDropsIDFields(id))
	}
	return map[string]any{
		"server_tick": strconv.FormatUint(removes.ServerTick, 10),
		"ids":         ids,
	}
}

// itemDropsRejectionCategory resolves the language-neutral rejection category
// for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and
// the outbound validators name, never over a sentinel identity, and a failure
// with no mapping is a hard error. The identity messages resolve at the
// identity boundary, the folded stack message at the value boundary, and the
// count, order, block-index and trailing boundaries at the boundaries the
// Rust consumer publishes for the same bytes.
func itemDropsRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "item drop batch count is outside 1..32"):
		return "invalid-value", true
	case strings.Contains(message, "are not strictly sorted"):
		return "invalid-value", true
	case strings.Contains(message, "item drop block index is outside the chunk"):
		return "invalid-value", true
	case strings.Contains(message, "invalid item drop stack"):
		return "invalid-value", true
	case strings.Contains(message, "invalid item drop ID"):
		return "invalid-identity", true
	case strings.Contains(message, "invalid ID"):
		return "invalid-identity", true
	}
	return "", false
}

// itemDropsPacketKey resolves one packet key to the Go state and numeric ID
// the codec dispatches on, and reports whether the packet travels
// server-to-client.
func itemDropsPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := itemDropFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no item drop producer owns", c.ID, c.Family)
	}
	if *c.PacketKey != registered {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names key %+v, want %+v", c.ID, *c.PacketKey, registered)
	}
	if registered.State != packetStatePlay {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names state %q, want play", c.ID, registered.State)
	}
	if registered.Direction != packetDirectionServer {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names direction %q, want server-to-client", c.ID, registered.Direction)
	}
	return protocol.StatePlay, registered.ID, nil
}

// newItemDropsCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newItemDropsCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// itemDropsFieldsForPacket renders the semantic fields of one decoded
// publication packet, dispatching on the concrete DTO the production decoder
// returned.
func itemDropsFieldsForPacket(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.ItemDropUpserts:
		return itemDropsUpsertsFields(message), nil
	case protocol.ItemDropRemoves:
		return itemDropsRemovesFields(message), nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// runItemDropsDecode executes one publication decode case through the real Go
// decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeServer` and classifies
// the failure it returns. It never reimplements the length, identity or
// ordering rules, so the recorded outcome is whatever the production codec
// decides about these exact bytes.
func runItemDropsDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := itemDropsPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newItemDropsCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeServer(state, packetID, input)
	if err != nil {
		category, classified := itemDropsRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := itemDropsFieldsForPacket(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// itemDropsIDRequest is the canonical JSON field input one drop identity
// carries.
type itemDropsIDRequest struct {
	Dimension  int32  `json:"dimension"`
	ChunkX     int32  `json:"chunk_x"`
	ChunkZ     int32  `json:"chunk_z"`
	Slot       uint8  `json:"slot"`
	Generation uint32 `json:"generation"`
}

// core renders the request as the Go drop identity the encoder validates.
func (request itemDropsIDRequest) core() core.DropID {
	return core.DropID{
		Dimension:  core.DimensionID(request.Dimension),
		Chunk:      core.ChunkPos{X: request.ChunkX, Z: request.ChunkZ},
		Slot:       request.Slot,
		Generation: request.Generation,
	}
}

// itemDropsStackRequest is the canonical JSON field input one item stack
// carries, where the exact empty triple is wire-valid.
type itemDropsStackRequest struct {
	Item       uint16 `json:"item"`
	Count      uint8  `json:"count"`
	Durability uint16 `json:"durability"`
}

// core renders the request as the Go stack value.
func (request itemDropsStackRequest) core() core.ItemStack {
	return core.ItemStack{
		Item: core.ItemID(request.Item), Count: request.Count, Durability: request.Durability,
	}
}

// itemDropsUpsertRequest is the canonical JSON field input one upsert record
// carries.
type itemDropsUpsertRequest struct {
	ID         itemDropsIDRequest    `json:"id"`
	BlockIndex uint32                `json:"block_index"`
	Stack      itemDropsStackRequest `json:"stack"`
}

// core renders the request as the Go drop record.
func (request itemDropsUpsertRequest) core() protocol.ItemDrop {
	return protocol.ItemDrop{
		ID:         request.ID.core(),
		BlockIndex: request.BlockIndex,
		Item:       core.ItemID(request.Stack.Item),
		Count:      request.Stack.Count,
		Durability: request.Stack.Durability,
	}
}

// itemDropsUpsertsRequest is the canonical JSON field input one upsert batch
// carries.
type itemDropsUpsertsRequest struct {
	ServerTick uint64                   `json:"server_tick"`
	Drops      []itemDropsUpsertRequest `json:"drops"`
}

// core renders the request as the Go batch DTO the encoder validates.
func (request itemDropsUpsertsRequest) core() protocol.ItemDropUpserts {
	drops := make([]protocol.ItemDrop, 0, len(request.Drops))
	for _, drop := range request.Drops {
		drops = append(drops, drop.core())
	}
	return protocol.ItemDropUpserts{ServerTick: request.ServerTick, Drops: drops}
}

// itemDropsRemovesRequest is the canonical JSON field input one remove batch
// carries.
type itemDropsRemovesRequest struct {
	ServerTick uint64               `json:"server_tick"`
	IDs        []itemDropsIDRequest `json:"ids"`
}

// core renders the request as the Go batch DTO the encoder validates.
func (request itemDropsRemovesRequest) core() protocol.ItemDropRemoves {
	ids := make([]core.DropID, 0, len(request.IDs))
	for _, id := range request.IDs {
		ids = append(ids, id.core())
	}
	return protocol.ItemDropRemoves{ServerTick: request.ServerTick, IDs: ids}
}

// runItemDropsEncode executes one publication encode case through the real Go
// encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects
// must never be recorded as evidence.
func runItemDropsEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := itemDropsPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newItemDropsCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet protocol.ServerPacket
	switch c.Family {
	case itemDropUpsertsFamily:
		var request itemDropsUpsertsRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	case itemDropRemovesFamily:
		var request itemDropsRemovesRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}

	encodedID, payload, err := wireCodec.EncodeServer(state, packet)
	if err != nil {
		category, classified := itemDropsRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified encode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	if encodedID != packetID {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoder published packet ID %d, want %d", c.ID, encodedID, packetID)
	}
	decoded, err := wireCodec.DecodeServer(state, encodedID, payload)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload does not decode back: %w", c.ID, err)
	}
	if !reflect.DeepEqual(decoded, packet) {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload decodes to %#v, want %#v", c.ID, decoded, packet)
	}
	fields, err := itemDropsFieldsForPacket(c, decoded)
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

// itemDropsCorpusRoutes is the closed route map the two families execute.
func itemDropsCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range itemDropFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: itemDropsVersion, Operation: "decode"}] = runItemDropsDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: itemDropsVersion, Operation: "encode"}] = runItemDropsEncode
	}
	return routes
}

// itemDropsCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer
// result.
type itemDropsCaseDefinition struct {
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

// itemDropsIDRequestFields renders one identity as its canonical JSON request
// fields.
func itemDropsIDRequestFields(id core.DropID) itemDropsIDRequest {
	return itemDropsIDRequest{
		Dimension:  int32(id.Dimension),
		ChunkX:     id.Chunk.X,
		ChunkZ:     id.Chunk.Z,
		Slot:       id.Slot,
		Generation: id.Generation,
	}
}

// itemDropsUpsertRequestFields renders one record as its canonical JSON
// request fields.
func itemDropsUpsertRequestFields(drop protocol.ItemDrop) itemDropsUpsertRequest {
	return itemDropsUpsertRequest{
		ID:         itemDropsIDRequestFields(drop.ID),
		BlockIndex: drop.BlockIndex,
		Stack: itemDropsStackRequest{
			Item: uint16(drop.Item), Count: drop.Count, Durability: drop.Durability,
		},
	}
}

// itemDropsUpsertsRequestFields is the canonical JSON request the valid upsert
// encode case carries, which is the reviewed wire's own field set.
func itemDropsUpsertsRequestFields() itemDropsUpsertsRequest {
	drops := make([]itemDropsUpsertRequest, 0, 2)
	for _, drop := range itemDropsCanonicalUpserts().Drops {
		drops = append(drops, itemDropsUpsertRequestFields(drop))
	}
	return itemDropsUpsertsRequest{ServerTick: 0, Drops: drops}
}

// itemDropsBlockIndexAboveRequest is the invalid encode request whose only
// violation is the block-index bound: the reviewed second record with block
// index 98304.
func itemDropsBlockIndexAboveRequest() itemDropsUpsertsRequest {
	request := itemDropsUpsertsRequestFields()
	request.Drops[1].BlockIndex = 98304
	return request
}

// itemDropsRemovesRequestFields is the canonical JSON request the valid remove
// encode case carries.
func itemDropsRemovesRequestFields() itemDropsRemovesRequest {
	ids := make([]itemDropsIDRequest, 0, 1)
	for _, id := range itemDropsCanonicalRemoves().IDs {
		ids = append(ids, itemDropsIDRequestFields(id))
	}
	return itemDropsRemovesRequest{ServerTick: 0, IDs: ids}
}

// itemDropsUpsertsDTO is the reviewed canonical upsert batch.
func itemDropsUpsertsDTO() protocol.ItemDropUpserts {
	return itemDropsCanonicalUpserts()
}

// itemDropsRemovesDTO is the reviewed canonical remove batch.
func itemDropsRemovesDTO() protocol.ItemDropRemoves {
	return itemDropsCanonicalRemoves()
}

// itemDropsFullRemovesDTO is the reviewed full thirty-two-record batch.
func itemDropsFullRemovesDTO() protocol.ItemDropRemoves {
	return itemDropsFullRemoves()
}

// itemDropsCountOneDTO is the reviewed one-record boundary with the raw
// dimension −1 identity, admitted beside the canonical vector.
func itemDropsCountOneDTO() protocol.ItemDropRemoves {
	return protocol.ItemDropRemoves{
		ServerTick: 0,
		IDs:        []core.DropID{itemDropsID(-1, 0, 0, 0, 1)},
	}
}

// buildItemDropsCandidate builds one case's manifest entry and asset bytes
// from its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildItemDropsCandidate(t *testing.T, definition itemDropsCaseDefinition) itemDropsCandidate {
	t.Helper()

	label := itemDropsLabel(definition.id)
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
		Version:      itemDropsVersion,
		Operation:    definition.op,
		PacketKey:    itemDropsKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return itemDropsCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// itemDropsKeyPointer resolves one family's reviewed packet key for a case
// spec.
func itemDropsKeyPointer(family string) *PacketKeySpec {
	key, owned := itemDropFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// itemDropsCandidate is one reviewed case: its manifest specification, the
// exact asset bytes it publishes, and the expectation an independent execution
// has to reproduce.
type itemDropsCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which
	// the producer's own bytes are compared against.
	Wire []byte
}

// itemDropsCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func itemDropsCandidates(t *testing.T, root string) []itemDropsCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := itemDropsCaseDefinitions()
	candidates := make([]itemDropsCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildItemDropsCandidate(t, definition)
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

// itemDropsRegisteredCase reports whether the base manifest already carries
// one case identity, so a re-merge of an integrated candidate adds nothing.
func itemDropsRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// itemDropsSelection is this group's registration: its cases, the Go sources
// its rules are read from, and the routes the two families execute.
func itemDropsSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := itemDropsCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range itemDropFamilies() {
		sources[family.id] = itemDropsServerSources()
	}
	routes := make([]ConsumerRoute, 0, len(itemDropFamilies())*2)
	for _, family := range itemDropFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: itemDropsVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: itemDropsVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  itemDropsProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// itemDropsServerSources lists the Go sources both families read their rules
// from: the shared server codec dispatch with its decode arms, the family's
// own message file, which owns the validator the shared dispatch applies at
// the tail, and the shared value codec, which owns the identity and record
// strides, the batch-count pre-allocation guard and the stack primitive.
func itemDropsServerSources() []string {
	return []string{
		"packages/shared/network/codec/codec_server.go",
		"packages/shared/network/protocol/message_drop.go",
		"packages/shared/network/codec/codec_values.go",
	}
}

// itemDropsManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func itemDropsManifest(t *testing.T, root string, candidates []itemDropsCandidate) Inventory {
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
		if !itemDropsOwnsFamily(family) {
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
		t.Fatalf("encode item drops working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write item drops working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load item drops working manifest: %v", err)
	}
	return loaded
}

// itemDropsOwnsFamily reports whether this group registers cases for one
// family.
func itemDropsOwnsFamily(family string) bool {
	_, owned := itemDropFamilyKeys[family]
	return owned
}

// itemDropsScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve under
// the root the runner is given.
func itemDropsScratchRoot(t *testing.T, root string, candidates []itemDropsCandidate) string {
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

// itemDropsCandidateByID resolves one candidate by its case identity.
func itemDropsCandidateByID(t *testing.T, candidates []itemDropsCandidate, id string) itemDropsCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return itemDropsCandidate{}
}

// itemDropsObservation resolves one executed observation by its case identity.
func itemDropsObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// itemDropsExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var itemDropsExportPublished bool

// itemDropsCandidatesExport publishes the reviewed candidates and the
// complete merged manifest candidate through the existing external exporter and
// returns the published producer directory. An unset export variable publishes
// nothing and returns "", so an ordinary test run never writes outside its own
// temporary storage.
func itemDropsCandidatesExport(t *testing.T, root string, candidates []itemDropsCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if itemDropsExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-drops")
	}
	itemDropsExportPublished = true
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
	published, err := exportGeneratedAssets(root, exportRoot, itemDropsProducerID, assets)
	if err != nil {
		t.Fatalf("export item drops candidates: %v", err)
	}
	return published
}

// itemDropsDecodeCaseIDs lists the valid decode cases the producer pins.
func itemDropsDecodeCaseIDs() []string {
	return []string{
		itemDropsUpsertsDecodeCaseID,
		itemDropsRemovesDecodeCaseID,
		itemDropsRemovesCountOneID,
		itemDropsRemovesCountThirtyID,
	}
}

// itemDropsEncodeCaseIDs lists the valid encode cases the producer pins.
func itemDropsEncodeCaseIDs() []string {
	return []string{
		itemDropsUpsertsEncodeCaseID,
		itemDropsRemovesEncodeCaseID,
	}
}

// itemDropsRejectedDecodeCaseIDs lists the malformed decode cases.
func itemDropsRejectedDecodeCaseIDs() []string {
	return []string{
		itemDropsUpsertsBlockIndexID,
		itemDropsUpsertsGenerationID,
		itemDropsUpsertsSlotAboveID,
		itemDropsUpsertsCountLimitID,
		itemDropsUpsertsDuplicateID,
		itemDropsUpsertsReversedID,
		itemDropsUpsertsTrailingID,
		itemDropsRemovesCountZeroID,
		itemDropsRemovesCountOverID,
		itemDropsRemovesDuplicateID,
		itemDropsRemovesReversedID,
		itemDropsRemovesBadGenerationI,
	}
}

// itemDropsRejectedEncodeCaseIDs lists the invalid encode cases.
func itemDropsRejectedEncodeCaseIDs() []string {
	return []string{itemDropsUpsertsEncodeBlockID}
}

// itemDropsLabel renders one case's asset label from its identity.
func itemDropsLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// TestProtocolDropsOracleDecodesEveryValidCase pins that the real Go decoder
// publishes the canonical fields for both families' valid decode cases,
// including the one-record and full-batch boundaries.
func TestProtocolDropsOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := itemDropsCandidates(t, root)

	for _, id := range itemDropsDecodeCaseIDs() {
		candidate := itemDropsCandidateByID(t, candidates, id)
		outcome, encoded, err := runItemDropsDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runItemDropsDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolDropsOracleEncodesCanonicalWire pins each encode producer
// against the reviewed wire literal and against the recorded digest.
func TestProtocolDropsOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := itemDropsCandidates(t, root)

	for _, id := range itemDropsEncodeCaseIDs() {
		candidate := itemDropsCandidateByID(t, candidates, id)
		outcome, encoded, err := runItemDropsEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runItemDropsEncode(%s): %v", id, err)
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

// TestProtocolDropsOracleRejectsMalformedCasesAtTheirBoundary pins that every
// malformed decode case and the invalid encode case are refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolDropsOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := itemDropsCandidates(t, root)

	for _, id := range itemDropsRejectedDecodeCaseIDs() {
		candidate := itemDropsCandidateByID(t, candidates, id)
		outcome, encoded, err := runItemDropsDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runItemDropsDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range itemDropsRejectedEncodeCaseIDs() {
		candidate := itemDropsCandidateByID(t, candidates, id)
		outcome, encoded, err := runItemDropsEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runItemDropsEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolDropsOracleExpectedFieldMutationFailsComparison pins that the
// recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded, and replacing the encoded
// digest fails comparison against the reviewed wire.
func TestProtocolDropsOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := itemDropsCandidates(t, root)

	decode := itemDropsCandidateByID(t, candidates, itemDropsUpsertsDecodeCaseID)
	produced, _, err := runItemDropsDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runItemDropsDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	// The dimension is the contract: a normalization that narrowed the raw
	// wire value would hide the −1 and 256 dimensions this vector carries.
	fields["drops"] = []map[string]any{{
		"id": map[string]any{
			"dimension":  int32(0),
			"chunk_x":    int32(7),
			"chunk_z":    int32(-3),
			"slot":       uint8(31),
			"generation": uint32(0xFFFFFFFF),
		},
		"block_index": uint32(0),
		"stack": map[string]any{
			"item": int32(core.ItemStone), "count": uint8(4), "durability": uint16(0),
		},
	}}
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated drop dimension compares equal to the produced outcome")
	}

	encode := itemDropsCandidateByID(t, candidates, itemDropsRemovesEncodeCaseID)
	producedEncode, _, err := runItemDropsEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runItemDropsEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolDropsOracleRoutesExecuteEveryCase executes this group's complete
// case set through the shared packet-case runner, once per case, and compares
// every observation with the reviewed expectation.
func TestProtocolDropsOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := itemDropsCandidates(t, root)
	manifest := itemDropsManifest(t, root, candidates)
	staged := itemDropsScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, itemDropsCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := itemDropsObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolDropsOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so
// a case cannot claim coverage from its name alone.
func TestProtocolDropsOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := itemDropsCandidates(t, root)
	manifest := itemDropsManifest(t, root, candidates)
	staged := itemDropsScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range itemDropFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: itemDropsVersion, Operation: "decode"}] = runItemDropsDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range itemDropFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+itemDropsVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolDropsOracleManifestMergeRegistersItemDropRoutes pins that the
// merged manifest registers both families' routes and case lists, leaves the
// source revision alone, and records this group's provenance sources.
func TestProtocolDropsOracleManifestMergeRegistersItemDropRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, itemDropsSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range itemDropFamilies() {
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
		for _, want := range itemDropsServerSources() {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so
	// a re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range itemDropsCandidates(t, root) {
		if !itemDropsRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolDropsOracleCandidatesExportForReview publishes the reviewed
// candidates and the manifest candidate. An unset export variable publishes
// nothing, so the tracked corpus is never written by this package.
func TestProtocolDropsOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := itemDropsCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), itemDropsSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := itemDropsCandidatesExport(t, root, candidates, merged)
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

// TestProtocolDropsGoDecoderAppliesMinimumRecordLength pins the one
// structural boundary this group shares with the Rust consumer: the Go decoder
// answers a payload shorter than the declared records with its
// remaining-bytes message and a padded payload with its trailing-byte message,
// so both implementations publish the truncation boundary for a short payload
// and the trailing boundary for a padded one.
func TestProtocolDropsGoDecoderAppliesMinimumRecordLength(t *testing.T) {
	wireCodec, err := newItemDropsCodec()
	if err != nil {
		t.Fatalf("newItemDropsCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	wire := itemDropsRemovesWire(0, 1, []core.DropID{itemDropsID(0, 1, -2, 3, 7)})
	extra := append(append([]byte(nil), wire...), 0x00)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 12, extra); err == nil {
		t.Fatal("the Go decoder accepted a batch with a trailing byte")
	} else if !strings.Contains(err.Error(), "trailing bytes") {
		t.Fatalf("the Go decoder answered the trailing byte with %v", err)
	}

	// The count bound fires before the remaining-bytes rule, so a declared
	// count above the ceiling is refused at the count boundary even when the
	// payload cannot back it.
	shortOver := append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 33}, itemDropsIDWire(itemDropsID(0, 1, -2, 3, 7))...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 12, shortOver); err == nil {
		t.Fatal("the Go decoder accepted a declared count above the ceiling")
	} else if !strings.Contains(err.Error(), "item drop batch count is outside 1..32") {
		t.Fatalf("the Go decoder answered the short over-count payload with %v", err)
	}
	shortTwo := append([]byte{0, 0, 0, 0, 0, 0, 0, 0, 2}, itemDropsIDWire(itemDropsID(0, 1, -2, 3, 7))...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 12, shortTwo); err == nil {
		t.Fatal("the Go decoder accepted a declared count the payload cannot back")
	} else if !strings.Contains(err.Error(), "packet count exceeds remaining payload") {
		t.Fatalf("the Go decoder answered the short two-count payload with %v", err)
	}
}

// TestProtocolDropsGoEncoderRefusesBlockIndexAboveChunk pins that the
// production encoder runs the outbound validator before it writes the block
// index, so a drop naming an index outside the chunk never becomes a silently
// published value.
//
// The Rust side published that index silently before this node's surface
// conversion, which is the behavioral red the fallible surface closes; the Go
// encoder has always refused the DTO, and the corpus case records that refusal
// so the two implementations stay pinned to one boundary.
func TestProtocolDropsGoEncoderRefusesBlockIndexAboveChunk(t *testing.T) {
	wireCodec, err := newItemDropsCodec()
	if err != nil {
		t.Fatalf("newItemDropsCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	request := itemDropsBlockIndexAboveRequest()
	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, request.core()); err == nil {
		t.Fatal("the Go encoder published a block index outside the chunk")
	} else if !strings.Contains(err.Error(), "item drop block index is outside the chunk") {
		t.Fatalf("the Go encoder refused the block index with %v", err)
	}
}

// itemDropsCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid
// cases use the canonical payloads the Go encoder produces for these fields,
// including the one-record and full-batch boundaries; the malformed decode
// cases mutate that payload at one boundary each, and the invalid encode case
// builds a DTO the production validator refuses, so each rejection names the
// boundary that owns it.
//
// The unregistered item number is deliberately absent from the table: the Go
// validator answers it with the same `network: invalid item drop stack`
// message as the count and durability boundaries, so a corpus case could not
// distinguish them and the Rust group test pins it at its own variant instead.
// No case combines two violations.
func itemDropsCaseDefinitions() []itemDropsCaseDefinition {
	validUpserts := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   itemDropsUpsertsFields(itemDropsUpsertsDTO()),
	}
	validRemoves := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   itemDropsRemovesFields(itemDropsRemovesDTO()),
	}
	validCountOne := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   itemDropsRemovesFields(itemDropsCountOneDTO()),
	}
	validCountThirtyTwo := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   itemDropsRemovesFields(itemDropsFullRemovesDTO()),
	}
	invalidValue := Outcome{Kind: "error", Category: "invalid-value"}
	invalidIdentity := Outcome{Kind: "error", Category: "invalid-identity"}
	trailing := Outcome{Kind: "error", Category: "trailing"}

	first := itemDropsCanonicalUpsert()
	second := itemDropsEmptyUpsert()
	canonicalIdentity := itemDropsID(0, 1, -2, 3, 7)

	upsertsWire := itemDropsUpsertsWire(0, 2, []protocol.ItemDrop{first, second})
	removesWire := itemDropsRemovesWire(0, 1, []core.DropID{canonicalIdentity})
	countOneWire := itemDropsRemovesWire(0, 1, itemDropsCountOneDTO().IDs)
	countThirtyTwoWire := itemDropsRemovesWire(0, itemDropsMaxRecords, itemDropsFullRemoves().IDs)

	definitions := make([]itemDropsCaseDefinition, 0, 19)
	definitions = append(definitions,
		itemDropsCaseDefinition{
			id:     itemDropsUpsertsDecodeCaseID,
			family: itemDropUpsertsFamily,
			relDir: itemDropUpsertsCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), upsertsWire...),
			expect: validUpserts,
		},
		itemDropsCaseDefinition{
			id:      itemDropsUpsertsEncodeCaseID,
			family:  itemDropUpsertsFamily,
			relDir:  itemDropUpsertsCorpusRelDir,
			op:      "encode",
			request: itemDropsUpsertsRequestFields(),
			wire:    append([]byte(nil), upsertsWire...),
			expect:  validUpserts,
		},
		itemDropsCaseDefinition{
			// The block index 98304 is one past the inclusive upper bound of
			// the chunk-ordered block index, which the Go validator refuses
			// with its own message at the value boundary.
			id:     itemDropsUpsertsBlockIndexID,
			family: itemDropUpsertsFamily,
			relDir: itemDropUpsertsCorpusRelDir,
			op:     "decode",
			input: itemDropsUpsertsWire(0, 2, []protocol.ItemDrop{
				protocol.ItemDrop{ID: first.ID, BlockIndex: 98304, Item: first.Item, Count: first.Count, Durability: first.Durability},
				second,
			}),
			expect: invalidValue,
		},
		itemDropsCaseDefinition{
			// The identity rule checks the slot range and the generation, so
			// a zero generation is the identity boundary both sides publish.
			id:     itemDropsUpsertsGenerationID,
			family: itemDropUpsertsFamily,
			relDir: itemDropUpsertsCorpusRelDir,
			op:     "decode",
			input: itemDropsUpsertsWire(0, 2, []protocol.ItemDrop{
				protocol.ItemDrop{ID: itemDropsID(-1, 7, -3, 31, 0), BlockIndex: first.BlockIndex, Item: first.Item, Count: first.Count, Durability: first.Durability},
				second,
			}),
			expect: invalidIdentity,
		},
		itemDropsCaseDefinition{
			// Slot 32 is one past the fixed per-chunk drop array, the other
			// identity rule the Go validator names with its own message.
			id:     itemDropsUpsertsSlotAboveID,
			family: itemDropUpsertsFamily,
			relDir: itemDropUpsertsCorpusRelDir,
			op:     "decode",
			input: itemDropsUpsertsWire(0, 2, []protocol.ItemDrop{
				protocol.ItemDrop{ID: itemDropsID(-1, 7, -3, 32, 0xFFFFFFFF), BlockIndex: first.BlockIndex, Item: first.Item, Count: first.Count, Durability: first.Durability},
				second,
			}),
			expect: invalidIdentity,
		},
		itemDropsCaseDefinition{
			// Count 65 is one above the stack limit of the reviewed stone
			// stack, which the folded Go stack message answers at the value
			// boundary the Rust range error publishes.
			id:     itemDropsUpsertsCountLimitID,
			family: itemDropUpsertsFamily,
			relDir: itemDropUpsertsCorpusRelDir,
			op:     "decode",
			input: itemDropsUpsertsWire(0, 2, []protocol.ItemDrop{
				protocol.ItemDrop{ID: first.ID, BlockIndex: first.BlockIndex, Item: first.Item, Count: 65, Durability: first.Durability},
				second,
			}),
			expect: invalidValue,
		},
		itemDropsCaseDefinition{
			// Two identical identities are refused by the strict order rule.
			id:     itemDropsUpsertsDuplicateID,
			family: itemDropUpsertsFamily,
			relDir: itemDropUpsertsCorpusRelDir,
			op:     "decode",
			input:  itemDropsUpsertsWire(0, 2, []protocol.ItemDrop{first, first}),
			expect: invalidValue,
		},
		itemDropsCaseDefinition{
			// The descending twin: the 256-dimension record first, so the
			// order rule that compares the raw dimension first answers.
			id:     itemDropsUpsertsReversedID,
			family: itemDropUpsertsFamily,
			relDir: itemDropUpsertsCorpusRelDir,
			op:     "decode",
			input:  itemDropsUpsertsWire(0, 2, []protocol.ItemDrop{second, first}),
			expect: invalidValue,
		},
		itemDropsCaseDefinition{
			// The encode twin of the block-index case: the outbound validator
			// refuses the same mutation before any byte is published.
			id:      itemDropsUpsertsEncodeBlockID,
			family:  itemDropUpsertsFamily,
			relDir:  itemDropUpsertsCorpusRelDir,
			op:      "encode",
			request: itemDropsBlockIndexAboveRequest(),
			expect:  invalidValue,
		},
		itemDropsCaseDefinition{
			// Both drop decoders apply the minimum-records rule, so the extra
			// byte is answered by the end-of-payload check rather than by a
			// remaining-length rule.
			id:     itemDropsUpsertsTrailingID,
			family: itemDropUpsertsFamily,
			relDir: itemDropUpsertsCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), upsertsWire...), 0x00),
			expect: trailing,
		},
		itemDropsCaseDefinition{
			id:     itemDropsRemovesDecodeCaseID,
			family: itemDropRemovesFamily,
			relDir: itemDropRemovesCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), removesWire...),
			expect: validRemoves,
		},
		itemDropsCaseDefinition{
			id:      itemDropsRemovesEncodeCaseID,
			family:  itemDropRemovesFamily,
			relDir:  itemDropRemovesCorpusRelDir,
			op:      "encode",
			request: itemDropsRemovesRequestFields(),
			wire:    append([]byte(nil), removesWire...),
			expect:  validRemoves,
		},
		itemDropsCaseDefinition{
			// A single record is the count lower boundary, admitted beside
			// the canonical vector with the raw dimension -1 identity.
			id:     itemDropsRemovesCountOneID,
			family: itemDropRemovesFamily,
			relDir: itemDropRemovesCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), countOneWire...),
			expect: validCountOne,
		},
		itemDropsCaseDefinition{
			// Thirty-two records is the count and record ceiling, admitted
			// exactly at the bounded drop set.
			id:     itemDropsRemovesCountThirtyID,
			family: itemDropRemovesFamily,
			relDir: itemDropRemovesCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), countThirtyTwoWire...),
			expect: validCountThirtyTwo,
		},
		itemDropsCaseDefinition{
			id:     itemDropsRemovesCountZeroID,
			family: itemDropRemovesFamily,
			relDir: itemDropRemovesCorpusRelDir,
			op:     "decode",
			input:  itemDropsRemovesWire(0, 0, nil),
			expect: invalidValue,
		},
		itemDropsCaseDefinition{
			// The count bound fires before the remaining-bytes rule, so a
			// declared count above the ceiling is refused at the count
			// boundary even though the payload carries one record.
			id:     itemDropsRemovesCountOverID,
			family: itemDropRemovesFamily,
			relDir: itemDropRemovesCorpusRelDir,
			op:     "decode",
			input:  itemDropsRemovesWire(0, itemDropsMaxRecords+1, []core.DropID{canonicalIdentity}),
			expect: invalidValue,
		},
		itemDropsCaseDefinition{
			id:     itemDropsRemovesDuplicateID,
			family: itemDropRemovesFamily,
			relDir: itemDropRemovesCorpusRelDir,
			op:     "decode",
			input:  itemDropsRemovesWire(0, 2, []core.DropID{canonicalIdentity, canonicalIdentity}),
			expect: invalidValue,
		},
		itemDropsCaseDefinition{
			// The cross-dimension descending twin: the 256-dimension identity
			// first and the -1-dimension identity second, so the order rule
			// that compares the raw dimension first answers.
			id:     itemDropsRemovesReversedID,
			family: itemDropRemovesFamily,
			relDir: itemDropRemovesCorpusRelDir,
			op:     "decode",
			input: itemDropsRemovesWire(0, 2, []core.DropID{
				itemDropsID(256, 0, 0, 0, 1),
				itemDropsID(-1, 0, 0, 0, 1),
			}),
			expect: invalidValue,
		},
		itemDropsCaseDefinition{
			// A zero generation is the identity boundary, which the Go remove
			// batch answers with its own indexed message.
			id:     itemDropsRemovesBadGenerationI,
			family: itemDropRemovesFamily,
			relDir: itemDropRemovesCorpusRelDir,
			op:     "decode",
			input:  itemDropsRemovesWire(0, 1, []core.DropID{itemDropsID(0, 1, -2, 3, 0)}),
			expect: invalidIdentity,
		},
	)

	return definitions
}
