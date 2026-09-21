package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
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

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file is the Go producer for the domain remote-player and companion
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
// The six rules this family executes are the Go `protocol.RemotePlayerSpawn`,
// `protocol.RemotePlayerDespawn`, `protocol.RemotePlayerStates`,
// `protocol.CompanionSpawn`, `protocol.CompanionStates` and
// `protocol.CompanionDespawn` records, which are the visibility-derived
// observations an authoritative session publishes about the other players and
// the companions one subscriber can see.
//
// Two Go wire rules deliberately stay out of this producer, because the domain
// value they describe does not carry them and a case pinning one could not be
// replayed against the Rust consumer. The 7-record remote-player batch maximum
// and the 4-record companion batch maximum are transport budgets the protocol
// layer applies, so the domain requires only a nonempty batch whose identities
// are strictly increasing; a case above a wire maximum would therefore record a
// rejection the domain does not publish. The publish tick is rendered under the
// Go DTO's own field name — "server_tick" on the remote records and `tick` on
// the companion ones — while the normalized outcome names it "server_tick",
// because that is the domain record's field name. Every array a case carries is
// serialized explicitly, an empty one included, so a replay consumer never
// reads a missing key as an absent batch.

const (
	// domainEventPeopleFamily is the corpus family this package executes. The
	// family is the existing `domain.event` row, whose eventual owner is the
	// Rust domain crate that owns the replay observation records.
	domainEventPeopleFamily = "domain.event"
	// domainEventPeopleVersion is the family's discovered version. A case has
	// to name its family's version, so the case identities carry this segment
	// rather than one this producer chose.
	domainEventPeopleVersion = "1"
	// domainEventPeopleOperation is the manifest operation name for a
	// remote-player or companion observation admission case.
	domainEventPeopleOperation = "admit"
	// domainEventPeopleConsumer is the manifest consumer the change pins for
	// this family: the Rust crate that owns these records.
	domainEventPeopleConsumer = "mornlea_domain"
	// domainEventPeopleCorpusRelDir is the repository-relative directory
	// holding the frozen event people corpus cases.
	domainEventPeopleCorpusRelDir = "testdata/runtime-migration/cases/domain/event_people"
	// domainEventPeopleProducerTestRelPath and the two producer test names
	// locate the package-local producer that executes the protocol DTOs. The
	// topic name is the entry point the domain plan's filter requires; a corpus
	// whose producer test is missing has no independently executed evidence at
	// all.
	domainEventPeopleProducerTestRelPath = "packages/tools/cmd/runtime-oracle/domain_event_people_test.go"
	domainEventPeopleProducerExecuteName = "TestDomainEventPeopleOracleExecutesEveryCase"
	domainEventPeopleProducerTopicName   = "TestDomainOracle_event_people"
	// domainEventPeopleCorpusReportName is the published report file name for
	// the executed event people evidence.
	domainEventPeopleCorpusReportName = "runtime-corpus-domain-event-people.json"
	// domainEventPeopleSource is the primary provenance source of the
	// remote-player publication rules.
	domainEventPeopleSource = "packages/shared/network/protocol/message_player.go"

	// domainEventPeopleRuleRemoteSpawn, domainEventPeopleRuleRemoteDespawn,
	// domainEventPeopleRuleRemoteStates, domainEventPeopleRuleCompanionSpawn,
	// domainEventPeopleRuleCompanionStates and
	// domainEventPeopleRuleCompanionDespawn are the six rule names a case
	// names. The family is shared with the world observations, the player and
	// outcome records and the inventory and container publications, so the rule
	// name is the discriminator the manifest and the family router both read.
	domainEventPeopleRuleRemoteSpawn      = "remote-player-spawn"
	domainEventPeopleRuleRemoteDespawn    = "remote-player-despawn"
	domainEventPeopleRuleRemoteStates     = "remote-player-states"
	domainEventPeopleRuleCompanionSpawn   = "companion-spawn"
	domainEventPeopleRuleCompanionStates  = "companion-states"
	domainEventPeopleRuleCompanionDespawn = "companion-despawn"

	// domainEventPeopleRemoteBatchMax is the remote-player state batch maximum
	// the Go `RemotePlayerStates.Validate` keeps as a literal, because the
	// protocol package exports no constant for it. It is a transport budget the
	// domain does not carry, so no case in this family sits above it; the
	// classifier names the bound only so a count a case does name stays
	// classifiable.
	domainEventPeopleRemoteBatchMax = 7

	// domainEventPeoplePitchLimit is the inclusive companion vertical look
	// bound the Go `validCompanionPose` predicate publishes, rendered as the
	// float32 the comparison uses. A remote-player record carries no pitch
	// bound at all, so the same value a companion case rejects is admitted for
	// a peer session.
	domainEventPeoplePitchLimit = float32(math.Pi / 2)
)

// domainEventPeopleFamilySources is the merged provenance set the family
// records. Every entry is a file the producer's rules are read from, so a
// change to any of them is a change to the recorded evidence. The two protocol
// message files carry the record shapes and the validators, the registry file
// decides which of these records are published in the play state, the core
// identity file owns the UUIDv4 gate and the display-name admission the spawn
// name goes through, and the companion identity file owns the companion name
// rule and the companion activity limit the protocol batch maximum aliases.
var domainEventPeopleFamilySources = []string{
	"packages/shared/network/protocol/message_player.go",
	"packages/shared/network/protocol/message_companion.go",
	"packages/shared/network/protocol/registry.go",
	"packages/shared/core/player_id.go",
	"packages/shared/companion/identity.go",
}

// domainEventPeopleLabelPattern is the shape a corpus label must have: a
// lowercase slug with an optional zero-padded numeric suffix, so a boundary row
// sorts in numeric order under a lexical sort.
var domainEventPeopleLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(-[0-9]+)?$`)

// updateDomainEventPeopleCorpus rewrites the frozen event people corpus from
// the executed protocol DTOs. It follows the same discipline as the other
// fixture update flags: an ordinary run only compares, so a frozen artifact is
// never silently regenerated to match an implementation.
var updateDomainEventPeopleCorpus = flag.Bool(
	"update-domain-event-people-corpus",
	false,
	"rewrite testdata/runtime-migration/cases/domain/event_people from the executed protocol remote player and companion DTOs",
)

// domainEventPeopleInput is the frozen, self-describing corpus input for one
// case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority. Every scalar is a pointer so an
// absent field and an explicit zero stay distinguishable, which matters
// because a zero server tick and a zero dimension are meaningful values.
// `Position`, `Yaw` and `Pitch` are decimal float texts rather than JSON
// numbers because a JSON number cannot express NaN, an infinity or a signed
// zero, and those exact values are boundaries this family pins. The two batch
// arrays are plain slices without "omitempty", so a case that carries a batch
// always carries the key and an empty batch renders as an explicit empty list
// rather than vanishing. The six rules share one envelope because each rule
// reads only the fields it names.
type domainEventPeopleInput struct {
	Consumer string `json:"consumer"`
	Rule     string `json:"rule"`

	// Remote-player spawn and despawn fields, in the Go DTOs' declaration
	// order.
	PlayerID    *string `json:"player_id,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`

	// Shared spawn fields. The remote records name the publish tick
	// "server_tick" and the companion records name it `tick`, because the input
	// is the Go DTO's own shape; the normalized outcome names both
	// "server_tick", which is the domain record's field name.
	ServerTick *uint64  `json:"server_tick,omitempty"`
	Tick       *uint64  `json:"tick,omitempty"`
	Dimension  *int32   `json:"dimension,omitempty"`
	Position   []string `json:"position,omitempty"`
	Yaw        *string  `json:"yaw,omitempty"`
	Pitch      *string  `json:"pitch,omitempty"`

	// Companion spawn and despawn fields, whose identity field is named `ID`
	// and whose text field is named `Name` in the Go DTO.
	ID   *string `json:"id,omitempty"`
	Name *string `json:"name,omitempty"`

	// Batch fields. The remote batch carries its records under the Go DTO's
	// `Players` name and the companion batch under `States`; the normalized
	// outcome names both `states`, which is the domain record's field name.
	Players []domainEventPeopleRemoteRecord    `json:"players"`
	States  []domainEventPeopleCompanionRecord `json:"states"`
}

// domainEventPeopleRemoteRecord is the frozen rendering of one remote-player
// state record: its identity, dimension, pose, look and replay reset bit.
type domainEventPeopleRemoteRecord struct {
	PlayerID  string   `json:"player_id"`
	Dimension int32    `json:"dimension"`
	Position  []string `json:"position"`
	Yaw       string   `json:"yaw"`
	Pitch     string   `json:"pitch"`
	Reset     bool     `json:"reset"`
}

// domainEventPeopleCompanionRecord is the frozen rendering of one companion
// state record, whose identity field is named `ID` in the Go DTO.
type domainEventPeopleCompanionRecord struct {
	ID        string   `json:"id"`
	Dimension int32    `json:"dimension"`
	Position  []string `json:"position"`
	Yaw       string   `json:"yaw"`
	Pitch     string   `json:"pitch"`
	Reset     bool     `json:"reset"`
}

// domainEventPeopleCase is one frozen corpus case: its label and its input, and
// nothing else.
type domainEventPeopleCase struct {
	label string
	input domainEventPeopleInput
}

// domainEventPeopleBaseInput is the envelope every case starts from: the
// consumer, the rule and two explicitly empty batch arrays, so a case that
// names no batch still serializes the key as an empty list rather than dropping
// it.
func domainEventPeopleBaseInput(rule string) domainEventPeopleInput {
	return domainEventPeopleInput{
		Consumer: domainEventPeopleConsumer,
		Rule:     rule,
		Players:  []domainEventPeopleRemoteRecord{},
		States:   []domainEventPeopleCompanionRecord{},
	}
}

