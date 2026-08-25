package main

// capture_water_underwater_test.go 钉住水下视觉场景必须位于 capture 场景表末尾的
// 位置性契约，避免权威高 tick 浸没状态污染后续场景。

import "testing"

// TestWaterUnderwaterCaptureSceneIsLast 钉住 water-underwater 排在场景表最后。
//
// 这条约束承的重是**位置性**的：该场景注入的权威 `PlayerState` 带一个远大于真实
// 值的 `ServerTick`，之后一切真实 `PlayerState` 都会被 `Predictor` 的单调校验静默
// 忽略，眼睛浸没标志因此永久停在"在水里"。排在它之后的任何场景都会带着水色
// 叠加与被压低的可见半径出图——实测把它插在 ai-companion 之前时，ai-companion
// 有 98.75% 的像素随之改变。
//
// 断言写成"最后一个是它"而不是"它在表里"：后者是存在性断言，插到中间也照样
// 通过，正是要挡的那种改动。
func TestWaterUnderwaterCaptureSceneIsLast(t *testing.T) {
	last := captureScenes[len(captureScenes)-1]
	if last.Name != "water-underwater" {
		t.Fatalf("场景表最后一个是 %q，想要 water-underwater", last.Name)
	}
	if last.Prepare == nil || last.Apply == nil || last.WarmupFrames != 8 {
		t.Fatalf("water-underwater 场景不完整: %+v", last)
	}
}
