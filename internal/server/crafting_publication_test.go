package server_test

// crafting_publication_test.go：合成网格状态的会话归属——网格发布只达所属
// session，绝不广播给其他在线玩家（spec authoritative-grid-crafting 的
// Requirement「网格状态私有同步且有界」Scenario「状态只发所属玩家」）。

import (
	"context"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/server"
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestCraftingStateStaysWithOwningSession 锁死网格发布的私有性：两名玩家各自
// 收到恰好一条登录初始网格状态；此后只有真正执行网格命令的玩家甲收到新的
// 完整网格状态，玩家乙在甲摆料期间不得收到任何网格发布。
func TestCraftingStateStaysWithOwningSession(t *testing.T) {
	firstClient, firstServer := network.NewMemoryPair(256)
	secondClient, secondServer := network.NewMemoryPair(256)
	// 甲的快捷栏 0（统一视图格 9）带一栈石头供摆料；乙从空背包起步。
	running := newMemoryAttachedWorldWithHotbar(
		hotbarTestConfig(2), firstServer, server.FlatTestGenerator{},
		stockedTestHotbar(core.ItemStone),
	)
	if _, err := running.AttachSession(externalSessionSpec(2, 1, secondServer, contract.PlayerRestore{
		SpawnDimension: core.Overworld,
	})); err != nil {
		t.Fatalf("附加第二个会话: %v", err)
	}
	shutdownHotbarServer(t, running, firstClient, secondClient)

	firstReady, secondReady := false, false
	firstInitial, secondInitial := 0, 0
	deadline := time.Now().Add(waitDeadline)
	// 阶段 0 等待两人 Ready（各自恰好一条初始网格状态）；阶段 1 验证甲摆料、
	// 乙不受影响。
	stage := 0
	for {
		if time.Now().After(deadline) {
			t.Fatal("等待两名玩家网格同步超时")
		}
		result := running.StepForTest()
		firstGrids := drainCraftingTick(t, firstClient, result.Tick, &firstReady)
		secondGrids := drainCraftingTick(t, secondClient, result.Tick, &secondReady)
		switch stage {
		case 0:
			firstInitial += len(firstGrids)
			secondInitial += len(secondGrids)
			if !firstReady || !secondReady {
				continue
			}
			stage = 1
			sendClientMessage(t, firstClient, network.MoveCraftingStack{
				Sequence: 1, From: 9, To: 0,
			})
		case 1:
			if len(secondGrids) != 0 {
				t.Fatalf("玩家乙收到了不属于自己的网格发布: %+v", secondGrids)
			}
			if len(firstGrids) == 0 {
				continue
			}
			want := network.CraftingState{Size: 2}
			want.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}
			if got := firstGrids[len(firstGrids)-1]; got != want {
				t.Fatalf("玩家甲网格 = %+v，想要 %+v", got, want)
			}
			if firstInitial != 1 || secondInitial != 1 {
				t.Fatalf(
					"初始网格状态数 = 甲 %d / 乙 %d，想要各恰好一条",
					firstInitial, secondInitial,
				)
			}
			return
		}
	}
}

// drainCraftingTick 读取一个会话到指定 tick 为止的全部消息，收集其中的网格
// 状态并跟踪 Ready 标志；其他定向消息（快照、物品、远端玩家等）静默跳过。
func drainCraftingTick(
	t *testing.T,
	endpoint network.ClientEndpoint,
	throughTick uint64,
	ready *bool,
) []network.CraftingState {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	var grids []network.CraftingState
	for {
		message, err := endpoint.Recv(ctx)
		if err != nil {
			t.Fatalf("接收服务端消息: %v", err)
		}
		switch message := message.(type) {
		case network.CraftingState:
			grids = append(grids, message)
		case network.CommandRejected:
			t.Fatalf("权威命令被拒绝: %+v", message)
		case network.PlayerState:
			*ready = message.Ready
			if message.ServerTick == throughTick {
				return grids
			}
			if message.ServerTick > throughTick {
				t.Fatalf("PlayerState tick=%d，跳过目标 tick=%d", message.ServerTick, throughTick)
			}
		}
	}
}
