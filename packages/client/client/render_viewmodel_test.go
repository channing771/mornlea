//go:build darwin

package client

// render_viewmodel_test.go：第一人称双手 viewmodel 实例流的条件段编码
// （client ABI v17 新增帧 TLV tag 11，定长 viewmodel 实例流）。流为空时
// 帧字节与引入前逐位一致（沿裂纹段的条件追加纪律）；流非空时追加恰一个
// tag 11 段并落在裂纹段之后，不扰动既有段序。

import (
	"bytes"
	"testing"
)

func TestEncodeRenderFrameViewmodelSegment(t *testing.T) {
	// tag 取下一个空闲值：1..10 已占用（9 退役仍保留拒绝语义）。
	if frameTagViewmodel != 11 {
		t.Fatalf("viewmodel TLV tag=%d，想要 11", frameTagViewmodel)
	}

	// 无 viewmodel 帧保持纯地形 layout：不因新字段出现而改变帧形态。
	dry := EncodeRenderFrame(RenderFrame{})
	if dry[188] != 0 || len(dry) != renderFrameHeaderBytes {
		t.Fatalf("无 viewmodel 帧 layout=%d len=%d，想要纯地形帧", dry[188], len(dry))
	}

	// 既有段 + 空 viewmodel 流：逐位等于只携带既有段的帧。
	outline := make([]byte, 12*80)
	baseline := EncodeRenderFrame(RenderFrame{OutlineInstances: outline})
	withEmpty := EncodeRenderFrame(RenderFrame{OutlineInstances: outline, ViewmodelInstances: nil})
	if !bytes.Equal(baseline, withEmpty) {
		t.Fatal("空 viewmodel 流改变了既有帧字节")
	}
	for _, segment := range walkFrameTLVs(t, baseline) {
		if segment[0] == frameTagViewmodel {
			t.Fatal("空 viewmodel 流出现了 tag 11 段")
		}
	}

	// 非空 viewmodel 流：layout 2，追加恰一个 tag 11 段，负载原样保留。
	viewmodel := make([]byte, 96)
	for index := range viewmodel {
		viewmodel[index] = byte(index)
	}
	out := EncodeRenderFrame(RenderFrame{ViewmodelInstances: viewmodel})
	if out[188] != 2 {
		t.Fatalf("有 viewmodel 帧 layout=%d，想要 2", out[188])
	}
	if len(out) != renderFrameHeaderBytes+8+len(viewmodel) {
		t.Fatalf("帧长度=%d，想要头部 + 一个 TLV 头 + %d 字节负载", len(out), len(viewmodel))
	}
	segments := walkFrameTLVs(t, out)
	if len(segments) != 1 || segments[0][0] != frameTagViewmodel || segments[0][1] != uint32(len(viewmodel)) {
		t.Fatalf("TLV 段=%v，想要恰一个 tag %d/%d", segments, frameTagViewmodel, len(viewmodel))
	}
	if payload := out[renderFrameHeaderBytes+8:]; !bytes.Equal(payload, viewmodel) {
		t.Fatal("viewmodel 段负载与输入不一致")
	}

	// 追加顺序：viewmodel 段在裂纹段之后，不扰动既有段序。
	avatars := make([]byte, 160)
	crack := make([]byte, 80)
	out = EncodeRenderFrame(RenderFrame{AvatarInstances: avatars, CrackInstances: crack, ViewmodelInstances: viewmodel})
	segments = walkFrameTLVs(t, out)
	if len(segments) != 3 ||
		segments[0] != [2]uint32{frameTagAvatar, uint32(len(avatars))} ||
		segments[1] != [2]uint32{frameTagCrack, uint32(len(crack))} ||
		segments[2] != [2]uint32{frameTagViewmodel, uint32(len(viewmodel))} {
		t.Fatalf("TLV 段=%v，想要 avatar、crack 在前、viewmodel 在后", segments)
	}
}
