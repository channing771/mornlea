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

// This file is the Go producer for the domain hostile and passive mob
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
// The six rules this family executes are the Go `protocol.HostileSpawn`,
// `protocol.HostileState`, `protocol.HostileDespawn`, `protocol.PassiveSpawn`,
// `protocol.PassiveState` and `protocol.PassiveDespawn` records, which are the
// mob observations an authoritative session publishes to a subscribed client.
//
// The 64-record batch maxima the Go validators keep are transport budgets the
// domain value does not carry, so no case in this family sits above one; the
// classifier names the bound only so a count a case does name stays
// classifiable. The domain requires a nonempty batch whose IDs are strictly
// increasing, which the `count_range` and `strictly_increasing_ids` rules
// pin. IDs and ticks are decimal strings in the frozen input so their full
// `u64` range stays lossless, the grazing byte is a JSON Boolean in the
// normalized outcome, and a rejected record retains the raw input value of an
// unknown enum instead of clamping it.
//
// The frozen cases this producer materializes are not yet registered in the
// canonical manifest: registration is a later node's work, so the only
// publication path is the explicit external export through
// `RUNTIME_ORACLE_EXPORT_DIR`.

const (
	// domainEventMobsFamily is the corpus family this package executes. The
	// family is the existing `domain.event` row, whose eventual owner is the
	// Rust domain crate that owns the replay observation records.
	domainEventMobsFamily = "domain.event"
	// domainEventMobsVersion is the family's discovered version. A case has
	// to name its family's version, so the case identities carry this segment
	// rather than one this producer chose.
	domainEventMobsVersion = "1"
	// domainEventMobsOperation is the manifest operation name for a mob
	// observation admission case.
	domainEventMobsOperation = "admit"
	// domainEventMobsConsumer is the manifest consumer the change pins for
	// this family: the Rust crate that owns these records.
	domainEventMobsConsumer = "mornlea_domain"
	// domainEventMobsCorpusRelDir is the repository-relative directory
	// holding the frozen event mobs corpus cases.
	domainEventMobsCorpusRelDir = "testdata/runtime-migration/cases/domain/event_mobs"
	// domainEventMobsProducerTestRelPath and the two producer test names
	// locate the package-local producer that executes the protocol DTOs. The
	// topic name is the entry point the domain plan's filter requires; a
	// corpus whose producer test is missing has no independently executed
	// evidence at all.
	domainEventMobsProducerTestRelPath = "packages/tools/cmd/runtime-oracle/domain_event_mobs_test.go"
	domainEventMobsProducerExecuteName = "TestDomainEventMobsOracleExecutesEveryCase"
	domainEventMobsProducerTopicName   = "TestDomainOracle_event_mobs"
	// domainEventMobsCorpusReportName is the published report file name for
	// the executed event mobs evidence.
	domainEventMobsCorpusReportName = "runtime-corpus-domain-event-mobs.json"
	// domainEventMobsProducerID is the exporter's producer identity for this
	// family's frozen assets.
	domainEventMobsProducerID = "runtime-oracle/domain-event-mobs"

	// The six rule names a case names. The family is shared with the world
	// observations, the player and outcome records, the inventory and
	// container publications and the remote-player and companion
	// observations, so the rule name is the discriminator the manifest and
	// the family router both read.
	domainEventMobsRuleHostileSpawn   = "hostile-spawn"
	domainEventMobsRuleHostileState   = "hostile-state"
	domainEventMobsRuleHostileDespawn = "hostile-despawn"
	domainEventMobsRulePassiveSpawn   = "passive-spawn"
	domainEventMobsRulePassiveState   = "passive-state"
	domainEventMobsRulePassiveDespawn = "passive-despawn"

	// The underscored subject names the rejection rules carry as their
	// prefix, one per rule above in the same order.
	domainEventMobsNameHostileSpawn   = "hostile_spawn"
	domainEventMobsNameHostileState   = "hostile_state"
	domainEventMobsNameHostileDespawn = "hostile_despawn"
	domainEventMobsNamePassiveSpawn   = "passive_spawn"
	domainEventMobsNamePassiveState   = "passive_state"
	domainEventMobsNamePassiveDespawn = "passive_despawn"

	// domainEventMobsCaseTotal is the exact size of the executed case table,
	// pinned by the outcomes test so a dropped or duplicated row fails the
	// run rather than shrinking the evidence silently.
	domainEventMobsCaseTotal = 68
)

// domainEventMobsFamilySources is the merged provenance set the family
// records. Every entry is a file the producer's rules are read from, so a
// change to any of them is a change to the recorded evidence. The two
// protocol message files carry the record shapes and the validators, the
// registry file decides which of these records are published in the play
// state, and the two core files own the dimension numbering and the health
// maximum the validators compare against.
var domainEventMobsFamilySources = []string{
	"packages/shared/network/protocol/message_hostile.go",
	"packages/shared/network/protocol/message_passive.go",
	"packages/shared/network/protocol/registry.go",
	"packages/shared/core/block.go",
	"packages/shared/core/health.go",
}

// domainEventMobsLabelPattern is the shape a corpus label must have: a
// lowercase slug with an optional zero-padded numeric suffix, so a boundary
// row sorts in numeric order under a lexical sort.
var domainEventMobsLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(-[0-9]+)?$`)

// domainEventMobsExportPublished keeps the explicit export to one publication
// per test process. The topic entry delegates to the execute-every-case test,
// so one filter selecting both entries executes the same deterministic
// candidate twice, and the exporter's exclusive producer child would
// otherwise treat the second publication as a preexisting directory rather
// than as the same evidence.
var domainEventMobsExportPublished bool

// domainEventMobsInput is the frozen, self-describing corpus input for one
// case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority. The server tick is a decimal string
// and every record ID is a decimal string, because a JSON number cannot carry
// the full `u64` range losslessly and those fields are boundaries this family
// keeps whole. `Position`, `Velocity` and `Yaw` are decimal float texts
// rather than JSON numbers so NaN, the infinities and a signed zero stay
// expressible. The six rules share one envelope whose four batch keys are
// always serialized, an empty batch included, so a replay consumer never
// reads a missing key as an absent batch: hostile rules fill `spawns`,
// `states` and `ids`, passive rules fill `spawns`, `states` and `despawns`,
// and the keys a rule does not read stay explicit empty lists.
type domainEventMobsInput struct {
	Consumer   string  `json:"consumer"`
	Rule       string  `json:"rule"`
	ServerTick *string `json:"server_tick,omitempty"`

	HostileSpawns   []domainEventMobsHostileSpawnRecord
	HostileStates   []domainEventMobsHostileStateRecord
	IDs             []string
	PassiveSpawns   []domainEventMobsPassiveSpawnRecord
	PassiveStates   []domainEventMobsPassiveStateRecord
	PassiveDespawns []domainEventMobsPassiveDespawnRecord
}

// domainEventMobsHostileSpawnRecord is the frozen rendering of one hostile
// spawn record, carrying exactly the fields the Go DTO declares.
type domainEventMobsHostileSpawnRecord struct {
	ID        string   `json:"id"`
	Dimension int32    `json:"dimension"`
	Position  []string `json:"position"`
	Yaw       string   `json:"yaw"`
	Health    uint8    `json:"health"`
	Kind      uint8    `json:"kind"`
}

// domainEventMobsHostileStateRecord is the frozen rendering of one hostile
// state record. It carries no dimension, exactly as the wire record does: a
// dimension change always goes through a despawn and spawn pair.
type domainEventMobsHostileStateRecord struct {
	ID       string   `json:"id"`
	Position []string `json:"position"`
	Velocity []string `json:"velocity"`
	Yaw      string   `json:"yaw"`
	Health   uint8    `json:"health"`
	Kind     uint8    `json:"kind"`
}

// domainEventMobsPassiveSpawnRecord is the frozen rendering of one passive
// spawn record, which carries no kind byte because a passive mob has no
// category to publish.
type domainEventMobsPassiveSpawnRecord struct {
	ID        string   `json:"id"`
	Dimension int32    `json:"dimension"`
	Position  []string `json:"position"`
	Yaw       string   `json:"yaw"`
	Health    uint8    `json:"health"`
}

// domainEventMobsPassiveStateRecord is the frozen rendering of one passive
// state record, whose grazing byte is a raw number here so the illegal value
// is expressible; the normalized outcome renders the admitted values as a
// Boolean.
type domainEventMobsPassiveStateRecord struct {
	ID       string   `json:"id"`
	Position []string `json:"position"`
	Velocity []string `json:"velocity"`
	Yaw      string   `json:"yaw"`
	Health   uint8    `json:"health"`
	Grazing  uint8    `json:"grazing"`
}

// domainEventMobsPassiveDespawnRecord is the frozen rendering of one passive
// despawn record: its identity and its removal reason.
type domainEventMobsPassiveDespawnRecord struct {
	ID     string `json:"id"`
	Reason uint8  `json:"reason"`
}

// domainEventMobsInputWire is the frozen on-disk rendering of one corpus
// input. The two mob families share the `spawns` and `states` keys while
// their record shapes differ, so the typed arrays live on the input and the
// wire renders the pair the rule names; the keys a rule does not read are
// explicit empty lists rather than absent keys.
type domainEventMobsInputWire struct {
	Consumer   string   `json:"consumer"`
	Rule       string   `json:"rule"`
	ServerTick *string  `json:"server_tick,omitempty"`
	Spawns     any      `json:"spawns"`
	States     any      `json:"states"`
	IDs        []string `json:"ids"`
	Despawns   any      `json:"despawns"`
}

// MarshalJSON renders one corpus input under the rule-scoped batch keys. A
// rule outside the six this family owns has no rendering, so a stray rule
// fails the marshal rather than silently publishing an empty case.
func (in domainEventMobsInput) MarshalJSON() ([]byte, error) {
	wire := domainEventMobsInputWire{
		Consumer:   in.Consumer,
		Rule:       in.Rule,
		ServerTick: in.ServerTick,
	}
	switch in.Rule {
	case domainEventMobsRuleHostileSpawn, domainEventMobsRuleHostileState, domainEventMobsRuleHostileDespawn:
		wire.Spawns = in.HostileSpawns
		wire.States = in.HostileStates
		wire.IDs = in.IDs
		wire.Despawns = []domainEventMobsPassiveDespawnRecord{}
	case domainEventMobsRulePassiveSpawn, domainEventMobsRulePassiveState, domainEventMobsRulePassiveDespawn:
		wire.Spawns = in.PassiveSpawns
		wire.States = in.PassiveStates
		wire.IDs = []string{}
		wire.Despawns = in.PassiveDespawns
	default:
		return nil, fmt.Errorf("runtime-oracle: mob event input names rule %q, which has no frozen rendering", in.Rule)
	}
	return json.Marshal(wire)
}

// UnmarshalJSON reads one frozen corpus input back into the typed arrays the
// rule names. An unknown rule keeps the scalar envelope, so the producer's
// own switch reports the unknown rule instead of the JSON decoder, and the
// strict decode wrapper still rejects trailing content after the value.
func (in *domainEventMobsInput) UnmarshalJSON(data []byte) error {
	var envelope struct {
		Consumer   string          `json:"consumer"`
		Rule       string          `json:"rule"`
		ServerTick *string         `json:"server_tick"`
		Spawns     json.RawMessage `json:"spawns"`
		States     json.RawMessage `json:"states"`
		IDs        []string        `json:"ids"`
		Despawns   json.RawMessage `json:"despawns"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&envelope); err != nil {
		return err
	}
	spec := domainEventMobsBaseInput(envelope.Rule)
	spec.Consumer = envelope.Consumer
	spec.ServerTick = envelope.ServerTick
	spec.IDs = envelope.IDs
	if spec.IDs == nil {
		spec.IDs = []string{}
	}
	switch envelope.Rule {
	case domainEventMobsRuleHostileSpawn, domainEventMobsRuleHostileState, domainEventMobsRuleHostileDespawn:
		if err := domainEventMobsDecodeRecords(envelope.Spawns, &spec.HostileSpawns); err != nil {
			return err
		}
		if err := domainEventMobsDecodeRecords(envelope.States, &spec.HostileStates); err != nil {
			return err
		}
	case domainEventMobsRulePassiveSpawn, domainEventMobsRulePassiveState, domainEventMobsRulePassiveDespawn:
		if err := domainEventMobsDecodeRecords(envelope.Spawns, &spec.PassiveSpawns); err != nil {
			return err
		}
		if err := domainEventMobsDecodeRecords(envelope.States, &spec.PassiveStates); err != nil {
			return err
		}
		if err := domainEventMobsDecodeRecords(envelope.Despawns, &spec.PassiveDespawns); err != nil {
			return err
		}
	}
	*in = spec
	return nil
}

