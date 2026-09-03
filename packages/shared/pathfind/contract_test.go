package pathfind_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/pathfind"
)

func TestPublicPathfindFindsTrivialPath(t *testing.T) {
	table := pathfind.NewPathBlockTable(map[core.BlockID]bool{core.AirID: true})
	grid, err := pathfind.NewPathGrid(
		core.BlockPos{Y: 63}, 1, 3, 1, table,
		func(_, y, _ int32) (core.BlockID, bool) {
			if y == 63 {
				return core.StoneID, true
			}
			return core.AirID, true
		},
		[]pathfind.ChunkRevision{{Chunk: core.ChunkPos{}, Revision: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	cell := pathfind.PathCell{Y: 64}
	result, err := pathfind.FindPath(grid, cell, cell)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Waypoints) != 1 || result.Waypoints[0] != cell {
		t.Fatalf("waypoints=%+v，想要单一站立格 %+v", result.Waypoints, cell)
	}
}
