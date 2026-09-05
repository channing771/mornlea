//go:build darwin

package app

// app_drop_fall_test.go：掉落物支撑下落在帧装配侧的接线证明——app 层从只读
// 镜像向下扫描支撑高度并随掉落呈现输入传入 render；无数据或超深时保持生成
// 高度；会话重置后下落年龄重起。

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/world"
)

// applyFallMirrorDrop 把指定方块位置的泥土掉落物写入权威掉落物镜像。
func applyFallMirrorDrop(t *testing.T, app *Application, block core.BlockPos, slot uint8) {
	t.Helper()
	blockIndex, ok := world.ChunkBlockIndex(block)
	if !ok {
		t.Fatalf("掉落方块 %+v 不在区块索引内", block)
	}
	id := core.DropID{Dimension: core.Overworld, Chunk: block.Chunk(), Slot: slot, Generation: 1}
	if err := app.itemDrops.Apply(network.ItemDropUpserts{
		ServerTick: app.serverTick,
		Drops:      []network.ItemDrop{{ID: id, BlockIndex: blockIndex, Item: core.ItemDirt, Count: 1}},
	}); err != nil {
		t.Fatalf("布置掉落物镜像: %v", err)
	}
}

// dropStreamCenterY 解码掉落物实例流首个实例的中心高度。
func dropStreamCenterY(t *testing.T, stream []byte) float32 {
	t.Helper()
	if len(stream) < 60 {
		t.Fatalf("掉落物流=%d 字节，不足一个实例", len(stream))
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(stream[52:56]))
}

// TestApplicationDropSupportComesFromMirror 钉住支撑高度的镜像来源：草地
// 上方 3 格的掉落支撑为草顶（y=1），未加载列无支撑。
func TestApplicationDropSupportComesFromMirror(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	configureTargetFeedback(t, app)
	loadInteractiveBlock(t, app, core.BlockPos{X: 0, Y: 0, Z: 0}, core.GrassID)
	app.serverTick = 10
	applyFallMirrorDrop(t, app, core.BlockPos{X: 0, Y: 3, Z: 0}, 3)

	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.itemDropInstances) != 1 {
		t.Fatalf("掉落物呈现=%d，想要 1", len(app.itemDropInstances))
	}
	if got := app.itemDropInstances[0]; !got.HasSupport || got.SupportY != 1 {
		t.Fatalf("支撑=(%v,%v)，想要 (true,1)", got.HasSupport, got.SupportY)
	}

	// 同一应用换一列无镜像覆盖的掉落：支撑缺席即保持，不下落。
	app.itemDrops.Reset()
	app.serverTick = 11
	applyFallMirrorDrop(t, app, core.BlockPos{X: 64, Y: 3, Z: 64}, 4)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("RenderFrame=(%v,%v)", rendered, err)
	}
	if got := app.itemDropInstances[0]; got.HasSupport {
		t.Fatalf("未加载列支撑=%v，想要 false（保持生成高度）", got.HasSupport)
	}
}

// TestApplicationDropSupportScanIsBounded 钉住支撑扫描定界 16 格：16 格内
// 无不透明方块时按保持处理，不继续下探。
func TestApplicationDropSupportScanIsBounded(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	configureTargetFeedback(t, app)
	// 支撑放在 17 格深处（超出定界），扫描窗内全是已加载空气。
	loadInteractiveBlock(t, app, core.BlockPos{X: 0, Y: -14, Z: 0}, core.StoneID)
	app.serverTick = 10
	applyFallMirrorDrop(t, app, core.BlockPos{X: 0, Y: 3, Z: 0}, 3)

	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("RenderFrame=(%v,%v)", rendered, err)
	}
	if got := app.itemDropInstances[0]; got.HasSupport {
		t.Fatalf("超深支撑=%v，想要 false（定界 16 格外按保持处理）", got.HasSupport)
	}
}

// TestApplicationSessionResetRestartsDropFall 钉住会话重置重起下落年龄：
// 下落后重置再注入，呈现高度回到生成高度附近。
func TestApplicationSessionResetRestartsDropFall(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	configureTargetFeedback(t, app)
	loadInteractiveBlock(t, app, core.BlockPos{X: 0, Y: 0, Z: 0}, core.GrassID)
	app.serverTick = 10
	applyFallMirrorDrop(t, app, core.BlockPos{X: 0, Y: 3, Z: 0}, 3)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("RenderFrame=(%v,%v)", rendered, err)
	}

	app.serverTick = 20
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("下落帧 RenderFrame=(%v,%v)", rendered, err)
	}
	sunk := dropStreamCenterY(t, app.dropStream)

	app.resetSessionOwnedState()
	app.serverTick = 21
	applyFallMirrorDrop(t, app, core.BlockPos{X: 0, Y: 3, Z: 0}, 3)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("重置后 RenderFrame=(%v,%v)", rendered, err)
	}
	restored := dropStreamCenterY(t, app.dropStream)
	if restored-sunk < 1 {
		t.Fatalf("重置后高度=%v，下落后=%v，想要回到生成高度附近（差值>1）", restored, sunk)
	}
}
