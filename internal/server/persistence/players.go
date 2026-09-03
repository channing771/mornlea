package persistence

import (
	"context"
	"errors"
	"sync"

	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/storage"
	"github.com/channing771/mornlea/packages/shared/core"
)

var ErrPlayerBackpressure = errors.New("server: player persistence backpressure")

const playerCacheCapacity = 16

type Players struct {
	store        storage.PlayerStore
	options      Options
	mu           sync.Mutex
	completionMu sync.Mutex
	cache        map[core.PlayerID]*cachedPlayer
	flushBarrier bool
	scheduler    *playerSaveScheduler
	jobs         chan playerSaveJob
	completions  chan playerSaveCompletion
	done         chan struct{}
	closeOnce    sync.Once
}

type cachedPlayer struct {
	id                  core.PlayerID
	name, pendingName   string
	persisted           uint64
	snapshot            contract.PlayerSnapshot
	hasSnapshot         bool
	hasObservedSnapshot bool
	missing             bool
	missingConfirmed    bool
	dirty               bool
	active, inFlight    bool
	inFlightRevision    uint64
	forcePending        bool
	retry               *playerSaveJob
	loadDone            chan struct{}
	loadErr             error
	loading             bool
}

type playerSaveJob struct {
	Save     storage.PlayerSave
	Attempt  uint32
	NextTick uint64
}

type playerSaveCompletion struct {
	Job      playerSaveJob
	Revision uint64
	Err      error
}

type playerSaveIdentity struct {
	playerID core.PlayerID
	revision uint64
}

func NewPlayers(store storage.PlayerStore, options Options) *Players {
	scheduler := newPlayerSaveScheduler(store)
	players := &Players{
		store:       store,
		options:     options,
		cache:       make(map[core.PlayerID]*cachedPlayer),
		scheduler:   scheduler,
		jobs:        scheduler.jobs,
		completions: scheduler.completions,
		done:        make(chan struct{}),
	}
	return players
}

func (p *Players) Prepare(
	ctx context.Context,
	id core.PlayerID,
	name string,
	metadata storage.Metadata,
) (contract.PlayerRestore, error) {
	for {
		p.mu.Lock()
		if player := p.cache[id]; player != nil {
			if player.pendingName != "" && player.pendingName != name {
				p.mu.Unlock()
				return contract.PlayerRestore{}, ErrPlayerBackpressure
			}
			if player.loading {
				loadDone := player.loadDone
				p.mu.Unlock()
				select {
				case <-loadDone:
				case <-ctx.Done():
					return contract.PlayerRestore{}, ctx.Err()
				}
				p.mu.Lock()
				loadErr := player.loadErr
				if loadErr != nil {
					p.mu.Unlock()
					return contract.PlayerRestore{}, loadErr
				}
				if p.cache[id] != player || player.loading || player.pendingName != name {
					p.mu.Unlock()
					return contract.PlayerRestore{}, ErrPlayerBackpressure
				}
				restore := player.restore(metadata)
				p.mu.Unlock()
				return restore, nil
			}
			player.pendingName = name
			restore := player.restore(metadata)
			p.mu.Unlock()
			return restore, nil
		}

		p.evictCleanLocked()
		if len(p.cache) >= playerCacheCapacity {
			p.mu.Unlock()
			return contract.PlayerRestore{}, ErrPlayerBackpressure
		}
		placeholder := &cachedPlayer{
			id:          id,
			pendingName: name,
			loadDone:    make(chan struct{}),
			loading:     true,
		}
		p.cache[id] = placeholder
		p.mu.Unlock()

		stored, err := p.store.LoadPlayer(ctx, id)
		p.mu.Lock()
		loadDone := placeholder.loadDone
		if p.cache[id] != placeholder {
			placeholder.loadErr = ErrPlayerBackpressure
			placeholder.loading = false
			close(loadDone)
			p.mu.Unlock()
			return contract.PlayerRestore{}, ErrPlayerBackpressure
		}
		switch {
		case errors.Is(err, storage.ErrPlayerNotFound):
			loaded := newMissingCachedPlayer(id, name, metadata)
			*placeholder = *loaded
			placeholder.loadDone = loadDone
		case err != nil:
			placeholder.loadErr = err
			placeholder.loading = false
			if p.cache[id] == placeholder {
				delete(p.cache, id)
			}
			close(loadDone)
			p.mu.Unlock()
			return contract.PlayerRestore{}, err
		default:
			loaded := cachedPlayerFromStored(stored, name)
			*placeholder = *loaded
			placeholder.loadDone = loadDone
		}
		placeholder.loading = false
		close(loadDone)
		restore := placeholder.restore(metadata)
		p.mu.Unlock()
		return restore, nil
	}
}

func (p *Players) Activate(id core.PlayerID, name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	player := p.cache[id]
	if player == nil || player.loading || player.pendingName != name {
		return ErrPlayerBackpressure
	}
	player.active = true
	return nil
}

