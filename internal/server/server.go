package server

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/sim"
	"github.com/channing771/mornlea/internal/storage"
)

type serverLifecycle uint8

const (
	serverRunning serverLifecycle = iota
	serverClosing
	serverClosed
)

type Server struct {
	config                    Config
	generator                 Generator
	store                     storage.Store
	engine                    *sim.Engine
	sessions                  map[sim.SessionID]*session
	playerSessions            map[core.PlayerID]sim.SessionID
	trustedObserver           *session
	trustedObserverGeneration uint64

	ctx         context.Context
	cancel      context.CancelFunc
	saveCtx     context.Context
	cancelSaves context.CancelFunc

	incoming         chan incomingCommand
	incomingChats    chan incomingChat
	companionsByName map[string]companion.Definition
	nextChatEventID  uint64
	companionManager *companionManager
	inputBoundary    atomic.Pointer[inputIngressBoundary]
	jobs             chan chunkJob
	acquired         chan sim.AcquiredChunk
	generated        chan sim.GeneratedChunk
	pending          []chunkJob
	queued           map[core.ChunkKey]struct{}

	trustedObserverMu       sync.Mutex
	trustedObserverCenters  chan trustedObserverCenter
	trustedObserverSequence uint64
	appliedTrustedObserver  appliedTrustedObserverCenter

	workers         sync.WaitGroup
	saveWorkers     sync.WaitGroup
	saveJobs        chan saveJob
	saveCompletions chan saveCompletion
	autosaveActive  bool
	metadataSave    metadataSaveState
	companions      *companionPersistence
	retry           map[storage.RegionKey][]retrySave
	retryInFlight   map[uint64]retrySave
	nextRetryID     uint64
	backpressured   bool
	lastSaveSuccess time.Time
	lastSaveError   string
	lastSaveErrorAt time.Time
	stepMu          sync.Mutex
	shutdownGate    chan struct{}
	lifecycle       serverLifecycle
	// paused 是权威暂停门：置位时整个权威 tick 不被调度执行——世界时间、
	// 随机 tick、作物、流体、实体与持久化调度全部停走，而消息接收 goroutine、
	// chunk/save worker 与既有缓冲保持存活，暂停期到达的命令在恢复后按序结算。
	// 门本身不持锁：`RunTicks` 调度层与 `step` 内的所有推进点只做一次原子读，
	// 热路径零分配、零锁竞争；幂等由布尔语义天然保证。
	paused      atomic.Bool
	runtimeDone chan struct{}
	saveDone    chan struct{}
	closedDone  chan struct{}
	storePhase  storeShutdownPhase
}

func NewWorld(config Config, generator Generator, store storage.Store) *Server {
	if len(config.Companions) != 0 {
		panic("server: NewWorld does not support companions; use NewHost")
	}
	return newWorld(config, generator, store, nil)
}

