//go:build darwin

package client

// render_weather_test.go：天气状态段与降水实例流的条件段编码（client ABI
// v18 新增帧 TLV tag 12/13：tag 12 是 4 字节天气灰度，tag 13 是 96 字节/实例
// 的降水实例流、与 avatar 同布局）。晴天两段恒为空，帧字节与引入前逐位一致
// （沿裂纹/viewmodel 段的条件追加纪律）；非空时各追加恰一个段并落在
// viewmodel 段之后，不扰动既有段序。

import (
	"bytes"
	"testing"
)

func TestEncodeRenderFrameWeatherSegments(t *testing.T) {
	// tag 取下一个空闲值：1..11 已占用（9 退役仍保留拒绝语义）。
	if frameTagWeather != 12 {
		t.Fatalf("天气状态 TLV tag=%d，想要 12", frameTagWeather)
	}
	if frameTagPrecip != 13 {
		t.Fatalf("降水实例 TLV tag=%d，想要 13", frameTagPrecip)
	}

	// 晴天（两段恒为空）保持纯地形 layout：不因新字段出现而改变帧形态。
	dry := EncodeRenderFrame(RenderFrame{})
	if dry[188] != 0 || len(dry) != renderFrameHeaderBytes {
		t.Fatalf("晴天帧 layout=%d len=%d，想要纯地形帧", dry[188], len(dry))
	}

	// 既有段 + 空天气段：逐位等于只携带既有段的帧。
	outline := make([]byte, 12*80)
	baseline := EncodeRenderFrame(RenderFrame{OutlineInstances: outline})
	withEmpty := EncodeRenderFrame(RenderFrame{
		OutlineInstances: outline, WeatherSegment: nil, PrecipInstances: nil,
	})
	if !bytes.Equal(baseline, withEmpty) {
		t.Fatal("空天气段改变了既有帧字节")
	}
	for _, segment := range walkFrameTLVs(t, baseline) {
		if segment[0] == frameTagWeather || segment[0] == frameTagPrecip {
			t.Fatal("空天气段出现了 tag 12/13 段")
		}
	}

	// 非空天气段：layout 2，追加恰一个 tag 12 段，负载原样保留。
	weather := []byte{0x11, 0x22, 0x33, 0x44}
	out := EncodeRenderFrame(RenderFrame{WeatherSegment: weather})
	if out[188] != 2 {
		t.Fatalf("有天气帧 layout=%d，想要 2", out[188])
	}
	segments := walkFrameTLVs(t, out)
	if len(segments) != 1 || segments[0][0] != frameTagWeather || segments[0][1] != uint32(len(weather)) {
		t.Fatalf("TLV 段=%v，想要恰一个 tag %d/%d", segments, frameTagWeather, len(weather))
	}
	if payload := out[renderFrameHeaderBytes+8:]; !bytes.Equal(payload, weather) {
		t.Fatal("天气段负载与输入不一致")
	}

	// 非空降水流：追加恰一个 tag 13 段。
	precip := make([]byte, 96)
	for index := range precip {
		precip[index] = byte(index)
	}
	out = EncodeRenderFrame(RenderFrame{PrecipInstances: precip})
	segments = walkFrameTLVs(t, out)
	if len(segments) != 1 || segments[0][0] != frameTagPrecip || segments[0][1] != uint32(len(precip)) {
		t.Fatalf("TLV 段=%v，想要恰一个 tag %d/%d", segments, frameTagPrecip, len(precip))
	}

	// 追加顺序：天气/降水段在 viewmodel 段之后，不扰动既有段序。
	avatars := make([]byte, 160)
	viewmodel := make([]byte, 96)
	out = EncodeRenderFrame(RenderFrame{
		AvatarInstances: avatars, ViewmodelInstances: viewmodel,
		WeatherSegment: weather, PrecipInstances: precip,
	})
	segments = walkFrameTLVs(t, out)
	if len(segments) != 4 ||
		segments[0] != [2]uint32{frameTagAvatar, uint32(len(avatars))} ||
		segments[1] != [2]uint32{frameTagViewmodel, uint32(len(viewmodel))} ||
		segments[2] != [2]uint32{frameTagWeather, uint32(len(weather))} ||
		segments[3] != [2]uint32{frameTagPrecip, uint32(len(precip))} {
		t.Fatalf("TLV 段=%v，想要 avatar、viewmodel 在前、天气、降水在后", segments)
	}
}
