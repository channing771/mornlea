package main

// This file is the chunk-snapshot producer group: the one Play
// server-to-client family whose wire payload is compressed (an eight-byte
// envelope — the declared decoded length, then the declared compressed length
// — followed by a zstd frame carrying the logical snapshot) registers a decode
// and an encode route through the shared packet-case runner, so every case is
// executed by the real Go codec rather than restated here.
//
// The compressed layer is where this group differs from the others. The Rust
// and Go zstd encoders legitimately publish different compressed bytes, so no
// case compares them: for every encode case the recorded
// encoded_payload_digest is the SHA-256 of the canonical LOGICAL payload.
// The producer encodes through the Go codec and decompresses its own output
// back to logical bytes with the zstd package this module already builds
// through `packages/shared` — the oracle adds no dependency of its own and
// never compresses bytes by hand. The Rust consumer digests the logical bytes
// of its own encoding the same way, and the two digests coincide because the
// logical layer is byte-identical across the two implementations.
//
// The negatives follow the layer split. Revision zero, a 23-section column, a
// swapped Y pair, a palette slot beyond the palette and direct high bits are
// encode-side cases the Go outbound validator refuses before compression.
// Compressed length above the cap, decoded length above the cap, a truncated
// frame and a flipped body byte are decode-side byte edits of one valid
// Go-encoded payload. Every negative carries exactly one violation, so the
// category each one records is the boundary that owns it: the envelope's
// declared-length ceilings are capacity, a payload cut off inside the
// compressed body is answered by the envelope's remaining-length check as
// truncated, and a byte flipped inside a length-complete frame is answered
// by the frame's own content checksum as integrity.
//
// This group's assets render compactly rather than with the committed
// two-space layout: the mixed vector's packed words make the indented layout
// of both the encode request and the normalized outcome exceed the 256 KiB
// JSON case budget. Every field name, value encoding and key order is the
// committed one; only the whitespace differs.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
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
	"github.com/klauspost/compress/zstd"
)

const (
	// snapshotFamily is the play chunk snapshot family this group registers.
	snapshotFamily = "protocol.server.ChunkSnapshot"
	// snapshotVersion is the protocol version the family is pinned to.
	snapshotVersion = "45"
	// snapshotProducerID is the exporter's producer identity for this group's
	// candidate assets and merged manifest.
	snapshotProducerID = "runtime-oracle/protocol-snapshot"

	// snapshotCorpusRelDir is the repository-relative directory holding the
	// family's committed case assets.
	snapshotCorpusRelDir = corpusCasesRelDir + "/protocol/ChunkSnapshot"

	// snapshotEnvelopeLength is the fixed eight-byte envelope header: the
	// declared decoded length, then the declared compressed length.
	snapshotEnvelopeLength = 8
	// snapshotMaxCompressed and snapshotMaxDecoded are the Go codec's two
	// snapshot ceilings, which the cap cases declare one past.
	snapshotMaxCompressed = 1 << 20
	snapshotMaxDecoded    = 2 << 20
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	snapshotDecodeFixtureID = snapshotFamily + "/" + snapshotVersion + "/decode-fixture"
	snapshotDecodeMixedID   = snapshotFamily + "/" + snapshotVersion + "/decode-valid-mixed"
	snapshotEncodeMixedID   = snapshotFamily + "/" + snapshotVersion + "/encode-valid-mixed"
	snapshotEncodeSingleID  = snapshotFamily + "/" + snapshotVersion + "/encode-valid-all-single"
	snapshotRevisionZeroID  = snapshotFamily + "/" + snapshotVersion + "/encode-revision-zero"
	snapshotSections23ID    = snapshotFamily + "/" + snapshotVersion + "/encode-23-sections"
	snapshotYSwapID         = snapshotFamily + "/" + snapshotVersion + "/encode-y-order-swap"
	snapshotPaletteSlotID   = snapshotFamily + "/" + snapshotVersion + "/encode-palette-index-out-of-range"
	snapshotDirectHighID    = snapshotFamily + "/" + snapshotVersion + "/encode-direct-high-bits"
	snapshotCompressedCapID = snapshotFamily + "/" + snapshotVersion + "/decode-compressed-length-above-cap"
	snapshotDecodedCapID    = snapshotFamily + "/" + snapshotVersion + "/decode-decoded-length-above-cap"
	snapshotTruncatedID     = snapshotFamily + "/" + snapshotVersion + "/decode-truncated-zstd-frame"
	snapshotChecksumID      = snapshotFamily + "/" + snapshotVersion + "/decode-checksum-failure"
)

// snapshotFamilyKeys pins the family's complete packet key: server-to-client,
// play state, packet ID 0. A case names its key instead of trusting its family.
var snapshotFamilyKeys = map[string]PacketKeySpec{
	snapshotFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 0},
}

// snapshotWord renders one packed word as sixteen lowercase hex digits, the
// canonical field encoding both implementations publish for packed slots.
func snapshotWord(word uint64) string {
	return fmt.Sprintf("%016x", word)
}

// snapshotPackedWords packs one section's cells so slot `index` carries
// `(index + seed) % modulus`, mirroring the Rust consumer's builder.
func snapshotPackedWords(bits uint8, modulus int, seed int) []string {
	perWord := 64 / int(bits)
	words := (core.BlocksPerSection + perWord - 1) / perWord
	rendered := make([]string, words)
	for index := range rendered {
		var word uint64
		for slot := 0; slot < perWord; slot++ {
			cell := index*perWord + slot
			if cell >= core.BlocksPerSection {
				break
			}
			value := uint64((cell + seed) % modulus)
			word |= value << (uint(slot) * uint(bits))
		}
		rendered[index] = snapshotWord(word)
	}
	return rendered
}

// snapshotUniformWords packs one section whose cells all carry `value`, except
// the last cell which carries `last`.
func snapshotUniformWords(bits uint8, value uint64, last uint64) []string {
	perWord := 64 / int(bits)
	words := (core.BlocksPerSection + perWord - 1) / perWord
	mask := uint64(1)<<uint(bits) - 1
	rendered := make([]string, words)
	for index := range rendered {
		var word uint64
		for slot := 0; slot < perWord; slot++ {
			cell := index*perWord + slot
			if cell >= core.BlocksPerSection {
				break
			}
			carried := value
			if cell == core.BlocksPerSection-1 {
				carried = last
			}
			word |= (carried & mask) << (uint(slot) * uint(bits))
		}
		rendered[index] = snapshotWord(word)
	}
	return rendered
}

// snapshotSectionRequest is one section's canonical JSON fields: the section
// index, the container kind (which implies the bits-per-slot), and only the
// fields that kind carries.
type snapshotSectionRequest struct {
	Y       int      `json:"y"`
	Kind    string   `json:"kind"`
	Block   *uint16  `json:"block,omitempty"`
	Palette []uint16 `json:"palette,omitempty"`
	Words   []string `json:"words,omitempty"`
}

