package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

const (
	// SnowKickParticles 是单次踢雪事件的固定粒子数（design 总表「疾跑/落地
	// 踢雪尘 ≤8 粒/帧」）：一次事件恰产出该数量的雪尘，与降水粒子共享同一条
	// 实例流与 `WeatherMaxParticles` 预算。
	SnowKickParticles = 8
	// snowKickLifetimeTicks 是踢雪事件的固定寿命（权威 tick）：粒子随年龄
	// 线性收缩到零，到期停止编码。落地边沿只有一帧事件沿，短寿命让扬尘
	// 在几格 tick 内自然散去；疾跑期间事件沿持续刷新锚点，尘雾跟随脚部。
	snowKickLifetimeTicks = uint64(6)
	// snowKickCubeSize 是雪尘小 cuboid 的首帧边长：小于降水雪点（0.09）与
	// 破碎 burst（0.09），近脚扬尘不遮蔽画面。
	snowKickCubeSize = float32(0.05)
	// snowKickRisePerTick 是雪尘的纵向扬起速度（格/tick）：只向上、无回落，
	// 消散由寿命收缩承担。
	snowKickRisePerTick = float32(0.09)
	// snowKickSalt 是踢雪散列的固定盐：把事件域与降水（weatherSalt）、破碎
	// burst（breakBurstSalt）的散列区分开。
	snowKickSalt = uint32(0x27220A95)
)

// SnowKickInput 是单帧踢雪尘的驱动输入：Kick 是本帧的事件沿（雪面疾跑进行
// 中或落地边沿，由 app 从本地预测状态派生），Feet 是事件脚位（本地玩家脚部
// 世界坐标）。全部为本地呈现量，不进协议、不回写权威。
type SnowKickInput struct {
	Kick bool
	Feet mgl32.Vec3
}

// SnowKicks 持有最近一次踢雪事件的零点跟踪：事件沿钉住 tick 与脚位，其后
// 粒子的位置与尺寸都由该零点与当前 tick 纯推导，不存逐帧状态（与
// `BreakBursts` 同一纪律）。同 tick 同输入重复驱动幂等（锚点重写为同值），
// 输出逐字节稳定；会话重置与场景切换经 `Reset` 清空，旧锚点不得带入新会话。
type SnowKicks struct {
	valid     bool
	startTick uint64
	origin    mgl32.Vec3
	seed      uint32
}

// Reset 清空事件锚点：会话重置、断线重连与抓帧场景切换后调用，旧事件的
// 扬尘不得在新会话首帧继续老化。
func (kicks *SnowKicks) Reset() {
	*kicks = SnowKicks{}
}

// BuildParts 把存活踢雪尘编码为 avatar 通道的实心小 cuboid：事件沿（Kick）
// 重锚零点后按 `SnowKickParticles` 产出粒子，无事件的帧沿旧锚点继续老化，
// 寿命到期或从未有过事件时返回空。粒子水平按散列铺展在脚部小半径圆环上、
// 纵向随年龄扬起，尺寸随年龄线性收缩到零；颜色复用降水雪点的近白纯色、
// 材质走哨兵纯色分支。调用方复用 `dst` 缓冲时稳定帧零分配。
func (kicks *SnowKicks) BuildParts(dst []avatarPart, serverTick uint64, input SnowKickInput) []avatarPart {
	if input.Kick {
		kicks.valid = true
		kicks.startTick = serverTick
		kicks.origin = input.Feet
		kicks.seed = snowKickSeed(serverTick, input.Feet)
	}
	if !kicks.valid {
		return dst
	}
	age := snowKickAge(serverTick, kicks.startTick)
	if age >= snowKickLifetimeTicks {
		return dst
	}
	elapsed := float32(age)
	fade := 1 - elapsed/float32(snowKickLifetimeTicks)
	size := snowKickCubeSize * fade
	for index := range SnowKickParticles {
		hash := snowKickHash(kicks.seed, index)
		azimuth := (float32(index) + float32(hash&7)/8) * (math.Pi / 4)
		radius := 0.08 + 0.045*float32((hash>>4)&7)
		// 各粒扬起幅度按散列分档（0.6..1.4 倍）：同环粒子不同高，避免整环
		// 齐升的机械感；纵向恒非负，雪尘只向上扬。
		rise := snowKickRisePerTick * elapsed * (0.6 + 0.8*float32((hash>>7)&7)/7)
		center := kicks.origin.Add(mgl32.Vec3{
			radius * float32(math.Cos(float64(azimuth))),
			rise,
			radius * float32(math.Sin(float64(azimuth))),
		})
		dst = append(dst, avatarPart{
			transform: mgl32.Translate3D(center.X(), center.Y(), center.Z()).
				Mul4(mgl32.Scale3D(size, size, size)),
			color:    weatherSnowColor,
			material: avatarMaterialSolid,
		})
	}
	return dst
}

// snowKickAge 返回事件年龄：tick 回退（重连/场景重钉）钳制为零不下溢；会话
// 边界由调用方经 `Reset` 清锚，钳制只兜底同会话内的抖动。
func snowKickAge(serverTick, startTick uint64) uint64 {
	if serverTick < startTick {
		return 0
	}
	return serverTick - startTick
}

// snowKickSeed 把（事件 tick，脚位所在格）折叠为 u32 事件种子：脚位按格量化
// ——同格内的帧间微移不重掷散列，尘雾在格内稳定、跨格才换形态；种子只来自
// 输入的纯函数，同 tick 同输入必同种子。
func snowKickSeed(serverTick uint64, feet mgl32.Vec3) uint32 {
	hash := uint32(serverTick)*0x9E3779B1 + snowKickSalt
	hash ^= uint32(int32(math.Floor(float64(feet.X())))) * 3
	hash ^= uint32(int32(math.Floor(float64(feet.Z())))) * 7
	hash ^= hash >> 16
	hash *= 16777619
	hash ^= hash >> 13
	return hash
}

// snowKickHash 把（事件种子，粒子序号）折叠为 u32 散列：沿用小整数乘加混合
// 与两次异或雪崩的习惯，每粒取不同字节段分别映射方位、半径与扬起档位。
func snowKickHash(seed uint32, index int) uint32 {
	hash := seed ^ (uint32(index+1) * 0x9E3779B1)
	hash ^= hash >> 16
	hash *= 16777619
	hash ^= hash >> 13
	return hash
}