// The two identity vectors the family pins, ending `fe` and `ff` so the
// lexicographic raw-byte order of a batch is `fe` before `ff`. They are case
// inputs, never expected outcomes: the producer's answer about each one comes
// from the Go validator.
const (
	domainEventPeoplePlayerIDFirst   = "00112233445546778899aabbccddeefe"
	domainEventPeoplePlayerIDSecond  = "00112233445546778899aabbccddeeff"
	domainEventPeoplePlayerIDZero    = "00000000000000000000000000000000"
	domainEventPeoplePlayerIDWrong   = "00112233445536778899aabbccddeeff"
	domainEventPeopleCompanionFirst  = "2233445546674889aabbccddeeff00fe"
	domainEventPeopleCompanionSecond = "2233445546674889aabbccddeeff00ff"
	domainEventPeopleCompanionZero   = "00000000000000000000000000000000"
	domainEventPeopleCompanionWrong  = "2233445546674889aabbccddeeff0036"
)

// domainEventPeoplePlayerID resolves the player identity field one case names.
// The value is the 16 raw bytes in lowercase hex, so a zero identity, a wrong
// version nibble and a wrong variant are expressible as inputs instead of being
// unreachable through a text parser that would reject them first.
func domainEventPeoplePlayerID(c CaseSpec, name string, value *string) (core.PlayerID, error) {
	if value == nil {
		return core.PlayerID{}, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	if len(*value) != 32 {
		return core.PlayerID{}, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not 32 hex digits", c.ID, name, *value)
	}
	if strings.ToLower(*value) != *value {
		return core.PlayerID{}, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not lowercase", c.ID, name, *value)
	}
	var id core.PlayerID
	decoded, err := hex.Decode(id[:], []byte(*value))
	if err != nil || decoded != len(id) {
		return core.PlayerID{}, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not hexadecimal", c.ID, name, *value)
	}
	return id, nil
}

// domainEventPeopleCompanionID resolves the companion identity field one case
// names, under the same raw-byte rule the player identity uses.
func domainEventPeopleCompanionID(c CaseSpec, name string, value *string) (companion.ID, error) {
	if value == nil {
		return companion.ID{}, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	if len(*value) != 32 {
		return companion.ID{}, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not 32 hex digits", c.ID, name, *value)
	}
	if strings.ToLower(*value) != *value {
		return companion.ID{}, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not lowercase", c.ID, name, *value)
	}
	var id companion.ID
	decoded, err := hex.Decode(id[:], []byte(*value))
	if err != nil || decoded != len(id) {
		return companion.ID{}, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not hexadecimal", c.ID, name, *value)
	}
	return id, nil
}

// domainEventPeopleSpawnSeed is the seed remote-player spawn: the `ff`
// identity, a canonical name, tick 41, the overworld, a finite pose and a
// finite look.
func domainEventPeopleSpawnSeed() protocol.RemotePlayerSpawn {
	return protocol.RemotePlayerSpawn{
		PlayerID:    domainEventPeoplePlayerVector(domainEventPeoplePlayerIDSecond),
		DisplayName: "Alice",
		ServerTick:  41,
		Dimension:   core.Overworld,
		Position:    mgl32.Vec3{1.5, 64, -3.25},
		Yaw:         0.5,
		Pitch:       -0.25,
	}
}

// domainEventPeopleSpawnCase renders one remote-player spawn row: the seed
// spawn with exactly one field changed, so a boundary row is always the seed
// plus the value the rule names.
func domainEventPeopleSpawnCase(label string, mutate func(*protocol.RemotePlayerSpawn)) domainEventPeopleCase {
	spawn := domainEventPeopleSpawnSeed()
	if mutate != nil {
		mutate(&spawn)
	}
	return domainEventPeopleCase{label: label, input: domainEventPeopleSpawnInput(spawn)}
}

// domainEventPeopleSpawnInput renders one Go spawn value as the frozen input
// envelope. The float texts keep NaN, the infinities and a signed zero
// expressible, so the envelope is a lossless rendering of the DTO value.
func domainEventPeopleSpawnInput(spawn protocol.RemotePlayerSpawn) domainEventPeopleInput {
	input := domainEventPeopleBaseInput(domainEventPeopleRuleRemoteSpawn)
	playerID := domainEventPeoplePlayerText(spawn.PlayerID)
	input.PlayerID = &playerID
	input.DisplayName = &spawn.DisplayName
	input.ServerTick = &spawn.ServerTick
	dimension := int32(spawn.Dimension)
	input.Dimension = &dimension
	input.Position = domainEventPeopleVectorText(spawn.Position)
	input.Yaw = domainEventPeopleFloatTextPointer(spawn.Yaw)
	input.Pitch = domainEventPeopleFloatTextPointer(spawn.Pitch)
	return input
}

// domainEventPeopleRemoteDespawnCase renders one remote-player despawn row,
// whose entire payload is the identity.
func domainEventPeopleRemoteDespawnCase(label string, id core.PlayerID) domainEventPeopleCase {
	input := domainEventPeopleBaseInput(domainEventPeopleRuleRemoteDespawn)
	playerID := domainEventPeoplePlayerText(id)
	input.PlayerID = &playerID
	return domainEventPeopleCase{label: label, input: input}
}

// domainEventPeopleRemoteStatesSeed is the seed remote-player state batch: tick
// 7 and the two identities in the canonical `fe` before `ff` order, each in the
// overworld with a finite pose, a finite look and a false reset.
func domainEventPeopleRemoteStatesSeed() protocol.RemotePlayerStates {
	return protocol.RemotePlayerStates{
		ServerTick: 7,
		Players: []protocol.RemotePlayerState{
			domainEventPeopleRemoteState(domainEventPeoplePlayerIDFirst, core.Overworld, false),
			domainEventPeopleRemoteState(domainEventPeoplePlayerIDSecond, core.Overworld, false),
		},
	}
}

// domainEventPeopleRemoteState renders one remote-player state record from its
// identity text, its dimension and its reset bit, with the seed pose and look.
func domainEventPeopleRemoteState(playerID string, dimension core.DimensionID, reset bool) protocol.RemotePlayerState {
	return protocol.RemotePlayerState{
		PlayerID:  domainEventPeoplePlayerVector(playerID),
		Dimension: dimension,
		Position:  mgl32.Vec3{2.5, 65, -4.25},
		Yaw:       -1,
		Pitch:     0.75,
		Reset:     reset,
	}
}

// domainEventPeopleRemoteStatesCase renders one remote-player state batch row:
// the seed batch with exactly one field changed.
func domainEventPeopleRemoteStatesCase(label string, mutate func(*protocol.RemotePlayerStates)) domainEventPeopleCase {
	states := domainEventPeopleRemoteStatesSeed()
	if mutate != nil {
		mutate(&states)
	}
	return domainEventPeopleCase{label: label, input: domainEventPeopleRemoteStatesInput(states)}
}

// domainEventPeopleRemoteStatesInput renders one Go batch value as the frozen
// input envelope. Every record of the batch is serialized, so a replay proves
// the exact sequence the record carried.
func domainEventPeopleRemoteStatesInput(states protocol.RemotePlayerStates) domainEventPeopleInput {
	input := domainEventPeopleBaseInput(domainEventPeopleRuleRemoteStates)
	input.ServerTick = &states.ServerTick
	records := make([]domainEventPeopleRemoteRecord, 0, len(states.Players))
	for _, state := range states.Players {
		records = append(records, domainEventPeopleRemoteRecord{
			PlayerID:  domainEventPeoplePlayerText(state.PlayerID),
			Dimension: int32(state.Dimension),
			Position:  domainEventPeopleVectorText(state.Position),
			Yaw:       domainEventPeopleFloatText(state.Yaw),
			Pitch:     domainEventPeopleFloatText(state.Pitch),
			Reset:     state.Reset,
		})
	}
	input.Players = records
	return input
}

// domainEventPeopleCompanionSpawnSeed is the seed companion spawn: the `ff`
// identity, a canonical name, tick 41, the overworld, a finite pose and a
// finite look.
func domainEventPeopleCompanionSpawnSeed() protocol.CompanionSpawn {
	return protocol.CompanionSpawn{
		ID:        domainEventPeopleCompanionVector(domainEventPeopleCompanionSecond),
		Name:      "Buddy",
		Tick:      41,
		Dimension: core.Overworld,
		Position:  mgl32.Vec3{-8.5, 70, 12.25},
		Yaw:       3,
		Pitch:     -0.5,
	}
}

// domainEventPeopleCompanionSpawnCase renders one companion spawn row: the seed
// spawn with exactly one field changed.
func domainEventPeopleCompanionSpawnCase(label string, mutate func(*protocol.CompanionSpawn)) domainEventPeopleCase {
	spawn := domainEventPeopleCompanionSpawnSeed()
	if mutate != nil {
		mutate(&spawn)
	}
	return domainEventPeopleCase{label: label, input: domainEventPeopleCompanionSpawnInput(spawn)}
}

// domainEventPeopleCompanionSpawnInput renders one Go companion spawn value as
// the frozen input envelope, under the companion DTO's own field names.
func domainEventPeopleCompanionSpawnInput(spawn protocol.CompanionSpawn) domainEventPeopleInput {
	input := domainEventPeopleBaseInput(domainEventPeopleRuleCompanionSpawn)
	id := domainEventPeopleCompanionText(spawn.ID)
	input.ID = &id
	input.Name = &spawn.Name
	input.Tick = &spawn.Tick
	dimension := int32(spawn.Dimension)
	input.Dimension = &dimension
	input.Position = domainEventPeopleVectorText(spawn.Position)
	input.Yaw = domainEventPeopleFloatTextPointer(spawn.Yaw)
	input.Pitch = domainEventPeopleFloatTextPointer(spawn.Pitch)
	return input
}

// domainEventPeopleCompanionDespawnCase renders one companion despawn row,
// whose entire payload is the identity.
func domainEventPeopleCompanionDespawnCase(label string, id companion.ID) domainEventPeopleCase {
	input := domainEventPeopleBaseInput(domainEventPeopleRuleCompanionDespawn)
	text := domainEventPeopleCompanionText(id)
	input.ID = &text
	return domainEventPeopleCase{label: label, input: input}
}

// domainEventPeopleCompanionStatesSeed is the seed companion state batch: tick
// 7 and the two identities in the canonical `fe` before `ff` order, each in the
// overworld with a finite pose, a finite look and a false reset.
func domainEventPeopleCompanionStatesSeed() protocol.CompanionStates {
	return protocol.CompanionStates{
		Tick: 7,
		States: []protocol.CompanionState{
			domainEventPeopleCompanionState(domainEventPeopleCompanionFirst, false),
			domainEventPeopleCompanionState(domainEventPeopleCompanionSecond, false),
		},
	}
}

// domainEventPeopleCompanionState renders one companion state record from its
// identity text and its reset bit, with the seed pose and look.
func domainEventPeopleCompanionState(id string, reset bool) protocol.CompanionState {
	return protocol.CompanionState{
		ID:        domainEventPeopleCompanionVector(id),
		Dimension: core.Overworld,
		Position:  mgl32.Vec3{-7.5, 70, 11.25},
		Yaw:       -3,
		Pitch:     0.5,
		Reset:     reset,
	}
}

// domainEventPeopleCompanionStatesCase renders one companion state batch row:
// the seed batch with exactly one field changed.
func domainEventPeopleCompanionStatesCase(label string, mutate func(*protocol.CompanionStates)) domainEventPeopleCase {
	states := domainEventPeopleCompanionStatesSeed()
	if mutate != nil {
		mutate(&states)
	}
	return domainEventPeopleCase{label: label, input: domainEventPeopleCompanionStatesInput(states)}
}

// domainEventPeopleCompanionStatesInput renders one Go companion batch value as
// the frozen input envelope. Every record of the batch is serialized.
func domainEventPeopleCompanionStatesInput(states protocol.CompanionStates) domainEventPeopleInput {
	input := domainEventPeopleBaseInput(domainEventPeopleRuleCompanionStates)
	input.Tick = &states.Tick
	records := make([]domainEventPeopleCompanionRecord, 0, len(states.States))
	for _, state := range states.States {
		records = append(records, domainEventPeopleCompanionRecord{
			ID:        domainEventPeopleCompanionText(state.ID),
			Dimension: int32(state.Dimension),
			Position:  domainEventPeopleVectorText(state.Position),
			Yaw:       domainEventPeopleFloatText(state.Yaw),
			Pitch:     domainEventPeopleFloatText(state.Pitch),
			Reset:     state.Reset,
		})
	}
	input.States = records
	return input
}

// domainEventPeopleCases is the ordered case table the producer executes.
//
// The remote-player spawn rows are the seed plus one boundary each: the zero
// server tick, the depths dimension, the pitch outside the vertical look range
// that a companion record would refuse, the non-finite pose and rotation, the
// zero and wrong-version identities, the untrimmed name and the unknown
// dimension. The despawn rows are the seed identity and the zero one. The
// remote-player state rows are the seed batch plus one boundary each: the zero
// tick, the empty batch, the reversed and duplicate identities, the depths
// dimension with an unbounded pitch, the invalid identity, the unknown
// dimension and the non-finite pose.
//
// The companion rows mirror that shape under the companion rules: the seed
// spawn, the zero tick, both inclusive pitch limits, the next float above and
// below each limit, the depths and unknown dimensions, the zero and
// wrong-version identities, the spaced name, and the non-finite pose and
// rotation; the despawn seed and its zero identity; and the state batch with
// the zero tick, the empty, reversed and duplicate batches, the pitch above the
// limit, the depths dimension, the invalid identity and the non-finite pose.
//
// No row sits above a wire batch maximum. The 7-record remote-player and
// 4-record companion maxima are protocol budgets, so a case above one would
// record a rejection the domain does not publish and could not be replayed
// against the Rust consumer; the Rust test pins that the domain admits a batch
// of eight and of five instead.
func domainEventPeopleCases() []domainEventPeopleCase {
	cases := []domainEventPeopleCase{
		domainEventPeopleSpawnCase("remote-player-spawn-seed", nil),
		domainEventPeopleSpawnCase("remote-player-spawn-zero-server-tick", func(spawn *protocol.RemotePlayerSpawn) {
			spawn.ServerTick = 0
		}),
		domainEventPeopleSpawnCase("remote-player-spawn-depths-dimension", func(spawn *protocol.RemotePlayerSpawn) {
			spawn.Dimension = core.Depths
		}),
		domainEventPeopleSpawnCase("remote-player-spawn-pitch-outside-vertical-look-range", func(spawn *protocol.RemotePlayerSpawn) {
			// The companion vertical look bound is a companion rule, so the
			// value a companion case rejects is publishable for a peer session.
			spawn.Pitch = 4
		}),
		domainEventPeopleSpawnCase("remote-player-spawn-non-finite-position", func(spawn *protocol.RemotePlayerSpawn) {
			spawn.Position = mgl32.Vec3{float32(math.NaN()), 0, 0}
		}),
		domainEventPeopleSpawnCase("remote-player-spawn-non-finite-rotation", func(spawn *protocol.RemotePlayerSpawn) {
			spawn.Yaw = float32(math.Inf(1))
		}),
		domainEventPeopleSpawnCase("remote-player-spawn-zero-identity", func(spawn *protocol.RemotePlayerSpawn) {
			spawn.PlayerID = domainEventPeoplePlayerVector(domainEventPeoplePlayerIDZero)
		}),
		domainEventPeopleSpawnCase("remote-player-spawn-wrong-version-identity", func(spawn *protocol.RemotePlayerSpawn) {
			spawn.PlayerID = domainEventPeoplePlayerVector(domainEventPeoplePlayerIDWrong)
		}),
		domainEventPeopleSpawnCase("remote-player-spawn-untrimmed-name", func(spawn *protocol.RemotePlayerSpawn) {
			// The name has to already equal its normalized form, so a name the
			// authority would have to trim is a rejection rather than something
			// quietly repaired.
			spawn.DisplayName = " Alice"
		}),
		domainEventPeopleSpawnCase("remote-player-spawn-dimension-unknown", func(spawn *protocol.RemotePlayerSpawn) {
			spawn.Dimension = core.DimensionID(2)
		}),

		domainEventPeopleRemoteDespawnCase("remote-player-despawn-seed", domainEventPeoplePlayerVector(domainEventPeoplePlayerIDFirst)),
		domainEventPeopleRemoteDespawnCase("remote-player-despawn-zero-identity", domainEventPeoplePlayerVector(domainEventPeoplePlayerIDZero)),

		domainEventPeopleRemoteStatesCase("remote-player-states-seed", nil),
		domainEventPeopleRemoteStatesCase("remote-player-states-zero-server-tick", func(states *protocol.RemotePlayerStates) {
			states.ServerTick = 0
		}),
		domainEventPeopleRemoteStatesCase("remote-player-states-empty", func(states *protocol.RemotePlayerStates) {
			states.Players = []protocol.RemotePlayerState{}
		}),
		domainEventPeopleRemoteStatesCase("remote-player-states-reversed", func(states *protocol.RemotePlayerStates) {
			// The two identities in the opposite order, so `ff` precedes `fe`.
			states.Players[0], states.Players[1] = states.Players[1], states.Players[0]
		}),
		domainEventPeopleRemoteStatesCase("remote-player-states-duplicate", func(states *protocol.RemotePlayerStates) {
			states.Players[1] = states.Players[0]
		}),
		domainEventPeopleRemoteStatesCase("remote-player-states-depths-dimension-and-unbounded-pitch", func(states *protocol.RemotePlayerStates) {
			states.Players[0].Dimension = core.Depths
			states.Players[0].Pitch = 4
		}),
		domainEventPeopleRemoteStatesCase("remote-player-states-reset-record", func(states *protocol.RemotePlayerStates) {
			// The reset bit is the per-record replay marker a mirror needs, so
			// the batch publishes it per record rather than collapsing it into
			// one flag.
			states.Players[1].Reset = true
		}),
		domainEventPeopleRemoteStatesCase("remote-player-states-zero-identity", func(states *protocol.RemotePlayerStates) {
			states.Players[0].PlayerID = domainEventPeoplePlayerVector(domainEventPeoplePlayerIDZero)
		}),
		domainEventPeopleRemoteStatesCase("remote-player-states-dimension-unknown", func(states *protocol.RemotePlayerStates) {
			states.Players[1].Dimension = core.DimensionID(2)
		}),
		domainEventPeopleRemoteStatesCase("remote-player-states-non-finite-position", func(states *protocol.RemotePlayerStates) {
			states.Players[1].Position = mgl32.Vec3{0, float32(math.Inf(-1)), 0}
		}),

		domainEventPeopleCompanionSpawnCase("companion-spawn-seed", nil),
		domainEventPeopleCompanionSpawnCase("companion-spawn-zero-tick", func(spawn *protocol.CompanionSpawn) {
			spawn.Tick = 0
		}),
		domainEventPeopleCompanionSpawnCase("companion-spawn-pitch-at-lower-limit", func(spawn *protocol.CompanionSpawn) {
			// The bound is inclusive, so the limit itself is published rather
			// than rejected.
			spawn.Pitch = -domainEventPeoplePitchLimit
		}),
		domainEventPeopleCompanionSpawnCase("companion-spawn-pitch-at-upper-limit", func(spawn *protocol.CompanionSpawn) {
			spawn.Pitch = domainEventPeoplePitchLimit
		}),
		domainEventPeopleCompanionSpawnCase("companion-spawn-pitch-above-limit", func(spawn *protocol.CompanionSpawn) {
			spawn.Pitch = domainEventPeoplePitchAboveLimit()
		}),
		domainEventPeopleCompanionSpawnCase("companion-spawn-pitch-below-limit", func(spawn *protocol.CompanionSpawn) {
			spawn.Pitch = domainEventPeoplePitchBelowLimit()
		}),
		domainEventPeopleCompanionSpawnCase("companion-spawn-depths-dimension", func(spawn *protocol.CompanionSpawn) {
			spawn.Dimension = core.Depths
		}),
		domainEventPeopleCompanionSpawnCase("companion-spawn-dimension-unknown", func(spawn *protocol.CompanionSpawn) {
			spawn.Dimension = core.DimensionID(2)
		}),
		domainEventPeopleCompanionSpawnCase("companion-spawn-zero-identity", func(spawn *protocol.CompanionSpawn) {
			spawn.ID = domainEventPeopleCompanionVector(domainEventPeopleCompanionZero)
		}),
		domainEventPeopleCompanionSpawnCase("companion-spawn-wrong-version-identity", func(spawn *protocol.CompanionSpawn) {
			spawn.ID = domainEventPeopleCompanionVector(domainEventPeopleCompanionWrong)
		}),
		domainEventPeopleCompanionSpawnCase("companion-spawn-spaced-name", func(spawn *protocol.CompanionSpawn) {
			// The companion name rule is the display-name rule plus a rejection
			// of any Unicode whitespace, so a spaced name fails while a player
			// display name carrying the same space is admitted.
			spawn.Name = "Best Buddy"
		}),
		domainEventPeopleCompanionSpawnCase("companion-spawn-non-finite-position", func(spawn *protocol.CompanionSpawn) {
			spawn.Position = mgl32.Vec3{float32(math.NaN()), 0, 0}
		}),
		domainEventPeopleCompanionSpawnCase("companion-spawn-non-finite-rotation", func(spawn *protocol.CompanionSpawn) {
			spawn.Pitch = float32(math.Inf(1))
		}),

		domainEventPeopleCompanionDespawnCase("companion-despawn-seed", domainEventPeopleCompanionVector(domainEventPeopleCompanionFirst)),
		domainEventPeopleCompanionDespawnCase("companion-despawn-zero-identity", domainEventPeopleCompanionVector(domainEventPeopleCompanionZero)),

		domainEventPeopleCompanionStatesCase("companion-states-seed", nil),
		domainEventPeopleCompanionStatesCase("companion-states-zero-tick", func(states *protocol.CompanionStates) {
			states.Tick = 0
		}),
		domainEventPeopleCompanionStatesCase("companion-states-empty", func(states *protocol.CompanionStates) {
			states.States = []protocol.CompanionState{}
		}),
		domainEventPeopleCompanionStatesCase("companion-states-reversed", func(states *protocol.CompanionStates) {
			states.States[0], states.States[1] = states.States[1], states.States[0]
		}),
		domainEventPeopleCompanionStatesCase("companion-states-duplicate", func(states *protocol.CompanionStates) {
			states.States[1] = states.States[0]
		}),
		domainEventPeopleCompanionStatesCase("companion-states-pitch-above-limit", func(states *protocol.CompanionStates) {
			states.States[0].Pitch = domainEventPeoplePitchAboveLimit()
		}),
		domainEventPeopleCompanionStatesCase("companion-states-depths-dimension", func(states *protocol.CompanionStates) {
			states.States[1].Dimension = core.Depths
		}),
		domainEventPeopleCompanionStatesCase("companion-states-zero-identity", func(states *protocol.CompanionStates) {
			states.States[0].ID = domainEventPeopleCompanionVector(domainEventPeopleCompanionZero)
		}),
		domainEventPeopleCompanionStatesCase("companion-states-non-finite-position", func(states *protocol.CompanionStates) {
			states.States[1].Position = mgl32.Vec3{0, float32(math.Inf(-1)), 0}
		}),
	}
	return cases
}

// runDomainEventPeople executes one corpus case through the current Go protocol
// remote-player and companion DTOs.
//
// The verdict always comes from `protocol.ValidateServerPacket` in the play
// state, which is the same validator the codec applies on both the encode and
// the decode side. The rule name and the rejection category come from the same
// bounds the DTO reads, and the two are cross-checked against each other, so a
// classification that disagrees with the authority fails the run instead of
// publishing a plausible-looking rejection.
func runDomainEventPeople(c CaseSpec, input []byte) (Outcome, []byte, error) {
	spec, err := domainEventPeopleDecodeInput(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	if spec.Consumer != domainEventPeopleConsumer {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names consumer %q, want %q", c.ID, spec.Consumer, domainEventPeopleConsumer)
	}

	switch spec.Rule {
	case domainEventPeopleRuleRemoteSpawn:
		return domainEventPeopleRunRemoteSpawn(c, spec)
	case domainEventPeopleRuleRemoteDespawn:
		return domainEventPeopleRunRemoteDespawn(c, spec)
	case domainEventPeopleRuleRemoteStates:
		return domainEventPeopleRunRemoteStates(c, spec)
	case domainEventPeopleRuleCompanionSpawn:
		return domainEventPeopleRunCompanionSpawn(c, spec)
	case domainEventPeopleRuleCompanionStates:
		return domainEventPeopleRunCompanionStates(c, spec)
	case domainEventPeopleRuleCompanionDespawn:
		return domainEventPeopleRunCompanionDespawn(c, spec)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, spec.Rule)
	}
}

// domainEventPeopleRunRemoteSpawn admits one remote-player spawn.
func domainEventPeopleRunRemoteSpawn(c CaseSpec, spec domainEventPeopleInput) (Outcome, []byte, error) {
	spawn, err := domainEventPeopleRemoteSpawn(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventPeopleNormalizeRemoteSpawn(spawn)
	category, rule := domainEventPeopleRemoteSpawnRule(spawn)
	return domainEventPeopleFinish(c, domainEventPeopleRuleRemoteSpawn, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, spawn) == nil)
}

// domainEventPeopleRunRemoteDespawn admits one remote-player despawn, whose
// entire payload is the identity.
func domainEventPeopleRunRemoteDespawn(c CaseSpec, spec domainEventPeopleInput) (Outcome, []byte, error) {
	playerID, err := domainEventPeoplePlayerID(c, "player_id", spec.PlayerID)
	if err != nil {
		return Outcome{}, nil, err
	}
	despawn := protocol.RemotePlayerDespawn{PlayerID: playerID}
	fields := map[string]any{"player_id": domainEventPeoplePlayerText(despawn.PlayerID)}
	category, rule := domainEventPeoplePlayerIDRule(playerID)
	return domainEventPeopleFinish(c, domainEventPeopleRuleRemoteDespawn, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, despawn) == nil)
}

// domainEventPeopleRunRemoteStates admits one remote-player state batch.
func domainEventPeopleRunRemoteStates(c CaseSpec, spec domainEventPeopleInput) (Outcome, []byte, error) {
	states, err := domainEventPeopleRemoteStates(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventPeopleNormalizeRemoteStates(states)
	category, rule := domainEventPeopleRemoteStatesRule(states)
	return domainEventPeopleFinish(c, domainEventPeopleRuleRemoteStates, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, states) == nil)
}

// domainEventPeopleRunCompanionSpawn admits one companion spawn.
func domainEventPeopleRunCompanionSpawn(c CaseSpec, spec domainEventPeopleInput) (Outcome, []byte, error) {
	spawn, err := domainEventPeopleCompanionSpawn(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventPeopleNormalizeCompanionSpawn(spawn)
	category, rule := domainEventPeopleCompanionSpawnRule(spawn)
	return domainEventPeopleFinish(c, domainEventPeopleRuleCompanionSpawn, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, spawn) == nil)
}

// domainEventPeopleRunCompanionStates admits one companion state batch.
func domainEventPeopleRunCompanionStates(c CaseSpec, spec domainEventPeopleInput) (Outcome, []byte, error) {
	states, err := domainEventPeopleCompanionStates(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventPeopleNormalizeCompanionStates(states)
	category, rule := domainEventPeopleCompanionStatesRule(states)
	return domainEventPeopleFinish(c, domainEventPeopleRuleCompanionStates, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, states) == nil)
}

// domainEventPeopleRunCompanionDespawn admits one companion despawn, whose
// entire payload is the identity.
func domainEventPeopleRunCompanionDespawn(c CaseSpec, spec domainEventPeopleInput) (Outcome, []byte, error) {
	id, err := domainEventPeopleCompanionID(c, "id", spec.ID)
	if err != nil {
		return Outcome{}, nil, err
	}
	despawn := protocol.CompanionDespawn{ID: id}
	fields := map[string]any{"id": domainEventPeopleCompanionText(despawn.ID)}
	category, rule := domainEventPeoplePlayerIDRule(core.PlayerID(despawn.ID))
	return domainEventPeopleFinish(c, domainEventPeopleRuleCompanionDespawn, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, despawn) == nil)
}

