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

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file is the Go producer for the domain projectile and item-drop
// observation event family. It executes every committed corpus case through
// the current Go protocol DTOs: the verdict comes from
// `protocol.ValidateServerPacket`, which is the same Play-state validator the
// codec applies on both the encode and the decode side, and an admitted record
// is normalized field by field through the frozen field map. No world
// authority, listener, model call or native ABI is involved: the producer only
// reads immutable corpus inputs and calls in-process validators, and the
// frozen corpus files it materializes are read-only inputs for later
// consumers.
//
// The five rules this family executes are the Go `protocol.ProjectileSpawn`,
// `protocol.ProjectileState`, `protocol.ProjectileDespawn`,
// `protocol.ItemDropUpserts` and `protocol.ItemDropRemoves` records, which
// are the transient-object observations an authoritative session publishes to
// a subscribed client.
//
// The 128-record projectile and 32-record drop batch maxima the Go validators
// keep are transport budgets the domain value does not carry, so no case in
// this family sits above one; the classifier names the bound only so a count
// a case does name stays classifiable. The domain requires a nonempty batch
// whose identities are strictly increasing, which the "count_range" and
// "strictly_increasing_ids" rules pin. Projectile IDs and ticks are decimal
// strings in the frozen input so their full `u64` range stays lossless, a
// `core.DropID` stays the object of its five ordered key fields, and a drop's
// raw dimension is deliberately unvalidated because the Go `DropID.Valid`
// rule checks only the slot range and the generation: a negative raw
// dimension is therefore an admitted boundary, and the drop ordering compares
// the raw dimension first.
//
// The frozen cases this producer materializes are not yet registered in the
// canonical manifest: registration is a later node's work, so the only
// publication path is the explicit external export through
// `RUNTIME_ORACLE_EXPORT_DIR`.

const (
	// domainEventObjectsFamily is the corpus family this package executes.
	// The family is the existing `domain.event` row, whose eventual owner is
	// the Rust domain crate that owns the replay observation records.
	domainEventObjectsFamily = "domain.event"
	// domainEventObjectsVersion is the family's discovered version. A case
	// has to name its family's version, so the case identities carry this
	// segment rather than one this producer chose.
	domainEventObjectsVersion = "1"
	// domainEventObjectsOperation is the manifest operation name for an
	// object observation admission case.
	domainEventObjectsOperation = "admit"
	// domainEventObjectsConsumer is the manifest consumer the change pins for
	// this family: the Rust crate that owns these records.
	domainEventObjectsConsumer = "mornlea_domain"
	// domainEventObjectsCorpusRelDir is the repository-relative directory
	// holding the frozen event objects corpus cases.
	domainEventObjectsCorpusRelDir = "testdata/runtime-migration/cases/domain/event_objects"
	// domainEventObjectsProducerTestRelPath and the two producer test names
	// locate the package-local producer that executes the protocol DTOs. The
	// topic name is the entry point the domain plan's filter requires; a
	// corpus whose producer test is missing has no independently executed
	// evidence at all.
	domainEventObjectsProducerTestRelPath = "packages/tools/cmd/runtime-oracle/domain_event_objects_test.go"
	domainEventObjectsProducerExecuteName = "TestDomainEventObjectsOracleExecutesEveryCase"
	domainEventObjectsProducerTopicName   = "TestDomainOracle_event_objects"
	// domainEventObjectsCorpusReportName is the published report file name
	// for the executed event objects evidence.
	domainEventObjectsCorpusReportName = "runtime-corpus-domain-event-objects.json"
	// domainEventObjectsProducerID is the exporter's producer identity for
	// this family's frozen assets.
	domainEventObjectsProducerID = "runtime-oracle/domain-event-objects"

	// The five rule names a case names. The family is shared with the world
	// observations, the player and outcome records, the inventory and
	// container publications, the remote-player and companion observations
	// and the hostile and passive mob observations, so the rule name is the
	// discriminator the manifest and the family router both read.
	domainEventObjectsRuleProjectileSpawn   = "projectile-spawn"
	domainEventObjectsRuleProjectileState   = "projectile-state"
	domainEventObjectsRuleProjectileDespawn = "projectile-despawn"
	domainEventObjectsRuleItemDropUpserts   = "item-drop-upserts"
	domainEventObjectsRuleItemDropRemoves   = "item-drop-removes"

	// The underscored subject names the rejection rules carry as their
	// prefix, one per rule above in the same order.
	domainEventObjectsNameProjectileSpawn   = "projectile_spawn"
	domainEventObjectsNameProjectileState   = "projectile_state"
	domainEventObjectsNameProjectileDespawn = "projectile_despawn"
	domainEventObjectsNameItemDropUpserts   = "item_drop_upserts"
	domainEventObjectsNameItemDropRemoves   = "item_drop_removes"

	// domainEventObjectsCaseTotal is the exact size of the executed case
	// table, pinned by the outcomes test so a dropped or duplicated row
	// fails the run rather than shrinking the evidence silently.
	domainEventObjectsCaseTotal = 45
)

// domainEventObjectsFamilySources is the merged provenance set the family
// records. Every entry is a file the producer's rules are read from, so a
// change to any of them is a change to the recorded evidence. The two
// protocol message files carry the record shapes and the validators, the
// chunk message file owns the block-index bound the drop validator compares
// against, the registry file decides which of these records are published in
// the play state, and the five core files own the dimension numbering, the
// drop identity rule, the item stack rule, the armor durability source and
// the world geometry the block-index bound derives from.
var domainEventObjectsFamilySources = []string{
	"packages/shared/network/protocol/message_projectile.go",
	"packages/shared/network/protocol/message_drop.go",
	"packages/shared/network/protocol/message_chunk.go",
	"packages/shared/network/protocol/registry.go",
	"packages/shared/core/block.go",
	"packages/shared/core/drop.go",
	"packages/shared/core/item.go",
	"packages/shared/core/armor.go",
	"packages/shared/core/pos.go",
}

// domainEventObjectsLabelPattern is the shape a corpus label must have: a
// lowercase slug with an optional zero-padded numeric suffix, so a boundary
// row sorts in numeric order under a lexical sort.
var domainEventObjectsLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(-[0-9]+)?$`)

// domainEventObjectsExportPublished keeps the explicit export to one
// publication per test process. The topic entry delegates to the
// execute-every-case test, so one filter selecting both entries executes the
// same deterministic candidate twice, and the exporter's exclusive producer
// child would otherwise treat the second publication as a preexisting
// directory rather than as the same evidence.
var domainEventObjectsExportPublished bool

// domainEventObjectsInput is the frozen, self-describing corpus input for one
// case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority. The server tick, every projectile
// identity and the projectile despawn identities are decimal strings, because
// a JSON number cannot carry the full `u64` range losslessly and those fields
// are boundaries this family keeps whole. `Position` and `Velocity` are
// decimal float texts rather than JSON numbers so NaN, the infinities and a
// signed zero stay expressible. The five rules share one envelope whose four
// batch keys are always serialized, an empty batch included, so a replay
// consumer never reads a missing key as an absent batch: the projectile rules
// fill "spawns", `states` and `ids`, the drop rules fill `drops` and `ids`,
// and the keys a rule does not read stay explicit empty lists. The `ids` key
// carries decimal projectile identities for `projectile-despawn` and drop-ID
// objects for `item-drop-removes`, so the two shapes never meet in one
// rendering.
type domainEventObjectsInput struct {
	Consumer   string  `json:"consumer"`
	Rule       string  `json:"rule"`
	ServerTick *string `json:"server_tick,omitempty"`

	ProjectileSpawns []domainEventObjectsProjectileSpawnRecord
	ProjectileStates []domainEventObjectsProjectileStateRecord
	ProjectileIDs    []string
	Drops            []domainEventObjectsDropRecord
	DropIDs          []domainEventObjectsDropID
}

// domainEventObjectsProjectileSpawnRecord is the frozen rendering of one
// projectile spawn record, carrying exactly the fields the Go DTO declares.
type domainEventObjectsProjectileSpawnRecord struct {
	ID        string   `json:"id"`
	Kind      uint8    `json:"kind"`
	Dimension int32    `json:"dimension"`
	Position  []string `json:"position"`
	Velocity  []string `json:"velocity"`
}

// domainEventObjectsProjectileStateRecord is the frozen rendering of one
// projectile state record. It carries no kind and no dimension, exactly as
// the wire record does: both are fixed for a projectile's whole life and the
// mirror records them at spawn.
type domainEventObjectsProjectileStateRecord struct {
	ID       string   `json:"id"`
	Position []string `json:"position"`
}

// domainEventObjectsDropID is the frozen rendering of one `core.DropID`: the
// raw i32 dimension, the `[x, z]` chunk pair, the u8 slot and the u32
// generation, in that key order. The dimension stays raw because the Go
// `DropID.Valid` rule deliberately leaves it unvalidated.
type domainEventObjectsDropID struct {
	Dimension  int32   `json:"dimension"`
	Chunk      []int32 `json:"chunk"`
	Slot       uint8   `json:"slot"`
	Generation uint32  `json:"generation"`
}

// domainEventObjectsStack is the frozen rendering of one `core.ItemStack` as
// the numeric `item/count/durability` object the corpus vocabulary uses.
type domainEventObjectsStack struct {
	Item       uint16 `json:"item"`
	Count      uint8  `json:"count"`
	Durability uint16 `json:"durability"`
}

// domainEventObjectsDropRecord is the frozen rendering of one item-drop
// upsert record: its identity, its block index inside the announced chunk and
// the stack it carries.
type domainEventObjectsDropRecord struct {
	ID         domainEventObjectsDropID `json:"id"`
	BlockIndex uint32                   `json:"block_index"`
	Stack      domainEventObjectsStack  `json:"stack"`
}

// domainEventObjectsInputWire is the frozen on-disk rendering of one corpus
// input. The `ids` batch is a decimal string array for the projectile despawn
// rule and a drop-ID object array for the item-drop-removes rule, so the wire
// renders whichever shape the rule names and the typed arrays live on the
// input; the keys a rule does not read are explicit empty lists rather than
// absent keys.
type domainEventObjectsInputWire struct {
	Consumer   string  `json:"consumer"`
	Rule       string  `json:"rule"`
	ServerTick *string `json:"server_tick,omitempty"`
	Spawns     any     `json:"spawns"`
	States     any     `json:"states"`
	IDs        any     `json:"ids"`
	Drops      any     `json:"drops"`
}

// MarshalJSON renders one corpus input under the rule-scoped batch keys. A
// rule outside the five this family owns has no rendering, so a stray rule
// fails the marshal rather than silently publishing an empty case.
func (in domainEventObjectsInput) MarshalJSON() ([]byte, error) {
	wire := domainEventObjectsInputWire{
		Consumer:   in.Consumer,
		Rule:       in.Rule,
		ServerTick: in.ServerTick,
		Spawns:     []domainEventObjectsProjectileSpawnRecord{},
		States:     []domainEventObjectsProjectileStateRecord{},
		IDs:        []string{},
		Drops:      []domainEventObjectsDropRecord{},
	}
	switch in.Rule {
	case domainEventObjectsRuleProjectileSpawn:
		wire.Spawns = in.ProjectileSpawns
	case domainEventObjectsRuleProjectileState:
		wire.States = in.ProjectileStates
	case domainEventObjectsRuleProjectileDespawn:
		wire.IDs = in.ProjectileIDs
	case domainEventObjectsRuleItemDropUpserts:
		wire.Drops = in.Drops
	case domainEventObjectsRuleItemDropRemoves:
		wire.IDs = in.DropIDs
	default:
		return nil, fmt.Errorf("runtime-oracle: object event input names rule %q, which has no frozen rendering", in.Rule)
	}
	return json.Marshal(wire)
}

// UnmarshalJSON reads one frozen corpus input back into the typed arrays the
// rule names. An unknown rule keeps the scalar envelope, so the producer's
// own switch reports the unknown rule instead of the JSON decoder, and the
// strict decode wrapper still rejects trailing content after the value.
func (in *domainEventObjectsInput) UnmarshalJSON(data []byte) error {
	var envelope struct {
		Consumer   string          `json:"consumer"`
		Rule       string          `json:"rule"`
		ServerTick *string         `json:"server_tick"`
		Spawns     json.RawMessage `json:"spawns"`
		States     json.RawMessage `json:"states"`
		IDs        json.RawMessage `json:"ids"`
		Drops      json.RawMessage `json:"drops"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&envelope); err != nil {
		return err
	}
	spec := domainEventObjectsBaseInput(envelope.Rule)
	spec.Consumer = envelope.Consumer
	spec.ServerTick = envelope.ServerTick
	switch envelope.Rule {
	case domainEventObjectsRuleProjectileSpawn:
		if err := domainEventObjectsDecodeRecords(envelope.Spawns, &spec.ProjectileSpawns); err != nil {
			return err
		}
	case domainEventObjectsRuleProjectileState:
		if err := domainEventObjectsDecodeRecords(envelope.States, &spec.ProjectileStates); err != nil {
			return err
		}
	case domainEventObjectsRuleProjectileDespawn:
		if err := domainEventObjectsDecodeRecords(envelope.IDs, &spec.ProjectileIDs); err != nil {
			return err
		}
	case domainEventObjectsRuleItemDropUpserts:
		if err := domainEventObjectsDecodeRecords(envelope.Drops, &spec.Drops); err != nil {
			return err
		}
	case domainEventObjectsRuleItemDropRemoves:
		if err := domainEventObjectsDecodeRecords(envelope.IDs, &spec.DropIDs); err != nil {
			return err
		}
	}
	*in = spec
	return nil
}

