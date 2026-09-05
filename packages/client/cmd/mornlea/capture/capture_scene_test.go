package capture

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/world"
)

func TestCaptureSkylightTunnelFixtureUsesMirrorAndMesher(t *testing.T) {
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := &application.Application{}
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)

	if err := prepareSkylightTunnel(app); err != nil {
		t.Fatal(err)
	}
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			chunk, ok := app.Mirror().Chunk(core.Overworld, core.ChunkPos{X: x, Z: z})
			if !ok || chunk.Revision != 2 {
				t.Fatalf("chunk (%d,%d) = (%v,%v)，想要 revision 2", x, z, chunk, ok)
			}
		}
	}
	for _, tc := range []struct {
		name     string
		position core.BlockPos
		want     core.BlockID
	}{
		{name: "入口露天", position: core.BlockPos{X: 0, Y: 5, Z: 4}, want: core.AirID},
		{name: "入口地面", position: core.BlockPos{X: 0, Y: 0, Z: 4}, want: core.StoneID},
		{name: "入口侧墙", position: core.BlockPos{X: -3, Y: 2, Z: 4}, want: core.StoneID},
		{name: "入口后屋顶", position: core.BlockPos{X: 0, Y: 5, Z: 3}, want: core.StoneID},
		{name: "深处空气", position: core.BlockPos{X: 0, Y: 2, Z: -15}, want: core.AirID},
		{name: "通道后墙", position: core.BlockPos{X: 0, Y: 2, Z: -16}, want: core.StoneID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, loaded := app.Mirror().BlockAt(core.Overworld, tc.position)
			if !loaded || got != tc.want {
				t.Fatalf("BlockAt(%+v) = (%d,%v)，想要 (%d,true)", tc.position, got, loaded, tc.want)
			}
		})
	}
	if got := app.Mesher().Stats().DirtySections; got != 9*core.SectionsPerChunk {
		t.Fatalf("dirty sections = %d，想要 %d", got, 9*core.SectionsPerChunk)
	}
}

// TestContainerCaptureFixturesUseIndependentConfirmedMirrors 锁住两个容器场景的
// 已确认镜像、统一来源索引和跨场景 reset；漏掉任一状态都会让下一张 golden
// 继承上一个容器或实体。