// domainEventPeopleRemoteSpawn resolves one remote-player spawn from its frozen
// envelope. Every field is required, because the record is complete by
// definition: an absent field would silently become a zero and a zero server
// tick or a zero dimension is a meaningful value.
func domainEventPeopleRemoteSpawn(c CaseSpec, spec domainEventPeopleInput) (protocol.RemotePlayerSpawn, error) {
	playerID, err := domainEventPeoplePlayerID(c, "player_id", spec.PlayerID)
	if err != nil {
		return protocol.RemotePlayerSpawn{}, err
	}
	if spec.DisplayName == nil {
		return protocol.RemotePlayerSpawn{}, fmt.Errorf("runtime-oracle: case %s requires a display name", c.ID)
	}
	if spec.ServerTick == nil {
		return protocol.RemotePlayerSpawn{}, fmt.Errorf("runtime-oracle: case %s requires a server tick", c.ID)
	}
	if spec.Dimension == nil {
		return protocol.RemotePlayerSpawn{}, fmt.Errorf("runtime-oracle: case %s requires a dimension", c.ID)
	}
	position, err := domainEventPeopleVec3(c, "position", spec.Position)
	if err != nil {
		return protocol.RemotePlayerSpawn{}, err
	}
	yaw, err := domainEventPeopleAngle(c, "yaw", spec.Yaw)
	if err != nil {
		return protocol.RemotePlayerSpawn{}, err
	}
	pitch, err := domainEventPeopleAngle(c, "pitch", spec.Pitch)
	if err != nil {
		return protocol.RemotePlayerSpawn{}, err
	}
	return protocol.RemotePlayerSpawn{
		PlayerID:    playerID,
		DisplayName: *spec.DisplayName,
		ServerTick:  *spec.ServerTick,
		Dimension:   core.DimensionID(*spec.Dimension),
		Position:    position,
		Yaw:         yaw,
		Pitch:       pitch,
	}, nil
}