// domainEventMobsDecodeRecords decodes one raw batch key into its typed
// array. A key the input does not carry leaves the initialized empty slice
// untouched, so an absent key and an explicit empty list stay distinguishable
// on the typed field.
func domainEventMobsDecodeRecords[T any](raw json.RawMessage, target *[]T) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, target)
}

// domainEventMobsCase is one frozen corpus case: its label and its input, and
// nothing else.
type domainEventMobsCase struct {
	label string
	input domainEventMobsInput
}

// domainEventMobsBaseInput is the envelope every case starts from: the
// consumer, the rule and six explicitly empty batch arrays, so a case that
// names no batch still serializes every key as an empty list rather than
// dropping it.
func domainEventMobsBaseInput(rule string) domainEventMobsInput {
	return domainEventMobsInput{
		Consumer:        domainEventMobsConsumer,
		Rule:            rule,
		HostileSpawns:   []domainEventMobsHostileSpawnRecord{},
		HostileStates:   []domainEventMobsHostileStateRecord{},
		IDs:             []string{},
		PassiveSpawns:   []domainEventMobsPassiveSpawnRecord{},
		PassiveStates:   []domainEventMobsPassiveStateRecord{},
		PassiveDespawns: []domainEventMobsPassiveDespawnRecord{},
	}
}

// domainEventMobsTickText renders one u64 tick as the decimal string the
// frozen input carries.
func domainEventMobsTickText(tick uint64) *string {
	text := strconv.FormatUint(tick, 10)
	return &text
}

// domainEventMobsIDText renders one u64 identity as the decimal string the
// frozen input and the normalized field map both use.
func domainEventMobsIDText(id uint64) string {
	return strconv.FormatUint(id, 10)
}

// domainEventMobsSeedPosition renders the finite position one seed record
// carries: the first record holds `[1.5, 64, -3.25]` and the second holds
// `[2.5, 65, -4.25]`, so the two records of a batch are distinguishable in
// the normalized evidence.
func domainEventMobsSeedPosition(index int) mgl32.Vec3 {
	if index == 0 {
		return mgl32.Vec3{1.5, 64, -3.25}
	}
	return mgl32.Vec3{2.5, 65, -4.25}
}

// domainEventMobsHostileSpawnSeed is the seed hostile spawn batch: tick 7 and
// the two identities 1 and 2 in canonical ascending order, each in the
// overworld with a finite pose, a finite yaw, health 10 and kind 0.
func domainEventMobsHostileSpawnSeed() protocol.HostileSpawn {
	return protocol.HostileSpawn{
		ServerTick: 7,
		Spawns: []protocol.HostileSpawnRecord{
			domainEventMobsHostileSpawnSeedRecord(0),
			domainEventMobsHostileSpawnSeedRecord(1),
		},
	}
}

// domainEventMobsHostileSpawnRecord renders one hostile spawn record from its
// identity, with the seed pose, look, health and kind.
func domainEventMobsHostileSpawnSeedRecord(index int) protocol.HostileSpawnRecord {
	return protocol.HostileSpawnRecord{
		ID:        uint64(index + 1),
		Dimension: core.Overworld,
		Position:  domainEventMobsSeedPosition(index),
		Yaw:       0.5,
		Health:    10,
		Kind:      protocol.HostileKindNightwalker,
	}
}

// domainEventMobsHostileSpawnCase renders one hostile spawn batch row: the
// seed batch with exactly one field changed, so a boundary row is always the
// seed plus the value the rule names.
func domainEventMobsHostileSpawnCase(label string, mutate func(*protocol.HostileSpawn)) domainEventMobsCase {
	spawn := domainEventMobsHostileSpawnSeed()
	if mutate != nil {
		mutate(&spawn)
	}
	input := domainEventMobsBaseInput(domainEventMobsRuleHostileSpawn)
	input.ServerTick = domainEventMobsTickText(spawn.ServerTick)
	records := make([]domainEventMobsHostileSpawnRecord, 0, len(spawn.Spawns))
	for _, record := range spawn.Spawns {
		records = append(records, domainEventMobsHostileSpawnRecordInput(record))
	}
	input.HostileSpawns = records
	return domainEventMobsCase{label: label, input: input}
}

// domainEventMobsHostileSpawnRecordInput renders one Go spawn record as the
// frozen input rendering.
func domainEventMobsHostileSpawnRecordInput(record protocol.HostileSpawnRecord) domainEventMobsHostileSpawnRecord {
	return domainEventMobsHostileSpawnRecord{
		ID:        domainEventMobsIDText(record.ID),
		Dimension: int32(record.Dimension),
		Position:  domainEventMobsVectorText(record.Position),
		Yaw:       domainEventMobsFloatText(record.Yaw),
		Health:    record.Health,
		Kind:      record.Kind,
	}
}

// domainEventMobsHostileStateSeed is the seed hostile state batch: tick 7 and
// the two identities in canonical ascending order, each with a finite pose, a
// finite velocity, a finite yaw, health 10 and kind 0.
func domainEventMobsHostileStateSeed() protocol.HostileState {
	return protocol.HostileState{
		ServerTick: 7,
		States: []protocol.HostileStateRecord{
			domainEventMobsHostileStateSeedRecord(0),
			domainEventMobsHostileStateSeedRecord(1),
		},
	}
}

// domainEventMobsHostileStateRecord renders one hostile state record from its
// identity, with the seed pose, velocity, look, health and kind.
func domainEventMobsHostileStateSeedRecord(index int) protocol.HostileStateRecord {
	return protocol.HostileStateRecord{
		ID:       uint64(index + 1),
		Position: domainEventMobsSeedPosition(index),
		Velocity: mgl32.Vec3{0.25, 0, -0.5},
		Yaw:      0.5,
		Health:   10,
		Kind:     protocol.HostileKindNightwalker,
	}
}

// domainEventMobsHostileStateCase renders one hostile state batch row: the
// seed batch with exactly one field changed.
func domainEventMobsHostileStateCase(label string, mutate func(*protocol.HostileState)) domainEventMobsCase {
	state := domainEventMobsHostileStateSeed()
	if mutate != nil {
		mutate(&state)
	}
	input := domainEventMobsBaseInput(domainEventMobsRuleHostileState)
	input.ServerTick = domainEventMobsTickText(state.ServerTick)
	records := make([]domainEventMobsHostileStateRecord, 0, len(state.States))
	for _, record := range state.States {
		records = append(records, domainEventMobsHostileStateRecordInput(record))
	}
	input.HostileStates = records
	return domainEventMobsCase{label: label, input: input}
}

// domainEventMobsHostileStateRecordInput renders one Go state record as the
// frozen input rendering.
func domainEventMobsHostileStateRecordInput(record protocol.HostileStateRecord) domainEventMobsHostileStateRecord {
	return domainEventMobsHostileStateRecord{
		ID:       domainEventMobsIDText(record.ID),
		Position: domainEventMobsVectorText(record.Position),
		Velocity: domainEventMobsVectorText(record.Velocity),
		Yaw:      domainEventMobsFloatText(record.Yaw),
		Health:   record.Health,
		Kind:     record.Kind,
	}
}

// domainEventMobsHostileDespawnSeed is the seed hostile despawn batch: tick 7
// and the two identities in canonical ascending order.
func domainEventMobsHostileDespawnSeed() protocol.HostileDespawn {
	return protocol.HostileDespawn{ServerTick: 7, IDs: []uint64{1, 2}}
}

// domainEventMobsHostileDespawnCase renders one hostile despawn batch row:
// the seed batch with exactly one field changed.
func domainEventMobsHostileDespawnCase(label string, mutate func(*protocol.HostileDespawn)) domainEventMobsCase {
	despawn := domainEventMobsHostileDespawnSeed()
	if mutate != nil {
		mutate(&despawn)
	}
	input := domainEventMobsBaseInput(domainEventMobsRuleHostileDespawn)
	input.ServerTick = domainEventMobsTickText(despawn.ServerTick)
	ids := make([]string, 0, len(despawn.IDs))
	for _, id := range despawn.IDs {
		ids = append(ids, domainEventMobsIDText(id))
	}
	input.IDs = ids
	return domainEventMobsCase{label: label, input: input}
}

// domainEventMobsPassiveSpawnSeed is the seed passive spawn batch: tick 7 and
// the two identities in canonical ascending order, each in the overworld with
// a finite pose, a finite yaw and health 10.
func domainEventMobsPassiveSpawnSeed() protocol.PassiveSpawn {
	return protocol.PassiveSpawn{
		ServerTick: 7,
		Spawns: []protocol.PassiveSpawnRecord{
			domainEventMobsPassiveSpawnSeedRecord(0),
			domainEventMobsPassiveSpawnSeedRecord(1),
		},
	}
}

// domainEventMobsPassiveSpawnRecord renders one passive spawn record from its
// identity, with the seed pose, look and health.
func domainEventMobsPassiveSpawnSeedRecord(index int) protocol.PassiveSpawnRecord {
	return protocol.PassiveSpawnRecord{
		ID:        uint64(index + 1),
		Dimension: core.Overworld,
		Position:  domainEventMobsSeedPosition(index),
		Yaw:       0.5,
		Health:    10,
	}
}

// domainEventMobsPassiveSpawnCase renders one passive spawn batch row: the
// seed batch with exactly one field changed.
func domainEventMobsPassiveSpawnCase(label string, mutate func(*protocol.PassiveSpawn)) domainEventMobsCase {
	spawn := domainEventMobsPassiveSpawnSeed()
	if mutate != nil {
		mutate(&spawn)
	}
	input := domainEventMobsBaseInput(domainEventMobsRulePassiveSpawn)
	input.ServerTick = domainEventMobsTickText(spawn.ServerTick)
	records := make([]domainEventMobsPassiveSpawnRecord, 0, len(spawn.Spawns))
	for _, record := range spawn.Spawns {
		records = append(records, domainEventMobsPassiveSpawnRecordInput(record))
	}
	input.PassiveSpawns = records
	return domainEventMobsCase{label: label, input: input}
}

// domainEventMobsPassiveSpawnRecordInput renders one Go passive spawn record
// as the frozen input rendering.
func domainEventMobsPassiveSpawnRecordInput(record protocol.PassiveSpawnRecord) domainEventMobsPassiveSpawnRecord {
	return domainEventMobsPassiveSpawnRecord{
		ID:        domainEventMobsIDText(record.ID),
		Dimension: int32(record.Dimension),
		Position:  domainEventMobsVectorText(record.Position),
		Yaw:       domainEventMobsFloatText(record.Yaw),
		Health:    record.Health,
	}
}

// domainEventMobsPassiveStateSeed is the seed passive state batch: tick 7 and
// the two identities in canonical ascending order, the first not grazing and
// the second grazing, so both admitted grazing values are pinned by the seed
// itself.
func domainEventMobsPassiveStateSeed() protocol.PassiveState {
	return protocol.PassiveState{
		ServerTick: 7,
		States: []protocol.PassiveStateRecord{
			domainEventMobsPassiveStateSeedRecord(0, protocol.PassiveDespawnVanished),
			domainEventMobsPassiveStateSeedRecord(1, 1),
		},
	}
}

