package render

import (
	"encoding/binary"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// WeatherSnowLineY 是降水形态判定的雪线高度：粒子世界高度在线上为雪、
	// 在线下为雨。它复用既有地表雪线常量（Rust 世界生成的 `SNOW_LINE`），
	// 纯本地只读计算，不进权威状态、不同步，由单测钉住两端同值。
	WeatherSnowLineY = float32(88)
	// WeatherMaxParticles 是单帧降水粒子的固定上限：相机周围有界体内的确定
	// 性数量，与天气稳定与否无关，绘制侧超限整帧拒绝。
	WeatherMaxParticles = 256
	// ThunderFlashPeriodTicks 是雷暴闪光的固定周期（权威 tick），
	// ThunderFlashOnTicks 是每周期内点亮的 tick 数，ThunderFlashMaxLift
	// 是单帧亮度增量的固定上限（无伤害语义，只叠加呈现）。
	ThunderFlashPeriodTicks = uint64(80)
	ThunderFlashOnTicks     = uint64(5)
	ThunderFlashMaxLift     = float32(0.22)
)

// WeatherDaylightCap 返回给定天气的昼夜亮度上限：晴天不压暗，雨天等效天空
// 光不超过 12/15，雷暴不超过 10/15。钳制只作用于呈现侧的 `Daylight` 均匀量，
// 不回写任何体素光照、不触发重网格。
func WeatherDaylightCap(kind core.WeatherKind) float32 {
	switch kind {
	case core.WeatherRain:
		return 12.0 / 15.0
	case core.WeatherThunder:
		return 10.0 / 15.0
	default:
		return 1
	}
}

// ThunderFlashLift 返回给定权威 tick 的雷暴闪光增量：周期内前固定 tick 点亮
// 固定幅度，其余时间为零。同 tick 重放一致，不读墙钟与本地随机。
func ThunderFlashLift(serverTick uint64) float32 {
	if serverTick%ThunderFlashPeriodTicks < ThunderFlashOnTicks {
		return ThunderFlashMaxLift
	}
	return 0
}

// ApplyWeatherDaylight 把天气压暗与雷暴闪光叠加到昼夜亮度上：先按天气上限
// 钳制，再只在雷暴叠加当 tick 的闪光增量，总量封顶在 1。晴天恒等，午夜低
// 亮度不受压暗影响。
func ApplyWeatherDaylight(daylight float32, kind core.WeatherKind, serverTick uint64) float32 {
	capped := min(daylight, WeatherDaylightCap(kind))
	if kind == core.WeatherThunder {
		capped += ThunderFlashLift(serverTick)
		if capped > 1 {
			capped = 1
		}
	}
	return capped
}

// WeatherSkyGray 返回给定天气的天空灰度因子：晴天为零（恒等），雨/雷暴为
// 固定正值且雷暴更灰。灰化只作用于呈现侧的天空均匀量与 clear 色，不新增
// GPU 管线。
func WeatherSkyGray(kind core.WeatherKind) float32 {
	switch kind {
	case core.WeatherRain:
		return 0.5
	case core.WeatherThunder:
		return 0.75
	default:
		return 0
	}
}

// WeatherSkyColor 把晴天 clear 色按天气灰度向亮度灰靠拢：每通道与灰的距离
// 缩小，alpha 不变；晴天恒等。`HUD` 与昵称不消费该值（沿用既有语义）。
func WeatherSkyColor(clear [4]float32, kind core.WeatherKind) [4]float32 {
	gray := WeatherSkyGray(kind)
	if gray == 0 {
		return clear
	}
	luma := 0.299*clear[0] + 0.587*clear[1] + 0.114*clear[2]
	out := clear
	for channel := range 3 {
		out[channel] = clear[channel] + (luma-clear[channel])*gray
	}
	return out
}

const (
	// weatherColumnHeight 是降水列高：粒子在相机上下各半列内随 tick 下落并
	// 回绕，无堆积（位置是序号与 tick 的纯函数，不存逐帧状态）。
	weatherColumnHeight = float32(20)
	// weatherFallPerTick 是降水下落速度（格/权威 tick），雨雪同速：形态只由
	// 高度选形，速度不参与区分，保持位置为 tick 的闭式纯函数。
	weatherFallPerTick = float32(0.55)
	// weatherSalt 是降水散列的固定盐：把序号混合到三轴偏移域。
	weatherSalt = uint32(0x85EBCA6B)
)

