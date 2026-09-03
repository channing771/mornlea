package capture

import (
	"fmt"
	"sort"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/world"
)

var captureAICompanionID = companion.ID{0: 0x42, 6: 0x40, 8: 0x80, 15: 0x14}

func prepareAICompanion(app SceneApplication) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}
	changes := make([]network.BlockChange, 0, 11*13*2)
	for z := int32(0); z <= 12; z++ {
		for x := int32(0); x <= 10; x++ {
			changes = append(changes,
				network.BlockChange{Position: core.BlockPos{X: x, Y: -1, Z: z}, Block: core.StoneID},
				network.BlockChange{Position: core.BlockPos{X: x, Y: 0, Z: z}, Block: core.GrassID},
			)
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		left, _ := world.ChunkBlockIndex(changes[i].Position)
		right, _ := world.ChunkBlockIndex(changes[j].Position)
		return left < right
	})
	return applyCaptureMirror(app, network.BlockChanges{
		Dimension: core.Overworld, Chunk: core.ChunkPos{},
		BaseRevision: 1, NewRevision: 2, Changes: changes,
	})
}

func applyAICompanionCaptureState(app SceneApplication) error {
	if app.RemotePlayers() == nil || app.Companions() == nil || app.ChatEvents() == nil || app.ItemDrops() == nil {
		return fmt.Errorf("ai-companion 需要完整客户端呈现镜像")
	}

	app.RemotePlayers().Reset()
	app.Companions().Reset()
	app.ChatEvents().Reset()
	app.ChatInput().Cancel()
	app.SetChatEventBuffer([client.ChatEventCapacity]network.ChatEvent{})
	app.SetChatLines([6]string{})
	app.SetChatLineCount(0)
	app.SetFormattedChatEventID(0)
	app.Inventory().Reset()
	app.SetInventoryOpen(false)
	app.SetInventorySource(-1)
	app.Furnace().Reset()
	app.Chest().Reset()
	// 合成网格镜像一并清空：ai-companion 场景不呈现容器与网格，前序容器
	// 场景（含 workbench-crafting 的尺寸 3 状态）不得静默渗入。
	app.Crafting().Reset()
	app.SetMiningOverlay(hud.MiningOverlay{})
	app.SetDamageFeedback(application.DamageFeedback{})
	app.SetDamageStrength(0)
	app.ItemDrops().Reset()
	app.SetRemotePresentations(app.RemotePresentations()[:0])
	app.SetCompanionPresentations(app.CompanionPresentations()[:0])
	app.SetRemoteAvatars(app.RemoteAvatars()[:0])
	app.SetRemoteNameTags(app.RemoteNameTags()[:0])
	app.SetItemDropInstances(app.ItemDropInstances()[:0])
	if app.Panel() != nil {
		app.Panel().SetVisible(false)
	}

	app.SetWorldTimeTicks(6000)
	*app.Camera() = client.Camera{
		Pos: mgl32.Vec3{5.5, 3.2, 9.5}, Yaw: 0, Pitch: -0.05,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	app.SetBlockTargetReset(false)

	if err := app.Companions().ApplySpawn(network.CompanionSpawn{
		ID: captureAICompanionID, Name: "阿木", Tick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{5.5, 1, 4}, Yaw: 3.1415927,
	}); err != nil {
		return fmt.Errorf("装入固定伙伴: %w", err)
	}
	if err := app.ChatEvents().Apply(network.ChatEvent{
		EventID: 1, PlayerID: core.PlayerID{0: 0x23, 6: 0x40, 8: 0x80, 15: 0x11},
		PlayerName: "旅人", CompanionID: captureAICompanionID, CompanionName: "阿木",
		Kind: network.ChatEventAccepted, Command: "挖石头",
	}); err != nil {
		return fmt.Errorf("装入固定聊天事件: %w", err)
	}
	app.ChatInput().Open()
	for _, value := range "@阿木 挖石头" {
		app.ChatInput().Append(value)
	}
	return nil
}

func prepareSkylightTunnel(app SceneApplication) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}

	stones := make(map[core.ChunkPos]map[core.BlockPos]struct{})
	addStone := func(position core.BlockPos) {
		chunk := position.Chunk()
		if stones[chunk] == nil {
			stones[chunk] = make(map[core.BlockPos]struct{})
		}
		stones[chunk][position] = struct{}{}
	}
	// 内部宽 5、高 4、长 20；入口 z=4 露天，z=-16 的后墙阻止背面漏光。
	for z := int32(-15); z <= 4; z++ {
		for x := int32(-3); x <= 3; x++ {
			addStone(core.BlockPos{X: x, Y: 0, Z: z})
			if z < 4 {
				addStone(core.BlockPos{X: x, Y: 5, Z: z})
			}
		}
		for y := int32(1); y <= 4; y++ {
			addStone(core.BlockPos{X: -3, Y: y, Z: z})
			addStone(core.BlockPos{X: 3, Y: y, Z: z})
		}
	}
	for y := int32(0); y <= 5; y++ {
		for x := int32(-3); x <= 3; x++ {
			addStone(core.BlockPos{X: x, Y: y, Z: -16})
		}
	}
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			chunk := core.ChunkPos{X: x, Z: z}
			changes := make([]network.BlockChange, 0, len(stones[chunk]))
			for position := range stones[chunk] {
				changes = append(changes, network.BlockChange{
					Position: position,
					Block:    core.StoneID,
				})
			}
			sort.Slice(changes, func(i, j int) bool {
				left, _ := world.ChunkBlockIndex(changes[i].Position)
				right, _ := world.ChunkBlockIndex(changes[j].Position)
				return left < right
			})
			if err := applyCaptureMirror(app, network.BlockChanges{
				Dimension:    core.Overworld,
				Chunk:        chunk,
				BaseRevision: 1,
				NewRevision:  2,
				Changes:      changes,
			}); err != nil {
				return fmt.Errorf("装入通道变化 (%d,%d): %w", x, z, err)
			}
		}
	}
	return nil
}