// domainEventMobsPassiveStateRecord renders one passive state record from its
// identity and grazing byte, with the seed pose, velocity, look and health.
func domainEventMobsPassiveStateSeedRecord(index int, grazing uint8) protocol.PassiveStateRecord {
	return protocol.PassiveStateRecord{
		ID:       uint64(index + 1),
		Position: domainEventMobsSeedPosition(index),
		Velocity: mgl32.Vec3{0.25, 0, -0.5},
		Yaw:      0.5,
		Health:   10,
		Grazing:  grazing,
	}
}

// domainEventMobsPassiveStateCase renders one passive state batch row: the
// seed batch with exactly one field changed.
func domainEventMobsPassiveStateCase(label string, mutate func(*protocol.PassiveState)) domainEventMobsCase {
	state := domainEventMobsPassiveStateSeed()
	if mutate != nil {
		mutate(&state)
	}
	input := domainEventMobsBaseInput(domainEventMobsRulePassiveState)
	input.ServerTick = domainEventMobsTickText(state.ServerTick)
	records := make([]domainEventMobsPassiveStateRecord, 0, len(state.States))
	for _, record := range state.States {
		records = append(records, domainEventMobsPassiveStateRecordInput(record))
	}
	input.PassiveStates = records
	return domainEventMobsCase{label: label, input: input}
}

// domainEventMobsPassiveStateRecordInput renders one Go passive state record
// as the frozen input rendering.
func domainEventMobsPassiveStateRecordInput(record protocol.PassiveStateRecord) domainEventMobsPassiveStateRecord {
	return domainEventMobsPassiveStateRecord{
		ID:       domainEventMobsIDText(record.ID),
		Position: domainEventMobsVectorText(record.Position),
		Velocity: domainEventMobsVectorText(record.Velocity),
		Yaw:      domainEventMobsFloatText(record.Yaw),
		Health:   record.Health,
		Grazing:  record.Grazing,
	}
}

// domainEventMobsPassiveDespawnSeed is the seed passive despawn batch: tick 7
// and the two identities in canonical ascending order, the first vanished and
// the second died, so both published reasons are pinned by the seed itself.
func domainEventMobsPassiveDespawnSeed() protocol.PassiveDespawn {
	return protocol.PassiveDespawn{
		ServerTick: 7,
		Despawns: []protocol.PassiveDespawnRecord{
			{ID: 1, Reason: protocol.PassiveDespawnVanished},
			{ID: 2, Reason: protocol.PassiveDespawnDied},
		},
	}
}

// domainEventMobsPassiveDespawnCase renders one passive despawn batch row:
// the seed batch with exactly one field changed.
func domainEventMobsPassiveDespawnCase(label string, mutate func(*protocol.PassiveDespawn)) domainEventMobsCase {
	despawn := domainEventMobsPassiveDespawnSeed()
	if mutate != nil {
		mutate(&despawn)
	}
	input := domainEventMobsBaseInput(domainEventMobsRulePassiveDespawn)
	input.ServerTick = domainEventMobsTickText(despawn.ServerTick)
	records := make([]domainEventMobsPassiveDespawnRecord, 0, len(despawn.Despawns))
	for _, record := range despawn.Despawns {
		records = append(records, domainEventMobsPassiveDespawnRecordInput(record))
	}
	input.PassiveDespawns = records
	return domainEventMobsCase{label: label, input: input}
}

// domainEventMobsPassiveDespawnRecordInput renders one Go passive despawn
// record as the frozen input rendering.
func domainEventMobsPassiveDespawnRecordInput(record protocol.PassiveDespawnRecord) domainEventMobsPassiveDespawnRecord {
	return domainEventMobsPassiveDespawnRecord{
		ID:     domainEventMobsIDText(record.ID),
		Reason: record.Reason,
	}
}

// domainEventMobsNonFinitePosition is the seed first-record position with its
// X component replaced by NaN, the non-finite pose boundary.
func domainEventMobsNonFinitePosition() mgl32.Vec3 {
	return mgl32.Vec3{float32(math.NaN()), 64, -3.25}
}

// domainEventMobsNonFiniteVelocity is the seed velocity with its Z component
// replaced by positive infinity, the non-finite velocity boundary.
func domainEventMobsNonFiniteVelocity() mgl32.Vec3 {
	return mgl32.Vec3{0.25, 0, float32(math.Inf(1))}
}

// domainEventMobsCases is the ordered case table the producer executes.
//
// Every rule's rows are the seed batch plus one boundary each: the zero tick,
// which the Go validators admit because they publish no tick rule for these
// records; health 1 and health 20, the inclusive ends of the health range;
// health 0 and health 21, one step outside each end; the depths dimension,
// which only the spawn records reject because the overworld alone is
// published; the unknown kind, grazing byte and reason, each raw value 2; the
// zero identity; the non-finite pose, velocity and yaw; the empty batch,
// which stays an explicit empty list rather than an absent key; the reversed
// and duplicate identity orders; and the hostile spawn batch carrying both
// published kinds. No row sits above a wire batch maximum, because those
// counts are transport budgets the domain does not carry.
func domainEventMobsCases() []domainEventMobsCase {
	cases := []domainEventMobsCase{
		domainEventMobsHostileSpawnCase("hostile-spawn-seed", nil),
		domainEventMobsHostileSpawnCase("hostile-spawn-zero-tick", func(spawn *protocol.HostileSpawn) {
			spawn.ServerTick = 0
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-health-one", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns[0].Health = 1
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-health-twenty", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns[0].Health = core.MaxHealth
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-health-zero", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns[0].Health = 0
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-health-twenty-one", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns[0].Health = core.MaxHealth + 1
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-depths-dimension", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns[0].Dimension = core.Depths
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-zero-id", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns[0].ID = 0
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-non-finite-position", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns[0].Position = domainEventMobsNonFinitePosition()
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-non-finite-yaw", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns[0].Yaw = float32(math.Inf(-1))
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-empty", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns = []protocol.HostileSpawnRecord{}
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-reversed", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns[0].ID, spawn.Spawns[1].ID = spawn.Spawns[1].ID, spawn.Spawns[0].ID
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-duplicate", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns[1].ID = spawn.Spawns[0].ID
		}),
		domainEventMobsHostileSpawnCase("hostile-spawn-both-kinds", func(spawn *protocol.HostileSpawn) {
			spawn.Spawns[1].Kind = protocol.HostileKindBoneThrower
		}),

		domainEventMobsHostileStateCase("hostile-state-seed", nil),
		domainEventMobsHostileStateCase("hostile-state-zero-tick", func(state *protocol.HostileState) {
			state.ServerTick = 0
		}),
		domainEventMobsHostileStateCase("hostile-state-health-one", func(state *protocol.HostileState) {
			state.States[0].Health = 1
		}),
		domainEventMobsHostileStateCase("hostile-state-health-twenty", func(state *protocol.HostileState) {
			state.States[0].Health = core.MaxHealth
		}),
		domainEventMobsHostileStateCase("hostile-state-health-zero", func(state *protocol.HostileState) {
			state.States[0].Health = 0
		}),
		domainEventMobsHostileStateCase("hostile-state-health-twenty-one", func(state *protocol.HostileState) {
			state.States[0].Health = core.MaxHealth + 1
		}),
		domainEventMobsHostileStateCase("hostile-state-unknown-kind", func(state *protocol.HostileState) {
			state.States[0].Kind = 2
		}),
		domainEventMobsHostileStateCase("hostile-state-zero-id", func(state *protocol.HostileState) {
			state.States[0].ID = 0
		}),
		domainEventMobsHostileStateCase("hostile-state-non-finite-position", func(state *protocol.HostileState) {
			state.States[0].Position = domainEventMobsNonFinitePosition()
		}),
		domainEventMobsHostileStateCase("hostile-state-non-finite-velocity", func(state *protocol.HostileState) {
			state.States[0].Velocity = domainEventMobsNonFiniteVelocity()
		}),
		domainEventMobsHostileStateCase("hostile-state-non-finite-yaw", func(state *protocol.HostileState) {
			state.States[0].Yaw = float32(math.Inf(-1))
		}),
		domainEventMobsHostileStateCase("hostile-state-empty", func(state *protocol.HostileState) {
			state.States = []protocol.HostileStateRecord{}
		}),
		domainEventMobsHostileStateCase("hostile-state-reversed", func(state *protocol.HostileState) {
			state.States[0].ID, state.States[1].ID = state.States[1].ID, state.States[0].ID
		}),
		domainEventMobsHostileStateCase("hostile-state-duplicate", func(state *protocol.HostileState) {
			state.States[1].ID = state.States[0].ID
		}),

		domainEventMobsHostileDespawnCase("hostile-despawn-seed", nil),
		domainEventMobsHostileDespawnCase("hostile-despawn-zero-tick", func(despawn *protocol.HostileDespawn) {
			despawn.ServerTick = 0
		}),
		domainEventMobsHostileDespawnCase("hostile-despawn-zero-id", func(despawn *protocol.HostileDespawn) {
			despawn.IDs[0] = 0
		}),
		domainEventMobsHostileDespawnCase("hostile-despawn-empty", func(despawn *protocol.HostileDespawn) {
			despawn.IDs = []uint64{}
		}),
		domainEventMobsHostileDespawnCase("hostile-despawn-reversed", func(despawn *protocol.HostileDespawn) {
			despawn.IDs[0], despawn.IDs[1] = despawn.IDs[1], despawn.IDs[0]
		}),
		domainEventMobsHostileDespawnCase("hostile-despawn-duplicate", func(despawn *protocol.HostileDespawn) {
			despawn.IDs[1] = despawn.IDs[0]
		}),

		domainEventMobsPassiveSpawnCase("passive-spawn-seed", nil),
		domainEventMobsPassiveSpawnCase("passive-spawn-zero-tick", func(spawn *protocol.PassiveSpawn) {
			spawn.ServerTick = 0
		}),
		domainEventMobsPassiveSpawnCase("passive-spawn-health-one", func(spawn *protocol.PassiveSpawn) {
			spawn.Spawns[0].Health = 1
		}),
		domainEventMobsPassiveSpawnCase("passive-spawn-health-twenty", func(spawn *protocol.PassiveSpawn) {
			spawn.Spawns[0].Health = core.MaxHealth
		}),
		domainEventMobsPassiveSpawnCase("passive-spawn-health-zero", func(spawn *protocol.PassiveSpawn) {
			spawn.Spawns[0].Health = 0
		}),
		domainEventMobsPassiveSpawnCase("passive-spawn-health-twenty-one", func(spawn *protocol.PassiveSpawn) {
			spawn.Spawns[0].Health = core.MaxHealth + 1
		}),
		domainEventMobsPassiveSpawnCase("passive-spawn-depths-dimension", func(spawn *protocol.PassiveSpawn) {
			spawn.Spawns[0].Dimension = core.Depths
		}),
		domainEventMobsPassiveSpawnCase("passive-spawn-zero-id", func(spawn *protocol.PassiveSpawn) {
			spawn.Spawns[0].ID = 0
		}),
		domainEventMobsPassiveSpawnCase("passive-spawn-non-finite-position", func(spawn *protocol.PassiveSpawn) {
			spawn.Spawns[0].Position = domainEventMobsNonFinitePosition()
		}),
		domainEventMobsPassiveSpawnCase("passive-spawn-non-finite-yaw", func(spawn *protocol.PassiveSpawn) {
			spawn.Spawns[0].Yaw = float32(math.Inf(-1))
		}),
		domainEventMobsPassiveSpawnCase("passive-spawn-empty", func(spawn *protocol.PassiveSpawn) {
			spawn.Spawns = []protocol.PassiveSpawnRecord{}
		}),
		domainEventMobsPassiveSpawnCase("passive-spawn-reversed", func(spawn *protocol.PassiveSpawn) {
			spawn.Spawns[0].ID, spawn.Spawns[1].ID = spawn.Spawns[1].ID, spawn.Spawns[0].ID
		}),
		domainEventMobsPassiveSpawnCase("passive-spawn-duplicate", func(spawn *protocol.PassiveSpawn) {
			spawn.Spawns[1].ID = spawn.Spawns[0].ID
		}),

		domainEventMobsPassiveStateCase("passive-state-seed", nil),
		domainEventMobsPassiveStateCase("passive-state-zero-tick", func(state *protocol.PassiveState) {
			state.ServerTick = 0
		}),
		domainEventMobsPassiveStateCase("passive-state-health-one", func(state *protocol.PassiveState) {
			state.States[0].Health = 1
		}),
		domainEventMobsPassiveStateCase("passive-state-health-twenty", func(state *protocol.PassiveState) {
			state.States[0].Health = core.MaxHealth
		}),
		domainEventMobsPassiveStateCase("passive-state-health-zero", func(state *protocol.PassiveState) {
			state.States[0].Health = 0
		}),
		domainEventMobsPassiveStateCase("passive-state-health-twenty-one", func(state *protocol.PassiveState) {
			state.States[0].Health = core.MaxHealth + 1
		}),
		domainEventMobsPassiveStateCase("passive-state-unknown-grazing", func(state *protocol.PassiveState) {
			state.States[0].Grazing = 2
		}),
		domainEventMobsPassiveStateCase("passive-state-zero-id", func(state *protocol.PassiveState) {
			state.States[0].ID = 0
		}),
		domainEventMobsPassiveStateCase("passive-state-non-finite-position", func(state *protocol.PassiveState) {
			state.States[0].Position = domainEventMobsNonFinitePosition()
		}),
		domainEventMobsPassiveStateCase("passive-state-non-finite-velocity", func(state *protocol.PassiveState) {
			state.States[0].Velocity = domainEventMobsNonFiniteVelocity()
		}),
		domainEventMobsPassiveStateCase("passive-state-non-finite-yaw", func(state *protocol.PassiveState) {
			state.States[0].Yaw = float32(math.Inf(-1))
		}),
		domainEventMobsPassiveStateCase("passive-state-empty", func(state *protocol.PassiveState) {
			state.States = []protocol.PassiveStateRecord{}
		}),
		domainEventMobsPassiveStateCase("passive-state-reversed", func(state *protocol.PassiveState) {
			state.States[0].ID, state.States[1].ID = state.States[1].ID, state.States[0].ID
		}),
		domainEventMobsPassiveStateCase("passive-state-duplicate", func(state *protocol.PassiveState) {
			state.States[1].ID = state.States[0].ID
		}),

		domainEventMobsPassiveDespawnCase("passive-despawn-seed", nil),
		domainEventMobsPassiveDespawnCase("passive-despawn-zero-tick", func(despawn *protocol.PassiveDespawn) {
			despawn.ServerTick = 0
		}),
		domainEventMobsPassiveDespawnCase("passive-despawn-unknown-reason", func(despawn *protocol.PassiveDespawn) {
			despawn.Despawns[0].Reason = 2
		}),
		domainEventMobsPassiveDespawnCase("passive-despawn-zero-id", func(despawn *protocol.PassiveDespawn) {
			despawn.Despawns[0].ID = 0
		}),
		domainEventMobsPassiveDespawnCase("passive-despawn-empty", func(despawn *protocol.PassiveDespawn) {
			despawn.Despawns = []protocol.PassiveDespawnRecord{}
		}),
		domainEventMobsPassiveDespawnCase("passive-despawn-reversed", func(despawn *protocol.PassiveDespawn) {
			despawn.Despawns[0].ID, despawn.Despawns[1].ID = despawn.Despawns[1].ID, despawn.Despawns[0].ID
		}),
		domainEventMobsPassiveDespawnCase("passive-despawn-duplicate", func(despawn *protocol.PassiveDespawn) {
			despawn.Despawns[1].ID = despawn.Despawns[0].ID
		}),
	}
	return cases
}