// domainEventPeopleRemoteStates resolves one remote-player state batch from its
// frozen envelope. The batch is carried, not absent, so an empty batch is an
// explicit empty list rather than a missing key.
func domainEventPeopleRemoteStates(c CaseSpec, spec domainEventPeopleInput) (protocol.RemotePlayerStates, error) {
	if spec.ServerTick == nil {
		return protocol.RemotePlayerStates{}, fmt.Errorf("runtime-oracle: case %s requires a server tick", c.ID)
	}
	if spec.Players == nil {
		return protocol.RemotePlayerStates{}, fmt.Errorf("runtime-oracle: case %s requires a player batch", c.ID)
	}
	players := make([]protocol.RemotePlayerState, 0, len(spec.Players))
	for index, record := range spec.Players {
		playerID, err := domainEventPeoplePlayerID(c, fmt.Sprintf("players[%d].player_id", index), &record.PlayerID)
		if err != nil {
			return protocol.RemotePlayerStates{}, err
		}
		position, err := domainEventPeopleVec3(c, fmt.Sprintf("players[%d].position", index), record.Position)
		if err != nil {
			return protocol.RemotePlayerStates{}, err
		}
		yaw, err := domainEventPeopleAngle(c, fmt.Sprintf("players[%d].yaw", index), &record.Yaw)
		if err != nil {
			return protocol.RemotePlayerStates{}, err
		}
		pitch, err := domainEventPeopleAngle(c, fmt.Sprintf("players[%d].pitch", index), &record.Pitch)
		if err != nil {
			return protocol.RemotePlayerStates{}, err
		}
		players = append(players, protocol.RemotePlayerState{
			PlayerID:  playerID,
			Dimension: core.DimensionID(record.Dimension),
			Position:  position,
			Yaw:       yaw,
			Pitch:     pitch,
			Reset:     record.Reset,
		})
	}
	return protocol.RemotePlayerStates{ServerTick: *spec.ServerTick, Players: players}, nil
}

