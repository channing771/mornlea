// Package audio 提供仅由图形客户端消费的本地确认提示音。
package audio

const (
	sampleRate = 22050
	channels   = 1
)

// Cue 标识一种预生成的本地确认提示音。
type Cue uint8

const (
	// CueUIClick 对应成功的界面确认操作。
	CueUIClick Cue = iota
	// CueMiningComplete 对应权威确认的一次采掘完成。
	CueMiningComplete
	// CueEatingComplete 对应权威确认的一次进食完成。
	CueEatingComplete
	// CueDamage 对应收到的一次权威伤害确认。
	CueDamage
	// CueWaterSplash 对应本地玩家身体从干燥进入流体的权威确认上升沿（入水水花）。
	CueWaterSplash
	// CueCombatHit 对应严格递增的权威命中确认。
	CueCombatHit
	// CueSnowStep 对应本地玩家在雪层上落足移动、水平位移累计跨过固定步长
	// （design 总表 0.8 格）的一次踩雪 crunch；纯本地预测派生，不回写权威。
	CueSnowStep
	cueCount
)

type cueSpec struct {
	samples   int
	startHz   int
	endHz     int
	amplitude int16
	// noise 为真时走噪声合成分支（踩雪 crunch）：startHz/endHz 无意义恒零，
	// 白噪符号位直接乘衰减包络，区别于方波扫频分支。
	noise bool
}

var cueSpecs = [cueCount]cueSpec{
	CueUIClick:        {samples: 772, startHz: 1200, endHz: 900, amplitude: 7000},
	CueMiningComplete: {samples: 2646, startHz: 180, endHz: 90, amplitude: 10000},
	CueEatingComplete: {samples: 3087, startHz: 440, endHz: 660, amplitude: 8000},
	CueDamage:         {samples: 2205, startHz: 95, endHz: 60, amplitude: 12000},
	// 水花复用方波下滑音：约 91 ms，比伤害/采掘亮、比 UI click 长（design.md Decision 3）。
	CueWaterSplash: {samples: 2000, startHz: 800, endHz: 220, amplitude: 11000},
	CueCombatHit:   {samples: 1323, startHz: 520, endHz: 180, amplitude: 10500},
	// 踩雪 crunch 是约 50 ms 的白噪短促爆发：密度远高于方波扫频的符号翻转
	// 构成「嘎吱」质感，振幅低于伤害/命中以适配步频重复播放。
	CueSnowStep: {samples: 1102, amplitude: 9000, noise: true},
}

func (cue Cue) valid() bool {
	return cue < cueCount
}

// synthesize 用整数方波和线性衰减包络生成固定 PCM，避免平台浮点实现差异。
// noise 分支改走整数 xorshift 白噪（见 synthesizeNoise），两条路径同为纯整数
// 运算，跨平台输出逐位一致。
func synthesize(spec cueSpec) []int16 {
	if spec.noise {
		return synthesizeNoise(spec)
	}
	pcm := make([]int16, spec.samples)
	var phase uint32
	for index := range pcm {
		frequency := spec.startHz + (spec.endHz-spec.startHz)*index/max(1, spec.samples-1)
		phase += uint32(frequency * 65536 / sampleRate)
		sign := int32(-1)
		if phase&0x8000 != 0 {
			sign = 1
		}
		envelope := int32(spec.samples - index)
		pcm[index] = int16(sign * int32(spec.amplitude) * envelope / int32(spec.samples))
	}
	return pcm
}

// synthesizeNoise 生成踩雪 crunch 的白噪 PCM：xorshift32 状态的最低位直接取
// 符号，乘与方波分支同一形状的线性衰减包络。种子是固定常量——同一 spec 恒
// 生成同一波形，不读全局随机源，重放与抓帧可复现。
func synthesizeNoise(spec cueSpec) []int16 {
	pcm := make([]int16, spec.samples)
	state := uint32(0x1D872B41)
	for index := range pcm {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		sign := int32(-1)
		if state&1 != 0 {
			sign = 1
		}
		envelope := int32(spec.samples - index)
		pcm[index] = int16(sign * int32(spec.amplitude) * envelope / int32(spec.samples))
	}
	return pcm
}