func prepareCaptureAirNeighborhood(app SceneApplication) error {
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			sections := make([]network.SectionData, core.SectionsPerChunk)
			for y := range sections {
				sections[y] = network.SectionData{
					Y: int32(y), Storage: network.SectionSingle, Single: core.AirID,
				}
			}
			if err := applyCaptureMirror(app, network.ChunkSnapshot{
				Dimension: core.Overworld,
				Chunk:     core.ChunkPos{X: x, Z: z},
				Revision:  1,
				Sections:  sections,
			}); err != nil {
				return fmt.Errorf("装入空气快照 (%d,%d): %w", x, z, err)
			}
		}
	}
	return nil
}

func prepareBlockLightRoom(app SceneApplication) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}
	return applyCaptureBlockLightRoomChanges(app)
}

// prepareTorchNightRoom 装入火把夜景夹具：一间全封闭的长石室（地板 y=0、
// 天花板 y=6、四壁合围，纵深 z=-16..2），室内空气从 y=1..5、x=-5..5、
// z=-15..1。封闭是硬要求：场景在夜晚（18000 tick）抓帧，天空光贡献近乎
// 为零，室内亮度完全由火把的方块光决定；墙体一旦留洞，室外的夜空会混进
// 画面，光衰减梯度就不再只由火把解释。纵深刻意拉长：相机在房间一端平视
// 另一端，远角与近处火把的距离差足够大，近亮远暗在画面里是一段可测的
// 梯度而不是两个相邻色块。夹具经客户端只读镜像装入，不依赖任何服务端
// 模拟推进。
func prepareTorchNightRoom(app SceneApplication) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}
	return applyCaptureTorchNightChanges(app)
}

