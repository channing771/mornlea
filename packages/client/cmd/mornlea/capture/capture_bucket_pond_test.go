package capture

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestBucketPondCaptureSceneIsRegistered 锁住 bucket-pond 场景条目的完整性：
// 与其余世界场景一样走「Prepare 装夹具 + Apply 定状态 + 8 帧预热」的完整链路。
func TestBucketPondCaptureSceneIsRegistered(t *testing.T) {
	scene := captureSceneByName(t, "bucket-pond")
	if scene.Prepare == nil || scene.Apply == nil {
		t.Fatalf("场景=%+v，想要完整 bucket-pond", scene)
	}
	if scene.WarmupFrames != 8 {
		t.Fatalf("bucket-pond WarmupFrames=%d，想要 8", scene.WarmupFrames)
	}
	if scene.Menu || scene.Settings != nil || scene.PinVolatile != nil {
		t.Fatalf("bucket-pond 不应携带菜单/设置/易变钉住夹具: %+v", scene)
	}
}

// TestPrepareBucketPondUsesMirrorAndMesher 核对水桶池塘夹具：空气邻域基线上的
// 草地支撑条，条上源水、空地、耕地各一格。源是取水目标，空地是放水落点，耕地
// 是湿度联动对照——三格同框即取放前后的视觉证据。
func TestPrepareBucketPondUsesMirrorAndMesher(t *testing.T) {
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := &application.Application{}
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)
	if err := prepareBucketPond(app); err != nil {
		t.Fatalf("准备水桶池塘夹具: %v", err)
	}
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			chunk, ok := app.Mirror().Chunk(core.Overworld, core.ChunkPos{X: x, Z: z})
			if !ok || chunk.Revision != 2 {
				t.Fatalf("chunk (%d,%d) = (%v,%v)，想要 revision 2", x, z, chunk, ok)
			}
		}
	}
	assertBlock := func(position core.BlockPos, want core.BlockID) {
		t.Helper()
		got, loaded := app.Mirror().BlockAt(core.Overworld, position)
		if !loaded || got != want {
			t.Fatalf("BlockAt(%+v) = (%d,%v)，想要 (%d,true)", position, got, loaded, want)
		}
	}
	// 支撑条其余部分仍是草地。
	for _, position := range []core.BlockPos{
		{X: -2, Y: 0, Z: -4}, {X: 2, Y: 0, Z: -2}, {X: 0, Y: 0, Z: -3},
	} {
		assertBlock(position, core.GrassID)
	}
	// 源+空地+耕地各一格。
	assertBlock(core.BlockPos{X: -1, Y: 0, Z: -3}, core.WaterSourceID)
	assertBlock(core.BlockPos{X: 0, Y: 1, Z: -3}, core.AirID)
	assertBlock(core.BlockPos{X: 1, Y: 0, Z: -2}, core.FarmlandWetID)
	// 空地正下方仍是草地支撑。
	assertBlock(core.BlockPos{X: 0, Y: 0, Z: -3}, core.GrassID)
	if got := app.Mesher().Stats().DirtySections; got == 0 {
		t.Fatal("水桶池塘装入后 mesher 没有 dirty section")
	}
}

// TestBucketPondApplyResetsSharedPresentationState 与水面斜坡场景同构：前序
// 场景留下的全部共享呈现状态都必须被 Apply 显式清空，正午时间与近景相机固定。
func TestBucketPondApplyResetsSharedPresentationState(t *testing.T) {
	scene := captureSceneByName(t, "bucket-pond")
	if scene.Apply == nil {
		t.Fatal("缺少 bucket-pond")
	}
	remotePlayers := client.NewRemotePlayers()
	if err := remotePlayers.Apply(network.RemotePlayerSpawn{
		PlayerID: core.PlayerID{6: 0x40, 8: 0x80, 15: 1}, DisplayName: "测试Player",
		ServerTick: 1, Position: mgl32.Vec3{0.5, 2, 0.5},
	}); err != nil {
		t.Fatal(err)
	}
	app := application.NewPresentationApplicationForTest()
	app.SetRemotePlayers(remotePlayers)
	app.Panel().SetVisible(true)
	app.SetInventoryOpen(true)
	inventory := core.Inventory{}
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.Inventory().Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	if err := app.Furnace().Apply(network.FurnaceState{Furnace: core.FurnaceRef{
		Dimension: core.Overworld, Generation: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := app.Chest().Apply(network.ChestState{Chest: core.ContainerRef{
		Dimension: core.Overworld, Kind: core.ContainerKindChest, Generation: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := scene.Apply(app); err != nil {
		t.Fatal(err)
	}
	if app.WorldTimeTicks() != 6000 {
		t.Fatalf("world time = %d，想要 6000", app.WorldTimeTicks())
	}
	if app.Camera().Pos != (mgl32.Vec3{0.5, 2.2, 1.5}) || app.Camera().Yaw != 0 || app.Camera().Pitch != -0.15 {
		t.Fatalf("camera = %+v yaw=%v pitch=%v", app.Camera().Pos, app.Camera().Yaw, app.Camera().Pitch)
	}
	if got, confirmed := app.Inventory().State(); !confirmed || got != (core.Inventory{}) {
		t.Fatalf("inventory = %+v confirmed=%v，想要已确认空物品栏", got, confirmed)
	}
	if got := app.RemotePlayers().Presentations(); len(got) != 0 {
		t.Fatalf("远端玩家未清空: %+v", got)
	}
	if _, opened := app.Furnace().State(); opened {
		t.Fatal("熔炉镜像未清空")
	}
	if _, opened := app.Chest().State(); opened {
		t.Fatal("箱子镜像未清空")
	}
	if app.InventoryOpen() || app.Panel().Visible() {
		t.Fatalf("共享界面状态未清空: inventoryOpen=%v panelVisible=%v",
			app.InventoryOpen(), app.Panel().Visible())
	}
}
