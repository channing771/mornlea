package contract

import (
	"errors"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

func TestBoundaryValuesPreserveCommandAndTickOutput(t *testing.T) {
	command := Command{
		Session:  SessionID(17),
		Sequence: 9,
		Kind:     CommandPlaceBlock,
	}
	if command.Session != 17 || command.Sequence != 9 || command.Kind != CommandPlaceBlock {
		t.Fatalf("command changed while crossing the boundary: %+v", command)
	}

	result := TickResult{
		Changes: []ChunkChangeBatch{{
			Dimension:    core.Overworld,
			Chunk:        core.ChunkPos{X: 2, Z: -3},
			BaseRevision: 4,
			NewRevision:  5,
			Changes: []BlockChange{{
				Position: core.BlockPos{X: 33, Y: 12, Z: -47},
				Block:    core.StoneID,
			}},
		}},
		Rejected: []Rejection{{
			Session:  command.Session,
			Sequence: command.Sequence,
			Reason:   RejectInvalidSlot,
		}},
		PlacementSuccesses: []PlacementSuccess{{
			Session:  command.Session,
			Sequence: command.Sequence,
		}},
	}
	if got := result.Changes[0]; got.BaseRevision != 4 || got.NewRevision != 5 ||
		got.Changes[0].Block != core.StoneID {
		t.Fatalf("chunk change changed while crossing the boundary: %+v", got)
	}
	if got := result.Rejected[0]; got.Session != command.Session ||
		got.Sequence != command.Sequence || got.Reason != RejectInvalidSlot {
		t.Fatalf("rejection changed while crossing the boundary: %+v", got)
	}
}

func TestChunkIngressPreservesValues(t *testing.T) {
	inputErr := errors.New("load failed")
	acquired := AcquiredChunk{
		Key:     core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 3, Z: 4}},
		Missing: true,
		Err:     inputErr,
	}
	generated := GeneratedChunk{
		Dimension: core.Overworld,
		Pos:       core.ChunkPos{X: -2, Z: 8},
		Err:       inputErr,
	}
	if !acquired.Missing || !errors.Is(acquired.Err, inputErr) ||
		generated.Pos != (core.ChunkPos{X: -2, Z: 8}) || !errors.Is(generated.Err, inputErr) {
		t.Fatalf("chunk ingress changed while crossing the boundary: acquired=%+v generated=%+v", acquired, generated)
	}
}