// applyCaptureTorchNightChanges 写入石室外壳与三朵火把：落地一朵在相机近处
// 右侧，左右墙各一朵墙面形态。形态与支撑格的关系按放置规则的冻结方向表
// （墙面形态名 = 命中面名，支撑位于火把的反方向一格），三朵的支撑全是实心
// 石块，夹具因此与真实放置结果一致、不是悬空摆拍。墙面形态选 ±X 两种：
// 这两种形态的可见斜板朝 ±Z，正对纵深方向平视的相机恰好各看到一片完整
// 的火柄斜影。
func applyCaptureTorchNightChanges(app SceneApplication) error {
	blocks := make(map[core.ChunkPos]map[core.BlockPos]core.BlockID)
	setBlock := func(position core.BlockPos, block core.BlockID) {
		chunk := position.Chunk()
		if blocks[chunk] == nil {
			blocks[chunk] = make(map[core.BlockPos]core.BlockID)
		}
		blocks[chunk][position] = block
	}
	for z := int32(-16); z <= 2; z++ {
		for x := int32(-6); x <= 6; x++ {
			setBlock(core.BlockPos{X: x, Y: 0, Z: z}, core.StoneID)
			setBlock(core.BlockPos{X: x, Y: 6, Z: z}, core.StoneID)
		}
		for y := int32(1); y <= 5; y++ {
			setBlock(core.BlockPos{X: -6, Y: y, Z: z}, core.StoneID)
			setBlock(core.BlockPos{X: 6, Y: y, Z: z}, core.StoneID)
		}
	}
	for y := int32(1); y <= 5; y++ {
		for x := int32(-6); x <= 6; x++ {
			setBlock(core.BlockPos{X: x, Y: y, Z: -16}, core.StoneID)
			setBlock(core.BlockPos{X: x, Y: y, Z: 2}, core.StoneID)
		}
	}
	// 落地一朵（支撑是正下方地板）＋左右墙各一朵：左墙 +X 形态（支撑在
	// 火把 −X 侧）、右墙 −X 形态（支撑在 +X 侧）。相机在 (0.5,2.8,0.5)
	// 朝 -Z 平视，三个形态全部在画面内。
	setBlock(core.BlockPos{X: 2, Y: 1, Z: -3}, core.TorchStandingID)
	setBlock(core.BlockPos{X: -5, Y: 2, Z: -5}, core.TorchWallPosXID)
	setBlock(core.BlockPos{X: 5, Y: 2, Z: -6}, core.TorchWallNegXID)
	return applyCaptureBlocks(app, blocks, captureWaterBasinChunkRadius, "火把夜景")
}

// prepareBedNightRoom 装入床夜景夹具：一间与火把夜景同壳的全封闭石室
// （地板 y=0、天花板 y=6、四壁合围，纵深 z=-16..2），室内按四个水平朝向
// 各摆一张完整床（床头与床尾成对、同框），光照只来自三朵火把的方块光。
// 封闭沿用火把夜景的同一理由：场景在夜晚（18000 tick）抓帧，室内亮度必须
// 只由固定光源解释，墙体留洞会让室外夜空混进画面、床面的明暗梯度就不再
// 只由火把解释。夹具经客户端只读镜像装入，不依赖任何服务端模拟推进；
// 床的双格都落在整片石地板上，与放置规则的支撑契约一致，不是悬空摆拍。
func prepareBedNightRoom(app SceneApplication) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}
	return applyCaptureBedNightChanges(app)
}

