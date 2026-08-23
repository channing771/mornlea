package server

import (
	"testing"

	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/sim"
)

func TestPublishLocalResultRoutesPlacementSuccessOnlyToInitiatingSession(t *testing.T) {
	current := &session{id: 1, outbox: make(chan network.ServerMessage, 2)}
	(&Server{}).publishLocalResult(current, sim.TickResult{
		PlacementSuccesses: []sim.PlacementSuccess{
			{Session: 2, Sequence: 20},
			{Session: 1, Sequence: 10},
		},
	}, sim.PlayerUpdate{})

	if len(current.outbox) != 1 {
		t.Fatalf("本会话放置成功消息数=%d，想要 1", len(current.outbox))
	}
	message := <-current.outbox
	if message != (network.PlaceBlockSucceeded{Sequence: 10}) {
		t.Fatalf("本会话放置成功消息=%#v", message)
	}
}

func TestPublishLocalResultRejectedPlacementHasNoSuccess(t *testing.T) {
	current := &session{id: 1, outbox: make(chan network.ServerMessage, 2)}
	(&Server{}).publishLocalResult(current, sim.TickResult{
		Rejected: []sim.Rejection{{
			Session: 1, Sequence: 10, Reason: sim.RejectOccupied,
		}},
	}, sim.PlayerUpdate{})

	if len(current.outbox) != 1 {
		t.Fatalf("拒绝放置消息数=%d，想要 1", len(current.outbox))
	}
	if message, ok := (<-current.outbox).(network.CommandRejected); !ok ||
		message != (network.CommandRejected{Sequence: 10, Reason: network.RejectOccupied}) {
		t.Fatalf("拒绝放置消息=%#v", message)
	}
}
