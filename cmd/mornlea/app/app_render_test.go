//go:build darwin

package app

// 渲染切换到 Rust 后,本文件的断言面从 gfx 假设备的 pass/uniform 捕获改为
// CPU 侧可见事实:实体/名牌集合、pass 段字节流、调用计数与错误传播。
// pass 录制顺序与 uniform 布线已内化于 Rust 渲染器(crate 结构 + 既有
// golden capture 字节不变共同守护),对应的假设备用例随 Go GPU 栈退役。

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/mesh"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/world"
)

func TestApplicationRendersSevenPlayersAndFourCompanionsInOneAvatarStream(t *testing.T) {
	glyphs := &IntegrationGlyphSource{}
	app := newRemoteRenderApplication(t, glyphs)
	app.companions = &client.Companions{}
	configureTargetFeedback(t, app)
	for index, name := range [...]string{"甲", "乙", "丙", "丁", "戊", "己", "庚"} {
		if err := app.remotePlayers.Apply(RemoteSpawn(
			byte(index+1), name, 1, mgl32.Vec3{float32(index), 2, -4},
		)); err != nil {
			t.Fatal(err)
		}
	}
	for index, name := range [...]string{"阿木", "小石", "青叶", "星尘"} {
		id := companion.ID(integrationPlayerID(byte(index + 1)))
		if err := app.companions.ApplySpawn(network.CompanionSpawn{
			ID: id, Name: name, Tick: 1, Dimension: core.Overworld,
			Position: mgl32.Vec3{float32(index), 2, -8},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("RenderFrame=(%v,%v)", rendered, err)
	}
	if got, want := len(app.remoteAvatars), 11; got != want {
		t.Fatalf("avatars=%d，想要 %d", got, want)
	}
	if got, want := len(app.remoteNameTags), 12; got != want {
		t.Fatalf("name tags=%d，想要 %d", got, want)
	}
	// 11 具身体 × 6 部件在单一实例流中(单 pass 的等价断言)。
	if got, want := len(app.avatarStream), 11*6*80; got != want {
		t.Fatalf("avatar 实例流=%d 字节，想要 %d", got, want)
	}
	avatarKeys := make(map[render.EntityKey]struct{}, len(app.remoteAvatars))
	for _, avatar := range app.remoteAvatars {
		avatarKeys[avatar.Key] = struct{}{}
	}
	if len(avatarKeys) != 11 {
		t.Fatalf("Avatar 实体键去重后=%d，想要 11", len(avatarKeys))
	}
	playerKey := render.EntityKey{Kind: render.EntityPlayer, ID: [16]byte(integrationPlayerID(1))}
	companionKey := render.EntityKey{Kind: render.EntityCompanion, ID: playerKey.ID}
	if _, ok := avatarKeys[playerKey]; !ok {
		t.Fatalf("缺少玩家键 %v", playerKey)
	}
	if _, ok := avatarKeys[companionKey]; !ok {
		t.Fatalf("缺少伙伴键 %v", companionKey)
	}
	nameTagKeys := make(map[render.EntityKey]struct{}, len(app.remoteNameTags))
	for _, tag := range app.remoteNameTags {
		nameTagKeys[tag.Key] = struct{}{}
	}
	if len(nameTagKeys) != 12 {
		t.Fatalf("NameTag 实体键去重后=%d，想要 12", len(nameTagKeys))
	}
	if _, ok := nameTagKeys[render.EntityKey{Kind: render.EntityTarget}]; !ok {
		t.Fatal("缺少目标 EntityTarget 名牌")
	}
	if glyphs.flushes != 1 {
		t.Fatalf("NameTag Prepare/Flush 次数=%d，想要 1", glyphs.flushes)
	}
	if err := validateEntityPresentationCounts(make([]render.Avatar, 76), app.remoteNameTags); err == nil {
		t.Fatal("76 个 Avatar 未被 App 层原子拒绝")
	}
	if err := validateEntityPresentationCounts(make([]render.Avatar, maxFrameAvatars), app.remoteNameTags); err != nil {
		t.Fatalf("%d 个 Avatar 被误拒: %v", maxFrameAvatars, err)
	}
	if err := validateEntityPresentationCounts(app.remoteAvatars, make([]render.NameTag, 13)); err == nil {
		t.Fatal("13 个 NameTag 未被 App 层原子拒绝")
	}
}

// TestApplicationRendersHostilesWithoutNameTags 锁定夜行者的呈现路径：镜像
// 记录进入 avatar 通道（敌怪身份域键），但绝不产生任何名称标签，玩家/伙伴
// 的名标不受影响。
func TestApplicationRendersHostilesWithoutNameTags(t *testing.T) {
	glyphs := &IntegrationGlyphSource{}
	app := newRemoteRenderApplication(t, glyphs)
	app.companions = &client.Companions{}
	configureTargetFeedback(t, app)
	if err := app.remotePlayers.Apply(RemoteSpawn(
		1, "Remote", 1, mgl32.Vec3{0, 2, -4},
	)); err != nil {
		t.Fatal(err)
	}
	companionID := companion.ID(integrationPlayerID(1))
	if err := app.companions.ApplySpawn(network.CompanionSpawn{
		ID: companionID, Name: "阿木", Tick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{1, 2, -8},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.hostiles.ApplySpawn(network.HostileSpawn{ServerTick: 1, Spawns: []network.HostileSpawnRecord{
		{ID: 7, Dimension: core.Overworld, Position: mgl32.Vec3{-2, 1, -6}, Yaw: 0.25, Health: 13},
		{ID: 9, Dimension: core.Overworld, Position: mgl32.Vec3{4, 1, -6}, Yaw: -1.5, Health: 20},
	}}); err != nil {
		t.Fatal(err)
	}

	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("renderFrame=(%v,%v)", rendered, err)
	}
	hostileKeys := map[render.EntityKey]struct{}{
		render.HostileEntityKey(7): {}, render.HostileEntityKey(9): {},
	}
	hostileAvatars := 0
	for _, avatar := range app.remoteAvatars {
		if _, hostile := hostileKeys[avatar.Key]; hostile {
			hostileAvatars++
		}
	}
	if hostileAvatars != 2 {
		t.Fatalf("avatar 通道中的夜行者=%d，想要 2", hostileAvatars)
	}
	for _, tag := range app.remoteNameTags {
		if _, hostile := hostileKeys[tag.Key]; hostile {
			t.Fatalf("夜行者 %v 产生了名称标签", tag.Key)
		}
	}
	// 玩家 + 伙伴 + 目标方块的名标数量不受夜行者影响（3 具名标身体 + 1 目标）。
	if got, want := len(app.remoteNameTags), 3; got != want {
		t.Fatalf("name tags=%d，想要 %d", got, want)
	}

	// despawn 后夜行者从 avatar 通道消失，后续帧不再出现。
	if err := app.hostiles.ApplyDespawn(network.HostileDespawn{ServerTick: 2, IDs: []uint64{7, 9}}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("despawn 后 renderFrame=(%v,%v)", rendered, err)
	}
	for _, avatar := range app.remoteAvatars {
		if _, hostile := hostileKeys[avatar.Key]; hostile {
			t.Fatalf("despawn 后夜行者 %v 仍被渲染", avatar.Key)
		}
	}
}

func TestApplicationRejectsActorOverflowBeforeGPUOrAtlasMutation(t *testing.T) {
	glyphs := &IntegrationGlyphSource{}
	app := newRemoteRenderApplication(t, glyphs)
	configureTargetFeedback(t, app)
	// 76 具身体越界：第 76 具在 App 层被原子拒绝（帧边界稳定报告）。
	for index := range 76 {
		if err := app.remotePlayers.Apply(RemoteSpawn(
			byte(index%255+1), "Remote", 1, mgl32.Vec3{float32(index), 2, -4},
		)); err != nil {
			t.Fatal(err)
		}
	}
	app.scheduler.QueueSection(core.SectionPos{Y: 4}, []mesh.Quad{{
		W: 1, H: 1, Face: mesh.FacePosY, AO: 0xff, Light: 0xf0,
	}})
	app.blockTargetReset = true
	glyphs.requests = 0
	glyphs.flushes = 0
	framesBefore := app.renderer.FrameCalls()

	if rendered, err := app.RenderFrame(1); err == nil || rendered {
		t.Fatalf("overflow RenderFrame=(%v,%v)，想要 false/error", rendered, err)
	}
	if got := app.renderer.FrameCalls(); got != framesBefore {
		t.Fatalf("overflow 后 render FFI=%d,想要保持 %d(GPU 未被触碰)", got, framesBefore)
	}
	if glyphs.requests != 0 || glyphs.flushes != 0 {
		t.Fatalf("overflow 后 atlas request/flush=%d/%d，想要 0/0", glyphs.requests, glyphs.flushes)
	}
	if !app.blockTargetReset {
		t.Fatal("overflow 消费了尚未成功呈现的 block target reset")
	}

	app.remotePlayers.Reset()
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("移除 overflow 后 RenderFrame=(%v,%v)", rendered, err)
	}
	if app.blockTargetReset {
		t.Fatal("成功帧没有消费 block target reset")
	}
	// reset 当帧目标反馈不呈现:无轮廓实例、无名牌。
	if len(app.outlineStream) != 0 || len(app.remoteNameTags) != 0 {
		t.Fatalf("reset 成功帧 outline/名牌=%d/%d,想要 0/0",
			len(app.outlineStream), len(app.remoteNameTags))
	}

	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("reset 后一帧 RenderFrame=(%v,%v)", rendered, err)
	}
	// reset 后一帧恢复:12 实例轮廓 + 目标名牌。
	if len(app.outlineStream) != 12*80 || len(app.remoteNameTags) != 1 {
		t.Fatalf("reset 后一帧 outline/名牌=%d/%d,想要 %d/1",
			len(app.outlineStream), len(app.remoteNameTags), 12*80)
	}
}

func TestApplicationBlockTargetStreamsAndCapacity(t *testing.T) {
	glyphs := &IntegrationGlyphSource{}
	app := newRemoteRenderApplication(t, glyphs)
	configureTargetFeedback(t, app)
	for index, name := range [...]string{"甲", "乙", "丙", "丁", "戊", "己", "庚"} {
		if err := app.remotePlayers.Apply(RemoteSpawn(
			byte(index+1), name, 1, mgl32.Vec3{float32(index), 2, 3},
		)); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.itemDrops.Apply(network.ItemDropUpserts{
		ServerTick: 2,
		Drops: []network.ItemDrop{{
			ID:   core.DropID{Dimension: core.Overworld, Generation: 1},
			Item: core.ItemStone, Count: 1, BlockIndex: 9,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("RenderFrame=(%v,%v)", rendered, err)
	}
	// 各 pass 段齐备:avatar/掉落物/轮廓实例流非空。
	if len(app.avatarStream) == 0 || len(app.dropStream) == 0 || len(app.outlineStream) != 12*80 {
		t.Fatalf("pass 段 avatar/drop/outline=%d/%d/%d",
			len(app.avatarStream), len(app.dropStream), len(app.outlineStream))
	}
	if len(app.remoteNameTags) != 8 || cap(app.remoteNameTags) != MaxFrameNameTags {
		t.Fatalf("name tags len/cap=%d/%d，想要 8/%d", len(app.remoteNameTags), cap(app.remoteNameTags), MaxFrameNameTags)
	}
	wantTargetTag := render.NameTag{
		Key:  render.EntityKey{Kind: render.EntityTarget},
		Text: "砖块", Anchor: mgl32.Vec3{0.5, 4.15, -2.5},
	}
	if got := app.remoteNameTags[7]; got != wantTargetTag {
		t.Fatalf("目标名牌=%+v，想要 %+v", got, wantTargetTag)
	}
	if glyphs.flushes != 1 {
		t.Fatalf("NameTag Prepare/Flush 次数=%d，想要 1", glyphs.flushes)
	}
}

func TestApplicationBlockTargetHiddenByUIAndSessionState(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式:重型测试由 CI 全量门禁运行")
	}
	tests := []struct {
		name string
		hide func(*testing.T, *Application)
	}{
		{name: "背包", hide: func(_ *testing.T, app *Application) { app.inventoryOpen = true }},
		{name: "熔炉", hide: func(t *testing.T, app *Application) {
			if err := app.furnace.Apply(network.FurnaceState{Furnace: core.FurnaceRef{Generation: 1}}); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "箱子", hide: func(t *testing.T, app *Application) {
			if err := app.chest.Apply(network.ChestState{Chest: core.ContainerRef{Kind: core.ContainerKindChest, Generation: 1}}); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "调试面板", hide: func(_ *testing.T, app *Application) {
			app.panel = &panelState{visible: true}
		}},
		{name: "reset 当帧", hide: func(_ *testing.T, app *Application) {
			app.blockTargetReset = true
		}},
		{name: "断线", hide: func(_ *testing.T, app *Application) {
			app.CloseClientSession(nil)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
			configureTargetFeedback(t, app)
			test.hide(t, app)
			if rendered, err := app.RenderFrame(1); err != nil || !rendered {
				t.Fatalf("RenderFrame=(%v,%v)", rendered, err)
			}
			if len(app.remoteNameTags) != 0 {
				t.Fatalf("隐藏状态仍提交 %d 个名牌", len(app.remoteNameTags))
			}
			if len(app.outlineStream) != 0 {
				t.Fatalf("隐藏状态仍提交 %d 字节轮廓实例", len(app.outlineStream))
			}
		})
	}
}

func TestApplicationPlayerResetHidesBlockTargetForOneFrame(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	configureTargetFeedback(t, app)
	clientEndpoint, serverEndpoint := network.NewMemoryPair(4)
	app.clientEndpoint = clientEndpoint
	app.receiver = client.NewReceiver(clientEndpoint, 4)
	t.Cleanup(func() { _ = serverEndpoint.Close() })
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 2, Dimension: core.Overworld, Ready: true, Reset: true,
	})
	app.DrainServerMessages(1)

	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("reset 当帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.outlineStream) != 0 || len(app.remoteNameTags) != 0 {
		t.Fatalf("reset 当帧 outline/名牌=%d/%d,想要 0/0",
			len(app.outlineStream), len(app.remoteNameTags))
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("reset 后一帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.outlineStream) != 12*80 || len(app.remoteNameTags) != 1 {
		t.Fatalf("reset 后一帧 outline/名牌=%d/%d,想要 %d/1",
			len(app.outlineStream), len(app.remoteNameTags), 12*80)
	}
}

func TestApplicationBlockTargetStablePathDoesNotAllocate(t *testing.T) {
	app := targetBlockHitApplication(t)
	remoteTags := make([]render.NameTag, 7, MaxFrameNameTags)
	for index := range remoteTags {
		id := integrationPlayerID(byte(index + 1))
		remoteTags[index] = render.NameTag{
			Key: render.EntityKey{Kind: render.EntityPlayer, ID: [16]byte(id)}, Text: "A",
		}
	}
	glyphs := &IntegrationGlyphSource{}
	nameTags := render.NewNameTagLayouter(glyphs)
	budget := render.NewUploadBudget(1 << 20)
	var tags []render.NameTag
	var outline render.BlockOutline
	var outlineStream []byte
	var outlineEncoder render.InstanceEncoder
	run := func() {
		tags, outline = app.appendCurrentBlockTarget(remoteTags[:7])
		if err := nameTags.Prepare(tags, budget); err != nil {
			panic(err)
		}
		outlineStream = outlineEncoder.EncodeBlockOutlineInstances(outlineStream, outline)
	}
	run()
	if allocations := testing.AllocsPerRun(100, run); allocations != 0 {
		t.Fatalf("稳定目标反馈路径分配=%v，想要 0", allocations)
	}
	if len(tags) != 8 || !outline.Visible || len(outlineStream) != 12*80 {
		t.Fatalf("稳定路径 tags/outline/流=%d/%+v/%d，想要 8/可见/%d",
			len(tags), outline, len(outlineStream), 12*80)
	}
}

func configureTargetFeedback(t *testing.T, app *Application) {
	t.Helper()
	app.camera = client.Camera{Pos: mgl32.Vec3{0.5, 3.5, 2.5}, FovY: mgl32.DegToRad(70), Aspect: 1, Near: 0.1, Far: 100}
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	applyTargetMirrorChunk(t, app.mirror, world.NewChunk(core.ChunkPos{}))
	applyTargetMirrorChunk(t, app.mirror, world.NewChunk(core.ChunkPos{Z: -1}))
	setTargetMirrorBlock(t, app.mirror, core.BlockPos{X: 0, Y: 3, Z: -3}, core.BrickID)
}

// Mutation killed: swallowing or replacing the atlas worker error prevents
// errors.Is from observing it at the frame boundary.
func TestRemoteGlyphErrorPropagatesFromFrame(t *testing.T) {
	glyphErr := errors.New("injected glyph worker failure")
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{FlushErr: glyphErr})
	if err := app.remotePlayers.Apply(RemoteSpawn(1, "Remote-1", 1, mgl32.Vec3{})); err != nil {
		t.Fatal(err)
	}
	rendered, err := app.Frame(0, 0, 25*time.Millisecond)
	if rendered || !errors.Is(err, glyphErr) {
		t.Fatalf("frame=(%v,%v), want wrapped glyph error", rendered, err)
	}
}

func TestApplicationConstructionSkipsPanelWhenDevOff(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if app.panel != nil {
		t.Fatal("Dev 为假时 panel 必须是 nil")
	}
}

func TestApplicationConstructionCreatesPanelWhenDevOn(t *testing.T) {
	rawEndpoint, _ := network.NewMemoryPair(1)
	endpoint := &ConnectionTestEndpoint{ClientEndpoint: rawEndpoint}
	t.Cleanup(func() { _ = rawEndpoint.Close() })
	stream := &connectionTestClientStream{}
	window := &connectionTestWindow{}
	dependencies := NewConnectionTestDependencies(t)
	dependencies.DialTCP = func(context.Context, string) (network.ClientPacketStream, error) {
		return stream, nil
	}
	dependencies.LoginClient = func(
		context.Context, network.ClientPacketStream, network.Identity,
	) (network.ClientEndpoint, uint64, error) {
		return endpoint, 0, nil
	}
	dependencies.NewWindow = func(int, int, string) (Window, error) { return window, nil }
	dependencies.NewWindowedRenderer = func(Window) (*client.Renderer, error) {
		renderer, err := client.NewRenderer(64, 64)
		if errors.Is(err, client.ErrNoGPUAdapter) {
			t.Skip("无 GPU 适配器")
		}
		return renderer, err
	}

	options := remoteConnectionOptions()
	options.Dev = true
	app, err := NewWithDependencies(options, dependencies)
	if err != nil {
		t.Fatalf("New dev=true: %v", err)
	}
	if app.panel == nil {
		t.Fatal("Dev 为真时 panel 不能是 nil")
	}
	if err := app.Close(); err != nil {
		t.Fatalf("dev=true Application Close: %v", err)
	}
}

// Mutation killed: 未确认权威快捷栏时提交 HUD 段会改变段字节。
func TestApplicationDrawsHotbarHUDOnlyWhenConfirmed(t *testing.T) {
	glyphs := &IntegrationGlyphSource{}
	app := newRemoteRenderApplication(t, glyphs)
	if err := app.remotePlayers.Apply(RemoteSpawn(1, "Remote-1", 1, mgl32.Vec3{1, 2, 3})); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("未确认快捷栏 RenderFrame=(%v,%v)", rendered, err)
	}
	if _, quads, _ := app.hotbarRenderer.FrameStreams(); len(quads) != 0 {
		t.Fatalf("未确认快捷栏仍产生 %d 字节 HUD quad", len(quads))
	}

	var confirmed core.Hotbar
	confirmed.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 4}
	if err := app.inventory.Apply(network.InventoryState{Inventory: core.Inventory{Hotbar: confirmed}}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("已确认快捷栏 RenderFrame=(%v,%v)", rendered, err)
	}
	if _, quads, _ := app.hotbarRenderer.FrameStreams(); len(quads) == 0 {
		t.Fatal("已确认快捷栏没有产生 HUD quad")
	}
}

// Mutation killed: 渲染动画把插值状态写回镜像会改变 Presentations。
func TestApplicationItemDropsDoNotMutateMirror(t *testing.T) {
	glyphs := &IntegrationGlyphSource{}
	app := newRemoteRenderApplication(t, glyphs)
	drop := network.ItemDrop{
		ID:   core.DropID{Dimension: core.Overworld, Slot: 0, Generation: 1},
		Item: core.ItemStone, Count: 1, BlockIndex: 9,
	}
	if err := app.itemDrops.Apply(network.ItemDropUpserts{
		ServerTick: 3, Drops: []network.ItemDrop{drop},
	}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("有掉落物 RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.dropStream) == 0 {
		t.Fatal("掉落物未进入实例流")
	}
	got := app.itemDrops.Presentations()
	if len(got) != 1 || got[0].BlockIndex != drop.BlockIndex || got[0].Count != drop.Count {
		t.Fatalf("渲染修改了镜像: %+v", got)
	}
}

func TestApplicationItemDropMirrorResetsWithSession(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	sendInteractiveServerMessage(t, serverEndpoint, network.ItemDropUpserts{
		ServerTick: 1,
		Drops: []network.ItemDrop{{
			ID:   core.DropID{Dimension: core.Overworld, Slot: 0, Generation: 1},
			Item: core.ItemGrass, Count: 1, BlockIndex: 4,
		}},
	})
	app.DrainServerMessages(1)
	if len(app.itemDrops.Presentations()) != 1 {
		t.Fatal("掉落物 upsert 未进入镜像")
	}

	app.CloseClientSession(nil)
	if got := app.itemDrops.Presentations(); len(got) != 0 {
		t.Fatalf("关闭会话后镜像 = %+v，想要为空", got)
	}
}

func TestCombatMarkerOnlyConsumedAfterSuccessfulNativeRender(t *testing.T) {
	glyphs := &IntegrationGlyphSource{}
	app := newRemoteRenderApplication(t, glyphs)
	configureTargetFeedback(t, app)
	// 确保有 HUD 可见，避免零 HUD 路径不进 Prepare
	var inv core.Hotbar
	inv.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.Inventory().Apply(network.InventoryState{Inventory: core.Inventory{Hotbar: inv}}); err != nil {
		t.Fatal(err)
	}
	app.ArmCombatMarker()
	if !app.CombatMarkerVisible() {
		t.Fatal("Arm 后不可见")
	}
	// 零 framebuffer 不扣帧
	origWindow := app.window
	origW, origH := app.frameWidth, app.frameHeight
	app.window = &zeroFramebufferWindow{}
	if rendered, err := app.RenderFrame(1); err != nil || rendered {
		t.Fatalf("零 framebuffer RenderFrame=(%v,%v)", rendered, err)
	}
	if !app.CombatMarkerVisible() {
		t.Fatalf("零 framebuffer 后 marker 被消耗")
	}
	// 恢复
	app.window = origWindow
	app.frameWidth, app.frameHeight = origW, origH
	// entity overflow 不扣帧
	for i := range 76 {
		if err := app.RemotePlayers().Apply(RemoteSpawn(byte(i%255+1), "R", 1, mgl32.Vec3{float32(i), 2, -4})); err != nil {
			t.Fatal(err)
		}
	}
	if rendered, err := app.RenderFrame(1); err == nil || rendered {
		t.Fatalf("overflow RenderFrame=(%v,%v) want error", rendered, err)
	}
	if !app.CombatMarkerVisible() {
		t.Fatalf("overflow 后 marker 被消耗")
	}
	app.RemotePlayers().Reset()
	// name-tag prepare error 不扣帧
	glyphs.FlushErr = errors.New("glyph fail")
	if err := app.RemotePlayers().Apply(RemoteSpawn(1, "A", 1, mgl32.Vec3{0, 2, -4})); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err == nil || rendered {
		t.Fatalf("nameTag error RenderFrame=(%v,%v)", rendered, err)
	}
	if !app.CombatMarkerVisible() {
		t.Fatalf("nameTag error 后 marker 被消耗")
	}
	glyphs.FlushErr = nil
	// HUD prepare error 不扣帧：需要 HUD 可见且触发 flush
	glyphs.FlushErr = errors.New("hud fail")
	// 确保 HUD 可见：已通过 inventory 确认
	if rendered, err := app.RenderFrame(1); err == nil || rendered {
		t.Fatalf("HUD error RenderFrame=(%v,%v)", rendered, err)
	}
	if !app.CombatMarkerVisible() {
		t.Fatalf("HUD error 后 marker 被消耗")
	}
	glyphs.FlushErr = nil
	// 成功渲染才扣一帧
	initial := app.CombatMarkerVisible()
	if !initial {
		t.Fatal("成功前 marker 不可见")
	}
	remainingBefore := app.combatFeedback.remainingFrames
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("成功 RenderFrame=(%v,%v)", rendered, err)
	}
	if app.combatFeedback.remainingFrames != remainingBefore-1 {
		t.Fatalf("成功后剩余帧未递减")
	}
	// 再成功 5 次后不可见
	for i := 0; i < 5; i++ {
		if _, err := app.RenderFrame(1); err != nil {
			t.Fatal(err)
		}
	}
	if app.CombatMarkerVisible() {
		t.Fatalf("六次成功后仍可见")
	}
}

type zeroFramebufferWindow struct{}

func (w *zeroFramebufferWindow) SetCursorCaptured(bool)               {}
func (w *zeroFramebufferWindow) CursorPos() (float64, float64)        { return 0, 0 }
func (w *zeroFramebufferWindow) ShouldClose() bool                    { return false }
func (w *zeroFramebufferWindow) Poll()                                {}
func (w *zeroFramebufferWindow) DrainTextInput([]rune) ([]rune, bool) { return nil, false }
func (w *zeroFramebufferWindow) KeyDown(client.Key) bool              { return false }
func (w *zeroFramebufferWindow) PrimaryButtonDown() bool              { return false }
func (w *zeroFramebufferWindow) SecondaryButtonDown() bool            { return false }
func (w *zeroFramebufferWindow) CursorCaptured() bool                 { return false }
func (w *zeroFramebufferWindow) Focus()                               {}
func (w *zeroFramebufferWindow) PushUIState([]byte)                   {}
func (w *zeroFramebufferWindow) FramebufferSize() (int, int)          { return 0, 0 }
func (w *zeroFramebufferWindow) ContentSize() (int, int)              { return 0, 0 }
func (w *zeroFramebufferWindow) SetContentSize(int, int)              {}
func (w *zeroFramebufferWindow) CancelClose()                         {}
func (w *zeroFramebufferWindow) Close()                               {}
