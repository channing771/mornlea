package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// dropFallBlocksPerTick 是掉落物呈现下落的恒定速率（方块/tick）：3 格
	// 落差恰好 20 tick 着陆。呈现下落不反馈服务端，权威位置、拾取与存档一律不动。
	dropFallBlocksPerTick = float32(0.15)
	// dropFallMaxTracked 是下落首现表的固定容量：只留最近的该数量掉落物 ID，
	// 满表后环形淘汰最老的 ID。
	dropFallMaxTracked = 128
)

// DropFalls 持有掉落物下落的首现 tick 跟踪表：`ID→首次 tick` 的定界映射。
// 调用方每帧以与掉落物同样的输入（serverTick + drops）调用
// `buildItemDropParts`；掉落物消失即删条目，表满时环形淘汰最老的条目。
// 被淘汰但仍存续的 ID 下次出现视为新掉落、年龄重起（呈现上是掉落跳回生成
// 高度再落一次：128 容量远超单画面常规掉落数，只在极端堆积时触发，
// burst 侧另有淘汰抑制集合兜底闪烁，这里只记行为不另起集合）。
type DropFalls struct {
	ids        []core.DropID
	firstTicks []uint64
}

// Reset 清空首现表：会话重置后旧首次 tick 不得带入新会话（tick 回退钳制
// 只兜底同会话内的抖动）。
func (falls *DropFalls) Reset() {
	falls.ids = falls.ids[:0]
	falls.firstTicks = falls.firstTicks[:0]
}

// age 返回掉落物年龄并注册首现：新 ID 以当前 tick 建条目（满表挤掉最老），
// 存续 ID 返回 tick 差；tick 回退时钳制为零，不下溢。
func (falls *DropFalls) age(serverTick uint64, id core.DropID) uint64 {
	for index, known := range falls.ids {
		if known == id {
			if serverTick < falls.firstTicks[index] {
				return 0
			}
			return serverTick - falls.firstTicks[index]
		}
	}
	if len(falls.ids) == dropFallMaxTracked {
		copy(falls.ids, falls.ids[1:])
		falls.ids = falls.ids[:len(falls.ids)-1]
		copy(falls.firstTicks, falls.firstTicks[1:])
		falls.firstTicks = falls.firstTicks[:len(falls.firstTicks)-1]
	}
	falls.ids = append(falls.ids, id)
	falls.firstTicks = append(falls.firstTicks, serverTick)
	return 0
}

// dropFallOffset 是下落偏移的纯函数：`fallen = min（年龄×速率，生成高度−
// 支撑高度）`，其中生成高度与支撑高度都取方块坐标系（掉落方块 Y、支撑顶面
// Y）：贴地掉落（生成方块紧贴支撑顶面）上限为零，恒零偏移。上钳着陆面、
// 下钳零（支撑高于生成时不反向顶起）；无支撑信息（镜像无数据或支撑超深）
// 时返回零，按生成高度保持。
func dropFallOffset(age uint64, spawnY, supportTopY float32, hasSupport bool) float32 {
	if !hasSupport {
		return 0
	}
	fallen := float32(age) * dropFallBlocksPerTick
	if max := spawnY - supportTopY; fallen > max {
		fallen = max
	}
	if fallen < 0 {
		fallen = 0
	}
	return fallen
}

// buildItemDropParts 把掉落物编码为实心小立方体：颜色与未知物品规则沿用
// 既有语义，中心高度 = 生成基准 − 下落偏移；正弦浮动与自转在着陆后继续，
// 输出恒不超过固定容量。年龄来自首现表，同 tick 重复编码逐字节一致。
func (falls *DropFalls) buildItemDropParts(dst []avatarPart, serverTick uint64, drops []ItemDrop) []avatarPart {
	for _, drop := range drops {
		if len(dst) == maxItemDrops {
			break
		}
		color, ok := itemDropColor(drop.Item)
		if !ok {
			continue
		}
		phase := dropAnimationPhase(serverTick, drop.ID)
		spawnBaseY := float32(drop.Block.Y) + dropBaseAltitude
		center := mgl32.Vec3{
			float32(drop.Block.X) + 0.5,
			spawnBaseY - dropFallOffset(falls.age(serverTick, drop.ID), float32(drop.Block.Y), drop.SupportY, drop.HasSupport) +
				dropFloatHeight*float32(math.Sin(float64(phase.float))),
			float32(drop.Block.Z) + 0.5,
		}
		transform := mgl32.Translate3D(center.X(), center.Y(), center.Z()).
			Mul4(mgl32.HomogRotate3DY(phase.spin)).
			Mul4(mgl32.Scale3D(dropCubeSize, dropCubeSize, dropCubeSize))
		dst = append(dst, avatarPart{transform: transform, color: color, material: avatarMaterialSolid})
	}
	return dst
}
