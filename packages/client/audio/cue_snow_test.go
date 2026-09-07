package audio

import (
	"testing"
)

// TestCueSnowStepSynthesisProperty 证踩雪 crunch 提示音的合成性质：常量紧随
// `CueCombatHit` 之后；噪声分支全程非零、无 int16 溢出、峰值恰为振幅，线性
// 衰减包络把末样本压到零附近；符号翻转密度显著高于任何方波扫频（crunch 的
// 白噪特征），且同一 spec 两次合成逐字节一致（整数实现无平台差异）。
//
// 刻意不锁 SHA-256 golden：crunch 音色微调属实现自由度（沿 `CueWaterSplash`
// 的裁决先例），性质断言已足够钉住「短促噪声 + 衰减包络」的形态。
func TestCueSnowStepSynthesisProperty(t *testing.T) {
	if CueSnowStep != CueCombatHit+1 {
		t.Fatalf("CueSnowStep = %d，想要 %d（必须排在 CueCombatHit 之后）", CueSnowStep, CueCombatHit+1)
	}
	if cueCount != CueSnowStep+1 {
		t.Fatalf("cueCount = %d，想要 %d", cueCount, CueSnowStep+1)
	}
	spec := cueSpecs[CueSnowStep]
	if spec.samples != 1102 || spec.amplitude != 9000 {
		t.Fatalf("cue spec = %+v，想要 samples=1102 amplitude=9000", spec)
	}
	pcm := synthesize(spec)
	if len(pcm) != spec.samples {
		t.Fatalf("samples = %d，想要 %d", len(pcm), spec.samples)
	}
	checkBasicPCM(t, pcm, spec.amplitude)
	last := max(int(pcm[len(pcm)-1]), -int(pcm[len(pcm)-1]))
	if step := int(spec.amplitude) / spec.samples; last > step {
		t.Fatalf("末样本幅度 = %d，想要 <= %d（线性包络末步）", last, step)
	}
	crossings := 0
	for index := 1; index < len(pcm); index++ {
		if (pcm[index] >= 0) != (pcm[index-1] >= 0) {
			crossings++
		}
	}
	// 方波扫频在 50ms 内至多几百次翻转（每半周期一次）；白噪的期望密度约为
	// 每样本 0.5 次。阈值取 samples/3 即可把两者分开。
	if crossings < spec.samples/3 {
		t.Fatalf("符号翻转数 = %d，想要 >= %d（噪声 crunch 特征）", crossings, spec.samples/3)
	}
	repeat := synthesize(spec)
	if pcmSHA256(pcm) != pcmSHA256(repeat) {
		t.Fatal("同一 spec 两次合成结果不一致")
	}
}
