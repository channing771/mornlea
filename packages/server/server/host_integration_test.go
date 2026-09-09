package server

import (
	"context"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/shared/network"
	networktcp "github.com/channing771/mornlea/packages/shared/network/tcp"
)

func TestHostTCPLoginDisconnectAndShutdown(t *testing.T) {
	store := newHostTestStore()
	host := newTestHostWithStore(t, store)
	listener, err := networktcp.ListenTCP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancelRun := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- host.Run(runCtx, listener) }()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), waitDeadline)
	stream, err := networktcp.DialTCP(dialCtx, listener.Addr())
	cancelDial()
	if err != nil {
		t.Fatal(err)
	}
	identity := playerIdentity(11)
	loginCtx, cancelLogin := context.WithTimeout(context.Background(), waitDeadline)
	client, err := network.LoginClient(loginCtx, stream, identity, 32)
	cancelLogin()
	if err != nil {
		t.Fatal(err)
	}
	waitReady(t, host, testLogin{Client: client, Identity: identity})
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	waitForNoActiveLogin(t, host)

	cancelRun()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run shutdown error = %v", err)
		}
	case <-time.After(waitDeadline):
		t.Fatal("Run did not complete TCP shutdown")
	}
	if store.syncCount() != 1 || store.closeCount() != 1 {
		t.Fatalf("TCP host store shutdown counts = sync %d close %d", store.syncCount(), store.closeCount())
	}
}

// TestHostStoresNegotiatedViewDistanceOnActiveLogin 钉死 v40 登录协商的
// 接纳半部：客户端声明的域内视距经 `PendingLogin` 抵达 host 后必须原样
// 存入会话侧 activeLogin，供后续订阅半径换算消费；host 自身在此不钳制、
// 不解释。登录成功应答在 promote 之后才发出，因此断言无需再等就绪。
func TestHostStoresNegotiatedViewDistanceOnActiveLogin(t *testing.T) {
	host := newTestHost(t)
	runCtx, cancelRun := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- host.Run(runCtx, nil) }()
	t.Cleanup(func() {
		cancelRun()
		select {
		case err := <-runDone:
			if err != nil {
				t.Errorf("Host Run cleanup: %v", err)
			}
		case <-time.After(waitDeadline):
			t.Error("Host Run cleanup timed out")
		}
	})

	identity := playerIdentity(12)
	clientStream, serverStream := network.NewMemoryStreamPair(64)
	done := make(chan error, 1)
	go func() { done <- host.AcceptStream(context.Background(), serverStream) }()
	client, err := network.LoginClient(context.Background(), clientStream, identity, 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	active := activeLoginForPlayer(t, host, identity.PlayerID)
	if got := active.ViewDistance; got != 8 {
		t.Fatalf("会话侧视距 = %d，想要登录时声明的 8", got)
	}
}

// TestNewHostWiresEngineSeedFromStoreMetadata 断言 NewHost 把
// store.Metadata().Seed 原样传给了 runtime.NewEngine：host.world.engine 的种子
// 必须与存档 metadata 的种子一致。newHostTestStore 用非零种子 42，避免和
// 「接线断了、engine 悄悄退回默认零值」这种失败混淆。
func TestNewHostWiresEngineSeedFromStoreMetadata(t *testing.T) {
	store := newHostTestStore()
	host := newTestHostWithStore(t, store)
	wantSeed := store.Metadata().Seed
	if wantSeed == 0 {
		t.Fatal("测试种子为 0，无法与默认零值区分")
	}
	if got := host.world.engine.SeedForTest(); got != wantSeed {
		t.Fatalf("engine 种子 = %d，想要 %d", got, wantSeed)
	}
}