// domainEventObjectsDecodeRecords decodes one raw batch key into its typed
// array. A key the input does not carry leaves the initialized empty slice
// untouched, so an absent key and an explicit empty list stay distinguishable
// on the typed field.
func domainEventObjectsDecodeRecords[T any](raw json.RawMessage, target *[]T) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, target)
}

// domainEventObjectsCase is one frozen corpus case: its label and its input,
// and nothing else.
type domainEventObjectsCase struct {
	label string
	input domainEventObjectsInput
}

// domainEventObjectsBaseInput is the envelope every case starts from: the
// consumer, the rule and five explicitly empty batch arrays, so a case that
// names no batch still serializes every key as an empty list rather than
// dropping it.
func domainEventObjectsBaseInput(rule string) domainEventObjectsInput {
	return domainEventObjectsInput{
		Consumer:         domainEventObjectsConsumer,
		Rule:             rule,
		ProjectileSpawns: []domainEventObjectsProjectileSpawnRecord{},
		ProjectileStates: []domainEventObjectsProjectileStateRecord{},
		ProjectileIDs:    []string{},
		Drops:            []domainEventObjectsDropRecord{},
		DropIDs:          []domainEventObjectsDropID{},
	}
}

// domainEventObjectsTickText renders one u64 tick as the decimal string the
// frozen input carries.
func domainEventObjectsTickText(tick uint64) *string {
	text := strconv.FormatUint(tick, 10)
	return &text
}

// domainEventObjectsIDText renders one u64 identity as the decimal string the
// frozen input and the normalized field map both use.
func domainEventObjectsIDText(id uint64) string {
	return strconv.FormatUint(id, 10)
}

// domainEventObjectsSeedPosition renders the finite position one seed record
// carries: record `i` holds `[1.5+i, 64+i, -3.25-i]`, so the records of a
// batch are distinguishable in the normalized evidence while the first record
// holds the exact `[1.5, 64, -3.25]` the change pins.
func domainEventObjectsSeedPosition(index int) mgl32.Vec3 {
	return mgl32.Vec3{1.5 + float32(index), 64 + float32(index), -3.25 - float32(index)}
}

// domainEventObjectsProjectileSpawnSeed is the seed projectile spawn batch:
// tick 7 and the four identities 1..4 in canonical ascending order, covering
// all four `(Shard|Arrow) × (Overworld|Depths)` combinations, each with a
// finite position and the finite velocity `[0.25, 0, -0.5]`.
func domainEventObjectsProjectileSpawnSeed() protocol.ProjectileSpawn {
	return protocol.ProjectileSpawn{
		ServerTick: 7,
		Spawns: []protocol.ProjectileSpawnRecord{
			domainEventObjectsProjectileSpawnSeedRecord(0),
			domainEventObjectsProjectileSpawnSeedRecord(1),
			domainEventObjectsProjectileSpawnSeedRecord(2),
			domainEventObjectsProjectileSpawnSeedRecord(3),
		},
	}
}

// domainEventObjectsProjectileSpawnSeedRecord renders one projectile spawn
// record from its index: indices 0 and 1 are shards, indices 2 and 3 are
// arrows, and odd indices sit in the depths while even indices sit in the
// overworld, so the four records enumerate the complete kind-by-dimension
// matrix.
func domainEventObjectsProjectileSpawnSeedRecord(index int) protocol.ProjectileSpawnRecord {
	kind := protocol.ProjectileKindShard
	dimension := core.Overworld
	if index >= 2 {
		kind = protocol.ProjectileKindArrow
	}
	if index%2 == 1 {
		dimension = core.Depths
	}
	return protocol.ProjectileSpawnRecord{
		ID:        uint64(index + 1),
		Kind:      kind,
		Dimension: dimension,
		Position:  domainEventObjectsSeedPosition(index),
		Velocity:  mgl32.Vec3{0.25, 0, -0.5},
	}
}

// domainEventObjectsProjectileSpawnCase renders one projectile spawn batch
// row: the seed batch with exactly one field changed, so a boundary row is
// always the seed plus the value the rule names.
func domainEventObjectsProjectileSpawnCase(label string, mutate func(*protocol.ProjectileSpawn)) domainEventObjectsCase {
	spawn := domainEventObjectsProjectileSpawnSeed()
	if mutate != nil {
		mutate(&spawn)
	}
	input := domainEventObjectsBaseInput(domainEventObjectsRuleProjectileSpawn)
	input.ServerTick = domainEventObjectsTickText(spawn.ServerTick)
	records := make([]domainEventObjectsProjectileSpawnRecord, 0, len(spawn.Spawns))
	for _, record := range spawn.Spawns {
		records = append(records, domainEventObjectsProjectileSpawnRecordInput(record))
	}
	input.ProjectileSpawns = records
	return domainEventObjectsCase{label: label, input: input}
}

// domainEventObjectsProjectileSpawnRecordInput renders one Go spawn record as
// the frozen input rendering.
func domainEventObjectsProjectileSpawnRecordInput(record protocol.ProjectileSpawnRecord) domainEventObjectsProjectileSpawnRecord {
	return domainEventObjectsProjectileSpawnRecord{
		ID:        domainEventObjectsIDText(record.ID),
		Kind:      record.Kind,
		Dimension: int32(record.Dimension),
		Position:  domainEventObjectsVectorText(record.Position),
		Velocity:  domainEventObjectsVectorText(record.Velocity),
	}
}

// domainEventObjectsProjectileStateSeed is the seed projectile state batch:
// tick 7 and the two identities 1 and 2 in canonical ascending order, each
// with a finite position. Kind, dimension and velocity stay off the record
// because they are fixed at spawn and the mirror replays them from there.
func domainEventObjectsProjectileStateSeed() protocol.ProjectileState {
	return protocol.ProjectileState{
		ServerTick: 7,
		States: []protocol.ProjectileStateRecord{
			{ID: 1, Position: domainEventObjectsSeedPosition(0)},
			{ID: 2, Position: domainEventObjectsSeedPosition(1)},
		},
	}
}

// domainEventObjectsProjectileStateCase renders one projectile state batch
// row: the seed batch with exactly one field changed.
func domainEventObjectsProjectileStateCase(label string, mutate func(*protocol.ProjectileState)) domainEventObjectsCase {
	state := domainEventObjectsProjectileStateSeed()
	if mutate != nil {
		mutate(&state)
	}
	input := domainEventObjectsBaseInput(domainEventObjectsRuleProjectileState)
	input.ServerTick = domainEventObjectsTickText(state.ServerTick)
	records := make([]domainEventObjectsProjectileStateRecord, 0, len(state.States))
	for _, record := range state.States {
		records = append(records, domainEventObjectsProjectileStateRecordInput(record))
	}
	input.ProjectileStates = records
	return domainEventObjectsCase{label: label, input: input}
}

// domainEventObjectsProjectileStateRecordInput renders one Go state record as
// the frozen input rendering.
func domainEventObjectsProjectileStateRecordInput(record protocol.ProjectileStateRecord) domainEventObjectsProjectileStateRecord {
	return domainEventObjectsProjectileStateRecord{
		ID:       domainEventObjectsIDText(record.ID),
		Position: domainEventObjectsVectorText(record.Position),
	}
}

// domainEventObjectsProjectileDespawnSeed is the seed projectile despawn
// batch: tick 7 and the two identities in canonical ascending order.
func domainEventObjectsProjectileDespawnSeed() protocol.ProjectileDespawn {
	return protocol.ProjectileDespawn{ServerTick: 7, IDs: []uint64{1, 2}}
}

// domainEventObjectsProjectileDespawnCase renders one projectile despawn
// batch row: the seed batch with exactly one field changed.
func domainEventObjectsProjectileDespawnCase(label string, mutate func(*protocol.ProjectileDespawn)) domainEventObjectsCase {
	despawn := domainEventObjectsProjectileDespawnSeed()
	if mutate != nil {
		mutate(&despawn)
	}
	input := domainEventObjectsBaseInput(domainEventObjectsRuleProjectileDespawn)
	input.ServerTick = domainEventObjectsTickText(despawn.ServerTick)
	ids := make([]string, 0, len(despawn.IDs))
	for _, id := range despawn.IDs {
		ids = append(ids, domainEventObjectsIDText(id))
	}
	input.ProjectileIDs = ids
	return domainEventObjectsCase{label: label, input: input}
}

// domainEventObjectsSeedDropID renders one seed drop identity: the overworld
// dimension, chunk `[0, 0]`, the named slot and generation 1, so the two
// records of a batch differ only in the slot.
func domainEventObjectsSeedDropID(slot uint8) core.DropID {
	return core.DropID{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: 0, Z: 0},
		Slot:       slot,
		Generation: 1,
	}
}

// domainEventObjectsDropUpsertsSeed is the seed item-drop upsert batch: tick
// 7 and two strictly ordered identities in the overworld and chunk `[0, 0]`,
// slots 1 then 2 and generation 1, each carrying one ordinary stone at block
// indices 17 then 18. This two-record seed is also the exact order-mutation
// fixture a later node requires.
func domainEventObjectsDropUpsertsSeed() protocol.ItemDropUpserts {
	return protocol.ItemDropUpserts{
		ServerTick: 7,
		Drops: []protocol.ItemDrop{
			{ID: domainEventObjectsSeedDropID(1), BlockIndex: 17, Item: core.ItemStone, Count: 1, Durability: 0},
			{ID: domainEventObjectsSeedDropID(2), BlockIndex: 18, Item: core.ItemStone, Count: 1, Durability: 0},
		},
	}
}

// domainEventObjectsDropUpsertsCase renders one item-drop upsert batch row:
// the seed batch with exactly one field changed.
func domainEventObjectsDropUpsertsCase(label string, mutate func(*protocol.ItemDropUpserts)) domainEventObjectsCase {
	upserts := domainEventObjectsDropUpsertsSeed()
	if mutate != nil {
		mutate(&upserts)
	}
	input := domainEventObjectsBaseInput(domainEventObjectsRuleItemDropUpserts)
	input.ServerTick = domainEventObjectsTickText(upserts.ServerTick)
	records := make([]domainEventObjectsDropRecord, 0, len(upserts.Drops))
	for _, drop := range upserts.Drops {
		records = append(records, domainEventObjectsDropRecordInput(drop))
	}
	input.Drops = records
	return domainEventObjectsCase{label: label, input: input}
}

// domainEventObjectsDropRecordInput renders one Go item-drop upsert record as
// the frozen input rendering.
func domainEventObjectsDropRecordInput(drop protocol.ItemDrop) domainEventObjectsDropRecord {
	return domainEventObjectsDropRecord{
		ID:         domainEventObjectsDropIDInput(drop.ID),
		BlockIndex: drop.BlockIndex,
		Stack: domainEventObjectsStack{
			Item:       uint16(drop.Item),
			Count:      drop.Count,
			Durability: drop.Durability,
		},
	}
}

