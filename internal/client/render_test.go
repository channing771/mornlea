//go:build darwin

package client

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

func TestRendererApplyRenderWorldUpdates(t *testing.T) {
	renderer, err := NewRenderer(32, 16)
	if errors.Is(err, ErrNoGPUAdapter) {
		t.Skip("无 GPU 适配器")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()

	frame := RenderFrame{
		Daylight: 1,
		SkyColor: [4]float32{0.2, 0.4, 1, 1},
		Visible:  [][3]int32{{0, 0, 0}},
	}
	for i := 0; i < 4; i++ {
		frame.ViewProj[i*4+i] = 1
		frame.ViewProjInv[i*4+i] = 1
	}
	beforeFrame := EncodeRenderFrame(frame)
	renderer.RenderFrame(frame)
	beforeReadback := renderer.Readback()

	encoded, err := EncodeRenderWorldBatch(RenderWorldBatch{
		Epoch: 1,
		Updates: []RenderWorldUpdate{
			{Kind: RenderWorldReset},
			{
				Kind: RenderWorldSectionUpsert,
				Key: core.SectionKey{
					Dimension: core.Overworld,
					Pos:       core.SectionPos{X: 0, Y: 0, Z: 0},
				},
				Revision: 1,
				Snapshot: world.ContainerSnapshot{
					Kind:   world.StorageSingle,
					Single: core.StoneID,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	frameCalls, uploadCalls := renderer.FrameCalls(), renderer.UploadCalls()
	renderer.ApplyRenderWorldUpdates(encoded)
	if got := renderer.FrameCalls(); got != frameCalls {
		t.Fatalf("cache update 后 frame FFI=%d,想要 %d", got, frameCalls)
	}
	if got := renderer.UploadCalls(); got != uploadCalls {
		t.Fatalf("cache update 后 upload FFI=%d,想要 %d", got, uploadCalls)
	}
	afterFrame := EncodeRenderFrame(frame)
	if !bytes.Equal(beforeFrame, afterFrame) {
		t.Fatal("cache update 改变了相同 RenderFrame 的编码")
	}
	renderer.RenderFrame(frame)
	afterReadback := renderer.Readback()
	if !bytes.Equal(beforeReadback, afterReadback) {
		t.Fatal("cache update 改变了相同 RenderFrame 的 readback")
	}
}

func TestRendererApplyRenderWorldUpdatesRejectsEmpty(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("空 render world update 应 panic")
		}
	}()
	new(Renderer).ApplyRenderWorldUpdates(nil)
}

// TestRendererRoundtripOrSkip 走一遍 create→atlas→section→frame→readback:
// 无 GPU 适配器时跳过(与 gfx.NewHeadlessDevice 约定一致)。
func TestRendererRoundtripOrSkip(t *testing.T) {
	renderer, err := NewRenderer(32, 16)
	if errors.Is(err, ErrNoGPUAdapter) {
		t.Skip("无 GPU 适配器")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()

	// 单 layer 合法 atlas:16²+8²+4²+2²+1² 像素 × 4 字节。
	perLayer := 0
	for size := 16; size >= 1; size /= 2 {
		perLayer += size * size * 4
	}
	renderer.UploadAtlas(1, make([]byte, perLayer))
	renderer.UploadSection(0, 5, 0, make([]byte, 32), make([]byte, 16))
	renderer.DropSection(9, 9, 9)

	frame := RenderFrame{Daylight: 1, SkyColor: [4]float32{0.2, 0.4, 1, 1}}
	for i := 0; i < 4; i++ {
		frame.ViewProj[i*4+i] = 1
		frame.ViewProjInv[i*4+i] = 1
	}
	frame.Visible = [][3]int32{{0, 5, 0}}
	renderer.RenderFrame(frame)
	first := renderer.Readback()
	if len(first) != 32*16*4 {
		t.Fatalf("readback 长度=%d", len(first))
	}
	nonZero := false
	for _, b := range first {
		if b != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Fatal("渲染后回读不应全零")
	}
	renderer.RenderFrame(frame)
	second := renderer.Readback()
	if string(first) != string(second) {
		t.Fatal("同输入两帧必须逐字节一致")
	}
}

func TestRendererPreparedBenchmarkBatchRoundtripOrSkip(t *testing.T) {
	renderer, err := NewRenderer(32, 16)
	if errors.Is(err, ErrNoGPUAdapter) {
		t.Skip("无 GPU 适配器")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()

	frame := RenderFrame{Daylight: 1, SkyColor: [4]float32{0.2, 0.4, 1, 1}}
	for i := 0; i < 4; i++ {
		frame.ViewProj[i*4+i] = 1
		frame.ViewProjInv[i*4+i] = 1
	}
	renderer.PrepareBenchmarkBatch(frame, 1)
	renderer.SubmitBenchmarkBatch()
	if got := renderer.BenchmarkBatchCalls(); got != 2 {
		t.Fatalf("benchmark batch FFI=%d,想要 2", got)
	}
	if got := renderer.FrameCalls(); got != 0 {
		t.Fatalf("prepared batch 调用了 render_frame %d 次", got)
	}
	if len(renderer.Readback()) != 32*16*4 {
		t.Fatal("prepared batch 回读长度错误")
	}
}

// TestDrainUIEventsEmptyAfterCreate 验证 `DrainUIEvents` 的正常路径:新建离屏
// 渲染器尚无菜单事件时返回空切片，并走完 cgo v9 新签名(out_written 字节数 +
// 合法空 batch)的全链路。参数校验由 Rust 层兜底,这里只测「状态码 OK + 空 batch → 空切片」;
// 无 GPU 适配器时跳过。
func TestDrainUIEventsEmptyAfterCreate(t *testing.T) {
	renderer, err := NewRenderer(32, 16)
	if errors.Is(err, ErrNoGPUAdapter) {
		t.Skip("无 GPU 适配器")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()

	events := renderer.DrainUIEvents()
	if len(events) != 0 {
		t.Fatalf("新渲染器 DrainUIEvents 应返回空切片, got %d 个事件", len(events))
	}
}

// TestEncodeRenderFrameLayout 锁定 render_frame ABI 编码布局。
func TestEncodeRenderFrameLayout(t *testing.T) {
	frame := RenderFrame{
		Daylight:       0.5,
		StarVisibility: 0.25,
		CloudMacroX:    7,
		Visible:        [][3]int32{{1, 2, 3}, {-1, 0, -3}},
	}
	out := EncodeRenderFrame(frame)
	if len(out) != renderFrameHeaderBytes+2*12 {
		t.Fatalf("编码长度=%d", len(out))
	}
	if out[184] != 2 {
		t.Fatalf("visible count 编码=%d", out[184])
	}
	// 第二条 record 的 z=-3(小端补码)。
	z := int32(uint32(out[renderFrameHeaderBytes+20]) |
		uint32(out[renderFrameHeaderBytes+21])<<8 |
		uint32(out[renderFrameHeaderBytes+22])<<16 |
		uint32(out[renderFrameHeaderBytes+23])<<24)
	if z != -3 {
		t.Fatalf("visible[1].z=%d", z)
	}
}

// TestEncodeRenderFrameV2Segments 锁定 v2 TLV 段编码:layout 版本、段序
// 与 EncodeQuadSegment 计数。
func TestEncodeRenderFrameV2Segments(t *testing.T) {
	frame := RenderFrame{
		AvatarInstances: make([]byte, 160),
		OverlayStrength: 0.5,
		HUDSegment:      EncodeQuadSegment(make([]byte, 16), make([]byte, 96), nil, 48),
	}
	out := EncodeRenderFrame(frame)
	if got := out[188]; got != 2 {
		t.Fatalf("layout=%d,想要 2", got)
	}
	cursor := renderFrameHeaderBytes
	readU32 := func() uint32 {
		v := uint32(out[cursor]) | uint32(out[cursor+1])<<8 |
			uint32(out[cursor+2])<<16 | uint32(out[cursor+3])<<24
		cursor += 4
		return v
	}
	if tag, length := readU32(), readU32(); tag != 1 || length != 160 {
		t.Fatalf("首段 tag=%d len=%d", tag, length)
	}
	cursor += 160
	if tag, length := readU32(), readU32(); tag != 4 || length != 4 {
		t.Fatalf("overlay 段 tag=%d len=%d", tag, length)
	}
	cursor += 4
	if tag := readU32(); tag != 6 {
		t.Fatalf("HUD 段 tag=%d", tag)
	}
	length := readU32()
	if int(length) != 16+8+96 {
		t.Fatalf("HUD 段 len=%d", length)
	}
	// EncodeQuadSegment 的计数字段:quad 2 个(96/48)、glyph 0。
	if got := out[cursor+16]; got != 2 {
		t.Fatalf("quad count=%d", got)
	}
	// 纯地形帧保持 layout 0。
	plain := EncodeRenderFrame(RenderFrame{})
	if plain[188] != 0 || len(plain) != renderFrameHeaderBytes {
		t.Fatalf("纯地形帧 layout=%d len=%d", plain[188], len(plain))
	}
}

// TestEncodeRenderFrameWaterTintSegment 锁定 v21 水下视觉的帧编码：
// alpha 为 0 时整段缺席且帧与本变更之前逐位一致（纯地形帧仍是 layout 0），
// alpha 大于 0 时追加 tag 8 的 16 字节 RGBA 段。
//
// 两侧对照落在**帧长度与 layout 版本**上：如果水色被写成"总是追加一段全零"，
// 关闭态的帧字节就会变，纯地形帧那条断言当场变红。
func TestEncodeRenderFrameWaterTintSegment(t *testing.T) {
	dry := EncodeRenderFrame(RenderFrame{})
	if dry[188] != 0 || len(dry) != renderFrameHeaderBytes {
		t.Fatalf("不叠加水色时 layout=%d len=%d，想要纯地形帧", dry[188], len(dry))
	}
	// alpha 为 0 的水色同样不得触发 layout 2：RGB 有值但完全透明等于不叠加。
	transparent := EncodeRenderFrame(RenderFrame{WaterTint: [4]float32{0.1, 0.2, 0.3, 0}})
	if len(transparent) != len(dry) {
		t.Fatalf("alpha=0 的水色改变了帧长度：%d vs %d", len(transparent), len(dry))
	}

	tint := [4]float32{0.12, 0.34, 0.52, 0.45}
	out := EncodeRenderFrame(RenderFrame{WaterTint: tint})
	if out[188] != 2 {
		t.Fatalf("叠加水色时 layout=%d，想要 2", out[188])
	}
	if len(out) != renderFrameHeaderBytes+8+16 {
		t.Fatalf("帧长度=%d，想要头部 + 一个 8 字节 TLV 头 + 16 字节负载", len(out))
	}
	cursor := renderFrameHeaderBytes
	readU32 := func() uint32 {
		value := binary.LittleEndian.Uint32(out[cursor:])
		cursor += 4
		return value
	}
	if tag, length := readU32(), readU32(); tag != frameTagWater || length != 16 {
		t.Fatalf("水色段 tag=%d len=%d，想要 %d/16", tag, length, frameTagWater)
	}
	for index, want := range tint {
		got := math.Float32frombits(binary.LittleEndian.Uint32(out[cursor+index*4:]))
		if got != want {
			t.Fatalf("水色分量 %d = %v，想要 %v", index, got, want)
		}
	}
}
