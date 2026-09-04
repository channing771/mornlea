//go:build darwin

package app

import (
	"errors"

	"github.com/channing771/mornlea/packages/shared/core"
)

var errBlockTargetUnknown = errors.New("mornlea: block target path is unknown")

type BlockTarget struct {
	Position core.BlockPos
	Name     string
}

func (a *Application) CurrentBlockTarget() (BlockTarget, bool) {
	if _, ready := a.predictor.State(); !ready || a.inventoryOpen || a.containerOpen() ||
		(a.panel != nil && a.panel.visible) {
		return BlockTarget{}, false
	}

	var targetID core.BlockID
	hit, found, err := core.RaycastBlocks(
		a.camera.Pos,
		a.camera.Forward(),
		6,
		func(position core.BlockPos) (bool, error) {
			id, loaded := a.mirror.BlockAt(core.Overworld, position)
			chunk, _ := a.mirror.Chunk(core.Overworld, position.Chunk())
			if !loaded || (chunk != nil && chunk.Desynced) || !core.RegisteredBlock(id) {
				return false, errBlockTargetUnknown
			}
			targetID = id
			return core.InteractionTarget(id), nil
		},
	)
	if err != nil || !found {
		return BlockTarget{}, false
	}
	name, ok := core.BlockDisplayName(targetID)
	if !ok || name == "" {
		return BlockTarget{}, false
	}
	return BlockTarget{Position: hit.Block, Name: name}, true
}
