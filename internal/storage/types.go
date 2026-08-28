// Package storage 定义世界存储的稳定值与接口。
//
// 根包保留编排与门面：disk/memory 编排、world_files/backup/metadata 与
// chunk_keys，外加对拆分出的域子包（storagedef/region/chunk 与后续
// player/companion/hostile）的别名再导出，保证既有 `storage.X` 消费方
// 源码零改动。
package storage

import (
	"context"
	"errors"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/storage/chunk"
	"github.com/channing771/mornlea/internal/storage/region"
	"github.com/channing771/mornlea/internal/storage/storagedef"
)

var (
	// ErrChunkNotFound 与 ErrRevisionConflict 随 chunk 记录层容器定义在
	// chunk 包（容器读写路径直接产生它们），此处绑定同一错误值再导出，
	// 保持既有 `storage.X` 引用与 errors.Is 身份不变。
	ErrChunkNotFound    = chunk.ErrChunkNotFound
	ErrRevisionConflict = chunk.ErrRevisionConflict
	// ErrPlayerNotFound/ErrWorldLocked 仍属根包：player 域与世界锁编排尚未
	// 拆出（后续任务随域迁移）。
	ErrPlayerNotFound = errors.New("storage: player not found")
	ErrWorldLocked    = errors.New("storage: world locked")
	// ErrCorrupt/ErrFutureVersion 定义在 `storagedef` 叶子包（跨域哨兵的公共
	// 下沉），此处绑定同一错误值再导出。
	ErrCorrupt       = storagedef.ErrCorrupt
	ErrFutureVersion = storagedef.ErrFutureVersion
)

type Metadata struct {
	FormatVersion  uint32
	Seed           int64
	SpawnDimension core.DimensionID
	SpawnAnchor    core.ChunkPos
	// WorldTimeTicks 是权威绝对世界时间，metadata v2 起持久化；v1 世界迁移后为零。
	WorldTimeTicks uint64
	// DayPhaseOffset 是显示相位偏移，metadata v3 起持久化；v1/v2 世界迁移后为零。
	// 它只进入显示相位计算 `(WorldTimeTicks + DayPhaseOffset) % 24000`，
	// 不影响绝对时间的推进或任何以绝对时间驱动的模拟。
	DayPhaseOffset uint64
}

// RegionKey 是 region 文件键的别名再导出，定义在 region 格式原语包。
type RegionKey = region.RegionKey

// RegionFor 转发 region 包的坐标换算原语，保持既有 storage.RegionFor 调用点
// 与签名不变。
func RegionFor(key core.ChunkKey) (RegionKey, int) {
	return region.RegionFor(key)
}

// StoredChunk/ChunkSave 是 chunk 记录层的值类型，别名再导出保持类型身份。
type (
	StoredChunk = chunk.StoredChunk
	ChunkSave   = chunk.ChunkSave
)

type SaveResult struct {
	Committed map[core.ChunkKey]uint64
}

type Store interface {
	Metadata() Metadata
	// SaveMetadata 原子提交一份世界 metadata 快照，失败时磁盘上必须保留完整旧版。
	SaveMetadata(context.Context, Metadata) error
	LoadChunk(context.Context, core.ChunkKey) (StoredChunk, error)
	SaveBatch(context.Context, []ChunkSave) (SaveResult, error)
	Sync(context.Context) error
	Close() error
}

// WorldStore 组合世界区块、玩家、伙伴与夜行者聚合存档。
type WorldStore interface {
	Store
	PlayerStore
	CompanionStore
	HostileMobStore
}
