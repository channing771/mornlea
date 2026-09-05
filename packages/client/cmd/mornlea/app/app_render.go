//go:build darwin

package app

import (
	"fmt"
	"math"
	"slices"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
)

func RemoteRenderPresentations(presentations []client.RemotePresentation) ([]render.Avatar, []render.NameTag) {
	ordered := append([]client.RemotePresentation(nil), presentations...)
	slices.SortFunc(ordered, func(left, right client.RemotePresentation) int {
		return slices.Compare(left.PlayerID[:], right.PlayerID[:])
	})
	return RemoteRenderPresentationsSortedInto(
		make([]render.Avatar, 0, len(ordered)),
		make([]render.NameTag, 0, MaxFrameNameTags),
		ordered,
	)
}

func (a *Application) appendCurrentBlockTarget(
	tags []render.NameTag,
) ([]render.NameTag, render.BlockOutline) {
	target, ok := a.CurrentBlockTarget()
	if !ok {
		return tags, render.BlockOutline{}
	}
	tags = append(tags, render.NameTag{
		Key:  render.EntityKey{Kind: render.EntityTarget},
		Text: target.Name,
		Anchor: mgl32.Vec3{
			float32(target.Position.X) + 0.5,
			float32(target.Position.Y) + 1.15,
			float32(target.Position.Z) + 0.5,
		},
	})
	return tags, render.BlockOutline{Visible: true, Position: target.Position}
}

// deriveBlockCrack 从本帧选框与最后确认的权威采掘镜像派生裂纹呈现输入。
// 与选框同源门控：轮廓不可见（reset 当帧、断线，以及 `appendCurrentBlockTarget`
// 内部的背包/容器/面板与无命中条件）时裂纹一并隐藏，可见性再要求权威采掘
// active 且携带有效目标、进度可映射为合法阶段。游戏相位门控兜底同帧隐藏：
// 暂停相位权威已冻结而采掘镜像尚在；主菜单/设置相位全景接管底图，而全景
// 相位（`vista != nil`）必然不是游戏相位，同一判定覆盖全部菜单形态，
// 不另起一套相位口径。
func (a *Application) deriveBlockCrack(outline render.BlockOutline) render.BlockCrack {
	if !outline.Visible || !a.miningOverlay.Active || !a.miningOverlay.HasTarget ||
		a.menu.phase != MenuPhaseGame {
		return render.BlockCrack{}
	}
	stage := render.BlockCrackStage(a.miningOverlay.ProgressTicks, a.miningOverlay.RequiredTicks)
	if stage == render.BlockCrackStageNone {
		return render.BlockCrack{}
	}
	return render.BlockCrack{Visible: true, Position: a.miningOverlay.Target, Stage: stage}
}

func RemoteRenderPresentationsSortedInto(
	avatars []render.Avatar,
	tags []render.NameTag,
	ordered []client.RemotePresentation,
) ([]render.Avatar, []render.NameTag) {
	for _, presentation := range ordered {
		key := render.EntityKey{Kind: render.EntityPlayer, ID: [16]byte(presentation.PlayerID)}
		avatars = append(avatars, render.Avatar{
			Key: key, Position: presentation.Position,
			Yaw: presentation.Yaw, Pitch: presentation.Pitch,
		})
		tags = append(tags, render.NameTag{
			Key:    key,
			Text:   presentation.DisplayName,
			Anchor: presentation.Position.Add(mgl32.Vec3{0, 2.05, 0}),
		})
	}
	return avatars, tags
}

func AppendCompanionRenderPresentationsInto(
	avatars []render.Avatar,
	tags []render.NameTag,
	presentations []client.CompanionPresentation,
) ([]render.Avatar, []render.NameTag) {
	for _, presentation := range presentations {
		key := render.EntityKey{Kind: render.EntityCompanion, ID: [16]byte(presentation.ID)}
		avatars = append(avatars, render.Avatar{
			Key: key, Position: presentation.Position,
			Yaw: presentation.Yaw, Pitch: presentation.Pitch,
		})
		tags = append(tags, render.NameTag{
			Key: key, Text: presentation.Name,
			Anchor: presentation.Position.Add(mgl32.Vec3{0, 2.05, 0}),
		})
	}
	return avatars, tags
}

// AppendHostileRenderPresentationsInto 把夜行者镜像转换为 avatar 记录：
// 夜行者只进入实体通道，绝不进入名称标签集合（名标容量不随敌怪数量变化）。
func AppendHostileRenderPresentationsInto(
	avatars []render.Avatar,
	presentations []client.HostilePresentation,
) []render.Avatar {
	for _, presentation := range presentations {
		avatars = append(avatars, render.Avatar{
			Key:      render.HostileEntityKey(presentation.ID),
			Position: presentation.Position,
			Yaw:      presentation.Yaw,
		})
	}
	return avatars
}

