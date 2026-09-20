// This file owns the replay half of the transcript helper: synthesizing
// protocol-legal packets from scenario declarations and serving them over
// one sequentially-accepted connection per round. Packet construction is
// pure and validated through the protocol's own validators before anything
// is sent, so a scenario bug fails the helper loudly instead of confusing
// the client with a malformed stream.
package main

import (
	"context"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
	networktcp "github.com/channing771/mornlea/packages/shared/network/tcp"
)

// scenarioReadyTick is the authoritative tick the ready PlayerState
// carries; one per round is enough because each scenario round logs into a
// fresh runtime whose player-tick watermark starts at zero.
const scenarioReadyTick uint64 = 900

// namedBlocks is the scenario's block-name vocabulary, resolved to the
// shared registry's stable block IDs.
var namedBlocks = map[string]core.BlockID{
	"air":   core.AirID,
	"stone": core.StoneID,
}

func blockID(name string) (core.BlockID, error) {
	id, ok := namedBlocks[name]
	if !ok {
		return 0, fmt.Errorf("block name %q is not in the scenario vocabulary", name)
	}
	return id, nil
}

// synthesizeStage builds one server message from a stage declaration. The
// returned message always passes the protocol's own play-state validator; a
// synthesis bug therefore fails the helper instead of reaching the wire.
func synthesizeStage(scenario *terrainScenario, stage scenarioStage) (protocol.ServerMessage, error) {
	var message protocol.ServerMessage
	switch stage.Send {
	case stagePlayerState:
		message = readyPlayerState(scenario.PlayerPosition)
	case stageChunkSnapshot:
		message = stoneFloorSnapshot(
			core.DimensionID(stage.Dimension),
			core.ChunkPos{X: stage.Chunk[0], Z: stage.Chunk[1]},
			stage.Revision,
		)
	case stageBlockChanges:
		changes, err := stoneBlockChanges(stage)
		if err != nil {
			return nil, fmt.Errorf("stage %q: %w", stage.Label, err)
		}
		message = changes
	case stageForgetChunks:
		message = forgetChunks(stage)
	case stageDisconnect:
		message = protocol.Disconnect{Code: protocol.DisconnectCode(stage.Code), Message: stage.Message}
	default:
		return nil, fmt.Errorf("stage %q: unknown send kind %q", stage.Label, stage.Send)
	}
	packet, ok := message.(protocol.ServerPacket)
	if !ok {
		return nil, fmt.Errorf("stage %q synthesized %T, which is not a wire packet", stage.Label, message)
	}
	if err := protocol.ValidateServerPacket(protocol.StatePlay, packet); err != nil {
		return nil, fmt.Errorf("stage %q synthesized an invalid packet: %w", stage.Label, err)
	}
	return message, nil
}

// readyPlayerState builds the authoritative state that marks the session
// playable: finite zeroed motion at the scenario's camera position, vitals
// at their valid upper bounds, and the ready flag set. Every field stays
// inside its protocol domain, which the stage validator proves.
func readyPlayerState(position [3]float32) protocol.PlayerState {
	return protocol.PlayerState{
		// A strictly positive server tick: the runtime drops authoritative
		// states whose tick does not advance the session, and every scenario
		// runtime starts from tick zero, so a zero-tick ready state would
		// never mark the session playable.
		ServerTick: scenarioReadyTick,
		Position:   position,
		Ready:      true,
		Health:     core.MaxHealth,
		Hunger:     core.MaxHunger,
	}
}

// stoneFloorSnapshot builds one whole-chunk snapshot: every one of the 24
// ordered sections is single-storage, section Y=0 stone and the rest air.
// Single storage keeps the payload tiny while exercising the same mirror,
// mesher, and world-batch path a full survival spawn uses.
func stoneFloorSnapshot(dimension core.DimensionID, chunk core.ChunkPos, revision uint64) protocol.ChunkSnapshot {
	sections := make([]protocol.SectionData, core.SectionsPerChunk)
	for index := range sections {
		sections[index] = protocol.SectionData{Y: int32(index), Storage: protocol.SectionSingle, Single: core.AirID}
	}
	sections[0].Single = core.StoneID
	return protocol.ChunkSnapshot{
		Dimension: dimension,
		Chunk:     chunk,
		Revision:  revision,
		Sections:  sections,
	}
}

