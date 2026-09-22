package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file is the Go producer for the domain world-observation event family.
// It executes every committed corpus case through the current Go protocol
// DTOs: the verdict comes from `protocol.ValidateServerPacket`, which is the
// same Play-state validator the codec applies on both the encode and the
// decode side, and an admitted record is normalized field by field through the
// frozen field map. No world authority, listener, model call or native ABI is
// involved: the producer only reads immutable corpus inputs and calls
// in-process validators, and the frozen corpus files it materializes are
// read-only inputs for later consumers.
//
// The three rules this family executes are the Go `protocol.ChunkSnapshot`,
// `protocol.BlockChanges` and `protocol.ForgetChunks` records, which are the
// world observations an authoritative session publishes.
//
// Two Go wire rules deliberately stay out of this producer, because the domain
// value they describe does not carry them and a case pinning them could not be
// replayed against the Rust consumer. A section's `Y` field is its array
// index, so the producer stamps the index and the normalized map omits the
// field, and the wire-only residue a single section must not carry is not
// expressible in the domain's `Single` variant at all. The 4096 wire batch
// caps on the change and forget batches stay in the protocol layer for the
// same reason: they are transport budgets rather than semantic relations.

const (
	// domainEventWorldFamily is the corpus family this package executes. The
	// family is the existing `domain.event` row, whose eventual owner is the
	// Rust domain crate that owns the replay observation records.
	domainEventWorldFamily = "domain.event"
	// domainEventWorldVersion is the family's discovered version. A case has
	// to name its family's version, so the case identities carry this segment
	// rather than one this producer chose.
	domainEventWorldVersion = "1"
	// domainEventWorldOperation is the manifest operation name for a world
	// observation admission case.
	domainEventWorldOperation = "admit"
	// domainEventWorldConsumer is the manifest consumer the change pins for
	// this family: the Rust crate that owns these records.
	domainEventWorldConsumer = "mornlea_domain"
	// domainEventWorldCorpusRelDir is the repository-relative directory
	// holding the frozen event world corpus cases.
	domainEventWorldCorpusRelDir = "testdata/runtime-migration/cases/domain/event_world"
	// domainEventWorldProducerTestRelPath and the two producer test names
	// locate the package-local producer that executes the protocol DTOs. The
	// topic name is the entry point the domain plan's filter requires; a corpus
	// whose producer test is missing has no independently executed evidence at
	// all.
	domainEventWorldProducerTestRelPath = "packages/tools/cmd/runtime-oracle/domain_event_world_test.go"
	domainEventWorldProducerExecuteName = "TestDomainEventWorldOracleExecutesEveryCase"
	domainEventWorldProducerTopicName   = "TestDomainOracle_event_world"
	// domainEventWorldCorpusReportName is the published report file name for
	// the executed event world evidence.
	domainEventWorldCorpusReportName = "runtime-corpus-domain-event-world.json"
	// domainEventWorldSource is the primary provenance source of the section,
	// snapshot and block-change rules.
	domainEventWorldSource = "packages/shared/network/protocol/snapshot.go"

	// domainEventWorldRuleSnapshot, domainEventWorldRuleChanges and
	// domainEventWorldRuleForget are the three rule names a case names. The
	// family is shared with the player and outcome records, so the rule name
	// is the discriminator the manifest and the family router both read.
	domainEventWorldRuleSnapshot = "chunk-snapshot"
	domainEventWorldRuleChanges  = "block-changes"
	domainEventWorldRuleForget   = "forget-chunks"

	// domainEventWorldStorageSingle, domainEventWorldStorageIndexed and
	// domainEventWorldStorageDirect are the wire section storage kinds, which
	// the domain value keeps as its three variants.
	domainEventWorldStorageSingle  = 0
	domainEventWorldStorageIndexed = 1
	domainEventWorldStorageDirect  = 2
)

// domainEventWorldFamilySources is the merged provenance set the family
// records. Every entry is a file the producer's rules are read from, so a
// change to any of them is a change to the recorded evidence. The geometry and
// block-numbering files belong to the set because the section and change rules
// are the Go `core` rules those files define, and the registry file belongs to
// it because it decides which world records are published in the play state.
var domainEventWorldFamilySources = []string{
	"packages/shared/network/protocol/snapshot.go",
	"packages/shared/network/protocol/message_chunk.go",
	"packages/shared/network/protocol/registry.go",
	"packages/shared/core/block.go",
	"packages/shared/core/pos.go",
}

