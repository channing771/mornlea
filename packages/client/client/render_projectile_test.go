//go:build darwin

package client

import (
	"encoding/binary"
	"testing"
)

// TestEncodeRenderFrameProjectileSegment 锁定投射物实例段的帧编码：tag 14
// 是降水段之后的下一个空闲值；空流时整段缺席、帧与引入前逐位一致（纯地形
// 帧仍是 layout 0），非空流追加在降水段之后。
func TestEncodeRenderFrameProjectileSegment(t *testing.T) {
	if frameTagProjectile != 14 {
		t.Fatalf("frameTagProjectile=%d，想要 14（tag 13 之后的下一个空闲值）", frameTagProjectile)
	}

	// 空投射物流：纯地形帧保持 layout 0。
	dry := EncodeRenderFrame(RenderFrame{})
	if dry[188] != 0 || len(dry) != renderFrameHeaderBytes {
		t.Fatalf("无投射物帧 layout=%d len=%d，想要纯地形帧", dry[188], len(dry))
	}

	// 非空投射物流：layout 2，段追加在降水段之后。
	frame := RenderFrame{
		WeatherSegment:      make([]byte, 4),
		PrecipInstances:     make([]byte, 96),
		ProjectileInstances: make([]byte, 2*96),
	}
	out := EncodeRenderFrame(frame)
	if out[188] != 2 {
		t.Fatalf("携带投射物时 layout=%d，想要 2", out[188])
	}
	cursor := renderFrameHeaderBytes
	readU32 := func() uint32 {
		value := binary.LittleEndian.Uint32(out[cursor:])
		cursor += 4
		return value
	}
	tag, length := readU32(), readU32()
	if tag != frameTagWeather || length != 4 {
		t.Fatalf("首段 tag=%d len=%d，想要天气段 12/4", tag, length)
	}
	cursor += 4
	if tag, length = readU32(), readU32(); tag != frameTagPrecip || length != 96 {
		t.Fatalf("第二段 tag=%d len=%d，想要降水段 13/96", tag, length)
	}
	cursor += 96
	if tag, length = readU32(), readU32(); tag != frameTagProjectile || length != 2*96 {
		t.Fatalf("投射物段 tag=%d len=%d，想要 14/%d", tag, length, 2*96)
	}
	cursor += 2 * 96
	if cursor != len(out) {
		t.Fatalf("投射物段应为帧内最后一段：游标=%d 帧长=%d", cursor, len(out))
	}

	// 只有投射物段非空也构成 pass 段（layout 2）。
	only := EncodeRenderFrame(RenderFrame{ProjectileInstances: make([]byte, 96)})
	if only[188] != 2 {
		t.Fatalf("仅投射物段 layout=%d，想要 2", only[188])
	}
}
