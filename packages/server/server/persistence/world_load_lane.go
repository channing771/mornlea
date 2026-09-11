package persistence

import (
	"context"

	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
)

// LoadRequest 是一批待从存档加载的区块键集合。提交成功后其中的
// `Keys` 切片视为不可变，调用方不得再修改。
type LoadRequest struct {
	Keys []core.ChunkKey
}

// LoadResult 是单个区块键的加载结果。加载失败时 `Err` 非空，
// 调用方按需有界重提 `SubmitLoad`。
type LoadResult struct {
	Key   core.ChunkKey
	Chunk storage.StoredChunk
	Err   error
}

// SubmitLoad 非阻塞提交一批加载请求，队列满时返回 false。
// 首次提交时懒启动加载 worker，此前不占 goroutine。
// `Close` 后不再接受提交（调用方契约）：既明确语义，也避免 `Close`
// 的等待与懒启动的计数增加形成竞态。
func (world *World) SubmitLoad(req LoadRequest) bool {
	if world.saveCtx.Err() != nil {
		return false
	}
	// 空请求不占队列槽位，直接成功。
	if len(req.Keys) == 0 {
		return true
	}
	world.ensureLoadWorkers()
	keys := append([]core.ChunkKey(nil), req.Keys...)
	world.loadPending.Add(int32(len(keys)))
	select {
	case world.loadJobs <- LoadRequest{Keys: keys}:
		return true
	default:
		world.loadPending.Add(-int32(len(keys)))
		return false
	}
}

// ensureLoadWorkers 在首次提交时一次性启动加载 worker。
// worker 数量取 `NewWorld` 已归一化的 `loadWorkerNum`，此处不重复默认值。
func (world *World) ensureLoadWorkers() {
	world.loadOnce.Do(func() {
		world.loadWorkers.Add(world.loadWorkerNum)
		for range world.loadWorkerNum {
			go world.loadWorker()
		}
	})
}

// DrainLoad 非阻塞取尽当前已完成的加载结果。
func (world *World) DrainLoad() []LoadResult {
	var results []LoadResult
	for {
		select {
		case result := <-world.loadResults:
			results = append(results, result)
		default:
			return results
		}
	}
}

// LoadPending 返回已提交但结果尚未就绪的加载键数量。
func (world *World) LoadPending() int {
	return int(world.loadPending.Load())
}

// loadWorker 逐键调用 `Store.LoadChunk` 并把结果送入 `loadResults`。
// 单键失败只以 `LoadResult.Err` 原样返回，内部不重试；取消时丢弃
// 未发布的结果、回滚剩余计数后直接退出，不再取新的队列任务。
func (world *World) loadWorker() {
	defer world.loadWorkers.Done()
	for {
		// 取消优先：阻塞等待前先非阻塞探一次，避免已取消时仍取走
		// 队列任务，导致 `Close` 被磁盘积压拖住。
		select {
		case <-world.saveCtx.Done():
			return
		default:
		}
		select {
		case <-world.saveCtx.Done():
			return
		case req := <-world.loadJobs:
			for index, key := range req.Keys {
				chunk, err := world.store.LoadChunk(context.Background(), key)
				select {
				case world.loadResults <- LoadResult{Key: key, Chunk: chunk, Err: err}:
					world.loadPending.Add(-1)
				case <-world.saveCtx.Done():
					// 回滚本键及后续未发布键的计数，保持 `LoadPending` 不泄露。
					world.loadPending.Add(-int32(len(req.Keys) - index))
					return
				}
			}
		}
	}
}