// domainEventWorldLabelPattern is the shape a corpus label must have: a
// lowercase slug with an optional zero-padded numeric suffix, so a boundary row
// sorts in numeric order under a lexical sort.
var domainEventWorldLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(-[0-9]+)?$`)

// domainEventWorldInput is the frozen, self-describing corpus input for one
// case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority. The three rules share one envelope
// because the snapshot and the change batch share the dimension, the chunk and
// the revision fields, and each rule reads only the fields it names. A section
// carries its storage kind plus the fields that kind names, so an absent field
// and an explicit zero stay distinguishable: a zero palette entry and a zero
// packed word are both meaningful values.
//
// The two batch arrays keep the same discipline at the record level: a case
// that carries a batch serializes it explicitly, an empty batch included, and a
// case that carries no batch at all omits the key. `MarshalJSON` owns that
// distinction so a replay consumer never has to read a missing key as an empty
// batch.
type domainEventWorldInput struct {
	Consumer string `json:"consumer"`
	Rule     string `json:"rule"`

	// Shared and snapshot fields, in the Go DTOs' declaration order.
	Dimension *int32                    `json:"dimension,omitempty"`
	Chunk     []int32                   `json:"chunk,omitempty"`
	Revision  *uint64                   `json:"revision,omitempty"`
	Sections  []domainEventWorldSection `json:"sections,omitempty"`

	// Change batch fields, which continue the shared revision with the exact
	// transition and the ordered change list.
	BaseRevision *uint64                  `json:"base_revision,omitempty"`
	NewRevision  *uint64                  `json:"new_revision,omitempty"`
	Changes      []domainEventWorldChange `json:"changes,omitempty"`

	// Forget batch fields, which retire whole chunk columns.
	Chunks [][]int32 `json:"chunks,omitempty"`
}

// domainEventWorldInputAlias is `domainEventWorldInput` without its method
// set. Marshaling through the alias is what lets `MarshalJSON` render the
// struct's own fields without calling itself.
type domainEventWorldInputAlias domainEventWorldInput

// domainEventWorldInputWire is the frozen on-disk rendering of one corpus
// input.
//
// The two batch arrays are pointers here rather than slices because Go's
// "omitempty" drops a nil slice and an empty slice alike: with plain slices an
// empty batch and a rule that names no batch would serialize to the same
// absent key, and a consumer would have to guess which one the case meant. A
// pointer separates them, because it is set only when the case carries the
// array at all. A nil array therefore stays absent from the input, an empty
// non-nil one serializes as an explicit empty list, and a non-empty one keeps
// the exact bytes it had before this rule existed.
type domainEventWorldInputWire struct {
	domainEventWorldInputAlias
	Changes *[]domainEventWorldChange `json:"changes,omitempty"`
	Chunks  *[][]int32                `json:"chunks,omitempty"`
}

// MarshalJSON renders one corpus input under the batch-array rule above. The
// shadowed pointer fields win over the alias's slice fields by Go's shallowest
// depth rule, so the alias contributes every other field unchanged and in its
// declared order.
func (in domainEventWorldInput) MarshalJSON() ([]byte, error) {
	wire := domainEventWorldInputWire{domainEventWorldInputAlias: domainEventWorldInputAlias(in)}
	if in.Changes != nil {
		wire.Changes = &in.Changes
	}
	if in.Chunks != nil {
		wire.Chunks = &in.Chunks
	}
	return json.Marshal(wire)
}

// domainEventWorldSection is the frozen rendering of one wire section.
//
// The palette and the packed words are written even when empty, because an
// empty palette is a boundary this family pins and an absent field has to stay
// distinguishable from an explicit empty list. The single block and the slot
// width are pointers for the same reason: a zero single block is air and a
// zero width is a rejection.
type domainEventWorldSection struct {
	Storage int      `json:"storage"`
	Single  *int     `json:"single,omitempty"`
	Bits    *int     `json:"bits,omitempty"`
	Palette []int    `json:"palette"`
	Packed  []uint64 `json:"packed"`
}

// domainEventWorldChange is the frozen rendering of one block write: its world
// position and the block it writes.
type domainEventWorldChange struct {
	Position []int32 `json:"position"`
	Block    int     `json:"block"`
}

// domainEventWorldCase is one frozen corpus case: its label and its input, and
// nothing else.
type domainEventWorldCase struct {
	label string
	input domainEventWorldInput
}

func domainEventWorldInt(value int) *int {
	return &value
}

func domainEventWorldInt32(value int32) *int32 {
	return &value
}

func domainEventWorldUint64(value uint64) *uint64 {
	return &value
}

// domainEventWorldAirSection is the single-storage air section, the empty
// value of a column.
func domainEventWorldAirSection() domainEventWorldSection {
	return domainEventWorldSection{
		Storage: domainEventWorldStorageSingle,
		Single:  domainEventWorldInt(0),
	}
}

// domainEventWorldColumn renders one 24-section column: the given sections at
// their indexes and air everywhere else, so a case names only the section it
// exercises.
func domainEventWorldColumn(at map[int]domainEventWorldSection) []domainEventWorldSection {
	sections := make([]domainEventWorldSection, core.SectionsPerChunk)
	for index := range sections {
		sections[index] = domainEventWorldAirSection()
	}
	for index, section := range at {
		sections[index] = section
	}
	return sections
}

// domainEventWorldIndexed4OneCell is the two-entry 4-bit palette with cell 1
// naming slot 1: sixteen cells per word, so the slot sits in word 0 shifted by
// four bits and every other cell names slot 0.
func domainEventWorldIndexed4OneCell() domainEventWorldSection {
	words := make([]uint64, protocol.SectionWords(4))
	words[0] = 1 << 4
	return domainEventWorldSection{
		Storage: domainEventWorldStorageIndexed,
		Bits:    domainEventWorldInt(4),
		Palette: []int{0, 1},
		Packed:  words,
	}
}

// domainEventWorldIndexed4Words renders the two-entry 4-bit palette with an
// explicit packed word count, so the exact-count boundary is a case rather
// than a helper detail.
func domainEventWorldIndexed4Words(words int) domainEventWorldSection {
	section := domainEventWorldIndexed4OneCell()
	packed := make([]uint64, words)
	copy(packed, section.Packed)
	section.Packed = packed
	return section
}

// domainEventWorldIndexed8NinetyIDs is the whole registered numbering as an
// 8-bit palette: ninety entries is inside the 256 the width holds, cell 0
// names the last entry and the last cell names the first, so both ends of the
// palette resolve.
func domainEventWorldIndexed8NinetyIDs() domainEventWorldSection {
	palette := make([]int, 0, 90)
	for id := 0; id < 90; id++ {
		palette = append(palette, id)
	}
	words := make([]uint64, protocol.SectionWords(8))
	words[0] = 89
	return domainEventWorldSection{
		Storage: domainEventWorldStorageIndexed,
		Bits:    domainEventWorldInt(8),
		Palette: palette,
		Packed:  words,
	}
}

// domainEventWorldDirectLastCell is the direct section with block 89 in the
// last cell: four slots per word, so the last cell is the top slot of the last
// word and the slots below it stay zero.
func domainEventWorldDirectLastCell() domainEventWorldSection {
	words := make([]uint64, protocol.SectionWords(15))
	words[len(words)-1] = 89 << 45
	return domainEventWorldSection{
		Storage: domainEventWorldStorageDirect,
		Packed:  words,
	}
}

// domainEventWorldDirectWords renders a direct section with an explicit packed
// word count and block 89 in the first cell.
func domainEventWorldDirectWords(words int) domainEventWorldSection {
	packed := make([]uint64, words)
	if words > 0 {
		packed[0] = 89
	}
	return domainEventWorldSection{
		Storage: domainEventWorldStorageDirect,
		Packed:  packed,
	}
}

// domainEventWorldSnapshotSeed is the seed snapshot: the overworld, chunk
// (3,-5), revision 7 and a column of 24 single-storage air sections.
func domainEventWorldSnapshotSeed() domainEventWorldInput {
	return domainEventWorldInput{
		Consumer:  domainEventWorldConsumer,
		Rule:      domainEventWorldRuleSnapshot,
		Dimension: domainEventWorldInt32(int32(core.Overworld)),
		Chunk:     []int32{3, -5},
		Revision:  domainEventWorldUint64(7),
		Sections:  domainEventWorldColumn(nil),
	}
}

// domainEventWorldSnapshotCase renders one snapshot row: the seed column with
// exactly one field changed, so a boundary row is always the seed plus the
// value the rule names.
func domainEventWorldSnapshotCase(label string, mutate func(*domainEventWorldInput)) domainEventWorldCase {
	input := domainEventWorldSnapshotSeed()
	if mutate != nil {
		mutate(&input)
	}
	return domainEventWorldCase{label: label, input: input}
}

// domainEventWorldChangesSeed is the seed change batch: the overworld, chunk
// (0,0), the 1 -> 2 revision transition and one stone write at the column's
// first cell.
func domainEventWorldChangesSeed() domainEventWorldInput {
	return domainEventWorldInput{
		Consumer:     domainEventWorldConsumer,
		Rule:         domainEventWorldRuleChanges,
		Dimension:    domainEventWorldInt32(int32(core.Overworld)),
		Chunk:        []int32{0, 0},
		BaseRevision: domainEventWorldUint64(1),
		NewRevision:  domainEventWorldUint64(2),
		Changes: []domainEventWorldChange{
			{Position: []int32{0, 64, 0}, Block: 2},
		},
	}
}

// domainEventWorldChangesCase renders one change batch row: the seed batch
// with exactly one field changed.
func domainEventWorldChangesCase(label string, mutate func(*domainEventWorldInput)) domainEventWorldCase {
	input := domainEventWorldChangesSeed()
	if mutate != nil {
		mutate(&input)
	}
	return domainEventWorldCase{label: label, input: input}
}

// domainEventWorldForgetSeed is the seed forget batch: the overworld and two
// chunks in the order the record carries them.
func domainEventWorldForgetSeed() domainEventWorldInput {
	return domainEventWorldInput{
		Consumer:  domainEventWorldConsumer,
		Rule:      domainEventWorldRuleForget,
		Dimension: domainEventWorldInt32(int32(core.Overworld)),
		Chunks:    [][]int32{{0, 0}, {1, -2}},
	}
}

// domainEventWorldForgetCase renders one forget batch row: the seed batch with
// exactly one field changed.
func domainEventWorldForgetCase(label string, mutate func(*domainEventWorldInput)) domainEventWorldCase {
	input := domainEventWorldForgetSeed()
	if mutate != nil {
		mutate(&input)
	}
	return domainEventWorldCase{label: label, input: input}
}

// domainEventWorldCases is the ordered case table the producer executes.
//
// The snapshot rows are the seed column plus one boundary each: the zero
// revision, the unknown dimension, the short column, the unregistered single
// block, the 4-bit two-entry palette with one cell set, the 255 and 257 word
// counts around the exact 256, the 8-bit ninety-ID palette, the duplicate,
// unregistered, oversized and empty palettes, the unknown width, the slot
// outside the palette, the direct section with block 89 in the last cell, the
// direct high bits, the short direct word count and the unregistered direct
// block. The change rows are the seed batch plus one boundary each: the empty
// revision barrier, the depths dimension, the negative chunk with the world
// span's first and last local index, the duplicate and unsorted positions, the
// three revision-transition arms, the two Y span ends, the position outside
// the announced chunk, the unregistered block and the unknown dimension. The
// forget rows are the seed batch plus the reversed unique order, the duplicate
// chunk, the empty batch and the unknown dimension.
func domainEventWorldCases() []domainEventWorldCase {
	cases := []domainEventWorldCase{
		domainEventWorldSnapshotCase("chunk-snapshot-seed", nil),
		domainEventWorldSnapshotCase("chunk-snapshot-zero-revision", func(in *domainEventWorldInput) {
			in.Revision = domainEventWorldUint64(0)
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-dimension-unknown", func(in *domainEventWorldInput) {
			in.Dimension = domainEventWorldInt32(2)
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-section-count-short", func(in *domainEventWorldInput) {
			in.Sections = in.Sections[:core.SectionsPerChunk-1]
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-single-unregistered", func(in *domainEventWorldInput) {
			in.Sections[0] = domainEventWorldSection{
				Storage: domainEventWorldStorageSingle,
				Single:  domainEventWorldInt(90),
			}
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-indexed4-one-cell", func(in *domainEventWorldInput) {
			in.Sections[5] = domainEventWorldIndexed4OneCell()
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-indexed4-words-short", func(in *domainEventWorldInput) {
			in.Sections[5] = domainEventWorldIndexed4Words(protocol.SectionWords(4) - 1)
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-indexed4-words-long", func(in *domainEventWorldInput) {
			in.Sections[5] = domainEventWorldIndexed4Words(protocol.SectionWords(4) + 1)
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-indexed8-ninety-ids", func(in *domainEventWorldInput) {
			in.Sections[9] = domainEventWorldIndexed8NinetyIDs()
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-indexed-palette-duplicate", func(in *domainEventWorldInput) {
			in.Sections[5] = domainEventWorldSection{
				Storage: domainEventWorldStorageIndexed,
				Bits:    domainEventWorldInt(4),
				Palette: []int{0, 0},
				Packed:  make([]uint64, protocol.SectionWords(4)),
			}
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-indexed-palette-unregistered", func(in *domainEventWorldInput) {
			in.Sections[5] = domainEventWorldSection{
				Storage: domainEventWorldStorageIndexed,
				Bits:    domainEventWorldInt(4),
				Palette: []int{0, 90},
				Packed:  make([]uint64, protocol.SectionWords(4)),
			}
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-indexed-palette-over-capacity", func(in *domainEventWorldInput) {
			palette := make([]int, 17)
			for id := range palette {
				palette[id] = id
			}
			in.Sections[5] = domainEventWorldSection{
				Storage: domainEventWorldStorageIndexed,
				Bits:    domainEventWorldInt(4),
				Palette: palette,
				Packed:  make([]uint64, protocol.SectionWords(4)),
			}
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-indexed-palette-empty", func(in *domainEventWorldInput) {
			in.Sections[5] = domainEventWorldSection{
				Storage: domainEventWorldStorageIndexed,
				Bits:    domainEventWorldInt(4),
				Palette: []int{},
				Packed:  make([]uint64, protocol.SectionWords(4)),
			}
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-indexed-bits-unknown", func(in *domainEventWorldInput) {
			in.Sections[5] = domainEventWorldSection{
				Storage: domainEventWorldStorageIndexed,
				Bits:    domainEventWorldInt(5),
				Palette: []int{0, 1},
				Packed:  make([]uint64, protocol.SectionWords(4)),
			}
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-indexed-slot-absent", func(in *domainEventWorldInput) {
			section := domainEventWorldIndexed4OneCell()
			// Cell 5 names palette slot 2, but the palette holds two entries.
			section.Packed[0] = 2 << 20
			in.Sections[5] = section
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-direct-last-cell", func(in *domainEventWorldInput) {
			in.Sections[core.SectionsPerChunk-1] = domainEventWorldDirectLastCell()
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-direct-high-bits", func(in *domainEventWorldInput) {
			section := domainEventWorldDirectLastCell()
			// The top four bits of a direct word are unused, so setting them
			// describes no legal section.
			section.Packed[7] |= 1 << 60
			in.Sections[5] = section
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-direct-words-short", func(in *domainEventWorldInput) {
			in.Sections[5] = domainEventWorldDirectWords(protocol.SectionWords(15) - 1)
		}),
		domainEventWorldSnapshotCase("chunk-snapshot-direct-unregistered", func(in *domainEventWorldInput) {
			section := domainEventWorldDirectLastCell()
			section.Packed[0] = 90
			in.Sections[5] = section
		}),

		domainEventWorldChangesCase("block-changes-seed", nil),
		domainEventWorldChangesCase("block-changes-empty-revision-barrier", func(in *domainEventWorldInput) {
			in.BaseRevision = domainEventWorldUint64(5)
			in.NewRevision = domainEventWorldUint64(6)
			// The revision barrier carries an empty batch rather than no
			// batch: the empty slice is what keeps the input's explicit
			// empty list distinct from a rule that names no batch at all.
			in.Changes = []domainEventWorldChange{}
		}),
		domainEventWorldChangesCase("block-changes-depths-dimension", func(in *domainEventWorldInput) {
			in.Dimension = domainEventWorldInt32(int32(core.Depths))
		}),
		domainEventWorldChangesCase("block-changes-negative-chunk", func(in *domainEventWorldInput) {
			// The column (-1,-1) holds world x and z in -16..-1 and the world
			// Y span starts at -64, so these two writes are the first and the
			// last chunk-ordered index of the column's first section.
			in.Chunk = []int32{-1, -1}
			in.Changes = []domainEventWorldChange{
				{Position: []int32{-16, core.MinY, -16}, Block: 2},
				{Position: []int32{-1, core.MinY, -1}, Block: 3},
			}
		}),
		domainEventWorldChangesCase("block-changes-duplicate-position", func(in *domainEventWorldInput) {
			in.Changes = []domainEventWorldChange{
				{Position: []int32{0, 64, 0}, Block: 2},
				{Position: []int32{0, 64, 0}, Block: 3},
			}
		}),
		domainEventWorldChangesCase("block-changes-unsorted", func(in *domainEventWorldInput) {
			// The same two writes of the negative column in the opposite
			// order, so the broken rule is the canonical order rather than
			// the chunk membership.
			in.Chunk = []int32{-1, -1}
			in.Changes = []domainEventWorldChange{
				{Position: []int32{-1, -64, -1}, Block: 2},
				{Position: []int32{-16, -64, -16}, Block: 3},
			}
		}),
		domainEventWorldChangesCase("block-changes-revision-zero-base", func(in *domainEventWorldInput) {
			in.BaseRevision = domainEventWorldUint64(0)
			in.NewRevision = domainEventWorldUint64(1)
		}),
		domainEventWorldChangesCase("block-changes-revision-overflow", func(in *domainEventWorldInput) {
			in.BaseRevision = domainEventWorldUint64(math.MaxUint64)
			in.NewRevision = domainEventWorldUint64(0)
		}),
		domainEventWorldChangesCase("block-changes-revision-gap", func(in *domainEventWorldInput) {
			in.NewRevision = domainEventWorldUint64(3)
		}),
		domainEventWorldChangesCase("block-changes-y-below-world", func(in *domainEventWorldInput) {
			in.Changes = []domainEventWorldChange{
				{Position: []int32{0, core.MinY - 1, 0}, Block: 2},
			}
		}),
		domainEventWorldChangesCase("block-changes-y-above-world", func(in *domainEventWorldInput) {
			in.Changes = []domainEventWorldChange{
				{Position: []int32{0, core.MaxY, 0}, Block: 2},
			}
		}),
		domainEventWorldChangesCase("block-changes-position-outside-chunk", func(in *domainEventWorldInput) {
			in.Changes = []domainEventWorldChange{
				{Position: []int32{16, 64, 0}, Block: 2},
			}
		}),
		domainEventWorldChangesCase("block-changes-unregistered-block", func(in *domainEventWorldInput) {
			in.Changes = []domainEventWorldChange{
				{Position: []int32{0, 64, 0}, Block: 90},
			}
		}),
		domainEventWorldChangesCase("block-changes-dimension-unknown", func(in *domainEventWorldInput) {
			in.Dimension = domainEventWorldInt32(2)
		}),

		domainEventWorldForgetCase("forget-chunks-seed", nil),
		domainEventWorldForgetCase("forget-chunks-reversed-unique-order", func(in *domainEventWorldInput) {
			in.Chunks = [][]int32{{5, 5}, {-1, -1}, {0, 0}}
		}),
		domainEventWorldForgetCase("forget-chunks-duplicate", func(in *domainEventWorldInput) {
			in.Chunks = [][]int32{{0, 0}, {0, 0}}
		}),
		domainEventWorldForgetCase("forget-chunks-empty", func(in *domainEventWorldInput) {
			// An empty forget batch is carried, not absent, for the same
			// reason as the change batch's revision barrier.
			in.Chunks = [][]int32{}
		}),
		domainEventWorldForgetCase("forget-chunks-dimension-unknown", func(in *domainEventWorldInput) {
			in.Dimension = domainEventWorldInt32(2)
		}),
	}
	return cases
}

// runDomainEventWorld executes one corpus case through the current Go protocol
// world-observation DTOs.
//
// The verdict always comes from `protocol.ValidateServerPacket` in the play
// state, which is the same validator the codec applies on both the encode and
// the decode side. The rule name and the rejection category come from the same
// bounds the DTO reads, and the two are cross-checked against each other, so a
// classification that disagrees with the authority fails the run instead of
// publishing a plausible-looking rejection.
func runDomainEventWorld(c CaseSpec, input []byte) (Outcome, []byte, error) {
	spec, err := domainEventWorldDecodeInput(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	if spec.Consumer != domainEventWorldConsumer {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names consumer %q, want %q", c.ID, spec.Consumer, domainEventWorldConsumer)
	}

	switch spec.Rule {
	case domainEventWorldRuleSnapshot:
		return domainEventWorldRunSnapshot(c, spec)
	case domainEventWorldRuleChanges:
		return domainEventWorldRunChanges(c, spec)
	case domainEventWorldRuleForget:
		return domainEventWorldRunForget(c, spec)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, spec.Rule)
	}
}

// domainEventWorldRunSnapshot admits one chunk snapshot, which is the only
// record in this family that carries a section array.
func domainEventWorldRunSnapshot(c CaseSpec, spec domainEventWorldInput) (Outcome, []byte, error) {
	snapshot, err := domainEventWorldSnapshot(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventWorldNormalizeSnapshot(snapshot)
	category, rule := domainEventWorldSnapshotRule(snapshot)
	return domainEventWorldFinish(c, domainEventWorldRuleSnapshot, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, snapshot) == nil)
}

// domainEventWorldRunChanges admits one block-change batch.
func domainEventWorldRunChanges(c CaseSpec, spec domainEventWorldInput) (Outcome, []byte, error) {
	changes, err := domainEventWorldBlockChanges(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventWorldNormalizeChanges(changes)
	category, rule := domainEventWorldChangesRule(changes)
	return domainEventWorldFinish(c, domainEventWorldRuleChanges, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, changes) == nil)
}

// domainEventWorldRunForget admits one forget batch, whose entire payload is
// the dimension and the chunk list.
func domainEventWorldRunForget(c CaseSpec, spec domainEventWorldInput) (Outcome, []byte, error) {
	forget, err := domainEventWorldForgetChunks(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventWorldNormalizeForget(forget)
	category, rule := domainEventWorldForgetRule(forget)
	return domainEventWorldFinish(c, domainEventWorldRuleForget, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, forget) == nil)
}

// domainEventWorldSnapshot resolves one chunk snapshot from its frozen
// envelope. Every field is required, because the record is complete by
// definition: an absent field would silently become a zero and a zero revision
// or a zero dimension is a meaningful value.
func domainEventWorldSnapshot(c CaseSpec, spec domainEventWorldInput) (protocol.ChunkSnapshot, error) {
	if spec.Dimension == nil {
		return protocol.ChunkSnapshot{}, fmt.Errorf("runtime-oracle: case %s requires a dimension", c.ID)
	}
	chunk, err := domainEventWorldChunkPos(c, "chunk", spec.Chunk)
	if err != nil {
		return protocol.ChunkSnapshot{}, err
	}
	if spec.Revision == nil {
		return protocol.ChunkSnapshot{}, fmt.Errorf("runtime-oracle: case %s requires a revision", c.ID)
	}
	if spec.Sections == nil {
		return protocol.ChunkSnapshot{}, fmt.Errorf("runtime-oracle: case %s requires sections", c.ID)
	}
	sections := make([]protocol.SectionData, 0, len(spec.Sections))
	for index, section := range spec.Sections {
		data, err := domainEventWorldSectionData(c, index, section)
		if err != nil {
			return protocol.ChunkSnapshot{}, err
		}
		sections = append(sections, data)
	}
	return protocol.ChunkSnapshot{
		Dimension: core.DimensionID(*spec.Dimension),
		Chunk:     chunk,
		Revision:  *spec.Revision,
		Sections:  sections,
	}, nil
}

// domainEventWorldSectionData resolves one section from its frozen rendering.
//
// The section's `Y` field is stamped from its array index rather than read
// from the case: the domain value has no Y because the position in the column
// is the section index, so a case cannot name a Y that disagrees with the
// order the record defines. A direct section's width and empty palette are
// properties of the representation rather than case fields for the same
// reason.
func domainEventWorldSectionData(c CaseSpec, index int, section domainEventWorldSection) (protocol.SectionData, error) {
	data := protocol.SectionData{Y: int32(index)}
	switch section.Storage {
	case domainEventWorldStorageSingle:
		if section.Single == nil {
			return data, fmt.Errorf("runtime-oracle: case %s section %d requires a single block", c.ID, index)
		}
		single, err := domainEventWorldBlockID(c, "single", section.Single)
		if err != nil {
			return data, err
		}
		data.Storage = protocol.SectionSingle
		data.Single = single
	case domainEventWorldStorageIndexed:
		if section.Bits == nil {
			return data, fmt.Errorf("runtime-oracle: case %s section %d requires bits", c.ID, index)
		}
		bits, err := domainEventWorldByte(c, "bits", section.Bits)
		if err != nil {
			return data, err
		}
		if section.Palette == nil {
			return data, fmt.Errorf("runtime-oracle: case %s section %d requires a palette", c.ID, index)
		}
		palette, err := domainEventWorldPalette(c, "palette", section.Palette)
		if err != nil {
			return data, err
		}
		data.Storage = protocol.SectionIndexed
		data.Bits = bits
		data.Palette = palette
		data.Packed = section.Packed
	case domainEventWorldStorageDirect:
		data.Storage = protocol.SectionDirect
		data.Bits = 15
		data.Packed = section.Packed
	default:
		return data, fmt.Errorf("runtime-oracle: case %s section %d names unknown storage %d", c.ID, index, section.Storage)
	}
	return data, nil
}

// domainEventWorldBlockChanges resolves one block-change batch from its frozen
// envelope.
func domainEventWorldBlockChanges(c CaseSpec, spec domainEventWorldInput) (protocol.BlockChanges, error) {
	if spec.Dimension == nil {
		return protocol.BlockChanges{}, fmt.Errorf("runtime-oracle: case %s requires a dimension", c.ID)
	}
	chunk, err := domainEventWorldChunkPos(c, "chunk", spec.Chunk)
	if err != nil {
		return protocol.BlockChanges{}, err
	}
	if spec.BaseRevision == nil {
		return protocol.BlockChanges{}, fmt.Errorf("runtime-oracle: case %s requires a base revision", c.ID)
	}
	if spec.NewRevision == nil {
		return protocol.BlockChanges{}, fmt.Errorf("runtime-oracle: case %s requires a new revision", c.ID)
	}
	changes := make([]protocol.BlockChange, 0, len(spec.Changes))
	for index, change := range spec.Changes {
		position, err := domainEventWorldBlockPos(c, fmt.Sprintf("changes[%d].position", index), change.Position)
		if err != nil {
			return protocol.BlockChanges{}, err
		}
		block, err := domainEventWorldBlockID(c, fmt.Sprintf("changes[%d].block", index), domainEventWorldInt(change.Block))
		if err != nil {
			return protocol.BlockChanges{}, err
		}
		changes = append(changes, protocol.BlockChange{Position: position, Block: block})
	}
	return protocol.BlockChanges{
		Dimension:    core.DimensionID(*spec.Dimension),
		Chunk:        chunk,
		BaseRevision: *spec.BaseRevision,
		NewRevision:  *spec.NewRevision,
		Changes:      changes,
	}, nil
}

// domainEventWorldForgetChunks resolves one forget batch from its frozen
// envelope.
func domainEventWorldForgetChunks(c CaseSpec, spec domainEventWorldInput) (protocol.ForgetChunks, error) {
	if spec.Dimension == nil {
		return protocol.ForgetChunks{}, fmt.Errorf("runtime-oracle: case %s requires a dimension", c.ID)
	}
	chunks := make([]core.ChunkPos, 0, len(spec.Chunks))
	for index, chunk := range spec.Chunks {
		position, err := domainEventWorldChunkPos(c, fmt.Sprintf("chunks[%d]", index), chunk)
		if err != nil {
			return protocol.ForgetChunks{}, err
		}
		chunks = append(chunks, position)
	}
	return protocol.ForgetChunks{
		Dimension: core.DimensionID(*spec.Dimension),
		Chunks:    chunks,
	}, nil
}

// domainEventWorldFinish records one admission.
//
// The classification is cross-checked against the authority's verdict, a
// rejection publishes the broken rule beside its category, and an admitted
// record publishes its normalized field map. No codec round trip is recorded:
// this family pins the semantic field values, and the wire layout of these
// packets belongs to the protocol nodes that port the codecs.
func domainEventWorldFinish(c CaseSpec, subject string, fields map[string]any, category, rule string, admitted bool) (Outcome, []byte, error) {
	if admitted == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with the protocol validator (admitted=%v)", c.ID, rule, admitted)
	}
	if !admitted {
		fields["rule"] = rule
		return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
	}
	return Outcome{Kind: "ok", Category: subject, Fields: fields}, nil, nil
}

// domainEventWorldNormalizeSnapshot renders one snapshot in the normalized
// field map. The u64 revision is a decimal string, the i32 fields are JSON
// integers, and a section publishes the fields its storage kind names: the
// single block, the indexed width, palette and packed words, or the direct
// packed words. The section's wire Y is deliberately absent, because the
// domain value has no Y field.
func domainEventWorldNormalizeSnapshot(snapshot protocol.ChunkSnapshot) map[string]any {
	sections := make([]map[string]any, 0, len(snapshot.Sections))
	for _, section := range snapshot.Sections {
		sections = append(sections, domainEventWorldNormalizeSection(section))
	}
	return map[string]any{
		"dimension": int(snapshot.Dimension),
		"chunk":     []int{int(snapshot.Chunk.X), int(snapshot.Chunk.Z)},
		"revision":  strconv.FormatUint(snapshot.Revision, 10),
		"sections":  sections,
	}
}

// domainEventWorldNormalizeSection renders one section in the normalized field
// map. The packed words are decimal strings because a u64 field is not a JSON
// integer in the frozen vocabulary, and the palette entries are JSON integers
// because a u16 field is.
func domainEventWorldNormalizeSection(section protocol.SectionData) map[string]any {
	switch section.Storage {
	case protocol.SectionSingle:
		return map[string]any{
			"storage": "single",
			"single":  int(section.Single),
		}
	case protocol.SectionIndexed:
		return map[string]any{
			"storage": "indexed",
			"bits":    int(section.Bits),
			"palette": domainEventWorldBlockIDList(section.Palette),
			"packed":  domainEventWorldWords(section.Packed),
		}
	default:
		return map[string]any{
			"storage": "direct",
			"packed":  domainEventWorldWords(section.Packed),
		}
	}
}

// domainEventWorldNormalizeChanges renders one change batch in the normalized
// field map.
func domainEventWorldNormalizeChanges(changes protocol.BlockChanges) map[string]any {
	rendered := make([]map[string]any, 0, len(changes.Changes))
	for _, change := range changes.Changes {
		rendered = append(rendered, map[string]any{
			"position": []int{
				int(change.Position.X),
				int(change.Position.Y),
				int(change.Position.Z),
			},
			"block": int(change.Block),
		})
	}
	return map[string]any{
		"dimension":     int(changes.Dimension),
		"chunk":         []int{int(changes.Chunk.X), int(changes.Chunk.Z)},
		"base_revision": strconv.FormatUint(changes.BaseRevision, 10),
		"new_revision":  strconv.FormatUint(changes.NewRevision, 10),
		"changes":       rendered,
	}
}

// domainEventWorldNormalizeForget renders one forget batch in the normalized
// field map. The chunk order is the record's order, so the map proves the
// sequence a replay has to observe.
func domainEventWorldNormalizeForget(forget protocol.ForgetChunks) map[string]any {
	chunks := make([][]int, 0, len(forget.Chunks))
	for _, chunk := range forget.Chunks {
		chunks = append(chunks, []int{int(chunk.X), int(chunk.Z)})
	}
	return map[string]any{
		"dimension": int(forget.Dimension),
		"chunks":    chunks,
	}
}

// domainEventWorldSnapshotRule names the first rule one snapshot breaks, in
// the same order the Go validator checks them: the dimension, the revision,
// the section count, and then every section in column order.
func domainEventWorldSnapshotRule(snapshot protocol.ChunkSnapshot) (string, string) {
	if snapshot.Dimension != core.Overworld && snapshot.Dimension != core.Depths {
		return "invalid-enum", "chunk_snapshot.dimension"
	}
	if snapshot.Revision == 0 {
		return "invalid-value", "chunk_snapshot.revision_nonzero"
	}
	if len(snapshot.Sections) != core.SectionsPerChunk {
		return "invalid-value", "chunk_snapshot.section_count"
	}
	for _, section := range snapshot.Sections {
		if category, rule := domainEventWorldSectionRule(section); rule != "" {
			return category, "chunk_snapshot." + rule
		}
	}
	return "", ""
}

// domainEventWorldSectionRule names the first rule one section breaks, in the
// same order the Go `SectionData.Validate` checks them.
//
// A rejection classifies as `invalid-enum` when the broken rule is an
// unregistered block number, an unknown dimension or an unknown slot width,
// and as `invalid-value` otherwise. That is the classification rule a Rust
// consumer of this family applies to its own rejection variants, so the two
// implementations cannot disagree about a category.
func domainEventWorldSectionRule(section protocol.SectionData) (string, string) {
	switch section.Storage {
	case protocol.SectionSingle:
		if !protocol.ValidBlockID(section.Single) {
			return "invalid-enum", "section_single_registered"
		}
	case protocol.SectionIndexed:
		if section.Bits != 4 && section.Bits != 8 {
			return "invalid-enum", "section_indexed_bits"
		}
		if len(section.Palette) == 0 || len(section.Palette) > 1<<section.Bits {
			return "invalid-value", "section_palette_bounds"
		}
		seen := make(map[core.BlockID]struct{}, len(section.Palette))
		for _, id := range section.Palette {
			if !protocol.ValidBlockID(id) {
				return "invalid-enum", "section_palette_registered"
			}
			if _, duplicate := seen[id]; duplicate {
				return "invalid-enum", "section_palette_unique"
			}
			seen[id] = struct{}{}
		}
		if len(section.Packed) != protocol.SectionWords(section.Bits) {
			return "invalid-value", "section_words_exact"
		}
		for index := 0; index < core.BlocksPerSection; index++ {
			if protocol.ReadSectionPacked(section.Packed, section.Bits, index) >= uint32(len(section.Palette)) {
				return "invalid-enum", "section_slot_in_palette"
			}
		}
	case protocol.SectionDirect:
		if len(section.Packed) != protocol.SectionWords(15) {
			return "invalid-value", "section_words_exact"
		}
		for _, word := range section.Packed {
			if word>>60 != 0 {
				return "invalid-value", "section_direct_high_bits"
			}
		}
		for index := 0; index < core.BlocksPerSection; index++ {
			if !core.RegisteredBlock(core.BlockID(protocol.ReadSectionPacked(section.Packed, 15, index))) {
				return "invalid-enum", "section_direct_block_registered"
			}
		}
	}
	return "", ""
}

// domainEventWorldChangesRule names the first rule one change batch breaks, in
// the same order the Go validator checks them: the dimension, the revision
// transition, and then every change's block, Y span, chunk membership and
// position in the canonical order.
func domainEventWorldChangesRule(changes protocol.BlockChanges) (string, string) {
	if changes.Dimension != core.Overworld && changes.Dimension != core.Depths {
		return "invalid-enum", "block_changes.dimension"
	}
	// The saturated base is checked before the transition is computed, so the
	// addition never wraps into a transition that looks legal.
	if changes.BaseRevision == 0 || changes.BaseRevision == math.MaxUint64 ||
		changes.NewRevision != changes.BaseRevision+1 {
		return "invalid-value", "block_changes.revision_transition"
	}
	var previous uint32
	for index, change := range changes.Changes {
		if !protocol.ValidBlockID(change.Block) {
			return "invalid-enum", "block_changes.block_registered"
		}
		if change.Position.Y < core.MinY || change.Position.Y >= core.MaxY {
			return "invalid-value", "block_changes.position_y_span"
		}
		if change.Position.Chunk() != changes.Chunk {
			return "invalid-value", "block_changes.position_chunk"
		}
		blockIndex := domainEventWorldChunkBlockIndex(change.Position)
		if index > 0 && blockIndex <= previous {
			return "invalid-value", "block_changes.strictly_increasing_index"
		}
		previous = blockIndex
	}
	return "", ""
}

// domainEventWorldForgetRule names the first rule one forget batch breaks: the
// dimension, then the nonempty and unique position rules.
func domainEventWorldForgetRule(forget protocol.ForgetChunks) (string, string) {
	if forget.Dimension != core.Overworld && forget.Dimension != core.Depths {
		return "invalid-enum", "forget_chunks.dimension"
	}
	if len(forget.Chunks) < 1 {
		return "invalid-value", "forget_chunks.nonempty_positions"
	}
	seen := make(map[core.ChunkPos]struct{}, len(forget.Chunks))
	for _, chunk := range forget.Chunks {
		if _, duplicate := seen[chunk]; duplicate {
			return "invalid-value", "forget_chunks.unique_positions"
		}
		seen[chunk] = struct{}{}
	}
	return "", ""
}

// domainEventWorldChunkBlockIndex computes the chunk-ordered block index one
// position names, which is the order the Go block-change validator compares.
//
// The producer keeps this copy because the protocol package's own
// `chunkBlockIndex` is private, and the copy is pinned by the same boundary
// cases the domain test pins: the negative column's first and last index and
// the world span's last index.
func domainEventWorldChunkBlockIndex(position core.BlockPos) uint32 {
	x, y, z := position.Local()
	return uint32(position.SectionIndex()*core.BlocksPerSection +
		y*core.SectionSize*core.SectionSize +
		z*core.SectionSize +
		x)
}

// domainEventWorldWords renders packed words as the decimal strings the
// normalized vocabulary uses for a u64 field.
func domainEventWorldWords(words []uint64) []string {
	rendered := make([]string, 0, len(words))
	for _, word := range words {
		rendered = append(rendered, strconv.FormatUint(word, 10))
	}
	return rendered
}

// domainEventWorldBlockIDList renders block numbers as the JSON integers the
// normalized vocabulary uses for a u16 field.
func domainEventWorldBlockIDList(ids []core.BlockID) []int {
	rendered := make([]int, 0, len(ids))
	for _, id := range ids {
		rendered = append(rendered, int(id))
	}
	return rendered
}

// domainEventWorldChunkPos resolves one chunk column from its two coordinates.
func domainEventWorldChunkPos(c CaseSpec, name string, coordinates []int32) (core.ChunkPos, error) {
	if len(coordinates) != 2 {
		return core.ChunkPos{}, fmt.Errorf("runtime-oracle: case %s requires two %s coordinates", c.ID, name)
	}
	return core.ChunkPos{X: coordinates[0], Z: coordinates[1]}, nil
}

// domainEventWorldBlockPos resolves one block position from its three
// coordinates.
func domainEventWorldBlockPos(c CaseSpec, name string, coordinates []int32) (core.BlockPos, error) {
	if len(coordinates) != 3 {
		return core.BlockPos{}, fmt.Errorf("runtime-oracle: case %s requires three %s coordinates", c.ID, name)
	}
	return core.BlockPos{X: coordinates[0], Y: coordinates[1], Z: coordinates[2]}, nil
}

// domainEventWorldPalette resolves one palette from its block numbers.
func domainEventWorldPalette(c CaseSpec, name string, entries []int) ([]core.BlockID, error) {
	palette := make([]core.BlockID, 0, len(entries))
	for index, entry := range entries {
		id, err := domainEventWorldBlockID(c, fmt.Sprintf("%s[%d]", name, index), domainEventWorldInt(entry))
		if err != nil {
			return nil, err
		}
		palette = append(palette, id)
	}
	return palette, nil
}

// domainEventWorldBlockID resolves one u16 block field a case names as a
// decimal integer.
func domainEventWorldBlockID(c CaseSpec, name string, value *int) (core.BlockID, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	if *value < 0 || *value > math.MaxUint16 {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %d, which is outside 0..65535", c.ID, name, *value)
	}
	return core.BlockID(*value), nil
}

// domainEventWorldByte resolves one u8 field a case names as a decimal
// integer.
func domainEventWorldByte(c CaseSpec, name string, value *int) (uint8, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	if *value < 0 || *value > math.MaxUint8 {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %d, which is outside 0..255", c.ID, name, *value)
	}
	return uint8(*value), nil
}

// domainEventWorldDecodeInput reads one frozen corpus input and rejects
// trailing content, so a producer never executes bytes the case did not name.
func domainEventWorldDecodeInput(data []byte) (domainEventWorldInput, error) {
	var spec domainEventWorldInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return domainEventWorldInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainEventWorldInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}

// domainEventWorldRecord pairs one executed case with the input and outcome
// the producer produced for it.
type domainEventWorldRecord struct {
	label   string
	input   domainEventWorldInput
	outcome Outcome
}

// domainEventWorldExecute runs the whole case table through the producer and
// returns one record per case in table order.
func domainEventWorldExecute(t *testing.T) []domainEventWorldRecord {
	t.Helper()

	cases := domainEventWorldCases()
	records := make([]domainEventWorldRecord, 0, len(cases))
	for _, entry := range cases {
		input, err := json.MarshalIndent(entry.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", entry.label, err)
		}
		input = append(input, '\n')
		outcome, _, err := runDomainEventWorld(CaseSpec{ID: domainEventWorldCaseID(entry.label)}, input)
		if err != nil {
			t.Fatalf("execute case %s: %v", entry.label, err)
		}
		records = append(records, domainEventWorldRecord{label: entry.label, input: entry.input, outcome: outcome})
	}
	return records
}

// `domainEventWorldSyncCorpus` compares committed assets and optionally exports a
// complete producer candidate for controller review.
//
// Ordinary runs compare committed assets read-only against current producer
// output. Explicit `RUNTIME_ORACLE_EXPORT_DIR` publication writes the complete
// candidate to a fresh external directory and never mutates tracked assets.
func domainEventWorldSyncCorpus(t *testing.T, records []domainEventWorldRecord) {
	t.Helper()

	root := mustRepoRoot(t)
	corpusDir := filepath.Join(root, filepath.FromSlash(domainEventWorldCorpusRelDir))
	want := make(map[string][]byte, len(records)*2)
	for _, record := range records {
		input, err := json.MarshalIndent(record.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", record.label, err)
		}
		outcome, err := json.MarshalIndent(record.outcome, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus outcome for %s: %v", record.label, err)
		}
		want[record.label+".input.json"] = append(input, '\n')
		want[record.label+".expected.json"] = append(outcome, '\n')
	}

	for relative, data := range want {
		target := filepath.Join(corpusDir, filepath.FromSlash(relative))
		committed, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read frozen corpus case %s: %v", relative, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("frozen corpus case %s drifted from the executed protocol DTO", relative)
		}
	}

	var committed []string
	if err := filepath.WalkDir(corpusDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		relative, relErr := filepath.Rel(corpusDir, path)
		if relErr != nil {
			return relErr
		}
		committed = append(committed, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		t.Fatalf("walk corpus directory: %v", err)
	}
	sort.Strings(committed)
	for _, relative := range committed {
		if _, expected := want[relative]; !expected {
			t.Errorf("frozen corpus case %s is not produced by any executed table row", relative)
		}
	}

	var relatives []string
	for relative := range want {
		relatives = append(relatives, relative)
	}
	sort.Strings(relatives)
	assets := make([]generatedAsset, 0, len(relatives))
	for _, relative := range relatives {
		assets = append(assets, generatedAsset{
			RelativePath: relative,
			Data:         want[relative],
		})
	}
	exportGeneratedAssetsFromEnvironment(t, root, "runtime-oracle/domain-event-world", assets)
}

// domainEventWorldCaseID renders the manifest case identity one corpus label
// carries. The version segment is the family's published version, because a
// case has to name the version of the family it belongs to.
func domainEventWorldCaseID(label string) string {
	return domainEventWorldFamily + "/" + domainEventWorldVersion + "/" + label
}

// `domainEventWorldWorkingManifest` clones the merged committed manifest into
// a producer-scoped selection stored in harness-owned temporary storage.
// `Cases` is narrowed to the world observation cases and unrelated family case
// lists are cleared. The existing `domain.event` identity is retained while its
// provenance and case list are replaced with this producer's current selection
// for `ReconcileWorking`.
func domainEventWorldWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	cases := domainEventWorldCorpusCases(t, root)

	cloned := Inventory{
		SchemaVersion:  frozen.SchemaVersion,
		SourceRevision: frozen.SourceRevision,
		Identities:     frozen.Identities,
		Families:       append([]Family(nil), frozen.Families...),
		Cases:          cases,
	}
	caseIDs := make([]string, 0, len(cases))
	for _, c := range cases {
		caseIDs = append(caseIDs, c.ID)
	}
	sort.Strings(caseIDs)
	sources := make([]SourceSpec, 0, len(domainEventWorldFamilySources))
	for _, relative := range domainEventWorldFamilySources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	registered := false
	for index := range cloned.Families {
		if cloned.Families[index].ID != domainEventWorldFamily {
			cloned.Families[index].Cases = nil
			continue
		}
		cloned.Families[index].Sources = sources
		cloned.Families[index].Cases = caseIDs
		registered = true
	}
	if !registered {
		t.Fatalf("working manifest has no %s family", domainEventWorldFamily)
	}

	encoded, err := encodeInventory(cloned)
	if err != nil {
		t.Fatalf("encode working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load working manifest: %v", err)
	}
	return loaded
}

// domainEventWorldCorpusCases reads the frozen corpus and registers one case
// per committed input, with digests proven against the files on disk.
func domainEventWorldCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(domainEventWorldCorpusRelDir))
	var inputs []string
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".input.json") {
			inputs = append(inputs, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk event world corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no event world case under %s", domainEventWorldCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		label := strings.TrimSuffix(filepath.ToSlash(relative), ".input.json")
		if label == "" || !domainEventWorldLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug with an optional numeric suffix", filepath.ToSlash(relative), label)
		}
		envelope := domainEventWorldReadEnvelope(t, input)
		if envelope.Consumer != domainEventWorldConsumer {
			t.Fatalf("corpus case %s names consumer %q, want %q", label, envelope.Consumer, domainEventWorldConsumer)
		}
		if strings.TrimSpace(envelope.Rule) == "" {
			t.Fatalf("corpus case %s names no rule", label)
		}

		inputHash, err := hashFile(input)
		if err != nil {
			t.Fatalf("hash %s: %v", label, err)
		}
		expectedPath := strings.TrimSuffix(input, ".input.json") + ".expected.json"
		expectedHash, err := hashFile(expectedPath)
		if err != nil {
			t.Fatalf("hash %s: %v", label+".expected.json", err)
		}
		cases = append(cases, CaseSpec{
			ID:           domainEventWorldCaseID(label),
			Family:       domainEventWorldFamily,
			Version:      domainEventWorldVersion,
			Operation:    domainEventWorldOperation,
			Input:        AssetRef{Path: domainEventWorldCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: domainEventWorldCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: domainEventWorldConsumer,
		})
	}
	return cases
}

// domainEventWorldReadEnvelope reads the provenance envelope of one frozen
// corpus input.
func domainEventWorldReadEnvelope(t *testing.T, path string) domainEventWorldInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope domainEventWorldInput
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// domainEventWorldCorpusPath renders one absolute corpus path as the
// repository-relative slash path the manifest requires.
func domainEventWorldCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}

// TestDomainEventWorldOracleExecutesEveryCase runs the whole case table
// through the real Go protocol DTOs, proves the frozen corpus still matches
// what they produce, and then runs the same cases through the production
// runner so the executed evidence satisfies the completeness rules a published
// trace report does.
func TestDomainEventWorldOracleExecutesEveryCase(t *testing.T) {
	records := domainEventWorldExecute(t)
	domainEventWorldSyncCorpus(t, records)

	root := mustRepoRoot(t)
	manifest := domainEventWorldWorkingManifest(t, root)
	if len(manifest.Cases) != len(records) {
		t.Fatalf("working manifest registers %d cases, want %d (one per executed table row)", len(manifest.Cases), len(records))
	}
	domainEventWorldAssertCaseSpecs(t, root, manifest)
	domainEventWorldAssertOutcomesDistinguishCases(t, records)

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for _, obs := range observations {
		c := domainEventWorldCaseByID(t, manifest, obs.CaseID)
		expected := readExpectedOutcome(t, root, c)
		if !outcomesEqual(obs.Outcome, expected) {
			t.Fatalf("case %s produced %#v, want %#v", obs.CaseID, obs.Outcome, expected)
		}
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	for _, obs := range trace.Observations {
		c := domainEventWorldCaseByID(t, manifest, obs.CaseID)
		if obs.ExpectedDigest != c.Expected.SHA256 {
			t.Fatalf("observation for %s carries expected digest %s, want %s", obs.CaseID, obs.ExpectedDigest, c.Expected.SHA256)
		}
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event world evidence failed trace validation: %v", err)
	}
}

// TestDomainEventWorldOracleCaseIdentitiesAreTheRustDomainConsumer pins the
// manifest identity of every event world case: the operation, the family
// version, the family and the consumer the change names for the Rust crate
// that owns these records.
func TestDomainEventWorldOracleCaseIdentitiesAreTheRustDomainConsumer(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventWorldWorkingManifest(t, root)

	labels := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != domainEventWorldOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, domainEventWorldOperation)
		}
		if c.Version != domainEventWorldVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, domainEventWorldVersion)
		}
		if c.Family != domainEventWorldFamily {
			t.Fatalf("case %s declares family %q, want %q", c.ID, c.Family, domainEventWorldFamily)
		}
		if c.RustConsumer != domainEventWorldConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, domainEventWorldConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		prefix := c.Family + "/" + c.Version + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !domainEventWorldLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug with an optional numeric suffix", c.ID, label)
		}
		if labels[label] {
			t.Fatalf("label %q is registered twice", label)
		}
		labels[label] = true

		envelope := domainEventWorldReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != domainEventWorldConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, domainEventWorldConsumer)
		}
		switch envelope.Rule {
		case domainEventWorldRuleSnapshot, domainEventWorldRuleChanges, domainEventWorldRuleForget:
		default:
			t.Fatalf("case %s names rule %q, which is not a world observation rule", c.ID, envelope.Rule)
		}
	}
}

// TestDomainEventWorldOracleWorkingManifestDescribesItself proves the working
// manifest is internally consistent: every case passes the production case
// validation, the family's provenance hashes match disk, and the family's case
// list matches exactly the cases the selection registers.
func TestDomainEventWorldOracleWorkingManifestDescribesItself(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventWorldWorkingManifest(t, root)
	domainEventWorldAssertCaseSpecs(t, root, manifest)

	family, ok := domainEventWorldFamilySpec(manifest)
	if !ok {
		t.Fatalf("working manifest has no %s family", domainEventWorldFamily)
	}
	if family.Role != "event" || family.Kind != "domain" {
		t.Fatalf("%s declares kind %q role %q, want domain event", domainEventWorldFamily, family.Kind, family.Role)
	}
	if family.CurrentVersion != domainEventWorldVersion {
		t.Fatalf("%s declares version %q, want %q", domainEventWorldFamily, family.CurrentVersion, domainEventWorldVersion)
	}
	if len(family.Sources) != len(domainEventWorldFamilySources) {
		t.Fatalf("%s records %d provenance sources, want %d", domainEventWorldFamily, len(family.Sources), len(domainEventWorldFamilySources))
	}
	for _, source := range family.Sources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(source.Path)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", source.Path, err)
		}
		if hash != source.SHA256 {
			t.Fatalf("provenance source %s records %s, disk has %s", source.Path, source.SHA256, hash)
		}
	}

	registered := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		registered[c.ID] = true
	}
	if len(family.Cases) != len(registered) {
		t.Fatalf("%s lists %d cases, want %d", domainEventWorldFamily, len(family.Cases), len(registered))
	}
	for _, id := range family.Cases {
		if !registered[id] {
			t.Fatalf("%s lists case %s which the selection does not register", domainEventWorldFamily, id)
		}
	}
}

// TestDomainEventWorldOracleOutcomesDistinguishAcceptedAndRejected pins that
// the producer is not returning one constant answer, that every rule records
// both an admitted record and a rejection, and that the boundaries the Rust
// event world test enumerates are each present in the executed evidence.
func TestDomainEventWorldOracleOutcomesDistinguishAcceptedAndRejected(t *testing.T) {
	records := domainEventWorldExecute(t)
	domainEventWorldAssertOutcomesDistinguishCases(t, records)

	tally := make(map[string]*domainEventWorldRuleTally)
	for _, record := range records {
		entry, ok := tally[record.input.Rule]
		if !ok {
			entry = &domainEventWorldRuleTally{}
			tally[record.input.Rule] = entry
		}
		switch record.outcome.Kind {
		case "ok":
			entry.accepted = append(entry.accepted, record.outcome.Fields)
		case "error":
			entry.rejected = append(entry.rejected, record.outcome.Fields)
		}
	}
	for rule, entry := range tally {
		// A rule that records no rejection has no admission boundary to pair.
		if len(entry.rejected) == 0 {
			continue
		}
		if len(entry.accepted) == 0 {
			t.Fatalf("rule %s records %d accepted and %d rejected cases, want both non-zero",
				rule, len(entry.accepted), len(entry.rejected))
		}
	}

	if !domainEventWorldFieldsContain(tally[domainEventWorldRuleSnapshot].accepted, "revision", "7") {
		t.Fatal("no admitted snapshot carries the seed revision")
	}
	if !domainEventWorldFieldsContain(tally[domainEventWorldRuleChanges].accepted, "dimension", 1) {
		t.Fatal("no admitted change batch carries the depths dimension")
	}
	if !domainEventWorldFieldsContain(tally[domainEventWorldRuleChanges].accepted, "base_revision", "5") {
		t.Fatal("no admitted change batch carries the empty revision barrier's base")
	}
	if !domainEventWorldFieldsContain(tally[domainEventWorldRuleChanges].accepted, "chunk", []int{-1, -1}) {
		t.Fatal("no admitted change batch carries the negative chunk column")
	}
	if !domainEventWorldFieldsContain(tally[domainEventWorldRuleForget].accepted, "chunks", [][]int{{5, 5}, {-1, -1}, {0, 0}}) {
		t.Fatal("no admitted forget batch carries the reversed unique order, which the record has to preserve")
	}

	for _, boundary := range []struct {
		field string
		value any
		rule  string
	}{
		{"revision", "0", "chunk_snapshot.revision_nonzero"},
		{"dimension", 2, "chunk_snapshot.dimension"},
		{"base_revision", "18446744073709551615", "block_changes.revision_transition"},
	} {
		if !domainEventWorldFieldsContain(tally[domainEventWorldRuleSnapshot].rejected, boundary.field, boundary.value) &&
			!domainEventWorldFieldsContain(tally[domainEventWorldRuleChanges].rejected, boundary.field, boundary.value) &&
			!domainEventWorldFieldsContain(tally[domainEventWorldRuleForget].rejected, boundary.field, boundary.value) {
			t.Fatalf("no rejection names %s %v, which is outside the published range", boundary.field, boundary.value)
		}
		if !domainEventWorldRejectionNamesRule(tally, boundary.rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", boundary.rule)
		}
	}
	// A section's and a change's own fields live inside their record's array,
	// so those boundaries are looked up there rather than at the record's top
	// level.
	for _, boundary := range []struct {
		array string
		field string
		value any
		rule  string
	}{
		{"sections", "single", 90, "chunk_snapshot.section_single_registered"},
		{"sections", "bits", 5, "chunk_snapshot.section_indexed_bits"},
		{"changes", "block", 90, "block_changes.block_registered"},
	} {
		if !domainEventWorldNestedFieldContains(tally[domainEventWorldRuleSnapshot].rejected, boundary.array, boundary.field, boundary.value) &&
			!domainEventWorldNestedFieldContains(tally[domainEventWorldRuleChanges].rejected, boundary.array, boundary.field, boundary.value) {
			t.Fatalf("no rejection names a %s entry carrying %s %v, which is outside the published range", boundary.array, boundary.field, boundary.value)
		}
		if !domainEventWorldRejectionNamesRule(tally, boundary.rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", boundary.rule)
		}
	}
	for _, rule := range []string{
		"chunk_snapshot.section_count",
		"chunk_snapshot.section_palette_bounds",
		"chunk_snapshot.section_palette_registered",
		"chunk_snapshot.section_palette_unique",
		"chunk_snapshot.section_words_exact",
		"chunk_snapshot.section_slot_in_palette",
		"chunk_snapshot.section_direct_high_bits",
		"chunk_snapshot.section_direct_block_registered",
		"block_changes.position_y_span",
		"block_changes.position_chunk",
		"block_changes.strictly_increasing_index",
		"forget_chunks.nonempty_positions",
		"forget_chunks.unique_positions",
	} {
		if !domainEventWorldRejectionNamesRule(tally, rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", rule)
		}
	}
}

// TestDomainEventWorldOracleRunnerRejectsUnregisteredFamily pins that a
// manifest naming a family this package cannot execute fails the run.
func TestDomainEventWorldOracleRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventWorldWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "domain.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family domain.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestDomainEventWorldOracleRunnerRejectsOperationFamilyMismatch pins that a
// case cannot declare one operation and be executed by another.
func TestDomainEventWorldOracleRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventWorldWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "decode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation-family mismatch failure, got: %v", err)
	}
}

// TestDomainEventWorldOracleRunnerRejectsTamperedInput pins that a case whose
// committed bytes no longer match their recorded digest fails the run instead
// of executing bytes the manifest does not name.
func TestDomainEventWorldOracleRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventWorldWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestDomainEventWorldOracleReportPublishesAndValidates assembles the executed
// evidence into a report, publishes it through the production atomic exporter,
// reloads it and validates it against the working manifest. This is the
// identity and content check a later Rust acceptance step performs; it does not
// claim any Rust behaviour.
func TestDomainEventWorldOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventWorldWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event world evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, domainEventWorldCorpusReportName)
	if err := ExportTrace(root, target, trace, manifest); err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	loaded, err := LoadTraceAtRoot(root, target, manifest)
	if err != nil {
		t.Fatalf("published report does not validate: %v", err)
	}
	if loaded.SchemaVersion != traceSchemaVersion {
		t.Fatalf("report schema_version = %d, want %d", loaded.SchemaVersion, traceSchemaVersion)
	}
	if loaded.SourceRevision != manifest.SourceRevision {
		t.Fatalf("report source revision = %s, want %s", loaded.SourceRevision, manifest.SourceRevision)
	}
	if len(loaded.Inputs) != len(manifest.Cases) || len(loaded.Observations) != len(manifest.Cases) {
		t.Fatalf("report carries %d inputs and %d observations, want %d each",
			len(loaded.Inputs), len(loaded.Observations), len(manifest.Cases))
	}
}

// TestDomainEventWorldOracleRejectsMissingProducerTest pins that the executed
// evidence has a real producer behind it, including the topic-named entry
// point the domain plan's filter selects. A corpus whose producer test is gone
// has no independent execution, only frozen files, and a missing topic entry
// point would make the plan filter pass without running anything.
func TestDomainEventWorldOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(domainEventWorldProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", domainEventWorldProducerTestRelPath, err)
	}
	for _, name := range []string{domainEventWorldProducerExecuteName, domainEventWorldProducerTopicName} {
		declaration := "func " + name + "(t *testing.T) {"
		if !strings.Contains(string(data), declaration) {
			t.Fatalf("%s does not declare %s", domainEventWorldProducerTestRelPath, declaration)
		}
	}
}

// TestDomainOracle_event_world is the topic-named entry point the domain plan
// names for this node. It delegates to the same executed table, so the two
// filters select one source of expected results rather than two.
func TestDomainOracle_event_world(t *testing.T) {
	TestDomainEventWorldOracleExecutesEveryCase(t)
}

// runDomainEvent routes one `domain.event` case to the producer that owns its
// rule. The family is shared by the world observations, the player and outcome
// records, the inventory and container publications and the remote-player and
// companion observations, so the rule name is the only discriminator the
// manifest carries, and a rule no producer names fails the run rather than
// falling back to one that cannot execute it.
func runDomainEvent(c CaseSpec, input []byte) (Outcome, []byte, error) {
	envelope, err := domainEventDecodeRule(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	switch envelope.Rule {
	case domainEventWorldRuleSnapshot, domainEventWorldRuleChanges, domainEventWorldRuleForget:
		return runDomainEventWorld(c, input)
	case "player-state", "command-rejected", "place-block-succeeded", "combat-hit":
		return runDomainEventPlayer(c, input)
	case domainEventInventoryRuleInventory, domainEventInventoryRuleCrafting,
		domainEventInventoryRuleFurnace, domainEventInventoryRuleChest,
		domainEventInventoryRuleClosed:
		return runDomainEventInventory(c, input)
	case domainEventPeopleRuleRemoteSpawn, domainEventPeopleRuleRemoteDespawn,
		domainEventPeopleRuleRemoteStates, domainEventPeopleRuleCompanionSpawn,
		domainEventPeopleRuleCompanionStates, domainEventPeopleRuleCompanionDespawn:
		return runDomainEventPeople(c, input)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, envelope.Rule)
	}
}

// domainEventRuleEnvelope is the minimal envelope the family router reads.
type domainEventRuleEnvelope struct {
	Consumer string `json:"consumer"`
	Rule     string `json:"rule"`
}

// domainEventDecodeRule reads only the rule name of one corpus input, so the
// family router can select a producer without decoding the whole record.
func domainEventDecodeRule(data []byte) (domainEventRuleEnvelope, error) {
	var envelope domainEventRuleEnvelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&envelope); err != nil {
		return domainEventRuleEnvelope{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainEventRuleEnvelope{}, fmt.Errorf("trailing content after the JSON value")
	}
	return envelope, nil
}

// domainEventWorldRuleTally collects the accepted and rejected normalized
// outcomes one rule records, so a boundary assertion can look for the field
// value it names.
type domainEventWorldRuleTally struct {
	accepted []map[string]any
	rejected []map[string]any
}

// domainEventWorldFieldsContain reports whether one recorded outcome set holds
// a record whose named field carries the value a boundary assertion names.
func domainEventWorldFieldsContain(records []map[string]any, field string, want any) bool {
	for _, fields := range records {
		if reflect.DeepEqual(fields[field], want) {
			return true
		}
	}
	return false
}

// domainEventWorldRejectionNamesRule reports whether any recorded rejection
// names the broken rule a boundary assertion pins.
func domainEventWorldRejectionNamesRule(tally map[string]*domainEventWorldRuleTally, rule string) bool {
	for _, entry := range tally {
		for _, fields := range entry.rejected {
			if fields["rule"] == rule {
				return true
			}
		}
	}
	return false
}

// domainEventWorldNestedFieldContains reports whether one recorded outcome set
// holds a record whose named array carries an entry with the named field
// value. A section and a block change both live inside their record's array,
// so a boundary that names one of their fields looks inside the array rather
// than at the record's top level. The array type is the producer's own
// in-memory rendering, because the executed records have not been decoded
// back from JSON.
func domainEventWorldNestedFieldContains(records []map[string]any, array, field string, want any) bool {
	for _, fields := range records {
		entries, ok := fields[array].([]map[string]any)
		if !ok {
			continue
		}
		for _, item := range entries {
			if reflect.DeepEqual(item[field], want) {
				return true
			}
		}
	}
	return false
}

// domainEventWorldAssertCaseSpecs runs the production case validation over the
// whole selection, so a case with a bad path, digest, format or checkpoint
// fails here rather than surfacing later as a confusing coverage failure.
func domainEventWorldAssertCaseSpecs(t *testing.T, root string, manifest Inventory) {
	t.Helper()
	families := make(map[string]Family, len(manifest.Families))
	for _, family := range manifest.Families {
		families[family.ID] = family
	}
	for _, c := range manifest.Cases {
		if err := validateCaseSpec(root, c, families); err != nil {
			t.Fatalf("case %s: %v", c.ID, err)
		}
	}
}

// domainEventWorldAssertOutcomesDistinguishCases proves the executed evidence
// separates admitted records from rejections instead of publishing one
// constant answer, and that the rejections name a rule the authority
// publishes.
func domainEventWorldAssertOutcomesDistinguishCases(t *testing.T, records []domainEventWorldRecord) {
	t.Helper()
	if len(records) == 0 {
		t.Fatal("executed evidence records no case")
	}
	accepted, rejected := 0, 0
	for _, record := range records {
		switch record.outcome.Kind {
		case "ok":
			accepted++
			if record.outcome.Category == "" {
				t.Fatalf("record %s publishes an accepted outcome with no category", record.label)
			}
		case "error":
			rejected++
			if !corpusStructuralCategories[record.outcome.Category] &&
				!corpusAdmissionCategories[record.outcome.Category] &&
				!corpusStorageCategories[record.outcome.Category] {
				t.Fatalf("record %s publishes rejection category %q, which is not in the frozen vocabulary", record.label, record.outcome.Category)
			}
			if record.outcome.Fields["rule"] == nil {
				t.Fatalf("record %s publishes a rejection with no rule name", record.label)
			}
		default:
			t.Fatalf("record %s publishes kind %q, want ok or error", record.label, record.outcome.Kind)
		}
	}
	if accepted == 0 || rejected == 0 {
		t.Fatalf("executed evidence records %d accepted and %d rejected cases, want both non-zero", accepted, rejected)
	}
}

// domainEventWorldFamilySpec resolves one family from a manifest selection.
func domainEventWorldFamilySpec(manifest Inventory) (Family, bool) {
	for _, family := range manifest.Families {
		if family.ID == domainEventWorldFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainEventWorldCaseByID indexes a manifest selection by case identity.
func domainEventWorldCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}
