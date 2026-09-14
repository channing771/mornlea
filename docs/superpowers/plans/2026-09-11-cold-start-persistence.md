# 冷启动与持久化解耦 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 冷启动时加载 I/O 优先、保存让路，启动按出生点分阶段装载，外圈后台预取。

**Architecture:** `persistence.World` 新增独立 load lane（有界队列加独立 worker，只调既有 `Store.LoadChunk`）；`storage` 复用既有 region 句柄 LRU 与拆锁，不动写路径；`app` 加载完成判据复用既有单一定义，不改判据行为。

**Tech Stack:** Go 1.26，go.work 六模块（`packages/server`、`packages/client`、`packages/shared`），`storage.Store` 门面，既有 `DiskStore` region 治理。

**Spec:** `docs/superpowers/specs/2026-09-11-cold-start-persistence-design.md`

## Global Constraints

- 权威 tick 只做有界非阻塞调度，磁盘 I/O 只在 worker 内。
- worker 只收克隆快照，发送成功后视为不可变；取消丢弃整结果，不发布部分输出。
- 零 wire/schema/ABI/benchmark scenario 变更；`EstimatedBytes` 双计入语义维持现状。
- 不新增 `packages/audit` 未登记的依赖边；改动后跑 `go test ./packages/audit -count=1`。
- 无用户明确要求不提交（`git commit` 需另行批准）；任务以验证通过为完成。
- 代码注释与 GoDoc 用中文，标识符保留英文；注释不出现任务编号。

---

## File Structure

- `packages/server/server/persistence/world.go`：`World` 结构加 load lane 字段，`NewWorld` 启动 load worker，`Close` 停 load worker。只加字段与方法，不改既有 save 调度语义。
- `packages/server/server/persistence/world_load_lane.go`（新建）：load 类型与 worker 实现。职责：有界队列、只调 `store.LoadChunk`、结果回传、无共享可变状态。
- `packages/server/server/persistence/world_load_lane_test.go`（新建）：load lane 独立测试，用包内 stub `Store`，不依赖 `MemoryStore` 构造名，不触 `Engine`。
- `packages/server/server/persistence/world_phased_load.go`（新建）：纯函数 `SplitPhasedLoad`，把已排好序的全量键切成首批与剩余。不依赖 `core` 内部结构，只切 `[]core.ChunkKey`。
- `packages/server/server/persistence/world_phased_load_test.go`（新建）：切分纯函数测试。
- `packages/server/storage/`：不新增文件。本计划只复用既有 `DiskStore` region 句柄 LRU 与 `Store.LoadChunk`，出现 load 侧真实竞态才修，修则另起任务。
- `packages/client/cmd/mornlea/app/app_load_coldstart_test.go`（新建，带 `//go:build darwin`）：锁定 `LoadedChunkTarget` 公式，不碰判据行为。

---

### Task 1: Load lane 类型与 worker

**Files:**
- Modify: `packages/server/server/persistence/world.go`
- Create: `packages/server/server/persistence/world_load_lane.go`
- Test: `packages/server/server/persistence/world_load_lane_test.go`

**Interfaces:**
- Consumes: 既有 `storage.Store.LoadChunk(context.Context, core.ChunkKey) (storage.StoredChunk, error)`，既有 `Options.SaveWorkers`、`saveCtx` 生命周期。
- Produces: `type LoadRequest struct { Keys []core.ChunkKey }`、`type LoadResult struct { Key core.ChunkKey; Chunk storage.StoredChunk; Err error }`、`func (w *World) SubmitLoad(req LoadRequest) bool`（队列满返回 false，不阻塞）、`func (w *World) DrainLoad() []LoadResult`（非阻塞取尽）、`func (w *World) LoadPending() int`、`Options.LoadWorkers int`（小于等于 0 时按 2 启动）。

- [ ] **Step 1: Write the failing test**

```go
package persistence

import (
	"context"
	"testing"

	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
)

type stubLoadStore struct{ storage.Store }

func (stubLoadStore) LoadChunk(_ context.Context, key core.ChunkKey) (storage.StoredChunk, error) {
	return storage.StoredChunk{}, storage.ErrChunkNotFound
}

func TestSubmitLoadMissingKeyDrainsNotFound(t *testing.T) {
	t.Parallel()
	store := stubLoadStore{}
	world := NewWorld(store, nil, Options{SaveWorkers: 1, LoadWorkers: 1})
	defer world.Close()
	var zero core.ChunkKey
	if !world.SubmitLoad(LoadRequest{Keys: []core.ChunkKey{zero}}) {
		t.Fatalf("SubmitLoad = false, want true")
	}
	deadline := 1000
	for world.LoadPending() > 0 && deadline > 0 {
		deadline--
	}
	results := world.DrainLoad()
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Err == nil {
		t.Fatalf("results[0].Err = nil, want non-nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./packages/server/server/persistence -run TestSubmitLoadMissingKeyDrainsNotFound -count=1`
