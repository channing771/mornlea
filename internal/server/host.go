package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/server/persistence"
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/storage"
)

const hostPreLoginCapacity = 16

type Host struct {
	config  Config
	world   *Server
	players *persistence.Players

	preLogin           chan struct{}
	mu                 sync.Mutex
	activeByPlayer     map[core.PlayerID]*activeLogin
	activeBySession    map[contract.SessionID]*activeLogin
	preLoginStreams    map[uint64]*pendingLoginStream
	nextPreLogin       uint64
	nextSession        contract.SessionID
	nextGeneration     uint64
	listener           network.Listener
	companionSnapshots *companion.SnapshotRegistry
	companionMCP       *companionMCPService
	companionAgent     companionAgentRuntimeClient
	companionLease     *companionAgentLeaseController
	runtimeCancel      context.CancelFunc
	runtimeDone        chan error
	acceptWG           sync.WaitGroup
	pendingWG          sync.WaitGroup
	sessionWG          sync.WaitGroup
	shutdownGate       chan struct{}
	closing            bool
}

// HostStats 是不暴露内部 map/channel 的瞬时有界队列快照。
type HostStats struct {
	ActivePlayers         int
	MaxSessionOutboxDepth int
	PlayerSaveJobDepth    int
	PlayerSaveDoneDepth   int
}

// Stats 分别短暂取得 host、world 与 player persistence 锁；从不嵌套持锁。
func (h *Host) Stats() HostStats {
	h.mu.Lock()
	stats := HostStats{ActivePlayers: len(h.activeBySession)}
	h.mu.Unlock()

	h.world.stepMu.Lock()
	for _, current := range h.world.sessions {
		if current != nil {
			stats.MaxSessionOutboxDepth = max(stats.MaxSessionOutboxDepth, len(current.outbox))
		}
	}
	h.world.stepMu.Unlock()

	stats.PlayerSaveJobDepth, stats.PlayerSaveDoneDepth = h.players.QueueDepths()
	return stats
}

