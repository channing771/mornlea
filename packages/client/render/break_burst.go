package render

import (
	"math"
	"slices"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// breakBurstParticlesPerBurst 是一个 burst 的固定粒子数。
	breakBurstParticlesPerBurst = 8
	// breakBurstLifetimeTicks 是 burst 的固定寿命：年龄达到该 tick 数即停止编码。
	breakBurstLifetimeTicks = uint64(20)
	// breakBurstMaxBursts 是跟踪表的固定容量：只留最近的该数量掉落物 ID。
	breakBurstMaxBursts = 16
	// breakBurstMaxInstances 是单帧编码的粒子总数上限。
	breakBurstMaxInstances = 64
	// breakBurstCubeSize 是粒子首帧边长，随年龄线性收缩到零。
	breakBurstCubeSize = float32(0.09)
	// breakBurstGravity 是纵向重力系数（方块/tick²），位置公式只在 Y 轴扣除。
	breakBurstGravity = float32(0.012)
	// breakBurstMaxTombstones 是淘汰抑制集合的固定容量：被挤出跟踪表但仍存续
	// 的掉落物 ID 在此留名，不再逐帧重 burst；掉落物消失即销名。
	breakBurstMaxTombstones = 16
	// breakBurstSalt 是 burst 散列的固定盐：把 ID 混合域与掉落物动画相位区分开。
	breakBurstSalt = uint32(0x9E3779B9)
)

// breakBurstEntry 是一个掉落物 burst 的跟踪条目：首次 tick 钉住年龄零点，
// 之后位置与尺寸都由该零点与当前 tick 纯推导，不存任何逐帧状态。
type breakBurstEntry struct {
	id        core.DropID
	firstTick uint64
	origin    mgl32.Vec3
	color     [4]float32
}

// BreakBursts 持有破碎 burst 的跨帧跟踪表：`ID→首次 tick` 的定界映射，另有
// 定界的淘汰抑制集合与输入规范化复用缓冲。调用方每帧以与 `buildItemDropParts`
// 同样的输入（serverTick + drops）调用 `BuildParts`；掉落物消失即删条目并销
// 抑制记号，表满时环形淘汰最老的条目（被淘汰但仍存续的 ID 留名抑制集合）。
type BreakBursts struct {
	entries []breakBurstEntry
	// tombstones 是被挤出跟踪表但仍存续的掉落物 ID：表满时仍在场的 ID 不再视
	// 为新 ID，避免每帧重 burst 的淘汰抖动；容量定界 16，掉落物消失即销名。
	tombstones []core.DropID
	// scratch 是输入规范化的复用缓冲：每帧把 drops 拷入并按 ID 全序排列，
	// 同集合乱序输入得到逐字节一致的编码，稳态零分配。
	scratch []ItemDrop
}

// Reset 清空跨帧跟踪表与淘汰抑制集合：会话重置后旧 ID 不得抑制新会话的首现，
// 旧首次 tick 也不得带入新会话（tick 回退钳制只兜底同会话内的抖动）。
func (bursts *BreakBursts) Reset() {
	bursts.entries = bursts.entries[:0]
	bursts.tombstones = bursts.tombstones[:0]
}

