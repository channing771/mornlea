// This file owns the terrain scenario schema: a transcript-family JSON
// corpus file whose frames are declared semantically instead of as recorded
// bytes. The existing corpus under testdata/godot-pilot/transcripts pins
// recorded wire bytes frame by frame; terrain scenarios instead declare what
// the server should publish (chunks, block deltas, forgets, disconnects), and
// the replay half synthesizes protocol-legal packets from those declarations
// through the shared codec packages. Both families share the strict-decoding
// discipline: unknown fields and trailing JSON are hard errors, so a drifted
// scenario file fails the helper before any client connects.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// Stage kind names the scenario vocabulary. Each kind names one server-side
// play-state packet plus the login handshake the helper always performs; no
// other kind exists, so the replay loop is total over this set.
const (
	stagePlayerState   = "player_state"
	stageChunkSnapshot = "chunk_snapshot"
	stageBlockChanges  = "block_changes"
	stageForgetChunks  = "forget_chunks"
	stageDisconnect    = "disconnect"
)

// sectionLayoutStoneFloor is the single deterministic section layout the
// terrain scenario uses: section Y=0 filled with stone, every other section
// air. The layout guarantees the floor section meshes to a visible upsert
// while the air sections above mesh to empty (drop) operations, which is
// exactly the upsert/drop mix the structural terrain summary observes.
const sectionLayoutStoneFloor = "stone_floor"