// domainEventObjectsDropIDInput renders one Go drop identity as the frozen
// input rendering.
func domainEventObjectsDropIDInput(id core.DropID) domainEventObjectsDropID {
	return domainEventObjectsDropID{
		Dimension:  int32(id.Dimension),
		Chunk:      []int32{id.Chunk.X, id.Chunk.Z},
		Slot:       id.Slot,
		Generation: id.Generation,
	}
}

// domainEventObjectsDropRemovesSeed is the seed item-drop remove batch: tick
// 7 and the same two strictly ordered identities the upsert seed carries.
func domainEventObjectsDropRemovesSeed() protocol.ItemDropRemoves {
	return protocol.ItemDropRemoves{
		ServerTick: 7,
		IDs:        []core.DropID{domainEventObjectsSeedDropID(1), domainEventObjectsSeedDropID(2)},
	}
}

// domainEventObjectsDropRemovesCase renders one item-drop remove batch row:
// the seed batch with exactly one field changed.
func domainEventObjectsDropRemovesCase(label string, mutate func(*protocol.ItemDropRemoves)) domainEventObjectsCase {
	removes := domainEventObjectsDropRemovesSeed()
	if mutate != nil {
		mutate(&removes)
	}
	input := domainEventObjectsBaseInput(domainEventObjectsRuleItemDropRemoves)
	input.ServerTick = domainEventObjectsTickText(removes.ServerTick)
	ids := make([]domainEventObjectsDropID, 0, len(removes.IDs))
	for _, id := range removes.IDs {
		ids = append(ids, domainEventObjectsDropIDInput(id))
	}
	input.DropIDs = ids
	return domainEventObjectsCase{label: label, input: input}
}

// domainEventObjectsNonFinitePosition is the seed first-record position with
// its X component replaced by NaN, the non-finite pose boundary.
func domainEventObjectsNonFinitePosition() mgl32.Vec3 {
	return mgl32.Vec3{float32(math.NaN()), 64, -3.25}
}

// domainEventObjectsNonFiniteVelocity is the seed velocity with its Z
// component replaced by positive infinity, the non-finite velocity boundary.
func domainEventObjectsNonFiniteVelocity() mgl32.Vec3 {
	return mgl32.Vec3{0.25, 0, float32(math.Inf(1))}
}

// domainEventObjectsMaxBlockIndex mirrors the protocol package's private
// `maxChunkBlockIndex`: the exclusive upper bound an item drop's block index
// must stay below. It derives from the same `core` geometry constants the
// protocol constant derives from, so the two cannot drift apart.
func domainEventObjectsMaxBlockIndex() uint32 {
	return uint32(core.SectionsPerChunk * core.BlocksPerSection)
}

// domainEventObjectsCases is the ordered case table the producer executes.
//
// Every rule's rows are the seed batch plus one boundary each: the zero tick,
// which the Go validators admit because they publish no tick rule for these
// records; the raw dimension -1, which the drop rules admit because the Go
// `DropID.Valid` rule checks only the slot range and the generation; the
// inclusive and exclusive block-index ends; the canonical empty stack, which
// is the exactly-zero triple and stays admitted; the unknown projectile kind
// and dimension, each raw value 2; the slot at the drop-slot bound and the
// zero generation; the unregistered item 66, the zero count and the
// non-durable durability; the zero identity; the non-finite pose and
// velocity; the empty batch, which stays an explicit empty list rather than
// an absent key; and the reversed and duplicate identity orders, which keep
// otherwise valid records. No row sits above a wire batch maximum, because
// those counts are transport budgets the domain does not carry.
func domainEventObjectsCases() []domainEventObjectsCase {
	cases := []domainEventObjectsCase{
		domainEventObjectsProjectileSpawnCase("projectile-spawn-all-kind-dimension-combinations", nil),
		domainEventObjectsProjectileSpawnCase("projectile-spawn-zero-tick", func(spawn *protocol.ProjectileSpawn) {
			spawn.ServerTick = 0
		}),
		domainEventObjectsProjectileSpawnCase("projectile-spawn-unknown-kind", func(spawn *protocol.ProjectileSpawn) {
			spawn.Spawns[0].Kind = 2
		}),
		domainEventObjectsProjectileSpawnCase("projectile-spawn-unknown-dimension", func(spawn *protocol.ProjectileSpawn) {
			spawn.Spawns[0].Dimension = core.DimensionID(2)
		}),
		domainEventObjectsProjectileSpawnCase("projectile-spawn-zero-id", func(spawn *protocol.ProjectileSpawn) {
			spawn.Spawns[0].ID = 0
		}),
		domainEventObjectsProjectileSpawnCase("projectile-spawn-non-finite-position", func(spawn *protocol.ProjectileSpawn) {
			spawn.Spawns[0].Position = domainEventObjectsNonFinitePosition()
		}),
		domainEventObjectsProjectileSpawnCase("projectile-spawn-non-finite-velocity", func(spawn *protocol.ProjectileSpawn) {
			spawn.Spawns[0].Velocity = domainEventObjectsNonFiniteVelocity()
		}),
		domainEventObjectsProjectileSpawnCase("projectile-spawn-empty", func(spawn *protocol.ProjectileSpawn) {
			spawn.Spawns = []protocol.ProjectileSpawnRecord{}
		}),
		domainEventObjectsProjectileSpawnCase("projectile-spawn-reversed", func(spawn *protocol.ProjectileSpawn) {
			spawn.Spawns[0], spawn.Spawns[1] = spawn.Spawns[1], spawn.Spawns[0]
		}),
		domainEventObjectsProjectileSpawnCase("projectile-spawn-duplicate", func(spawn *protocol.ProjectileSpawn) {
			spawn.Spawns[1].ID = spawn.Spawns[0].ID
		}),

		domainEventObjectsProjectileStateCase("projectile-state-seed", nil),
		domainEventObjectsProjectileStateCase("projectile-state-zero-tick", func(state *protocol.ProjectileState) {
			state.ServerTick = 0
		}),
		domainEventObjectsProjectileStateCase("projectile-state-zero-id", func(state *protocol.ProjectileState) {
			state.States[0].ID = 0
		}),
		domainEventObjectsProjectileStateCase("projectile-state-non-finite-position", func(state *protocol.ProjectileState) {
			state.States[0].Position = domainEventObjectsNonFinitePosition()
		}),
		domainEventObjectsProjectileStateCase("projectile-state-empty", func(state *protocol.ProjectileState) {
			state.States = []protocol.ProjectileStateRecord{}
		}),
		domainEventObjectsProjectileStateCase("projectile-state-reversed", func(state *protocol.ProjectileState) {
			state.States[0], state.States[1] = state.States[1], state.States[0]
		}),
		domainEventObjectsProjectileStateCase("projectile-state-duplicate", func(state *protocol.ProjectileState) {
			state.States[1].ID = state.States[0].ID
		}),

		domainEventObjectsProjectileDespawnCase("projectile-despawn-seed", nil),
		domainEventObjectsProjectileDespawnCase("projectile-despawn-zero-tick", func(despawn *protocol.ProjectileDespawn) {
			despawn.ServerTick = 0
		}),
		domainEventObjectsProjectileDespawnCase("projectile-despawn-zero-id", func(despawn *protocol.ProjectileDespawn) {
			despawn.IDs[0] = 0
		}),
		domainEventObjectsProjectileDespawnCase("projectile-despawn-empty", func(despawn *protocol.ProjectileDespawn) {
			despawn.IDs = []uint64{}
		}),
		domainEventObjectsProjectileDespawnCase("projectile-despawn-reversed", func(despawn *protocol.ProjectileDespawn) {
			despawn.IDs[0], despawn.IDs[1] = despawn.IDs[1], despawn.IDs[0]
		}),
		domainEventObjectsProjectileDespawnCase("projectile-despawn-duplicate", func(despawn *protocol.ProjectileDespawn) {
			despawn.IDs[1] = despawn.IDs[0]
		}),

		domainEventObjectsDropUpsertsCase("item-drop-upserts-seed", nil),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-zero-tick", func(upserts *protocol.ItemDropUpserts) {
			upserts.ServerTick = 0
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-raw-dimension-negative-one", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops[0].ID.Dimension = core.DimensionID(-1)
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-block-index-98303", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops[0].BlockIndex = domainEventObjectsMaxBlockIndex() - 1
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-block-index-98304", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops[0].BlockIndex = domainEventObjectsMaxBlockIndex()
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-empty-stack", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops[0].Item = core.ItemNone
			upserts.Drops[0].Count = 0
			upserts.Drops[0].Durability = 0
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-slot-32", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops[0].ID.Slot = core.DropsPerChunk
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-zero-generation", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops[0].ID.Generation = 0
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-unregistered-item-66", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops[0].Item = core.ItemID(66)
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-zero-count", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops[0].Count = 0
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-nondurable-with-durability", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops[0].Durability = 1
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-empty", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops = []protocol.ItemDrop{}
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-reversed", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops[0], upserts.Drops[1] = upserts.Drops[1], upserts.Drops[0]
		}),
		domainEventObjectsDropUpsertsCase("item-drop-upserts-duplicate", func(upserts *protocol.ItemDropUpserts) {
			upserts.Drops[1].ID = upserts.Drops[0].ID
		}),

		domainEventObjectsDropRemovesCase("item-drop-removes-seed", nil),
		domainEventObjectsDropRemovesCase("item-drop-removes-zero-tick", func(removes *protocol.ItemDropRemoves) {
			removes.ServerTick = 0
		}),
		domainEventObjectsDropRemovesCase("item-drop-removes-raw-dimension-negative-one", func(removes *protocol.ItemDropRemoves) {
			removes.IDs[0].Dimension = core.DimensionID(-1)
		}),
		domainEventObjectsDropRemovesCase("item-drop-removes-slot-32", func(removes *protocol.ItemDropRemoves) {
			removes.IDs[0].Slot = core.DropsPerChunk
		}),
		domainEventObjectsDropRemovesCase("item-drop-removes-zero-generation", func(removes *protocol.ItemDropRemoves) {
			removes.IDs[0].Generation = 0
		}),
		domainEventObjectsDropRemovesCase("item-drop-removes-empty", func(removes *protocol.ItemDropRemoves) {
			removes.IDs = []core.DropID{}
		}),
		domainEventObjectsDropRemovesCase("item-drop-removes-reversed", func(removes *protocol.ItemDropRemoves) {
			removes.IDs[0], removes.IDs[1] = removes.IDs[1], removes.IDs[0]
		}),
		domainEventObjectsDropRemovesCase("item-drop-removes-duplicate", func(removes *protocol.ItemDropRemoves) {
			removes.IDs[1] = removes.IDs[0]
		}),
	}
	return cases
}

// runDomainEventObjects executes one corpus case through the current Go
// protocol projectile and item-drop DTOs.
//
// The verdict always comes from `protocol.ValidateServerPacket` in the play
// state, which is the same validator the codec applies on both the encode and
// the decode side. The rule name and the rejection category come from the same
// bounds the DTO reads, and the two are cross-checked against each other, so a
// classification that disagrees with the authority fails the run instead of
// publishing a plausible-looking rejection.
func runDomainEventObjects(c CaseSpec, input []byte) (Outcome, []byte, error) {
	spec, err := domainEventObjectsDecodeInput(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	if spec.Consumer != domainEventObjectsConsumer {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names consumer %q, want %q", c.ID, spec.Consumer, domainEventObjectsConsumer)
	}

	switch spec.Rule {
	case domainEventObjectsRuleProjectileSpawn:
		return domainEventObjectsRunProjectileSpawn(c, spec)
	case domainEventObjectsRuleProjectileState:
		return domainEventObjectsRunProjectileState(c, spec)
	case domainEventObjectsRuleProjectileDespawn:
		return domainEventObjectsRunProjectileDespawn(c, spec)
	case domainEventObjectsRuleItemDropUpserts:
		return domainEventObjectsRunItemDropUpserts(c, spec)
	case domainEventObjectsRuleItemDropRemoves:
		return domainEventObjectsRunItemDropRemoves(c, spec)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, spec.Rule)
	}
}

// domainEventObjectsRunProjectileSpawn admits one projectile spawn batch.
func domainEventObjectsRunProjectileSpawn(c CaseSpec, spec domainEventObjectsInput) (Outcome, []byte, error) {
	spawn, err := domainEventObjectsProjectileSpawn(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventObjectsNormalizeProjectileSpawn(spawn)
	category, rule := domainEventObjectsProjectileSpawnRule(spawn)
	return domainEventObjectsFinish(c, domainEventObjectsRuleProjectileSpawn, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, spawn) == nil)
}