// RunAtInputBoundary 在完整 world tick 之间执行 action，并等待 wantPlayers 个不同
// session 的指定输入序号进入 world ingress。action 不得调用会再次取得 world step 锁的方法。
func (h *Host) RunAtInputBoundary(
	ctx context.Context,
	sequence uint64,
	wantPlayers int,
	action func() error,
) error {
	if ctx == nil {
		return errors.New("server: nil input boundary context")
	}
	if action == nil {
		return errors.New("server: nil input boundary action")
	}
	if sequence == 0 || wantPlayers <= 0 {
		return fmt.Errorf(
			"server: invalid input boundary sequence=%d players=%d",
			sequence, wantPlayers,
		)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	h.world.stepMu.Lock()
	defer h.world.stepMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	boundary := newInputIngressBoundary(sequence, wantPlayers)
	if !h.world.inputBoundary.CompareAndSwap(nil, boundary) {
		return errors.New("server: input boundary already active")
	}
	defer h.world.inputBoundary.CompareAndSwap(boundary, nil)
	if err := action(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-boundary.done:
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-h.world.ctx.Done():
		return h.world.ctx.Err()
	}
}

// Pause 把权威暂停门经宿主转发到内持世界。单机交互客户端经 cmd/mornlea 的
// `app.Host` 接口只持有 `*Host`，这是暂停门唯一的宿主暴露面——不另
// 设取回内持 `*Server` 的通道，冻结与恢复的全部语义（整个权威 tick 不存在、
// 消息通路存活、关服路径不受门影响）以 server 包 `paused` 字段注释为单一落点。
// 幂等：重复置位只是重复写同一原子位。
func (h *Host) Pause() {
	h.world.Pause()
}

// Resume 清除宿主转发的暂停门：模拟从冻结时刻确定性续跑。幂等：对未暂停的
// 世界重复调用同样无害。
func (h *Host) Resume() {
	h.world.Resume()
}

func NewHost(
	ctx context.Context,
	config Config,
	generator Generator,
	store storage.WorldStore,
) (*Host, error) {
	if ctx == nil {
		return nil, errors.New("server: nil host constructor context")
	}
	config.validate()
	if store == nil {
		panic("server: nil host store")
	}
	if generator == nil {
		panic("server: nil generator")
	}
	companions, err := bootstrapCompanionPersistence(ctx, config, store)
	if err != nil {
		return nil, err
	}
	// 夜行者聚合存档与伙伴配置解耦，凡世界存储都参与启动矩阵：missing 视同
	// 空集合；损坏/未来版本/读取失败在此整体拒绝（tick 与路径 worker 都不会
	// 启动），旧文件由存储层保持原样，重启不可能成为清怪手段。加载后的记录
	// 由 newWorld 在首 tick 前经 `RestoreHostile` 接线（存储校验矩阵覆盖
	// sim 侧全部不变量，重复/超限集合在加载边界已被拒绝，恢复失败属不可达
	// 防御路径）。
	loadedHostiles, err := store.LoadHostileMobs(ctx)
	if errors.Is(err, storage.ErrHostileMobsNotFound) {
		loadedHostiles = storage.StoredHostileMobs{}
	} else if err != nil {
		if companions != nil {
			companions.Close()
		}
		return nil, fmt.Errorf("load hostiles: %w", err)
	}
	var companionSnapshots *companion.SnapshotRegistry
	var companionMCP *companionMCPService
	var companionAgent companionAgentRuntimeClient
	var companionLease *companionAgentLeaseController
	planner := config.companionPlanner
	dialogue := config.companionDialogue
	if len(config.Companions) != 0 {
		if planner == nil {
			factory := config.companionAgentClientFactory
			if factory == nil {
				factory = func(settings companion.AgentServiceSettings, credential string) (companionAgentRuntimeClient, error) {
					return companion.NewAgentClient(settings, credential, nil)
				}
			}
			companionAgent, err = factory(config.AgentService, config.AgentCredential)
			if err == nil && companionAgent == nil {
				err = errors.New("nil companion Agent client")
			}
			if err != nil {
				if companions != nil {
					companions.Close()
				}
				return nil, fmt.Errorf("server: 启动 companion Agent client: %w", err)
			}
			newID := config.companionAgentIDGenerator
			if newID == nil {
				newID = newAgentRequestID
			}
			clientInstanceID, identityErr := newID()
			if identityErr != nil {
				companionAgent.Close()
				companions.Close()
				return nil, fmt.Errorf("server: 生成 Agent client instance identity: %w", identityErr)
			}
			namespaceID := companion.ID(companions.AgentNamespaceID()).String()
			companionLease = newCompanionAgentLeaseController(agentLeaseControllerOptions{
				Client: companionAgent, ClientInstanceID: clientInstanceID,
				NamespaceID: namespaceID, HeartbeatEvery: companionAgentHeartbeatEvery,
				LeaseTTL: companionAgentLeaseTTL, NewID: newID,
			})
		}
		companionSnapshots = companion.NewSnapshotRegistry()
		factory := config.companionMCPFactory
		if factory == nil {
			factory = newCompanionMCPService
		}
		companionMCP, err = factory(companionSnapshots)
		if err == nil && companionMCP == nil {
			err = errors.New("nil companion MCP service")
		}
		if err != nil {
			if companionMCP != nil {
				companionMCP.Close()
			} else {
				companionSnapshots.Close()
			}
			if companionLease != nil {
				companionLease.Close()
			}
			if companionAgent != nil {
				companionAgent.Close()
			}
			if companions != nil {
				companions.Close()
			}
			return nil, fmt.Errorf("server: 启动 companion MCP: %w", err)
		}
		if planner == nil {
			clientInstanceID := companionLease.clientInstanceID
			namespaceID := companionLease.namespaceID
			planner = newCompanionAgentPlanner(agentPlannerOptions{
				Client: companionAgent, Lease: companionLease, Registry: companionSnapshots,
				MCPEndpoint: companionMCP.Endpoint(), ClientInstanceID: clientInstanceID,
				NamespaceID: namespaceID, NewID: companionLease.newID,
			})
		}
	}
	hostiles := persistence.NewHostiles(store, loadedHostiles, persistenceOptions(config, nil))
	world, err := newWorld(config, generator, store, companions, hostiles, planner, dialogue)
	if err != nil {
		// 持久化 worker 已随构造启动；恢复/装配阶段的任何失败都必须先停掉
		// worker 再返回，否则每次启动失败都泄漏一个永不退出的 goroutine。
		hostiles.Close()
		if companionMCP != nil {
			companionMCP.Close()
		}
		if companionLease != nil {
			companionLease.Close()
		}
		if companionAgent != nil {
			companionAgent.Close()
		}
		if companions != nil {
			companions.Close()
		}
		return nil, err
	}
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return &Host{
		config:             config,
		world:              world,
		players:            persistence.NewPlayers(store, persistenceOptions(config, nil)),
		preLogin:           make(chan struct{}, hostPreLoginCapacity),
		activeByPlayer:     make(map[core.PlayerID]*activeLogin),
		activeBySession:    make(map[contract.SessionID]*activeLogin),
		preLoginStreams:    make(map[uint64]*pendingLoginStream),
		companionSnapshots: companionSnapshots,
		companionMCP:       companionMCP,
		companionAgent:     companionAgent,
		companionLease:     companionLease,
		runtimeDone:        make(chan error, 1),
		shutdownGate:       gate,
	}, nil
}

func bootstrapCompanionPersistence(
	ctx context.Context,
	config Config,
	store storage.WorldStore,
) (*persistence.Companions, error) {
	generate := config.companionIdentityGenerator
	if generate == nil {
		generate = storage.GenerateCompanionIdentity
	}

	if len(config.Companions) == 0 {
		exists, err := store.CompanionsExist(ctx)
		if err != nil {
			return nil, fmt.Errorf("probe companions: %w", err)
		}
		if !exists {
			return nil, nil
		}
		loaded, err := store.LoadCompanions(ctx)
		if err != nil {
			return nil, fmt.Errorf("load companions: %w", err)
		}
		merged, changed, err := storage.MergeCompanionsV5(loaded, nil, generate)
		if err != nil {
			return nil, fmt.Errorf("merge companions: %w", err)
		}
		if changed {
			if err := store.SaveCompanions(ctx, companionSaveFromStored(merged)); err != nil {
				return nil, fmt.Errorf("save companion bootstrap: %w", err)
			}
		}
		return nil, nil
	}

	// 伙伴启用即要求 Agent 运行时配置完整。该静态边界必须先于任何伙伴
	// 存档读取，避免坏配置触发 I/O 或启动后台资源。
	if err := config.AgentService.Validate(); err != nil {
		return nil, fmt.Errorf("server: 伙伴配置缺少可用的 agentService: %w", err)
	}
	if config.AgentCredential == "" {
		return nil, errors.New("server: Agent credential 为空")
	}
	if err := companion.ValidateTaskTimeoutMinutes(config.TaskTimeoutMinutes); err != nil {
		return nil, fmt.Errorf("server: taskTimeoutMinutes: %w", err)
	}

	loaded, err := store.LoadCompanions(ctx)
	if errors.Is(err, storage.ErrCompanionsNotFound) {
		loaded = storage.StoredCompanions{}
	} else if err != nil {
		return nil, fmt.Errorf("load companions: %w", err)
	}
	metadata := store.Metadata()
	active := make([]companion.Body, len(config.Companions))
	for index, definition := range config.Companions {
		active[index] = companion.Body{
			ID:        definition.ID,
			Dimension: metadata.SpawnDimension,
			Position: [3]float32{
				float32(metadata.SpawnAnchor.X*core.SectionSize) + 0.5,
				core.MaxY + 1,
				float32(metadata.SpawnAnchor.Z*core.SectionSize) + 0.5,
			},
		}
	}
	merged, changed, err := storage.MergeCompanionsV5(loaded, active, generate)
	if err != nil {
		return nil, fmt.Errorf("merge companions: %w", err)
	}
	if changed {
		if err := store.SaveCompanions(ctx, companionSaveFromStored(merged)); err != nil {
			return nil, fmt.Errorf("save companion bootstrap: %w", err)
		}
	}
	return persistence.NewCompanions(store, merged, persistenceOptions(config, nil)), nil
}

func companionSaveFromStored(stored storage.StoredCompanions) storage.CompanionSave {
	return storage.CompanionSave{
		Revision:         stored.Revision,
		AgentNamespaceID: stored.AgentNamespaceID,
		Records:          stored.Records,
		Lifecycles:       stored.Lifecycles,
		Queues:           stored.Queues,
	}
}

func (h *Host) Run(ctx context.Context, listener network.Listener) error {
	if ctx == nil {
		panic("server: nil host run context")
	}
	h.mu.Lock()
	if h.closing {
		h.mu.Unlock()
		return network.ErrClosed
	}
	if h.runtimeCancel != nil {
		h.mu.Unlock()
		return errors.New("server: host is already running")
	}
	runCtx, cancel := context.WithCancel(context.Background())
	h.listener = listener
	h.runtimeCancel = cancel
	if listener != nil {
		h.acceptWG.Add(1)
	}
	h.mu.Unlock()
	go func() { h.runtimeDone <- h.world.RunTicks(runCtx) }()
	if listener != nil {
		go func() {
			defer h.acceptWG.Done()
			h.acceptLoop(runCtx, listener)
		}()
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-h.runtimeDone:
			h.mu.Lock()
			closing := h.closing
			h.mu.Unlock()
			if closing && (errors.Is(err, context.Canceled) || err == nil) {
				return nil
			}
			return err
		case <-ticker.C:
			h.pollPlayers()
		case <-ctx.Done():
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), h.config.ShutdownTimeout)
			err := h.Shutdown(shutdownCtx)
			shutdownCancel()
			return err
		}
	}
}

func (h *Host) pollPlayers() {
	tick := h.world.TickCount()
	for _, active := range h.activeLogins() {
		if snapshot, ok := h.world.PlayerSnapshotFor(active.Session); ok {
			if err := h.players.Observe(active.PlayerID, active.Name, snapshot, tick, false); err != nil {
				slog.Warn("观察在线玩家快照失败", "error", err)
			}
		}
	}
	if err := h.players.Poll(tick); err != nil {
		slog.Warn("玩家自动保存失败，保留重试", "error", err)
	}
}