func TestCaptureMaterialsShowcaseFixtureUsesMirrorAndMesher(t *testing.T) {
	var scene captureScene
	for _, candidate := range captureScenes {
		if candidate.Name == "materials-showcase" {
			scene = candidate
			break
		}
	}
	if scene.Prepare == nil {
		t.Fatal("materials-showcase 缺少 Prepare")
	}

	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := &application.Application{}
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)
	if err := scene.Prepare(app); err != nil {
		t.Fatal(err)
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
	materials := [...]core.BlockID{
		core.CobblestoneID, core.SmoothStoneID, core.SandID, core.GravelID,
		core.OakLogID, core.OakPlanksID, core.LeavesID, core.GlassID,
		core.BrickID, core.WhiteWoolID, core.RoofTileID, core.ClayID,
		core.SnowBlockID, core.MossyCobblestoneID,
	}
	columnStarts := [...]int32{-10, -7, -4, -1, 2, 5, 8}
	for index, block := range materials {
		startY := int32(1)
		if index >= len(columnStarts) {
			startY = 4
		}
		startX := columnStarts[index%len(columnStarts)]
		for y := startY; y <= startY+1; y++ {
			for x := startX; x <= startX+1; x++ {
				assertBlock(core.BlockPos{X: x, Y: y, Z: -8}, block)
			}
		}
	}
	for y := int32(4); y <= 5; y++ {
		for x := int32(-10); x <= -9; x++ {
			assertBlock(core.BlockPos{X: x, Y: y, Z: -9}, core.BrickID)
		}
	}
	for x := int32(-4); x <= 3; x++ {
		assertBlock(core.BlockPos{X: x, Y: 0, Z: -1}, core.GrassID)
	}
	for z := int32(-2); z <= 0; z++ {
		for x := int32(0); x <= 3; x++ {
			assertBlock(core.BlockPos{X: x, Y: 4, Z: z}, core.StoneID)
		}
	}
	for y := int32(1); y <= 3; y++ {
		assertBlock(core.BlockPos{X: 7, Y: y, Z: -1}, core.OakLogID)
	}
	// 耕地两态列：干（x=-6..-5）与湿（x=-8..-7）各一个 2 格宽的单层列，
	// 与草地条同层（y=0）、同深（z=-1）。规格断言锁的是坐标与块型本身；
	// 下沉顶面的呈现由 golden 视觉门禁验收。
	assertBlock(core.BlockPos{X: -6, Y: 0, Z: -1}, core.FarmlandDryID)
	assertBlock(core.BlockPos{X: -5, Y: 0, Z: -1}, core.FarmlandDryID)
	assertBlock(core.BlockPos{X: -8, Y: 0, Z: -1}, core.FarmlandWetID)
	assertBlock(core.BlockPos{X: -7, Y: 0, Z: -1}, core.FarmlandWetID)
	// 列外一格必须仍是空气：耕地不得越出声明的 2×2 格范围。
	assertBlock(core.BlockPos{X: -9, Y: 0, Z: -1}, core.AirID)
	if got := app.Mesher().Stats().DirtySections; got == 0 {
		t.Fatal("材料展示装入后 mesher 没有 dirty section")
	}

	remotePlayers := client.NewRemotePlayers()
	if err := remotePlayers.Apply(network.RemotePlayerSpawn{
		PlayerID: core.PlayerID{6: 0x40, 8: 0x80, 15: 1}, DisplayName: "测试Player",
		ServerTick: 1, Position: mgl32.Vec3{0, 2, 0},
	}); err != nil {
		t.Fatal(err)
	}
	stateApp := application.NewPresentationApplicationForTest()
	stateApp.SetRemotePlayers(remotePlayers)
	stateApp.Panel().SetVisible(true)
	stateApp.SetInventoryOpen(true)
	if err := stateApp.Furnace().Apply(network.FurnaceState{
		Furnace: core.FurnaceRef{Dimension: core.Overworld, Generation: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := stateApp.Chest().Apply(network.ChestState{
		Chest: core.ContainerRef{
			Dimension: core.Overworld, Kind: core.ContainerKindChest, Generation: 1,
		},
	}); err != nil {
		t.Fatal(err)
	}
	inventory := core.Inventory{}
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := stateApp.Inventory().Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	if err := scene.Apply(stateApp); err != nil {
		t.Fatal(err)
	}
	if stateApp.WorldTimeTicks() != 6000 ||
		stateApp.Camera().Pos != (mgl32.Vec3{0.5, 5.8, 13.5}) ||
		stateApp.Camera().Yaw != 0 || stateApp.Camera().Pitch != -0.12 {
		t.Fatalf("场景状态错误: time=%d camera=%+v yaw=%v pitch=%v",
			stateApp.WorldTimeTicks(), stateApp.Camera().Pos, stateApp.Camera().Yaw, stateApp.Camera().Pitch)
	}
	if stateApp.InventoryOpen() || stateApp.Panel().Visible() {
		t.Fatalf("界面状态未重置: inventoryOpen=%v panelVisible=%v",
			stateApp.InventoryOpen(), stateApp.Panel().Visible())
	}
	if len(remotePlayers.Presentations()) != 0 {
		t.Fatal("远端玩家未重置")
	}
	if _, ok := stateApp.Furnace().State(); ok {
		t.Fatal("熔炉镜像未重置")
	}
	if _, ok := stateApp.Chest().State(); ok {
		t.Fatal("箱子镜像未重置")
	}
	if got, confirmed := stateApp.Inventory().State(); !confirmed || got != (core.Inventory{}) {
		t.Fatalf("inventory = %+v confirmed=%v，想要已确认空物品栏", got, confirmed)
	}
}

// 杀死变异：遗漏目标反馈场景、未装入唯一砖块、绕过 Mirror/Mesher、未固定相机，
// 或继承上个场景的 UI 与远端玩家状态都会改变这些可观察结果。
func TestCaptureTargetBlockFeedbackFindsSceneByName(t *testing.T) {
	scene := captureSceneByName(t, "target-block-feedback")
	if scene.Name != "target-block-feedback" || scene.WarmupFrames != 8 ||
		scene.Prepare == nil || scene.Apply == nil {
		t.Fatalf("场景=%+v，想要完整 target-block-feedback", scene)
	}

	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	remotePlayers := client.NewRemotePlayers()
	if err := remotePlayers.Apply(network.RemotePlayerSpawn{
		PlayerID: core.PlayerID{6: 0x40, 8: 0x80, 15: 1}, DisplayName: "测试Player",
		ServerTick: 1, Position: mgl32.Vec3{0.5, 3.5, 1.5},
	}); err != nil {
		t.Fatal(err)
	}
	app := application.NewPresentationApplicationForTest()
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)
	app.SetPredictor(client.NewPredictor())
	app.SetRemotePlayers(remotePlayers)
	app.Panel().SetVisible(true)
	app.SetInventoryOpen(true)
	if err := app.Predictor().Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.Furnace().Apply(network.FurnaceState{
		Furnace: core.FurnaceRef{Dimension: core.Overworld, Generation: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.Chest().Apply(network.ChestState{Chest: core.ContainerRef{
		Dimension: core.Overworld, Kind: core.ContainerKindChest, Generation: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	inventory := core.Inventory{}
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.Inventory().Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}

	if err := scene.Prepare(app); err != nil {
		t.Fatal(err)
	}
	targetPosition := core.BlockPos{X: 0, Y: 3, Z: -3}
	for chunkZ := int32(-1); chunkZ <= 1; chunkZ++ {
		for chunkX := int32(-1); chunkX <= 1; chunkX++ {
			position := core.ChunkPos{X: chunkX, Z: chunkZ}
			want := world.NewChunk(position)
			wantRevision := uint64(1)
			if position == targetPosition.Chunk() {
				x, _, z := targetPosition.Local()
				want.SetBlock(x, targetPosition.Y, z, core.BrickID)
				wantRevision = 2
			}
			gotHash, gotRevision, loaded := app.Mirror().Hash(core.Overworld, position)
			if !loaded || gotRevision != wantRevision || gotHash != want.Hash() {
				t.Fatalf("chunk %+v hash/revision/loaded=(%x,%d,%v)，想要 (%x,%d,true)",
					position, gotHash, gotRevision, loaded, want.Hash(), wantRevision)
			}
		}
	}
	if got := app.Mesher().Stats().DirtySections; got == 0 {
		t.Fatal("目标夹具装入后 mesher 没有 dirty section")
	}

	if err := scene.Apply(app); err != nil {
		t.Fatal(err)
	}
	if app.WorldTimeTicks() != 6000 || app.Camera().Pos != (mgl32.Vec3{0.5, 3.5, 2.5}) ||
		app.Camera().Yaw != 0 || app.Camera().Pitch != 0 {
		t.Fatalf("场景状态错误: time=%d camera=%+v yaw=%v pitch=%v",
			app.WorldTimeTicks(), app.Camera().Pos, app.Camera().Yaw, app.Camera().Pitch)
	}
	if app.InventoryOpen() || app.Panel().Visible() ||
		len(app.RemotePlayers().Presentations()) != 0 {
		t.Fatalf("共享状态未清空: inventory=%v panel=%v remotes=%d",
			app.InventoryOpen(), app.Panel().Visible(),
			len(app.RemotePlayers().Presentations()))
	}
	if _, opened := app.Furnace().State(); opened {
		t.Fatal("熔炉状态未清空")
	}
	if _, opened := app.Chest().State(); opened {
		t.Fatal("箱子状态未清空")
	}
	if got, confirmed := app.Inventory().State(); !confirmed || got != (core.Inventory{}) {
		t.Fatalf("inventory=%+v confirmed=%v，想要已确认空物品栏", got, confirmed)
	}
	if got, ok := app.CurrentBlockTarget(); !ok || got != (application.BlockTarget{
		Position: targetPosition,
		Name:     "砖块",
	}) {
		t.Fatalf("currentBlockTarget()=%+v, %v，想要 %+v, true",
			got, ok, application.BlockTarget{Position: targetPosition, Name: "砖块"})
	}
}

func TestCaptureSettled(t *testing.T) {
	tests := []struct {
		name         string
		stats        client.MesherStats
		pending      int
		lodBusy      int
		vistaPending int
		want         bool
	}{
		{name: "全部归零", want: true},
		{name: "仍有 dirty", stats: client.MesherStats{DirtySections: 1}},
		{name: "仍有 queued", stats: client.MesherStats{QueuedJobs: 1}},
		{name: "仍有 in-flight", stats: client.MesherStats{InFlightJobs: 1}},
		{name: "仍有 ready", stats: client.MesherStats{ReadyResults: 1}},
		{name: "仍有上传", pending: 1},
		// 远环 tile 未收敛时同样不能判定 settled：异步上传依赖机器速度，
		// 提前抓帧会让 golden 里远景带时有时无，不可复现。
		{name: "远环仍有 tile", lodBusy: 1},
		// 全景管线（菜单相位场景）未装配完同样不能抓帧：全景底图的地形带
		// 与远环壳必须全部就绪。
		{name: "全景仍有工作", vistaPending: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := captureSettled(tc.stats, tc.pending, tc.lodBusy, tc.vistaPending); got != tc.want {
				t.Fatalf("captureSettled(%+v, %d, %d, %d) = %v，想要 %v",
					tc.stats, tc.pending, tc.lodBusy, tc.vistaPending, got, tc.want)
			}
		})
	}
}

func TestCaptureSkylightTunnelSceneFixesPresentationState(t *testing.T) {
	var scene captureScene
	for _, candidate := range captureScenes {
		if candidate.Name == "skylight-tunnel" {
			scene = candidate
			break
		}
	}
	if scene.Name == "" || scene.Prepare == nil {
		t.Fatal("capture 场景缺少完整 skylight-tunnel")
	}
	t.Run("空远端玩家列表", func(t *testing.T) {
		app := application.NewPresentationApplicationForTest()
		app.SetRemotePlayers(client.NewRemotePlayers())
		if err := scene.Apply(app); err != nil {
			t.Fatalf("空列表应用场景: %v", err)
		}
	})

	remotePlayers := client.NewRemotePlayers()
	for index, playerID := range []core.PlayerID{
		{6: 0x40, 8: 0x80, 15: 1},
		{0: 0x12, 6: 0x40, 8: 0x80, 15: 2},
	} {
		if err := remotePlayers.Apply(network.RemotePlayerSpawn{
			PlayerID: playerID, DisplayName: "测试Player", ServerTick: uint64(index + 1),
			Position: mgl32.Vec3{0.5, 2, 0.5},
		}); err != nil {
			t.Fatal(err)
		}
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

	if err := scene.Apply(app); err != nil {
		t.Fatal(err)
	}
	if app.WorldTimeTicks() != 6000 {
		t.Fatalf("world time = %d，想要 6000", app.WorldTimeTicks())
	}
	if app.Camera().Pos != (mgl32.Vec3{0.5, 2.8, 8.5}) || app.Camera().Yaw != 0 || app.Camera().Pitch != -0.04 {
		t.Fatalf("camera = %+v yaw=%v pitch=%v", app.Camera().Pos, app.Camera().Yaw, app.Camera().Pitch)
	}
	if got, confirmed := app.Inventory().State(); !confirmed || got != (core.Inventory{}) {
		t.Fatalf("inventory = %+v confirmed=%v，想要已确认空物品栏", got, confirmed)
	}
	if got := remotePlayers.Presentations(); len(got) != 0 {
		t.Fatalf("远端玩家未清空: %+v", got)
	}
	if app.InventoryOpen() || app.Panel().Visible() {
		t.Fatalf("上个场景的界面状态未清空: inventoryOpen=%v panelVisible=%v",
			app.InventoryOpen(), app.Panel().Visible())
	}
}

func TestBlockLightRoomCaptureSceneIsRegistered(t *testing.T) {
	for _, scene := range captureScenes {
		if scene.Name == "block-light-room" {
			if scene.Prepare == nil || scene.Apply == nil {
				t.Fatalf("场景=%+v，想要完整 block-light-room", scene)
			}
			return
		}
	}
	t.Fatal("缺少 block-light-room")
}

func TestPrepareBlockLightRoomUsesMirrorAndMesher(t *testing.T) {
	airMesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(airMesher.Close)
	app := &application.Application{}
	app.SetMirror(client.NewMirror())
	app.SetMesher(airMesher)

	if err := prepareCaptureAirNeighborhood(app); err != nil {
		t.Fatal(err)
	}
	roomMesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(roomMesher.Close)
	app.SetMesher(roomMesher)
	if got := roomMesher.Stats().DirtySections; got != 0 {
		t.Fatalf("施加房间变化前 dirty sections = %d，想要 0", got)
	}
	if err := applyCaptureBlockLightRoomChanges(app); err != nil {
		t.Fatal(err)
	}
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			chunk, ok := app.Mirror().Chunk(core.Overworld, core.ChunkPos{X: x, Z: z})
			if !ok || chunk.Revision != 2 {
				t.Fatalf("chunk (%d,%d) = (%v,%v)，想要 revision 2", x, z, chunk, ok)
			}
		}
	}
	for y := int32(0); y <= 6; y++ {
		for z := int32(-10); z <= 2; z++ {
			for x := int32(-6); x <= 6; x++ {
				position := core.BlockPos{X: x, Y: y, Z: z}
				want := core.AirID
				if y == 0 || y == 6 || x == -6 || x == 6 || z == -10 || z == 2 {
					want = core.StoneID
				}
				if position == (core.BlockPos{X: 0, Y: 3, Z: -4}) {
					want = core.LightBlockID
				}
				got, loaded := app.Mirror().BlockAt(core.Overworld, position)
				if !loaded || got != want {
					t.Fatalf("BlockAt(%+v) = (%d,%v)，想要 (%d,true)", position, got, loaded, want)
				}
			}
		}
	}
	for _, position := range []core.BlockPos{
		{X: -7, Y: 3, Z: -4},
		{X: 7, Y: 3, Z: -4},
		{X: 0, Y: 3, Z: -11},
		{X: 0, Y: 3, Z: 3},
	} {
		if got, loaded := app.Mirror().BlockAt(core.Overworld, position); !loaded || got != core.AirID {
			t.Fatalf("房外 BlockAt(%+v) = (%d,%v)，想要 (AirID,true)", position, got, loaded)
		}
	}
	// 空列顶 -65 到地板 y=0 的变化加传播半径 16，覆盖 Y=-64..16，
	// 即每个已加载邻区 6 个 section；房间后续方块不会扩大这个范围。
	const wantDirtySections = 9 * 6
	if got := roomMesher.Stats().DirtySections; got != wantDirtySections {
		t.Fatalf("dirty sections = %d，想要 %d", got, wantDirtySections)
	}
}

func TestBlockLightRoomApplyResetsSharedPresentationState(t *testing.T) {
	var scene captureScene
	for _, candidate := range captureScenes {
		if candidate.Name == "block-light-room" {
			scene = candidate
			break
		}
	}
	if scene.Apply == nil {
		t.Fatal("缺少 block-light-room")
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
	if app.WorldTimeTicks() != 18000 {
		t.Fatalf("world time = %d，想要 18000", app.WorldTimeTicks())
	}
	if app.Camera().Pos != (mgl32.Vec3{0.5, 2.8, 0.5}) || app.Camera().Yaw != 0 || app.Camera().Pitch != 0 {
		t.Fatalf("camera = %+v yaw=%v pitch=%v", app.Camera().Pos, app.Camera().Yaw, app.Camera().Pitch)
	}
	if got, confirmed := app.Inventory().State(); !confirmed || got != (core.Inventory{}) {
		t.Fatalf("inventory = %+v confirmed=%v，想要已确认空物品栏", got, confirmed)
	}
	if got := app.RemotePlayers().Presentations(); len(got) != 0 {
		t.Fatalf("远端玩家未清空: %+v", got)
	}
	if _, opened := app.Furnace().State(); opened {
		t.Fatal("熔炉状态未清空")
	}
	if _, opened := app.Chest().State(); opened {
		t.Fatal("箱子状态未清空")
	}
	if app.InventoryOpen() || app.Panel().Visible() {
		t.Fatalf("共享界面状态未清空: inventoryOpen=%v panelVisible=%v",
			app.InventoryOpen(), app.Panel().Visible())
	}
}

func TestCaptureSkylightTunnelUnsettledErrorNamesScene(t *testing.T) {
	app := application.NewOffscreenRenderApplicationForTest(t, &application.IntegrationGlyphSource{}, 64, 64, config.Render{})
	sections := make([]network.SectionData, core.SectionsPerChunk)
	for y := range sections {
		sections[y] = network.SectionData{
			Y: int32(y), Storage: network.SectionSingle, Single: core.AirID,
		}
	}
	oldTimeout := captureSettleTimeout
	captureSettleTimeout = 0
	t.Cleanup(func() { captureSettleTimeout = oldTimeout })
	scene := captureScene{
		Name: "skylight-tunnel",
		Prepare: func(app SceneApplication) error {
			update, err := app.Mirror().Apply(network.ChunkSnapshot{
				Dimension: core.Overworld, Revision: 1, Sections: sections,
			})
			if err != nil {
				return err
			}
			release := app.Mesher().BlockForTest(update.Dirty[0])
			t.Cleanup(release)
			app.Mesher().MarkDirty(update.Dirty...)
			return nil
		},
		Apply: func(SceneApplication) error { return nil },
	}
	dir := t.TempDir()
	err := captureOne(app, dir, scene, false)
	if err == nil || !strings.Contains(err.Error(), scene.Name) {
		t.Fatalf("未收敛错误 = %v，想要包含场景名 %q", err, scene.Name)
	}
	if _, statErr := os.Stat(filepath.Join(dir, scene.Name+".png")); !os.IsNotExist(statErr) {
		t.Fatalf("未收敛场景不应写图，statErr = %v", statErr)
	}
}

// TestTorchNightCaptureSceneIsRegistered 锁住 torch-night 场景条目的完整性：
// 与其余世界场景一样走「Prepare 装夹具 + Apply 定状态 + 8 帧预热」的完整链路。
func TestTorchNightCaptureSceneIsRegistered(t *testing.T) {
	scene := captureSceneByName(t, "torch-night")
	if scene.Prepare == nil || scene.Apply == nil {
		t.Fatalf("场景=%+v，想要完整 torch-night", scene)
	}
	if scene.WarmupFrames != 8 {
		t.Fatalf("torch-night WarmupFrames=%d，想要 8", scene.WarmupFrames)
	}
	if scene.Menu || scene.Settings != nil || scene.PinVolatile != nil {
		t.Fatalf("torch-night 不应携带菜单/设置/易变钉住夹具: %+v", scene)
	}
}

// TestPrepareTorchNightUsesMirrorAndMesher 穷举核对火把夜景夹具的全封闭长
// 石室与三朵火把的位置、形态与支撑：落地一朵在相机近处，左右墙各一朵
// 墙面形态（+X / −X），支撑格全部是实心石块。
func TestPrepareTorchNightUsesMirrorAndMesher(t *testing.T) {
	airMesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(airMesher.Close)
	app := &application.Application{}
	app.SetMirror(client.NewMirror())
	app.SetMesher(airMesher)

	if err := prepareCaptureAirNeighborhood(app); err != nil {
		t.Fatal(err)
	}
	roomMesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(roomMesher.Close)
	app.SetMesher(roomMesher)
	if got := roomMesher.Stats().DirtySections; got != 0 {
		t.Fatalf("施加房间变化前 dirty sections = %d，想要 0", got)
	}
	if err := applyCaptureTorchNightChanges(app); err != nil {
		t.Fatal(err)
	}
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			chunk, ok := app.Mirror().Chunk(core.Overworld, core.ChunkPos{X: x, Z: z})
			if !ok || chunk.Revision != 2 {
				t.Fatalf("chunk (%d,%d) = (%v,%v)，想要 revision 2", x, z, chunk, ok)
			}
		}
	}
	torches := map[core.BlockPos]core.BlockID{
		{X: 2, Y: 1, Z: -3}:  core.TorchStandingID,
		{X: -5, Y: 2, Z: -5}: core.TorchWallPosXID,
		{X: 5, Y: 2, Z: -6}:  core.TorchWallNegXID,
	}
	for y := int32(0); y <= 6; y++ {
		for z := int32(-16); z <= 2; z++ {
			for x := int32(-6); x <= 6; x++ {
				position := core.BlockPos{X: x, Y: y, Z: z}
				want := core.AirID
				if y == 0 || y == 6 || x == -6 || x == 6 || z == -16 || z == 2 {
					want = core.StoneID
				}
				if torch, ok := torches[position]; ok {
					want = torch
				}
				got, loaded := app.Mirror().BlockAt(core.Overworld, position)
				if !loaded || got != want {
					t.Fatalf("BlockAt(%+v) = (%d,%v)，想要 (%d,true)", position, got, loaded, want)
				}
			}
		}
	}
	// 火把之外不再有任何非空气/非石头方块：夹具的可见变化只有这三朵火把。
	for _, position := range []core.BlockPos{
		{X: -7, Y: 3, Z: -6},
		{X: 0, Y: 3, Z: 3},
		{X: 0, Y: 7, Z: -6},
	} {
		if got, loaded := app.Mirror().BlockAt(core.Overworld, position); !loaded || got != core.AirID {
			t.Fatalf("房外 BlockAt(%+v) = (%d,%v)，想要 (AirID,true)", position, got, loaded)
		}
	}
	// 支撑格逐一验证：落地火把踩在地板上，两朵墙面火把的支撑（形态反方向
	// 一格）都是石墙——夹具因此与放置规则的支撑契约一致，不是悬空摆拍。
	for _, support := range []core.BlockPos{
		{X: 2, Y: 0, Z: -3},
		{X: -6, Y: 2, Z: -5},
		{X: 6, Y: 2, Z: -6},
	} {
		if got, loaded := app.Mirror().BlockAt(core.Overworld, support); !loaded || got != core.StoneID {
			t.Fatalf("火把支撑 BlockAt(%+v) = (%d,%v)，想要 (StoneID,true)", support, got, loaded)
		}
	}
	if got := roomMesher.Stats().DirtySections; got == 0 {
		t.Fatal("火把夜景装入后 mesher 没有 dirty section")
	}
}

// TestTorchNightApplyResetsSharedPresentationState 与 block-light-room 的同名
// 断言同构：前序场景留下的全部共享呈现状态都必须被 Apply 显式清空，夜晚
// 时间与相机姿态固定。
func TestTorchNightApplyResetsSharedPresentationState(t *testing.T) {
	scene := captureSceneByName(t, "torch-night")
	if scene.Apply == nil {
		t.Fatal("缺少 torch-night")
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
	if app.WorldTimeTicks() != 18000 {
		t.Fatalf("world time = %d，想要 18000（夜晚：室内亮度只由方块光决定）", app.WorldTimeTicks())
	}
	if app.Camera().Pos != (mgl32.Vec3{0.5, 2.8, 0.5}) || app.Camera().Yaw != 0 || app.Camera().Pitch != 0 {
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

// newCaptureSceneRenderApplication 构造与正式 capture 同尺寸（640×360）的最小
// 离屏渲染 application：世界夜景场景（torch-night、bed-night 等）的像素断言经
// 它复用 captureSceneImage 的完整链路（预热、装夹具、收敛、回读）。渲染器是
// 离屏设备，不创建也不聚焦任何前台窗口；无 GPU 适配器时跳过（与既有渲染测试
// 同口径）。材质用内嵌默认材质包，与 golden 抓帧路径看到的是同一份像素。
func newCaptureSceneRenderApplication(t *testing.T) *application.Application {
	t.Helper()
	app := application.NewOffscreenRenderApplicationForTest(
		t, &application.IntegrationGlyphSource{}, captureWidth, captureHeight,
		config.Defaults().Render,
	)
	*app.Camera() = client.Camera{
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	return app
}

// captureSceneProject 把世界坐标投影到 capture 屏幕像素（y 轴向下）。相机取
// Apply 之后的实例，投影矩阵与渲染用的 `Camera.ViewProj` 同一实现，不另写
// 一套。
func captureSceneProject(t *testing.T, camera client.Camera, p mgl32.Vec3) image.Point {
	t.Helper()
	clip := camera.ViewProj().Mul4x1(mgl32.Vec4{p.X(), p.Y(), p.Z(), 1})
	if clip.W() <= 0 {
		t.Fatalf("采样点 %v 落在相机身后", p)
	}
	x := int((clip.X()/clip.W()*0.5 + 0.5) * float32(captureWidth))
	y := int((1 - (clip.Y()/clip.W()*0.5 + 0.5)) * float32(captureHeight))
	return image.Pt(x, y)
}

// captureSceneCellRect 返回一个方块格八 corner 投影后的屏幕外接矩形。
func captureSceneCellRect(t *testing.T, camera client.Camera, cell core.BlockPos) image.Rectangle {
	t.Helper()
	minX, minY, maxX, maxY := 0, 0, 0, 0
	for index, corner := range [8]mgl32.Vec3{
		{0, 0, 0}, {1, 0, 0}, {0, 1, 0}, {1, 1, 0},
		{0, 0, 1}, {1, 0, 1}, {0, 1, 1}, {1, 1, 1},
	} {
		point := captureSceneProject(t, camera, mgl32.Vec3{
			float32(cell.X) + corner.X(),
			float32(cell.Y) + corner.Y(),
			float32(cell.Z) + corner.Z(),
		})
		if index == 0 {
			minX, minY, maxX, maxY = point.X, point.Y, point.X, point.Y
			continue
		}
		minX, minY = min(minX, point.X), min(minY, point.Y)
		maxX, maxY = max(maxX, point.X), max(maxY, point.Y)
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

// torchNightPatchLuma 返回屏幕上以 center 为中心的 11×11 像素平均亮度。
func torchNightPatchLuma(t *testing.T, img *image.NRGBA, center image.Point) int {
	t.Helper()
	const radius = 5
	sum, count := 0, 0
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			x, y := center.X+dx, center.Y+dy
			if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				t.Fatalf("采样块 (%d,%d) 越出图像边界", x, y)
			}
			i := img.PixOffset(x, y)
			sum += (int(img.Pix[i]) + int(img.Pix[i+1]) + int(img.Pix[i+2])) / 3
			count++
		}
	}
	return sum / count
}

// torchNightFlamePixels 收集屏幕矩形内判为「火芯暖色」的像素：亮暖（R 显著
// 高于 B）。石面被火把照亮是中性灰（R≈B），不会被误判成火芯。暖色门槛按内嵌
// 默认包标定：Pastelcraft 火把是浅奶油色杆（纹理暖色 R-B≈140），夜景渲染后
// R-B 实测 78，中性石面 R-B 贴近 0，门槛取 60 留出两侧裕量。
func torchNightFlamePixels(img *image.NRGBA, rect image.Rectangle) []image.Point {
	var flames []image.Point
	rect = rect.Intersect(img.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := img.PixOffset(x, y)
			r, b := int(img.Pix[i]), int(img.Pix[i+2])
			if r >= 160 && r-b >= 60 {
				flames = append(flames, image.Pt(x, y))
			}
		}
	}
	return flames
}

// TestTorchNightScenePixelsShowLightFalloffAndCutout 是 torch-night 的场景内
// 像素断言（spec visual-verification「无窗口夜景场景」的 Scenario）：
//
//   - 近亮远暗：火把旁的地板采样块明显亮于远角的同材质地板采样块。阈值按
//     输出经过 sRGB gamma 编码标定——线性光在画面里被压缩，25 点亮度差已经
//     对应数级方块光衰减；
//   - 透明边缘：落地火把格的屏幕外接矩形里，火芯像素只占小部分、四角都
//     不是火芯——16×16 图层的其余像素 alpha=0，透过它能看到背景；
//   - 封墙暗室：无火把照到的远端天花板保持暗——若天空光灌进封闭房间或
//     方块光误穿墙体，该采样块会明显亮起来。
//
// 同时核对落地与左右两墙火把的火芯都真实出现在各自的屏幕格内（墙面形态经
// model tag 2..3 的贴面斜板几何渲染，而不是只有落地形态在画）。
func TestTorchNightScenePixelsShowLightFalloffAndCutout(t *testing.T) {
	app := newCaptureSceneRenderApplication(t)
	scene := captureSceneByName(t, "torch-night")
	img, err := captureSceneImage(app, scene)
	if err != nil {
		t.Fatalf("抓取 torch-night: %v", err)
	}
	if got := img.Bounds(); got != image.Rect(0, 0, captureWidth, captureHeight) {
		t.Fatalf("抓帧尺寸=%v，想要 %dx%d", got, captureWidth, captureHeight)
	}
	camera := *app.Camera()

	t.Run("近亮远暗", func(t *testing.T) {
		near := torchNightPatchLuma(t, img, captureSceneProject(t, camera, mgl32.Vec3{1.0, 1.0, -3.0}))
		far := torchNightPatchLuma(t, img, captureSceneProject(t, camera, mgl32.Vec3{5.0, 1.0, -15.0}))
		if near < 70 {
			t.Fatalf("火把旁地板亮度=%d，想要至少 70：火把没有点亮近处", near)
		}
		if near-far < 25 {
			t.Fatalf("近处亮度=%d 远处=%d，衰减梯度不足 25", near, far)
		}
		if far < 12 {
			t.Fatalf("远角亮度=%d，火把的方块光应衰减到达而不是缺席", far)
		}
	})
	t.Run("封墙暗室无漏光", func(t *testing.T) {
		ceiling := torchNightPatchLuma(t, img, captureSceneProject(t, camera, mgl32.Vec3{4.0, 6.0, -12.0}))
		// 阈值按内嵌默认包标定：光照与几何与换肤前完全一致，Pastelcraft 石面
		// 反照率更高，同光场下该采样块实测 76（换肤前 <70），取 90 保留裕量；
		// 真实漏光（天空光灌入或方块光穿墙）会把该值推高到被照亮面量级。
		if ceiling >= 90 {
			t.Fatalf("远端天花板亮度=%d，封闭暗室的未照到面应当保持暗色", ceiling)
		}
	})
	t.Run("透明边缘非实心矩形", func(t *testing.T) {
		rect := captureSceneCellRect(t, camera, core.BlockPos{X: 2, Y: 1, Z: -3})
		flames := torchNightFlamePixels(img, rect)
		if len(flames) < 8 {
			t.Fatalf("落地火把格内火芯像素=%d，想要至少 8（矩形=%v）", len(flames), rect)
		}
		area := rect.Dx() * rect.Dy()
		if len(flames)*2 >= area {
			t.Fatalf("火芯像素=%d 占格子矩形 %d 的一半以上：火把被画成实心矩形", len(flames), area)
		}
		for _, corner := range []image.Point{rect.Min, {rect.Max.X - 1, rect.Min.Y}, rect.Max.Sub(image.Pt(1, 1)), {rect.Min.X, rect.Max.Y - 1}} {
			// 相机贴地时格子外接矩形会越出画面下缘，角落采样先钳回帧内。
			x := min(max(corner.X, 0), captureWidth-1)
			y := min(max(corner.Y, 0), captureHeight-1)
			i := img.PixOffset(x, y)
			r, b := int(img.Pix[i]), int(img.Pix[i+2])
			if r >= 160 && r-b >= 80 {
				t.Fatalf("落地火把格角落 (%d,%d) 是火芯色：cutout 的透明边缘应露出背景", x, y)
			}
		}
	})
	t.Run("墙面火把可见", func(t *testing.T) {
		for _, wall := range []struct {
			name string
			cell core.BlockPos
		}{
			{"左墙 +X 形态", core.BlockPos{X: -5, Y: 2, Z: -5}},
			{"右墙 −X 形态", core.BlockPos{X: 5, Y: 2, Z: -6}},
		} {
			t.Run(wall.name, func(t *testing.T) {
				flames := torchNightFlamePixels(img, captureSceneCellRect(t, camera, wall.cell))
				if len(flames) < 4 {
					t.Fatalf("墙面火把格内火芯像素=%d，想要至少 4：贴面斜板几何没有渲染", len(flames))
				}
			})
		}
	})
}

// TestTorchNightPatchLumaCatchesGrossLeak 用合成亮斑证明暗室阈值的漏光侧仍
// 被抓住：同一采样函数 `torchNightPatchLuma`（11×11 均值）在被照亮面量级的
// 灰斑上返回值远高于阈值 90。干净侧实测 76（阈值 90，裕量 14），漏光侧合成
// 160（阈值 90，裕量 70），两侧都有明确余量。
func TestTorchNightPatchLumaCatchesGrossLeak(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, captureWidth, captureHeight))
	center := image.Pt(captureWidth/2, captureHeight/2)
	const lit = 160
	for dy := -5; dy <= 5; dy++ {
		for dx := -5; dx <= 5; dx++ {
			i := img.PixOffset(center.X+dx, center.Y+dy)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = lit, lit, lit, 255
		}
	}
	got := torchNightPatchLuma(t, img, center)
	if got < 150 {
		t.Fatalf("合成漏光采样=%d，想要至少 150（被照亮面量级）", got)
	}
	if got < 90 {
		t.Fatalf("合成漏光采样=%d，未触发暗室阈值 90", got)
	}
	t.Logf("干净侧实测 76 vs 阈值 90（裕量 14），合成漏光 %d vs 阈值 90（裕量 %d）", got, got-90)
}

// TestBedNightCaptureSceneIsRegistered 锁住 bed-night 场景条目的完整性：
// 与其余世界场景一样走「Prepare 装夹具 + Apply 定状态 + 8 帧预热」的完整链路。
func TestBedNightCaptureSceneIsRegistered(t *testing.T) {
	scene := captureSceneByName(t, "bed-night")
	if scene.Prepare == nil || scene.Apply == nil {
		t.Fatalf("场景=%+v，想要完整 bed-night", scene)
	}
	if scene.WarmupFrames != 8 {
		t.Fatalf("bed-night WarmupFrames=%d，想要 8", scene.WarmupFrames)
	}
	if scene.Menu || scene.Settings != nil || scene.PinVolatile != nil {
		t.Fatalf("bed-night 不应携带菜单/设置/易变钉住夹具: %+v", scene)
	}
}

// TestDebugPanelAndTerrainNoonShareNoonWorldFixture 钉住 debug-panel 与
// terrain-noon 的夹具恒等：同一正午 tick、同一相机姿态、都不装世界夹具
// （无 Prepare，共用登录世界）。debug-panel 的「面板可见不产生无头面板像素」
// 这条 golden 钉值只有在两景夹具完全一致时才成立——任何一侧单独改夹具都会
// 让两张 golden 的差异不再只归因于面板状态，该钉值于是静默失效。
func TestDebugPanelAndTerrainNoonShareNoonWorldFixture(t *testing.T) {
	terrain := captureSceneByName(t, "terrain-noon")
	panel := captureSceneByName(t, "debug-panel")
	if terrain.Prepare != nil || panel.Prepare != nil {
		t.Fatalf("两景必须共用登录世界夹具：terrain.Prepare=%v panel.Prepare=%v",
			terrain.Prepare != nil, panel.Prepare != nil)
	}
	app := newCaptureAICompanionState()
	for _, scene := range []captureScene{terrain, panel} {
		if err := scene.Apply(app); err != nil {
			t.Fatalf("应用 %s: %v", scene.Name, err)
		}
		if app.WorldTimeTicks() != 6000 || app.Camera().Yaw != 0 || app.Camera().Pitch != -0.25 {
			t.Fatalf("%s 夹具 time=%d yaw=%v pitch=%v，想要 6000/0/-0.25（与 terrain-noon 恒等）",
				scene.Name, app.WorldTimeTicks(), app.Camera().Yaw, app.Camera().Pitch)
		}
	}
	if !app.Panel().Visible() {
		t.Fatal("debug-panel 的 Apply 必须装入面板可见态，否则零面板像素钉值没有被驱动")
	}
}

// bedNightBeds 是床夜景夹具的四张床（格 → 形态编号）：四个水平朝向各一张
// 完整床（床头与床尾成对、同框），坐标与 `applyCaptureBedNightChanges` 里的
// 夹具逐一对应——生产表与测试锁各声明一份，任何一侧单独漂移都会红。
var bedNightBeds = map[core.BlockPos]core.BlockID{
	{X: -3, Y: 1, Z: -4}: core.BedFootEastID,
	{X: -2, Y: 1, Z: -4}: core.BedHeadEastID,
	{X: 2, Y: 1, Z: -4}:  core.BedFootWestID,
	{X: 1, Y: 1, Z: -4}:  core.BedHeadWestID,
	{X: 4, Y: 1, Z: -6}:  core.BedFootSouthID,
	{X: 4, Y: 1, Z: -5}:  core.BedHeadSouthID,
	{X: -5, Y: 1, Z: -5}: core.BedFootNorthID,
	{X: -5, Y: 1, Z: -6}: core.BedHeadNorthID,
}

// bedNightTorches 是床夜景夹具的三朵火把：落地一朵居中，左右墙各一朵墙面
// 形态，与火把夜景同一支撑契约。
var bedNightTorches = map[core.BlockPos]core.BlockID{
	{X: 0, Y: 1, Z: -7}:  core.TorchStandingID,
	{X: -5, Y: 2, Z: -3}: core.TorchWallPosXID,
	{X: 5, Y: 2, Z: -5}:  core.TorchWallNegXID,
}

// TestPrepareBedNightRoomUsesMirrorAndMesher 穷举核对床夜景夹具的全封闭石室、
// 四朝向床与三朵火把的位置、形态与支撑：床的 8 个格与火把 3 个格之外，室内
// 必须全是空气，外壳全是石块；床双格按 `core` 的朝向映射成对出现且床尾格的
// 正下方是实心石地板——夹具因此与放置规则的支撑契约一致，不是悬空摆拍。
// 最后经下一场景（materials-showcase）的 Prepare 重装空气基线，验证床与火把
// 夹具不泄入后续场景（spec delta「不污染后续场景」的 Scenario）。
func TestPrepareBedNightRoomUsesMirrorAndMesher(t *testing.T) {
	airMesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(airMesher.Close)
	app := &application.Application{}
	app.SetMirror(client.NewMirror())
	app.SetMesher(airMesher)

	if err := prepareCaptureAirNeighborhood(app); err != nil {
		t.Fatal(err)
	}
	roomMesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(roomMesher.Close)
	app.SetMesher(roomMesher)
	if got := roomMesher.Stats().DirtySections; got != 0 {
		t.Fatalf("施加房间变化前 dirty sections = %d，想要 0", got)
	}
	if err := applyCaptureBedNightChanges(app); err != nil {
		t.Fatal(err)
	}
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			chunk, ok := app.Mirror().Chunk(core.Overworld, core.ChunkPos{X: x, Z: z})
			if !ok || chunk.Revision != 2 {
				t.Fatalf("chunk (%d,%d) = (%v,%v)，想要 revision 2", x, z, chunk, ok)
			}
		}
	}
	for y := int32(0); y <= 6; y++ {
		for z := int32(-16); z <= 2; z++ {
			for x := int32(-6); x <= 6; x++ {
				position := core.BlockPos{X: x, Y: y, Z: z}
				want := core.AirID
				if y == 0 || y == 6 || x == -6 || x == 6 || z == -16 || z == 2 {
					want = core.StoneID
				}
				if bed, ok := bedNightBeds[position]; ok {
					want = bed
				}
				if torch, ok := bedNightTorches[position]; ok {
					want = torch
				}
				got, loaded := app.Mirror().BlockAt(core.Overworld, position)
				if !loaded || got != want {
					t.Fatalf("BlockAt(%+v) = (%d,%v)，想要 (%d,true)", position, got, loaded, want)
				}
			}
		}
	}
	// 床双格按 core 的「朝向 ↔ 编号」唯一窗口成对出现：床头格恰为床尾格按
	// 朝向的邻格、编号与 `core.BedHeadID` 一致，且两格正下方都有实心支撑。
	for position, id := range bedNightBeds {
		if !core.IsBedFoot(id) {
			continue
		}
		dir := core.BedDir(id)
		head := core.BedHeadNeighbor(position, dir)
		got, loaded := app.Mirror().BlockAt(core.Overworld, head)
		if !loaded || got != core.BedHeadID(dir) {
			t.Fatalf("床头 BlockAt(%+v) = (%d,%v)，想要 (%d,true)", head, got, loaded, core.BedHeadID(dir))
		}
	}
	// 床与火把的正下方（墙面形态的支撑在墙内同一高度）逐一核对支撑格。
	// 落地火把与 8 个床格的支撑是正下方地板；墙面火把的支撑按形态反方向
	// 在同一高度。
	for _, position := range []core.BlockPos{
		{X: -3, Y: 1, Z: -4}, {X: -2, Y: 1, Z: -4},
		{X: 2, Y: 1, Z: -4}, {X: 1, Y: 1, Z: -4},
		{X: 4, Y: 1, Z: -6}, {X: 4, Y: 1, Z: -5},
		{X: -5, Y: 1, Z: -5}, {X: -5, Y: 1, Z: -6},
		{X: 0, Y: 1, Z: -7},
	} {
		support := core.BlockPos{X: position.X, Y: position.Y - 1, Z: position.Z}
		if got, loaded := app.Mirror().BlockAt(core.Overworld, support); !loaded || got != core.StoneID {
			t.Fatalf("支撑 BlockAt(%+v) = (%d,%v)，想要 (StoneID,true)", support, got, loaded)
		}
	}
	for _, support := range []core.BlockPos{
		{X: -6, Y: 2, Z: -3},
		{X: 6, Y: 2, Z: -5},
	} {
		if got, loaded := app.Mirror().BlockAt(core.Overworld, support); !loaded || got != core.StoneID {
			t.Fatalf("墙面火把支撑 BlockAt(%+v) = (%d,%v)，想要 (StoneID,true)", support, got, loaded)
		}
	}
	// 房外一格必须仍是空气：夹具的可见变化不越出外壳。
	for _, position := range []core.BlockPos{
		{X: -7, Y: 3, Z: -8},
		{X: 0, Y: 3, Z: 3},
		{X: 0, Y: 7, Z: -8},
	} {
		if got, loaded := app.Mirror().BlockAt(core.Overworld, position); !loaded || got != core.AirID {
			t.Fatalf("房外 BlockAt(%+v) = (%d,%v)，想要 (AirID,true)", position, got, loaded)
		}
	}
	if got := roomMesher.Stats().DirtySections; got == 0 {
		t.Fatal("床夜景装入后 mesher 没有 dirty section")
	}

	// 下一场景从空气基线重装后，床与火把格必须全部消失：夹具值不得泄入
	// 后续场景。
	if err := prepareMaterialsShowcase(app); err != nil {
		t.Fatal(err)
	}
	for position := range bedNightBeds {
		if got, loaded := app.Mirror().BlockAt(core.Overworld, position); !loaded || got != core.AirID {
			t.Fatalf("后续场景床格 BlockAt(%+v) = (%d,%v)，想要 (AirID,true)", position, got, loaded)
		}
	}
	for position := range bedNightTorches {
		if got, loaded := app.Mirror().BlockAt(core.Overworld, position); !loaded || got != core.AirID {
			t.Fatalf("后续场景火把格 BlockAt(%+v) = (%d,%v)，想要 (AirID,true)", position, got, loaded)
		}
	}
}

// TestBedNightApplyResetsSharedPresentationState 与 torch-night 的同名断言
// 同构：前序场景留下的全部共享呈现状态都必须被 Apply 显式清空，夜晚时间
// 与相机姿态固定。
func TestBedNightApplyResetsSharedPresentationState(t *testing.T) {
	scene := captureSceneByName(t, "bed-night")
	if scene.Apply == nil {
		t.Fatal("缺少 bed-night")
	}
	remotePlayers := client.NewRemotePlayers()
	if err := remotePlayers.Apply(network.RemotePlayerSpawn{
		PlayerID: core.PlayerID{6: 0x40, 8: 0x80, 15: 1}, DisplayName: "测试Player",
		ServerTick: 1, Position: mgl32.Vec3{0.5, 2, 0.5},
	}); err != nil {
		t.Fatal(err)
	}
	app := &application.Application{}
	app.SetRemotePlayers(remotePlayers)
	app.SetPanel(&application.PanelState{})
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
	if app.WorldTimeTicks() != 18000 {
		t.Fatalf("world time = %d，想要 18000（夜晚：室内亮度只由方块光决定）", app.WorldTimeTicks())
	}
	if app.Camera().Pos != (mgl32.Vec3{0.5, 3.4, 0.5}) || app.Camera().Yaw != 0 || app.Camera().Pitch != -0.3 {
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
	if app.InventoryOpen() || app.Panel() != nil && app.Panel().Visible() {
		t.Fatalf("共享界面状态未清空: inventoryOpen=%v panelVisible=%v",
			app.InventoryOpen(), app.Panel() != nil && app.Panel().Visible())
	}
}

// bedNightPatchRGB 返回以 center 为中心的 3×3 像素平均 RGB。床面亮带在
// 640×360 的画面里只有几个像素厚，3×3 已覆盖采样点邻域而不至于混入相邻
// 材质之外的大片背景。
func bedNightPatchRGB(t *testing.T, img *image.NRGBA, center image.Point) (int, int, int) {
	t.Helper()
	const radius = 1
	sum := [3]int{}
	count := 0
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			x, y := center.X+dx, center.Y+dy
			if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				t.Fatalf("采样块 (%d,%d) 越出图像边界", x, y)
			}
			i := img.PixOffset(x, y)
			sum[0] += int(img.Pix[i])
			sum[1] += int(img.Pix[i+1])
			sum[2] += int(img.Pix[i+2])
			count++
		}
	}
	return sum[0] / count, sum[1] / count, sum[2] / count
}

