package capture

// capture_passive_graze.go 装配放牧吃草的无窗口昼间 capture 场景：固定正午
//（6000 tick，与牛群场景同相位）的开阔草地，2 头牛经客户端被动镜像夹具站在
// 草地上——其中 1 头经与权威消息相同的 state 入口置放牧位（低头），另 1 头
// 保持常态作对照；低头牛吻部身前的那格经方块夹具摆成泥土（吃草结算把脚下草
// 方块变为泥土的前后对照），周围仍是草地。场景只呈现镜像事实，不依赖任何
// 服务端模拟推进，因此与机器速度无关。
//
// 构图沿用牛群场景的已验证机位（见 `applyPassiveGrazeCaptureState`）：低头牛
// 在画面中央呈侧面（yaw 0，面朝 +X 即屏幕右侧，低头俯转在侧面剪影里最可辨），
// 吻部正下方西缘即泥土格；常态牛在左侧同框对照。调整坐标前先复算投影。
//
// 本场景不含掉落物与任何随机器速度变化的读数：位姿在 Apply 里经镜像钉死，
// 收敛帧内不再 drain，因此无需 PinVolatile 即可确定。

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// capturePassiveGrazeSpawns 是 2 头牛的出生批次（tick 1）：ID 严格升序、生命
// 全部为满值。出生不带瞬态，两头默认都是常态位姿，放牧位由随后的 state 批次
// 单独置位——与线上“出生身体不含瞬态、state 批次投影放牧位”的语义同形。
var capturePassiveGrazeSpawns = []network.PassiveSpawnRecord{
	{ID: 211, Dimension: core.Overworld, Position: mgl32.Vec3{0.3, 1, 0.5}, Yaw: 0, Health: core.MaxHealth},
	{ID: 212, Dimension: core.Overworld, Position: mgl32.Vec3{-3.4, 1, 1.2}, Yaw: 0.7, Health: core.MaxHealth},
}

// capturePassiveGrazeStates 是 2 头牛的权威 state 批次（tick 2）：位置与朝向
// 与出生批次逐字段一致（插值落点因此与快照数无关，断言确定），只有放牧位不
// 同——211 置位（低头）、212 清位（常态对照）。
var capturePassiveGrazeStates = []network.PassiveStateRecord{
	{ID: 211, Position: mgl32.Vec3{0.3, 1, 0.5}, Velocity: mgl32.Vec3{}, Yaw: 0, Health: core.MaxHealth, Grazing: 1},
	{ID: 212, Position: mgl32.Vec3{-3.4, 1, 1.2}, Velocity: mgl32.Vec3{}, Yaw: 0.7, Health: core.MaxHealth, Grazing: 0},
}

// capturePassiveGrazeDirt 是吃草结算的那格：低头牛（211 站位 x=0.3、头部相对
// 偏移 +0.7，吻部世界 x≈1.0）身前的泥土块，与周围草地形成一格对照。牛脚底
// 格仍是草地——结算只写一格，画面里泥土恰为一格。
var capturePassiveGrazeDirt = core.BlockPos{X: 1, Y: 0, Z: 0}

// capturePassiveGrazeServerTick 是抓帧最终帧钉死的权威 tick：常态牛的闲时点
// 头相位是该 tick 与牛 ID 的纯函数，取值与加载耗时无关，基线因此可复现（与
// 牛群场景的掉落相位钉死同纪律）。
const capturePassiveGrazeServerTick = 64

// preparePassiveGrazeDay 装入吃草场景的世界夹具：与牛群场景同 footprint 的
// 一层 y=0 草地（上表面 y=1，横向 x=±10、纵向 z=-16..8，近端延伸到相机下方
// 不露悬空断口），唯独吃草结算格摆成泥土。白昼场景不需要火把：正午日光是唯
// 一的固定光源。
func preparePassiveGrazeDay(app SceneApplication) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}
	blocks := make(map[core.ChunkPos]map[core.BlockPos]core.BlockID)
	setBlock := func(position core.BlockPos, block core.BlockID) {
		chunk := position.Chunk()
		if blocks[chunk] == nil {
			blocks[chunk] = make(map[core.BlockPos]core.BlockID)
		}
		blocks[chunk][position] = block
	}
	for z := int32(-16); z <= 8; z++ {
		for x := int32(-10); x <= 10; x++ {
			setBlock(core.BlockPos{X: x, Y: 0, Z: z}, core.GrassID)
		}
	}
	setBlock(capturePassiveGrazeDirt, core.DirtID)
	return applyCaptureBlocks(app, blocks, captureWaterBasinChunkRadius, "吃草草地")
}

// applyPassiveGrazeCaptureState 钉死本场景的全部呈现状态并装入牛群镜像。
// 清理复用 `resetCapturePresentation`（含被动牛镜像），保证前一场景留下的
// 牛群不渗入本场景；随后按 spawn→state 两批经与权威消息相同的 Apply 入口
// 注入 2 头牛（含 1 头放牧置位）。注入在 Apply 中完成（drain 之后、收敛帧
// 之前），不会被任何服务端消息覆盖。
func applyPassiveGrazeCaptureState(app SceneApplication) error {
	if app.Passives() == nil {
		return fmt.Errorf("passive-graze 需要被动牛镜像，当前为 nil")
	}
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(6000)
	// 相机沿用牛群场景的已验证机位：钉在草地上方平视牛群，低头牛居中呈侧面、
	// 常态牛在左侧同框对照、泥土格落在低头牛吻部下方。
	*app.Camera() = client.Camera{
		Pos: mgl32.Vec3{0.5, 2.8, 6.5}, Yaw: 0, Pitch: -0.12,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	app.SetBlockTargetReset(false)

	if err := app.Passives().ApplySpawn(network.PassiveSpawn{
		ServerTick: 1, Spawns: capturePassiveGrazeSpawns,
	}); err != nil {
		return fmt.Errorf("装入吃草牛群出生批次: %w", err)
	}
	if err := app.Passives().ApplyStates(network.PassiveState{
		ServerTick: 2, States: capturePassiveGrazeStates,
	}); err != nil {
		return fmt.Errorf("装入吃草牛群放牧位批次: %w", err)
	}
	// 夹具自检：批次的 ID 升序由镜像校验兜底，这里只锁数量与放牧位分布——
	// 少装任何一头或放牧位错位都会让 golden 缺像素或错位姿，宁可当场失败也不
	// 产出静默错误的基线。
	presentations := app.Passives().AppendPresentations(nil)
	if len(presentations) != len(capturePassiveGrazeSpawns) {
		return fmt.Errorf("被动牛镜像数量=%d，想要 %d", len(presentations), len(capturePassiveGrazeSpawns))
	}
	if !presentations[0].Grazing || presentations[1].Grazing {
		return fmt.Errorf("被动牛放牧位=%v，想要 [true false]", presentations)
	}
	return nil
}

// pinPassiveGrazeVolatile 把闲时点头相位钉死为常量。收敛帧本身会推进权威
// tick（帧间隔、加载耗时都随机器速度变化），在 Apply 里钉死会被它们重新
// 覆盖，因此必须在收敛帧之后、最后一帧之前重钉（`PinVolatile` 语义）。
func pinPassiveGrazeVolatile(app SceneApplication) error {
	app.SetServerTick(capturePassiveGrazeServerTick)
	return nil
}