// domainEventObjectsRunProjectileState admits one projectile state batch.
func domainEventObjectsRunProjectileState(c CaseSpec, spec domainEventObjectsInput) (Outcome, []byte, error) {
	state, err := domainEventObjectsProjectileState(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventObjectsNormalizeProjectileState(state)
	category, rule := domainEventObjectsProjectileStateRule(state)
	return domainEventObjectsFinish(c, domainEventObjectsRuleProjectileState, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, state) == nil)
}

// domainEventObjectsRunProjectileDespawn admits one projectile despawn batch,
// whose records carry only identities.
func domainEventObjectsRunProjectileDespawn(c CaseSpec, spec domainEventObjectsInput) (Outcome, []byte, error) {
	despawn, err := domainEventObjectsProjectileDespawn(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventObjectsNormalizeProjectileDespawn(despawn)
	category, rule := domainEventObjectsProjectileDespawnRule(despawn)
	return domainEventObjectsFinish(c, domainEventObjectsRuleProjectileDespawn, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, despawn) == nil)
}

// domainEventObjectsRunItemDropUpserts admits one item-drop upsert batch.
func domainEventObjectsRunItemDropUpserts(c CaseSpec, spec domainEventObjectsInput) (Outcome, []byte, error) {
	upserts, err := domainEventObjectsDropUpserts(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventObjectsNormalizeDropUpserts(upserts)
	category, rule := domainEventObjectsDropUpsertsRule(upserts)
	return domainEventObjectsFinish(c, domainEventObjectsRuleItemDropUpserts, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, upserts) == nil)
}

// domainEventObjectsRunItemDropRemoves admits one item-drop remove batch,
// whose records are bare drop identities.
func domainEventObjectsRunItemDropRemoves(c CaseSpec, spec domainEventObjectsInput) (Outcome, []byte, error) {
	removes, err := domainEventObjectsDropRemoves(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventObjectsNormalizeDropRemoves(removes)
	category, rule := domainEventObjectsDropRemovesRule(removes)
	return domainEventObjectsFinish(c, domainEventObjectsRuleItemDropRemoves, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, removes) == nil)
}

// domainEventObjectsTick resolves the decimal-string server tick every rule
// requires. An absent field is a producer error rather than a silent zero,
// because the tick is a meaningful value the record always carries.
func domainEventObjectsTick(c CaseSpec, spec domainEventObjectsInput) (uint64, error) {
	if spec.ServerTick == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires a server tick", c.ID)
	}
	tick, err := strconv.ParseUint(*spec.ServerTick, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: case %s names server tick %q, which is not a decimal u64", c.ID, *spec.ServerTick)
	}
	return tick, nil
}

// domainEventObjectsID resolves one decimal-string identity a record names.
func domainEventObjectsID(c CaseSpec, name, text string) (uint64, error) {
	id, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not a decimal u64", c.ID, name, text)
	}
	return id, nil
}

// domainEventObjectsDropID resolves one drop identity from its frozen
// rendering. The dimension stays raw because the Go validity rule leaves it
// unvalidated, so an unusual dimension reaches the DTO exactly as the corpus
// names it.
func domainEventObjectsResolveDropID(c CaseSpec, name string, id domainEventObjectsDropID) (core.DropID, error) {
	if len(id.Chunk) != 2 {
		return core.DropID{}, fmt.Errorf("runtime-oracle: case %s requires two %s.chunk coordinates", c.ID, name)
	}
	return core.DropID{
		Dimension:  core.DimensionID(id.Dimension),
		Chunk:      core.ChunkPos{X: id.Chunk[0], Z: id.Chunk[1]},
		Slot:       id.Slot,
		Generation: id.Generation,
	}, nil
}

// domainEventObjectsProjectileSpawn resolves one projectile spawn batch from
// its frozen envelope. The batch is carried, not absent, so an empty batch is
// an explicit empty list rather than a missing key.
func domainEventObjectsProjectileSpawn(c CaseSpec, spec domainEventObjectsInput) (protocol.ProjectileSpawn, error) {
	tick, err := domainEventObjectsTick(c, spec)
	if err != nil {
		return protocol.ProjectileSpawn{}, err
	}
	if spec.ProjectileSpawns == nil {
		return protocol.ProjectileSpawn{}, fmt.Errorf("runtime-oracle: case %s requires a projectile spawn batch", c.ID)
	}
	spawns := make([]protocol.ProjectileSpawnRecord, 0, len(spec.ProjectileSpawns))
	for index, record := range spec.ProjectileSpawns {
		id, err := domainEventObjectsID(c, fmt.Sprintf("spawns[%d].id", index), record.ID)
		if err != nil {
			return protocol.ProjectileSpawn{}, err
		}
		position, err := domainEventObjectsVec3(c, fmt.Sprintf("spawns[%d].position", index), record.Position)
		if err != nil {
			return protocol.ProjectileSpawn{}, err
		}
		velocity, err := domainEventObjectsVec3(c, fmt.Sprintf("spawns[%d].velocity", index), record.Velocity)
		if err != nil {
			return protocol.ProjectileSpawn{}, err
		}
		spawns = append(spawns, protocol.ProjectileSpawnRecord{
			ID:        id,
			Kind:      record.Kind,
			Dimension: core.DimensionID(record.Dimension),
			Position:  position,
			Velocity:  velocity,
		})
	}
	return protocol.ProjectileSpawn{ServerTick: tick, Spawns: spawns}, nil
}

// domainEventObjectsProjectileState resolves one projectile state batch from
// its frozen envelope.
func domainEventObjectsProjectileState(c CaseSpec, spec domainEventObjectsInput) (protocol.ProjectileState, error) {
	tick, err := domainEventObjectsTick(c, spec)
	if err != nil {
		return protocol.ProjectileState{}, err
	}
	if spec.ProjectileStates == nil {
		return protocol.ProjectileState{}, fmt.Errorf("runtime-oracle: case %s requires a projectile state batch", c.ID)
	}
	states := make([]protocol.ProjectileStateRecord, 0, len(spec.ProjectileStates))
	for index, record := range spec.ProjectileStates {
		id, err := domainEventObjectsID(c, fmt.Sprintf("states[%d].id", index), record.ID)
		if err != nil {
			return protocol.ProjectileState{}, err
		}
		position, err := domainEventObjectsVec3(c, fmt.Sprintf("states[%d].position", index), record.Position)
		if err != nil {
			return protocol.ProjectileState{}, err
		}
		states = append(states, protocol.ProjectileStateRecord{ID: id, Position: position})
	}
	return protocol.ProjectileState{ServerTick: tick, States: states}, nil
}

// domainEventObjectsProjectileDespawn resolves one projectile despawn batch
// from its frozen envelope.
func domainEventObjectsProjectileDespawn(c CaseSpec, spec domainEventObjectsInput) (protocol.ProjectileDespawn, error) {
	tick, err := domainEventObjectsTick(c, spec)
	if err != nil {
		return protocol.ProjectileDespawn{}, err
	}
	if spec.ProjectileIDs == nil {
		return protocol.ProjectileDespawn{}, fmt.Errorf("runtime-oracle: case %s requires a projectile despawn batch", c.ID)
	}
	ids := make([]uint64, 0, len(spec.ProjectileIDs))
	for index, text := range spec.ProjectileIDs {
		id, err := domainEventObjectsID(c, fmt.Sprintf("ids[%d]", index), text)
		if err != nil {
			return protocol.ProjectileDespawn{}, err
		}
		ids = append(ids, id)
	}
	return protocol.ProjectileDespawn{ServerTick: tick, IDs: ids}, nil
}

// domainEventObjectsDropUpserts resolves one item-drop upsert batch from its
// frozen envelope.
func domainEventObjectsDropUpserts(c CaseSpec, spec domainEventObjectsInput) (protocol.ItemDropUpserts, error) {
	tick, err := domainEventObjectsTick(c, spec)
	if err != nil {
		return protocol.ItemDropUpserts{}, err
	}
	if spec.Drops == nil {
		return protocol.ItemDropUpserts{}, fmt.Errorf("runtime-oracle: case %s requires an item drop upsert batch", c.ID)
	}
	drops := make([]protocol.ItemDrop, 0, len(spec.Drops))
	for index, record := range spec.Drops {
		id, err := domainEventObjectsResolveDropID(c, fmt.Sprintf("drops[%d].id", index), record.ID)
		if err != nil {
			return protocol.ItemDropUpserts{}, err
		}
		drops = append(drops, protocol.ItemDrop{
			ID:         id,
			BlockIndex: record.BlockIndex,
			Item:       core.ItemID(record.Stack.Item),
			Count:      record.Stack.Count,
			Durability: record.Stack.Durability,
		})
	}
	return protocol.ItemDropUpserts{ServerTick: tick, Drops: drops}, nil
}

// domainEventObjectsDropRemoves resolves one item-drop remove batch from its
// frozen envelope.
func domainEventObjectsDropRemoves(c CaseSpec, spec domainEventObjectsInput) (protocol.ItemDropRemoves, error) {
	tick, err := domainEventObjectsTick(c, spec)
	if err != nil {
		return protocol.ItemDropRemoves{}, err
	}
	if spec.DropIDs == nil {
		return protocol.ItemDropRemoves{}, fmt.Errorf("runtime-oracle: case %s requires an item drop remove batch", c.ID)
	}
	ids := make([]core.DropID, 0, len(spec.DropIDs))
	for index, record := range spec.DropIDs {
		id, err := domainEventObjectsResolveDropID(c, fmt.Sprintf("ids[%d]", index), record)
		if err != nil {
			return protocol.ItemDropRemoves{}, err
		}
		ids = append(ids, id)
	}
	return protocol.ItemDropRemoves{ServerTick: tick, IDs: ids}, nil
}