// applyCaptureBedNightChanges 写入石室外壳、四张床与三朵火把。四张床覆盖
// 全部四个水平朝向（南/北/东/西各一）：床面层的枕头/毯沿亮带随朝向旋转，
// 四个朝向同框即可互证「多朝向可辨」，超过 spec 要求的两种朝向以锁死
// 逐朝向的床面层差异。落地火把居中照亮四张床，左右墙各一朵补足边缘照度
// （床的床面材质取床顶上方格的光照，三朵把四张床上方的光照都抬到 10 级
// 上下），支撑格全部是实心石块。坐标由 capture_scene_test.go 的夹具测试
// 穷举锁定。
func applyCaptureBedNightChanges(app SceneApplication) error {
	blocks := make(map[core.ChunkPos]map[core.BlockPos]core.BlockID)
	setBlock := func(position core.BlockPos, block core.BlockID) {
		chunk := position.Chunk()
		if blocks[chunk] == nil {
			blocks[chunk] = make(map[core.BlockPos]core.BlockID)
		}
		blocks[chunk][position] = block
	}
	for z := int32(-16); z <= 2; z++ {
		for x := int32(-6); x <= 6; x++ {
			setBlock(core.BlockPos{X: x, Y: 0, Z: z}, core.StoneID)
			setBlock(core.BlockPos{X: x, Y: 6, Z: z}, core.StoneID)
		}
		for y := int32(1); y <= 5; y++ {
			setBlock(core.BlockPos{X: -6, Y: y, Z: z}, core.StoneID)
			setBlock(core.BlockPos{X: 6, Y: y, Z: z}, core.StoneID)
		}
	}
	for y := int32(1); y <= 5; y++ {
		for x := int32(-6); x <= 6; x++ {
			setBlock(core.BlockPos{X: x, Y: y, Z: -16}, core.StoneID)
			setBlock(core.BlockPos{X: x, Y: y, Z: 2}, core.StoneID)
		}
	}
	// 四张床：南向（床头 +Z）、北向（床头 −Z）、东向（床头 +X）、西向
	// （床头 −X），床尾格在前、床头格按 `core.BedHeadNeighbor` 成对。布局
	// 刻意聚在相机近处（z=-4 一排东西向、x=±5/4 两侧各一张南北向）：床面
	// 亮带在画面里只有几个像素厚，摆远了像素证据随之消失（详见场景注释）。
	setBlock(core.BlockPos{X: -3, Y: 1, Z: -4}, core.BedFootEastID)
	setBlock(core.BlockPos{X: -2, Y: 1, Z: -4}, core.BedHeadEastID)
	setBlock(core.BlockPos{X: 2, Y: 1, Z: -4}, core.BedFootWestID)
	setBlock(core.BlockPos{X: 1, Y: 1, Z: -4}, core.BedHeadWestID)
	setBlock(core.BlockPos{X: 4, Y: 1, Z: -6}, core.BedFootSouthID)
	setBlock(core.BlockPos{X: 4, Y: 1, Z: -5}, core.BedHeadSouthID)
	setBlock(core.BlockPos{X: -5, Y: 1, Z: -5}, core.BedFootNorthID)
	setBlock(core.BlockPos{X: -5, Y: 1, Z: -6}, core.BedHeadNorthID)
	// 三朵火把：落地一朵居中（支撑是正下方地板），左右墙各一朵墙面形态
	// （支撑在命中面反方向的墙内），把四张床上方的光照都抬到 10 级上下。
	setBlock(core.BlockPos{X: 0, Y: 1, Z: -7}, core.TorchStandingID)
	setBlock(core.BlockPos{X: -5, Y: 2, Z: -3}, core.TorchWallPosXID)
	setBlock(core.BlockPos{X: 5, Y: 2, Z: -5}, core.TorchWallNegXID)
	return applyCaptureBlocks(app, blocks, captureWaterBasinChunkRadius, "床夜景")
}

func applyCaptureBlockLightRoomChanges(app SceneApplication) error {
	blocks := make(map[core.ChunkPos]map[core.BlockPos]core.BlockID)
	setBlock := func(position core.BlockPos, block core.BlockID) {
		chunk := position.Chunk()
		if blocks[chunk] == nil {
			blocks[chunk] = make(map[core.BlockPos]core.BlockID)
		}
		blocks[chunk][position] = block
	}
	for z := int32(-10); z <= 2; z++ {
		for x := int32(-6); x <= 6; x++ {
			setBlock(core.BlockPos{X: x, Y: 0, Z: z}, core.StoneID)
			setBlock(core.BlockPos{X: x, Y: 6, Z: z}, core.StoneID)
		}
		for y := int32(1); y <= 5; y++ {
			setBlock(core.BlockPos{X: -6, Y: y, Z: z}, core.StoneID)
			setBlock(core.BlockPos{X: 6, Y: y, Z: z}, core.StoneID)
		}
	}
	for y := int32(1); y <= 5; y++ {
		for x := int32(-6); x <= 6; x++ {
			setBlock(core.BlockPos{X: x, Y: y, Z: -10}, core.StoneID)
			setBlock(core.BlockPos{X: x, Y: y, Z: 2}, core.StoneID)
		}
	}
	setBlock(core.BlockPos{X: 0, Y: 3, Z: -4}, core.LightBlockID)

	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			chunk := core.ChunkPos{X: x, Z: z}
			changes := make([]network.BlockChange, 0, len(blocks[chunk]))
			for position, block := range blocks[chunk] {
				changes = append(changes, network.BlockChange{
					Position: position,
					Block:    block,
				})
			}
			sort.Slice(changes, func(i, j int) bool {
				left, _ := world.ChunkBlockIndex(changes[i].Position)
				right, _ := world.ChunkBlockIndex(changes[j].Position)
				return left < right
			})
			if err := applyCaptureMirror(app, network.BlockChanges{
				Dimension:    core.Overworld,
				Chunk:        chunk,
				BaseRevision: 1,
				NewRevision:  2,
				Changes:      changes,
			}); err != nil {
				return fmt.Errorf("装入发光房间变化 (%d,%d): %w", x, z, err)
			}
		}
	}
	return nil
}

