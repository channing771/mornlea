//go:build darwin

package client

// render_crack_test.go：裂纹实例流的条件段编码。流为空时帧字节与本字段
// 加入之前逐位一致（TLV 段按条件追加，镜像 OverlayStrength/WaterTint 的
// 条件段先例，既有 golden 场景依赖这一点）；流非空时追加恰一个 tag 10
// 段，内容可原样解析回。

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// walkFrameTLVs 解析 layout 2 帧的 TLV 段序列，返回按出现顺序的 (tag, len)。
func walkFrameTLVs(t *testing.T, out []byte) [][2]uint32 {
	t.Helper()
	// 前置守卫：地形可见列表非空时，header 之后紧跟的是逐 section 12 字节的
	// 可见列表而不是 TLV 段，从 header 尾开始解析会把地形数据误读成段序列。
	// 现有用例全部使用零可见列表的纯地形帧；带可见列表的帧须先裁掉列表。
	if visible := binary.LittleEndian.Uint32(out[184:]); visible != 0 {
		t.Fatalf("walkFrameTLVs 只支持无地形可见列表的帧：Visible=%d", visible)
	}
	var segments [][2]uint32
	cursor := renderFrameHeaderBytes
	for cursor < len(out) {
		if cursor+8 > len(out) {
			t.Fatalf("TLV 头越界: cursor=%d len=%d", cursor, len(out))
		}
		tag := binary.LittleEndian.Uint32(out[cursor:])
		length := binary.LittleEndian.Uint32(out[cursor+4:])
		if cursor+8+int(length) > len(out) {
			t.Fatalf("TLV 负载越界: tag=%d len=%d", tag, length)
		}
		segments = append(segments, [2]uint32{tag, length})
		cursor += 8 + int(length)
	}
	return segments
}

func TestEncodeRenderFrameCrackSegment(t *testing.T) {
	// 无裂纹帧保持纯地形 layout：不因新字段出现而改变帧形态。
	dry := EncodeRenderFrame(RenderFrame{})
	if dry[188] != 0 || len(dry) != renderFrameHeaderBytes {
		t.Fatalf("无裂纹帧 layout=%d len=%d，想要纯地形帧", dry[188], len(dry))
	}

	// 既有段 + 空裂纹流：逐位等于只携带既有段的帧（字段缺席不改字节）。
	outline := make([]byte, 12*80)
	baseline := EncodeRenderFrame(RenderFrame{OutlineInstances: outline})
	withEmptyCrack := EncodeRenderFrame(RenderFrame{OutlineInstances: outline, CrackInstances: nil})
	if !bytes.Equal(baseline, withEmptyCrack) {
		t.Fatal("空裂纹流改变了既有帧字节")
	}
	for _, segment := range walkFrameTLVs(t, baseline) {
		if segment[0] == frameTagCrack {
			t.Fatal("空裂纹流出现了 tag 10 段")
		}
	}

	// 非空裂纹流：layout 2，追加恰一个 tag 10 段，负载原样保留。
	crack := make([]byte, 80)
	for index := range crack {
		crack[index] = byte(index)
	}
	out := EncodeRenderFrame(RenderFrame{CrackInstances: crack})
	if out[188] != 2 {
		t.Fatalf("有裂纹帧 layout=%d，想要 2", out[188])
	}
	if len(out) != renderFrameHeaderBytes+8+len(crack) {
		t.Fatalf("帧长度=%d，想要头部 + 一个 TLV 头 + %d 字节负载", len(out), len(crack))
	}
	segments := walkFrameTLVs(t, out)
	if len(segments) != 1 || segments[0][0] != frameTagCrack || segments[0][1] != uint32(len(crack)) {
		t.Fatalf("TLV 段=%v，想要恰一个 tag %d/%d", segments, frameTagCrack, len(crack))
	}
	if payload := out[renderFrameHeaderBytes+8:]; !bytes.Equal(payload, crack) {
		t.Fatal("裂纹段负载与输入不一致")
	}

	// 追加顺序：裂纹段在既有段之后，不扰动既有段序。
	avatars := make([]byte, 160)
	out = EncodeRenderFrame(RenderFrame{AvatarInstances: avatars, CrackInstances: crack})
	segments = walkFrameTLVs(t, out)
	if len(segments) != 2 ||
		segments[0] != [2]uint32{frameTagAvatar, uint32(len(avatars))} ||
		segments[1] != [2]uint32{frameTagCrack, uint32(len(crack))} {
		t.Fatalf("TLV 段=%v，想要 avatar 在前、crack 在后", segments)
	}
}
