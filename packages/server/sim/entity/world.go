// Package entity 实现玩家、伙伴、敌对生物与物品生命周期的权威状态。
package entity

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
