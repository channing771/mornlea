package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/sim"
	"github.com/channing771/mornlea/internal/sim/contract"
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
	sessions                  map[contract.SessionID]*session
	playerSessions            map[core.PlayerID]contract.SessionID
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
	hostileManager   *hostileManager
	inputBoundary    atomic.Pointer[inputIngressBoundary]
	jobs             chan chunkJob
	acquired         chan contract.AcquiredChunk
	generated        chan contract.GeneratedChunk
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
	hostiles        *hostilePersistence
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
	// benchmark 形态不接夜行者持久化（世界随进程生存，无跨重启恢复语义）；
	// newWorld 的错误只可能来自需要持久化装配的路径，这里不可达，防御口径
	// 与伙伴 planner 构造的 panic 一致。
	server, err := newWorld(config, generator, store, nil, nil)
	if err != nil {
		panic("server: NewWorld: " + err.Error())
	}
	return server
}

func newWorld(
	config Config,
	generator Generator,
	store storage.Store,
	companions *companionPersistence,
	hostiles *hostilePersistence,
) (*Server, error) {
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
		sessions:        make(map[contract.SessionID]*session),
		playerSessions:  make(map[core.PlayerID]contract.SessionID),
		ctx:             ctx,
		cancel:          cancel,
		saveCtx:         saveCtx,
		cancelSaves:     cancelSaves,
		incoming:        make(chan incomingCommand, inputCapacity),
		incomingChats:   make(chan incomingChat, inputCapacity),
		jobs:            make(chan chunkJob, queueCapacity),
		acquired:        make(chan contract.AcquiredChunk, queueCapacity),
		generated:       make(chan contract.GeneratedChunk, queueCapacity),
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
	// 显示相位偏移与绝对世界时间同源于世界 metadata：装配阶段在首个 tick 之前
	// 恢复，其后的唯一写者是 sim 的跳夜结算。偏移只进入
	// `(WorldTimeTicks + DayPhaseOffset) % 24000`，对 u64 存储值按周期长度取模
	// 再收窄，正常存档（0..23999）逐值不变，异常旧值也保持显示相位等价。
	//
	// 与 wire 侧的语义分界：这里（存储装配侧）对越界旧值**宽容归一**——存档
	// 是历史载体，折回周期内即可延续既有显示相位；而协议 `PlayerState` 的
	// `DayPhaseOffset` **严格拒绝** >23999——wire 只传播权威单值，没有历史
	// 包袱。两侧策略不同是刻意的，装配归一后的值随后经 wire 下发时必然合法。
	server.engine.RestoreDayPhaseOffset(uint16(metadata.DayPhaseOffset % core.DayLengthTicks))
	if companions != nil {
		companions.mu.Lock()
		records := slices.Clone(companions.records)
		loadedQueues := cloneStoredQueues(companions.loadedQueues)
		companions.mu.Unlock()
		for _, definition := range config.Companions {
			restore := contract.CompanionRestore{
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
	// 夜行者有界追逐编排独立于伙伴配置存在；目标事实同样经会话注册表注入，
	// manager 只消费这一权威源。
	server.hostileManager = newHostileManager(server.engine)
	server.hostileManager.onlinePlayers = server.onlineHostileTargets
	// 夜行者恢复接线：启动矩阵已在 NewHost 完成（corrupt/future/read error
	// 到不了这里），存档记录在首 tick 前整体回到权威侧。存储校验矩阵覆盖
	// sim 侧全部记录不变量（重复/超限在加载边界已被拒），恢复失败属不可达
	// 防御路径——绝不允许以「跳过该条」的截断姿态静默继续。
	if hostiles != nil {
		hostiles.mu.Lock()
		hostileRecords := slices.Clone(hostiles.records)
		hostiles.mu.Unlock()
		for _, record := range hostileRecords {
			if err := server.engine.RestoreHostile(hostileRestoreRecord(record)); err != nil {
				return nil, fmt.Errorf("restore hostile %d: %w", record.ID, err)
			}
		}
		server.hostiles = hostiles
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
	return server, nil
}

func (server *Server) AttachSession(spec SessionSpec) (<-chan SessionExit, error) {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.attachSessionLocked(spec)
}

func (server *Server) DetachSession(
	id contract.SessionID,
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
	id contract.SessionID,
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
func (server *Server) Step() contract.TickResult {
	return server.step(time.Time{})
}

func (server *Server) step(scheduled time.Time) contract.TickResult {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	if server.lifecycle != serverRunning {
		return contract.TickResult{}
	}
	// 暂停门放在编排最前面、observer 计时登记之前：被跳过的周期不发时长
	// 样本、无持久化副作用。这里与 `RunTicks` 的前置读共享同一份原子位，
	// 因此包括显式 `Step` 在内的全部推进点都受同一道门约束。
	if server.paused.Load() {
		return contract.TickResult{}
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
	// 夜行者编排同样先于 engine.Step：有界追逐的移动/攻击意图必须先进
	// inbox 才能被同 tick 的夜行者阶段消费；派发绝不等待 A*。
	server.advanceHostileChase()
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
	// 夜行者持久化与伙伴同一节奏：tick 边界观察权威排序快照并回收保存完成，
	// autosave 到点才派发；保存异步进行，tick 绝不等待落盘。
	if server.hostiles != nil {
		server.hostiles.Observe(server.engine.HostileMobs())
		if err := server.hostiles.Poll(result.Tick); err != nil {
			slog.Warn("夜行者自动保存失败，保留重试", "error", err)
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
func (server *Server) StepForTest() contract.TickResult {
	return server.Step()
}

// Pause 置位权威暂停门。幂等：重复调用只是重复写入同一个布尔位，不排队、
// 不计数，因此不会产生额外的状态变化。门覆盖所有 tick 推进点（`RunTicks`
// 的调度周期与显式的 `Step` 调用），保证暂停期间任何调用方都无法让世界
// 时间前进；关服路径不经本门，`Shutdown` 行为不受影响。
//
// 并发边界：对与在途 tick 并发的调用方，`Pause()` 返回时已经开始执行的
// 一个 step 仍会跑完本轮编排，其后的周期才被短路——这是门不持锁的固有
// 语义。
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
			// 每个 ticker 到期先读暂停门：置位则整段跳过本周期、不进入编排，
			// 让暂停期完全不触碰 `stepMu`（跳过语义见 `step` 内注释）。
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
) (contract.ChunkInfo, bool) {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.engine.ChunkInfo(core.ChunkKey{
		Dimension: dimension,
		Pos:       pos,
	})
}

func (server *Server) PlayerStateFor(id contract.SessionID) (contract.PlayerUpdate, bool) {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.engine.Player(id)
}

func (server *Server) PlayerSnapshotFor(id contract.SessionID) (contract.PlayerSnapshot, bool) {
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
