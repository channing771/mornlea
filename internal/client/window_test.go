//go:build darwin

package client

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

// buildSnapshot 构造一份合法快照字节,便于无头验证解码与缓存语义。
func buildSnapshot(mutate func([]byte)) []byte {
	buf := make([]byte, snapshotBytes)
	binary.LittleEndian.PutUint32(buf[0:4], snapshotLayoutVersion)
	if mutate != nil {
		mutate(buf)
	}
	return buf
}

func TestClientABIVersionMatchesHeader(t *testing.T) {
	// v13:新增窗口合成捕获出口 window_capture(两段式 BGRA8,溢出与
	// 捕获不可用独立状态);v12:菜单层迁 WebView——退役字体上传出口与
	// 帧 tag 9 段,新增 ui_push_state 状态下行,drain 改版本化 JSON 信封;
	// v11:离屏 benchmark batch prepare/submit 入口。导出版本与编译期
	// header 常量必须逐位一致:加载低于 v13 的旧动态库会在首个 FFI 入口
	// 被稳定拒绝（STATUS_ABI_VERSION），不产生半启动。
	if got := ClientABIVersion(); got != 13 {
		t.Fatalf("client ABI version=%d,想要 13", got)
	}
	if got := clientABIHeaderVersion(); got != 13 {
		t.Fatalf("client header ABI version=%d,想要 13", got)
	}
}

func TestWindowCapturePanicsWithStableTextOnUnknownHandle(t *testing.T) {
	// 零值 Window 的句柄在 Rust 侧 thread-local 表中不存在:捕获必须以
	// 稳定中文文案 panic,不得返回部分结果或让 Rust 状态泄漏为裸错误。
	defer func() {
		value := recover()
		if value == nil {
			t.Fatal("无效句柄的捕获必须 panic")
		}
		text, ok := value.(string)
		if !ok || !strings.Contains(text, "client 窗口句柄无效或窗口操作失败") {
			t.Fatalf("panic=%v,缺少稳定状态文案", value)
		}
	}()
	var window Window
	window.Capture()
}

func TestWindowMapsBackspaceAndReportsTextOverflow(t *testing.T) {
	// 键位 iota 稳定性:位序即快照 bitmask 契约,Backspace 必须保持在末尾。
	if KeyBackspace != KeyLeftAlt+1 || int(KeyBackspace) != 28 {
		t.Fatalf("Backspace iota=%d", KeyBackspace)
	}
	var window Window
	for range 1024 {
		window.enqueueTextInput('a')
	}
	window.enqueueTextInput('b')
	got, overflow := window.DrainTextInput(make([]rune, 0, 1024))
	if len(got) != 1024 || !overflow || got[1023] != 'a' {
		t.Fatalf("DrainTextInput len=%d overflow=%v tail=%q", len(got), overflow, got[1023])
	}
	if got, overflow := window.DrainTextInput(got[:0]); len(got) != 0 || overflow {
		t.Fatalf("second drain len=%d overflow=%v", len(got), overflow)
	}
}

func TestDecodeSnapshotCachesAllFields(t *testing.T) {
	buf := buildSnapshot(func(buf []byte) {
		binary.LittleEndian.PutUint32(buf[4:8], 1) // should_close
		binary.LittleEndian.PutUint64(buf[8:16], 1<<KeyW|1<<KeyBackspace)
		binary.LittleEndian.PutUint32(buf[16:20], 0b11)
		binary.LittleEndian.PutUint32(buf[20:24], 2)
		binary.LittleEndian.PutUint64(buf[24:32], math.Float64bits(12.5))
		binary.LittleEndian.PutUint64(buf[32:40], math.Float64bits(-3.0))
		binary.LittleEndian.PutUint32(buf[40:44], 2560)
		binary.LittleEndian.PutUint32(buf[44:48], 1440)
		binary.LittleEndian.PutUint32(buf[48:52], 1280)
		binary.LittleEndian.PutUint32(buf[52:56], 720)
		binary.LittleEndian.PutUint32(buf[64:68], uint32('你'))
		binary.LittleEndian.PutUint32(buf[68:72], uint32('A'))
	})

	var window Window
	window.state = decodeSnapshot(buf, window.enqueueTextInput, func() { window.textInputOverflow = true })

	if !window.ShouldClose() {
		t.Fatal("should_close 标志未解码")
	}
	if !window.KeyDown(KeyW) || !window.KeyDown(KeyBackspace) || window.KeyDown(KeyA) {
		t.Fatal("键位 bitmask 解码错误")
	}
	if !window.PrimaryButtonDown() || !window.SecondaryButtonDown() {
		t.Fatal("鼠标键位解码错误")
	}
	if x, y := window.CursorPos(); x != 12.5 || y != -3.0 {
		t.Fatalf("光标位置=(%f,%f)", x, y)
	}
	if w, h := window.FramebufferSize(); w != 2560 || h != 1440 {
		t.Fatalf("framebuffer=(%d,%d)", w, h)
	}
	if w, h := window.ContentSize(); w != 1280 || h != 720 {
		t.Fatalf("content=(%d,%d)", w, h)
	}
	text, overflow := window.DrainTextInput(nil)
	if string(text) != "你A" || overflow {
		t.Fatalf("text=%q overflow=%v", string(text), overflow)
	}
}

func TestDecodeSnapshotOverflowFlagSetsQueueOverflow(t *testing.T) {
	buf := buildSnapshot(func(buf []byte) {
		binary.LittleEndian.PutUint32(buf[4:8], 2) // text_overflow
	})
	var window Window
	window.state = decodeSnapshot(buf, window.enqueueTextInput, func() { window.textInputOverflow = true })
	if _, overflow := window.DrainTextInput(nil); !overflow {
		t.Fatal("快照 overflow 标志必须传导到队列")
	}
}

func TestDecodeSnapshotRejectsMalformedInput(t *testing.T) {
	for name, buf := range map[string][]byte{
		"短缓冲":  make([]byte, snapshotBytes-1),
		"布局版本": buildSnapshot(func(buf []byte) { binary.LittleEndian.PutUint32(buf[0:4], 2) }),
		"文本计数": buildSnapshot(func(buf []byte) { binary.LittleEndian.PutUint32(buf[20:24], textCapacity+1) }),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("非法快照必须 panic")
				}
			}()
			decodeSnapshot(buf, func(rune) {}, func() {})
		})
	}
}
