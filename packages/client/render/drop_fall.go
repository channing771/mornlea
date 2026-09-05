package render

import (
	"math"
	"slices"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// dropFallTickSeconds 是呈现下落的 tick 步长（秒）：与权威 50ms ticker
	// 同值，`g`/终端速度由调用方显式传参（生产取生效 tunables）。
	dropFallTickSeconds = float32(0.05)
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
	ordered    [maxItemDrops]ItemDrop
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

// dropFallOffset 是下落偏移的纯函数：从静止起按半隐式欧拉积分（每 tick
// `v += g·dt`、终端钳制、`y -= v·dt`，与角色重力同形），再按
// `min（积分下落，生成高度−支撑高度）`着陆钳制。其中生成高度与支撑高度都取
// 方块坐标系（掉落方块 Y、支撑顶面 Y）：贴地掉落（生成方块紧贴支撑顶面）上限
// 为零，恒零偏移。上钳着陆面、下钳零（支撑高于生成时不反向顶起）；无支撑信息
// （镜像无数据或支撑超深）时返回零，按生成高度保持。`g`/终端由调用方显式传参，
// 本包不读全局 tunables；闭式解逐帧有界，不随年龄增长做无界循环。
func dropFallOffset(age uint64, spawnY, supportTopY float32, hasSupport bool, gravity, terminal float32) float32 {
	if !hasSupport {
		return 0
	}
	if max := spawnY - supportTopY; max <= 0 {
		return 0
	} else if gravity <= 0 || terminal <= 0 {
		return 0
	} else {
		step := gravity * dropFallTickSeconds
		reach := uint64(math.Ceil(float64(terminal / step)))
		if reach < 1 {
			reach = 1
		}
		var fallen float32
		if age < reach {
			fallen = step * dropFallTickSeconds * float32(age) * float32(age+1) / 2
		} else {
			fallen = step*dropFallTickSeconds*float32(reach-1)*float32(reach)/2 +
				float32(age-reach+1)*terminal*dropFallTickSeconds
		}
		if fallen > max {
			fallen = max
		}
		if fallen < 0 {
			fallen = 0
		}
		return fallen
	}
}

// buildItemDropParts 把掉落物编码为贴图实例：输入复制到固定 scratch，按完整
// 同格键与 `DropID` 排序后线性分组。每堆独占 XZ cell；死亡前隐藏堆仍占槽，
// 首次可见时才登记年龄。中心从支撑顶面按实际半高和非负浮动锚定，下落、自转、
// 渐显与白闪保持原语义。生产输入已由镜像拒绝非法值与真实 overflow；此处仍
// 防御性限制固定实例容量，不扩大 CPU/GPU 缓冲。
func (falls *DropFalls) buildItemDropParts(dst []avatarPart, serverTick uint64, drops []ItemDrop, gravity, terminal float32) []avatarPart {
	count := min(len(drops), maxItemDrops)
	ordered := falls.ordered[:count]
	copy(ordered, drops[:count])
	slices.SortFunc(ordered, compareDropScatterInputs)

	for groupStart := 0; groupStart < len(ordered); {
		groupEnd := groupStart + 1
		for groupEnd < len(ordered) && sameDropScatterGroup(ordered[groupStart], ordered[groupEnd]) {
			groupEnd++
		}
		groupCount := groupEnd - groupStart
		for rank, drop := range ordered[groupStart:groupEnd] {
			if len(dst) == maxItemDrops {
				return dst
			}
			material, ok := itemDropMaterial(drop.Item)
			if !ok {
				continue
			}
			unit := float32(1)
			color := [4]float32{1, 1, 1, 1}
			if drop.DeathTick != 0 {
				visible, linkedScale, flash := deathLinkedDropAppearance(serverTick, drop.DeathTick)
				if !visible {
					continue
				}
				unit = linkedScale
				if flash {
					color = [4]float32{2, 2, 2, 1}
				}
			}
			age := falls.age(serverTick, drop.ID)
			placement := dropScatterPlacementFor(groupCount, rank, drop.ID)
			actualScale := placement.scale * unit
			sx, sy, sz := dropCubeSize*actualScale, dropCubeSize*actualScale, dropCubeSize*actualScale
			if itemDropFlake(drop.Item) {
				sx, sy, sz = dropFlakeSize*actualScale, dropFlakeSize*actualScale, dropFlakeThin*actualScale
			}
			phase := dropAnimationPhase(serverTick, drop.ID)
			center := mgl32.Vec3{
				float32(drop.Block.X) + placement.x,
				dropScatterCenterY(
					dropScatterBaseY(drop, age, gravity, terminal), sy/2, placement.layerRise,
					placement.bob*unit, phase.float,
				),
				float32(drop.Block.Z) + placement.z,
			}
			transform := mgl32.Translate3D(center.X(), center.Y(), center.Z()).
				Mul4(mgl32.HomogRotate3DY(phase.spin)).
				Mul4(mgl32.Scale3D(sx, sy, sz))
			// 贴图分支颜色与漫反射相乘（见 avatar 着色器）：中性白保持原样，
			// 超白即一次白色闪光；填色保持编码确定。
			dst = append(dst, avatarPart{transform: transform, color: color, material: material})
		}
		groupStart = groupEnd
	}
	return dst
}