// snapshotEncodeRequest is the canonical JSON field input one snapshot encode
// case carries.
type snapshotEncodeRequest struct {
	Dimension int                      `json:"dimension"`
	ChunkX    int32                    `json:"chunk_x"`
	ChunkZ    int32                    `json:"chunk_z"`
	Revision  uint64                   `json:"revision"`
	Sections  []snapshotSectionRequest `json:"sections"`
}

// snapshotSingleSection builds one all-distinct single-block section list.
func snapshotSingleSections() []snapshotSectionRequest {
	sections := make([]snapshotSectionRequest, 0, core.SectionsPerChunk)
	for y := 0; y < core.SectionsPerChunk; y++ {
		block := uint16(y)
		sections = append(sections, snapshotSectionRequest{Y: y, Kind: "single", Block: &block})
	}
	return sections
}

// snapshotNinetyPalette is the ninety registered block numbers 0..89.
func snapshotNinetyPalette() []uint16 {
	palette := make([]uint16, 0, 90)
	for id := uint16(0); id < 90; id++ {
		palette = append(palette, id)
	}
	return palette
}

// snapshotMixedSections builds the canonical mixed vector: Y0..5 single,
// Y6..11 indexed 4-bit over a two-entry palette, Y12..17 indexed 8-bit over
// the ninety-ID palette, and Y18..23 direct 15-bit with block 89 at the last
// cell of Y23.
func snapshotMixedSections() []snapshotSectionRequest {
	sections := make([]snapshotSectionRequest, 0, core.SectionsPerChunk)
	palette := snapshotNinetyPalette()
	for y := 0; y < core.SectionsPerChunk; y++ {
		switch {
		case y <= 5:
			block := uint16(y)
			sections = append(sections, snapshotSectionRequest{Y: y, Kind: "single", Block: &block})
		case y <= 11:
			sections = append(sections, snapshotSectionRequest{
				Y: y, Kind: "indexed4", Palette: []uint16{1, 2},
				Words: snapshotPackedWords(4, 2, y),
			})
		case y <= 17:
			sections = append(sections, snapshotSectionRequest{
				Y: y, Kind: "indexed8", Palette: palette,
				Words: snapshotPackedWords(8, 90, y),
			})
		case y == core.SectionsPerChunk-1:
			sections = append(sections, snapshotSectionRequest{
				Y: y, Kind: "direct", Words: snapshotUniformWords(15, 1, 89),
			})
		default:
			sections = append(sections, snapshotSectionRequest{
				Y: y, Kind: "direct", Words: snapshotUniformWords(15, 1, 1),
			})
		}
	}
	return sections
}

// snapshotMixedRequest is the canonical mixed vector the valid cases carry:
// dimension 1 (Depths), chunk (-1, 0), revision 1.
func snapshotMixedRequest() snapshotEncodeRequest {
	return snapshotEncodeRequest{
		Dimension: 1, ChunkX: -1, ChunkZ: 0, Revision: 1,
		Sections: snapshotMixedSections(),
	}
}

// snapshotSingleRequest is the all-single vector the second valid encode case
// carries.
func snapshotSingleRequest() snapshotEncodeRequest {
	return snapshotEncodeRequest{
		Dimension: 1, ChunkX: -1, ChunkZ: 0, Revision: 1,
		Sections: snapshotSingleSections(),
	}
}

// snapshotCloneRequest deep-copies one request so a negative case can mutate a
// single field of a valid vector.
func snapshotCloneRequest(request snapshotEncodeRequest) snapshotEncodeRequest {
	clone := request
	clone.Sections = make([]snapshotSectionRequest, len(request.Sections))
	for index, section := range request.Sections {
		copied := section
		if section.Block != nil {
			block := *section.Block
			copied.Block = &block
		}
		if section.Palette != nil {
			copied.Palette = append([]uint16(nil), section.Palette...)
		}
		if section.Words != nil {
			copied.Words = append([]string(nil), section.Words...)
		}
		clone.Sections[index] = copied
	}
	return clone
}

// snapshotPacketKey resolves one packet key to the Go state and numeric ID the
// codec dispatches on.
func snapshotPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := snapshotFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no snapshot producer owns", c.ID, c.Family)
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

// newSnapshotCodec builds the production codec one producer call uses. The codec
// owns the snapshot compression context, so the producer cannot bypass it with
// a hand-written payload writer. The caller closes it.
func newSnapshotCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// snapshotBitsOf reports the bits-per-slot one kind implies on the wire.
func snapshotBitsOf(kind string) (uint8, bool) {
	switch kind {
	case "single":
		return 0, true
	case "indexed4":
		return 4, true
	case "indexed8":
		return 8, true
	case "direct":
		return 15, true
	default:
		return 0, false
	}
}

// snapshotDTO builds the DTO one encode case names from its typed fields, so
// the outbound validator decides about the same record the decode path would
// publish.
func snapshotDTO(request snapshotEncodeRequest) (protocol.ChunkSnapshot, error) {
	sections := make([]protocol.SectionData, 0, len(request.Sections))
	for _, section := range request.Sections {
		bits, known := snapshotBitsOf(section.Kind)
		if !known {
			return protocol.ChunkSnapshot{}, fmt.Errorf("snapshot section %d names unknown kind %q", section.Y, section.Kind)
		}
		built := protocol.SectionData{Y: int32(section.Y), Bits: bits}
		// The decoder leaves a kind's unused fields nil, so the request builder
		// does the same: an empty-but-non-nil slice would fail the read-back
		// comparison for a different reason than a wire mismatch.
		if len(section.Palette) > 0 {
			built.Palette = make([]core.BlockID, 0, len(section.Palette))
			for _, id := range section.Palette {
				built.Palette = append(built.Palette, core.BlockID(id))
			}
		}
		if len(section.Words) > 0 {
			built.Packed = make([]uint64, 0, len(section.Words))
			for _, word := range section.Words {
				parsed, err := strconv.ParseUint(word, 16, 64)
				if err != nil {
					return protocol.ChunkSnapshot{}, fmt.Errorf("snapshot section %d names invalid word %q: %w", section.Y, word, err)
				}
				built.Packed = append(built.Packed, parsed)
			}
		}
		switch section.Kind {
		case "single":
			if section.Block == nil {
				return protocol.ChunkSnapshot{}, fmt.Errorf("snapshot section %d names no single block", section.Y)
			}
			built.Storage, built.Single = protocol.SectionSingle, core.BlockID(*section.Block)
		case "indexed4", "indexed8":
			built.Storage = protocol.SectionIndexed
		case "direct":
			built.Storage = protocol.SectionDirect
		}
		sections = append(sections, built)
	}
	return protocol.ChunkSnapshot{
		Dimension: core.DimensionID(request.Dimension),
		Chunk:     core.ChunkPos{X: request.ChunkX, Z: request.ChunkZ},
		Revision:  request.Revision,
		Sections:  sections,
	}, nil
}