// domainEventObjectsFinish records one admission.
//
// The classification is cross-checked against the authority's verdict, a
// rejection publishes the broken rule beside its category, and an admitted
// record publishes its normalized field map. No codec round trip is recorded:
// this family pins the semantic field values, and the wire layout of these
// packets belongs to the protocol nodes that port the codecs.
func domainEventObjectsFinish(c CaseSpec, subject string, fields map[string]any, category, rule string, admitted bool) (Outcome, []byte, error) {
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

// domainEventObjectsNormalizeProjectileSpawn renders one projectile spawn
// batch in the normalized field map. The tick and the identities are decimal
// strings, the pose and the velocity are hexadecimal IEEE-754 bit strings,
// and every record is published in the submitted order.
func domainEventObjectsNormalizeProjectileSpawn(spawn protocol.ProjectileSpawn) map[string]any {
	records := make([]map[string]any, 0, len(spawn.Spawns))
	for _, record := range spawn.Spawns {
		records = append(records, map[string]any{
			"id":        domainEventObjectsIDText(record.ID),
			"kind":      int(record.Kind),
			"dimension": int(record.Dimension),
			"position":  domainEventObjectsVectorBits(record.Position),
			"velocity":  domainEventObjectsVectorBits(record.Velocity),
		})
	}
	return map[string]any{
		"server_tick": domainEventObjectsIDText(spawn.ServerTick),
		"spawns":      records,
	}
}

// domainEventObjectsNormalizeProjectileState renders one projectile state
// batch in the normalized field map. No kind, dimension or velocity is
// published, exactly as the wire record carries none.
func domainEventObjectsNormalizeProjectileState(state protocol.ProjectileState) map[string]any {
	records := make([]map[string]any, 0, len(state.States))
	for _, record := range state.States {
		records = append(records, map[string]any{
			"id":       domainEventObjectsIDText(record.ID),
			"position": domainEventObjectsVectorBits(record.Position),
		})
	}
	return map[string]any{
		"server_tick": domainEventObjectsIDText(state.ServerTick),
		"states":      records,
	}
}

// domainEventObjectsNormalizeProjectileDespawn renders one projectile despawn
// batch in the normalized field map, whose records are bare identities.
func domainEventObjectsNormalizeProjectileDespawn(despawn protocol.ProjectileDespawn) map[string]any {
	ids := make([]string, 0, len(despawn.IDs))
	for _, id := range despawn.IDs {
		ids = append(ids, domainEventObjectsIDText(id))
	}
	return map[string]any{
		"server_tick": domainEventObjectsIDText(despawn.ServerTick),
		"ids":         ids,
	}
}

// domainEventObjectsNormalizeDropUpserts renders one item-drop upsert batch
// in the normalized field map. Each drop identity stays the object of its
// five ordered key fields and each stack the numeric `item/count/durability`
// object, exactly as the corpus vocabulary freezes them.
func domainEventObjectsNormalizeDropUpserts(upserts protocol.ItemDropUpserts) map[string]any {
	records := make([]map[string]any, 0, len(upserts.Drops))
	for _, drop := range upserts.Drops {
		records = append(records, map[string]any{
			"id":          domainEventObjectsNormalizeDropID(drop.ID),
			"block_index": int(drop.BlockIndex),
			"stack": map[string]any{
				"item":       int(drop.Item),
				"count":      int(drop.Count),
				"durability": int(drop.Durability),
			},
		})
	}
	return map[string]any{
		"server_tick": domainEventObjectsIDText(upserts.ServerTick),
		"drops":       records,
	}
}

// domainEventObjectsNormalizeDropRemoves renders one item-drop remove batch
// in the normalized field map, whose records are bare drop identities.
func domainEventObjectsNormalizeDropRemoves(removes protocol.ItemDropRemoves) map[string]any {
	ids := make([]map[string]any, 0, len(removes.IDs))
	for _, id := range removes.IDs {
		ids = append(ids, domainEventObjectsNormalizeDropID(id))
	}
	return map[string]any{
		"server_tick": domainEventObjectsIDText(removes.ServerTick),
		"ids":         ids,
	}
}

// domainEventObjectsNormalizeDropID renders one drop identity as the object
// of its five ordered key fields, keeping the raw dimension exactly as the
// record carried it.
func domainEventObjectsNormalizeDropID(id core.DropID) map[string]any {
	return map[string]any{
		"dimension":  int(id.Dimension),
		"chunk":      []int{int(id.Chunk.X), int(id.Chunk.Z)},
		"slot":       int(id.Slot),
		"generation": int(id.Generation),
	}
}

// domainEventObjectsProjectileSpawnRule names the first rule one projectile
// spawn batch breaks, in the same order the Go validator checks them: the
// batch count bound, then every record in batch order, then the strict
// identity order.
//
// A rejection classifies as `invalid-identity` for a zero entity ID, as
// `invalid-enum` for an unknown kind or dimension, and as `invalid-value`
// otherwise, which is the classification rule a Rust consumer of this family
// applies to its own rejection variants. The count bound names both ends
// because the upper end is a wire budget no case sits above.
func domainEventObjectsProjectileSpawnRule(spawn protocol.ProjectileSpawn) (string, string) {
	if len(spawn.Spawns) < 1 || len(spawn.Spawns) > protocol.MaxProjectileRecords {
		return "invalid-value", domainEventObjectsNameProjectileSpawn + ".count_range"
	}
	for index, record := range spawn.Spawns {
		if category, field := domainEventObjectsProjectileSpawnRecordRule(record); field != "" {
			return category, fmt.Sprintf("%s.record_%d.%s", domainEventObjectsNameProjectileSpawn, index, field)
		}
		if index > 0 && spawn.Spawns[index-1].ID >= record.ID {
			return "invalid-value", domainEventObjectsNameProjectileSpawn + ".strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventObjectsProjectileSpawnRecordRule names the first rule one
// projectile spawn record breaks: the identity, the kind domain, the two
// playable dimensions, and the finite pose and velocity. The kind-by-
// dimension policy is an authority concern the wire layer does not enforce,
// so every combination of a legal kind and a legal dimension is publishable
// here.
func domainEventObjectsProjectileSpawnRecordRule(record protocol.ProjectileSpawnRecord) (string, string) {
	if record.ID == 0 {
		return "invalid-identity", "id"
	}
	if record.Kind != protocol.ProjectileKindShard && record.Kind != protocol.ProjectileKindArrow {
		return "invalid-enum", "kind"
	}
	if record.Dimension != core.Overworld && record.Dimension != core.Depths {
		return "invalid-enum", "dimension"
	}
	if !domainEventObjectsFiniteVec3(record.Position) {
		return "invalid-value", "position"
	}
	if !domainEventObjectsFiniteVec3(record.Velocity) {
		return "invalid-value", "velocity"
	}
	return "", ""
}

// domainEventObjectsProjectileStateRule names the first rule one projectile
// state batch breaks, in the same order the Go validator checks them.
func domainEventObjectsProjectileStateRule(state protocol.ProjectileState) (string, string) {
	if len(state.States) < 1 || len(state.States) > protocol.MaxProjectileRecords {
		return "invalid-value", domainEventObjectsNameProjectileState + ".count_range"
	}
	for index, record := range state.States {
		if category, field := domainEventObjectsProjectileStateRecordRule(record); field != "" {
			return category, fmt.Sprintf("%s.record_%d.%s", domainEventObjectsNameProjectileState, index, field)
		}
		if index > 0 && state.States[index-1].ID >= record.ID {
			return "invalid-value", domainEventObjectsNameProjectileState + ".strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventObjectsProjectileStateRecordRule names the first rule one
// projectile state record breaks: the identity and the finite pose.
func domainEventObjectsProjectileStateRecordRule(record protocol.ProjectileStateRecord) (string, string) {
	if record.ID == 0 {
		return "invalid-identity", "id"
	}
	if !domainEventObjectsFiniteVec3(record.Position) {
		return "invalid-value", "position"
	}
	return "", ""
}

// domainEventObjectsProjectileDespawnRule names the first rule one projectile
// despawn batch breaks: the batch count bound, then the nonzero identities
// and the strict identity order.
func domainEventObjectsProjectileDespawnRule(despawn protocol.ProjectileDespawn) (string, string) {
	if len(despawn.IDs) < 1 || len(despawn.IDs) > protocol.MaxProjectileRecords {
		return "invalid-value", domainEventObjectsNameProjectileDespawn + ".count_range"
	}
	for index, id := range despawn.IDs {
		if id == 0 {
			return "invalid-identity", fmt.Sprintf("%s.record_%d.id", domainEventObjectsNameProjectileDespawn, index)
		}
		if index > 0 && despawn.IDs[index-1] >= id {
			return "invalid-value", domainEventObjectsNameProjectileDespawn + ".strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventObjectsDropUpsertsRule names the first rule one item-drop upsert
// batch breaks, in the same order the Go validator checks them: the batch
// count bound, then every record in batch order, then the strict identity
// order.
func domainEventObjectsDropUpsertsRule(upserts protocol.ItemDropUpserts) (string, string) {
	if len(upserts.Drops) < 1 || len(upserts.Drops) > protocol.MaxItemDropBatch {
		return "invalid-value", domainEventObjectsNameItemDropUpserts + ".count_range"
	}
	for index, drop := range upserts.Drops {
		if category, field := domainEventObjectsDropRecordRule(drop); field != "" {
			return category, fmt.Sprintf("%s.drop_%d.%s", domainEventObjectsNameItemDropUpserts, index, field)
		}
		if index > 0 && upserts.Drops[index-1].ID.Compare(drop.ID) >= 0 {
			return "invalid-value", domainEventObjectsNameItemDropUpserts + ".strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventObjectsDropRecordRule names the first rule one item-drop upsert
// record breaks: the identity's slot range and generation, the block index
// inside the announced chunk, and the carried stack. The raw dimension is
// deliberately not named, because the Go `DropID.Valid` rule leaves it
// unvalidated and the ordering compares it as a raw number.
func domainEventObjectsDropRecordRule(drop protocol.ItemDrop) (string, string) {
	if drop.ID.Slot >= core.DropsPerChunk {
		return "invalid-identity", "id.slot"
	}
	if drop.ID.Generation == 0 {
		return "invalid-identity", "id.generation"
	}
	if drop.BlockIndex >= domainEventObjectsMaxBlockIndex() {
		return "invalid-value", "block_index"
	}
	return domainEventObjectsStackRule(core.ItemStack{Item: drop.Item, Count: drop.Count, Durability: drop.Durability})
}

// domainEventObjectsStackRule names the first rule one carried stack breaks,
// in the same precedence the Go `core.ItemStack.Valid` rule applies:
// registration first, then the count, then the durability. The exactly-zero
// triple is the canonical empty stack and stays admitted.
func domainEventObjectsStackRule(stack core.ItemStack) (string, string) {
	limit, ok := core.ItemStackLimit(stack.Item)
	if !ok {
		if stack.Item == core.ItemNone && stack.Count == 0 && stack.Durability == 0 {
			return "", ""
		}
		return "invalid-value", "stack.item"
	}
	if stack.Count == 0 || stack.Count > limit {
		return "invalid-value", "stack.count"
	}
	maxDurability, hasDurability := core.ItemMaxDurability(stack.Item)
	if !hasDurability {
		if stack.Durability != 0 {
			return "invalid-value", "stack.durability"
		}
		return "", ""
	}
	if stack.Durability < 1 || stack.Durability > maxDurability {
		return "invalid-value", "stack.durability"
	}
	return "", ""
}

// domainEventObjectsDropRemovesRule names the first rule one item-drop remove
// batch breaks: the batch count bound, then each identity's slot range and
// generation, then the strict identity order.
func domainEventObjectsDropRemovesRule(removes protocol.ItemDropRemoves) (string, string) {
	if len(removes.IDs) < 1 || len(removes.IDs) > protocol.MaxItemDropBatch {
		return "invalid-value", domainEventObjectsNameItemDropRemoves + ".count_range"
	}
	for index, id := range removes.IDs {
		if category, field := domainEventObjectsDropIDRule(id); field != "" {
			return category, fmt.Sprintf("%s.id_%d.%s", domainEventObjectsNameItemDropRemoves, index, field)
		}
		if index > 0 && removes.IDs[index-1].Compare(id) >= 0 {
			return "invalid-value", domainEventObjectsNameItemDropRemoves + ".strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventObjectsDropIDRule names the first rule one bare drop identity
// breaks: the slot range, then the nonzero generation.
func domainEventObjectsDropIDRule(id core.DropID) (string, string) {
	if id.Slot >= core.DropsPerChunk {
		return "invalid-identity", "slot"
	}
	if id.Generation == 0 {
		return "invalid-identity", "generation"
	}
	return "", ""
}

// domainEventObjectsFinite32 reports whether one float is neither NaN nor an
// infinity, which is the Go protocol package's private `finite32` predicate.
//
// The producer keeps this copy because the predicate is unexported, and the
// copy is pinned by the same non-finite boundary cases the domain test pins.
func domainEventObjectsFinite32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

// domainEventObjectsFiniteVec3 reports whether every component of one vector
// is finite, which is the Go protocol package's private `finiteVec3`
// predicate.
func domainEventObjectsFiniteVec3(value mgl32.Vec3) bool {
	for _, component := range value {
		if !domainEventObjectsFinite32(component) {
			return false
		}
	}
	return true
}

// domainEventObjectsVectorBits renders one f32 vector as the three
// hexadecimal IEEE-754 bit strings the normalized corpus vocabulary uses. A
// non-finite component renders its exact bits, so a rejection shows the exact
// value the record carried rather than a rounded decimal.
func domainEventObjectsVectorBits(vector mgl32.Vec3) []string {
	return []string{
		domainEventObjectsFloatBits(vector.X()),
		domainEventObjectsFloatBits(vector.Y()),
		domainEventObjectsFloatBits(vector.Z()),
	}
}

// domainEventObjectsFloatBits renders one float32 as the eight-digit
// hexadecimal IEEE-754 bit string the normalized corpus vocabulary uses.
func domainEventObjectsFloatBits(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

// domainEventObjectsFloatText renders one float32 as decimal float text, so
// NaN, the infinities and a signed zero stay expressible in the frozen input.
// The normalized outcome renders the same value as its IEEE-754 bit string,
// so the input and the outcome pin the value in two independent renderings.
func domainEventObjectsFloatText(value float32) string {
	return strconv.FormatFloat(float64(value), 'g', -1, 32)
}

// domainEventObjectsVectorText renders one vector as its three decimal float
// texts.
func domainEventObjectsVectorText(vector mgl32.Vec3) []string {
	return []string{
		domainEventObjectsFloatText(vector.X()),
		domainEventObjectsFloatText(vector.Y()),
		domainEventObjectsFloatText(vector.Z()),
	}
}

// domainEventObjectsVec3 resolves one position or velocity from its three
// decimal float texts.
func domainEventObjectsVec3(c CaseSpec, name string, components []string) (mgl32.Vec3, error) {
	if len(components) != 3 {
		return mgl32.Vec3{}, fmt.Errorf("runtime-oracle: case %s requires three %s components", c.ID, name)
	}
	values := make([]float32, 0, 3)
	for index, component := range components {
		value, err := strconv.ParseFloat(component, 32)
		if err != nil {
			return mgl32.Vec3{}, fmt.Errorf("runtime-oracle: case %s names %s[%d] %q, which is not a float32", c.ID, name, index, component)
		}
		values = append(values, float32(value))
	}
	return mgl32.Vec3{values[0], values[1], values[2]}, nil
}

// domainEventObjectsDecodeInput reads one frozen corpus input and rejects
// trailing content, so a producer never executes bytes the case did not name.
func domainEventObjectsDecodeInput(data []byte) (domainEventObjectsInput, error) {
	var spec domainEventObjectsInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return domainEventObjectsInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainEventObjectsInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}

// domainEventObjectsRecord pairs one executed case with the input and outcome
// the producer produced for it.
type domainEventObjectsRecord struct {
	label   string
	input   domainEventObjectsInput
	outcome Outcome
}

// domainEventObjectsExecute runs the whole case table through the producer
// and returns one record per case in table order.
func domainEventObjectsExecute(t *testing.T) []domainEventObjectsRecord {
	t.Helper()

	cases := domainEventObjectsCases()
	records := make([]domainEventObjectsRecord, 0, len(cases))
	for _, entry := range cases {
		input, err := json.MarshalIndent(entry.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", entry.label, err)
		}
		input = append(input, '\n')
		outcome, _, err := runDomainEventObjects(CaseSpec{ID: domainEventObjectsCaseID(entry.label)}, input)
		if err != nil {
			t.Fatalf("execute case %s: %v", entry.label, err)
		}
		records = append(records, domainEventObjectsRecord{label: entry.label, input: entry.input, outcome: outcome})
	}
	return records
}

// `domainEventObjectsSyncCorpus` compares committed assets and optionally
// exports a complete producer candidate for controller review.
//
// Ordinary runs compare committed assets read-only against current producer
// output. The explicit export publishes the complete candidate to a fresh
// external directory before the committed bytes are compared, so an initial
// export run creates every candidate file even while the tracked directory is
// still absent, and never mutates tracked assets.
func domainEventObjectsSyncCorpus(t *testing.T, records []domainEventObjectsRecord) {
	t.Helper()

	root := mustRepoRoot(t)
	corpusDir := filepath.Join(root, filepath.FromSlash(domainEventObjectsCorpusRelDir))
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
	domainEventObjectsExport(t, root, assets)

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
}

// domainEventObjectsExport publishes the complete candidate once per test
// process when the export directory is named. The topic entry delegates to
// the execute-every-case test, so one filter selecting both entries executes
// the same deterministic candidate twice, and the exporter's exclusive
// producer child must not treat the second publication as a conflict.
func domainEventObjectsExport(t *testing.T, root string, assets []generatedAsset) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv)) == "" {
		return
	}
	if domainEventObjectsExportPublished {
		return
	}
	domainEventObjectsExportPublished = true
	exportGeneratedAssetsFromEnvironment(t, root, domainEventObjectsProducerID, assets)
}

// domainEventObjectsCaseID renders the manifest case identity one corpus
// label carries. The version segment is the family's published version,
// because a case has to name the version of the family it belongs to.
func domainEventObjectsCaseID(label string) string {
	return domainEventObjectsFamily + "/" + domainEventObjectsVersion + "/" + label
}

// `domainEventObjectsWorkingManifest` clones the merged committed manifest
// into a producer-scoped selection stored in harness-owned temporary storage.
// `Cases` is narrowed to the projectile and item-drop cases and unrelated
// family case lists are cleared. The existing `domain.event` identity is
// retained while its provenance and case list are replaced with this
// producer's current selection, because the canonical manifest does not yet
// register these cases; that registration is a later node's work.
func domainEventObjectsWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	cases := domainEventObjectsCorpusCases(t, root)

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
	sources := make([]SourceSpec, 0, len(domainEventObjectsFamilySources))
	for _, relative := range domainEventObjectsFamilySources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	registered := false
	for index := range cloned.Families {
		if cloned.Families[index].ID != domainEventObjectsFamily {
			cloned.Families[index].Cases = nil
			continue
		}
		cloned.Families[index].Sources = sources
		cloned.Families[index].Cases = caseIDs
		registered = true
	}
	if !registered {
		t.Fatalf("working manifest has no %s family", domainEventObjectsFamily)
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

// domainEventObjectsCorpusCases reads the frozen corpus and registers one
// case per committed input, with digests proven against the files on disk.
func domainEventObjectsCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(domainEventObjectsCorpusRelDir))
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
		t.Fatalf("walk event objects corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no event objects case under %s", domainEventObjectsCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		label := strings.TrimSuffix(filepath.ToSlash(relative), ".input.json")
		if label == "" || !domainEventObjectsLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug with an optional numeric suffix", filepath.ToSlash(relative), label)
		}
		envelope := domainEventObjectsReadEnvelope(t, input)
		if envelope.Consumer != domainEventObjectsConsumer {
			t.Fatalf("corpus case %s names consumer %q, want %q", label, envelope.Consumer, domainEventObjectsConsumer)
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
			ID:           domainEventObjectsCaseID(label),
			Family:       domainEventObjectsFamily,
			Version:      domainEventObjectsVersion,
			Operation:    domainEventObjectsOperation,
			Input:        AssetRef{Path: domainEventObjectsCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: domainEventObjectsCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: domainEventObjectsConsumer,
		})
	}
	return cases
}

// domainEventObjectsReadEnvelope reads the provenance envelope of one frozen
// corpus input.
func domainEventObjectsReadEnvelope(t *testing.T, path string) domainEventObjectsInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope domainEventObjectsInput
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// domainEventObjectsCorpusPath renders one absolute corpus path as the
// repository-relative slash path the manifest requires.
func domainEventObjectsCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}

// TestDomainEventObjectsOracleExecutesEveryCase runs the whole case table
// through the real Go protocol DTOs, proves the frozen corpus still matches
// what they produce, and then runs the same cases through the production
// runner so the executed evidence satisfies the completeness rules a
// published trace report does.
func TestDomainEventObjectsOracleExecutesEveryCase(t *testing.T) {
	records := domainEventObjectsExecute(t)
	domainEventObjectsSyncCorpus(t, records)

	root := mustRepoRoot(t)
	manifest := domainEventObjectsWorkingManifest(t, root)
	if len(manifest.Cases) != len(records) {
		t.Fatalf("working manifest registers %d cases, want %d (one per executed table row)", len(manifest.Cases), len(records))
	}
	domainEventObjectsAssertCaseSpecs(t, root, manifest)
	domainEventObjectsAssertOutcomesDistinguishCases(t, records)

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for _, obs := range observations {
		c := domainEventObjectsCaseByID(t, manifest, obs.CaseID)
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
		c := domainEventObjectsCaseByID(t, manifest, obs.CaseID)
		if obs.ExpectedDigest != c.Expected.SHA256 {
			t.Fatalf("observation for %s carries expected digest %s, want %s", obs.CaseID, obs.ExpectedDigest, c.Expected.SHA256)
		}
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event objects evidence failed trace validation: %v", err)
	}
}

// TestDomainEventObjectsOracleCaseIdentitiesAreTheRustDomainConsumer pins
// the manifest identity of every event objects case: the operation, the
// family version, the family and the consumer the change names for the Rust
// crate that owns these records.
func TestDomainEventObjectsOracleCaseIdentitiesAreTheRustDomainConsumer(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventObjectsWorkingManifest(t, root)

	labels := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != domainEventObjectsOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, domainEventObjectsOperation)
		}
		if c.Version != domainEventObjectsVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, domainEventObjectsVersion)
		}
		if c.Family != domainEventObjectsFamily {
			t.Fatalf("case %s declares family %q, want %q", c.ID, c.Family, domainEventObjectsFamily)
		}
		if c.RustConsumer != domainEventObjectsConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, domainEventObjectsConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		prefix := c.Family + "/" + c.Version + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !domainEventObjectsLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug with an optional numeric suffix", c.ID, label)
		}
		if labels[label] {
			t.Fatalf("label %q is registered twice", label)
		}
		labels[label] = true

		envelope := domainEventObjectsReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != domainEventObjectsConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, domainEventObjectsConsumer)
		}
		switch envelope.Rule {
		case domainEventObjectsRuleProjectileSpawn, domainEventObjectsRuleProjectileState,
			domainEventObjectsRuleProjectileDespawn, domainEventObjectsRuleItemDropUpserts,
			domainEventObjectsRuleItemDropRemoves:
		default:
			t.Fatalf("case %s names rule %q, which is not a projectile or item-drop observation rule", c.ID, envelope.Rule)
		}
	}
}

// TestDomainEventObjectsOracleWorkingManifestDescribesItself proves the
// working manifest is internally consistent: every case passes the production
// case validation, the family's provenance hashes match disk, and the
// family's case list matches exactly the cases the selection registers.
func TestDomainEventObjectsOracleWorkingManifestDescribesItself(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventObjectsWorkingManifest(t, root)
	domainEventObjectsAssertCaseSpecs(t, root, manifest)

	family, ok := domainEventObjectsFamilySpec(manifest)
	if !ok {
		t.Fatalf("working manifest has no %s family", domainEventObjectsFamily)
	}
	if family.Role != "event" || family.Kind != "domain" {
		t.Fatalf("%s declares kind %q role %q, want domain event", domainEventObjectsFamily, family.Kind, family.Role)
	}
	if family.CurrentVersion != domainEventObjectsVersion {
		t.Fatalf("%s declares version %q, want %q", domainEventObjectsFamily, family.CurrentVersion, domainEventObjectsVersion)
	}
	if len(family.Sources) != len(domainEventObjectsFamilySources) {
		t.Fatalf("%s records %d provenance sources, want %d", domainEventObjectsFamily, len(family.Sources), len(domainEventObjectsFamilySources))
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
		t.Fatalf("%s lists %d cases, want %d", domainEventObjectsFamily, len(family.Cases), len(registered))
	}
	for _, id := range family.Cases {
		if !registered[id] {
			t.Fatalf("%s lists case %s which the selection does not register", domainEventObjectsFamily, id)
		}
	}
}

// TestDomainEventObjectsOracleOutcomesDistinguishAcceptedAndRejected pins
// the exact shape of the executed evidence: the 45 unique labels, both an
// admitted record and a rejection for every rule, the accepted zero tick, the
// complete kind-by-dimension matrix, the accepted raw dimension -1, the
// inclusive and exclusive block-index ends, the canonical empty stack, and
// every aggregate and field rule the classifiers name.
func TestDomainEventObjectsOracleOutcomesDistinguishAcceptedAndRejected(t *testing.T) {
	records := domainEventObjectsExecute(t)
	domainEventObjectsAssertOutcomesDistinguishCases(t, records)

	labels := make(map[string]bool, len(records))
	for _, record := range records {
		if labels[record.label] {
			t.Fatalf("label %q executes twice", record.label)
		}
		labels[record.label] = true
	}
	if len(labels) != domainEventObjectsCaseTotal {
		t.Fatalf("executed evidence carries %d unique labels, want %d", len(labels), domainEventObjectsCaseTotal)
	}

	tally := make(map[string]*domainEventObjectsRuleTally)
	perRule := make(map[string]int, len(records))
	for _, record := range records {
		perRule[record.input.Rule]++
		entry, ok := tally[record.input.Rule]
		if !ok {
			entry = &domainEventObjectsRuleTally{}
			tally[record.input.Rule] = entry
		}
		switch record.outcome.Kind {
		case "ok":
			entry.accepted = append(entry.accepted, record.outcome.Fields)
		case "error":
			entry.rejected = append(entry.rejected, record.outcome.Fields)
		}
	}
	for rule, want := range map[string]int{
		domainEventObjectsRuleProjectileSpawn:   10,
		domainEventObjectsRuleProjectileState:   7,
		domainEventObjectsRuleProjectileDespawn: 6,
		domainEventObjectsRuleItemDropUpserts:   14,
		domainEventObjectsRuleItemDropRemoves:   8,
	} {
		if perRule[rule] != want {
			t.Fatalf("rule %s executes %d cases, want %d", rule, perRule[rule], want)
		}
	}
	for rule, entry := range tally {
		if len(entry.accepted) == 0 || len(entry.rejected) == 0 {
			t.Fatalf("rule %s records %d accepted and %d rejected cases, want both non-zero",
				rule, len(entry.accepted), len(entry.rejected))
		}
	}

	// The Go validators publish no tick rule for these records, so the zero
	// tick is an admitted boundary every rule pins.
	for _, rule := range []string{
		domainEventObjectsRuleProjectileSpawn, domainEventObjectsRuleProjectileState,
		domainEventObjectsRuleProjectileDespawn, domainEventObjectsRuleItemDropUpserts,
		domainEventObjectsRuleItemDropRemoves,
	} {
		if !domainEventObjectsFieldsContain(tally[rule].accepted, "server_tick", "0") {
			t.Fatalf("no admitted %s batch carries the zero tick", rule)
		}
	}
	// The raw dimension -1 is an admitted boundary both drop rules pin,
	// because the Go `DropID.Valid` rule leaves the dimension unvalidated.
	for _, rule := range []string{
		domainEventObjectsRuleItemDropUpserts, domainEventObjectsRuleItemDropRemoves,
	} {
		if !domainEventObjectsIDArrayContainsRawDimension(tally[rule].accepted, -1) {
			t.Fatalf("no admitted %s batch carries the raw dimension -1", rule)
		}
	}
	// The inclusive block-index end stays admitted while the exclusive end
	// one step above it is the rejection, and the canonical empty stack is
	// an admitted stack value.
	if !domainEventObjectsNestedContains(tally[domainEventObjectsRuleItemDropUpserts].accepted, "drops", "block_index", int(domainEventObjectsMaxBlockIndex()-1)) {
		t.Fatalf("no admitted item drop upsert carries the inclusive block-index end %d", domainEventObjectsMaxBlockIndex()-1)
	}
	for _, stack := range []map[string]any{
		{"item": 0, "count": 0, "durability": 0},
	} {
		if !domainEventObjectsDropsContainStack(tally[domainEventObjectsRuleItemDropUpserts].accepted, stack) {
			t.Fatalf("no admitted item drop upsert carries the canonical empty stack %v", stack)
		}
	}

	// Every boundary below is looked up by the exact rule name the
	// classifier publishes, so a renamed or dropped boundary fails here
	// rather than surviving as an unpinned rule.
	for _, rule := range []string{
		domainEventObjectsNameProjectileSpawn + ".count_range",
		domainEventObjectsNameProjectileSpawn + ".strictly_increasing_ids",
		domainEventObjectsNameProjectileSpawn + ".record_0.id",
		domainEventObjectsNameProjectileSpawn + ".record_0.kind",
		domainEventObjectsNameProjectileSpawn + ".record_0.dimension",
		domainEventObjectsNameProjectileSpawn + ".record_0.position",
		domainEventObjectsNameProjectileSpawn + ".record_0.velocity",
		domainEventObjectsNameProjectileState + ".count_range",
		domainEventObjectsNameProjectileState + ".strictly_increasing_ids",
		domainEventObjectsNameProjectileState + ".record_0.id",
		domainEventObjectsNameProjectileState + ".record_0.position",
		domainEventObjectsNameProjectileDespawn + ".count_range",
		domainEventObjectsNameProjectileDespawn + ".strictly_increasing_ids",
		domainEventObjectsNameProjectileDespawn + ".record_0.id",
		domainEventObjectsNameItemDropUpserts + ".count_range",
		domainEventObjectsNameItemDropUpserts + ".strictly_increasing_ids",
		domainEventObjectsNameItemDropUpserts + ".drop_0.id.slot",
		domainEventObjectsNameItemDropUpserts + ".drop_0.id.generation",
		domainEventObjectsNameItemDropUpserts + ".drop_0.block_index",
		domainEventObjectsNameItemDropUpserts + ".drop_0.stack.item",
		domainEventObjectsNameItemDropUpserts + ".drop_0.stack.count",
		domainEventObjectsNameItemDropUpserts + ".drop_0.stack.durability",
		domainEventObjectsNameItemDropRemoves + ".count_range",
		domainEventObjectsNameItemDropRemoves + ".strictly_increasing_ids",
		domainEventObjectsNameItemDropRemoves + ".id_0.slot",
		domainEventObjectsNameItemDropRemoves + ".id_0.generation",
	} {
		if !domainEventObjectsRejectionNamesRule(tally, rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", rule)
		}
	}
}

// TestDomainEventObjectsOracleRunnerRejectsUnregisteredFamily pins that a
// case cannot pass by being ignored.
func TestDomainEventObjectsOracleRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventObjectsWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "domain.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family domain.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestDomainEventObjectsOracleRunnerRejectsOperationFamilyMismatch pins that
// a case cannot declare one operation and be executed by another.
func TestDomainEventObjectsOracleRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventObjectsWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "decode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation-family mismatch failure, got: %v", err)
	}
}