// runDomainEventMobs executes one corpus case through the current Go protocol
// hostile and passive mob DTOs.
//
// The verdict always comes from `protocol.ValidateServerPacket` in the play
// state, which is the same validator the codec applies on both the encode and
// the decode side. The rule name and the rejection category come from the same
// bounds the DTO reads, and the two are cross-checked against each other, so a
// classification that disagrees with the authority fails the run instead of
// publishing a plausible-looking rejection.
func runDomainEventMobs(c CaseSpec, input []byte) (Outcome, []byte, error) {
	spec, err := domainEventMobsDecodeInput(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	if spec.Consumer != domainEventMobsConsumer {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names consumer %q, want %q", c.ID, spec.Consumer, domainEventMobsConsumer)
	}

	switch spec.Rule {
	case domainEventMobsRuleHostileSpawn:
		return domainEventMobsRunHostileSpawn(c, spec)
	case domainEventMobsRuleHostileState:
		return domainEventMobsRunHostileState(c, spec)
	case domainEventMobsRuleHostileDespawn:
		return domainEventMobsRunHostileDespawn(c, spec)
	case domainEventMobsRulePassiveSpawn:
		return domainEventMobsRunPassiveSpawn(c, spec)
	case domainEventMobsRulePassiveState:
		return domainEventMobsRunPassiveState(c, spec)
	case domainEventMobsRulePassiveDespawn:
		return domainEventMobsRunPassiveDespawn(c, spec)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, spec.Rule)
	}
}

// domainEventMobsRunHostileSpawn admits one hostile spawn batch.
func domainEventMobsRunHostileSpawn(c CaseSpec, spec domainEventMobsInput) (Outcome, []byte, error) {
	spawn, err := domainEventMobsHostileSpawn(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventMobsNormalizeHostileSpawn(spawn)
	category, rule := domainEventMobsHostileSpawnRule(spawn)
	return domainEventMobsFinish(c, domainEventMobsRuleHostileSpawn, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, spawn) == nil)
}

// domainEventMobsRunHostileState admits one hostile state batch.
func domainEventMobsRunHostileState(c CaseSpec, spec domainEventMobsInput) (Outcome, []byte, error) {
	state, err := domainEventMobsHostileState(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventMobsNormalizeHostileState(state)
	category, rule := domainEventMobsHostileStateRule(state)
	return domainEventMobsFinish(c, domainEventMobsRuleHostileState, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, state) == nil)
}

// domainEventMobsRunHostileDespawn admits one hostile despawn batch, whose
// records carry only identities.
func domainEventMobsRunHostileDespawn(c CaseSpec, spec domainEventMobsInput) (Outcome, []byte, error) {
	despawn, err := domainEventMobsHostileDespawn(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventMobsNormalizeHostileDespawn(despawn)
	category, rule := domainEventMobsHostileDespawnRule(despawn)
	return domainEventMobsFinish(c, domainEventMobsRuleHostileDespawn, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, despawn) == nil)
}

// domainEventMobsRunPassiveSpawn admits one passive spawn batch.
func domainEventMobsRunPassiveSpawn(c CaseSpec, spec domainEventMobsInput) (Outcome, []byte, error) {
	spawn, err := domainEventMobsPassiveSpawn(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventMobsNormalizePassiveSpawn(spawn)
	category, rule := domainEventMobsPassiveSpawnRule(spawn)
	return domainEventMobsFinish(c, domainEventMobsRulePassiveSpawn, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, spawn) == nil)
}

// domainEventMobsRunPassiveState admits one passive state batch.
func domainEventMobsRunPassiveState(c CaseSpec, spec domainEventMobsInput) (Outcome, []byte, error) {
	state, err := domainEventMobsPassiveState(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventMobsNormalizePassiveState(state)
	category, rule := domainEventMobsPassiveStateRule(state)
	return domainEventMobsFinish(c, domainEventMobsRulePassiveState, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, state) == nil)
}

// domainEventMobsRunPassiveDespawn admits one passive despawn batch, whose
// records carry identities and removal reasons.
func domainEventMobsRunPassiveDespawn(c CaseSpec, spec domainEventMobsInput) (Outcome, []byte, error) {
	despawn, err := domainEventMobsPassiveDespawn(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventMobsNormalizePassiveDespawn(despawn)
	category, rule := domainEventMobsPassiveDespawnRule(despawn)
	return domainEventMobsFinish(c, domainEventMobsRulePassiveDespawn, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, despawn) == nil)
}

// domainEventMobsTick resolves the decimal-string server tick every rule
// requires. An absent field is a producer error rather than a silent zero,
// because the tick is a meaningful value the record always carries.
func domainEventMobsTick(c CaseSpec, spec domainEventMobsInput) (uint64, error) {
	if spec.ServerTick == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires a server tick", c.ID)
	}
	tick, err := strconv.ParseUint(*spec.ServerTick, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: case %s names server tick %q, which is not a decimal u64", c.ID, *spec.ServerTick)
	}
	return tick, nil
}

// domainEventMobsID resolves one decimal-string identity a record names.
func domainEventMobsID(c CaseSpec, name, text string) (uint64, error) {
	id, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not a decimal u64", c.ID, name, text)
	}
	return id, nil
}

// domainEventMobsHostileSpawn resolves one hostile spawn batch from its
// frozen envelope. The batch is carried, not absent, so an empty batch is an
// explicit empty list rather than a missing key.
func domainEventMobsHostileSpawn(c CaseSpec, spec domainEventMobsInput) (protocol.HostileSpawn, error) {
	tick, err := domainEventMobsTick(c, spec)
	if err != nil {
		return protocol.HostileSpawn{}, err
	}
	if spec.HostileSpawns == nil {
		return protocol.HostileSpawn{}, fmt.Errorf("runtime-oracle: case %s requires a hostile spawn batch", c.ID)
	}
	spawns := make([]protocol.HostileSpawnRecord, 0, len(spec.HostileSpawns))
	for index, record := range spec.HostileSpawns {
		id, err := domainEventMobsID(c, fmt.Sprintf("spawns[%d].id", index), record.ID)
		if err != nil {
			return protocol.HostileSpawn{}, err
		}
		position, err := domainEventMobsVec3(c, fmt.Sprintf("spawns[%d].position", index), record.Position)
		if err != nil {
			return protocol.HostileSpawn{}, err
		}
		yaw, err := domainEventMobsAngle(c, fmt.Sprintf("spawns[%d].yaw", index), record.Yaw)
		if err != nil {
			return protocol.HostileSpawn{}, err
		}
		spawns = append(spawns, protocol.HostileSpawnRecord{
			ID:        id,
			Dimension: core.DimensionID(record.Dimension),
			Position:  position,
			Yaw:       yaw,
			Health:    record.Health,
			Kind:      record.Kind,
		})
	}
	return protocol.HostileSpawn{ServerTick: tick, Spawns: spawns}, nil
}

// domainEventMobsHostileState resolves one hostile state batch from its
// frozen envelope.
func domainEventMobsHostileState(c CaseSpec, spec domainEventMobsInput) (protocol.HostileState, error) {
	tick, err := domainEventMobsTick(c, spec)
	if err != nil {
		return protocol.HostileState{}, err
	}
	if spec.HostileStates == nil {
		return protocol.HostileState{}, fmt.Errorf("runtime-oracle: case %s requires a hostile state batch", c.ID)
	}
	states := make([]protocol.HostileStateRecord, 0, len(spec.HostileStates))
	for index, record := range spec.HostileStates {
		state, err := domainEventMobsResolveHostileStateRecord(c, index, record)
		if err != nil {
			return protocol.HostileState{}, err
		}
		states = append(states, state)
	}
	return protocol.HostileState{ServerTick: tick, States: states}, nil
}

