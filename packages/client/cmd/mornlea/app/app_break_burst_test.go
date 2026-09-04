//go:build darwin

package app

// app_break_burst_test.go：破碎 burst 在帧装配侧的接线证明——新掉落物出现
// 的当帧 avatar 实例段多出 8 粒 burst，同 tick 重复帧逐字节一致，20 tick 后
// 到期消失，会话重置后同 ID 可重新 burst。

import (
	"bytes"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// applyBurstMirrorDrop 把一个泥土掉落物写入权威掉落物镜像。
func applyBurstMirrorDrop(t *testing.T, app *Application, slot uint8) {
	t.Helper()
	id := core.DropID{Dimension: core.Overworld, Chunk: core.ChunkPos{}, Slot: slot, Generation: 1}
	if err := app.itemDrops.Apply(network.ItemDropUpserts{
		ServerTick: app.serverTick,
		Drops:      []network.ItemDrop{{ID: id, BlockIndex: 7, Item: core.ItemDirt, Count: 1}},
	}); err != nil {
		t.Fatalf("布置掉落物镜像: %v", err)
	}
}

// TestApplicationBreakBurstMergesIntoAvatarStream 钉住帧接线：与掉落物本体
// 同输入的 burst 编码并入 avatar 实例段，新掉落物首帧恰多 8 实例。
func TestApplicationBreakBurstMergesIntoAvatarStream(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	configureTargetFeedback(t, app)
	app.serverTick = 10
	applyBurstMirrorDrop(t, app, 3)

	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("RenderFrame=(%v,%v)", rendered, err)
	}
	if got, want := len(app.avatarStream), 8*96; got != want {
		t.Fatalf("新掉落物帧 avatar 流=%d 字节，想要 burst 的 %d", got, want)
	}
	if got, want := len(app.dropStream), 96; got != want {
		t.Fatalf("掉落物流=%d 字节，想要本体 %d", got, want)
	}
	first := append([]byte(nil), app.avatarStream...)

	// 同 tick 下一帧逐字节一致：跟踪表不把存续掉落物误判为新 ID。
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("同 tick 重复 RenderFrame=(%v,%v)", rendered, err)
	}
	if !bytes.Equal(first, app.avatarStream) {
		t.Fatal("同 tick 两帧 avatar 流不一致，想要逐字节一致")
	}

	// 20 tick 后 burst 到期：avatar 段归零，掉落物本体仍在。
	app.serverTick = 30
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("到期帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.avatarStream) != 0 {
		t.Fatalf("到期后 avatar 流=%d 字节，想要 0", len(app.avatarStream))
	}
	if got, want := len(app.dropStream), 96; got != want {
		t.Fatalf("到期后掉落物流=%d 字节，想要本体 %d", got, want)
	}
}

// TestApplicationSessionResetRestartsBreakBursts 钉住会话重置清 burst 表：
// 重置后同 ID 再次出现视为首现，重 burst 一次（不清则年龄到期无 burst）。
func TestApplicationSessionResetRestartsBreakBursts(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	configureTargetFeedback(t, app)
	app.serverTick = 10
	applyBurstMirrorDrop(t, app, 3)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.avatarStream) != 8*96 {
		t.Fatalf("首现 avatar 流=%d 字节，想要 %d", len(app.avatarStream), 8*96)
	}

	app.resetSessionOwnedState()
	app.serverTick = 11
	applyBurstMirrorDrop(t, app, 3)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("重置后 RenderFrame=(%v,%v)", rendered, err)
	}
	if got, want := len(app.avatarStream), 8*96; got != want {
		t.Fatalf("重置后同 ID 首现 avatar 流=%d 字节，想要重 burst 的 %d", got, want)
	}
}