// domainEventPeopleCompanionSpawn resolves one companion spawn from its frozen
// envelope, under the companion DTO's own field names.
func domainEventPeopleCompanionSpawn(c CaseSpec, spec domainEventPeopleInput) (protocol.CompanionSpawn, error) {
	id, err := domainEventPeopleCompanionID(c, "id", spec.ID)
	if err != nil {
		return protocol.CompanionSpawn{}, err
	}
	if spec.Name == nil {
		return protocol.CompanionSpawn{}, fmt.Errorf("runtime-oracle: case %s requires a name", c.ID)
	}
	if spec.Tick == nil {
		return protocol.CompanionSpawn{}, fmt.Errorf("runtime-oracle: case %s requires a tick", c.ID)
	}
	if spec.Dimension == nil {
		return protocol.CompanionSpawn{}, fmt.Errorf("runtime-oracle: case %s requires a dimension", c.ID)
	}
	position, err := domainEventPeopleVec3(c, "position", spec.Position)
	if err != nil {
		return protocol.CompanionSpawn{}, err
	}
	yaw, err := domainEventPeopleAngle(c, "yaw", spec.Yaw)
	if err != nil {
		return protocol.CompanionSpawn{}, err
	}
	pitch, err := domainEventPeopleAngle(c, "pitch", spec.Pitch)
	if err != nil {
		return protocol.CompanionSpawn{}, err
	}
	return protocol.CompanionSpawn{
		ID:        id,
		Name:      *spec.Name,
		Tick:      *spec.Tick,
		Dimension: core.DimensionID(*spec.Dimension),
		Position:  position,
		Yaw:       yaw,
		Pitch:     pitch,
	}, nil
}

// domainEventPeopleCompanionStates resolves one companion state batch from its
// frozen envelope.
func domainEventPeopleCompanionStates(c CaseSpec, spec domainEventPeopleInput) (protocol.CompanionStates, error) {
	if spec.Tick == nil {
		return protocol.CompanionStates{}, fmt.Errorf("runtime-oracle: case %s requires a tick", c.ID)
	}
	if spec.States == nil {
		return protocol.CompanionStates{}, fmt.Errorf("runtime-oracle: case %s requires a state batch", c.ID)
	}
	states := make([]protocol.CompanionState, 0, len(spec.States))
	for index, record := range spec.States {
		id, err := domainEventPeopleCompanionID(c, fmt.Sprintf("states[%d].id", index), &record.ID)
		if err != nil {
			return protocol.CompanionStates{}, err
		}
		position, err := domainEventPeopleVec3(c, fmt.Sprintf("states[%d].position", index), record.Position)
		if err != nil {
			return protocol.CompanionStates{}, err
		}
		yaw, err := domainEventPeopleAngle(c, fmt.Sprintf("states[%d].yaw", index), &record.Yaw)
		if err != nil {
			return protocol.CompanionStates{}, err
		}
		pitch, err := domainEventPeopleAngle(c, fmt.Sprintf("states[%d].pitch", index), &record.Pitch)
		if err != nil {
			return protocol.CompanionStates{}, err
		}
		states = append(states, protocol.CompanionState{
			ID:        id,
			Dimension: core.DimensionID(record.Dimension),
			Position:  position,
			Yaw:       yaw,
			Pitch:     pitch,
			Reset:     record.Reset,
		})
	}
	return protocol.CompanionStates{Tick: *spec.Tick, States: states}, nil
}