func prepareMaterialsShowcase(app SceneApplication) error {
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
				setBlock(core.BlockPos{X: x, Y: y, Z: -8}, block)
			}
		}
	}
	for y := int32(4); y <= 5; y++ {
		for x := int32(-10); x <= -9; x++ {
			setBlock(core.BlockPos{X: x, Y: y, Z: -9}, core.BrickID)
		}
	}
	for x := int32(-4); x <= 3; x++ {
		setBlock(core.BlockPos{X: x, Y: 0, Z: -1}, core.GrassID)
	}
	// 耕地两态列：与草地条同层（y=0）向左延伸，干（x=-6..-5）与湿（x=-8..-7）
	// 各一个 2 格宽、单层高的列。registry 给耕地填 block_top_raw=14，顶面因此
	// 下沉到 15/16——本场景把它纳入视觉回归网的意义所在。刻意放在相机近处的
	// 空地而不是远处 z=-8 的材料柱网格：相机在 (0.5,5.8,13.5) 以约 -0.12 rad
	// 俯视，z=-1 列的顶面俯角约 18°，1/16 格的下沉清晰可辨；z=-8 柱顶俯角
	// 不足 8°，同样的下沉会退化成亚像素噪声。该区域在既有夹具中全空，
	// 不移动、不覆盖任何既有方块。
	for x := int32(-8); x <= -7; x++ {
		setBlock(core.BlockPos{X: x, Y: 0, Z: -1}, core.FarmlandWetID)
	}
	for x := int32(-6); x <= -5; x++ {
		setBlock(core.BlockPos{X: x, Y: 0, Z: -1}, core.FarmlandDryID)
	}
	for z := int32(-2); z <= 0; z++ {
		for x := int32(0); x <= 3; x++ {
			setBlock(core.BlockPos{X: x, Y: 4, Z: z}, core.StoneID)
		}
	}
	for y := int32(1); y <= 3; y++ {
		setBlock(core.BlockPos{X: 7, Y: y, Z: -1}, core.OakLogID)
	}

	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			chunk := core.ChunkPos{X: x, Z: z}
			changes := make([]network.BlockChange, 0, len(blocks[chunk]))
			for position, block := range blocks[chunk] {
				changes = append(changes, network.BlockChange{Position: position, Block: block})
			}
			sort.Slice(changes, func(i, j int) bool {
				left, _ := world.ChunkBlockIndex(changes[i].Position)
				right, _ := world.ChunkBlockIndex(changes[j].Position)
				return left < right
			})
			if err := applyCaptureMirror(app, network.BlockChanges{
				Dimension: core.Overworld, Chunk: chunk,
				BaseRevision: 1, NewRevision: 2, Changes: changes,
			}); err != nil {
				return fmt.Errorf("装入材料展示变化 (%d,%d): %w", x, z, err)
			}
		}
	}
	return nil
}

func prepareTargetBlockFeedback(app SceneApplication) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}
	return applyCaptureMirror(app, network.BlockChanges{
		Dimension:    core.Overworld,
		Chunk:        core.ChunkPos{Z: -1},
		BaseRevision: 1,
		NewRevision:  2,
		Changes: []network.BlockChange{{
			Position: core.BlockPos{X: 0, Y: 3, Z: -3},
			Block:    core.BrickID,
		}},
	})
}

func applyCaptureMirror(app SceneApplication, message network.ServerMessage) error {
	update, err := app.Mirror().Apply(message)
	if err != nil {
		return err
	}
	app.Mesher().MarkDirty(update.Dirty...)
	return nil
}

// captureWaterBasinChunkRadius 是水景夹具覆盖的区块半径（以原点区块为中心）。
// 夹具在 x 上跨到 ±9、z 上跨到 −16，正好落在 3×3 的区块窗口里，与其余固定
// 场景（tunnel/room/showcase）用同一个窗口，便于共用空气快照与遍历。
const captureWaterBasinChunkRadius = 1

