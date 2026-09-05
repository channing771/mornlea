package capture

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/world"
	"github.com/go-gl/mathgl/mgl32"
)

const motionMaxFrames = 180

// `prepareAvatarStage` 通过正式镜像和网格铺设有距离参照的草地。
func prepareAvatarStage(app SceneApplication) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}
	blocks := make(map[core.ChunkPos]map[core.BlockPos]core.BlockID)
	for z := int32(-15); z <= 8; z++ {
		for x := int32(-8); x <= 8; x++ {
			pos := core.BlockPos{X: x, Y: 0, Z: z}
			if blocks[pos.Chunk()] == nil {
				blocks[pos.Chunk()] = make(map[core.BlockPos]core.BlockID)
			}
			block := core.GrassID
			if z%2 == 0 && x == -2 {
				block = core.StoneBrickID
			}
			blocks[pos.Chunk()][pos] = block
		}
	}
	return applyCaptureBlocks(app, blocks, 1, "人物与掉落舞台")
}

func applyExperienceStage(app SceneApplication) error {
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(6000)
	app.SetServerTick(1)
	*app.Camera() = client.Camera{Pos: mgl32.Vec3{0, 2.7, 7}, Pitch: -.17, FovY: mgl32.DegToRad(50), Aspect: float32(captureWidth) / captureHeight, Near: .1, Far: 2000}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	return nil
}

func spawnExperienceAvatar(app SceneApplication, id byte, pos mgl32.Vec3, yaw float32) error {
	return app.RemotePlayers().Apply(network.RemotePlayerSpawn{PlayerID: core.PlayerID{6: 0x40, 8: 0x80, 15: id}, DisplayName: fmt.Sprintf("旅人 %d", id), ServerTick: app.ServerTick(), Position: pos, Yaw: yaw})
}

// `applyAvatarDetail` 同帧呈现正侧背；运动相位由独立 GIF 验收。
func applyAvatarDetail(app SceneApplication) error {
	if err := applyExperienceStage(app); err != nil {
		return err
	}
	for index, yaw := range []float32{math.Pi, math.Pi / 2, 0} {
		if err := spawnExperienceAvatar(app, byte(index+1), mgl32.Vec3{float32(index-1) * 2.1, 1, 0}, yaw); err != nil {
			return err
		}
	}
	return nil
}

// `avatarWalkDistance` 用真实 20Hz：慢走 40 tick、快走 20 tick，各走 4.3 格。
func avatarWalkDistance(frame int) float32 {
	slow := min(max(frame-19, 0), 40)
	fast := min(max(frame-59, 0), 20)
	return float32(slow)*2.15/20 + float32(fast)*4.3/20
}

func motionDropCount(scene string, frame int) int {
	if frame < 10 {
		return 0
	}
	if scene == "drop-scatter" {
		return 4
	}
	switch {
	case frame < 30:
		return 1
	case frame < 50:
		return 4
	case frame < 70:
		return 9
	case frame < 90:
		return 16
	case frame < 130:
		return 32
	default:
		return 16
	}
}

func applyExperienceMotionFrame(app SceneApplication, scene string, frame int) error {
	tick := uint64(frame + 2)
	app.SetServerTick(tick)
	app.SetBlockTargetReset(true)
	if scene == "avatar-walk" {
		distance := avatarWalkDistance(frame)
		// 镜像重装仅钉住确定性位置，稳定实体 ID 保留生产编码器的距离累积。
		app.RemotePlayers().Reset()
		if err := spawnExperienceAvatar(app, 1, mgl32.Vec3{0, 1, -distance}, 0); err != nil {
			return err
		}
		app.Camera().Pos = mgl32.Vec3{3, 2.6, 4.5 - distance}
		app.Camera().Yaw = .59
		app.Camera().Pitch = -.16
		app.SetCenter(application.CameraChunk(app.Camera().Pos))
		return nil
	}
	count := motionDropCount(scene, frame)
	prior := motionDropCount(scene, frame-1)
	block := core.BlockPos{X: 0, Y: 2, Z: 0}
	blockIndex, _ := world.ChunkBlockIndex(block)
	items := []core.ItemID{core.ItemStone, core.ItemRawBeef, core.ItemIronPickaxe, core.ItemWheat, core.ItemGrass, core.ItemBread, core.ItemIronIngot, core.ItemCarrot}
	id := func(index int) core.DropID {
		return core.DropID{Dimension: core.Overworld, Chunk: block.Chunk(), Slot: uint8(index), Generation: uint32(index + 1)}
	}
	if count > prior {
		drops := make([]network.ItemDrop, 0, count-prior)
		for index := prior; index < count; index++ {
			durability, _ := core.ItemMaxDurability(items[index%len(items)])
			drops = append(drops, network.ItemDrop{ID: id(index), BlockIndex: blockIndex, Item: items[index%len(items)], Count: 1, Durability: durability})
		}
		if err := app.ItemDrops().Apply(network.ItemDropUpserts{ServerTick: tick, Drops: drops}); err != nil {
			return err
		}
	} else if count < prior {
		ids := make([]core.DropID, 0, prior-count)
		for index := count; index < prior; index++ {
			ids = append(ids, id(index))
		}
		if err := app.ItemDrops().Apply(network.ItemDropRemoves{ServerTick: tick, IDs: ids}); err != nil {
			return err
		}
	}
	return nil
}