// BuildParts 更新跟踪表并把存活 burst 编码为 avatar pass 的实心小立方体：
// 新 ID 建条目、消失 ID 删条目、年龄达到寿命停止编码、总数超限时淘汰最老的
// burst，有支撑时粒子纵向以输入携带的支撑高度为地板钳制。输入先按 `DropID`
// 全序规范化再建条目，编码上限内按该顺序输出，同集合乱序输入逐帧一致。
func (bursts *BreakBursts) BuildParts(dst []avatarPart, serverTick uint64, drops []ItemDrop) []avatarPart {
	kept := bursts.entries[:0]
	for _, entry := range bursts.entries {
		if hasBreakBurstDrop(drops, entry.id) {
			kept = append(kept, entry)
		}
	}
	bursts.entries = kept
	remembered := bursts.tombstones[:0]
	for _, id := range bursts.tombstones {
		if hasBreakBurstDrop(drops, id) {
			remembered = append(remembered, id)
		}
	}
	bursts.tombstones = remembered
	ordered := append(bursts.scratch[:0], drops...)
	bursts.scratch = ordered
	slices.SortFunc(ordered, func(left, right ItemDrop) int { return left.ID.Compare(right.ID) })
	for _, drop := range ordered {
		if bursts.find(drop.ID) >= 0 || bursts.suppressed(drop.ID) {
			continue
		}
		color, ok := itemDropColor(drop.Item)
		if !ok {
			continue
		}
		if len(bursts.entries) == breakBurstMaxBursts {
			evicted := bursts.entries[0]
			copy(bursts.entries, bursts.entries[1:])
			bursts.entries = bursts.entries[:len(bursts.entries)-1]
			// 被挤掉的条目刚经存续剪枝，ID 必仍在场：留名抑制集合，
			// 不让它下帧被误判为新 ID 而逐帧重 burst。
			if len(bursts.tombstones) == breakBurstMaxTombstones {
				copy(bursts.tombstones, bursts.tombstones[1:])
				bursts.tombstones = bursts.tombstones[:len(bursts.tombstones)-1]
			}
			bursts.tombstones = append(bursts.tombstones, evicted.id)
		}
		bursts.entries = append(bursts.entries, breakBurstEntry{
			id:        drop.ID,
			firstTick: serverTick,
			origin: mgl32.Vec3{
				float32(drop.Block.X) + 0.5,
				float32(drop.Block.Y) + 0.5,
				float32(drop.Block.Z) + 0.5,
			},
			color: color,
		})
	}
	alive := 0
	for _, entry := range bursts.entries {
		if breakBurstAge(serverTick, entry.firstTick) < breakBurstLifetimeTicks {
			alive++
		}
	}
	skip := alive - breakBurstMaxInstances/breakBurstParticlesPerBurst
	for _, entry := range bursts.entries {
		age := breakBurstAge(serverTick, entry.firstTick)
		if age >= breakBurstLifetimeTicks {
			continue
		}
		if skip > 0 {
			skip--
			continue
		}
		elapsed := float32(age)
		size := breakBurstCubeSize * (1 - elapsed/float32(breakBurstLifetimeTicks))
		supportY, hasSupport := breakBurstSupport(ordered, entry.id)
		for index := range breakBurstParticlesPerBurst {
			velocity := breakBurstVelocity(entry.id, index)
			center := entry.origin.Add(velocity.Mul(elapsed))
			center[1] -= breakBurstGravity * elapsed * elapsed
			// 粒子以同一支撑高度为地板钳制（防穿地）：burst 原点仍是破坏方块
			// 位置，不动；地板取支撑顶面加当前半边长，立方体整体留在地上。
			if hasSupport {
				if floor := supportY + size/2; center[1] < floor {
					center[1] = floor
				}
			}
			transform := mgl32.Translate3D(center.X(), center.Y(), center.Z()).
				Mul4(mgl32.Scale3D(size, size, size))
			dst = append(dst, avatarPart{transform: transform, color: entry.color, material: avatarMaterialSolid})
		}
	}
	return dst
}

func (bursts *BreakBursts) find(id core.DropID) int {
	for index, entry := range bursts.entries {
		if entry.id == id {
			return index
		}
	}
	return -1
}

func (bursts *BreakBursts) suppressed(id core.DropID) bool {
	for _, tomb := range bursts.tombstones {
		if tomb == id {
			return true
		}
	}
	return false
}

func hasBreakBurstDrop(drops []ItemDrop, id core.DropID) bool {
	for _, drop := range drops {
		if drop.ID == id {
			return true
		}
	}
	return false
}

// breakBurstSupport 取本帧输入中该 ID 的支撑顶面：支撑随镜像加载逐帧变化，
// 故每次从本帧输入现查，不存入条目；线性扫描定界（条目 ≤16，输入 ≤800），
// 与本文件既有的 `find`/`hasBreakBurstDrop` 同量级。
func breakBurstSupport(drops []ItemDrop, id core.DropID) (float32, bool) {
	for _, drop := range drops {
		if drop.ID == id {
			return drop.SupportY, drop.HasSupport
		}
	}
	return 0, false
}

// breakBurstAge 返回 burst 年龄：tick 回退时钳制为零，不下溢。钳制只兜底同
// 会话内的 tick 抖动；会话重置由调用方经 `Reset` 清表，不依赖钳制。
func breakBurstAge(serverTick, firstTick uint64) uint64 {
	if serverTick < firstTick {
		return 0
	}
	return serverTick - firstTick
}

// breakBurstHash 把掉落物稳定 ID 折叠为 u32 散列：沿用掉落物动画相位的小整数
// 乘加混合习惯，再经固定盐与两次异或雪崩区分用途域。`DropID.Dimension` 参与
// 混合（`BlockPos` 本身不携带维度：同帧输入来自同一维度镜像，跨维度同槽位复
// 用的 ID 初速也不重合）；`Chunk` 只有 XZ 两轴，与动画相位取同一分量。
func breakBurstHash(id core.DropID) uint32 {
	hash := uint32(id.Generation)*31 + uint32(id.Slot)*7 +
		uint32(id.Chunk.X)*3 + uint32(id.Chunk.Z) +
		uint32(id.Dimension)*13 + breakBurstSalt
	hash ^= hash >> 16
	hash *= 16777619
	hash ^= hash >> 13
	return hash
}

// breakBurstVelocity 由掉落物 ID 散列派生第 index 粒的初速：8 粒方位角均分整圆
// （散列只给整体旋转），纵向分量恒为负即下半球向外下坠，径向与纵向的档位取自散列位。
func breakBurstVelocity(id core.DropID, index int) mgl32.Vec3 {
	hash := breakBurstHash(id)
	azimuth := (float32(index) + float32(hash&7)/8) * (math.Pi / 4)
	spread := (hash >> (4 * uint(index))) & 15
	radius := 0.05 + 0.01*float32(spread&3)
	down := -(0.16 + 0.03*float32(spread>>2))
	return mgl32.Vec3{
		radius * float32(math.Cos(float64(azimuth))),
		down,
		radius * float32(math.Sin(float64(azimuth))),
	}
}