func (p *Players) Confirm(id core.PlayerID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	player := p.cache[id]
	if player == nil || player.loading || !player.active || player.pendingName == "" {
		return
	}
	becamePersistable := player.missing && !player.missingConfirmed
	if becamePersistable {
		player.missingConfirmed = true
	}
	if becamePersistable || player.name != player.pendingName {
		player.name = player.pendingName
		player.dirty = true
	}
	player.pendingName = ""
}

func (p *Players) Abort(id core.PlayerID) {
	p.mu.Lock()
	player := p.cache[id]
	if player == nil {
		p.mu.Unlock()
		return
	}
	if player.loading {
		loadDone := player.loadDone
		p.mu.Unlock()
		<-loadDone
		p.mu.Lock()
	}
	defer p.mu.Unlock()
	if p.cache[id] != player || player.loading {
		return
	}
	player.pendingName = ""
	player.active = false
	if player.missing && !player.missingConfirmed && p.cache[id] == player {
		delete(p.cache, id)
	}
}

func (p *Players) Deactivate(id core.PlayerID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	player := p.cache[id]
	if player == nil || player.loading {
		return
	}
	player.active = false
	if p.cache[id] == player && player.evictable() {
		delete(p.cache, id)
	}
}

func (p *Players) Observe(
	id core.PlayerID,
	_ string,
	snapshot contract.PlayerSnapshot,
	tick uint64,
	force bool,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	player := p.cache[id]
	if player == nil || player.loading {
		return ErrPlayerBackpressure
	}
	snapshotChanged := !player.hasSnapshot || !playerSnapshotsEqual(player.snapshot, snapshot)
	if snapshotChanged {
		player.snapshot = clonePlayerSnapshot(snapshot)
		player.hasSnapshot = true
		player.hasObservedSnapshot = true
	}
	if player.missing && !player.missingConfirmed {
		return nil
	}
	if snapshotChanged {
		player.dirty = true
	}
	if force {
		player.forcePending = true
	}
	if force && player.dirty && !player.inFlight {
		if player.retry != nil {
			job := *player.retry
			if p.dispatchLocked(job) {
				player.retry = nil
			}
		} else {
			p.dispatchLocked(playerSaveJob{
				Save:     player.save(player.persisted + 1),
				Attempt:  1,
				NextTick: tick,
			})
		}
	}
	return nil
}

func (p *Players) Poll(tick uint64) error {
	p.completionMu.Lock()
	defer p.completionMu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	err := p.drainCompletionsLocked(tick)
	for _, player := range p.sortedPlayersLocked(func(player *cachedPlayer) bool {
		return !player.loading && !player.inFlight && player.retry != nil &&
			player.retry.NextTick <= tick
	}) {
		job := *player.retry
		if p.dispatchLocked(job) {
			player.retry = nil
		}
	}
	if tick%p.options.AutosaveTicks != 0 {
		return err
	}
	for _, player := range p.sortedPlayersLocked(func(player *cachedPlayer) bool {
		return !player.loading && !player.inFlight && player.dirty && player.retry == nil
	}) {
		p.dispatchLocked(playerSaveJob{
			Save:    player.save(player.persisted + 1),
			Attempt: 1,
		})
	}
	return err
}

func (p *Players) Close() {
	p.closeOnce.Do(func() {
		p.scheduler.CloseJobs()
		p.scheduler.Wait()
		p.completionMu.Lock()
		p.mu.Lock()
		_ = p.drainCompletionsLocked(0)
		p.mu.Unlock()
		p.completionMu.Unlock()
		close(p.done)
	})
}

// QueueDepths 返回保存队列与完成队列的当前长度快照，供 HostStats 使用。
func (p *Players) QueueDepths() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.jobs), len(p.completions)
}

// CloseWorker 为历史测试保留的别名，语义同 Close。
func (p *Players) CloseWorker() {
	p.Close()
}

// Done 返回在 Close 后关闭的 done 通道，供关服测试观测。
func (p *Players) Done() <-chan struct{} {
	return p.done
}

// IsJobsClosed 报告保存通道是否已关闭，供关服重试测试观测。
func (p *Players) IsJobsClosed() bool {
	p.scheduler.submitMu.RLock()
	defer p.scheduler.submitMu.RUnlock()
	return p.scheduler.closed
}

// PlayerHasRetry 报告指定玩家是否有待重试的保存，供集成测试观测。
func (p *Players) PlayerHasRetry(id core.PlayerID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if player := p.cache[id]; player != nil {
		return player.retry != nil
	}
	return false
}

// PlayerIsDirty 报告指定玩家是否为脏，供集成测试观测。
func (p *Players) PlayerIsDirty(id core.PlayerID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if player := p.cache[id]; player != nil {
		return player.dirty
	}
	return false
}

// PlayerIsInFlight 报告指定玩家是否有在途保存。
func (p *Players) PlayerIsInFlight(id core.PlayerID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if player := p.cache[id]; player != nil {
		return player.inFlight
	}
	return false
}

// PlayerPersisted 报告指定玩家的已持久化版本号。
func (p *Players) PlayerPersisted(id core.PlayerID) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if player := p.cache[id]; player != nil {
		return player.persisted
	}
	return 0
}
