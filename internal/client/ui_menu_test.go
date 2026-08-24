//go:build darwin

package client

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
)

// goldenFourButtonFrame 是 Rust decode_ui_frame 金色夹具的字节(小端),来源:
// engine/crates/mornlea_client/src/ui.rs 测试的 four_button_frame()(Task 1 实现,
// 由 decode_four_button_with_enabled_fields_exact 正向解码)。用于把 Go
// EncodeUIMenu 与 Rust 解码端逐字节交叉锁定——任一端字节布局漂移都会让
// 本测试与 Rust 端的 decode 测试同时变红。
const goldenFourButtonFrame = "01000000" +
	"01000000" +
	"04000000" +
	"01000000" + "0c000000" + "e8bf9be585a5e6b8b8e6888f" + "01000000" +
	"02000000" + "0c000000" + "e5a49ae4babae6b8b8e6888f" + "00000000" +
	"03000000" + "06000000" + "e8aebee7bdae" + "00000000" +
	"04000000" + "0c000000" + "e98080e587bae6b8b8e6888f" + "01000000" +
	"07000000" + "4d6f726e6c6561" +
	"03000000" + "646576" +
	"00000000"

// TestEncodeUIMenuCrossLanguageGolden 与 Rust decode_ui_frame 金色夹具交叉锁定:
// 断言 Go EncodeUIMenu 对同一菜单逐字节产出与 Rust 侧 four_button_frame() 相同字节。
func TestEncodeUIMenuCrossLanguageGolden(t *testing.T) {
	want, err := hex.DecodeString(goldenFourButtonFrame)
	if err != nil {
		t.Fatalf("golden hex 解码失败: %v", err)
	}
	got := EncodeUIMenu(UIMenu{
		Visible: true,
		Title:   "Mornlea",
		Version: "dev",
		Buttons: []UIButton{
			{ID: 1, Label: "进入游戏", Enabled: true},
			{ID: 2, Label: "多人游戏", Enabled: false},
			{ID: 3, Label: "设置", Enabled: false},
			{ID: 4, Label: "退出游戏", Enabled: true},
		},
	})
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeUIMenu 与 Rust 金色字节不一致:\n got=%x\nwant=%x", got, want)
	}
	if len(got) != len(want) {
		t.Fatalf("长度=%d, want %d", len(got), len(want))
	}
}

