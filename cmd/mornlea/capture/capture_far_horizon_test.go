package capture

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/lod"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestFarHorizonCaptureSceneIsRegistered 锁住 far-horizon 场景在清单中的
// 存在与形态(spec delta「MUST 新增 far-horizon 视觉场景作为长期门禁」):
// 追加在列表末尾(场景共用 application,顺序即状态继承链),相机钉在
// 近环边缘内侧的高空、朝 -z 地平线观察,画面同时包含近景地形、远环壳
// 带、雾过渡带与天空。
func TestFarHorizonCaptureSceneIsRegistered(t *testing.T) {
	var scene captureScene
	found := false
	for _, candidate := range captureScenes {
		if candidate.Name == "far-horizon" {
			scene = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatal("缺少 far-horizon 场景")
	}
	if scene.WarmupFrames != 8 || scene.Apply == nil {
		t.Fatalf("场景=%+v,想要 WarmupFrames=8 且 Apply 齐备", scene)
	}
	// 变基排序协调(Ruling 21):fluid 规格钉死 water-underwater 必须排在
	// 场景表最后(其注入的权威 `PlayerState` 会永久钉住浸没标志),far-horizon
	// 插在它之前(倒数第二)。原断言「必须追加在表尾」是 fluid 合并前的
	// 事实,变基后由本断言取代:倒数第二 = far-horizon,末位 = water-underwater。
	// far-horizon 的 `Apply` 经 `resetCapturePresentation` 清空全部共享呈现
	// 状态,对前序场景不构成顺序依赖。
	if captureScenes[len(captureScenes)-2].Name != "far-horizon" {
		t.Fatal("far-horizon 必须排在倒数第二(water-underwater 固定居末)")
	}
	if captureScenes[len(captureScenes)-1].Name != "water-underwater" {
		t.Fatal("water-underwater 必须排在场景表最后(fluid 规格)")
	}
}

// TestFarHorizonApplyPinsCameraAndResetsSharedState 杀死这些变异:相机
// 未钉死(位置/朝向/ FOV 随登录或前一场景漂移)、远环装配事实被改动
// (a.center/ `lodTileCenter`,会触发近环释放与远环增量入队)、或继承
// ai-companion 留下的物品栏/聊天/伙伴/界面状态。
func TestFarHorizonApplyPinsCameraAndResetsSharedState(t *testing.T) {
	var scene captureScene
	for _, candidate := range captureScenes {
		if candidate.Name == "far-horizon" {
			scene = candidate
			break
		}
	}
	if scene.Apply == nil {
		t.Fatal("缺少 far-horizon Apply")
	}

	// 呈现状态替身经 app 导出测试装配入口构造，再把本测试预置的 roster 与
	// 界面状态逐一注入，等价于旧的白盒字面量初始条件。
	app := application.NewPresentationApplicationForTest()
	if err := app.RemotePlayers().Apply(network.RemotePlayerSpawn{
		PlayerID:    core.PlayerID{6: 0x40, 8: 0x80, 15: 1},
		DisplayName: "测试Player",
		ServerTick:  1, Position: mgl32.Vec3{0, 2, 0},
	}); err != nil {
		t.Fatal(err)
	}
	if app.Panel() == nil {
		t.Fatal("呈现替身必须携带调试面板")
	}
	app.Panel().SetVisible(true)
	app.SetInventoryOpen(true)
	app.SetInventorySource(12)
	app.SetCenter(core.ChunkPos{X: 0, Z: 0})
	app.SetLODTileCenter(lod.TilePos{X: 0, Z: 0})
	*app.Camera() = client.Camera{Pos: mgl32.Vec3{99, 99, 99}, Yaw: 2, Pitch: 1}
	if err := app.Furnace().Apply(network.FurnaceState{
		Furnace: core.FurnaceRef{Dimension: core.Overworld, Generation: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.Chest().Apply(network.ChestState{
		Chest: core.ContainerRef{
			Dimension: core.Overworld, Kind: core.ContainerKindChest, Generation: 1,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用场景: %v", err)
	}
	if app.WorldTimeTicks() != 6000 {
		t.Fatalf("world time = %d, want 6000", app.WorldTimeTicks())
	}
	if got := app.Camera().Pos; got != (mgl32.Vec3{8, 110, -352}) {
		t.Fatalf("camera.Pos = %v, want (8,110,-352)(近环边缘 -z 内侧高空)", got)
	}
	if app.Camera().Yaw != 0 || app.Camera().Pitch != -0.25 {
		t.Fatalf("yaw/pitch = %v/%v, want 0/-0.25(朝 -z 地平线)", app.Camera().Yaw, app.Camera().Pitch)
	}
	// 远环与近环的装配事实必须原样保留:场景不得移动视距中心,否则
	// 触发近环 `DropOutside` 与远环增量入队,收敛域随场景执行漂移。
	if app.Center() != (core.ChunkPos{X: 0, Z: 0}) {
		t.Fatalf("center = %v, want (0,0)(场景不得移动视距中心)", app.Center())
	}
	if app.LODTileCenter() != (lod.TilePos{X: 0, Z: 0}) {
		t.Fatalf("lodTileCenter = %v, want (0,0)", app.LODTileCenter())
	}
	if app.InventoryOpen() || app.InventorySource() != -1 || app.Panel().Visible() ||
		len(app.RemotePlayers().Presentations()) != 0 {
		t.Fatalf("共享状态未清空: inventory=%v/%d panel=%v remotes=%d",
			app.InventoryOpen(), app.InventorySource(), app.Panel().Visible(),
			len(app.RemotePlayers().Presentations()))
	}
	if _, opened := app.Furnace().State(); opened {
		t.Fatal("熔炉状态未清空")
	}
	if _, opened := app.Chest().State(); opened {
		t.Fatal("箱子状态未清空")
	}
}