// TestBedNightScenePixelsShowMultiOrientationBedsAtNight 是 bed-night 的场景内
// 像素断言（spec delta visual-verification「夜间配色可辨认」的 Scenario）：
//
//   - 多朝向床头可辨：四张床的枕头带（床头亮带）采样块都比同床毯沿带更亮、
//     更接近中性（R−G 更小）——床头方向因此逐床可读，四个朝向同框互证；
//   - 床面配色可辨：床垫暖色与**同照度**石面的暖度差显著为正——火把照明
//     本身给石面染色，差值扣掉照明色温后剩下的才是床的材质本色；
//   - 半高轮廓：床侧板是暖色木板，其正上方透视到的背景是暗色石墙——
//     床只占半格高，上半透出背景，轮廓因此可辨。
//
// 采样点钉在床面层 UV 的带位上（带沿床头朝向边内侧 3px：南/北带沿 z、
// 东/西带沿 x），与 `assets.bedBand` 的画带约定一致。经完整呈现链路抓帧，
// 渲染器是离屏设备，不创建也不聚焦任何前台窗口；无 GPU 适配器时跳过。
func TestBedNightScenePixelsShowMultiOrientationBedsAtNight(t *testing.T) {
	app := newCaptureSceneRenderApplication(t)
	scene := captureSceneByName(t, "bed-night")
	img, err := captureSceneImage(app, scene)
	if err != nil {
		t.Fatalf("抓取 bed-night: %v", err)
	}
	camera := *app.Camera()

	// 床顶平面高度：床是 9/16 半高板（块原点 y=1，顶面在 1+9/16）。
	const bedTopY = float32(1) + 9.0/16.0
	samples := []struct {
		name     string
		pillow   mgl32.Vec3
		blanket  mgl32.Vec3
		mattress mgl32.Vec3
	}{
		{"东向床", mgl32.Vec3{-1.22, bedTopY, -3.5}, mgl32.Vec3{-2.22, bedTopY, -3.5}, mgl32.Vec3{-1.75, bedTopY, -3.5}},
		{"西向床", mgl32.Vec3{1.22, bedTopY, -3.5}, mgl32.Vec3{2.22, bedTopY, -3.5}, mgl32.Vec3{1.75, bedTopY, -3.5}},
		{"南向床", mgl32.Vec3{4.5, bedTopY, -4.22}, mgl32.Vec3{4.5, bedTopY, -5.22}, mgl32.Vec3{4.5, bedTopY, -5.75}},
		{"北向床", mgl32.Vec3{-4.5, bedTopY, -5.78}, mgl32.Vec3{-4.5, bedTopY, -4.78}, mgl32.Vec3{-4.5, bedTopY, -5.25}},
	}

	t.Run("床头床尾同框且完整在画面内", func(t *testing.T) {
		for position := range bedNightBeds {
			rect := captureSceneCellRect(t, camera, position)
			if !rect.In(image.Rect(0, 0, captureWidth, captureHeight)) {
				t.Fatalf("床格 %v 屏幕外接矩形 %v 越出画面", position, rect)
			}
		}
	})

	// 石面对照样点：东向床与西向床之间的地板，与床面同受居中火把照明、
	// 光照等级与床面相近——床垫暖度减去它，就是「材质本色」而非照明色温。
	const stoneX, stoneY, stoneZ = float32(-0.5), float32(1.0), float32(-3.5)

	t.Run("多朝向床头亮带可辨", func(t *testing.T) {
		sr, sg, _ := bedNightPatchRGB(t, img, captureSceneProject(t, camera, mgl32.Vec3{stoneX, stoneY, stoneZ}))
		for _, sample := range samples {
			t.Run(sample.name, func(t *testing.T) {
				pr, pg, pb := bedNightPatchRGB(t, img, captureSceneProject(t, camera, sample.pillow))
				br, bg, bb := bedNightPatchRGB(t, img, captureSceneProject(t, camera, sample.blanket))
				mr, mg, _ := bedNightPatchRGB(t, img, captureSceneProject(t, camera, sample.mattress))
				pillowLuma, blanketLuma := (pr+pg+pb)/3, (br+bg+bb)/3
				t.Logf("%s: 枕头 rgb=(%d,%d,%d) 毯沿 rgb=(%d,%d,%d) 床垫 r=%d g=%d 石面 r=%d g=%d",
					sample.name, pr, pg, pb, br, bg, bb, mr, mg, sr, sg)
				if pillowLuma-blanketLuma < 15 {
					t.Fatalf("枕头亮度=%d 毯沿亮度=%d：床头亮带不可辨", pillowLuma, blanketLuma)
				}
				if (br-bg)-(pr-pg) < 15 {
					t.Fatalf("毯沿暖度(r-g)=%d 枕头暖度(r-g)=%d：毯沿暖带不可辨", br-bg, pr-pg)
				}
				if pillowLuma < 90 {
					t.Fatalf("枕头亮度=%d：夜间光照下床头不可辨", pillowLuma)
				}
				if (mr-mg)-(sr-sg) < 10 {
					t.Fatalf("床垫暖度(r-g)=%d 同照度石面暖度(r-g)=%d：床面配色与石面不可分", mr-mg, sr-sg)
				}
			})
		}
	})

	t.Run("半高轮廓可辨", func(t *testing.T) {
		// 南向床的 −X 侧板：板面是橡木木板层（暖色），板顶（9/16）以上同一
		// 屏幕列透视到的是远处暗色石墙——床只占下半格，上半透出背景。
		boardR, boardG, _ := bedNightPatchRGB(t, img, captureSceneProject(t, camera, mgl32.Vec3{4.0, 1.28, -5.5}))
		aboveR, aboveG, _ := bedNightPatchRGB(t, img, captureSceneProject(t, camera, mgl32.Vec3{4.0, 2.35, -5.5}))
		t.Logf("侧板 r=%d g=%d，板上背景 r=%d g=%d", boardR, boardG, aboveR, aboveG)
		if boardR-boardG < 10 {
			t.Fatalf("床侧板暖度(r-g)=%d：床体木色不可辨", boardR-boardG)
		}
		if (boardR-boardG)-(aboveR-aboveG) < 8 {
			t.Fatalf("侧板暖度=%d 板上背景暖度=%d：半高轮廓不可辨",
				boardR-boardG, aboveR-aboveG)
		}
	})
}