// stoneBlockChanges builds one chunk delta that places the scenario's
// blocks. The change list is sorted by the protocol's chunk block index
// (strictly ascending is a wire requirement) and every position must belong
// to the stage's chunk.
func stoneBlockChanges(stage scenarioStage) (protocol.BlockChanges, error) {
	changes := make([]protocol.BlockChange, 0, len(stage.Changes))
	for _, place := range stage.Changes {
		id, err := blockID(place.Block)
		if err != nil {
			return protocol.BlockChanges{}, err
		}
		changes = append(changes, protocol.BlockChange{
			Position: core.BlockPos{X: place.Position[0], Y: place.Position[1], Z: place.Position[2]},
			Block:    id,
		})
	}
	chunk := core.ChunkPos{X: stage.Chunk[0], Z: stage.Chunk[1]}
	slices.SortStableFunc(changes, func(left, right protocol.BlockChange) int {
		return chunkBlockIndex(left.Position) - chunkBlockIndex(right.Position)
	})
	return protocol.BlockChanges{
		Dimension:    core.DimensionID(stage.Dimension),
		Chunk:        chunk,
		BaseRevision: stage.BaseRevision,
		NewRevision:  stage.NewRevision,
		Changes:      changes,
	}, nil
}

// chunkBlockIndex mirrors the protocol's own strictly-sorted change order:
// section-major, then local y, z, x.
func chunkBlockIndex(position core.BlockPos) int {
	x, y, z := position.Local()
	return position.SectionIndex()*core.BlocksPerSection +
		y*core.SectionSize*core.SectionSize + z*core.SectionSize + x
}

// forgetChunks builds one forget message from the stage's chunk list.
func forgetChunks(stage scenarioStage) protocol.ForgetChunks {
	chunks := make([]core.ChunkPos, 0, len(stage.Chunks))
	for _, chunk := range stage.Chunks {
		chunks = append(chunks, core.ChunkPos{X: chunk[0], Z: chunk[1]})
	}
	return protocol.ForgetChunks{Dimension: core.DimensionID(stage.Dimension), Chunks: chunks}
}

// serveScenario listens once and replays every round on sequentially
// accepted connections. Exactly one client is connected at any time; a
// round ends by closing its stream after the final stage.
func serveScenario(ctx context.Context, scenario *terrainScenario, port int) error {
	listener, err := networktcp.ListenTCP(fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer func() {
		_ = listener.Close()
	}()
	log.Printf("listening on %s", listener.Addr())
	for _, round := range scenario.Rounds {
		if err := replayRound(ctx, listener, scenario, round); err != nil {
			return fmt.Errorf("round %q: %w", round.Name, err)
		}
		log.Printf("round %q complete", round.Name)
	}
	return nil
}

// replayRound accepts one connection, performs the v44 server login, and
// replays the round's stages with the scenario's fixed inter-stage pause.
func replayRound(ctx context.Context, listener network.Listener, scenario *terrainScenario, round scenarioRound) error {
	stream, err := listener.Accept(ctx)
	if err != nil {
		return fmt.Errorf("accept: %w", err)
	}
	defer func() {
		_ = stream.Close()
	}()
	pending, err := network.BeginServerLogin(ctx, stream, scenario.WorldSeed)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	var endpoint network.ServerEndpoint
	if err := pending.Accept(ctx, func(accepted network.ServerEndpoint) error {
		endpoint = accepted
		return nil
	}); err != nil {
		return fmt.Errorf("login accept: %w", err)
	}
	log.Printf("round %q: login accepted player %s (%s) view %d",
		round.Name, pending.Identity().PlayerID, pending.Identity().DisplayName, pending.ViewDistance())
	interval := time.Duration(scenario.StageIntervalMS) * time.Millisecond
	for index, stage := range round.Stages {
		message, err := synthesizeStage(scenario, stage)
		if err != nil {
			return err
		}
		if err := endpoint.Send(ctx, message); err != nil {
			return fmt.Errorf("stage %q send: %w", stage.Label, err)
		}
		log.Printf("round %q: stage %q sent", round.Name, stage.Label)
		if index+1 < len(round.Stages) {
			if err := sleepContext(ctx, interval); err != nil {
				return fmt.Errorf("stage %q pacing: %w", stage.Label, err)
			}
		}
	}
	// The disconnect stage is the round's terminal packet; closing the
	// stream right after it hands the client its receiver-terminal
	// transition even if it processes the packet and the EOF in either
	// order.
	return nil
}

// sleepContext waits for the interval or the context end, whichever first.
func sleepContext(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