// domainEventPeopleFinish records one admission.
//
// The classification is cross-checked against the authority's verdict, a
// rejection publishes the broken rule beside its category, and an admitted
// record publishes its normalized field map. No codec round trip is recorded:
// this family pins the semantic field values, and the wire layout of these
// packets belongs to the protocol nodes that port the codecs.
func domainEventPeopleFinish(c CaseSpec, subject string, fields map[string]any, category, rule string, admitted bool) (Outcome, []byte, error) {
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

// domainEventPeopleNormalizeRemoteSpawn renders one remote-player spawn in the
// normalized field map. The publish tick is a decimal string, the two angles
// are the hexadecimal IEEE-754 bit strings the corpus vocabulary uses, and the
// flattened yaw and pitch are published as one `look` member because that is
// the domain record's field.
func domainEventPeopleNormalizeRemoteSpawn(spawn protocol.RemotePlayerSpawn) map[string]any {
	return map[string]any{
		"player_id":    domainEventPeoplePlayerText(spawn.PlayerID),
		"display_name": spawn.DisplayName,
		"server_tick":  strconv.FormatUint(spawn.ServerTick, 10),
		"dimension":    int(spawn.Dimension),
		"position":     domainEventPeopleVectorBits(spawn.Position),
		"look":         domainEventPeopleLookBits(spawn.Yaw, spawn.Pitch),
	}
}

// domainEventPeopleNormalizeCompanionSpawn renders one companion spawn in the
// normalized field map, under the domain record's field names.
func domainEventPeopleNormalizeCompanionSpawn(spawn protocol.CompanionSpawn) map[string]any {
	return map[string]any{
		"id":          domainEventPeopleCompanionText(spawn.ID),
		"name":        spawn.Name,
		"server_tick": strconv.FormatUint(spawn.Tick, 10),
		"dimension":   int(spawn.Dimension),
		"position":    domainEventPeopleVectorBits(spawn.Position),
		"look":        domainEventPeopleLookBits(spawn.Yaw, spawn.Pitch),
	}
}

// domainEventPeopleNormalizeRemoteStates renders one remote-player state batch
// in the normalized field map. Every record is published, so a replay proves
// the exact sequence the record carried, and the batch member is named `states`
// because that is the domain record's field name.
func domainEventPeopleNormalizeRemoteStates(states protocol.RemotePlayerStates) map[string]any {
	records := make([]map[string]any, 0, len(states.Players))
	for _, state := range states.Players {
		records = append(records, map[string]any{
			"player_id": domainEventPeoplePlayerText(state.PlayerID),
			"dimension": int(state.Dimension),
			"position":  domainEventPeopleVectorBits(state.Position),
			"look":      domainEventPeopleLookBits(state.Yaw, state.Pitch),
			"reset":     state.Reset,
		})
	}
	return map[string]any{
		"server_tick": strconv.FormatUint(states.ServerTick, 10),
		"states":      records,
	}
}

// domainEventPeopleNormalizeCompanionStates renders one companion state batch
// in the normalized field map, under the domain record's field names.
func domainEventPeopleNormalizeCompanionStates(states protocol.CompanionStates) map[string]any {
	records := make([]map[string]any, 0, len(states.States))
	for _, state := range states.States {
		records = append(records, map[string]any{
			"id":        domainEventPeopleCompanionText(state.ID),
			"dimension": int(state.Dimension),
			"position":  domainEventPeopleVectorBits(state.Position),
			"look":      domainEventPeopleLookBits(state.Yaw, state.Pitch),
			"reset":     state.Reset,
		})
	}
	return map[string]any{
		"server_tick": strconv.FormatUint(states.Tick, 10),
		"states":      records,
	}
}

// domainEventPeopleLookBits renders the flattened yaw and pitch as one look
// member, so a replay compares the domain record's field rather than two wire
// fields the domain does not carry.
func domainEventPeopleLookBits(yaw, pitch float32) map[string]any {
	return map[string]any{
		"yaw":   domainEventPeopleFloatBits(yaw),
		"pitch": domainEventPeopleFloatBits(pitch),
	}
}

// domainEventPeopleRemoteSpawnRule names the first rule one remote-player spawn
// breaks, in the same order the Go validator checks them: the name, the
// identity, the dimension and then the finite pose and rotation.
//
// A rejection classifies as `invalid-identity` for a UUID that is not a
// nonzero UUIDv4, as `invalid-enum` for an unknown dimension, and as
// `invalid-value` otherwise, which is the classification rule a Rust consumer
// of this family applies to its own rejection variants. No pitch bound is
// named, because the Go validator publishes none for a remote player.
func domainEventPeopleRemoteSpawnRule(spawn protocol.RemotePlayerSpawn) (string, string) {
	if category, rule := domainIdentityDisplayNameRule(spawn.DisplayName); rule != "" {
		return category, "remote_player_spawn." + rule
	}
	if category, rule := domainEventPeoplePlayerIDRule(spawn.PlayerID); rule != "" {
		return category, "remote_player_spawn." + rule
	}
	if spawn.Dimension != core.Overworld && spawn.Dimension != core.Depths {
		return "invalid-enum", "remote_player_spawn.dimension"
	}
	if !domainEventPeopleFiniteVec3(spawn.Position) {
		return "invalid-value", "remote_player_spawn.position_finite"
	}
	if !domainEventPeopleFinite32(spawn.Yaw) || !domainEventPeopleFinite32(spawn.Pitch) {
		return "invalid-value", "remote_player_spawn.rotation_finite"
	}
	return "", ""
}

// domainEventPeopleCompanionSpawnRule names the first rule one companion spawn
// breaks, in the same order the Go validator checks them: the identity, the
// name, the overworld-only dimension, and then the finite pose and the
// inclusive vertical look range.
func domainEventPeopleCompanionSpawnRule(spawn protocol.CompanionSpawn) (string, string) {
	if category, rule := domainEventPeoplePlayerIDRule(core.PlayerID(spawn.ID)); rule != "" {
		return category, "companion_spawn." + rule
	}
	if category, rule := domainIdentityCompanionNameRule(spawn.Name); rule != "" {
		return category, "companion_spawn." + rule
	}
	if spawn.Dimension != core.Overworld {
		return "invalid-enum", "companion_spawn.dimension"
	}
	if !domainEventPeopleFiniteVec3(spawn.Position) {
		return "invalid-value", "companion_spawn.position_finite"
	}
	if !domainEventPeopleFinite32(spawn.Yaw) || !domainEventPeopleFinite32(spawn.Pitch) {
		return "invalid-value", "companion_spawn.rotation_finite"
	}
	if category, rule := domainEventPeopleCompanionPitchRule(spawn.Pitch); rule != "" {
		return category, "companion_spawn." + rule
	}
	return "", ""
}

// domainEventPeopleRemoteStatesRule names the first rule one remote-player state
// batch breaks, in the same order the Go validator checks them: the batch count
// bound, then every record in batch order, then the strict identity order.
//
// The count bound names both ends because the Go validator rejects a count
// below one and above seven; the upper end is a wire budget the domain does not
// carry, so no case in this family sits above it and the rule is only reachable
// through the count a case does name.
func domainEventPeopleRemoteStatesRule(states protocol.RemotePlayerStates) (string, string) {
	if len(states.Players) < 1 || len(states.Players) > domainEventPeopleRemoteBatchMax {
		return "invalid-value", "remote_player_states.count_range"
	}
	for index, state := range states.Players {
		if category, rule := domainEventPeopleRemoteStateRule(state); rule != "" {
			return category, fmt.Sprintf("remote_player_states.player_%d.%s", index, rule)
		}
		if index > 0 && bytes.Compare(states.Players[index-1].PlayerID[:], state.PlayerID[:]) >= 0 {
			return "invalid-value", "remote_player_states.strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventPeopleRemoteStateRule names the first rule one remote-player state
// record breaks: the identity, the dimension and then the finite pose and
// rotation. No pitch bound is named, exactly as on the spawn.
func domainEventPeopleRemoteStateRule(state protocol.RemotePlayerState) (string, string) {
	if category, rule := domainEventPeoplePlayerIDRule(state.PlayerID); rule != "" {
		return category, rule
	}
	if state.Dimension != core.Overworld && state.Dimension != core.Depths {
		return "invalid-enum", "dimension"
	}
	if !domainEventPeopleFiniteVec3(state.Position) {
		return "invalid-value", "position_finite"
	}
	if !domainEventPeopleFinite32(state.Yaw) || !domainEventPeopleFinite32(state.Pitch) {
		return "invalid-value", "rotation_finite"
	}
	return "", ""
}

// domainEventPeopleCompanionStatesRule names the first rule one companion state
// batch breaks, in the same order the Go validator checks them: the batch count
// bound, then every record in batch order, then the strict identity order.
func domainEventPeopleCompanionStatesRule(states protocol.CompanionStates) (string, string) {
	if len(states.States) < 1 || len(states.States) > protocol.MaxCompanionStates {
		return "invalid-value", "companion_states.count_range"
	}
	for index, state := range states.States {
		if category, rule := domainEventPeopleCompanionStateRule(state); rule != "" {
			return category, fmt.Sprintf("companion_states.state_%d.%s", index, rule)
		}
		if index > 0 && bytes.Compare(states.States[index-1].ID[:], state.ID[:]) >= 0 {
			return "invalid-value", "companion_states.strictly_increasing_ids"
		}
	}
	return "", ""
}

// domainEventPeopleCompanionStateRule names the first rule one companion state
// record breaks: the identity, the overworld-only dimension, the finite pose
// and rotation, and the inclusive vertical look range.
func domainEventPeopleCompanionStateRule(state protocol.CompanionState) (string, string) {
	if category, rule := domainEventPeoplePlayerIDRule(core.PlayerID(state.ID)); rule != "" {
		return category, rule
	}
	if state.Dimension != core.Overworld {
		return "invalid-enum", "dimension"
	}
	if !domainEventPeopleFiniteVec3(state.Position) {
		return "invalid-value", "position_finite"
	}
	if !domainEventPeopleFinite32(state.Yaw) || !domainEventPeopleFinite32(state.Pitch) {
		return "invalid-value", "rotation_finite"
	}
	if category, rule := domainEventPeopleCompanionPitchRule(state.Pitch); rule != "" {
		return category, rule
	}
	return "", ""
}

// domainEventPeopleCompanionPitchRule names the inclusive vertical look range
// one companion pitch has to stay inside. The comparison is on the exact float32
// the wire carries, so the limit itself is admitted and the next representable
// value above or below it is not.
func domainEventPeopleCompanionPitchRule(pitch float32) (string, string) {
	if pitch < -domainEventPeoplePitchLimit || pitch > domainEventPeoplePitchLimit {
		return "invalid-value", "pitch_vertical_look_range"
	}
	return "", ""
}

// domainEventPeoplePlayerIDRule names the first rule one raw identity breaks,
// read from the same Go `core.PlayerID.Valid` gate the protocol validators
// apply. The companion identity is validated by the same rule, so this one
// classifier covers both identity types.
func domainEventPeoplePlayerIDRule(id core.PlayerID) (string, string) {
	if id == (core.PlayerID{}) {
		return "invalid-identity", "player_id.nonzero"
	}
	if id[6]>>4 != 4 {
		return "invalid-identity", "player_id.version_v4"
	}
	if id[8]&0xc0 != 0x80 {
		return "invalid-identity", "player_id.variant_rfc4122"
	}
	return "", ""
}

// domainEventPeopleFinite32 reports whether one float is neither NaN nor an
// infinity, which is the Go protocol package's private `finite32` predicate.
//
// The producer keeps this copy because the predicate is unexported, and the
// copy is pinned by the same non-finite boundary cases the domain test pins.
func domainEventPeopleFinite32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

// domainEventPeopleFiniteVec3 reports whether every component of one vector is
// finite, which is the Go protocol package's private `finiteVec3` predicate.
func domainEventPeopleFiniteVec3(value mgl32.Vec3) bool {
	for _, component := range value {
		if !domainEventPeopleFinite32(component) {
			return false
		}
	}
	return true
}

// domainEventPeoplePitchAboveLimit is the value one representable step above
// the inclusive companion limit, so the bound is pinned at the exact float the
// Go comparison rejects rather than at a nearby decimal.
func domainEventPeoplePitchAboveLimit() float32 {
	return float32(math.Nextafter32(domainEventPeoplePitchLimit, float32(math.Inf(1))))
}

// domainEventPeoplePitchBelowLimit is the value one representable step below the
// inclusive companion limit.
func domainEventPeoplePitchBelowLimit() float32 {
	return float32(math.Nextafter32(-domainEventPeoplePitchLimit, float32(math.Inf(-1))))
}

// domainEventPeoplePlayerText renders one player identity as the 32 lowercase
// hex digits the corpus input and the normalized field map both use.
func domainEventPeoplePlayerText(id core.PlayerID) string {
	return hex.EncodeToString(id[:])
}

// domainEventPeopleCompanionText renders one companion identity as the 32
// lowercase hex digits the corpus input and the normalized field map both use.
func domainEventPeopleCompanionText(id companion.ID) string {
	return hex.EncodeToString(id[:])
}

// domainEventPeoplePlayerVector resolves one identity text into the raw player
// bytes, for a case table that names an identity directly rather than through
// the frozen envelope.
func domainEventPeoplePlayerVector(text string) core.PlayerID {
	var id core.PlayerID
	if _, err := hex.Decode(id[:], []byte(text)); err != nil {
		panic(fmt.Sprintf("runtime-oracle: case table names player identity %q: %v", text, err))
	}
	return id
}

// domainEventPeopleCompanionVector resolves one identity text into the raw
// companion bytes, for a case table that names an identity directly.
func domainEventPeopleCompanionVector(text string) companion.ID {
	var id companion.ID
	if _, err := hex.Decode(id[:], []byte(text)); err != nil {
		panic(fmt.Sprintf("runtime-oracle: case table names companion identity %q: %v", text, err))
	}
	return id
}

// domainEventPeopleVectorBits renders one f32 vector as the three hexadecimal
// IEEE-754 bit strings the normalized corpus vocabulary uses.
func domainEventPeopleVectorBits(vector mgl32.Vec3) []string {
	return []string{
		domainEventPeopleFloatBits(vector.X()),
		domainEventPeopleFloatBits(vector.Y()),
		domainEventPeopleFloatBits(vector.Z()),
	}
}

// domainEventPeopleFloatBits renders one float32 as the eight-digit hexadecimal
// IEEE-754 bit string the normalized corpus vocabulary uses.
func domainEventPeopleFloatBits(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

// domainEventPeopleFloatText renders one float32 as decimal float text, so NaN,
// the infinities and a signed zero stay expressible in the frozen input. The
// normalized outcome renders the same value as its IEEE-754 bit string, so the
// input and the outcome pin the value in two independent renderings.
func domainEventPeopleFloatText(value float32) string {
	return strconv.FormatFloat(float64(value), 'g', -1, 32)
}

// domainEventPeopleFloatTextPointer renders one optional float32 field as
// decimal float text, keeping an absent field distinguishable from a zero.
func domainEventPeopleFloatTextPointer(value float32) *string {
	text := domainEventPeopleFloatText(value)
	return &text
}

// domainEventPeopleVectorText renders one vector as its three decimal float
// texts.
func domainEventPeopleVectorText(vector mgl32.Vec3) []string {
	return []string{
		domainEventPeopleFloatText(vector.X()),
		domainEventPeopleFloatText(vector.Y()),
		domainEventPeopleFloatText(vector.Z()),
	}
}

// domainEventPeopleAngle resolves one angle field a case names as decimal float
// text.
func domainEventPeopleAngle(c CaseSpec, name string, text *string) (float32, error) {
	if text == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	value, err := strconv.ParseFloat(*text, 32)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not a float32", c.ID, name, *text)
	}
	return float32(value), nil
}

// domainEventPeopleVec3 resolves one position from its three decimal float
// texts.
func domainEventPeopleVec3(c CaseSpec, name string, components []string) (mgl32.Vec3, error) {
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

// domainEventPeopleDecodeInput reads one frozen corpus input and rejects
// trailing content, so a producer never executes bytes the case did not name.
func domainEventPeopleDecodeInput(data []byte) (domainEventPeopleInput, error) {
	var spec domainEventPeopleInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return domainEventPeopleInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainEventPeopleInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}

// domainEventPeopleRecord pairs one executed case with the input and outcome
// the producer produced for it.
type domainEventPeopleRecord struct {
	label   string
	input   domainEventPeopleInput
	outcome Outcome
}

// domainEventPeopleExecute runs the whole case table through the producer and
// returns one record per case in table order.
func domainEventPeopleExecute(t *testing.T) []domainEventPeopleRecord {
	t.Helper()

	cases := domainEventPeopleCases()
	records := make([]domainEventPeopleRecord, 0, len(cases))
	for _, entry := range cases {
		input, err := json.MarshalIndent(entry.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", entry.label, err)
		}
		input = append(input, '\n')
		outcome, _, err := runDomainEventPeople(CaseSpec{ID: domainEventPeopleCaseID(entry.label)}, input)
		if err != nil {
			t.Fatalf("execute case %s: %v", entry.label, err)
		}
		records = append(records, domainEventPeopleRecord{label: entry.label, input: entry.input, outcome: outcome})
	}
	return records
}

// domainEventPeopleSyncCorpus writes or verifies the frozen corpus files.
//
// An ordinary run is read-only: it proves the committed files still match what
// the protocol DTOs produce now, so a drifted artifact fails instead of being
// regenerated. The explicit update flag rewrites them, which is the only way a
// frozen artifact changes.
func domainEventPeopleSyncCorpus(t *testing.T, records []domainEventPeopleRecord) {
	t.Helper()

	root := mustRepoRoot(t)
	corpusDir := filepath.Join(root, filepath.FromSlash(domainEventPeopleCorpusRelDir))
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

	if *updateDomainEventPeopleCorpus {
		for relative, data := range want {
			target := filepath.Join(corpusDir, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create corpus directory: %v", err)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				t.Fatalf("write %s: %v", relative, err)
			}
		}
		return
	}

	for relative, data := range want {
		target := filepath.Join(corpusDir, filepath.FromSlash(relative))
		committed, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read frozen corpus case %s: %v (rerun with -update-domain-event-people-corpus after reviewing the change)", relative, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("frozen corpus case %s drifted from the executed protocol DTO (rerun with -update-domain-event-people-corpus after reviewing the change)", relative)
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

// domainEventPeopleCaseID renders the manifest case identity one corpus label
// carries. The version segment is the family's published version, because a
// case has to name the version of the family it belongs to.
func domainEventPeopleCaseID(label string) string {
	return domainEventPeopleFamily + "/" + domainEventPeopleVersion + "/" + label
}

// domainEventPeopleWorkingManifest assembles the manifest this node executes
// inside a harness-owned temporary directory.
//
// The frozen manifest carries the `domain.event` family and its player, world
// and inventory event cases, because the controller merges manifest fragments
// after acceptance. The working manifest is therefore a family-scoped
// selection: `Cases` holds exactly the remote-player and companion cases the
// committed corpus registers, every other family's case list is cleared because
// `Reconcile` requires each family's list to match the cases this selection
// registers for it, and the `domain.event` entry keeps its discovered identity —
// kind, role, versions, source, eventual owner and numeric semantics — while its
// provenance and case list are replaced with this producer's. Registering the
// family's merged provenance and case list in the live registry is deliberately
// left to the manifest merge.
func domainEventPeopleWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	cases := domainEventPeopleCorpusCases(t, root)

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
	sources := make([]SourceSpec, 0, len(domainEventPeopleFamilySources))
	for _, relative := range domainEventPeopleFamilySources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	registered := false
	for index := range cloned.Families {
		if cloned.Families[index].ID != domainEventPeopleFamily {
			cloned.Families[index].Cases = nil
			continue
		}
		cloned.Families[index].Sources = sources
		cloned.Families[index].Cases = caseIDs
		registered = true
	}
	if !registered {
		t.Fatalf("working manifest has no %s family", domainEventPeopleFamily)
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

// domainEventPeopleCorpusCases reads the frozen corpus and registers one case
// per committed input, with digests proven against the files on disk.
func domainEventPeopleCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(domainEventPeopleCorpusRelDir))
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
		t.Fatalf("walk event people corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no event people case under %s", domainEventPeopleCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		label := strings.TrimSuffix(filepath.ToSlash(relative), ".input.json")
		if label == "" || !domainEventPeopleLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug with an optional numeric suffix", filepath.ToSlash(relative), label)
		}
		envelope := domainEventPeopleReadEnvelope(t, input)
		if envelope.Consumer != domainEventPeopleConsumer {
			t.Fatalf("corpus case %s names consumer %q, want %q", label, envelope.Consumer, domainEventPeopleConsumer)
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
			ID:           domainEventPeopleCaseID(label),
			Family:       domainEventPeopleFamily,
			Version:      domainEventPeopleVersion,
			Operation:    domainEventPeopleOperation,
			Input:        AssetRef{Path: domainEventPeopleCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: domainEventPeopleCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: domainEventPeopleConsumer,
		})
	}
	return cases
}

// domainEventPeopleReadEnvelope reads the provenance envelope of one frozen
// corpus input.
func domainEventPeopleReadEnvelope(t *testing.T, path string) domainEventPeopleInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope domainEventPeopleInput
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// domainEventPeopleCorpusPath renders one absolute corpus path as the
// repository-relative slash path the manifest requires.
func domainEventPeopleCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}

// TestDomainEventPeopleOracleExecutesEveryCase runs the whole case table
// through the real Go protocol DTOs, proves the frozen corpus still matches
// what they produce, and then runs the same cases through the production
// runner so the executed evidence satisfies the completeness rules a published
// trace report does.
func TestDomainEventPeopleOracleExecutesEveryCase(t *testing.T) {
	records := domainEventPeopleExecute(t)
	domainEventPeopleSyncCorpus(t, records)

	root := mustRepoRoot(t)
	manifest := domainEventPeopleWorkingManifest(t, root)
	if len(manifest.Cases) != len(records) {
		t.Fatalf("working manifest registers %d cases, want %d (one per executed table row)", len(manifest.Cases), len(records))
	}
	domainEventPeopleAssertCaseSpecs(t, root, manifest)
	domainEventPeopleAssertOutcomesDistinguishCases(t, records)

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for _, obs := range observations {
		c := domainEventPeopleCaseByID(t, manifest, obs.CaseID)
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
		c := domainEventPeopleCaseByID(t, manifest, obs.CaseID)
		if obs.ExpectedDigest != c.Expected.SHA256 {
			t.Fatalf("observation for %s carries expected digest %s, want %s", obs.CaseID, obs.ExpectedDigest, c.Expected.SHA256)
		}
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event people evidence failed trace validation: %v", err)
	}
}

// TestDomainEventPeopleOracleCaseIdentitiesAreTheRustDomainConsumer pins the
// manifest identity of every event people case: the operation, the family
// version, the family and the consumer the change names for the Rust crate
// that owns these records.
func TestDomainEventPeopleOracleCaseIdentitiesAreTheRustDomainConsumer(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPeopleWorkingManifest(t, root)

	labels := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != domainEventPeopleOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, domainEventPeopleOperation)
		}
		if c.Version != domainEventPeopleVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, domainEventPeopleVersion)
		}
		if c.Family != domainEventPeopleFamily {
			t.Fatalf("case %s declares family %q, want %q", c.ID, c.Family, domainEventPeopleFamily)
		}
		if c.RustConsumer != domainEventPeopleConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, domainEventPeopleConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		prefix := c.Family + "/" + c.Version + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !domainEventPeopleLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug with an optional numeric suffix", c.ID, label)
		}
		if labels[label] {
			t.Fatalf("label %q is registered twice", label)
		}
		labels[label] = true

		envelope := domainEventPeopleReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != domainEventPeopleConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, domainEventPeopleConsumer)
		}
		switch envelope.Rule {
		case domainEventPeopleRuleRemoteSpawn, domainEventPeopleRuleRemoteDespawn,
			domainEventPeopleRuleRemoteStates, domainEventPeopleRuleCompanionSpawn,
			domainEventPeopleRuleCompanionStates, domainEventPeopleRuleCompanionDespawn:
		default:
			t.Fatalf("case %s names rule %q, which is not a remote-player or companion observation rule", c.ID, envelope.Rule)
		}
	}
}