// snapshotPacket builds the DTO one encode case names from its JSON input.
func snapshotPacket(c CaseSpec, input []byte) (protocol.ServerPacket, error) {
	var request snapshotEncodeRequest
	if err := decodeEncodeRequest(c, input, &request); err != nil {
		return nil, err
	}
	snapshot, err := snapshotDTO(request)
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: case %s: %w", c.ID, err)
	}
	return snapshot, nil
}

// snapshotLogicalBytes decompresses one Go-encoded payload's frame back to the
// canonical logical bytes the digest is taken over.
//
// Compression itself always comes from the production codec: this helper only
// reads the codec's own output back, through the zstd package this module
// already builds via `packages/shared`. The envelope's declared lengths are
// re-checked here, mirroring the decoder's own envelope rule, so a payload the
// codec would have refused never reaches the digest.
func snapshotLogicalBytes(c CaseSpec, payload []byte) ([]byte, error) {
	if len(payload) < snapshotEnvelopeLength {
		return nil, fmt.Errorf("runtime-oracle: case %s: encoded payload is shorter than the snapshot envelope", c.ID)
	}
	declaredDecoded := binary.LittleEndian.Uint32(payload[0:4])
	declaredCompressed := binary.LittleEndian.Uint32(payload[4:8])
	if int(declaredCompressed) != len(payload)-snapshotEnvelopeLength {
		return nil, fmt.Errorf(
			"runtime-oracle: case %s: envelope declares %d compressed bytes, payload carries %d",
			c.ID, declaredCompressed, len(payload)-snapshotEnvelopeLength,
		)
	}
	if int(declaredDecoded) > snapshotMaxDecoded {
		return nil, fmt.Errorf("runtime-oracle: case %s: envelope declares %d decoded bytes above the cap", c.ID, declaredDecoded)
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create zstd reader: %w", err)
	}
	defer decoder.Close()
	logical, err := decoder.DecodeAll(payload[snapshotEnvelopeLength:], nil)
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: case %s: decompress own output: %w", c.ID, err)
	}
	if uint32(len(logical)) != declaredDecoded {
		return nil, fmt.Errorf(
			"runtime-oracle: case %s: decoded %d logical bytes, envelope declares %d",
			c.ID, len(logical), declaredDecoded,
		)
	}
	return logical, nil
}

// snapshotSectionFields renders one section's semantic fields: the section
// index, the container kind (which implies the bits-per-slot), and only the
// fields that kind carries. The palette renders as plain numbers so its order
// survives, and the packed words render as fixed-width lowercase hex so their
// exact bits do.
func snapshotSectionFields(section protocol.SectionData) (map[string]any, error) {
	fields := map[string]any{"y": section.Y}
	switch section.Storage {
	case protocol.SectionSingle:
		fields["kind"] = "single"
		fields["block"] = uint16(section.Single)
	case protocol.SectionIndexed:
		switch section.Bits {
		case 4:
			fields["kind"] = "indexed4"
		case 8:
			fields["kind"] = "indexed8"
		default:
			return nil, fmt.Errorf("snapshot section %d names invalid indexed bits %d", section.Y, section.Bits)
		}
		palette := make([]uint16, 0, len(section.Palette))
		for _, id := range section.Palette {
			palette = append(palette, uint16(id))
		}
		fields["palette"] = palette
		words := make([]string, 0, len(section.Packed))
		for _, word := range section.Packed {
			words = append(words, snapshotWord(word))
		}
		fields["words"] = words
	case protocol.SectionDirect:
		fields["kind"] = "direct"
		words := make([]string, 0, len(section.Packed))
		for _, word := range section.Packed {
			words = append(words, snapshotWord(word))
		}
		fields["words"] = words
	default:
		return nil, fmt.Errorf("snapshot section %d names unknown storage %d", section.Y, section.Storage)
	}
	return fields, nil
}

// snapshotFields renders the semantic fields one decoded snapshot publishes.
//
// The fields come from the DTO the production decoder returned, never from the
// case input. The revision renders as a decimal string so the full u64 range
// stays lossless, the coordinates render as plain JSON integers, and the
// section list renders in column order.
func snapshotFields(c CaseSpec, packet any) (map[string]any, error) {
	snapshot, ok := packet.(protocol.ChunkSnapshot)
	if !ok {
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
	sections := make([]map[string]any, 0, len(snapshot.Sections))
	for _, section := range snapshot.Sections {
		rendered, err := snapshotSectionFields(section)
		if err != nil {
			return nil, fmt.Errorf("runtime-oracle: case %s: %w", c.ID, err)
		}
		sections = append(sections, rendered)
	}
	return map[string]any{
		"dimension": int32(snapshot.Dimension),
		"chunk_x":   snapshot.Chunk.X,
		"chunk_z":   snapshot.Chunk.Z,
		"revision":  strconv.FormatUint(snapshot.Revision, 10),
		"sections":  sections,
	}, nil
}

// snapshotRejectionCategory resolves the language-neutral rejection category
// for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions this group's cases can
// reach, never over a sentinel identity, and a failure with no mapping is a
// hard error. The envelope's declared-length ceilings are the capacity
// boundary; a declared compressed length the payload cannot back is the
// truncated boundary the envelope check owns; a length-complete frame the zstd
// layer rejects is the integrity boundary the frame's own checksum owns, which
// this family publishes for the first time; and the five logical-layer
// validator rejections are the invalid-value boundary the corresponding Go
// messages name.
func snapshotRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "exceeds limit"):
		return "capacity", true
	case strings.Contains(message, "compressed length does not match snapshot envelope"):
		return "truncated", true
	case strings.Contains(message, "decompress snapshot"):
		return "integrity", true
	case strings.Contains(message, "chunk snapshot revision is zero"),
		strings.Contains(message, "chunk snapshot has"),
		strings.Contains(message, "has Y"),
		strings.Contains(message, "palette slot"),
		strings.Contains(message, "unused high bits"):
		return "invalid-value", true
	}
	return "", false
}