func newWorld(
	config Config,
	generator Generator,
	store storage.Store,
	companions *companionPersistence,
) *Server {
	config.validate()
	if generator == nil {
		panic("server: nil generator")
	}
	if store == nil {
		panic("server: nil store")
	}
	ctx, cancel := context.WithCancel(context.Background())
	saveCtx, cancelSaves := context.WithCancel(context.Background())
	shutdownGate := make(chan struct{}, 1)
	shutdownGate <- struct{}{}
	queueCapacity := max(1, config.Workers*2)
	metadata := store.Metadata()
	server := &Server{
		config:          config,
		generator:       generator,
		store:           store,
		engine:          sim.NewEngine(config.ViewRadius, metadata.WorldTimeTicks, metadata.Seed),
		sessions:        make(map[sim.SessionID]*session),
		playerSessions:  make(map[core.PlayerID]sim.SessionID),
		ctx:             ctx,
		cancel:          cancel,
		saveCtx:         saveCtx,
		cancelSaves:     cancelSaves,
		incoming:        make(chan incomingCommand, inputCapacity),
		incomingChats:   make(chan incomingChat, inputCapacity),
		jobs:            make(chan chunkJob, queueCapacity),
		acquired:        make(chan sim.AcquiredChunk, queueCapacity),
		generated:       make(chan sim.GeneratedChunk, queueCapacity),
		saveJobs:        make(chan saveJob, config.SaveWorkers*2),
		saveCompletions: make(chan saveCompletion, config.SaveWorkers*2),
		retry:           make(map[storage.RegionKey][]retrySave),
		retryInFlight:   make(map[uint64]retrySave),
		queued:          make(map[core.ChunkKey]struct{}),
		runtimeDone:     make(chan struct{}),
		saveDone:        make(chan struct{}),
		closedDone:      make(chan struct{}),
		shutdownGate:    shutdownGate,
		companions:      companions,
	}
	if len(config.Companions) != 0 {
		server.companionsByName = make(
			map[string]companion.Definition,
			len(config.Companions),
		)
		for _, definition := range config.Companions {
			server.companionsByName[definition.Name] = definition
		}
	}
	if config.TrustedObserver {
		server.trustedObserverCenters = make(chan trustedObserverCenter, 1)
	}
	if companions != nil {
		companions.mu.Lock()
		records := slices.Clone(companions.records)
		loadedQueues := cloneStoredQueues(companions.loadedQueues)
		companions.mu.Unlock()
		for _, definition := range config.Companions {
			restore := sim.CompanionRestore{
				ID:             definition.ID,
				SpawnDimension: metadata.SpawnDimension,
				SpawnAnchor:    metadata.SpawnAnchor,
			}
			for index := range records {
				if records[index].ID == definition.ID {
					body := records[index]
					restore.Body = &body
					break
				}
			}
			server.engine.RegisterCompanion(restore)
		}
		// 伙伴启用即装配任务编排。NewHost 已在存档加载前校验模型设置，
		// 这里构造失败只可能是不可达的防御路径。Dialogue 客户端与 Planner
		// 共用同一 AIModel 设置与密钥，独立构造（提示与输入互不共享）。
		planner, err := companion.NewPlannerClient(config.AIModel, config.AIAPIKey, nil)
		if err != nil {
			panic("server: construct companion planner: " + err.Error())
		}
		dialogue, err := companion.NewDialogueClient(config.AIModel, config.AIAPIKey, nil)
		if err != nil {
			panic("server: construct companion dialogue client: " + err.Error())
		}
		server.companionManager = newCompanionManager(server.engine, config, planner, dialogue)
		// 注入在线玩家权威源：规划快照的 OnlinePlayers 填充与 follow 目标
		// 的在线性/位置解析共用同一会话注册表读取路径。
		server.companionManager.onlinePlayers = server.onlinePlanPlayersSnapshot
		// 恢复接线：任务域载荷在首个 tick 之前回填槽位（Planning/
		// Validating 归一为 Queued，Running 保留进度且路径留空待重算）。
		server.companionManager.restoreQueues(loadedQueues)
	}

	server.workers.Add(config.Workers)
	for range config.Workers {
		go server.chunkWorker()
	}
	server.saveWorkers.Add(config.SaveWorkers)
	for range config.SaveWorkers {
		go server.saveWorker()
	}
	go func() {
		server.workers.Wait()
		close(server.runtimeDone)
	}()
	go func() {
		server.saveWorkers.Wait()
		close(server.saveDone)
	}()
	return server
}

func (server *Server) AttachSession(spec SessionSpec) (<-chan SessionExit, error) {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.attachSessionLocked(spec)
}

func (server *Server) DetachSession(
	id sim.SessionID,
	generation uint64,
	cause error,
) bool {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.detachSessionLocked(id, generation, cause)
}

func (server *Server) AttachTrustedObserver(endpoint network.ServerEndpoint) error {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.attachTrustedObserverLocked(endpoint)
}

func (server *Server) CloseTrustedObserver() error {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	current := server.trustedObserver
	if current == nil {
		return nil
	}
	server.detachTrustedObserverLocked(current.id, current.generation, nil)
	return current.closeErr
}

func (server *Server) detachTrustedObserver(
	id sim.SessionID,
	generation uint64,
	cause error,
) bool {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.detachTrustedObserverLocked(id, generation, cause)
}