// TestDomainEventPeopleOracleWorkingManifestDescribesItself proves the working
// manifest is internally consistent: every case passes the production case
// validation, the family's provenance hashes match disk, and the family's case
// list matches exactly the cases the selection registers.
func TestDomainEventPeopleOracleWorkingManifestDescribesItself(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPeopleWorkingManifest(t, root)
	domainEventPeopleAssertCaseSpecs(t, root, manifest)

	family, ok := domainEventPeopleFamilySpec(manifest)
	if !ok {
		t.Fatalf("working manifest has no %s family", domainEventPeopleFamily)
	}
	if family.Role != "event" || family.Kind != "domain" {
		t.Fatalf("%s declares kind %q role %q, want domain event", domainEventPeopleFamily, family.Kind, family.Role)
	}
	if family.CurrentVersion != domainEventPeopleVersion {
		t.Fatalf("%s declares version %q, want %q", domainEventPeopleFamily, family.CurrentVersion, domainEventPeopleVersion)
	}
	if len(family.Sources) != len(domainEventPeopleFamilySources) {
		t.Fatalf("%s records %d provenance sources, want %d", domainEventPeopleFamily, len(family.Sources), len(domainEventPeopleFamilySources))
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
		t.Fatalf("%s lists %d cases, want %d", domainEventPeopleFamily, len(family.Cases), len(registered))
	}
	for _, id := range family.Cases {
		if !registered[id] {
			t.Fatalf("%s lists case %s which the selection does not register", domainEventPeopleFamily, id)
		}
	}
}