// runSnapshotDecode executes one snapshot decode case through the real Go
// decoder named by the case's own packet key.
func runSnapshotDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := snapshotPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newSnapshotCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeServer(state, packetID, input)
	if err != nil {
		category, classified := snapshotRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := snapshotFields(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// runSnapshotEncode executes one snapshot encode case through the real Go
// encoder named by the case's own packet key and records the logical digest.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The encoded payload
// is read back through the production decoder, and the recorded digest is taken
// over the logical bytes the codec's own frame carries — never over the
// compressed bytes, which the Rust encoder legitimately publishes differently.
// The producer returns that logical payload as its encoded output, because the
// shared runner derives the recorded digest from the bytes it returns.
func runSnapshotEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := snapshotPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newSnapshotCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := snapshotPacket(c, input)
	if err != nil {
		return Outcome{}, nil, err
	}
	encodedID, payload, err := wireCodec.EncodeServer(state, packet)
	if err != nil {
		category, classified := snapshotRejectionCategory(err)
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
	fields, err := snapshotFields(c, decoded)
	if err != nil {
		return Outcome{}, nil, err
	}
	logical, err := snapshotLogicalBytes(c, payload)
	if err != nil {
		return Outcome{}, nil, err
	}
	sum := sha256.Sum256(logical)
	// The shared runner derives a case's recorded digest from the bytes its
	// producer returns, so this family returns the canonical logical payload
	// rather than the compressed frame: the recorded digest is then the logical
	// one the cross-implementation ruling names, and no corpus case ever
	// records a digest of compressed bytes the two encoders publish
	// differently.
	return Outcome{
		Kind:                 "ok",
		Category:             packetOutcomeCategory,
		Fields:               fields,
		EncodedPayloadDigest: fmt.Sprintf("sha256:%x", sum),
	}, logical, nil
}

// snapshotCorpusRoutes is the closed route map the family executes.
func snapshotCorpusRoutes() map[ConsumerRoute]GoOperation {
	return map[ConsumerRoute]GoOperation{
		{FamilyID: snapshotFamily, Version: snapshotVersion, Operation: "decode"}: runSnapshotDecode,
		{FamilyID: snapshotFamily, Version: snapshotVersion, Operation: "encode"}: runSnapshotEncode,
	}
}

// snapshotCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer
// result.
type snapshotCaseDefinition struct {
	id      string
	op      string
	input   []byte
	request *snapshotEncodeRequest
	expect  Outcome
}

// snapshotCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid
// cases use the canonical mixed and all-single vectors; the decode-side
// negatives mutate one valid Go-encoded payload at a single boundary each, and
// the encode-side negatives build a DTO the production validator refuses. The
// recorded expectations for the accepted cases are derived once here from the
// production path — the input JSON is the reviewed literal, and the group test
// pins the normalized shape and the digest's presence separately.
func snapshotCaseDefinitions(t *testing.T) []snapshotCaseDefinition {
	t.Helper()

	mixedRequest := snapshotMixedRequest()
	singleRequest := snapshotSingleRequest()
	mixedPayload := snapshotCanonicalPayload(t, mixedRequest)

	invalidValue := Outcome{Kind: "error", Category: "invalid-value"}
	capacity := Outcome{Kind: "error", Category: "capacity"}
	truncated := Outcome{Kind: "error", Category: "truncated"}
	integrity := Outcome{Kind: "error", Category: "integrity"}

	// Decode-side byte edits of the one valid payload.
	compressedCap := append([]byte(nil), mixedPayload...)
	binary.LittleEndian.PutUint32(compressedCap[4:8], snapshotMaxCompressed+1)
	decodedCap := append([]byte(nil), mixedPayload...)
	binary.LittleEndian.PutUint32(decodedCap[0:4], snapshotMaxDecoded+1)
	truncatedFrame := append([]byte(nil), mixedPayload[:len(mixedPayload)-5]...)
	checksumFailure := append([]byte(nil), mixedPayload...)
	checksumFailure[len(checksumFailure)-1] ^= 0xff

	definitions := make([]snapshotCaseDefinition, 0, 13)
	fixture := readGoSnapshotFixture(t)
	definitions = append(definitions,
		snapshotCaseDefinition{
			id:     snapshotDecodeFixtureID,
			op:     "decode",
			input:  fixture,
			expect: snapshotDecodedExpectation(t, snapshotDecodeFixtureID, fixture),
		},
		snapshotCaseDefinition{
			id:     snapshotDecodeMixedID,
			op:     "decode",
			input:  append([]byte(nil), mixedPayload...),
			expect: snapshotDecodedExpectation(t, snapshotDecodeMixedID, mixedPayload),
		},
		snapshotCaseDefinition{
			id:      snapshotEncodeMixedID,
			op:      "encode",
			request: &mixedRequest,
			expect:  snapshotEncodedExpectation(t, snapshotEncodeMixedID, mixedRequest),
		},
		snapshotCaseDefinition{
			id:      snapshotEncodeSingleID,
			op:      "encode",
			request: &singleRequest,
			expect:  snapshotEncodedExpectation(t, snapshotEncodeSingleID, singleRequest),
		},
		snapshotCaseDefinition{
			id:      snapshotRevisionZeroID,
			op:      "encode",
			request: &snapshotEncodeRequest{Dimension: 1, ChunkX: -1, ChunkZ: 0, Revision: 0, Sections: snapshotMixedSections()},
			expect:  invalidValue,
		},
		snapshotCaseDefinition{
			id: snapshotSections23ID,
			op: "encode",
			request: func() *snapshotEncodeRequest {
				mutated := snapshotCloneRequest(mixedRequest)
				mutated.Sections = mutated.Sections[:len(mutated.Sections)-1]
				return &mutated
			}(),
			expect: invalidValue,
		},
		snapshotCaseDefinition{
			id: snapshotYSwapID,
			op: "encode",
			request: func() *snapshotEncodeRequest {
				mutated := snapshotCloneRequest(mixedRequest)
				mutated.Sections[3].Y, mutated.Sections[4].Y = mutated.Sections[4].Y, mutated.Sections[3].Y
				return &mutated
			}(),
			expect: invalidValue,
		},
		snapshotCaseDefinition{
			id: snapshotPaletteSlotID,
			op: "encode",
			request: func() *snapshotEncodeRequest {
				mutated := snapshotCloneRequest(mixedRequest)
				mutated.Sections[6].Words[0] = snapshotWord(snapshotParseWord(t, mutated.Sections[6].Words[0]) | 2)
				return &mutated
			}(),
			expect: invalidValue,
		},
		snapshotCaseDefinition{
			id: snapshotDirectHighID,
			op: "encode",
			request: func() *snapshotEncodeRequest {
				mutated := snapshotCloneRequest(mixedRequest)
				mutated.Sections[18].Words[0] = snapshotWord(snapshotParseWord(t, mutated.Sections[18].Words[0]) | 1<<60)
				return &mutated
			}(),
			expect: invalidValue,
		},
		snapshotCaseDefinition{
			id:     snapshotCompressedCapID,
			op:     "decode",
			input:  compressedCap,
			expect: capacity,
		},
		snapshotCaseDefinition{
			id:     snapshotDecodedCapID,
			op:     "decode",
			input:  decodedCap,
			expect: capacity,
		},
		snapshotCaseDefinition{
			id:     snapshotTruncatedID,
			op:     "decode",
			input:  truncatedFrame,
			expect: truncated,
		},
		snapshotCaseDefinition{
			id:     snapshotChecksumID,
			op:     "decode",
			input:  checksumFailure,
			expect: integrity,
		},
	)
	return definitions
}

// snapshotLabel renders one case's asset label from its identity.
func snapshotLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// snapshotKeyPointer resolves the family's reviewed packet key for a case spec.
func snapshotKeyPointer() *PacketKeySpec {
	key := snapshotFamilyKeys[snapshotFamily]
	return &key
}

// snapshotSpec builds the case specification one derived expectation executes
// against.
func snapshotSpec(id, operation string) CaseSpec {
	return CaseSpec{
		ID:        id,
		Family:    snapshotFamily,
		Version:   snapshotVersion,
		Operation: operation,
		PacketKey: snapshotKeyPointer(),
	}
}

// snapshotParseWord parses one canonical packed-word field.
func snapshotParseWord(t *testing.T, word string) uint64 {
	t.Helper()
	parsed, err := strconv.ParseUint(word, 16, 64)
	if err != nil {
		t.Fatalf("parse packed word %q: %v", word, err)
	}
	return parsed
}

// marshalSnapshotAsset renders one of this group's JSON assets compactly.
//
// The committed corpus layout is two-space indentation, but the mixed vector's
// packed words put the indented layout of both the encode request and the
// normalized outcome above the 256 KiB JSON case budget. Every field name,
// value encoding and key order is the committed one; only the whitespace
// differs.
func marshalSnapshotAsset(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// readGoSnapshotFixture reads the committed Go chunk-snapshot fixture verbatim.
// The tracked fixture is read-only evidence and is never rewritten here.
func readGoSnapshotFixture(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(mustRepoRoot(t), "packages", "shared", "network", "codec", "testdata", "chunk-snapshot-v1.bin")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(data) < snapshotEnvelopeLength {
		t.Fatalf("fixture %s is shorter than the snapshot envelope: %d bytes", path, len(data))
	}
	return data
}

// snapshotCanonicalPayload encodes one request through the production codec and
// returns the whole Play payload. The compression itself always comes from the
// codec; this is the same entry point the producer uses.
func snapshotCanonicalPayload(t *testing.T, request snapshotEncodeRequest) []byte {
	t.Helper()
	wireCodec, err := newSnapshotCodec()
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()
	dto, err := snapshotDTO(request)
	if err != nil {
		t.Fatalf("build the request DTO: %v", err)
	}
	_, payload, err := wireCodec.EncodeServer(protocol.StatePlay, dto)
	if err != nil {
		t.Fatalf("encode the request: %v", err)
	}
	return payload
}

// snapshotRequestExpectation renders the expected outcome of one valid case from
// the request's own reviewed section list, so the expectation is the input
// contract rather than a copy of the producer's run: the production round trip
// has to be lossless for the two to agree.
func snapshotRequestExpectation(t *testing.T, id string, request snapshotEncodeRequest) Outcome {
	t.Helper()
	dto, err := snapshotDTO(request)
	if err != nil {
		t.Fatalf("case %s: build the request DTO: %v", id, err)
	}
	fields, err := snapshotFields(snapshotSpec(id, "encode"), dto)
	if err != nil {
		t.Fatalf("case %s: render the expectation: %v", id, err)
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}
}

// snapshotEncodedExpectation records one valid encode case's expectation: the
// fields the request names, beside the digest of the canonical logical payload
// the codec publishes for them.
func snapshotEncodedExpectation(t *testing.T, id string, request snapshotEncodeRequest) Outcome {
	t.Helper()
	expectation := snapshotRequestExpectation(t, id, request)
	payload := snapshotCanonicalPayload(t, request)
	logical, err := snapshotLogicalBytes(snapshotSpec(id, "encode"), payload)
	if err != nil {
		t.Fatalf("case %s: read the logical payload back: %v", id, err)
	}
	sum := sha256.Sum256(logical)
	expectation.EncodedPayloadDigest = fmt.Sprintf("sha256:%x", sum)
	return expectation
}

// snapshotDecodedExpectation records one valid decode case's expectation by
// executing the production decoder once over the reviewed payload.
func snapshotDecodedExpectation(t *testing.T, id string, payload []byte) Outcome {
	t.Helper()
	outcome, _, err := runSnapshotDecode(snapshotSpec(id, "decode"), payload)
	if err != nil {
		t.Fatalf("case %s: derive the expectation: %v", id, err)
	}
	return outcome
}

// snapshotCandidate is one reviewed case: its manifest specification, the exact
// asset bytes it publishes, and the expectation an independent execution has to
// reproduce.
type snapshotCandidate struct {
	Spec    CaseSpec
	Assets  map[string][]byte
	Expect  Outcome
	Request *snapshotEncodeRequest
}

// buildSnapshotCandidate builds one case's manifest entry and asset bytes from
// its definition.
func buildSnapshotCandidate(t *testing.T, definition snapshotCaseDefinition) snapshotCandidate {
	t.Helper()

	label := snapshotLabel(definition.id)
	inputPath := filepath.ToSlash(filepath.Join(snapshotCorpusRelDir, label+".input"+inputExtension(definition.op)))
	expectedPath := filepath.ToSlash(filepath.Join(snapshotCorpusRelDir, label+".expected.json"))

	var input []byte
	switch {
	case definition.op == "decode":
		input = append([]byte(nil), definition.input...)
	case definition.request != nil:
		rendered, err := marshalSnapshotAsset(*definition.request)
		if err != nil {
			t.Fatalf("case %s: marshal encode input: %v", definition.id, err)
		}
		input = rendered
	default:
		t.Fatalf("case %s: neither a decode payload nor an encode request", definition.id)
	}

	expected, err := marshalSnapshotAsset(definition.expect)
	if err != nil {
		t.Fatalf("case %s: render expectation: %v", definition.id, err)
	}
	// The corpus case budget bounds JSON inputs and expectations at 256 KiB;
	// an asset above it would be rejected by the corpus loader, so the producer
	// refuses it here with the case named.
	for name, asset := range map[string][]byte{inputPath: input, expectedPath: expected} {
		if definition.op == "encode" || name == expectedPath {
			if len(asset) > snapshotMaxCaseJSONBytes {
				t.Fatalf("case %s: asset %s is %d bytes above the %d-byte JSON case budget", definition.id, name, len(asset), snapshotMaxCaseJSONBytes)
			}
		}
	}
	assets := map[string][]byte{
		inputPath:    input,
		expectedPath: expected,
	}

	spec := CaseSpec{
		ID:           definition.id,
		Family:       snapshotFamily,
		Version:      snapshotVersion,
		Operation:    definition.op,
		PacketKey:    snapshotKeyPointer(),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return snapshotCandidate{Spec: spec, Assets: assets, Expect: definition.expect, Request: definition.request}
}

// snapshotMaxCaseJSONBytes is the corpus JSON case budget the assets must stay
// inside.
const snapshotMaxCaseJSONBytes = 256 * 1024

// snapshotCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func snapshotCandidates(t *testing.T, root string) []snapshotCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := snapshotCaseDefinitions(t)
	candidates := make([]snapshotCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildSnapshotCandidate(t, definition)
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

// snapshotRegisteredCase reports whether the base manifest already carries one
// case identity, so a re-merge of an integrated candidate adds nothing.
func snapshotRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// snapshotSelection is this group's registration: its cases, the Go sources its
// rules are read from, and the routes the family executes.
func snapshotSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := snapshotCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	return ProtocolSelection{
		ProducerID:  snapshotProducerID,
		Cases:       cases,
		SourcePaths: map[string][]string{snapshotFamily: snapshotServerSources()},
		Routes: []ConsumerRoute{
			{FamilyID: snapshotFamily, Version: snapshotVersion, Operation: "decode"},
			{FamilyID: snapshotFamily, Version: snapshotVersion, Operation: "encode"},
		},
	}
}

// snapshotServerSources lists the Go sources the family reads its rules from:
// the shared server codec dispatch with its decode arm, the snapshot codec that
// owns the envelope and the logical layer, and the message file that owns the
// validators.
func snapshotServerSources() []string {
	return []string{
		"packages/shared/network/codec/codec_server.go",
		"packages/shared/network/codec/chunk_codec.go",
		"packages/shared/network/protocol/snapshot.go",
	}
}

// snapshotManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func snapshotManifest(t *testing.T, root string, candidates []snapshotCandidate) Inventory {
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
		if family != snapshotFamily {
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
		t.Fatalf("encode snapshot working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write snapshot working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load snapshot working manifest: %v", err)
	}
	return loaded
}

// snapshotScratchRoot stages this group's candidate assets in a harness-owned
// temporary directory, because a corpus case has to resolve under the root the
// runner is given.
func snapshotScratchRoot(t *testing.T, root string, candidates []snapshotCandidate) string {
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

// snapshotCandidateByID resolves one candidate by its case identity.
func snapshotCandidateByID(t *testing.T, candidates []snapshotCandidate, id string) snapshotCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return snapshotCandidate{}
}

// snapshotObservation resolves one executed observation by its case identity.
func snapshotObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// snapshotExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var snapshotExportPublished bool

// snapshotCandidatesExport publishes the reviewed candidates and the complete
// merged manifest candidate through the existing external exporter and returns
// the published producer directory. An unset export variable publishes nothing.
func snapshotCandidatesExport(t *testing.T, root string, candidates []snapshotCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if snapshotExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-snapshot")
	}
	snapshotExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, snapshotProducerID, assets)
	if err != nil {
		t.Fatalf("export snapshot candidates: %v", err)
	}
	return published
}

// snapshotDecodeCaseIDs lists the valid decode cases the producer pins.
func snapshotDecodeCaseIDs() []string {
	return []string{snapshotDecodeFixtureID, snapshotDecodeMixedID}
}

// snapshotEncodeCaseIDs lists the valid encode cases the producer pins.
func snapshotEncodeCaseIDs() []string {
	return []string{snapshotEncodeMixedID, snapshotEncodeSingleID}
}

// snapshotRejectedDecodeCaseIDs lists the malformed decode cases.
func snapshotRejectedDecodeCaseIDs() []string {
	return []string{
		snapshotCompressedCapID,
		snapshotDecodedCapID,
		snapshotTruncatedID,
		snapshotChecksumID,
	}
}

// snapshotRejectedEncodeCaseIDs lists the invalid encode cases.
func snapshotRejectedEncodeCaseIDs() []string {
	return []string{
		snapshotRevisionZeroID,
		snapshotSections23ID,
		snapshotYSwapID,
		snapshotPaletteSlotID,
		snapshotDirectHighID,
	}
}

// TestProtocolSnapshotOracleDecodesEveryValidCase pins that the real Go decoder
// publishes the canonical logical fields for both valid decode cases: the
// committed fixture and the mixed vector.
func TestProtocolSnapshotOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := snapshotCandidates(t, root)

	for _, id := range snapshotDecodeCaseIDs() {
		candidate := snapshotCandidateByID(t, candidates, id)
		outcome, encoded, err := runSnapshotDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runSnapshotDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolSnapshotOracleEncodesCanonicalLogicalDigest pins that every valid
// encode case records the logical payload's digest — never the compressed
// bytes, which the Rust encoder legitimately publishes differently — and that
// the encoded payload is read back through the production decoder.
//
// The producer returns the canonical logical payload for the shared runner's
// digest derivation, so this test also pins the logical layout: the identity
// fields lead it, followed by the 24-section count.
func TestProtocolSnapshotOracleEncodesCanonicalLogicalDigest(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := snapshotCandidates(t, root)

	for _, id := range snapshotEncodeCaseIDs() {
		candidate := snapshotCandidateByID(t, candidates, id)
		outcome, logical, err := runSnapshotEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runSnapshotEncode(%s): %v", id, err)
		}
		if len(logical) == 0 {
			t.Fatalf("case %s returned no logical payload", id)
		}
		if len(logical) < 21 {
			t.Fatalf("case %s logical payload is %d bytes, below the identity header", id, len(logical))
		}
		// Identity fields lead the logical payload: dimension, chunk X, chunk Z,
		// revision, then the 24-section count.
		if dimension := int32(binary.LittleEndian.Uint32(logical[0:4])); dimension != 1 {
			t.Fatalf("case %s logical dimension = %d, want 1", id, dimension)
		}
		if chunkX := int32(binary.LittleEndian.Uint32(logical[4:8])); chunkX != -1 {
			t.Fatalf("case %s logical chunk X = %d, want -1", id, chunkX)
		}
		if chunkZ := int32(binary.LittleEndian.Uint32(logical[8:12])); chunkZ != 0 {
			t.Fatalf("case %s logical chunk Z = %d, want 0", id, chunkZ)
		}
		if revision := binary.LittleEndian.Uint64(logical[12:20]); revision != 1 {
			t.Fatalf("case %s logical revision = %d, want 1", id, revision)
		}
		if count := int(logical[20]); count != core.SectionsPerChunk {
			t.Fatalf("case %s logical section count = %d, want %d", id, count, core.SectionsPerChunk)
		}
		// The compressed payload the codec published still has to decode back
		// through the production decoder, which the read-back guard inside the
		// producer already enforced; the compressed frame itself is never the
		// recorded digest.
		payload := snapshotCanonicalPayload(t, *candidate.Request)
		if len(payload) <= snapshotEnvelopeLength {
			t.Fatalf("case %s published %d bytes, below the snapshot envelope", id, len(payload))
		}
		readBack, err := snapshotLogicalBytes(candidate.Spec, payload)
		if err != nil {
			t.Fatalf("case %s: read the logical payload back: %v", id, err)
		}
		if !bytes.Equal(readBack, logical) {
			t.Fatalf("case %s: the logical payload is not deterministic", id)
		}
		sum := sha256.Sum256(logical)
		if outcome.EncodedPayloadDigest != fmt.Sprintf("sha256:%x", sum) {
			t.Fatalf("case %s digest = %s, want sha256:%x", id, outcome.EncodedPayloadDigest, sum)
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolSnapshotOracleRejectsMalformedCasesAtTheirBoundary pins that every
// malformed decode case and invalid encode case is refused by the production
// codec and classified at the boundary that owns it: the envelope ceilings are
// capacity, a payload cut inside the compressed body is truncated, a flipped
// body byte is integrity, and the five logical-layer rejections are
// invalid-value.
func TestProtocolSnapshotOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := snapshotCandidates(t, root)

	for _, id := range snapshotRejectedDecodeCaseIDs() {
		candidate := snapshotCandidateByID(t, candidates, id)
		outcome, encoded, err := runSnapshotDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runSnapshotDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range snapshotRejectedEncodeCaseIDs() {
		candidate := snapshotCandidateByID(t, candidates, id)
		outcome, encoded, err := runSnapshotEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runSnapshotEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolSnapshotOracleNormalizesEverySectionKind pins the normalized
// logical shape with hand-written expectations, so the derived expectations
// above cannot drift away from the contract silently: the identity fields, the
// four container kinds at their column positions, the palette order, and the
// direct word that carries block 89 at the last cell of Y23.
func TestProtocolSnapshotOracleNormalizesEverySectionKind(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := snapshotCandidates(t, root)
	mixed := snapshotCandidateByID(t, candidates, snapshotEncodeMixedID)
	outcome, _, err := runSnapshotEncode(mixed.Spec, mixed.Assets[mixed.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runSnapshotEncode(%s): %v", mixed.Spec.ID, err)
	}
	if outcome.Fields["dimension"] != int32(1) {
		t.Fatalf("dimension = %#v, want 1", outcome.Fields["dimension"])
	}
	if outcome.Fields["chunk_x"] != int32(-1) || outcome.Fields["chunk_z"] != int32(0) {
		t.Fatalf("chunk = (%#v, %#v), want (-1, 0)", outcome.Fields["chunk_x"], outcome.Fields["chunk_z"])
	}
	if outcome.Fields["revision"] != "1" {
		t.Fatalf("revision = %#v, want the decimal string \"1\"", outcome.Fields["revision"])
	}
	sections, ok := outcome.Fields["sections"].([]map[string]any)
	if !ok {
		t.Fatalf("sections = %#v, want an ordered array", outcome.Fields["sections"])
	}
	if len(sections) != core.SectionsPerChunk {
		t.Fatalf("%d sections, want %d", len(sections), core.SectionsPerChunk)
	}
	for index, section := range sections {
		if section["y"] != int32(index) {
			t.Fatalf("section %d names y %#v", index, section["y"])
		}
	}
	if sections[0]["kind"] != "single" || sections[0]["block"] != uint16(0) {
		t.Fatalf("section 0 = %#v, want single block 0", sections[0])
	}
	if sections[6]["kind"] != "indexed4" {
		t.Fatalf("section 6 kind = %#v, want indexed4", sections[6]["kind"])
	}
	if !reflect.DeepEqual(sections[6]["palette"], []uint16{1, 2}) {
		t.Fatalf("section 6 palette = %#v, want [1 2]", sections[6]["palette"])
	}
	if sections[12]["kind"] != "indexed8" {
		t.Fatalf("section 12 kind = %#v, want indexed8", sections[12]["kind"])
	}
	if palette, ok := sections[12]["palette"].([]uint16); !ok || len(palette) != 90 || palette[0] != 0 || palette[89] != 89 {
		t.Fatalf("section 12 palette = %#v, want the ninety registered numbers 0..89", sections[12]["palette"])
	}
	if sections[23]["kind"] != "direct" {
		t.Fatalf("section 23 kind = %#v, want direct", sections[23]["kind"])
	}
	words, ok := sections[23]["words"].([]string)
	if !ok || len(words) == 0 {
		t.Fatalf("section 23 words = %#v, want the packed word array", sections[23]["words"])
	}
	// The last cell of the last word carries block 89: a 15-bit section packs
	// four cells per word, so the final cell sits in the high slot of the last
	// word.
	perWord := 64 / 15
	shift := uint((core.BlocksPerSection-1)%perWord) * 15
	lastCell := (snapshotParseWord(t, words[len(words)-1]) >> shift) & 0x7fff
	if lastCell != 89 {
		t.Fatalf("section 23 last cell = %d, want block 89", lastCell)
	}
	for index, word := range words {
		if len(word) != 16 {
			t.Fatalf("section 23 word %d is %d characters, want sixteen", index, len(word))
		}
	}
	if len(outcome.EncodedPayloadDigest) != len("sha256:")+64 {
		t.Fatalf("digest = %q, want a sha256 of the logical payload", outcome.EncodedPayloadDigest)
	}

	// The committed fixture's expectation keeps its own identity: overworld
	// chunk (-3, 7) at revision 19 with all three storages in column order.
	fixture := snapshotCandidateByID(t, candidates, snapshotDecodeFixtureID)
	decoded, _, err := runSnapshotDecode(fixture.Spec, fixture.Assets[fixture.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runSnapshotDecode(%s): %v", fixture.Spec.ID, err)
	}
	if decoded.Fields["dimension"] != int32(0) {
		t.Fatalf("fixture dimension = %#v, want 0", decoded.Fields["dimension"])
	}
	if decoded.Fields["chunk_x"] != int32(-3) || decoded.Fields["chunk_z"] != int32(7) {
		t.Fatalf("fixture chunk = (%#v, %#v), want (-3, 7)", decoded.Fields["chunk_x"], decoded.Fields["chunk_z"])
	}
	if decoded.Fields["revision"] != "19" {
		t.Fatalf("fixture revision = %#v, want \"19\"", decoded.Fields["revision"])
	}
	fixtureSections, ok := decoded.Fields["sections"].([]map[string]any)
	if !ok || len(fixtureSections) != core.SectionsPerChunk {
		t.Fatalf("fixture sections = %#v, want %d ordered entries", decoded.Fields["sections"], core.SectionsPerChunk)
	}
	if fixtureSections[0]["kind"] != "single" || fixtureSections[1]["kind"] != "indexed4" ||
		fixtureSections[2]["kind"] != "indexed8" || fixtureSections[3]["kind"] != "direct" {
		t.Fatalf("fixture section kinds = (%#v, %#v, %#v, %#v), want the cycling single/indexed4/indexed8/direct column",
			fixtureSections[0]["kind"], fixtureSections[1]["kind"], fixtureSections[2]["kind"], fixtureSections[3]["kind"])
	}
	if decoded.EncodedPayloadDigest != "" {
		t.Fatalf("fixture decode recorded a digest %q, want none", decoded.EncodedPayloadDigest)
	}
}

// TestProtocolSnapshotOracleExpectedFieldMutationFailsComparison pins that the
// recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer published, and replacing the recorded
// digest fails comparison against the logical payload's digest.
func TestProtocolSnapshotOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := snapshotCandidates(t, root)

	encode := snapshotCandidateByID(t, candidates, snapshotEncodeMixedID)
	produced, _, err := runSnapshotEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runSnapshotEncode(%s): %v", encode.Spec.ID, err)
	}
	mutated := encode.Expect
	fields := make(map[string]any, len(encode.Expect.Fields))
	for key, value := range encode.Expect.Fields {
		fields[key] = value
	}
	fields["revision"] = "2"
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated revision compares equal to the produced outcome")
	}
	mutatedDigest := produced
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated logical digest compares equal to the reviewed expectation")
	}

	decode := snapshotCandidateByID(t, candidates, snapshotDecodeFixtureID)
	producedDecode, _, err := runSnapshotDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runSnapshotDecode(%s): %v", decode.Spec.ID, err)
	}
	mutatedDecode := decode.Expect
	decodeFields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		decodeFields[key] = value
	}
	decodeFields["revision"] = "18"
	mutatedDecode.Fields = decodeFields
	if outcomesEqual(mutatedDecode, producedDecode) {
		t.Fatal("mutated fixture revision compares equal to the produced outcome")
	}
}

// TestProtocolSnapshotOracleRoutesExecuteEveryCase executes this group's
// complete case set through the shared packet-case runner, once per case, and
// compares every observation with the reviewed expectation.
func TestProtocolSnapshotOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := snapshotCandidates(t, root)
	manifest := snapshotManifest(t, root, candidates)
	staged := snapshotScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, snapshotCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := snapshotObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolSnapshotOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so a
// case cannot claim coverage from its name alone.
func TestProtocolSnapshotOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := snapshotCandidates(t, root)
	manifest := snapshotManifest(t, root, candidates)
	staged := snapshotScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{
		{FamilyID: snapshotFamily, Version: snapshotVersion, Operation: "decode"}: runSnapshotDecode,
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	if !strings.Contains(err.Error(), snapshotFamily+"/"+snapshotVersion+"/encode") {
		t.Fatalf("rejection %v does not name the encode route", err)
	}
}

// TestProtocolSnapshotOracleManifestMergeRegistersSnapshotRoutes pins that the
// merged manifest registers both routes and the family's case list, leaves the
// source revision alone, and records this group's provenance sources.
func TestProtocolSnapshotOracleManifestMergeRegistersSnapshotRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, snapshotSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	var row *Family
	for index := range merged.Families {
		if merged.Families[index].ID == snapshotFamily {
			row = &merged.Families[index]
			break
		}
	}
	if row == nil {
		t.Fatalf("merged manifest has no %s family", snapshotFamily)
	}
	var listed []string
	for _, c := range merged.Cases {
		if c.Family == snapshotFamily {
			listed = append(listed, c.ID)
		}
	}
	sort.Strings(listed)
	if !reflect.DeepEqual(row.Cases, listed) {
		t.Fatalf("%s family lists %v, want %v", snapshotFamily, row.Cases, listed)
	}
	if len(listed) != 13 {
		t.Fatalf("%s registers %d cases, want the reviewed thirteen", snapshotFamily, len(listed))
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
	for _, want := range snapshotServerSources() {
		if !sources[want] {
			t.Fatalf("%s provenance drops %s", snapshotFamily, want)
		}
	}

	wantCases := len(base.Cases)
	for _, candidate := range snapshotCandidates(t, root) {
		if !snapshotRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolSnapshotOracleCandidatesExportForReview publishes the reviewed
// candidates and the manifest candidate. An unset export variable publishes
// nothing, so the tracked corpus is never written by this package.
func TestProtocolSnapshotOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := snapshotCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), snapshotSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := snapshotCandidatesExport(t, root, candidates, merged)
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

// `TestRustSnapshotDecodesInGo` pins the Go decoder's acceptance of the Rust
// codec's compressed output while comparing complete logical packet values.
func TestRustSnapshotDecodesInGo(t *testing.T) {
	root, err := RepositoryRoot()
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	readFixture := func(relative string) []byte {
		t.Helper()
		payload, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if readErr != nil {
			t.Fatalf("read %s: %v", relative, readErr)
		}
		return payload
	}
	rustBytes := readFixture("packages/engine/crates/mornlea_protocol/tests/testdata/rust-chunk-snapshot-v45.bin")
	goBytes := readFixture("packages/shared/network/codec/testdata/chunk-snapshot-v1.bin")
	if bytes.Equal(rustBytes, goBytes) {
		t.Fatal("Rust and Go fixtures unexpectedly have identical compressed bytes")
	}

	wireCodec, err := codec.NewCodec()
	if err != nil {
		t.Fatalf("new Go codec: %v", err)
	}
	defer wireCodec.Close()
	rustPacket, err := wireCodec.DecodeServer(protocol.StatePlay, 0, rustBytes)
	if err != nil {
		t.Fatalf("decode Rust fixture: %v", err)
	}
	if _, ok := rustPacket.(protocol.ChunkSnapshot); !ok {
		t.Fatalf("Rust fixture decoded as %T, want protocol.ChunkSnapshot", rustPacket)
	}
	goPacket, err := wireCodec.DecodeServer(protocol.StatePlay, 0, goBytes)
	if err != nil {
		t.Fatalf("decode Go fixture: %v", err)
	}
	if !reflect.DeepEqual(rustPacket, goPacket) {
		t.Fatal("Rust and Go fixtures decode to different complete snapshots")
	}

	corrupted := bytes.Clone(rustBytes)
	corrupted[len(corrupted)-1] ^= 0xff // The final byte belongs to the zstd content checksum.
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 0, corrupted); err == nil {
		t.Fatal("Go accepted the Rust frame with a flipped content checksum")
	}
}