var (
	// weatherRainColor 是雨丝的原创蓝灰纯色，weatherSnowColor 是雪点的近白
	// 纯色：纯色分支走哨兵材质，不采样 atlas。
	weatherRainColor = [4]float32{0.58, 0.68, 0.88, 0.8}
	weatherSnowColor = [4]float32{0.94, 0.95, 1.0, 0.9}
)

// weatherHash 把粒子序号折叠为 u32 散列：小整数乘加混合后雪崩，每粒取三
// 字节分别映射三轴，256 粒在体内几乎必然两两不同。
func weatherHash(index int) uint32 {
	hash := uint32(index+1)*0x9E3779B1 + weatherSalt
	hash ^= hash >> 16
	hash *= 16777619
	hash ^= hash >> 13
	return hash
}

// BuildWeatherParts 把降水编码为 avatar 通道的实心小 cuboid：晴天返回空，
// 雨/雷暴返回固定上限数量。位置是（序号，权威 tick）的纯函数——水平按散列
// 铺展在相机前方有界体内（含前向偏置），纵向随 tick 下落并在列内回绕；形态
// 只由粒子当前世界高度相对雪线确定（线上雪点、线下雨丝），雪点叠加小幅水平
// 摆动。调用方复用 `dst` 缓冲时稳定天气帧零分配。
func BuildWeatherParts(dst []avatarPart, cam mgl32.Vec3, yaw float32, serverTick uint64, kind core.WeatherKind) []avatarPart {
	if kind != core.WeatherRain && kind != core.WeatherThunder {
		return dst[:0]
	}
	sy := float32(math.Sin(float64(yaw)))
	cy := float32(math.Cos(float64(yaw)))
	// 相机前向（`-Z` 为零偏航）与右向：与 `Camera.Forward` 及帧装配的
	// billboard 右向同式，体在水平面内前向偏置（后 6 格、前 18 格）。
	forward := mgl32.Vec3{-sy, 0, -cy}
	right := mgl32.Vec3{cy, 0, -sy}
	fall := float32(serverTick) * weatherFallPerTick
	for index := range WeatherMaxParticles {
		hash := weatherHash(index)
		x01 := float32(hash&255) / 255
		y01 := float32((hash>>8)&255) / 255
		z01 := float32((hash>>16)&255) / 255
		lateral := (x01 - 0.5) * 24
		depth := -6 + z01*24
		center := cam.Add(right.Mul(lateral)).Add(forward.Mul(depth))
		phase := y01*weatherColumnHeight + fall
		center[1] = cam.Y() + weatherColumnHeight/2 - float32(math.Mod(float64(phase), float64(weatherColumnHeight)))
		if center.Y() >= WeatherSnowLineY {
			// 雪点：小立方体 + 随 tick 的水平摆动（同 tick 重放一致）。
			sway := 0.4 * float32(math.Sin(2*math.Pi*float64((serverTick+uint64(hash&31))%32)/32))
			center[0] += sway
			dst = append(dst, avatarPart{
				transform: mgl32.Translate3D(center[0], center[1], center[2]).
					Mul4(mgl32.Scale3D(0.09, 0.09, 0.09)),
				color:    weatherSnowColor,
				material: avatarMaterialSolid,
			})
			continue
		}
		dst = append(dst, avatarPart{
			transform: mgl32.Translate3D(center[0], center[1], center[2]).
				Mul4(mgl32.Scale3D(0.035, 0.6, 0.035)),
			color:    weatherRainColor,
			material: avatarMaterialSolid,
		})
	}
	return dst
}

// weatherStateBytes 是天气状态段的字节数：灰度因子单个 f32。
const weatherStateBytes = 4

// EncodeWeatherState 把天气灰度编码为帧状态段负载：晴天返回空（调用方不
// 追加段，保证晴天帧与变更前逐位一致），雨/雷暴返回 4 字节小端 f32。
// `dst` 会被重置复用。本文件不依赖 darwin 专属的编码缓冲（`frame_streams.go`
// 只在 darwin 构建），生长逻辑内联以保持跨平台可构建。
func EncodeWeatherState(dst []byte, kind core.WeatherKind) []byte {
	if kind != core.WeatherRain && kind != core.WeatherThunder {
		return dst[:0]
	}
	if cap(dst) < weatherStateBytes {
		dst = make([]byte, weatherStateBytes)
	}
	dst = dst[:weatherStateBytes]
	binary.LittleEndian.PutUint32(dst, math.Float32bits(WeatherSkyGray(kind)))
	return dst
}