// terrainScenario is the decoded corpus file. The rounds run sequentially:
// each round accepts exactly one client connection, performs the v44 server
// login, and replays its stages in order.
type terrainScenario struct {
	SchemaVersion   int             `json:"schema_version"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	StageIntervalMS int             `json:"stage_interval_ms"`
	WorldSeed       uint64          `json:"world_seed"`
	PlayerPosition  [3]float32      `json:"player_position"`
	Rounds          []scenarioRound `json:"rounds"`
}

// scenarioRound is one single-connection serving round.
type scenarioRound struct {
	Name   string          `json:"name"`
	Stages []scenarioStage `json:"stages"`
}

// scenarioStage declares one server packet semantically. Which fields are
// required depends on the kind; validateScenario pins the full matrix.
type scenarioStage struct {
	Label        string               `json:"label"`
	Send         string               `json:"send"`
	Dimension    uint32               `json:"dimension"`
	Chunk        [2]int32             `json:"chunk"`
	Chunks       [][2]int32           `json:"chunks"`
	Revision     uint64               `json:"revision"`
	BaseRevision uint64               `json:"base_revision"`
	NewRevision  uint64               `json:"new_revision"`
	Sections     string               `json:"sections"`
	Changes      []scenarioBlockPlace `json:"changes"`
	Code         uint8                `json:"code"`
	Message      string               `json:"message"`
}

// scenarioBlockPlace places one named block at one absolute block position.
type scenarioBlockPlace struct {
	Position [3]int32 `json:"position"`
	Block    string   `json:"block"`
}

// loadScenario reads and validates one scenario file.
func loadScenario(path string) (*terrainScenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario: %w", err)
	}
	scenario, err := decodeScenario(data)
	if err != nil {
		return nil, fmt.Errorf("read scenario %s: %w", path, err)
	}
	return scenario, nil
}

// decodeScenario decodes strictly: unknown fields and trailing JSON are
// errors, mirroring the recorded-transcript corpus decoder.
func decodeScenario(data []byte) (*terrainScenario, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var scenario terrainScenario
	if err := decoder.Decode(&scenario); err != nil {
		return nil, fmt.Errorf("decode scenario: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("trailing JSON value")
		}
		return nil, fmt.Errorf("decode scenario: %w", err)
	}
	if err := validateScenario(&scenario); err != nil {
		return nil, err
	}
	return &scenario, nil
}

// validateScenario pins the schema matrix: identity fields, a bounded stage
// interval, a finite player position, at least one round, unique stage
// labels, and every kind's required fields.
func validateScenario(scenario *terrainScenario) error {
	if scenario.SchemaVersion != 1 {
		return fmt.Errorf("scenario schema_version %d is not 1", scenario.SchemaVersion)
	}
	if scenario.Name == "" || scenario.Description == "" {
		return errors.New("scenario name and description are required")
	}
	if scenario.StageIntervalMS < 50 || scenario.StageIntervalMS > 10_000 {
		return fmt.Errorf("scenario stage_interval_ms %d is outside 50..10000", scenario.StageIntervalMS)
	}
	for _, value := range scenario.PlayerPosition {
		if !isFinite32(value) {
			return errors.New("scenario player_position is not finite")
		}
	}
	if len(scenario.Rounds) == 0 || len(scenario.Rounds) > 8 {
		return fmt.Errorf("scenario rounds %d outside 1..8", len(scenario.Rounds))
	}
	roundNames := make(map[string]bool, len(scenario.Rounds))
	for roundIndex, round := range scenario.Rounds {
		if round.Name == "" {
			return fmt.Errorf("round %d has an empty name", roundIndex)
		}
		if roundNames[round.Name] {
			return fmt.Errorf("duplicate round name %q", round.Name)
		}
		roundNames[round.Name] = true
		if len(round.Stages) == 0 {
			return fmt.Errorf("round %q has no stages", round.Name)
		}
		labels := make(map[string]bool, len(round.Stages))
		for stageIndex, stage := range round.Stages {
			if stage.Label == "" {
				return fmt.Errorf("round %q stage %d has an empty label", round.Name, stageIndex)
			}
			if labels[stage.Label] {
				return fmt.Errorf("round %q has duplicate stage label %q", round.Name, stage.Label)
			}
			labels[stage.Label] = true
			if err := validateStage(round.Name, stage); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateStage(round string, stage scenarioStage) error {
	where := fmt.Sprintf("round %q stage %q", round, stage.Label)
	switch stage.Send {
	case stagePlayerState:
		// The player position comes from the scenario level; this stage
		// carries no per-stage fields.
		return nil
	case stageChunkSnapshot:
		if stage.Dimension > 1 {
			return fmt.Errorf("%s dimension %d is outside 0..1", where, stage.Dimension)
		}
		if stage.Revision == 0 {
			return fmt.Errorf("%s revision is zero", where)
		}
		if stage.Sections != sectionLayoutStoneFloor {
			return fmt.Errorf("%s sections layout %q is unknown", where, stage.Sections)
		}
		return nil
	case stageBlockChanges:
		if stage.Dimension > 1 {
			return fmt.Errorf("%s dimension %d is outside 0..1", where, stage.Dimension)
		}
		if stage.BaseRevision == 0 || stage.NewRevision != stage.BaseRevision+1 {
			return fmt.Errorf("%s revision transition %d -> %d is invalid",
				where, stage.BaseRevision, stage.NewRevision)
		}
		if len(stage.Changes) == 0 || len(stage.Changes) > 16 {
			return fmt.Errorf("%s changes %d outside 1..16", where, len(stage.Changes))
		}
		for _, change := range stage.Changes {
			if _, err := blockID(change.Block); err != nil {
				return fmt.Errorf("%s: %w", where, err)
			}
		}
		return nil
	case stageForgetChunks:
		if stage.Dimension > 1 {
			return fmt.Errorf("%s dimension %d is outside 0..1", where, stage.Dimension)
		}
		if len(stage.Chunks) == 0 || len(stage.Chunks) > 64 {
			return fmt.Errorf("%s chunks %d outside 1..64", where, len(stage.Chunks))
		}
		seen := make(map[[2]int32]bool, len(stage.Chunks))
		for _, chunk := range stage.Chunks {
			if seen[chunk] {
				return fmt.Errorf("%s forgets chunk %d/%d twice", where, chunk[0], chunk[1])
			}
			seen[chunk] = true
		}
		return nil
	case stageDisconnect:
		if stage.Message == "" {
			return fmt.Errorf("%s disconnect message is empty", where)
		}
		return nil
	default:
		return fmt.Errorf("%s has unknown send kind %q", where, stage.Send)
	}
}

// isFinite32 mirrors the protocol's own finiteness predicate for float32.
func isFinite32(value float32) bool {
	return value == value && value > -1e38 && value < 1e38
}
