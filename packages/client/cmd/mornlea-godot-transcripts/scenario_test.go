package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// committedScenarioPath resolves the terrain scenario checked in under the
// shared transcript corpus directory. The scenario lives in a dedicated
// subdirectory because the recorded-corpus pinning test enumerates the
// top-level transcript files exactly.
func committedScenarioPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "..", "testdata", "godot-pilot", "transcripts",
		"terrain", "terrain-scenario.json")
}

// encodeStage deterministically serializes one synthesized packet through
// the production codec, which is the same encoder the replay loop's stream
// uses; equal bytes therefore prove reproducible wire output.
func encodeStage(t *testing.T, codec *network.Codec, packet protocol.ServerPacket) []byte {
	t.Helper()
	id, encoded, err := codec.EncodeServer(protocol.StatePlay, packet)
	if err != nil {
		t.Fatalf("encode %T: %v", packet, err)
	}
	_ = id
	return encoded
}

func TestCommittedScenarioDecodesAndStaysDeterministic(t *testing.T) {
	scenario, err := loadScenario(committedScenarioPath(t))
	if err != nil {
		t.Fatalf("load committed scenario: %v", err)
	}
	if scenario.Name != "terrain-scenario" {
		t.Fatalf("scenario name = %q", scenario.Name)
	}
	if len(scenario.Rounds) != 2 {
		t.Fatalf("scenario rounds = %d, want 2 (first entry and re-entry)", len(scenario.Rounds))
	}
	first := scenario.Rounds[0]
	kinds := make([]string, 0, len(first.Stages))
	for _, stage := range first.Stages {
		kinds = append(kinds, stage.Send)
	}
	want := []string{stagePlayerState, stageChunkSnapshot, stageBlockChanges, stageForgetChunks, stageDisconnect}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("first round stages = %v, want %v", kinds, want)
	}

	// The ready state carries a strictly positive tick and the ready flag;
	// the runtime would drop a zero-tick authoritative state, so the pin
	// keeps that contract visible.
	var readyStage scenarioStage
	for _, stage := range scenario.Rounds[0].Stages {
		if stage.Send == stagePlayerState {
			readyStage = stage
		}
	}
	ready, err := synthesizeStage(scenario, readyStage)
	if err != nil {
		t.Fatalf("synthesize ready state: %v", err)
	}
	readyState, ok := ready.(protocol.PlayerState)
	if !ok {
		t.Fatalf("ready stage synthesized %T", ready)
	}
	if !readyState.Ready || readyState.ServerTick == 0 {
		t.Fatalf("ready state is not playable: %+v", readyState)
	}
	if readyState.Position != scenario.PlayerPosition {
		t.Fatalf("ready position %v, want %v", readyState.Position, scenario.PlayerPosition)
	}

	// Determinism: synthesizing and encoding the same stage twice yields
	// identical wire bytes, which is what makes the helper reproducible.
	codec, err := network.NewCodec()
	if err != nil {
		t.Fatalf("codec: %v", err)
	}
	t.Cleanup(func() {
		if err := codec.Close(); err != nil {
			t.Errorf("close codec: %v", err)
		}
	})
	for _, round := range scenario.Rounds {
		for _, stage := range round.Stages {
			left, leftErr := synthesizeStage(scenario, stage)
			right, rightErr := synthesizeStage(scenario, stage)
			if leftErr != nil || rightErr != nil {
				t.Fatalf("stage %q synthesis error: %v / %v", stage.Label, leftErr, rightErr)
			}
			leftBytes := encodeStage(t, codec, left.(protocol.ServerPacket))
			rightBytes := encodeStage(t, codec, right.(protocol.ServerPacket))
			if !bytes.Equal(leftBytes, rightBytes) {
				t.Fatalf("stage %q synthesis is not deterministic", stage.Label)
			}
		}
	}
}