// TestDomainEventObjectsOracleRunnerRejectsTamperedInput pins that a case
// whose committed bytes no longer match their recorded digest fails the run
// instead of executing bytes the manifest does not name.
func TestDomainEventObjectsOracleRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventObjectsWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestDomainEventObjectsOracleReportPublishesAndValidates assembles the
// executed evidence into a report, publishes it through the production atomic
// exporter, reloads it and validates it against the working manifest. This is
// the identity and content check a later Rust acceptance step performs; it
// does not claim any Rust behaviour.
func TestDomainEventObjectsOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventObjectsWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event objects evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, domainEventObjectsCorpusReportName)
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

// TestDomainEventObjectsOracleRejectsMissingProducerTest pins that the
// executed evidence has a real producer behind it, including the topic-named
// entry point the domain plan's filter selects. A corpus whose producer test
// is gone has no independent execution, only frozen files, and a missing
// topic entry point would make the plan filter pass without running anything.
func TestDomainEventObjectsOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(domainEventObjectsProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", domainEventObjectsProducerTestRelPath, err)
	}
	for _, name := range []string{domainEventObjectsProducerExecuteName, domainEventObjectsProducerTopicName} {
		declaration := "func " + name + "(t *testing.T) {"
		if !strings.Contains(string(data), declaration) {
			t.Fatalf("%s does not declare %s", domainEventObjectsProducerTestRelPath, declaration)
		}
	}
}

