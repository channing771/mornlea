// Package chunk 承载 chunk 存档域：信封编解码、schema 迁移、chunk 值类型
// 与 region 记录层容器（`Region`，原根包局部 `*region`）。
//
// 记录层容器随本包而非 region 包：`Region.Save`/`Region.Load` 直接调用本包
// 信封编解码并经手 `ChunkSave`/`StoredChunk`，与格式原语（internal/storage/
// region）之间是单向依赖。依赖方向：chunk → {region, storagedef, core,
// world}，禁止依赖根包或其他域子包。
package chunk

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// File 是 region 记录层容器对底层文件的抽象（原根包 types.go 的 regionFile），
// 供根包编排测试注入观察桩。
type File interface {
	ReadAt([]byte, int64) (int, error)
	WriteAt([]byte, int64) (int, error)
	Stat() (os.FileInfo, error)
	Sync() error
	Truncate(int64) error
	Close() error
}

// regionFileHooks 是容器打开 region 文件时的注入钩子；生产路径由
// openRegionFile 提供 os 文件实现，测试经 openRegionWithHooks 注入包装。
type regionFileHooks struct {
	Open func(string, int, fs.FileMode) (File, error)
}

var (
	// ErrChunkNotFound 表示请求的区块在任何已提交 region entry 中都不存在。
	// 定义随记录层容器迁入本包，根包以同一错误值别名再导出。
	ErrChunkNotFound = errors.New("storage: chunk not found")
	// ErrRevisionConflict 表示同一键的等修订号保存携带了不同内容。它同时被
	// 根包 player/companion/hostile 编排路径复用，根包以同一错误值别名再导出。
	ErrRevisionConflict = errors.New("storage: revision conflict")
)

type StoredChunk struct {
	Key               core.ChunkKey
	Revision          uint64
	PersistedRevision uint64
	NeedsRewrite      bool
	Recovered         bool
	Chunk             *world.Chunk
}

type ChunkSave struct {
	Key      core.ChunkKey
	Revision uint64
	Chunk    *world.Chunk
}

// SaveResult 汇总一次 region 批量保存的提交结果；根包编排层把它合并进
// 自己的 storage.SaveResult。
type SaveResult struct {
	Committed map[core.ChunkKey]uint64
}

// ValidateChunkSave 校验单条 chunk 保存的基础约束：非 nil 区块、键位一致、
// 修订号非零。根包的内存与磁盘编排都经它做入口校验。
func ValidateChunkSave(save ChunkSave) error {
	if save.Chunk == nil {
		return fmt.Errorf("storage: chunk save for %v has nil chunk", save.Key)
	}
	if save.Chunk.Pos != save.Key.Pos {
		return fmt.Errorf("storage: chunk save key %v does not match chunk position %v", save.Key, save.Chunk.Pos)
	}
	if save.Revision == 0 {
		return fmt.Errorf("storage: chunk save for %v has zero revision", save.Key)
	}
	return nil
}