func captureBoundedMotionFrames(count int, capture func(int) (*image.NRGBA, error)) ([]*image.NRGBA, error) {
	if count <= 0 || count > motionMaxFrames {
		return nil, fmt.Errorf("motion 帧预算 %d 越界", count)
	}
	frames := make([]*image.NRGBA, 0, count)
	for frame := 0; frame < count; frame++ {
		img, err := capture(frame)
		if err != nil {
			return nil, fmt.Errorf("motion 帧 %d: %w", frame, err)
		}
		if img == nil {
			return nil, fmt.Errorf("motion 帧 %d 为空", frame)
		}
		frames = append(frames, img)
	}
	return frames, nil
}

// `RunMotion` 只接受显式有界剧本，复用应用渲染与 GIF 编码；出生在正式帧注入。
func RunMotion(app SceneApplication, outPath, scene string) error {
	if scene == "break-burst" {
		return RunBreakBurstMotion(app, outPath)
	}
	count := 100
	switch scene {
	case "avatar-walk":
	case "drop-scatter":
		count = 80
	case "drop-density":
		count = 160
	default:
		return fmt.Errorf("未知 motion 场景 %q", scene)
	}
	if outPath == "" {
		return fmt.Errorf("motion 输出路径为空")
	}
	if err := prepareCaptureApplication(app); err != nil {
		return err
	}
	app.SetWorldTimeFrozen(true)
	defer app.SetWorldTimeFrozen(false)
	stage := captureScene{Name: scene, WarmupFrames: 8, Prepare: prepareAvatarStage, Apply: func(app SceneApplication) error {
		if err := applyExperienceStage(app); err != nil {
			return err
		}
		if scene == "avatar-walk" {
			if err := spawnExperienceAvatar(app, 1, mgl32.Vec3{0, 1, 0}, 0); err != nil {
				return err
			}
		} else {
			app.Camera().Pos = mgl32.Vec3{1.8, 2.6, 3.5}
			app.Camera().Yaw = .393
			app.Camera().Pitch = -.23
			app.Camera().FovY = mgl32.DegToRad(42)
		}
		return nil
	}}
	if _, err := captureSceneImage(app, stage); err != nil {
		return err
	}
	frames, err := captureBoundedMotionFrames(count, func(frame int) (*image.NRGBA, error) {
		if err := applyExperienceMotionFrame(app, scene, frame); err != nil {
			return nil, err
		}
		if _, err := app.RenderFrame(captureDrainMax); err != nil {
			return nil, err
		}
		return bgraToNRGBA(app.Renderer().Readback(), captureWidth, captureHeight), nil
	})
	if err != nil {
		return err
	}
	frameDir := outPath + "-frames"
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return err
	}
	for index, frame := range frames {
		if index%10 == 0 || index == len(frames)-1 {
			if err := writePNG(filepath.Join(frameDir, fmt.Sprintf("%03d.png", index)), frame); err != nil {
				return err
			}
		}
	}
	data, err := encodeMotionGIF(frames, 5)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("已生成 motion %s：%s（%d 帧，20Hz）\n", scene, outPath, count)
	return nil
}