// AppendPassiveRenderPresentationsInto 把被动牛镜像转换为 avatar 记录：
// 被动牛只进入实体通道，绝不进入名称标签集合（名标容量不随牛群数量变化）；
// 牛头俯仰由放牧位经 `render.PassiveGrazeHeadPitch` 直通（置位下压、清位时按
// 权威 tick 叠闲时点头），死亡保留体的侧倒与红闪由 `render.PassiveDeathPhase`
// 按死亡 tick 与当前权威 tick 派生；位姿完全由权威镜像驱动，本函数不做任何
// 墙钟推测，点头相位只读权威 tick 与牛 ID。
func AppendPassiveRenderPresentationsInto(
	avatars []render.Avatar,
	presentations []client.PassivePresentation,
	nowTick uint64,
) []render.Avatar {
	for _, presentation := range presentations {
		pitch := render.PassiveGrazeHeadPitch(presentation.Grazing)
		if !presentation.Grazing && !presentation.Dying {
			pitch = render.PassiveIdleNodPitch(nowTick, presentation.ID)
		}
		avatar := render.Avatar{
			Key:      render.PassiveEntityKey(presentation.ID),
			Position: presentation.Position,
			Yaw:      presentation.Yaw,
			Pitch:    pitch,
		}
		if presentation.Dying {
			avatar.Roll, avatar.Flash = render.PassiveDeathPhase(presentation.DeathTick, presentation.ID, nowTick)
		}
		avatars = append(avatars, avatar)
	}
	return avatars
}

func validateEntityPresentationCounts(avatars []render.Avatar, tags []render.NameTag) error {
	if len(avatars) > maxFrameAvatars {
		return fmt.Errorf("avatar count %d exceeds %d", len(avatars), maxFrameAvatars)
	}
	if len(tags) > MaxFrameNameTags {
		return fmt.Errorf("name tag count %d exceeds %d", len(tags), MaxFrameNameTags)
	}
	return nil
}

func (a *Application) FramebufferLabel() string {
	width, height := a.FramebufferSize()
	return fmt.Sprintf("%dx%d", width, height)
}

func (a *Application) FramebufferSize() (int, int) {
	if a.window != nil {
		return a.window.FramebufferSize()
	}
	return a.frameWidth, a.frameHeight
}

// cameraSectionPos 返回相机所在 section(Y 槽位钳制),复刻旧渲染器的
// cameraSection 语义,供可见性 BFS 起点使用。
func cameraSectionPos(pos mgl32.Vec3) core.SectionPos {
	block := core.BlockPos{
		X: int32(math.Floor(float64(pos[0]))),
		Y: int32(math.Floor(float64(pos[1]))),
		Z: int32(math.Floor(float64(pos[2]))),
	}
	y := int32(block.SectionIndex())
	if y < 0 {
		y = 0
	} else if y >= core.SectionsPerChunk {
		y = core.SectionsPerChunk - 1
	}
	return core.SectionPos{X: block.Chunk().X, Y: y, Z: block.Chunk().Z}
}

func CameraChunk(pos mgl32.Vec3) core.ChunkPos {
	return core.BlockPos{
		X: int32(math.Floor(float64(pos.X()))),
		Z: int32(math.Floor(float64(pos.Z()))),
	}.Chunk()
}

// appendItemDropInstances 把只读镜像转换为渲染实例，复用调用方切片。死亡
// 保留期内的掉落按（死亡牛位置邻域 2 格 + upsert tick 落在 [deathTick,
// deathTick+20] 窗内）关联死亡：关联只影响呈现（50% 前隐藏、后 scale-in +
// 白闪，见 `render` 掉落 pass），拾取走权威不受影响。密集击杀下多个死亡同
// 时命中一格时取最近者、再取小 ID——任取其一亦可接受，测试只锁确定性。
func appendItemDropInstances(
	dst []render.ItemDrop,
	drops []client.ItemDropPresentation,
	deaths []client.PassivePresentation,
) []render.ItemDrop {
	for _, drop := range drops {
		block, ok := render.ItemDropBlock(drop.ID.Chunk, drop.BlockIndex)
		if !ok {
			continue
		}
		dst = append(dst, render.ItemDrop{
			ID: drop.ID, Block: block, Item: drop.Item,
			DeathTick: linkDeathTick(block, drop.UpsertTick, deaths),
		})
	}
	return dst
}

// linkDeathTick 在死亡保留体中找与掉落关联的死亡 tick：切比雪夫邻域 2 格且
// upsert tick 落在保留窗内；无命中返回 0（不关联）。
func linkDeathTick(
	block core.BlockPos,
	upsertTick uint64,
	deaths []client.PassivePresentation,
) uint64 {
	var (
		bestTick uint64
		bestDist int32 = 3
		bestID   uint64
		matched  bool
	)
	for _, death := range deaths {
		if !death.Dying {
			continue
		}
		if upsertTick < death.DeathTick || upsertTick-death.DeathTick > render.PassiveDeathTicks {
			continue
		}
		grave := core.BlockPos{
			X: int32(math.Floor(float64(death.Position.X()))),
			Y: int32(math.Floor(float64(death.Position.Y()))),
			Z: int32(math.Floor(float64(death.Position.Z()))),
		}
		dist := chebyshevBlockDistance(block, grave)
		if dist > 2 {
			continue
		}
		if !matched || dist < bestDist || (dist == bestDist && death.ID < bestID) {
			bestTick, bestDist, bestID, matched = death.DeathTick, dist, death.ID, true
		}
	}
	if !matched {
		return 0
	}
	return bestTick
}

// chebyshevBlockDistance 返回两方块的切比雪夫距离（格）。
func chebyshevBlockDistance(left, right core.BlockPos) int32 {
	dx := left.X - right.X
	if dx < 0 {
		dx = -dx
	}
	dy := left.Y - right.Y
	if dy < 0 {
		dy = -dy
	}
	dz := left.Z - right.Z
	if dz < 0 {
		dz = -dz
	}
	return max(dx, max(dy, dz))
}
