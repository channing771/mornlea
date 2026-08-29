package persistence

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/channing771/mornlea/internal/core"
)

type playerSaveKey struct {
	playerID core.PlayerID
	revision uint64
}

// errPlayerFlushStalled 是 `Flush` 的失速哨兵。`Flush` 在 `!inFlight && !dispatched`
// 退出路径（见 `recordFlushStallLocked`）返回前，为每个仍处于脏状态、且本次 `Flush`
// 未记录该玩家任何保存失败的玩家计入一次本错误——这既覆盖恒脏态（保存结果永不等于
// 当前快照，`playerFlushJob` 每轮铸出新 revision、双类名额用尽后不再派发），也覆盖
// 从未被成功派发过的玩家（例如 `dispatchFlushLocked` 因 `p.scheduler.TrySubmit` 被拒
// 而返回 false，玩家从未占用任何名额）。旧实现在这两种情形下都会掩盖残余脏状态、
// 静默成功返回；这里改为显式带错误返回是方案 A′ 有意的收紧（design.md「附带收紧」），
// 用户可见变化是原本静默成功的 Flush 调用可能开始返回错误——脏状态、快照与冻结的
// `cachedPlayer.retry` 全部原样保留，不影响下一次 `Flush` 正常重试。
var errPlayerFlushStalled = errors.New("server: player flush stalled: still dirty after bounded dispatches")

// playerFlushSlots 记录单个玩家在本次 `Flush` 调用内已占用的派发类。旧实现用
// `(playerID, revision)` 精确键去重，但 `applyCompletionWithDispatchLocked` 在恒脏态下
// 让 `player.persisted` 单调递增、`playerFlushJob` 每轮铸出全新 revision，精确键永远
// 不会重复命中，导致 `Flush` 无界重派。playerFlushSlots 改按玩家身份记账，把名额分成
// retry 与 fresh 两类而非单一名额，是因为三条既有钉住测试要求的语义单名额会破坏：
// `TestPlayerFlushFailureRetainsFrozenJobForLaterRetry`（冻结重试成功后仍需追派最新
// 快照）、`TestPlayerFlushWaitsForInheritedRevisionAndAttemptsOnlyLatestFollowup`（继承
// in-flight 完成后需追派最新快照）、`TestPlayerFlushInheritedFailureDoesNotDispatchForcedFollowup`
// （继承失败冻结的 retry 不得在同次 Flush 内重派）。
type playerFlushSlots struct {
	// retry 为真表示：本次 Flush 内该玩家的冻结重试类名额已占用——或是 `player.retry != nil`
	// 时的一次重派，或是 Flush 开始时继承的 in-flight 快照的预占（继承的保存本身就是一次
	// 隐式的重试类占用，不应在等待屏障完成后于本次 Flush 内被再次派发）。
	retry bool
	// fresh 为真表示：本次 Flush 内该玩家的最新快照类名额已占用，即
	// `save(player.persisted + 1)` 已被派发过一次；成功派发 fresh 保存同时把 retry 也
	// 置真，因为 fresh 若失败会冻结同 revision 的 retry job，同一次 Flush 内重派它将违
	// 反「每 revision 只试一次」。
	fresh bool
}

func (p *Players) Flush(ctx context.Context) error {
	if ctx == nil {
		panic("server: nil player flush context")
	}
	p.completionMu.Lock()
	defer p.completionMu.Unlock()

	attempted := make(map[core.PlayerID]playerFlushSlots, playerCacheCapacity)
	failures := make(map[playerSaveKey]error, playerCacheCapacity)
	p.mu.Lock()
	p.flushBarrier = true
	inherited := make(map[playerSaveKey]struct{}, playerCacheCapacity)
	for id, player := range p.cache {
		if !player.inFlight {
			continue
		}
		key := playerSaveKey{playerID: id, revision: player.inFlightRevision}
		inherited[key] = struct{}{}
		slots := attempted[id]
		slots.retry = true
		attempted[id] = slots
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.flushBarrier = false
		p.mu.Unlock()
	}()

	if err := p.waitInheritedFlushCompletions(ctx, inherited, failures); err != nil {
		return joinPlayerFlushErrors(failures, err)
	}
	for {
		if err := ctx.Err(); err != nil {
			return joinPlayerFlushErrors(failures, err)
		}

		p.mu.Lock()
		if !p.hasDirtyOrInFlightLocked() {
			p.mu.Unlock()
			return joinPlayerFlushErrors(failures, nil)
		}
		dispatched := false
		if !p.hasInFlightLocked() {
			for _, player := range p.sortedPlayersLocked(func(player *cachedPlayer) bool {
				return !player.loading && !player.inFlight && player.dirty
			}) {
				job := playerFlushJob(player)
				// 派发前先固定 job 所属的类：`dispatchFlushLocked` 成功后会清空
				// `player.retry`，事后再判会把一次 retry 派发误记成 fresh。
				isRetry := player.retry != nil
				slots := attempted[player.id]
				if isRetry {
					if slots.retry {
						continue
					}
				} else if slots.fresh {
					continue
				}
				if p.dispatchFlushLocked(job) {
					slots.retry = true
					if !isRetry {
						slots.fresh = true
					}
					attempted[player.id] = slots
					dispatched = true
				}
				break
			}
		}
		inFlight := p.hasInFlightLocked()
		if !inFlight && !dispatched {
			// 唯一可能带残余脏状态返回的路径：在仍持有 `p.mu` 时把恒脏失速上报进
			// `failures`，避免解锁后玩家状态被其他 goroutine 并发改写导致收集不确定。
			p.recordFlushStallLocked(failures)
			p.mu.Unlock()
			return joinPlayerFlushErrors(failures, nil)
		}
		p.mu.Unlock()

		select {
		case completion := <-p.completions:
			key := playerSaveKey{
				playerID: completion.Job.Save.PlayerID,
				revision: completion.Job.Save.Revision,
			}
			p.mu.Lock()
			err := p.applyCompletionWithDispatchLocked(completion, 0, false)
			p.mu.Unlock()
			if err != nil {
				failures[key] = err
			}
		case <-ctx.Done():
			return joinPlayerFlushErrors(failures, ctx.Err())
		case <-p.scheduler.ctx.Done():
			return joinPlayerFlushErrors(failures, p.scheduler.ctx.Err())
		}
	}
}