// TestEncodeUIMenuFourButtonsWithErrorFieldPositions 逐字段位置断言(小端 u32):
// 四按钮混合启用 + 中文错误行,验证每个 u32 字段落位与字节宽度正确。
func TestEncodeUIMenuFourButtonsWithErrorFieldPositions(t *testing.T) {
	got := EncodeUIMenu(UIMenu{
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
	if len(got) != 142 {
		t.Fatalf("长度=%d, want 142", len(got))
	}
	assertU32At(t, got, 0, 1)                     // layout
	assertU32At(t, got, 4, uint32(uiFlagVisible)) // flags
	assertU32At(t, got, 8, 4)                     // button_count
	assertU32At(t, got, 12, 1)                    // button1 id
	assertU32At(t, got, 16, 12)                   // button1 label_len
	if string(got[20:32]) != "进入游戏" {
		t.Fatalf("button1 label 字节错误: %q", got[20:32])
	}
	assertU32At(t, got, 32, 1)  // button1 enabled
	assertU32At(t, got, 36, 2)  // button2 id
	assertU32At(t, got, 40, 12) // button2 label_len
	if string(got[44:56]) != "多人游戏" {
		t.Fatalf("button2 label 字节错误: %q", got[44:56])
	}
	assertU32At(t, got, 56, 0) // button2 enabled
	assertU32At(t, got, 60, 3) // button3 id
	assertU32At(t, got, 64, 6) // button3 label_len
	if string(got[68:74]) != "设置" {
		t.Fatalf("button3 label 字节错误: %q", got[68:74])
	}
	assertU32At(t, got, 74, 0)  // button3 enabled
	assertU32At(t, got, 78, 4)  // button4 id
	assertU32At(t, got, 82, 12) // button4 label_len
	if string(got[86:98]) != "退出游戏" {
		t.Fatalf("button4 label 字节错误: %q", got[86:98])
	}
	assertU32At(t, got, 98, 1)  // button4 enabled
	assertU32At(t, got, 102, 7) // title_len
	if string(got[106:113]) != "Mornlea" {
		t.Fatalf("title 字节错误: %q", got[106:113])
	}
	assertU32At(t, got, 113, 3) // version_len
	if string(got[117:120]) != "dev" {
		t.Fatalf("version 字节错误: %q", got[117:120])
	}
	assertU32At(t, got, 120, 18) // error_len
	if string(got[124:142]) != "存档无法打开" {
		t.Fatalf("error 字节错误: %q", got[124:142])
	}
}

func assertU32At(t *testing.T, b []byte, offset int, want uint32) {
	t.Helper()
	got := binary.LittleEndian.Uint32(b[offset : offset+4])
	if got != want {
		t.Fatalf("offset %d u32 = %d, want %d", offset, got, want)
	}
}

// TestEncodeUIMenuMinimal 最小菜单(visible=false、无按钮、空串字段):仅
// layout+flags+button_count+三个长度字段共六个 u32(24 字节)。
func TestEncodeUIMenuMinimal(t *testing.T) {
	got := EncodeUIMenu(UIMenu{})
	want := []byte{
		1, 0, 0, 0, // layout
		0, 0, 0, 0, // flags(不可见)
		0, 0, 0, 0, // button_count
		0, 0, 0, 0, // title_len
		0, 0, 0, 0, // version_len
		0, 0, 0, 0, // error_len
	}
	if len(got) != 24 {
		t.Fatalf("最小帧长度=%d, want 24", len(got))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("最小帧字节=%x, want %x", got, want)
	}
}

// TestEncodeUIMenuMaximalValid 恰好处于各上界(8 按钮、label 64 字节、title 128、
// version 64、error 256)应正常编码,不 panic。
func TestEncodeUIMenuMaximalValid(t *testing.T) {
	buttons := make([]UIButton, 8)
	for i := range buttons {
		buttons[i] = UIButton{ID: uint32(i + 1), Label: strings.Repeat("a", 64), Enabled: true}
	}
	menu := UIMenu{
		Visible: true,
		Title:   strings.Repeat("t", 128),
		Version: strings.Repeat("v", 64),
		Error:   strings.Repeat("e", 256),
		Buttons: buttons,
	}
	got := EncodeUIMenu(menu)
	wantLen := 3*4 + 8*(4+4+64+4) + (4 + 128) + (4 + 64) + (4 + 256)
	if len(got) != wantLen {
		t.Fatalf("最大帧长度=%d, want %d", len(got), wantLen)
	}
}

// TestEncodeUIMenuPanicsOutOfBounds 越界是编程错误,必须 panic(与既有段落编码
// 口径一致):按钮 >8、label >64 字节、title >128、version >64、error >256。
func TestEncodeUIMenuPanicsOutOfBounds(t *testing.T) {
	cases := []struct {
		name string
		menu UIMenu
	}{
		{"9 按钮", UIMenu{Buttons: make([]UIButton, 9)}},
		{"65 字节 label", UIMenu{Buttons: []UIButton{{ID: 1, Label: strings.Repeat("a", 65)}}}},
		{"129 字节 title", UIMenu{Title: strings.Repeat("t", 129)}},
		{"65 字节 version", UIMenu{Version: strings.Repeat("v", 65)}},
		{"257 字节 error", UIMenu{Error: strings.Repeat("e", 257)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mustPanic(t, tc.name, func() { EncodeUIMenu(tc.menu) })
		})
	}
}

func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s: 必须触发 panic", name)
		}
	}()
	fn()
}