// domainEventMobsHostileStateRecord resolves one hostile state record from
// its frozen rendering.
func domainEventMobsResolveHostileStateRecord(c CaseSpec, index int, record domainEventMobsHostileStateRecord) (protocol.HostileStateRecord, error) {
	id, err := domainEventMobsID(c, fmt.Sprintf("states[%d].id", index), record.ID)
	if err != nil {
		return protocol.HostileStateRecord{}, err
	}
	position, err := domainEventMobsVec3(c, fmt.Sprintf("states[%d].position", index), record.Position)
	if err != nil {
		return protocol.HostileStateRecord{}, err
	}
	velocity, err := domainEventMobsVec3(c, fmt.Sprintf("states[%d].velocity", index), record.Velocity)
	if err != nil {
		return protocol.HostileStateRecord{}, err
	}
	yaw, err := domainEventMobsAngle(c, fmt.Sprintf("states[%d].yaw", index), record.Yaw)
	if err != nil {
		return protocol.HostileStateRecord{}, err
	}
	return protocol.HostileStateRecord{
		ID:       id,
		Position: position,
		Velocity: velocity,
		Yaw:      yaw,
		Health:   record.Health,
		Kind:     record.Kind,
	}, nil
}

// domainEventMobsHostileDespawn resolves one hostile despawn batch from its
// frozen envelope.
func domainEventMobsHostileDespawn(c CaseSpec, spec domainEventMobsInput) (protocol.HostileDespawn, error) {
	tick, err := domainEventMobsTick(c, spec)
	if err != nil {
		return protocol.HostileDespawn{}, err
	}
	if spec.IDs == nil {
		return protocol.HostileDespawn{}, fmt.Errorf("runtime-oracle: case %s requires a hostile despawn batch", c.ID)
	}
	ids := make([]uint64, 0, len(spec.IDs))
	for index, text := range spec.IDs {
		id, err := domainEventMobsID(c, fmt.Sprintf("ids[%d]", index), text)
		if err != nil {
			return protocol.HostileDespawn{}, err
		}
		ids = append(ids, id)
	}
	return protocol.HostileDespawn{ServerTick: tick, IDs: ids}, nil
}

// domainEventMobsPassiveSpawn resolves one passive spawn batch from its
// frozen envelope.
func domainEventMobsPassiveSpawn(c CaseSpec, spec domainEventMobsInput) (protocol.PassiveSpawn, error) {
	tick, err := domainEventMobsTick(c, spec)
	if err != nil {
		return protocol.PassiveSpawn{}, err
	}
	if spec.PassiveSpawns == nil {
		return protocol.PassiveSpawn{}, fmt.Errorf("runtime-oracle: case %s requires a passive spawn batch", c.ID)
	}
	spawns := make([]protocol.PassiveSpawnRecord, 0, len(spec.PassiveSpawns))
	for index, record := range spec.PassiveSpawns {
		id, err := domainEventMobsID(c, fmt.Sprintf("spawns[%d].id", index), record.ID)
		if err != nil {
			return protocol.PassiveSpawn{}, err
		}
		position, err := domainEventMobsVec3(c, fmt.Sprintf("spawns[%d].position", index), record.Position)
		if err != nil {
			return protocol.PassiveSpawn{}, err
		}
		yaw, err := domainEventMobsAngle(c, fmt.Sprintf("spawns[%d].yaw", index), record.Yaw)
		if err != nil {
			return protocol.PassiveSpawn{}, err
		}
		spawns = append(spawns, protocol.PassiveSpawnRecord{
			ID:        id,
			Dimension: core.DimensionID(record.Dimension),
			Position:  position,
			Yaw:       yaw,
			Health:    record.Health,
		})
	}
	return protocol.PassiveSpawn{ServerTick: tick, Spawns: spawns}, nil
}

// domainEventMobsPassiveState resolves one passive state batch from its
// frozen envelope.
func domainEventMobsPassiveState(c CaseSpec, spec domainEventMobsInput) (protocol.PassiveState, error) {
	tick, err := domainEventMobsTick(c, spec)
	if err != nil {
		return protocol.PassiveState{}, err
	}
	if spec.PassiveStates == nil {
		return protocol.PassiveState{}, fmt.Errorf("runtime-oracle: case %s requires a passive state batch", c.ID)
	}
	states := make([]protocol.PassiveStateRecord, 0, len(spec.PassiveStates))
	for index, record := range spec.PassiveStates {
		id, err := domainEventMobsID(c, fmt.Sprintf("states[%d].id", index), record.ID)
		if err != nil {
			return protocol.PassiveState{}, err
		}
		position, err := domainEventMobsVec3(c, fmt.Sprintf("states[%d].position", index), record.Position)
		if err != nil {
			return protocol.PassiveState{}, err
		}
		velocity, err := domainEventMobsVec3(c, fmt.Sprintf("states[%d].velocity", index), record.Velocity)
		if err != nil {
			return protocol.PassiveState{}, err
		}
		yaw, err := domainEventMobsAngle(c, fmt.Sprintf("states[%d].yaw", index), record.Yaw)
		if err != nil {
			return protocol.PassiveState{}, err
		}
		states = append(states, protocol.PassiveStateRecord{
			ID:       id,
			Position: position,
			Velocity: velocity,
			Yaw:      yaw,
			Health:   record.Health,
			Grazing:  record.Grazing,
		})
	}
	return protocol.PassiveState{ServerTick: tick, States: states}, nil
}

// domainEventMobsPassiveDespawn resolves one passive despawn batch from its
// frozen envelope.
func domainEventMobsPassiveDespawn(c CaseSpec, spec domainEventMobsInput) (protocol.PassiveDespawn, error) {
	tick, err := domainEventMobsTick(c, spec)
	if err != nil {
		return protocol.PassiveDespawn{}, err
	}
	if spec.PassiveDespawns == nil {
		return protocol.PassiveDespawn{}, fmt.Errorf("runtime-oracle: case %s requires a passive despawn batch", c.ID)
	}
	despawns := make([]protocol.PassiveDespawnRecord, 0, len(spec.PassiveDespawns))
	for index, record := range spec.PassiveDespawns {
		id, err := domainEventMobsID(c, fmt.Sprintf("despawns[%d].id", index), record.ID)
		if err != nil {
			return protocol.PassiveDespawn{}, err
		}
		despawns = append(despawns, protocol.PassiveDespawnRecord{ID: id, Reason: record.Reason})
	}
	return protocol.PassiveDespawn{ServerTick: tick, Despawns: despawns}, nil
}