func (p *Players) waitInheritedFlushCompletions(
	ctx context.Context,
	inherited map[playerSaveKey]struct{},
	failures map[playerSaveKey]error,
) error {
	for len(inherited) != 0 {
		select {
		case completion := <-p.completions:
			key := playerSaveKey{
				playerID: completion.Job.Save.PlayerID,
				revision: completion.Job.Save.Revision,
			}
			if _, ok := inherited[key]; !ok {
				continue
			}
			delete(inherited, key)
			p.mu.Lock()
			err := p.applyCompletionWithDispatchLocked(completion, 0, false)
			p.mu.Unlock()
			if err != nil {
				failures[key] = err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-p.scheduler.ctx.Done():
			return p.scheduler.ctx.Err()
		}
	}
	return nil
}

// recordFlushStallLocked 在 `Flush` 即将以「无 in-flight 且本轮未派发」退出前，为每个
// 仍处于脏状态、且本次 Flush 未记录任何保存失败的玩家计入一次 `errPlayerFlushStalled`。
// 已有失败记录的玩家（`failures` 中已存在同 `playerID` 的键）只保留原错误，不重复上
// 报，逐字匹配既有测试对错误文本的断言。结果的确定性来自「每个脏玩家至多产出一条以
// 自身 `playerID` 为键的记录」这一事实本身，与收集顺序无关；这里用 `sortedPlayersLocked`
// 只是为了让枚举顺序稳定、便于调试和复现，不承担保证确定性的作用（最终 `failures` 仍
// 由 `joinPlayerFlushErrors` 按键排序拼接成错误文本）。调用方必须持有 `p.mu`。
func (p *Players) recordFlushStallLocked(failures map[playerSaveKey]error) {
	for _, player := range p.sortedPlayersLocked(func(player *cachedPlayer) bool {
		return !player.loading && player.dirty
	}) {
		hasFailure := false
		for key := range failures {
			if key.playerID == player.id {
				hasFailure = true
				break
			}
		}
		if !hasFailure {
			failures[playerSaveKey{
				playerID: player.id,
				revision: player.persisted + 1,
			}] = errPlayerFlushStalled
		}
	}
}

func playerFlushJob(player *cachedPlayer) playerSaveJob {
	if player.retry != nil {
		return *player.retry
	}
	return playerSaveJob{
		Save:    player.save(player.persisted + 1),
		Attempt: 1,
	}
}

func (p *Players) dispatchFlushLocked(job playerSaveJob) bool {
	player := p.cache[job.Save.PlayerID]
	if player == nil || player.loading || player.inFlight {
		return false
	}
	if !p.scheduler.TrySubmit(job) {
		return false
	}
	player.inFlight = true
	player.inFlightRevision = job.Save.Revision
	if player.matchesSave(job.Save) {
		player.forcePending = false
	}
	if player.retry != nil {
		player.retry = nil
	}
	return true
}

func joinPlayerFlushErrors(failures map[playerSaveKey]error, terminal error) error {
	keys := make([]playerSaveKey, 0, len(failures))
	for key := range failures {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if compared := bytes.Compare(keys[left].playerID[:], keys[right].playerID[:]); compared != 0 {
			return compared < 0
		}
		return keys[left].revision < keys[right].revision
	})
	joined := make([]error, 0, len(keys)+1)
	for _, key := range keys {
		joined = append(joined, fmt.Errorf(
			"save player %s revision %d: %w",
			key.playerID,
			key.revision,
			failures[key],
		))
	}
	if terminal != nil {
		joined = append(joined, terminal)
	}
	return errors.Join(joined...)
}
