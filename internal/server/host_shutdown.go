package server

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/sim/contract"
)

func (h *Host) collectSessionExit(
	active *activeLogin,
	session contract.SessionID,
	generation uint64,
	exit <-chan SessionExit,
	cause error,
) error {
	if cause != nil {
		h.world.DetachSession(session, generation, cause)
	}
	result := <-exit
	var observeErr error
	if result.HasSnapshot {
		observeErr = h.players.Observe(
			active.PlayerID,
			active.Name,
			result.Snapshot,
			h.world.TickCount(),
			true,
		)
	}
	if cause != nil {
		return errors.Join(cause, observeErr)
	}
	return errors.Join(result.Err, observeErr)
}

func (h *Host) Shutdown(ctx context.Context) error {
	if ctx == nil {
		panic("server: nil host shutdown context")
	}
	select {
	case <-h.shutdownGate:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { h.shutdownGate <- struct{}{} }()

	h.world.stepMu.Lock()
	closed := h.world.lifecycle == serverClosed
	h.world.stepMu.Unlock()
	if closed {
		_ = h.closeCompanionRuntime(ctx)
		h.players.CloseWorker()
		return nil
	}

	h.mu.Lock()
	h.closing = true
	listener := h.listener
	h.mu.Unlock()

	var listenerErr error
	if listener != nil {
		listenerErr = listener.Close()
	}
	if err := h.waitAcceptLoop(ctx); err != nil {
		return errors.Join(listenerErr, err)
	}
	h.closePendingLogins()
	if err := waitForHostWorkers(ctx, &h.pendingWG); err != nil {
		return errors.Join(listenerErr, err)
	}
	for _, active := range h.activeLogins() {
		h.world.DetachSession(active.Session, active.Generation, network.ErrClosed)
	}
	if err := waitForHostWorkers(ctx, &h.sessionWG); err != nil {
		return errors.Join(listenerErr, err)
	}
	if err := h.players.Flush(ctx); err != nil {
		return errors.Join(listenerErr, err)
	}

	h.mu.Lock()
	runtimeCancel := h.runtimeCancel
	h.mu.Unlock()
	if runtimeCancel != nil {
		runtimeCancel()
	}
	worldErr := h.world.Shutdown(ctx)
	h.world.stepMu.Lock()
	worldClosed := h.world.lifecycle == serverClosed
	h.world.stepMu.Unlock()
	if !worldClosed {
		return errors.Join(listenerErr, worldErr)
	}
	_ = h.closeCompanionRuntime(ctx)
	h.players.CloseWorker()
	return errors.Join(listenerErr, worldErr)
}

// closeCompanionRuntime 仅在世界持久化成功后由 Server 的 pre-close 钩子调用。
// Release 使用冻结 lease 与独立有界 context；无论 Release 成败，已持久化事实
// 都不会回滚，Agent/MCP 随后关闭且错误仍返回给调用方。
func (h *Host) closeCompanionRuntime(ctx context.Context) error {
	if h.companionRuntimeClosed {
		return nil
	}
	h.companionRuntimeClosed = true
	var releaseErr error
	if h.companionLease != nil && h.companionAgent != nil {
		lease, ok := h.companionLease.Freeze()
		if ok {
			requestID, err := h.companionLease.newID()
			if err != nil {
				releaseErr = companion.ErrAgentUnavailable
			} else {
				deadline := time.Now().Add(companionAgentReleaseTimeout)
				if callerDeadline, hasDeadline := ctx.Deadline(); hasDeadline && callerDeadline.Before(deadline) {
					deadline = callerDeadline
				}
				releaseContext, cancel := context.WithDeadline(ctx, deadline)
				response, callErr := h.companionAgent.Release(releaseContext, companion.LeaseRequest{
					ContractVersion:  companion.AgentContractVersion,
					RequestID:        requestID,
					ClientInstanceID: h.companionLease.clientInstanceID,
					NamespaceID:      h.companionLease.namespaceID,
					LeaseID:          lease.ID,
				})
				cancel()
				if callErr != nil || !response.Released {
					releaseErr = errors.Join(companion.ErrAgentUnavailable, callErr)
				}
			}
		}
	}
	if h.companionLease != nil {
		h.companionLease.Close()
	}
	if h.companionAgent != nil {
		h.companionAgent.Close()
	}
	if h.companionMCP != nil {
		h.companionMCP.Close()
	}
	return releaseErr
}

func (h *Host) waitAcceptLoop(ctx context.Context) error {
	return waitForHostWorkers(ctx, &h.acceptWG)
}

func (h *Host) closePendingLogins() {
	h.mu.Lock()
	ids := make([]uint64, 0, len(h.preLoginStreams))
	for id := range h.preLoginStreams {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	pending := make([]*pendingLoginStream, 0, len(ids))
	for _, id := range ids {
		login := h.preLoginStreams[id]
		pending = append(pending, &pendingLoginStream{
			stream: login.stream,
			cancel: login.cancel,
		})
	}
	h.mu.Unlock()
	for _, login := range pending {
		if login == nil {
			continue
		}
		if login.cancel != nil {
			login.cancel()
		}
		_ = login.stream.Close()
	}
}

func (h *Host) waitPendingLogins() {
	h.pendingWG.Wait()
}