func (server *Server) SetTrustedObserverCenter(
	dimension core.DimensionID,
	center core.ChunkPos,
) error {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.setTrustedObserverCenterLocked(dimension, center)
}

func (server *Server) AppliedTrustedObserverCenter() (
	core.DimensionID,
	core.ChunkPos,
	uint64,
	bool,
) {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.appliedTrustedObserverCenterLocked()
}

// Step 执行一次服务端编排与权威模拟 tick。
func (server *Server) Step() sim.TickResult {
	return server.step(time.Time{})
}

func (server *Server) step(scheduled time.Time) sim.TickResult {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	if server.lifecycle != serverRunning {
		return sim.TickResult{}
	}
	// 暂停门放在编排最前面、observer 计时登记之前：被跳过的周期对外必须
	// 表现为「tick 从未发生」——不发时长样本、不产生任何持久化副作用。
	// 门覆盖包括显式 `Step` 在内的全部推进点，任何调用方都无法在暂停期
	// 让世界时间前进；调度层的前置读只是省去暂停期对 `stepMu` 的争用，
	// 两处共享同一份原子位。
	if server.paused.Load() {
		return sim.TickResult{}
	}
	started := time.Now()
	if server.config.TickObserver != nil ||
		(server.config.ScheduledTickObserver != nil && !scheduled.IsZero()) {
		defer func() {
			duration := time.Since(started)
			if server.config.TickObserver != nil {
				server.config.TickObserver(duration)
			}
			if server.config.ScheduledTickObserver != nil && !scheduled.IsZero() {
				server.config.ScheduledTickObserver(scheduled, duration)
			}
		}()
	}

	server.drainSaveCompletions()
	trustedCenter, trustedSequence, hasTrustedCenter := server.drainTrustedObserverCenter()
	server.drainIncoming()
	chatDeliveries := server.drainIncomingChats()
	server.drainAcquired()
	server.drainGenerated()
	// 任务编排位于聊天 drain 之后（Accepted 指令刚入队即可同 tick 派发规划）、
	// engine.Step 之前（伙伴移动输入必须先进 inbox 才能被本 tick 消费）。
	taskDeliveries := server.advanceCompanionTasks()
	result := server.engine.Step()
	if server.companionManager != nil {
		// 采掘进度只在 TickResult.Companions 发布（CompanionBodies 不含采掘
		// 域）：tick 末回填缓存，下一 tick 的 advanceRunners 与 bodies 缓存
		// 同截面消费。
		server.companionManager.observeTickResult(result)
	}
	if server.companions != nil {
		server.companions.Observe(
			server.engine.CompanionBodies(),
			server.companionManagerTaskStates(),
			server.companionManagerSummaries(),
		)
		if err := server.companions.Poll(result.Tick); err != nil {
			slog.Warn("伙伴自动保存失败，保留重试", "error", err)
		}
	}
	if hasTrustedCenter {
		server.appliedTrustedObserver = appliedTrustedObserverCenter{
			dimension: trustedCenter.dimension,
			center:    trustedCenter.center,
			sequence:  trustedSequence,
		}
	}
	server.publishWithChats(result, append(chatDeliveries, taskDeliveries...))
	server.cancelUnwantedPending()
	server.appendChunkRequests(chunkJobLoad, result.Acquire)
	server.appendChunkRequests(chunkJobGenerate, result.Generate)
	server.schedulePersistence(result.Tick)
	server.scheduleMetadataSave(result.Tick, result.WorldTimeTicks)
	server.updatePersistenceBackpressure()
	if !server.backpressured {
		server.scheduleChunkJobs()
	}
	return result
}

// StepForTest 显式推进一个完整 tick，供无头确定性集成测试使用。
func (server *Server) StepForTest() sim.TickResult {
	return server.Step()
}