func TestCommittedScenarioPinsTheExpectedSectionContent(t *testing.T) {
	scenario, err := loadScenario(committedScenarioPath(t))
	if err != nil {
		t.Fatalf("load committed scenario: %v", err)
	}
	var snapshotStage, deltaStage scenarioStage
	for _, stage := range scenario.Rounds[0].Stages {
		switch stage.Send {
		case stageChunkSnapshot:
			snapshotStage = stage
		case stageBlockChanges:
			deltaStage = stage
		}
	}
	packet, err := synthesizeStage(scenario, snapshotStage)
	if err != nil {
		t.Fatalf("synthesize initial snapshot: %v", err)
	}
	snapshot, ok := packet.(protocol.ChunkSnapshot)
	if !ok {
		t.Fatalf("initial snapshot synthesized %T", packet)
	}
	if snapshot.Dimension != core.Overworld || snapshot.Chunk != (core.ChunkPos{X: 0, Z: 0}) {
		t.Fatalf("snapshot identity = dim %d chunk %v", snapshot.Dimension, snapshot.Chunk)
	}
	if snapshot.Revision != 1 {
		t.Fatalf("snapshot revision = %d, want 1", snapshot.Revision)
	}
	if len(snapshot.Sections) != core.SectionsPerChunk {
		t.Fatalf("snapshot sections = %d", len(snapshot.Sections))
	}
	// The deterministic section content: section 0 is stone, the rest air,
	// so the floor section meshes to one visible upsert while the air
	// sections above mesh to empty-drop operations.
	if snapshot.Sections[0].Single != core.StoneID {
		t.Fatalf("section 0 block = %d, want stone %d", snapshot.Sections[0].Single, core.StoneID)
	}
	for index, section := range snapshot.Sections[1:] {
		if section.Single != core.AirID {
			t.Fatalf("section %d block = %d, want air", index+1, section.Single)
		}
	}

	// The delta places one stone block in section Y=1 of the same chunk,
	// directly on top of the floor section's top layer.
	delta, err := synthesizeStage(scenario, deltaStage)
	if err != nil {
		t.Fatalf("synthesize delta: %v", err)
	}
	changes, ok := delta.(protocol.BlockChanges)
	if !ok {
		t.Fatalf("delta synthesized %T", delta)
	}
	if changes.BaseRevision != 1 || changes.NewRevision != 2 || len(changes.Changes) != 1 {
		t.Fatalf("delta identity = %+v", changes)
	}
	placed := changes.Changes[0]
	if placed.Block != core.StoneID {
		t.Fatalf("delta block = %d, want stone", placed.Block)
	}
	if placed.Position.Section().Y != 1 {
		t.Fatalf("delta position %v is not in section Y=1", placed.Position)
	}
	if changes.Chunk != (core.ChunkPos{X: 0, Z: 0}) {
		t.Fatalf("delta chunk = %v", changes.Chunk)
	}
}

func TestScenarioSchemaRejectsDrift(t *testing.T) {
	scenario, err := loadScenario(committedScenarioPath(t))
	if err != nil {
		t.Fatalf("load committed scenario: %v", err)
	}
	encoded, err := json.Marshal(scenario)
	if err != nil {
		t.Fatalf("re-encode scenario: %v", err)
	}
	mutations := []struct {
		name string
		data []byte
	}{
		{"unknown field", []byte(`{"schema_version":1,"name":"x","description":"x","stage_interval_ms":100,"world_seed":1,"player_position":[0,0,0],"rounds":[],"unexpected":true}`)},
		{"trailing JSON", append(append([]byte(nil), encoded...), []byte(" {}")...)},
		{"wrong schema version", bytes.Replace(encoded, []byte(`"schema_version":1`), []byte(`"schema_version":2`), 1)},
		{"unknown send kind", bytes.Replace(encoded, []byte(`"send":"player_state"`), []byte(`"send":"teleport"`), 1)},
		{"empty rounds", bytes.Replace(encoded, []byte(`"rounds":[`), []byte(`"rounds":[],"x":[`), 1)},
	}
	for _, mutation := range mutations {
		if _, err := decodeScenario(mutation.data); err == nil {
			t.Fatalf("mutation %q was accepted", mutation.name)
		}
	}

	// The zero-revision snapshot is the one content violation the matrix
	// cannot build from a re-encoded file (json.Marshal of a zero revision
	// requires editing the struct), so mutate the decoded value directly.
	scenario.Rounds[0].Stages[1].Revision = 0
	if err := validateScenario(scenario); err == nil {
		t.Fatal("zero snapshot revision was accepted")
	}
}