// domainEventMobsFinish records one admission.
//
// The classification is cross-checked against the authority's verdict, a
// rejection publishes the broken rule beside its category, and an admitted
// record publishes its normalized field map. No codec round trip is recorded:
// this family pins the semantic field values, and the wire layout of these
// packets belongs to the protocol nodes that port the codecs.
func domainEventMobsFinish(c CaseSpec, subject string, fields map[string]any, category, rule string, admitted bool) (Outcome, []byte, error) {
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

// domainEventMobsNormalizeHostileSpawn renders one hostile spawn batch in the
// normalized field map. The tick and the identities are decimal strings, the
// pose and the yaw are hexadecimal IEEE-754 bit strings, and every record is
// published in the submitted order.
func domainEventMobsNormalizeHostileSpawn(spawn protocol.HostileSpawn) map[string]any {
	records := make([]map[string]any, 0, len(spawn.Spawns))
	for _, record := range spawn.Spawns {
		records = append(records, map[string]any{
			"id":        domainEventMobsIDText(record.ID),
			"dimension": int(record.Dimension),
			"position":  domainEventMobsVectorBits(record.Position),
			"yaw":       domainEventMobsFloatBits(record.Yaw),
			"health":    int(record.Health),
			"kind":      int(record.Kind),
		})
	}
	return map[string]any{
		"server_tick": domainEventMobsIDText(spawn.ServerTick),
		"spawns":      records,
	}
}

// domainEventMobsNormalizeHostileState renders one hostile state batch in the
// normalized field map. No dimension is published, exactly as the wire record
// carries none.
func domainEventMobsNormalizeHostileState(state protocol.HostileState) map[string]any {
	records := make([]map[string]any, 0, len(state.States))
	for _, record := range state.States {
		records = append(records, map[string]any{
			"id":       domainEventMobsIDText(record.ID),
			"position": domainEventMobsVectorBits(record.Position),
			"velocity": domainEventMobsVectorBits(record.Velocity),
			"yaw":      domainEventMobsFloatBits(record.Yaw),
			"health":   int(record.Health),
			"kind":     int(record.Kind),
		})
	}
	return map[string]any{
		"server_tick": domainEventMobsIDText(state.ServerTick),
		"states":      records,
	}
}

// domainEventMobsNormalizeHostileDespawn renders one hostile despawn batch in
// the normalized field map, whose records are bare identities.
func domainEventMobsNormalizeHostileDespawn(despawn protocol.HostileDespawn) map[string]any {
	ids := make([]string, 0, len(despawn.IDs))
	for _, id := range despawn.IDs {
		ids = append(ids, domainEventMobsIDText(id))
	}
	return map[string]any{
		"server_tick": domainEventMobsIDText(despawn.ServerTick),
		"ids":         ids,
	}
}

// domainEventMobsNormalizePassiveSpawn renders one passive spawn batch in the
// normalized field map.
func domainEventMobsNormalizePassiveSpawn(spawn protocol.PassiveSpawn) map[string]any {
	records := make([]map[string]any, 0, len(spawn.Spawns))
	for _, record := range spawn.Spawns {
		records = append(records, map[string]any{
			"id":        domainEventMobsIDText(record.ID),
			"dimension": int(record.Dimension),
			"position":  domainEventMobsVectorBits(record.Position),
			"yaw":       domainEventMobsFloatBits(record.Yaw),
			"health":    int(record.Health),
		})
	}
	return map[string]any{
		"server_tick": domainEventMobsIDText(spawn.ServerTick),
		"spawns":      records,
	}
}

// domainEventMobsNormalizePassiveState renders one passive state batch in the
// normalized field map. The grazing byte is a Boolean for the two admitted
// values, and an unknown value retains its raw number so a rejection shows
// the exact byte the record carried.
func domainEventMobsNormalizePassiveState(state protocol.PassiveState) map[string]any {
	records := make([]map[string]any, 0, len(state.States))
	for _, record := range state.States {
		records = append(records, map[string]any{
			"id":       domainEventMobsIDText(record.ID),
			"position": domainEventMobsVectorBits(record.Position),
			"velocity": domainEventMobsVectorBits(record.Velocity),
			"yaw":      domainEventMobsFloatBits(record.Yaw),
			"health":   int(record.Health),
			"grazing":  domainEventMobsGrazingValue(record.Grazing),
		})
	}
	return map[string]any{
		"server_tick": domainEventMobsIDText(state.ServerTick),
		"states":      records,
	}
}

// domainEventMobsNormalizePassiveDespawn renders one passive despawn batch in
// the normalized field map, whose records are identities and removal reasons.
func domainEventMobsNormalizePassiveDespawn(despawn protocol.PassiveDespawn) map[string]any {
	records := make([]map[string]any, 0, len(despawn.Despawns))
	for _, record := range despawn.Despawns {
		records = append(records, map[string]any{
			"id":     domainEventMobsIDText(record.ID),
			"reason": int(record.Reason),
		})
	}
	return map[string]any{
		"server_tick": domainEventMobsIDText(despawn.ServerTick),
		"despawns":    records,
	}
}

// domainEventMobsGrazingValue renders the grazing byte as the Boolean the
// normalized vocabulary uses for the two admitted values, keeping any other
// raw number so an unknown enum stays visible in a rejection.
func domainEventMobsGrazingValue(grazing uint8) any {
	switch grazing {
	case 0:
		return false
	case 1:
		return true
	default:
		return int(grazing)
	}
}

// domainEventMobsHostileSpawnRule names the first rule one hostile spawn
// batch breaks, in the same order the Go validator checks them: the batch
// count bound, then every record in batch order, then the strict identity
// order.
//
// A rejection classifies as `invalid-identity` for a zero entity ID, as
// `invalid-enum` for an unknown dimension or kind, and as `invalid-value`
// otherwise, which is the classification rule a Rust consumer of this family
// applies to its own rejection variants. The count bound names both ends
// because the upper end is a wire budget no case sits above.
func domainEventMobsHostileSpawnRule(spawn protocol.HostileSpawn) (string, string) {
	if len(spawn.Spawns) < 1 || len(spawn.Spawns) > protocol.MaxHostileRecords {
		return "invalid-value", domainEventMobsNameHostileSpawn + ".count_range"
	}
	for index, record := range spawn.Spawns {
		if category, field := domainEventMobsHostileSpawnRecordRule(record); field != "" {
			return category, fmt.Sprintf("%s.record_%d.%s", domainEventMobsNameHostileSpawn, index, field)
		}
		if index > 0 && spawn.Spawns[index-1].ID >= record.ID {
			return "invalid-value", domainEventMobsNameHostileSpawn + ".strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventMobsHostileSpawnRecordRule names the first rule one hostile
// spawn record breaks: the identity, the overworld-only dimension, the finite
// pose and yaw, the health range and the kind domain.
func domainEventMobsHostileSpawnRecordRule(record protocol.HostileSpawnRecord) (string, string) {
	if record.ID == 0 {
		return "invalid-identity", "id"
	}
	if record.Dimension != core.Overworld {
		return "invalid-enum", "dimension"
	}
	if !domainEventMobsFiniteVec3(record.Position) {
		return "invalid-value", "position"
	}
	if !domainEventMobsFinite32(record.Yaw) {
		return "invalid-value", "yaw"
	}
	if record.Health == 0 || record.Health > core.MaxHealth {
		return "invalid-value", "health"
	}
	if record.Kind != protocol.HostileKindNightwalker && record.Kind != protocol.HostileKindBoneThrower {
		return "invalid-enum", "kind"
	}
	return "", ""
}

// domainEventMobsHostileStateRule names the first rule one hostile state
// batch breaks, in the same order the Go validator checks them.
func domainEventMobsHostileStateRule(state protocol.HostileState) (string, string) {
	if len(state.States) < 1 || len(state.States) > protocol.MaxHostileRecords {
		return "invalid-value", domainEventMobsNameHostileState + ".count_range"
	}
	for index, record := range state.States {
		if category, field := domainEventMobsHostileStateRecordRule(record); field != "" {
			return category, fmt.Sprintf("%s.record_%d.%s", domainEventMobsNameHostileState, index, field)
		}
		if index > 0 && state.States[index-1].ID >= record.ID {
			return "invalid-value", domainEventMobsNameHostileState + ".strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventMobsHostileStateRecordRule names the first rule one hostile
// state record breaks: the identity, the finite pose, velocity and yaw, the
// health range and the kind domain. No dimension is named, exactly as the
// wire record carries none.
func domainEventMobsHostileStateRecordRule(record protocol.HostileStateRecord) (string, string) {
	if record.ID == 0 {
		return "invalid-identity", "id"
	}
	if !domainEventMobsFiniteVec3(record.Position) {
		return "invalid-value", "position"
	}
	if !domainEventMobsFiniteVec3(record.Velocity) {
		return "invalid-value", "velocity"
	}
	if !domainEventMobsFinite32(record.Yaw) {
		return "invalid-value", "yaw"
	}
	if record.Health == 0 || record.Health > core.MaxHealth {
		return "invalid-value", "health"
	}
	if record.Kind != protocol.HostileKindNightwalker && record.Kind != protocol.HostileKindBoneThrower {
		return "invalid-enum", "kind"
	}
	return "", ""
}

// domainEventMobsHostileDespawnRule names the first rule one hostile despawn
// batch breaks: the batch count bound, then the nonzero identities and the
// strict identity order.
func domainEventMobsHostileDespawnRule(despawn protocol.HostileDespawn) (string, string) {
	if len(despawn.IDs) < 1 || len(despawn.IDs) > protocol.MaxHostileRecords {
		return "invalid-value", domainEventMobsNameHostileDespawn + ".count_range"
	}
	for index, id := range despawn.IDs {
		if id == 0 {
			return "invalid-identity", fmt.Sprintf("%s.record_%d.id", domainEventMobsNameHostileDespawn, index)
		}
		if index > 0 && despawn.IDs[index-1] >= id {
			return "invalid-value", domainEventMobsNameHostileDespawn + ".strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventMobsPassiveSpawnRule names the first rule one passive spawn
// batch breaks, in the same order the Go validator checks them.
func domainEventMobsPassiveSpawnRule(spawn protocol.PassiveSpawn) (string, string) {
	if len(spawn.Spawns) < 1 || len(spawn.Spawns) > protocol.MaxPassiveRecords {
		return "invalid-value", domainEventMobsNamePassiveSpawn + ".count_range"
	}
	for index, record := range spawn.Spawns {
		if category, field := domainEventMobsPassiveSpawnRecordRule(record); field != "" {
			return category, fmt.Sprintf("%s.record_%d.%s", domainEventMobsNamePassiveSpawn, index, field)
		}
		if index > 0 && spawn.Spawns[index-1].ID >= record.ID {
			return "invalid-value", domainEventMobsNamePassiveSpawn + ".strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventMobsPassiveSpawnRecordRule names the first rule one passive
// spawn record breaks: the identity, the overworld-only dimension, the finite
// pose and yaw, and the health range. No kind is named, because a passive mob
// has no category to publish.
func domainEventMobsPassiveSpawnRecordRule(record protocol.PassiveSpawnRecord) (string, string) {
	if record.ID == 0 {
		return "invalid-identity", "id"
	}
	if record.Dimension != core.Overworld {
		return "invalid-enum", "dimension"
	}
	if !domainEventMobsFiniteVec3(record.Position) {
		return "invalid-value", "position"
	}
	if !domainEventMobsFinite32(record.Yaw) {
		return "invalid-value", "yaw"
	}
	if record.Health == 0 || record.Health > core.MaxHealth {
		return "invalid-value", "health"
	}
	return "", ""
}

// domainEventMobsPassiveStateRule names the first rule one passive state
// batch breaks, in the same order the Go validator checks them.
func domainEventMobsPassiveStateRule(state protocol.PassiveState) (string, string) {
	if len(state.States) < 1 || len(state.States) > protocol.MaxPassiveRecords {
		return "invalid-value", domainEventMobsNamePassiveState + ".count_range"
	}
	for index, record := range state.States {
		if category, field := domainEventMobsPassiveStateRecordRule(record); field != "" {
			return category, fmt.Sprintf("%s.record_%d.%s", domainEventMobsNamePassiveState, index, field)
		}
		if index > 0 && state.States[index-1].ID >= record.ID {
			return "invalid-value", domainEventMobsNamePassiveState + ".strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventMobsPassiveStateRecordRule names the first rule one passive
// state record breaks: the identity, the finite pose, velocity and yaw, the
// health range, and the two-value grazing domain.
func domainEventMobsPassiveStateRecordRule(record protocol.PassiveStateRecord) (string, string) {
	if record.ID == 0 {
		return "invalid-identity", "id"
	}
	if !domainEventMobsFiniteVec3(record.Position) {
		return "invalid-value", "position"
	}
	if !domainEventMobsFiniteVec3(record.Velocity) {
		return "invalid-value", "velocity"
	}
	if !domainEventMobsFinite32(record.Yaw) {
		return "invalid-value", "yaw"
	}
	if record.Health == 0 || record.Health > core.MaxHealth {
		return "invalid-value", "health"
	}
	if record.Grazing > 1 {
		return "invalid-enum", "grazing"
	}
	return "", ""
}

// domainEventMobsPassiveDespawnRule names the first rule one passive despawn
// batch breaks: the batch count bound, then each record's nonzero identity
// and published reason, then the strict identity order.
func domainEventMobsPassiveDespawnRule(despawn protocol.PassiveDespawn) (string, string) {
	if len(despawn.Despawns) < 1 || len(despawn.Despawns) > protocol.MaxPassiveRecords {
		return "invalid-value", domainEventMobsNamePassiveDespawn + ".count_range"
	}
	for index, record := range despawn.Despawns {
		if record.ID == 0 {
			return "invalid-identity", fmt.Sprintf("%s.record_%d.id", domainEventMobsNamePassiveDespawn, index)
		}
		if record.Reason != protocol.PassiveDespawnVanished && record.Reason != protocol.PassiveDespawnDied {
			return "invalid-enum", fmt.Sprintf("%s.record_%d.reason", domainEventMobsNamePassiveDespawn, index)
		}
		if index > 0 && despawn.Despawns[index-1].ID >= record.ID {
			return "invalid-value", domainEventMobsNamePassiveDespawn + ".strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventMobsFinite32 reports whether one float is neither NaN nor an
// infinity, which is the Go protocol package's private `finite32` predicate.
//
// The producer keeps this copy because the predicate is unexported, and the
// copy is pinned by the same non-finite boundary cases the domain test pins.
func domainEventMobsFinite32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

// domainEventMobsFiniteVec3 reports whether every component of one vector is
// finite, which is the Go protocol package's private `finiteVec3` predicate.
func domainEventMobsFiniteVec3(value mgl32.Vec3) bool {
	for _, component := range value {
		if !domainEventMobsFinite32(component) {
			return false
		}
	}
	return true
}

// domainEventMobsVectorBits renders one f32 vector as the three hexadecimal
// IEEE-754 bit strings the normalized corpus vocabulary uses. A non-finite
// component renders its exact bits, so a rejection shows the exact value the
// record carried rather than a rounded decimal.
func domainEventMobsVectorBits(vector mgl32.Vec3) []string {
	return []string{
		domainEventMobsFloatBits(vector.X()),
		domainEventMobsFloatBits(vector.Y()),
		domainEventMobsFloatBits(vector.Z()),
	}
}

// domainEventMobsFloatBits renders one float32 as the eight-digit hexadecimal
// IEEE-754 bit string the normalized corpus vocabulary uses.
func domainEventMobsFloatBits(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

// domainEventMobsFloatText renders one float32 as decimal float text, so NaN,
// the infinities and a signed zero stay expressible in the frozen input. The
// normalized outcome renders the same value as its IEEE-754 bit string, so
// the input and the outcome pin the value in two independent renderings.
func domainEventMobsFloatText(value float32) string {
	return strconv.FormatFloat(float64(value), 'g', -1, 32)
}

// domainEventMobsVectorText renders one vector as its three decimal float
// texts.
func domainEventMobsVectorText(vector mgl32.Vec3) []string {
	return []string{
		domainEventMobsFloatText(vector.X()),
		domainEventMobsFloatText(vector.Y()),
		domainEventMobsFloatText(vector.Z()),
	}
}

// domainEventMobsAngle resolves one angle field a case names as decimal float
// text.
func domainEventMobsAngle(c CaseSpec, name, text string) (float32, error) {
	value, err := strconv.ParseFloat(text, 32)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not a float32", c.ID, name, text)
	}
	return float32(value), nil
}

// domainEventMobsVec3 resolves one position or velocity from its three
// decimal float texts.
func domainEventMobsVec3(c CaseSpec, name string, components []string) (mgl32.Vec3, error) {
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

// domainEventMobsDecodeInput reads one frozen corpus input and rejects
// trailing content, so a producer never executes bytes the case did not name.
func domainEventMobsDecodeInput(data []byte) (domainEventMobsInput, error) {
	var spec domainEventMobsInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return domainEventMobsInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainEventMobsInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}

// domainEventMobsRecord pairs one executed case with the input and outcome
// the producer produced for it.
type domainEventMobsRecord struct {
	label   string
	input   domainEventMobsInput
	outcome Outcome
}

// domainEventMobsExecute runs the whole case table through the producer and
// returns one record per case in table order.
func domainEventMobsExecute(t *testing.T) []domainEventMobsRecord {
	t.Helper()

	cases := domainEventMobsCases()
	records := make([]domainEventMobsRecord, 0, len(cases))
	for _, entry := range cases {
		input, err := json.MarshalIndent(entry.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", entry.label, err)
		}
		input = append(input, '\n')
		outcome, _, err := runDomainEventMobs(CaseSpec{ID: domainEventMobsCaseID(entry.label)}, input)
		if err != nil {
			t.Fatalf("execute case %s: %v", entry.label, err)
		}
		records = append(records, domainEventMobsRecord{label: entry.label, input: entry.input, outcome: outcome})
	}
	return records
}

// `domainEventMobsSyncCorpus` compares committed assets and optionally
// exports a complete producer candidate for controller review.
//
// Ordinary runs compare committed assets read-only against current producer
// output. The explicit export publishes the complete candidate to a fresh
// external directory before the committed bytes are compared, so an initial
// export run creates every candidate file even while the tracked directory is
// still absent, and never mutates tracked assets.
func domainEventMobsSyncCorpus(t *testing.T, records []domainEventMobsRecord) {
	t.Helper()

	root := mustRepoRoot(t)
	corpusDir := filepath.Join(root, filepath.FromSlash(domainEventMobsCorpusRelDir))
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
	domainEventMobsExport(t, root, assets)

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

// domainEventMobsExport publishes the complete candidate once per test
// process when the export directory is named. The topic entry delegates to
// the execute-every-case test, so one filter selecting both entries executes
// the same deterministic candidate twice, and the exporter's exclusive
// producer child must not treat the second publication as a conflict.
func domainEventMobsExport(t *testing.T, root string, assets []generatedAsset) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv)) == "" {
		return
	}
	if domainEventMobsExportPublished {
		return
	}
	domainEventMobsExportPublished = true
	exportGeneratedAssetsFromEnvironment(t, root, domainEventMobsProducerID, assets)
}

// domainEventMobsCaseID renders the manifest case identity one corpus label
// carries. The version segment is the family's published version, because a
// case has to name the version of the family it belongs to.
func domainEventMobsCaseID(label string) string {
	return domainEventMobsFamily + "/" + domainEventMobsVersion + "/" + label
}

// `domainEventMobsWorkingManifest` clones the merged committed manifest into
// a producer-scoped selection stored in harness-owned temporary storage.
// `Cases` is narrowed to the hostile and passive mob cases and unrelated
// family case lists are cleared. The existing `domain.event` identity is
// retained while its provenance and case list are replaced with this
// producer's current selection, because the canonical manifest does not yet
// register these cases; that registration is a later node's work.
func domainEventMobsWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	cases := domainEventMobsCorpusCases(t, root)

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
	sources := make([]SourceSpec, 0, len(domainEventMobsFamilySources))
	for _, relative := range domainEventMobsFamilySources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	registered := false
	for index := range cloned.Families {
		if cloned.Families[index].ID != domainEventMobsFamily {
			cloned.Families[index].Cases = nil
			continue
		}
		cloned.Families[index].Sources = sources
		cloned.Families[index].Cases = caseIDs
		registered = true
	}
	if !registered {
		t.Fatalf("working manifest has no %s family", domainEventMobsFamily)
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

// domainEventMobsCorpusCases reads the frozen corpus and registers one case
// per committed input, with digests proven against the files on disk.
func domainEventMobsCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(domainEventMobsCorpusRelDir))
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
		t.Fatalf("walk event mobs corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no event mobs case under %s", domainEventMobsCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		label := strings.TrimSuffix(filepath.ToSlash(relative), ".input.json")
		if label == "" || !domainEventMobsLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug with an optional numeric suffix", filepath.ToSlash(relative), label)
		}
		envelope := domainEventMobsReadEnvelope(t, input)
		if envelope.Consumer != domainEventMobsConsumer {
			t.Fatalf("corpus case %s names consumer %q, want %q", label, envelope.Consumer, domainEventMobsConsumer)
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
			ID:           domainEventMobsCaseID(label),
			Family:       domainEventMobsFamily,
			Version:      domainEventMobsVersion,
			Operation:    domainEventMobsOperation,
			Input:        AssetRef{Path: domainEventMobsCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: domainEventMobsCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: domainEventMobsConsumer,
		})
	}
	return cases
}

// domainEventMobsReadEnvelope reads the provenance envelope of one frozen
// corpus input.
func domainEventMobsReadEnvelope(t *testing.T, path string) domainEventMobsInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope domainEventMobsInput
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// domainEventMobsCorpusPath renders one absolute corpus path as the
// repository-relative slash path the manifest requires.
func domainEventMobsCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}

// TestDomainEventMobsOracleExecutesEveryCase runs the whole case table
// through the real Go protocol DTOs, proves the frozen corpus still matches
// what they produce, and then runs the same cases through the production
// runner so the executed evidence satisfies the completeness rules a
// published trace report does.
func TestDomainEventMobsOracleExecutesEveryCase(t *testing.T) {
	records := domainEventMobsExecute(t)
	domainEventMobsSyncCorpus(t, records)

	root := mustRepoRoot(t)
	manifest := domainEventMobsWorkingManifest(t, root)
	if len(manifest.Cases) != len(records) {
		t.Fatalf("working manifest registers %d cases, want %d (one per executed table row)", len(manifest.Cases), len(records))
	}
	domainEventMobsAssertCaseSpecs(t, root, manifest)
	domainEventMobsAssertOutcomesDistinguishCases(t, records)

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for _, obs := range observations {
		c := domainEventMobsCaseByID(t, manifest, obs.CaseID)
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
		c := domainEventMobsCaseByID(t, manifest, obs.CaseID)
		if obs.ExpectedDigest != c.Expected.SHA256 {
			t.Fatalf("observation for %s carries expected digest %s, want %s", obs.CaseID, obs.ExpectedDigest, c.Expected.SHA256)
		}
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event mobs evidence failed trace validation: %v", err)
	}
}

// TestDomainEventMobsOracleCaseIdentitiesAreTheRustDomainConsumer pins the
// manifest identity of every event mobs case: the operation, the family
// version, the family and the consumer the change names for the Rust crate
// that owns these records.
func TestDomainEventMobsOracleCaseIdentitiesAreTheRustDomainConsumer(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventMobsWorkingManifest(t, root)

	labels := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != domainEventMobsOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, domainEventMobsOperation)
		}
		if c.Version != domainEventMobsVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, domainEventMobsVersion)
		}
		if c.Family != domainEventMobsFamily {
			t.Fatalf("case %s declares family %q, want %q", c.ID, c.Family, domainEventMobsFamily)
		}
		if c.RustConsumer != domainEventMobsConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, domainEventMobsConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		prefix := c.Family + "/" + c.Version + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !domainEventMobsLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug with an optional numeric suffix", c.ID, label)
		}
		if labels[label] {
			t.Fatalf("label %q is registered twice", label)
		}
		labels[label] = true

		envelope := domainEventMobsReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != domainEventMobsConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, domainEventMobsConsumer)
		}
		switch envelope.Rule {
		case domainEventMobsRuleHostileSpawn, domainEventMobsRuleHostileState,
			domainEventMobsRuleHostileDespawn, domainEventMobsRulePassiveSpawn,
			domainEventMobsRulePassiveState, domainEventMobsRulePassiveDespawn:
		default:
			t.Fatalf("case %s names rule %q, which is not a hostile or passive mob observation rule", c.ID, envelope.Rule)
		}
	}
}

// TestDomainEventMobsOracleWorkingManifestDescribesItself proves the working
// manifest is internally consistent: every case passes the production case
// validation, the family's provenance hashes match disk, and the family's
// case list matches exactly the cases the selection registers.
func TestDomainEventMobsOracleWorkingManifestDescribesItself(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventMobsWorkingManifest(t, root)
	domainEventMobsAssertCaseSpecs(t, root, manifest)

	family, ok := domainEventMobsFamilySpec(manifest)
	if !ok {
		t.Fatalf("working manifest has no %s family", domainEventMobsFamily)
	}
	if family.Role != "event" || family.Kind != "domain" {
		t.Fatalf("%s declares kind %q role %q, want domain event", domainEventMobsFamily, family.Kind, family.Role)
	}
	if family.CurrentVersion != domainEventMobsVersion {
		t.Fatalf("%s declares version %q, want %q", domainEventMobsFamily, family.CurrentVersion, domainEventMobsVersion)
	}
	if len(family.Sources) != len(domainEventMobsFamilySources) {
		t.Fatalf("%s records %d provenance sources, want %d", domainEventMobsFamily, len(family.Sources), len(domainEventMobsFamilySources))
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
		t.Fatalf("%s lists %d cases, want %d", domainEventMobsFamily, len(family.Cases), len(registered))
	}
	for _, id := range family.Cases {
		if !registered[id] {
			t.Fatalf("%s lists case %s which the selection does not register", domainEventMobsFamily, id)
		}
	}
}

// TestDomainEventMobsOracleOutcomesDistinguishAcceptedAndRejected pins the
// exact shape of the executed evidence: the 68 unique labels, both an
// admitted record and a rejection for every rule, the accepted zero tick, the
// inclusive health ends, both hostile kinds, both grazing values, both
// despawn reasons, and every aggregate and field rule the classifiers name.
func TestDomainEventMobsOracleOutcomesDistinguishAcceptedAndRejected(t *testing.T) {
	records := domainEventMobsExecute(t)
	domainEventMobsAssertOutcomesDistinguishCases(t, records)

	labels := make(map[string]bool, len(records))
	for _, record := range records {
		if labels[record.label] {
			t.Fatalf("label %q executes twice", record.label)
		}
		labels[record.label] = true
	}
	if len(labels) != domainEventMobsCaseTotal {
		t.Fatalf("executed evidence carries %d unique labels, want %d", len(labels), domainEventMobsCaseTotal)
	}

	tally := make(map[string]*domainEventMobsRuleTally)
	perRule := make(map[string]int, len(records))
	for _, record := range records {
		perRule[record.input.Rule]++
		entry, ok := tally[record.input.Rule]
		if !ok {
			entry = &domainEventMobsRuleTally{}
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
		domainEventMobsRuleHostileSpawn:   14,
		domainEventMobsRuleHostileState:   14,
		domainEventMobsRuleHostileDespawn: 6,
		domainEventMobsRulePassiveSpawn:   13,
		domainEventMobsRulePassiveState:   14,
		domainEventMobsRulePassiveDespawn: 7,
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
		domainEventMobsRuleHostileSpawn, domainEventMobsRuleHostileState, domainEventMobsRuleHostileDespawn,
		domainEventMobsRulePassiveSpawn, domainEventMobsRulePassiveState, domainEventMobsRulePassiveDespawn,
	} {
		if !domainEventMobsFieldsContain(tally[rule].accepted, "server_tick", "0") {
			t.Fatalf("no admitted %s batch carries the zero tick", rule)
		}
	}
	for _, spawn := range []string{
		domainEventMobsRuleHostileSpawn, domainEventMobsRuleHostileState,
		domainEventMobsRulePassiveSpawn, domainEventMobsRulePassiveState,
	} {
		array := "spawns"
		if tally[spawn].accepted[0]["spawns"] == nil {
			array = "states"
		}
		for _, health := range []any{1, 20} {
			if !domainEventMobsNestedContains(tally[spawn].accepted, array, "health", health) {
				t.Fatalf("no admitted %s record carries the inclusive health end %v", spawn, health)
			}
		}
	}
	for _, kind := range []any{0, 1} {
		if !domainEventMobsNestedContains(tally[domainEventMobsRuleHostileSpawn].accepted, "spawns", "kind", kind) {
			t.Fatalf("no admitted hostile spawn carries the published kind %v", kind)
		}
	}
	for _, grazing := range []any{false, true} {
		if !domainEventMobsNestedContains(tally[domainEventMobsRulePassiveState].accepted, "states", "grazing", grazing) {
			t.Fatalf("no admitted passive state carries the grazing value %v", grazing)
		}
	}
	for _, reason := range []any{0, 1} {
		if !domainEventMobsNestedContains(tally[domainEventMobsRulePassiveDespawn].accepted, "despawns", "reason", reason) {
			t.Fatalf("no admitted passive despawn carries the published reason %v", reason)
		}
	}

	// Every boundary below is looked up by the exact rule name the
	// classifier publishes, so a renamed or dropped boundary fails here
	// rather than surviving as an unpinned rule.
	for _, rule := range []string{
		domainEventMobsNameHostileSpawn + ".count_range",
		domainEventMobsNameHostileSpawn + ".strictly_increasing_ids",
		domainEventMobsNameHostileSpawn + ".record_0.id",
		domainEventMobsNameHostileSpawn + ".record_0.dimension",
		domainEventMobsNameHostileSpawn + ".record_0.position",
		domainEventMobsNameHostileSpawn + ".record_0.yaw",
		domainEventMobsNameHostileSpawn + ".record_0.health",
		domainEventMobsNameHostileState + ".count_range",
		domainEventMobsNameHostileState + ".strictly_increasing_ids",
		domainEventMobsNameHostileState + ".record_0.id",
		domainEventMobsNameHostileState + ".record_0.position",
		domainEventMobsNameHostileState + ".record_0.velocity",
		domainEventMobsNameHostileState + ".record_0.yaw",
		domainEventMobsNameHostileState + ".record_0.health",
		domainEventMobsNameHostileState + ".record_0.kind",
		domainEventMobsNameHostileDespawn + ".count_range",
		domainEventMobsNameHostileDespawn + ".strictly_increasing_ids",
		domainEventMobsNameHostileDespawn + ".record_0.id",
		domainEventMobsNamePassiveSpawn + ".count_range",
		domainEventMobsNamePassiveSpawn + ".strictly_increasing_ids",
		domainEventMobsNamePassiveSpawn + ".record_0.id",
		domainEventMobsNamePassiveSpawn + ".record_0.dimension",
		domainEventMobsNamePassiveSpawn + ".record_0.position",
		domainEventMobsNamePassiveSpawn + ".record_0.yaw",
		domainEventMobsNamePassiveSpawn + ".record_0.health",
		domainEventMobsNamePassiveState + ".count_range",
		domainEventMobsNamePassiveState + ".strictly_increasing_ids",
		domainEventMobsNamePassiveState + ".record_0.id",
		domainEventMobsNamePassiveState + ".record_0.position",
		domainEventMobsNamePassiveState + ".record_0.velocity",
		domainEventMobsNamePassiveState + ".record_0.yaw",
		domainEventMobsNamePassiveState + ".record_0.health",
		domainEventMobsNamePassiveState + ".record_0.grazing",
		domainEventMobsNamePassiveDespawn + ".count_range",
		domainEventMobsNamePassiveDespawn + ".strictly_increasing_ids",
		domainEventMobsNamePassiveDespawn + ".record_0.id",
		domainEventMobsNamePassiveDespawn + ".record_0.reason",
	} {
		if !domainEventMobsRejectionNamesRule(tally, rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", rule)
		}
	}
}

// TestDomainEventMobsOracleRunnerRejectsUnregisteredFamily pins that a case
// cannot pass by being ignored.
func TestDomainEventMobsOracleRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventMobsWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "domain.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family domain.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestDomainEventMobsOracleRunnerRejectsOperationFamilyMismatch pins that a
// case cannot declare one operation and be executed by another.
func TestDomainEventMobsOracleRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventMobsWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "decode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation-family mismatch failure, got: %v", err)
	}
}

// TestDomainEventMobsOracleRunnerRejectsTamperedInput pins that a case whose
// committed bytes no longer match their recorded digest fails the run instead
// of executing bytes the manifest does not name.
func TestDomainEventMobsOracleRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventMobsWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestDomainEventMobsOracleReportPublishesAndValidates assembles the executed
// evidence into a report, publishes it through the production atomic
// exporter, reloads it and validates it against the working manifest. This is
// the identity and content check a later Rust acceptance step performs; it
// does not claim any Rust behaviour.
func TestDomainEventMobsOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventMobsWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event mobs evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, domainEventMobsCorpusReportName)
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

// TestDomainEventMobsOracleRejectsMissingProducerTest pins that the executed
// evidence has a real producer behind it, including the topic-named entry
// point the domain plan's filter selects. A corpus whose producer test is
// gone has no independent execution, only frozen files, and a missing topic
// entry point would make the plan filter pass without running anything.
func TestDomainEventMobsOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(domainEventMobsProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", domainEventMobsProducerTestRelPath, err)
	}
	for _, name := range []string{domainEventMobsProducerExecuteName, domainEventMobsProducerTopicName} {
		declaration := "func " + name + "(t *testing.T) {"
		if !strings.Contains(string(data), declaration) {
			t.Fatalf("%s does not declare %s", domainEventMobsProducerTestRelPath, declaration)
		}
	}
}

// TestDomainEventMobsExportRejectsRepositoryAndSymlinkTargets pins the
// publication boundaries of this producer's export identity: an export root
// inside the repository is refused before anything is created, and an export
// root reached through a symlinked ancestor is refused without writing
// through the link.
func TestDomainEventMobsExportRejectsRepositoryAndSymlinkTargets(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	assets := []generatedAsset{{RelativePath: "hostile-spawn-seed.input.json", Data: []byte("{}\n")}}

	contained := filepath.Join(root, "mobs-export-probe")
	_, err := exportGeneratedAssets(root, contained, domainEventMobsProducerID, assets)
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
	_, err = exportGeneratedAssets(root, filepath.Join(symlinkDir, "export"), domainEventMobsProducerID, assets)
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

// domainEventMobsSelection exposes this producer's exact reviewed corpus
// registration: the 68 committed case specifications with digests proven
// against the files on disk, plus the provenance paths those rules are read
// from. The shared manifest-candidate helper consumes it, so the candidate
// this node publishes is assembled from the same reviewed `CaseSpec` values
// the committed corpus freezes rather than from a second rendering.
func domainEventMobsSelection(t *testing.T, root string) domainEventSelection {
	t.Helper()
	return domainEventSelection{
		Cases:   domainEventMobsCorpusCases(t, root),
		Sources: append([]string(nil), domainEventMobsFamilySources...),
	}
}

// The frozen manifest counts this producer's re-merge must reach now that
// the domain corpus partition is closed: 533 total `mornlea_domain` cases,
// 321 of them registered under the shared `domain.event` family. The
// node-era intermediate totals (444 and 232) were correct while the corpus
// was still growing; the closed partition is the final target every
// producer's merge gate shares.
const (
	domainEventMobsMergedDomainTotal = 533
	domainEventMobsMergedFamilyTotal = 321
)

// TestDomainEventMobsManifestCandidateRegistersEveryMobsCase merges this
// producer's selection into the tracked manifest and proves the merged
// candidate reaches the frozen node counts before it is published. A base
// that already registers the mobs corpus accepts an idempotent re-merge of
// the same reviewed specs, so the same test gate covers both the first
// publication and every later ordinary run.
func TestDomainEventMobsManifestCandidateRegistersEveryMobsCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	selection := domainEventMobsSelection(t, root)
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
			t.Fatalf("merged manifest drops mobs case %s", want.ID)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("merged manifest rewrites mobs case %s", want.ID)
		}
		if !baseIDs[want.ID] {
			added++
		}
	}
	if added != 0 && added != domainEventMobsCaseTotal {
		t.Fatalf("merged manifest adds %d new mobs cases, want 0 (already registered) or %d", added, domainEventMobsCaseTotal)
	}
	if len(merged.Cases) != len(base.Cases)+added {
		t.Fatalf("merged manifest carries %d cases, want %d", len(merged.Cases), len(base.Cases)+added)
	}

	domainTotal, familyTopLevel := 0, 0
	for _, c := range merged.Cases {
		if c.RustConsumer == domainEventMobsConsumer {
			domainTotal++
		}
		if c.Family == domainEventMobsFamily {
			familyTopLevel++
		}
	}
	if domainTotal != domainEventMobsMergedDomainTotal {
		t.Fatalf("merged manifest registers %d mornlea_domain cases, want %d", domainTotal, domainEventMobsMergedDomainTotal)
	}
	if familyTopLevel != domainEventMobsMergedFamilyTotal {
		t.Fatalf("merged manifest registers %d domain.event cases, want %d", familyTopLevel, domainEventMobsMergedFamilyTotal)
	}
	family, ok := domainEventMobsFamilySpec(merged)
	if !ok {
		t.Fatalf("merged manifest has no %s family", domainEventMobsFamily)
	}
	if len(family.Cases) != domainEventMobsMergedFamilyTotal {
		t.Fatalf("%s lists %d cases, want %d", domainEventMobsFamily, len(family.Cases), domainEventMobsMergedFamilyTotal)
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

// TestDomainOracle_event_mobs is the topic-named entry point the domain plan
// names for this node. It delegates to the same executed table, so the two
// filters select one source of expected results rather than two.
func TestDomainOracle_event_mobs(t *testing.T) {
	TestDomainEventMobsOracleExecutesEveryCase(t)
}

// domainEventMobsRuleTally collects the accepted and rejected normalized
// outcomes one rule records, so a boundary assertion can look for the field
// value it names.
type domainEventMobsRuleTally struct {
	accepted []map[string]any
	rejected []map[string]any
}

// domainEventMobsFieldsContain reports whether one recorded outcome set holds
// a record whose named field carries the value a boundary assertion names.
func domainEventMobsFieldsContain(records []map[string]any, field string, want any) bool {
	for _, fields := range records {
		if reflect.DeepEqual(fields[field], want) {
			return true
		}
	}
	return false
}

// domainEventMobsNestedContains reports whether one recorded outcome set
// holds a batch whose records carry the named field value. Every mob rule is
// a batch, so a boundary that names a record field looks inside the rule's
// array rather than at the record's top level. The array type is the
// producer's own in-memory rendering, because the executed records have not
// been decoded back from JSON.
func domainEventMobsNestedContains(records []map[string]any, array, field string, want any) bool {
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

// domainEventMobsRejectionNamesRule reports whether any recorded rejection
// names the broken rule a boundary assertion pins.
func domainEventMobsRejectionNamesRule(tally map[string]*domainEventMobsRuleTally, rule string) bool {
	for _, entry := range tally {
		for _, fields := range entry.rejected {
			if fields["rule"] == rule {
				return true
			}
		}
	}
	return false
}

// domainEventMobsAssertCaseSpecs runs the production case validation over the
// whole selection, so a case with a bad path, digest, format or checkpoint
// fails here rather than surfacing later as a confusing coverage failure.
func domainEventMobsAssertCaseSpecs(t *testing.T, root string, manifest Inventory) {
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

// domainEventMobsAssertOutcomesDistinguishCases proves the executed evidence
// separates admitted records from rejections instead of publishing one
// constant answer, and that the rejections name a rule the authority
// publishes.
func domainEventMobsAssertOutcomesDistinguishCases(t *testing.T, records []domainEventMobsRecord) {
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

// domainEventMobsFamilySpec indexes the working manifest by family identity.
func domainEventMobsFamilySpec(manifest Inventory) (Family, bool) {
	for _, family := range manifest.Families {
		if family.ID == domainEventMobsFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainEventMobsCaseByID indexes a manifest selection by case identity.
func domainEventMobsCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}