// prepareWaterBasin 装入水景夹具：一座石质水池，池底有可辨识的水下地形，
// 水体顶层沿 **+Z** 方向（即由池深处向岸边）从源方块递减到 7 级流动水，
// 形成一段连续的斜水面：z 越大等级越弱、`h_raw = 14 - level` 越低。
//
// 三件事同时被这一份夹具覆盖，与视觉门禁要求的三点对应：
//   - **水面斜坡**：顶层 y=4 沿 z 增大依次是 WaterSourceID → WaterLevel1..7ID
//     （见下方 `slope[z+10]`：z=−10 取 1 级、z=−4 取 7 级），角高度按
//     `h_raw = 14 - level` 插值，渲染出沿 +Z 连续下降的斜面而不是台阶。
//   - **水下视角**：水体足够深且四周留有余量，相机放进去就是眼睛浸没态。
//   - **水面之下的地形**：池底铺了沙丘、砾石带与一处露出水面的圆石堆，
//     三种材质在水下的可辨识度是「水下变暗但不立刻归零」的直接证据。
//
// 夹具全部走客户端只读镜像装入，不依赖任何流体模拟推进，因此与机器速度无关。
func prepareWaterBasin(app SceneApplication) error {
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
	// 池底与围墙。围墙比水面高两格，挡住背面漏光，让水下的暗度只由水深决定。
	for z := int32(-16); z <= 2; z++ {
		for x := int32(-9); x <= 9; x++ {
			setBlock(core.BlockPos{X: x, Y: 0, Z: z}, core.StoneID)
		}
		for y := int32(1); y <= 6; y++ {
			setBlock(core.BlockPos{X: -9, Y: y, Z: z}, core.StoneID)
			setBlock(core.BlockPos{X: 9, Y: y, Z: z}, core.StoneID)
		}
	}
	for y := int32(1); y <= 6; y++ {
		for x := int32(-9); x <= 9; x++ {
			setBlock(core.BlockPos{X: x, Y: y, Z: -16}, core.StoneID)
		}
	}
	// 水下地形：三块材质不同、高度不同的水下地形，用来看"水面之下的地形可见"。
	for z := int32(-14); z <= -11; z++ {
		for x := int32(-6); x <= -3; x++ {
			setBlock(core.BlockPos{X: x, Y: 1, Z: z}, core.SandID)
			setBlock(core.BlockPos{X: x, Y: 2, Z: z}, core.SandID)
		}
	}
	for z := int32(-13); z <= -6; z++ {
		for x := int32(1); x <= 6; x++ {
			setBlock(core.BlockPos{X: x, Y: 1, Z: z}, core.GravelID)
		}
	}
	// 圆石堆顶到 y=5，露出水面，给出"同一材质水上水下各是什么样"的对照。
	for z := int32(-8); z <= -7; z++ {
		for x := int32(-1); x <= 0; x++ {
			for y := int32(1); y <= 5; y++ {
				setBlock(core.BlockPos{X: x, Y: y, Z: z}, core.CobblestoneID)
			}
		}
	}
	// 岸：干地一侧铺草，让岸线在画面里可辨。
	for z := int32(-3); z <= 2; z++ {
		for x := int32(-8); x <= 8; x++ {
			setBlock(core.BlockPos{X: x, Y: 0, Z: z}, core.GrassID)
		}
	}
	// 水体。y=1..3 是满格水源；y=4 是顶层，沿 +Z（z 增大）递减出斜面。
	// 顺序在固体之后写，但只写尚未被固体占据的格——池底地形优先。
	slope := [...]core.BlockID{
		core.WaterLevel1ID, core.WaterLevel2ID, core.WaterLevel3ID, core.WaterLevel4ID,
		core.WaterLevel5ID, core.WaterLevel6ID, core.WaterLevel7ID,
	}
	for z := int32(-15); z <= -4; z++ {
		for x := int32(-8); x <= 8; x++ {
			for y := int32(1); y <= 4; y++ {
				position := core.BlockPos{X: x, Y: y, Z: z}
				if _, occupied := blocks[position.Chunk()][position]; occupied {
					continue
				}
				block := core.WaterSourceID
				if y == 4 && z >= -10 {
					block = slope[z+10]
				}
				setBlock(position, block)
			}
		}
	}
	return applyCaptureBlocks(app, blocks, captureWaterBasinChunkRadius, "水池")
}

