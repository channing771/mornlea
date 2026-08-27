//go:build darwin

package client

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

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

// TestHasPassSegmentsIncludesUISegment 守住 UI 段计入 pass 段:UISegment 非空即
// 切到 layout v2,否则 UI 段永不进 TLV。
func TestHasPassSegmentsIncludesUISegment(t *testing.T) {
	var frame RenderFrame
	if frame.hasPassSegments() {
		t.Fatal("空帧不应携带 pass 段")
	}
	frame.UISegment = []byte{1, 2, 3}
	if !frame.hasPassSegments() {
		t.Fatal("UISegment 非空应计入 pass 段(切到 layout v2)")
	}
}

// TestEncodeRenderFrameUISegment 守住 UI 段编码为帧尾 TLV(tag 9,water 之后),
// 负载与 EncodeUIMenu 产物逐字一致。
func TestEncodeRenderFrameUISegment(t *testing.T) {
	menu := EncodeUIMenu(UIMenu{
		Visible: true,
		Title:   "Mornlea",
		Version: "dev",
		Buttons: []UIButton{{ID: 1, Label: "进入游戏", Enabled: true}},
	})
	out := EncodeRenderFrame(RenderFrame{UISegment: menu})
	if out[188] != 2 {
		t.Fatalf("UI 段帧 layout=%d, want 2", out[188])
	}
	cursor := renderFrameHeaderBytes
	readU32 := func() uint32 {
		v := binary.LittleEndian.Uint32(out[cursor:])
		cursor += 4
		return v
	}
	var lastTag, lastLen uint32
	for cursor < len(out) {
		lastTag = readU32()
		lastLen = readU32()
		cursor += int(lastLen)
	}
	if lastTag != frameTagUI {
		t.Fatalf("末段 tag=%d, want %d", lastTag, frameTagUI)
	}
	if int(lastLen) != len(menu) {
		t.Fatalf("UI 段 len=%d, want %d", lastLen, len(menu))
	}
	payload := out[len(out)-int(lastLen):]
	if string(payload) != string(menu) {
		t.Fatal("UI 段负载与 EncodeUIMenu 产物不一致")
	}
}

// TestEncodeRenderFrameUISegmentPreservesRealLength 守住 EncodeRenderFrame 对 UI 段的
// 字节透传:真实菜单内容(四按钮 + 中文 error)编码后非 4 对齐(142 字节),TLV 长度
// 字段必须等于实际载荷字节数——禁止未来偷偷填充到 4 对齐。跨语言侧 Rust 的 parse_frame
// 已豁免 UI 段(FRAME_TAG_UI)的 4 对齐检查,这里锁 Go 侧原样透传、不填充。
func TestEncodeRenderFrameUISegmentPreservesRealLength(t *testing.T) {
	menu := EncodeUIMenu(UIMenu{
		Visible: true,
		Title:   "Mornlea",
		Version: "dev",
		Error:   "存档无法打开",
		Buttons: []UIButton{
			{ID: 1, Label: "进入游戏", Enabled: true},
			{ID: 2, Label: "多人游戏", Enabled: false},
			{ID: 3, Label: "设置", Enabled: false},
			{ID: 4, Label: "退出游戏", Enabled: true},
		},
	})
	if len(menu) != 142 || len(menu)%4 == 0 {
		t.Fatalf("夹具长度=%d(%%4=%d), 应为 142 且非 4 对齐", len(menu), len(menu)%4)
	}
	out := EncodeRenderFrame(RenderFrame{UISegment: menu})
	cursor := renderFrameHeaderBytes
	var lastTag, lastLen uint32
	for cursor < len(out) {
		lastTag = binary.LittleEndian.Uint32(out[cursor:])
		cursor += 4
		lastLen = binary.LittleEndian.Uint32(out[cursor:])
		cursor += 4
		cursor += int(lastLen)
	}
	if lastTag != frameTagUI {
		t.Fatalf("末段 tag=%d, want %d", lastTag, frameTagUI)
	}
	if int(lastLen) != len(menu) {
		t.Fatalf("UI 段 TLV 长度=%d, 载荷=%d(应为真实字节长度,非 4 对齐也原样,禁止填充)", lastLen, len(menu))
	}
	if string(out[len(out)-int(lastLen):]) != string(menu) {
		t.Fatal("UI 段负载与 EncodeUIMenu 产物不一致")
	}
}