// Pause 置位权威暂停门。幂等：重复调用只是重复写入同一个布尔位，不排队、
// 不计数，因此不会产生额外的状态变化。
//
// 边界：本门冻结的是「权威模拟推进」这一事件本身，覆盖所有 tick 推进点
// （`RunTicks` 的调度周期与显式的 `Step` 调用），保证暂停期间任何调用方都
// 无法让世界时间前进；但它不关闭消息通路、不暂停 worker goroutine、也不
// 参与生命周期关服路径（`Shutdown` 照常工作）。暂停时长由人工交互决定，
// 期间不产生任何周期性工作。
func (server *Server) Pause() {
	server.paused.Store(true)
}

// Resume 清除权威暂停门：模拟从冻结时刻确定性续跑——恢复后的第一个 tick
// 恰好接在冻结前的最后一个已执行 tick 之后，暂停段不留下任何可观察痕迹。
// 幂等：对未暂停的世界重复调用同样无害。
func (server *Server) Resume() {
	server.paused.Store(false)
}

func (server *Server) RunTicks(ctx context.Context) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-server.ctx.Done():
			select {
			case <-server.closedDone:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		case scheduled := <-ticker.C:
			// 每个 ticker 到期先读暂停门：置位时本周期整个权威 tick 不存在
			// （世界时间、随机 tick、持久化调度都不发生），而不是空转一个
			// 降级 tick。真正的短路判定在 `step` 内共享，这里的前置读让
			// 暂停期完全不触碰 `stepMu`。
			if server.paused.Load() {
				continue
			}
			server.step(scheduled)
		}
	}
}

// Run 暂时保留为测试兼容包装；RunTicks 返回后执行既有安全关服。
func (server *Server) Run(ctx context.Context) error {
	runErr := server.RunTicks(ctx)
	if errors.Is(runErr, context.Canceled) ||
		errors.Is(runErr, context.DeadlineExceeded) {
		return server.shutdownAfterRunCancellation(runErr)
	}
	return runErr
}

func (server *Server) shutdownAfterRunCancellation(runErr error) error {
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		server.config.ShutdownTimeout,
	)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return runErr
}

func (server *Server) ChunkInfo(
	dimension core.DimensionID,
	pos core.ChunkPos,
) (sim.ChunkInfo, bool) {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.engine.ChunkInfo(core.ChunkKey{
		Dimension: dimension,
		Pos:       pos,
	})
}

func (server *Server) PlayerStateFor(id sim.SessionID) (sim.PlayerUpdate, bool) {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.engine.Player(id)
}

func (server *Server) PlayerSnapshotFor(id sim.SessionID) (sim.PlayerSnapshot, bool) {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.engine.PlayerSnapshot(id)
}

func (server *Server) TickCount() uint64 {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.engine.TickCount()
}

func (server *Server) ChunkHash(
	dimension core.DimensionID,
	pos core.ChunkPos,
) ([32]byte, uint64, bool) {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.engine.ChunkHash(core.ChunkKey{
		Dimension: dimension,
		Pos:       pos,
	})
}

func (server *Server) drainAcquired() {
	for {
		select {
		case result := <-server.acquired:
			server.engine.SubmitAcquired(result)
		default:
			return
		}
	}
}

func (server *Server) drainGenerated() {
	for {
		select {
		case result := <-server.generated:
			server.engine.SubmitGenerated(result)
		default:
			return
		}
	}
}

func (server *Server) appendChunkRequests(kind chunkJobKind, keys []core.ChunkKey) {
	for _, key := range keys {
		if _, exists := server.queued[key]; exists {
			continue
		}
		server.queued[key] = struct{}{}
		server.pending = append(server.pending, chunkJob{Kind: kind, Key: key})
	}
}

func (server *Server) cancelUnwantedPending() {
	if len(server.pending) == 0 {
		return
	}
	kept := server.pending[:0]
	for _, job := range server.pending {
		if !server.engine.WantsChunk(job.Key) {
			delete(server.queued, job.Key)
			continue
		}
		kept = append(kept, job)
	}
	server.pending = kept
}

func (server *Server) scheduleChunkJobs() {
	for len(server.pending) != 0 {
		job := server.pending[0]
		select {
		case server.jobs <- job:
			server.pending = server.pending[1:]
			delete(server.queued, job.Key)
		default:
			return
		}
	}
}