// applyCaptureBlocks 把按区块分组的方块表逐区块装入客户端镜像。
// 每个区块内按 world.ChunkBlockIndex 排序后再发，保证同一份夹具在任何机器上
// 都产生逐字节相同的 BlockChanges——map 的遍历序不能进线上字节。
func applyCaptureBlocks(
	app SceneApplication,
	blocks map[core.ChunkPos]map[core.BlockPos]core.BlockID,
	chunkRadius int32,
	label string,
) error {
	for z := -chunkRadius; z <= chunkRadius; z++ {
		for x := -chunkRadius; x <= chunkRadius; x++ {
			chunk := core.ChunkPos{X: x, Z: z}
			changes := make([]network.BlockChange, 0, len(blocks[chunk]))
			for position, block := range blocks[chunk] {
				changes = append(changes, network.BlockChange{Position: position, Block: block})
			}
			sort.Slice(changes, func(i, j int) bool {
				left, _ := world.ChunkBlockIndex(changes[i].Position)
				right, _ := world.ChunkBlockIndex(changes[j].Position)
				return left < right
			})
			if err := applyCaptureMirror(app, network.BlockChanges{
				Dimension:    core.Overworld,
				Chunk:        chunk,
				BaseRevision: 1,
				NewRevision:  2,
				Changes:      changes,
			}); err != nil {
				return fmt.Errorf("装入%s变化 (%d,%d): %w", label, x, z, err)
			}
		}
	}
	return nil
}

// resetCapturePresentation 把共用同一个 application 的呈现状态清回空。
//
// 水景两个场景排在 ai-companion 之后，而那个场景会留下伙伴、聊天事件、打开的
// 聊天输入与格式化缓存——不显式清掉就会静默出现在水面上。清理项与
// applyAICompanionCaptureState 自己开头的那段一一对应：谁留下的谁负责被清掉，
// 但清理的落点必须在**后一个**场景，因为场景表没有 teardown 钩子。
func resetCapturePresentation(app SceneApplication) error {
	if app.RemotePlayers() == nil || app.Companions() == nil ||
		app.ChatEvents() == nil || app.ItemDrops() == nil {
		return fmt.Errorf("水景场景需要完整客户端呈现镜像")
	}
	app.RemotePlayers().Reset()
	app.Companions().Reset()
	app.ChatEvents().Reset()
	app.ChatInput().Cancel()
	app.SetChatEventBuffer([client.ChatEventCapacity]network.ChatEvent{})
	app.SetChatLines([6]string{})
	app.SetChatLineCount(0)
	app.SetFormattedChatEventID(0)
	app.ItemDrops().Reset()
	app.SetRemotePresentations(app.RemotePresentations()[:0])
	app.SetCompanionPresentations(app.CompanionPresentations()[:0])
	app.SetRemoteAvatars(app.RemoteAvatars()[:0])
	app.SetRemoteNameTags(app.RemoteNameTags()[:0])
	app.SetItemDropInstances(app.ItemDropInstances()[:0])
	// 夜行者镜像是场景夹具的一部分：hostile-mob 注入的 8 只个体（含受击与
	// 追逐状态）必须在这里一并恢复，否则水景会带着夜景敌怪出图。镜像可能
	// 为 nil（最小测试装配），nil 时无从谈起夹具残留，跳过即可。
	if app.Hostiles() != nil {
		app.Hostiles().Reset()
	}
	app.SetHostilePresentations(app.HostilePresentations()[:0])
	app.SetMiningOverlay(hud.MiningOverlay{})
	app.SetDamageFeedback(application.DamageFeedback{})
	app.SetDamageStrength(0)
	app.ResetCombatFeedback()
	app.Furnace().Reset()
	app.Chest().Reset()
	app.Crafting().Reset()
	app.SetInventoryOpen(false)
	app.SetInventorySource(-1)
	app.SetBlockTargetReset(false)
	if app.Panel() != nil {
		app.Panel().SetVisible(false)
	}
	return app.Inventory().Apply(network.InventoryState{Inventory: core.Inventory{}})
}

