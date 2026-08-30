// Package sim 实现与协议和渲染无关的权威世界状态。
package entity

import "github.com/channing771/mornlea/internal/sim/realm"

var (
	ErrChunkNotReady   = realm.ErrChunkNotReady
	ErrBlockOutOfWorld = realm.ErrBlockOutOfWorld
	NewDimension       = realm.NewDimension
)

type (
	ChunkRecord = realm.ChunkRecord
	Dimension   = realm.Dimension
)
