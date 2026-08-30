package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/channing771/mornlea/internal/sim/runtime"
)

func waitForHostWorkers(ctx context.Context, workers *sync.WaitGroup) error {
	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type storeShutdownPhase uint8

const (
	storeShutdownNeedsSync storeShutdownPhase = iota
	storeShutdownNeedsClose
	storeShutdownClosed
)

// Shutdown freezes authority, drains persistence, and releases Store ownership.
// A failed call leaves the frozen Server retryable by a later call.
func (server *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		panic("server: nil shutdown context")
	}
	select {
	case <-server.shutdownGate:
	default:
		select {
		case <-server.shutdownGate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	defer func() { server.shutdownGate <- struct{}{} }()

	server.stepMu.Lock()
	if server.lifecycle == serverClosed {
		server.stepMu.Unlock()
		return nil
	}
	var freezeErr error
	if server.lifecycle == serverRunning {
		server.lifecycle = serverClosing
		tickTunables := runtime.ActiveTickTunables()
		freezeErr = server.world.Drain()
		trustedCenter, trustedSequence, hasTrustedCenter := server.drainTrustedObserverCenter()
		server.drainIncoming()
		_ = server.drainIncomingChats(tickTunables)
		server.drainAcquired()
		server.drainGenerated()
		// 关服顺序（spec：companion-task-queue）：聊天接受已随生命周期冻结与
		// 会话拆除停止，这里取消在途模型请求，再冻结队列与 actor 状态并做
		// 最终 AI 保存，最后才轮到世界存储的同步与关闭（见本函数末段）。
		if server.companionManager != nil {
			server.companionManager.beginShutdown()
		}
		if server.hostileManager != nil {
			server.hostileManager.beginShutdown()
		}
		server.engine.StepWithTunables(tickTunables)
		if server.companions != nil {
			server.companions.Observe(
				server.engine.CompanionBodies(),
				server.companionManagerTaskStates(),
				server.companionManagerSummaries(),
			)
		}
		if server.hostiles != nil {
			// 关服最终快照与伙伴同一时点冻结：末次 `engine.Step` 之后、会话
			// 拆除之前，目标等权威事实不因拆除而漂移。
			server.hostiles.Observe(server.engine.HostileMobs())
		}
		if hasTrustedCenter {
			server.appliedTrustedObserver = appliedTrustedObserverCenter{
				dimension: trustedCenter.dimension,
				center:    trustedCenter.center,
				sequence:  trustedSequence,
			}
		}
		for _, id := range server.sortedSessionIDsLocked() {
			current := server.sessions[id]
			server.detachSessionLocked(id, current.generation, nil)
		}
		if server.trustedObserver != nil {
			server.detachTrustedObserverLocked(
				server.trustedObserver.id,
				server.trustedObserver.generation,
				nil,
			)
		}
		server.cancel()
	}
	server.stepMu.Unlock()

	if err := waitForServerWorkers(ctx, server.runtimeDone); err != nil {
		return server.world.ShutdownContextError(err, freezeErr)
	}
	if freezeErr != nil {
		return server.persistenceErrorWithContext(freezeErr, ctx)
	}
	if err := server.world.Flush(ctx); err != nil {
		return err
	}
	if server.companions != nil {
		if err := server.companions.Flush(ctx); err != nil {
			return server.persistenceErrorWithContext(
				fmt.Errorf("flush companions: %w", err), ctx,
			)
		}
	}
	// 夜行者关服屏障失败同样整体失败：存档可能落后于权威事实，绝不谎报成
	// 功——可重试的关服状态由本函数的 phase 语义保留。
	if server.hostiles != nil {
		if err := server.hostiles.Flush(ctx); err != nil {
			return server.persistenceErrorWithContext(
				fmt.Errorf("flush hostiles: %w", err), ctx,
			)
		}
	}
	if server.storePhase == storeShutdownNeedsSync {
		if err := server.store.Sync(ctx); err != nil {
			return server.persistenceErrorWithContext(fmt.Errorf("sync world: %w", err), ctx)
		}
		server.storePhase = storeShutdownNeedsClose
	}
	if err := ctx.Err(); err != nil {
		return server.world.ShutdownContextError(err, nil)
	}
	if server.storePhase == storeShutdownNeedsClose {
		if err := server.store.Close(); err != nil {
			return server.persistenceErrorWithContext(fmt.Errorf("close world: %w", err), ctx)
		}
		server.storePhase = storeShutdownClosed
	}
	if server.companions != nil {
		server.companions.Close()
	}
	if server.hostiles != nil {
		server.hostiles.Close()
	}
	if server.companionManager != nil {
		server.companionManager.close()
	}
	if server.hostileManager != nil {
		server.hostileManager.close()
	}
	server.world.Close()

	server.stepMu.Lock()
	server.lifecycle = serverClosed
	close(server.closedDone)
	server.stepMu.Unlock()
	return nil
}

func (server *Server) persistenceErrorWithContext(err error, ctx context.Context) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return server.world.ShutdownContextError(ctxErr, err)
	}
	return err
}

func waitForServerWorkers(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