// TestDomainEventObjectsExportRejectsRepositoryAndSymlinkTargets pins the
// publication boundaries of this producer's export identity: an export root
// inside the repository is refused before anything is created, and an export
// root reached through a symlinked ancestor is refused without writing
// through the link.
func TestDomainEventObjectsExportRejectsRepositoryAndSymlinkTargets(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	assets := []generatedAsset{{RelativePath: "projectile-spawn-all-kind-dimension-combinations.input.json", Data: []byte("{}\n")}}

	contained := filepath.Join(root, "objects-export-probe")
	_, err := exportGeneratedAssets(root, contained, domainEventObjectsProducerID, assets)
	if err == nil || !strings.Contains(err.Error(), "live-path") {
		t.Fatalf("expected a live-path rejection for a repository-contained root, got: %v", err)
	}
	if _, statErr := os.Lstat(contained); !os.IsNotExist(statErr) {
		t.Fatalf("export directory was created inside repository: %v", statErr)
	}

	parent := t.TempDir()
	realDir := filepath.Join(parent, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkDir := filepath.Join(parent, "symlink-ancestor")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Fatal(err)
	}
	_, err = exportGeneratedAssets(root, filepath.Join(symlinkDir, "export"), domainEventObjectsProducerID, assets)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected a symlink rejection for a symlinked ancestor, got: %v", err)
	}
	entries, readErr := os.ReadDir(realDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("export wrote through the symlinked ancestor: %v", entries)
	}
}

// TestDomainEventObjectsProjectileKindDimensionMatrixIsComplete pins the
// seed's kind-by-dimension coverage: the four records enumerate every
// `(Shard|Arrow) × (Overworld|Depths)` combination, the authority admits the
// whole batch, and the normalized evidence publishes the first record's exact
// pose and velocity bits, including the velocity X `3e800000` the change
// names.
func TestDomainEventObjectsProjectileKindDimensionMatrixIsComplete(t *testing.T) {
	entry := domainEventObjectsProjectileSpawnCase("projectile-spawn-all-kind-dimension-combinations", nil)
	input, err := json.MarshalIndent(entry.input, "", "  ")
	if err != nil {
		t.Fatalf("encode corpus input: %v", err)
	}
	outcome, _, err := runDomainEventObjects(CaseSpec{ID: domainEventObjectsCaseID(entry.label)}, input)
	if err != nil {
		t.Fatalf("execute seed case: %v", err)
	}
	if outcome.Kind != "ok" {
		t.Fatalf("seed outcome kind = %q, want ok", outcome.Kind)
	}
	spawns, ok := outcome.Fields["spawns"].([]map[string]any)
	if !ok {
		t.Fatalf("seed outcome carries no spawns array: %#v", outcome.Fields["spawns"])
	}
	if len(spawns) != 4 {
		t.Fatalf("seed outcome carries %d spawn records, want 4", len(spawns))
	}

	seen := make(map[int]bool, 4)
	for _, record := range spawns {
		kind, kindOk := record["kind"].(int)
		dimension, dimensionOk := record["dimension"].(int)
		if !kindOk || !dimensionOk {
			t.Fatalf("seed record carries a non-numeric kind or dimension: %#v", record)
		}
		switch {
		case kind == int(protocol.ProjectileKindShard) && dimension == int(core.Overworld):
			seen[0] = true
		case kind == int(protocol.ProjectileKindShard) && dimension == int(core.Depths):
			seen[1] = true
		case kind == int(protocol.ProjectileKindArrow) && dimension == int(core.Overworld):
			seen[2] = true
		case kind == int(protocol.ProjectileKindArrow) && dimension == int(core.Depths):
			seen[3] = true
		default:
			t.Fatalf("seed record carries kind %d dimension %d, which is not a published pair", kind, dimension)
		}
	}
	for index, combination := range []string{
		"shard/overworld", "shard/depths", "arrow/overworld", "arrow/depths",
	} {
		if !seen[index] {
			t.Fatalf("seed does not cover the %s combination", combination)
		}
	}

	first := spawns[0]
	if first["id"] != "1" {
		t.Fatalf("first seed record id = %v, want 1", first["id"])
	}
	position, positionOk := first["position"].([]string)
	if !positionOk || len(position) != 3 {
		t.Fatalf("first seed record carries no position bits: %#v", first["position"])
	}
	if position[0] != "3fc00000" || position[1] != "42800000" || position[2] != "c0500000" {
		t.Fatalf("first seed record position bits = %v, want [3fc00000 42800000 c0500000]", position)
	}
	velocity, velocityOk := first["velocity"].([]string)
	if !velocityOk || len(velocity) != 3 {
		t.Fatalf("first seed record carries no velocity bits: %#v", first["velocity"])
	}
	if velocity[0] != "3e800000" {
		t.Fatalf("first seed record velocity X bits = %q, want 3e800000", velocity[0])
	}
}

