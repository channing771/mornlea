package audio

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

// TestCuePCM 锁定每个本地提示音的整数波形。若频率、包络或相位推进被意外修改，
// 固定摘要会立即暴露，避免在没有音频设备的自动测试中静默漂移。
func TestCuePCM(t *testing.T) {
	if sampleRate != 22050 {
		t.Fatalf("sample rate = %d, want 22050", sampleRate)
	}
	if channels != 1 {
		t.Fatalf("channels = %d, want mono", channels)
	}
	cases := []struct {
		name      string
		cue       Cue
		samples   int
		startHz   int
		endHz     int
		amplitude int
		hash      string
	}{
		{"click", CueUIClick, 772, 1200, 900, 7000, "6abfada26f17733f85a1da11648c8471a7609b96e1df428d4776d77640ebe011"},
		{"mining", CueMiningComplete, 2646, 180, 90, 10000, "0fd329cb57015968b435da587887563713d4795a4ad33545ee051e46a8134d25"},
		{"eating", CueEatingComplete, 3087, 440, 660, 8000, "f1a63c03b7bd7668aa2035787e7904d41b3347c93fb3f848b36d368e9ca83005"},
		{"damage", CueDamage, 2205, 95, 60, 12000, "2e844e35047ef374ae72f40e1858fa7a86abbc832ec17cf05071665ab8fbd213"},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cue != Cue(index) {
				t.Fatalf("cue = %d, want stable value %d", tc.cue, index)
			}
			spec := cueSpecs[tc.cue]
			if spec.samples != tc.samples || spec.startHz != tc.startHz || spec.endHz != tc.endHz || int(spec.amplitude) != tc.amplitude {
				t.Fatalf("cue spec = %+v, want samples=%d frequency=%d→%d amplitude=%d", spec, tc.samples, tc.startHz, tc.endHz, tc.amplitude)
			}
			pcm := synthesize(spec)
			if len(pcm) != tc.samples {
				t.Fatalf("samples = %d, want %d", len(pcm), tc.samples)
			}
			checkBasicPCM(t, pcm, int16(tc.amplitude))
			if got := pcmSHA256(pcm); got != tc.hash {
				t.Fatalf("little-endian PCM SHA-256 = %s, want %s", got, tc.hash)
			}
		})
	}
}

func pcmSHA256(pcm []int16) string {
	encoded := make([]byte, len(pcm)*2)
	for index, sample := range pcm {
		binary.LittleEndian.PutUint16(encoded[index*2:], uint16(sample))
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// TestCueWaterSplashSynthesisProperty 证水花提示音的合成性质：常量紧随
// `CueDamage` 之后，PCM 全程非零、无 int16 溢出、峰值恰为振幅，且线性衰减
// 包络把末样本压到零附近（单个包络步长以内），不会在播放结束时突兀截断。
//
// 刻意不锁 SHA-256 golden：音色微调属 design.md Decision 3 的实现自由度，
// 性质断言已足够。
func TestCueWaterSplashSynthesisProperty(t *testing.T) {
	if CueWaterSplash != CueDamage+1 {
		t.Fatalf("CueWaterSplash = %d, want %d（必须排在 CueDamage 之后）", CueWaterSplash, CueDamage+1)
	}
	spec := cueSpecs[CueWaterSplash]
	pcm := synthesize(spec)
	if len(pcm) != spec.samples {
		t.Fatalf("samples = %d, want %d", len(pcm), spec.samples)
	}
	checkBasicPCM(t, pcm, spec.amplitude)
	last := max(int(pcm[len(pcm)-1]), -int(pcm[len(pcm)-1]))
	if step := int(spec.amplitude) / spec.samples; last > step {
		t.Fatalf("末样本幅度 = %d, want <= %d（线性包络末步）", last, step)
	}
}

// checkBasicPCM 断言合成 PCM 的基础性质：全程非零、无 int16 溢出且峰值恰为
// 振幅。供 `TestCuePMC` 与 `TestCueWaterSplashSynthesisProperty` 共用。
func checkBasicPCM(t *testing.T, pcm []int16, amplitude int16) {
	t.Helper()
	var nonZero bool
	peak := 0
	for _, sample := range pcm {
		if sample != 0 {
			nonZero = true
		}
		if sample == -1<<15 {
			t.Fatalf("sample overflows int16 magnitude: %d", sample)
		}
		magnitude := max(int(sample), -int(sample))
		peak = max(peak, magnitude)
	}
	if !nonZero {
		t.Fatal("PCM must not be silent")
	}
	if peak != int(amplitude) {
		t.Fatalf("PCM peak = %d, want %d", peak, amplitude)
	}
}
