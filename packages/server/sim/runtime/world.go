// Package runtime 实现权威模拟的 tick 编排与边界委派。
package runtime

import "github.com/channing771/mornlea/packages/server/sim/realm"

var (
	ErrChunkNotReady   = realm.ErrChunkNotReady
	ErrBlockOutOfWorld = realm.ErrBlockOutOfWorld
	NewDimension       = realm.NewDimension
)

type (
	ChunkRecord = realm.ChunkRecord
	Dimension   = realm.Dimension
)
