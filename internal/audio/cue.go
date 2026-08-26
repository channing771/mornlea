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
	cueCount
)

type cueSpec struct {
	samples   int
	startHz   int
	endHz     int
	amplitude int16
}

var cueSpecs = [cueCount]cueSpec{
	CueUIClick:        {samples: 772, startHz: 1200, endHz: 900, amplitude: 7000},
	CueMiningComplete: {samples: 2646, startHz: 180, endHz: 90, amplitude: 10000},
	CueEatingComplete: {samples: 3087, startHz: 440, endHz: 660, amplitude: 8000},
	CueDamage:         {samples: 2205, startHz: 95, endHz: 60, amplitude: 12000},
	// 水花复用方波下滑音：约 91 ms，比伤害/采掘亮、比 UI click 长（design.md Decision 3）。
	CueWaterSplash: {samples: 2000, startHz: 800, endHz: 220, amplitude: 11000},
}

func (cue Cue) valid() bool {
	return cue < cueCount
}

// synthesize 用整数方波和线性衰减包络生成固定 PCM，避免平台浮点实现差异。
func synthesize(spec cueSpec) []int16 {
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