Expected: FAIL（`SubmitLoad`、`LoadRequest` 未定义）。

- [ ] **Step 3: Write minimal implementation**

新建 `world_load_lane.go`，内容：`LoadRequest`、`LoadResult` 两类型；`World` 加 `loadJobs chan LoadRequest`、`loadResults chan LoadResult`、`loadWorkers sync.WaitGroup`、`loadPending atomic.Int32`、`loadOnce sync.Once`、`loadWorkerNum int` 六字段（与既有 `mu`/`saveJobs` 互不干扰）；`SubmitLoad` 遇已关闭（`saveCtx.Err() != nil`）直接回 false，空 `Keys` 直接回 true，随后经 `loadOnce` 懒起 worker（`loadWorkerNum` 存归一化后的数量）再非阻塞入队（满回 false）；`loadWorker` 循环取请求、逐键调 `store.LoadChunk(context.Background(), key)`、结果进 `loadResults` 并减计数，worker 内部不循环重试（单列失败以 `LoadResult.Err` 原样返回，调用方按返回计数有界重提 `SubmitLoad`，tick 永不阻塞）；load 与 save 用各自独立 worker 池，save 池数量与语义一行不动，故 load 满载也不饿死 save。改 `world.go`：`Options` 加 `LoadWorkers int`；`NewWorld` 按 `LoadWorkers`（小于等于 0 置 2）建缓冲（容量 `LoadWorkers*8`）但不启动 worker（零使用时 goroutine 基线与改前一致）；`Close` 先 `cancelSaves` 后等 load 与 save 两组 `WaitGroup`（未启动时 load 等待立即返回；复用既有 `saveDone` 语义，不改 save 顺序）。tick 路径（`Observe`/`Drain`）一行不碰。回退语义：调用方不提交任何 `SubmitLoad` 即为当前行为，零配置项、零分支开关。

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./packages/server/server/persistence -run TestSubmitLoadMissingKeyDrainsNotFound -race -count=1`
Expected: PASS。
Run: `go test ./packages/server/server/persistence -race -count=1`
Expected: PASS（既有 save/flush 测试零回归）。

---

### Task 2: 分阶段切分纯函数

**Files:**
- Create: `packages/server/server/persistence/world_phased_load.go`
- Test: `packages/server/server/persistence/world_phased_load_test.go`

**Interfaces:**
- Consumes: Task 1 的 `LoadRequest`（调用方把首批装进一次 `SubmitLoad`，剩余后台分批提交）。
- Produces: `func SplitPhasedLoad(all []core.ChunkKey, firstN int) (first, rest []core.ChunkKey)`（不排序，只按入参顺序切；`firstN <= 0` 返回空首批，`firstN >= len(all)` 全归首批；返回切片与入参无共享写）。

- [ ] **Step 1: Write the failing test**

```go
package persistence

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestSplitPhasedLoadKeepsOrder(t *testing.T) {
	t.Parallel()
	var a, b, c core.ChunkKey
	first, rest := SplitPhasedLoad([]core.ChunkKey{a, b, c}, 2)
	if len(first) != 2 || len(rest) != 1 {
		t.Fatalf("got %d/%d, want 2/1", len(first), len(rest))
	}
	if first[0] != a || first[1] != b || rest[0] != c {
		t.Fatalf("order changed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./packages/server/server/persistence -run TestSplitPhasedLoadKeepsOrder -count=1`
Expected: FAIL（`SplitPhasedLoad` 未定义）。

- [ ] **Step 3: Write minimal implementation**

```go
package persistence

import "github.com/channing771/mornlea/packages/shared/core"

// SplitPhasedLoad 把已排好序的全量键切成首批与剩余：只切分不排序，调用方
// 负责按出生点距离排好 `all`。
func SplitPhasedLoad(all []core.ChunkKey, firstN int) (first, rest []core.ChunkKey) {
	if firstN <= 0 {
		rest = make([]core.ChunkKey, len(all))
		copy(rest, all)
		return nil, rest
	}
	if firstN >= len(all) {
		out := make([]core.ChunkKey, len(all))
		copy(out, all)
		return out, nil
	}
	first = make([]core.ChunkKey, firstN)
	copy(first, all[:firstN])
	rest = make([]core.ChunkKey, len(all)-firstN)
	copy(rest, all[firstN:])
	return first, rest
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./packages/server/server/persistence -run TestSplitPhasedLoad -race -count=1`
Expected: PASS。

---

### Task 3: App 加载判据回归锁（零行为变更）

**Files:**
- Create: `packages/client/cmd/mornlea/app/app_load_coldstart_test.go`
- Modify: 无生产文件。

**Interfaces:**
- Consumes: 既有 `LoadedChunkTarget(app LoadingApplication) int`，既有 `config.Render.ViewDistance`。
- Produces: 无新导出，只钉住公式 `(2*(ViewDistance+1)+1)^2`。

- [ ] **Step 1: Write the failing test**

```go
//go:build darwin

package app

import (
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
)

type coldstartFakeApp struct{ viewDistance int }

func (f coldstartFakeApp) Frame(_, _ int, _ time.Duration) (bool, error) {
	return false, nil
}
func (f coldstartFakeApp) Window() Window { return nil }
func (f coldstartFakeApp) LoadedChunks() map[core.ChunkPos]struct{} {
	return map[core.ChunkPos]struct{}{}
}
func (f coldstartFakeApp) Mesher() *client.Mesher { return nil }
func (f coldstartFakeApp) Scheduler() *render.SectionScheduler {
	return nil
}
func (f coldstartFakeApp) Render() config.Render {
	return config.Render{ViewDistance: f.viewDistance}
}

func TestLoadedChunkTargetFormulaLocked(t *testing.T) {
	t.Parallel()
	if got := LoadedChunkTarget(coldstartFakeApp{viewDistance: 0}); got != 9 {
		t.Fatalf("VD=0 got %d, want 9", got)
	}
	if got := LoadedChunkTarget(coldstartFakeApp{viewDistance: 1}); got != 25 {
		t.Fatalf("VD=1 got %d, want 25", got)
	}
}
```

- [ ] **Step 2: Confirm the lock is missing before adding it**

Run: `go test ./packages/client/cmd/mornlea/app -run TestLoadedChunkTargetFormulaLocked -count=1`
Expected: `no tests to run`（锁缺失的诚实基线；落盘 Step 3 后 Step 4 必须 PASS，若未来公式被改则 FAIL，有牙齿）。

- [ ] **Step 3: Add the test file only**

只落盘上述测试文件，不改 `app_load.go` 任何生产代码。若 `config.Render` 字段名与此处不同，以 `app_load.go` 的 `app.Render().ViewDistance` 用法为准对齐字段，其余不动。

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./packages/client/cmd/mornlea/app -run TestLoadedChunkTargetFormulaLocked -race -count=1`
Expected: PASS。

---

### Task 4: 收尾门禁与记录

**Files:**
- Modify: 无生产代码；冷启动 wall-time 记录进 `docs/notes/`（若该目录要求另行批准则只在任务记录贴数字，不落盘）。

**Interfaces:**
- Consumes: Task 1–3 的全部测试。
- Produces: 门禁结果清单（命令加结果，不贴长日志）。

- [ ] **Step 1: Run persistence and storage suites**

Run: `go test ./packages/server/server/persistence -race -count=1`
Expected: PASS。
Run: `go test ./packages/server/storage/... -race -count=1`
Expected: PASS。

- [ ] **Step 2: Run app and audit gates**

Run: `go test ./packages/client/cmd/mornlea/app -race -count=1`
Expected: PASS。
Run: `go test ./packages/audit -count=1`
Expected: PASS。

- [ ] **Step 3: Run repo gates**

Run: `make dev-check`
Expected: PASS。
Run: `openspec validate --all --strict --no-interactive`
Expected: PASS。
Run: `make visual-check`
Expected: PASS（golden 零差异；若环境无 GPU，按仓库既有跳过口径记录，不放宽正确性）。

- [ ] **Step 4: Record cold-start numbers only**

用固定大存档跑冷启动 wall-time（`WaitUntilLoaded` 返回的 snapshot 时长），只记录数字，不设硬门禁，不改 `perf-baseline` 口径。