// TestDomainEventPeopleOracleOutcomesDistinguishAcceptedAndRejected pins that
// the producer is not returning one constant answer, that every rule records
// both an admitted record and a rejection, and that the boundaries the Rust
// event people test enumerates are each present in the executed evidence.
func TestDomainEventPeopleOracleOutcomesDistinguishAcceptedAndRejected(t *testing.T) {
	records := domainEventPeopleExecute(t)
	domainEventPeopleAssertOutcomesDistinguishCases(t, records)

	tally := make(map[string]*domainEventPeopleRuleTally)
	for _, record := range records {
		entry, ok := tally[record.input.Rule]
		if !ok {
			entry = &domainEventPeopleRuleTally{}
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

	if !domainEventPeopleFieldsContain(tally[domainEventPeopleRuleRemoteSpawn].accepted, "server_tick", "0") {
		t.Fatal("no admitted remote-player spawn carries a zero server tick")
	}
	if !domainEventPeopleFieldsContain(tally[domainEventPeopleRuleRemoteSpawn].accepted, "dimension", 1) {
		t.Fatal("no admitted remote-player spawn carries the depths dimension")
	}
	if !domainEventPeopleNestedContains(tally[domainEventPeopleRuleRemoteSpawn].accepted, "look", "pitch", domainEventPeopleFloatBits(4)) {
		t.Fatal("no admitted remote-player spawn carries a pitch outside the companion vertical look range")
	}
	if !domainEventPeopleFieldsContain(tally[domainEventPeopleRuleCompanionSpawn].accepted, "server_tick", "0") {
		t.Fatal("no admitted companion spawn carries a zero tick")
	}
	for _, pitch := range []float32{-domainEventPeoplePitchLimit, domainEventPeoplePitchLimit} {
		if !domainEventPeopleNestedContains(tally[domainEventPeopleRuleCompanionSpawn].accepted, "look", "pitch", domainEventPeopleFloatBits(pitch)) {
			t.Fatalf("no admitted companion spawn carries the inclusive vertical look limit %v", pitch)
		}
	}
	if !domainEventPeopleNestedContains(tally[domainEventPeopleRuleRemoteStates].accepted, "states", "reset", true) ||
		!domainEventPeopleNestedContains(tally[domainEventPeopleRuleCompanionStates].accepted, "states", "reset", false) {
		t.Fatal("no admitted state batch carries the per-record replay reset bit")
	}
	// Every boundary below is looked up by the rule name the classifier
	// publishes, because the identity, name, dimension and pitch rules all
	// reject through the record's own classifier rather than through one
	// top-level field.
	for _, rule := range []string{
		"remote_player_spawn.dimension",
		"remote_player_states.player_1.dimension",
		"companion_spawn.dimension",
		"companion_states.state_1.dimension",
		"remote_player_spawn.player_id.nonzero",
		"remote_player_spawn.player_id.version_v4",
		"remote_player_spawn.display_name.canonical_trim",
		"remote_player_states.player_0.player_id.nonzero",
		"remote_player_states.strictly_increasing_ids",
		"remote_player_states.count_range",
		"companion_spawn.player_id.nonzero",
		"companion_spawn.companion_name.embedded_whitespace",
		"companion_spawn.pitch_vertical_look_range",
		"companion_states.state_0.pitch_vertical_look_range",
		"companion_states.strictly_increasing_ids",
		"companion_states.count_range",
	} {
		if !domainEventPeopleRejectionNamesRule(tally, rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", rule)
		}
	}
}

// TestDomainEventPeopleOracleRunnerRejectsUnregisteredFamily pins that a case
// cannot pass by being ignored.
func TestDomainEventPeopleOracleRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPeopleWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "domain.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family domain.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestDomainEventPeopleOracleRunnerRejectsOperationFamilyMismatch pins that a
// case cannot declare one operation and be executed by another.
func TestDomainEventPeopleOracleRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPeopleWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "decode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation-family mismatch failure, got: %v", err)
	}
}

// TestDomainEventPeopleOracleRunnerRejectsTamperedInput pins that a case whose
// committed bytes no longer match their recorded digest fails the run instead
// of executing bytes the manifest does not name.
func TestDomainEventPeopleOracleRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPeopleWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestDomainEventPeopleOracleReportPublishesAndValidates assembles the executed
// evidence into a report, publishes it through the production atomic exporter,
// reloads it and validates it against the working manifest. This is the
// identity and content check a later Rust acceptance step performs; it does not
// claim any Rust behaviour.
func TestDomainEventPeopleOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPeopleWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event people evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, domainEventPeopleCorpusReportName)
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

// TestDomainEventPeopleOracleRejectsMissingProducerTest pins that the executed
// evidence has a real producer behind it, including the topic-named entry point
// the domain plan's filter selects. A corpus whose producer test is gone has no
// independent execution, only frozen files, and a missing topic entry point
// would make the plan filter pass without running anything.
func TestDomainEventPeopleOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(domainEventPeopleProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", domainEventPeopleProducerTestRelPath, err)
	}
	for _, name := range []string{domainEventPeopleProducerExecuteName, domainEventPeopleProducerTopicName} {
		declaration := "func " + name + "(t *testing.T) {"
		if !strings.Contains(string(data), declaration) {
			t.Fatalf("%s does not declare %s", domainEventPeopleProducerTestRelPath, declaration)
		}
	}
}

// TestDomainOracle_event_people is the topic-named entry point the domain plan
// names for this node. It delegates to the same executed table, so the two
// filters select one source of expected results rather than two.
func TestDomainOracle_event_people(t *testing.T) {
	TestDomainEventPeopleOracleExecutesEveryCase(t)
}

// domainEventPeopleRuleTally collects the accepted and rejected normalized
// outcomes one rule records, so a boundary assertion can look for the field
// value it names.
type domainEventPeopleRuleTally struct {
	accepted []map[string]any
	rejected []map[string]any
}

// domainEventPeopleFieldsContain reports whether one recorded outcome set holds
// a record whose named field carries the value a boundary assertion names.
func domainEventPeopleFieldsContain(records []map[string]any, field string, want any) bool {
	for _, fields := range records {
		if reflect.DeepEqual(fields[field], want) {
			return true
		}
	}
	return false
}

// domainEventPeopleNestedContains reports whether one recorded outcome set holds
// a record whose named member carries an entry with the named field value. A
// look member and a state array both live inside their record's map, so a
// boundary that names one of their fields looks inside the member rather than
// at the record's top level. The member type is the producer's own in-memory
// rendering, because the executed records have not been decoded back from JSON.
func domainEventPeopleNestedContains(records []map[string]any, member, field string, want any) bool {
	for _, fields := range records {
		switch value := fields[member].(type) {
		case map[string]any:
			if reflect.DeepEqual(value[field], want) {
				return true
			}
		case []map[string]any:
			for _, item := range value {
				if reflect.DeepEqual(item[field], want) {
					return true
				}
			}
		}
	}
	return false
}

// domainEventPeopleRejectionNamesRule reports whether any recorded rejection
// names the broken rule a boundary assertion pins.
func domainEventPeopleRejectionNamesRule(tally map[string]*domainEventPeopleRuleTally, rule string) bool {
	for _, entry := range tally {
		for _, fields := range entry.rejected {
			if fields["rule"] == rule {
				return true
			}
		}
	}
	return false
}

// domainEventPeopleAssertCaseSpecs runs the production case validation over the
// whole selection, so a case with a bad path, digest, format or checkpoint
// fails here rather than surfacing later as a confusing coverage failure.
func domainEventPeopleAssertCaseSpecs(t *testing.T, root string, manifest Inventory) {
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

// domainEventPeopleAssertOutcomesDistinguishCases proves the executed evidence
// separates admitted records from rejections instead of publishing one constant
// answer, and that the rejections name a rule the authority publishes.
func domainEventPeopleAssertOutcomesDistinguishCases(t *testing.T, records []domainEventPeopleRecord) {
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

// domainEventPeopleFamilySpec indexes the working manifest by family identity.
func domainEventPeopleFamilySpec(manifest Inventory) (Family, bool) {
	for _, family := range manifest.Families {
		if family.ID == domainEventPeopleFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainEventPeopleCaseByID indexes a manifest selection by case identity.
func domainEventPeopleCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}