// captureUnderwaterEyePosition 是 water-underwater 场景注入的权威玩家位置
// （脚底）。选点要求：脚与眼所在格都是水，且四周离最近的非流体格有若干格余量。
//
// 余量是必需的：ApplyPlayerState 会重放尚未确认的历史输入，而历史长度取决于
// 服务端在抓帧期间确认到第几条，也就是取决于机器速度。位置选在水体正中，
// 重放几步造成的微小位移就不可能把浸没标志翻过去，画面因此仍然确定。
var captureUnderwaterEyePosition = mgl32.Vec3{4.5, 1.2, -4.5}

// captureUnderwaterOxygen 是 water-underwater 场景注入的权威氧气值。
// 取满值（core.MaxOxygenTicks = 300）的一半上下：氧气呈现已迁 WebView HUD
// 组件，无头画面不再画氧气条，这个值只经 HUD 状态下行验证耗损值可注入，
// 满值反而测不出「非满氧气」这条路径。
const captureUnderwaterOxygen = 160

// applyWaterUnderwaterCaptureState 把相机与权威玩家状态一起放进水里。
//
// 水下视觉读的是 Predictor.EyeInFluid()，即权威/预测共用的那一个浸没标志，
// 不是相机坐标——规格要求视觉与溺水判定 MUST NOT 存在第二套判定。所以这里
// 必须注入一条权威 PlayerState 让预测器按镜像重算标志，只摆相机是不够的。
//
// 注入走非 Reset 分支：Reset 会命中 Predictor.Begin，而 Begin 手上没有方块视图，
// 按其文档把标志置为 false，画面就会变成"人在水里但视觉没入水"。
func applyWaterUnderwaterCaptureState(app SceneApplication) error {
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(6000)
	// ServerTick 取一个远大于抓帧期间真实 tick 的常量：ApplyPlayerState 有
	// 单调校验，小于等于已收到的 tick 会被静默忽略，而真实 tick 取决于加载
	// 花了多久，不能拿来做常量。
	if _, err := app.Predictor().ApplyPlayerState(network.PlayerState{
		ServerTick:     1 << 20,
		Dimension:      core.Overworld,
		Position:       captureUnderwaterEyePosition,
		Yaw:            0,
		Pitch:          -0.05,
		Ready:          true,
		Health:         core.MaxHealth,
		Oxygen:         captureUnderwaterOxygen,
		WorldTimeTicks: 6000,
	}, client.MirrorCollisionSource{
		Mirror:    app.Mirror(),
		Dimension: core.Overworld,
	}); err != nil {
		return fmt.Errorf("注入水下权威玩家状态: %w", err)
	}
	if !app.Predictor().EyeInFluid() {
		return fmt.Errorf(
			"water-underwater 的相机没有落在水里：位置 %v 处 EyeInFluid=false",
			captureUnderwaterEyePosition)
	}
	app.Camera().Pos = captureUnderwaterEyePosition.Add(
		mgl32.Vec3{0, physics.ActiveTunables().EyeHeight, 0})
	app.Camera().Yaw = 0
	app.Camera().Pitch = -0.05
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	return nil
}

// applyFarHorizonCaptureState 钉死 far-horizon 场景的全部呈现状态:前序场景
// (ai-companion 及水景)留下的共享状态经 resetCapturePresentation 统一清空
// (变基前该函数自带一份清理清单,fluid 系列把同样的清单沉淀成了公共 helper,
// 变基后直接复用,不维护第二份)。几何依据:近环以出生 chunk (0,0) 为中心
// (视距 32 chunk → 近 mesh 覆盖 block [-512, 528]²),远环带内半径 9 tile
// (Ruling 19,floor(32/4)+1)→ 朝 -z 方向壳从 block -512 起(tile -9 覆盖
// [-576,-512)),与近 mesh 边缘零缝衔接;相机 (8, 110, -352) 距近环 -z 边缘
// 与壳起点都是 160 block,距全雾线(1152)与环外缘(1184)均留余量,画面上
// 天空、雾过渡带、壳带、近景四段齐备。相机 y=110 保持在壳上界(112)之下,
// 与近处不变断言的截止推导自洽。a.center 与 lodTileCenter 刻意不动:场景
// 不得触发近环 DropOutside 或远环增量入队,收敛域与 terrain-noon 同源。
func applyFarHorizonCaptureState(app SceneApplication) error {
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(6000)
	*app.Camera() = client.Camera{
		Pos: mgl32.Vec3{8, 110, -352}, Yaw: 0, Pitch: -0.25,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.SetBlockTargetReset(false)
	return nil
}