// TestDomainEventObjectsDropOrderingUsesRawDimensionFirst pins that both drop
// batches order their records by the raw dimension before any later field: a
// batch whose first record carries a raw dimension smaller than the second's
// stays admitted even when every later ordering field is greater, and the
// same records with their dimensions equalized flip to the strict-order
// rejection because the chunk then decides. The classifier agrees with the
// authority on both verdicts.
func TestDomainEventObjectsDropOrderingUsesRawDimensionFirst(t *testing.T) {
	firstID := core.DropID{Dimension: core.DimensionID(-1), Chunk: core.ChunkPos{X: 5, Z: 5}, Slot: 9, Generation: 3}
	secondID := core.DropID{Dimension: core.Overworld, Chunk: core.ChunkPos{X: -9, Z: -9}, Slot: 0, Generation: 1}

	crossDimension := protocol.ItemDropUpserts{
		ServerTick: 7,
		Drops: []protocol.ItemDrop{
			{ID: firstID, BlockIndex: 17, Item: core.ItemStone, Count: 1},
			{ID: secondID, BlockIndex: 18, Item: core.ItemStone, Count: 1},
		},
	}
	if err := protocol.ValidateServerPacket(protocol.StatePlay, crossDimension); err != nil {
		t.Fatalf("cross-dimension upsert batch with the raw dimension first was rejected: %v", err)
	}
	if category, rule := domainEventObjectsDropUpsertsRule(crossDimension); rule != "" {
		t.Fatalf("cross-dimension upsert batch classified as %s/%s, want no broken rule", category, rule)
	}

	sameDimension := crossDimension
	sameDimension.Drops[0].ID.Dimension = core.Overworld
	if err := protocol.ValidateServerPacket(protocol.StatePlay, sameDimension); err == nil {
		t.Fatal("same-dimension upsert batch with the greater chunk first was admitted")
	}
	if category, rule := domainEventObjectsDropUpsertsRule(sameDimension); category != "invalid-value" ||
		rule != domainEventObjectsNameItemDropUpserts+".strictly_increasing_ids" {
		t.Fatalf("same-dimension upsert batch classified as %s/%s, want invalid-value strictly_increasing_ids", category, rule)
	}

	removesCross := protocol.ItemDropRemoves{ServerTick: 7, IDs: []core.DropID{firstID, secondID}}
	if err := protocol.ValidateServerPacket(protocol.StatePlay, removesCross); err != nil {
		t.Fatalf("cross-dimension remove batch with the raw dimension first was rejected: %v", err)
	}
	if category, rule := domainEventObjectsDropRemovesRule(removesCross); rule != "" {
		t.Fatalf("cross-dimension remove batch classified as %s/%s, want no broken rule", category, rule)
	}

	removesSame := protocol.ItemDropRemoves{ServerTick: 7, IDs: []core.DropID{
		{Dimension: core.Overworld, Chunk: firstID.Chunk, Slot: firstID.Slot, Generation: firstID.Generation},
		secondID,
	}}
	if err := protocol.ValidateServerPacket(protocol.StatePlay, removesSame); err == nil {
		t.Fatal("same-dimension remove batch with the greater chunk first was admitted")
	}
	if category, rule := domainEventObjectsDropRemovesRule(removesSame); category != "invalid-value" ||
		rule != domainEventObjectsNameItemDropRemoves+".strictly_increasing_ids" {
		t.Fatalf("same-dimension remove batch classified as %s/%s, want invalid-value strictly_increasing_ids", category, rule)
	}
}

// domainEventObjectsSelection is this producer's exact reviewed registration:
// the 45 committed case specifications with digests proven against the files
// on disk, plus the provenance paths those rules are read from. The shared
// manifest-candidate helper consumes it, so the candidate this node publishes
// is assembled from the same reviewed `CaseSpec` values the committed corpus
// freezes rather than from a second rendering.
func domainEventObjectsSelection(t *testing.T, root string) domainEventSelection {
	t.Helper()
	return domainEventSelection{
		Cases:   domainEventObjectsCorpusCases(t, root),
		Sources: append([]string(nil), domainEventObjectsFamilySources...),
	}
}

// The frozen manifest counts this producer's re-merge must reach now that
// the domain corpus partition is closed: 533 total mornlea_domain cases,
// 321 of them registered under the shared `domain.event` family, and a
// family provenance union of 30 paths. The node-era intermediate totals
// (489 and 277) were correct while the corpus was still growing; the
// closed partition is the final target every producer's merge gate shares.
const (
	domainEventObjectsMergedDomainTotal = 533
	domainEventObjectsMergedFamilyTotal = 321
	domainEventObjectsMergedSourceTotal = 30
)

// TestDomainEventObjectsManifestCandidateRegistersEveryObjectsCase merges
// this producer's selection into the tracked manifest and proves the merged
// candidate reaches the frozen node counts before it is published. A base
// that already registers the objects corpus accepts an idempotent re-merge
// of the same reviewed specs, so the same test gate covers both the first
// publication and every later ordinary run.
func TestDomainEventObjectsManifestCandidateRegistersEveryObjectsCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	selection := domainEventObjectsSelection(t, root)
	merged := mergeDomainEventSelections(t, root, base, selection)

	baseIDs := make(map[string]bool, len(base.Cases))
	for _, c := range base.Cases {
		baseIDs[c.ID] = true
	}
	mergedByID := make(map[string]CaseSpec, len(merged.Cases))
	for _, c := range merged.Cases {
		mergedByID[c.ID] = c
	}
	added := 0
	for _, want := range selection.Cases {
		got, ok := mergedByID[want.ID]
		if !ok {
			t.Fatalf("merged manifest drops objects case %s", want.ID)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("merged manifest rewrites objects case %s", want.ID)
		}
		if !baseIDs[want.ID] {
			added++
		}
	}
	if added != 0 && added != domainEventObjectsCaseTotal {
		t.Fatalf("merged manifest adds %d new objects cases, want 0 (already registered) or %d", added, domainEventObjectsCaseTotal)
	}
	if len(merged.Cases) != len(base.Cases)+added {
		t.Fatalf("merged manifest carries %d cases, want %d", len(merged.Cases), len(base.Cases)+added)
	}

	domainTotal, familyTopLevel := 0, 0
	for _, c := range merged.Cases {
		if c.RustConsumer == domainEventObjectsConsumer {
			domainTotal++
		}
		if c.Family == domainEventObjectsFamily {
			familyTopLevel++
		}
	}
	if domainTotal != domainEventObjectsMergedDomainTotal {
		t.Fatalf("merged manifest registers %d mornlea_domain cases, want %d", domainTotal, domainEventObjectsMergedDomainTotal)
	}
	if familyTopLevel != domainEventObjectsMergedFamilyTotal {
		t.Fatalf("merged manifest registers %d domain.event cases, want %d", familyTopLevel, domainEventObjectsMergedFamilyTotal)
	}
	family, ok := domainEventFamilySpec(merged)
	if !ok {
		t.Fatalf("merged manifest has no %s family", domainEventObjectsFamily)
	}
	if len(family.Cases) != domainEventObjectsMergedFamilyTotal {
		t.Fatalf("%s lists %d cases, want %d", domainEventObjectsFamily, len(family.Cases), domainEventObjectsMergedFamilyTotal)
	}
	if len(family.Sources) != domainEventObjectsMergedSourceTotal {
		t.Fatalf("%s records %d provenance sources, want %d", domainEventObjectsFamily, len(family.Sources), domainEventObjectsMergedSourceTotal)
	}
	basePaths := make(map[string]bool, len(family.Sources))
	baseFamily, baseFamilyOK := domainEventFamilySpec(base)
	if !baseFamilyOK {
		t.Fatalf("tracked manifest has no %s family", domainEventObjectsFamily)
	}
	for _, source := range baseFamily.Sources {
		basePaths[source.Path] = true
	}
	paths := make(map[string]bool, len(family.Sources))
	for _, source := range family.Sources {
		paths[source.Path] = true
	}
	for _, source := range baseFamily.Sources {
		if !paths[source.Path] {
			t.Fatalf("%s provenance drops the previously present path %s", domainEventObjectsFamily, source.Path)
		}
	}
	for _, required := range []string{
		"packages/shared/core/drop.go",
		"packages/shared/network/protocol/message_drop.go",
		"packages/shared/network/protocol/message_projectile.go",
	} {
		if !paths[required] {
			t.Fatalf("%s provenance misses the object path %s", domainEventObjectsFamily, required)
		}
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged manifest changes source_revision from %s to %s", base.SourceRevision, merged.SourceRevision)
	}

	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	candidate := writeDomainEventManifestCandidate(t, root, exportRoot, merged)
	if exportRoot != "" && candidate == "" {
		t.Fatalf("manifest candidate export was rejected for %s", exportRoot)
	}
}

// TestDomainOracle_event_objects is the topic-named entry point the domain
// plan names for this node. It delegates to the same executed table, so the
// two filters select one source of expected results rather than two.
func TestDomainOracle_event_objects(t *testing.T) {
	TestDomainEventObjectsOracleExecutesEveryCase(t)
}

// domainEventObjectsRuleTally collects the accepted and rejected normalized
// outcomes one rule records, so a boundary assertion can look for the field
// value it names.
type domainEventObjectsRuleTally struct {
	accepted []map[string]any
	rejected []map[string]any
}

// domainEventObjectsFieldsContain reports whether one recorded outcome set
// holds a record whose named field carries the value a boundary assertion
// names.
func domainEventObjectsFieldsContain(records []map[string]any, field string, want any) bool {
	for _, fields := range records {
		if reflect.DeepEqual(fields[field], want) {
			return true
		}
	}
	return false
}

// domainEventObjectsNestedContains reports whether one recorded outcome set
// holds a batch whose records carry the named field value. Every object rule
// is a batch, so a boundary that names a record field looks inside the rule's
// array rather than at the record's top level. The array type is the
// producer's own in-memory rendering, because the executed records have not
// been decoded back from JSON.
func domainEventObjectsNestedContains(records []map[string]any, array, field string, want any) bool {
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

// domainEventObjectsIDArrayContainsRawDimension reports whether one recorded
// outcome set holds a batch whose drop identities carry the named raw
// dimension. The upsert rule keeps its identities inside each drop record
// while the remove rule keeps them at the top level, so both shapes are
// inspected.
func domainEventObjectsIDArrayContainsRawDimension(records []map[string]any, dimension int) bool {
	for _, fields := range records {
		if drops, ok := fields["drops"].([]map[string]any); ok {
			for _, drop := range drops {
				if id, idOk := drop["id"].(map[string]any); idOk {
					if value, valueOk := id["dimension"].(int); valueOk && value == dimension {
						return true
					}
				}
			}
		}
		if ids, ok := fields["ids"].([]map[string]any); ok {
			for _, id := range ids {
				if value, valueOk := id["dimension"].(int); valueOk && value == dimension {
					return true
				}
			}
		}
	}
	return false
}

// domainEventObjectsDropsContainStack reports whether one recorded outcome
// set holds an upsert batch whose drops carry the named stack value.
func domainEventObjectsDropsContainStack(records []map[string]any, stack map[string]any) bool {
	for _, fields := range records {
		drops, ok := fields["drops"].([]map[string]any)
		if !ok {
			continue
		}
		for _, drop := range drops {
			if reflect.DeepEqual(drop["stack"], stack) {
				return true
			}
		}
	}
	return false
}

// domainEventObjectsRejectionNamesRule reports whether any recorded rejection
// names the broken rule a boundary assertion pins.
func domainEventObjectsRejectionNamesRule(tally map[string]*domainEventObjectsRuleTally, rule string) bool {
	for _, entry := range tally {
		for _, fields := range entry.rejected {
			if fields["rule"] == rule {
				return true
			}
		}
	}
	return false
}

// domainEventObjectsAssertCaseSpecs runs the production case validation over
// the whole selection, so a case with a bad path, digest, format or
// checkpoint fails here rather than surfacing later as a confusing coverage
// failure.
func domainEventObjectsAssertCaseSpecs(t *testing.T, root string, manifest Inventory) {
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

// domainEventObjectsAssertOutcomesDistinguishCases proves the executed
// evidence separates admitted records from rejections instead of publishing
// one constant answer, and that the rejections name a rule the authority
// publishes.
func domainEventObjectsAssertOutcomesDistinguishCases(t *testing.T, records []domainEventObjectsRecord) {
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
				t.Fatalf("record %s publishes rejection category %q, which is outside the frozen vocabulary", record.label, record.outcome.Category)
			}
			if strings.TrimSpace(record.outcome.Fields["rule"].(string)) == "" {
				t.Fatalf("record %s publishes a rejection that names no rule", record.label)
			}
		default:
			t.Fatalf("record %s publishes outcome kind %q, which is neither ok nor error", record.label, record.outcome.Kind)
		}
	}
	if accepted == 0 || rejected == 0 {
		t.Fatalf("executed evidence records %d accepted and %d rejected cases, want both non-zero", accepted, rejected)
	}
}

// domainEventObjectsFamilySpec indexes the working manifest by family
// identity.
func domainEventObjectsFamilySpec(manifest Inventory) (Family, bool) {
	for _, family := range manifest.Families {
		if family.ID == domainEventObjectsFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainEventObjectsCaseByID indexes a manifest selection by case identity.
func domainEventObjectsCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}
