//go:build darwin

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/sim"
)

// selectFieldForTest 按 Group+"."+Name 定位字段并设置 selected，找不到时 Fatal。
// 与 internal/client mesher_test.go 的 BlockForTest 等一脉相承的
// "XxxForTest" 测试专用辅助方法命名约定。
func (s *panelState) selectFieldForTest(t *testing.T, name string) {
	t.Helper()
	for i, field := range config.Fields() {
		if field.Group+"."+field.Name == name {
			s.selected = i
			return
		}
	}
	t.Fatalf("未找到字段 %s", name)
}

// dataRowsForTest 从 rows() 输出里过滤掉段头行（panelSectionHeaderRow：
// ReadOnly 且 Value 为空），返回与 config.Fields() 顺序一一对应的数据行。
// rows() 现在按 Group+"."+Name 与 config.Fields() 下标是一一对应的关系
// 不再成立——rows() 里插入了分组段头，标签也从 "Group.Name" 改成裸
// field.Name（见 Finding 4：列宽/rune 数不匹配导致标签在 180px 列宽下
// 越界，改成"段头 + 裸字段名"而不是继续拉长每行标签）——测试要按
// field.Group/Name 断言，只能先把段头过滤掉，再按下标对齐 config.Fields()。
func dataRowsForTest(t *testing.T, rows []render.PanelRow) []render.PanelRow {
	t.Helper()
	data := make([]render.PanelRow, 0, len(rows))
	for _, row := range rows {
		if row.ReadOnly && row.Value == "" {
			continue // 段头行
		}
		data = append(data, row)
	}
	fields := config.Fields()
	if len(data) != len(fields) {
		t.Fatalf("data rows = %d, want %d(=len(config.Fields()))", len(data), len(fields))
	}
	return data
}

// selectedRowForTest 返回 rows 中被标记 Selected 的那一行，找不到时 Fatal。
// rows() 插入段头后，state.selected（config.Fields() 下标）不再等于
// rows() 切片自身的下标，不能再直接 rows[state.selected]。
func selectedRowForTest(t *testing.T, rows []render.PanelRow) render.PanelRow {
	t.Helper()
	for _, row := range rows {
		if row.Selected {
			return row
		}
	}
	t.Fatal("没有任何一行被标记为 Selected")
	return render.PanelRow{}
}

func TestPanelRowsMarkAuthoritativeGroupsReadOnlyWhenRemote(t *testing.T) {
	state := newPanelState(config.Defaults())
	fields := config.Fields()
	rows := dataRowsForTest(t, state.rows(true))
	for i, field := range fields {
		if field.Group == "physics" || field.Group == "sim" {
			if !rows[i].ReadOnly {
				t.Fatalf("联机时 %s.%s 必须只读", field.Group, field.Name)
			}
		}
	}
}

func TestPanelRowsAllowAuthoritativeGroupsWhenLocal(t *testing.T) {
	state := newPanelState(config.Defaults())
	fields := config.Fields()
	rows := dataRowsForTest(t, state.rows(false))
	editable := 0
	for i, field := range fields {
		if field.Group == "physics" && !rows[i].ReadOnly {
			editable++
		}
	}
	if editable == 0 {
		t.Fatal("单机时物理组必须可编辑")
	}
}

func TestPanelViewDistanceIsAlwaysReadOnly(t *testing.T) {
	state := newPanelState(config.Defaults())
	fields := config.Fields()
	viewDistanceIndex := -1
	for i, field := range fields {
		if field.Group == "render" && field.Name == "viewDistance" {
			viewDistanceIndex = i
		}
	}
	if viewDistanceIndex < 0 {
		t.Fatal("config.Fields() 缺少 render.viewDistance")
	}
	for _, remote := range []bool{false, true} {
		rows := dataRowsForTest(t, state.rows(remote))
		if !rows[viewDistanceIndex].ReadOnly {
			t.Fatalf("viewDistance 在 remote=%v 下也必须只读（重启生效）", remote)
		}
	}
}

// TestPanelRowsInsertSectionHeadersWithoutGroupPrefix 锁住 Finding 4 的修法：
// 标签列宽 170px 装不下"分组前缀+字段名"（最长的 sim.playerDropPickupDelayTicks
// 有 30 个 ASCII 字符），改成每组一个段头行 + 裸字段名（最长
// playerDropPickupDelayTicks 仍有 26 个字符，但至少不再需要在每一行里
// 重复分组名）。
func TestPanelRowsInsertSectionHeadersWithoutGroupPrefix(t *testing.T) {
	state := newPanelState(config.Defaults())
	rows := state.rows(false)
	headers := 0
	for _, row := range rows {
		if strings.Contains(row.Label, ".") {
			t.Fatalf("裸标签不应再带分组前缀: %q", row.Label)
		}
		if row.ReadOnly && row.Value == "" {
			headers++
		}
	}
	if headers != 3 {
		t.Fatalf("段头行数 = %d, want 3（physics/sim/render 各一个）", headers)
	}
	if len(rows) != len(config.Fields())+3 {
		t.Fatalf("rows 总数 = %d, want %d", len(rows), len(config.Fields())+3)
	}
}

// TestPanelSelectedRowTracksSelectedField 钉住"高亮行就是 Fields()[selected]
// 那一行"。
//
// rows() 会插入三个段头行，因此 s.selected（Fields() 下标）与 rows 切片下标
// 不再一一对应。既有测试只断言"存在一行被标记 Selected"或"被选中的行不是
// 只读行"，用 rows 自身的下标去标记 Selected 的实现（高亮行随段头逐组下移）
// 一样能通过——面板上高亮的是一个字段，方向键改的却是另一个。
func TestPanelSelectedRowTracksSelectedField(t *testing.T) {
	state := newPanelState(config.Defaults())
	for _, name := range []string{"physics.gravity", "sim.spawnRadius", "render.fovDegrees"} {
		t.Run(name, func(t *testing.T) {
			state.selectFieldForTest(t, name)
			want := name[strings.Index(name, ".")+1:]
			if got := selectedRowForTest(t, state.rows(false)).Label; got != want {
				t.Fatalf("高亮行 Label = %q，want %q（高亮必须落在 config.Fields()[selected] 上，"+
					"不能被段头行挤偏）", got, want)
			}
		})
	}
}

func TestPanelFrameInputIsAllocationFreeWhenHidden(t *testing.T) {
	app := &application{panel: newPanelState(config.Defaults())}
	now := time.Now()
	if allocations := testing.AllocsPerRun(100, func() {
		app.panelFrameInput(now)
	}); allocations != 0 {
		t.Fatalf("面板关闭时每帧分配 %v 次；读数与参数行必须在 visible 判定之后才构造", allocations)
	}

	app.panel.visible = true
	readout, rows := app.panelFrameInput(now)
	if len(rows) != len(config.Fields())+3 {
		t.Fatalf("面板打开时参数行 = %d，want %d（含三个段头行）", len(rows), len(config.Fields())+3)
	}
	if readout.Mode == "" {
		t.Fatal("面板打开时必须填出运行模式读数")
	}
}

func TestPanelToggleDoesNotReportChanged(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.handleKeys(panelKeys{Toggle: true})
	if !state.visible {
		t.Fatal("Toggle 必须切换可见性")
	}
	state.handleKeys(panelKeys{Toggle: true})
	if state.visible {
		t.Fatal("再次 Toggle 必须隐藏面板")
	}
}

func TestPanelToggleClearsEditing(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.gravity")
	state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionEnterEdit}}, false)
	if !state.editing {
		t.Fatal("应处于编辑态")
	}
	state.handleKeys(panelKeys{Toggle: true})
	if state.editing {
		t.Fatal("面板关闭必须清空编辑态")
	}
}

// TestApplyPanelChangeWritesCameraFovY 证明面板改 FOV 会同时写 a.camera.FovY，
// 而不是只改 a.render.FovDegrees。a.camera.FovY 在构造相机时被一次性烘焙、
// 之后不再每帧重读（不同于鼠标灵敏度那样每帧读 a.render），单靠肉眼看不出
// 遗漏，必须有测试锁住。
func TestApplyPanelChangeWritesCameraFovY(t *testing.T) {
	originalPhysics := physics.ActiveTunables()
	originalSim := sim.ActiveTunables()
	t.Cleanup(func() {
		physics.SetTunables(originalPhysics)
		sim.SetTunables(originalSim)
	})

	app := &application{panel: newPanelState(config.Defaults())}
	app.panel.effective.Render.FovDegrees = 42
	app.applyPanelChange()

	want := mgl32.DegToRad(42)
	if app.camera.FovY != want {
		t.Fatalf("camera.FovY = %v, want %v（FOV 未写回相机）", app.camera.FovY, want)
	}
	if app.render.FovDegrees != 42 {
		t.Fatalf("a.render.FovDegrees = %v, want 42", app.render.FovDegrees)
	}
}

func TestPanelRemoteRejectsAuthoritativeEnterEdit(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.gravity")
	state.applyPanelEvents([]debugPanelEvent{{
		action: client.DebugPanelActionEnterEdit,
	}}, true)
	if state.editing {
		t.Fatal("联机时不得进入编辑态")
	}
	state.applyPanelEvents([]debugPanelEvent{{
		action: client.DebugPanelActionConfirm, value: "99",
	}}, false)
	if state.effective.Physics.Gravity == 99 {
		t.Fatalf("未进入编辑态的 confirm 不得改值：%v", state.effective.Physics.Gravity)
	}
}

// encodeDebugPanelSegmentGolden 按 Rust debug_abi_tests 的 valid_frame() 夹具
// 手工拼出期望字节，逐字节锁定 layout v3 的字段序与定宽记录布局。
func TestEncodeDebugPanelSegmentCrossLanguageGolden(t *testing.T) {
	readout := render.PanelReadout{
		FrameMillis:  12.5,
		Position:     mgl32.Vec3{10, 64, -3},
		Yaw:          45,
		Pitch:        -12,
		Tick:         1234,
		WorldTime:    42,
		LoadedChunks: 137,
		Mode:         "单机",
	}
	rows := []render.PanelRow{
		{Label: "── physics ──", ReadOnly: true},
		{Label: "gravity", Value: "9.8", Selected: true},
		{Label: "fovDegrees", Value: "70"},
	}
	got := encodeDebugPanelSegment(true, true, readout, rows)

	fixed24 := func(value string) []byte {
		out := make([]byte, 24)
		copy(out, value)
		return out
	}
	u32 := func(value uint32) []byte { return binary.LittleEndian.AppendUint32(nil, value) }
	f32 := func(value float32) []byte {
		return binary.LittleEndian.AppendUint32(nil, math.Float32bits(value))
	}
	f64 := func(value float64) []byte {
		return binary.LittleEndian.AppendUint64(nil, math.Float64bits(value))
	}
	appendRow := func(out []byte, label, value string, flags uint32, edit []byte) []byte {
		out = append(out, fixed24(label)...)
		out = append(out, fixed24(value)...)
		out = append(out, u32(flags)...)
		return append(out, edit...)
	}
	want := u32(3)                    // layout
	want = append(want, u32(1)...)    // flags: visible
	want = append(want, f64(12.5)...) // frame_millis
	want = append(want, f32(10)...)
	want = append(want, f32(64)...)
	want = append(want, f32(-3)...)
	want = append(want, f32(45)...)                                     // yaw
	want = append(want, f32(-12)...)                                    // pitch
	want = append(want, binary.LittleEndian.AppendUint64(nil, 1234)...) // tick
	want = append(want, binary.LittleEndian.AppendUint64(nil, 42)...)   // world_time
	want = append(want, u32(137)...)                                    // loaded_chunks
	want = append(want, u32(6)...)
	want = append(want, "单机"...)
	want = append(want, u32(3)...) // row_count
	want = appendRow(want, "── physics ──", "", 1, nil)
	edit := u32(3)
	edit = append(edit, "9.8"...)
	edit = append(edit, u32(3)...)
	want = appendRow(want, "gravity", "9.8", 2+4+8, edit)
	want = appendRow(want, "fovDegrees", "70", 4, nil)
	if !bytes.Equal(got, want) {
		t.Fatalf("layout v3 段字节不一致:\n got=%x\nwant=%x", got, want)
	}
}

// rowsOffset 返回段头之后第一条行记录的字节偏移：常数部分 56 字节
// （layout 4 + flags 4 + frame_millis 8 + position 3×f32 12 + yaw 4 +
// pitch 4 + tick 8 + world_time 8 + loaded_chunks 4），加 mode
// （4 字节长度 + 文本）与 row_count 4。
func rowsOffset(mode string) int { return 56 + 4 + len(mode) + 4 }

func TestEncodeDebugPanelSegmentUTF8Truncation(t *testing.T) {
	label := "一二三四五六七八九十" // 10 个 CJK = 30 字节 > 24
	value := "一二三四五六七八九"  // 9 个 CJK = 27 字节 > 24
	got := encodeDebugPanelSegment(true, false, render.PanelReadout{Mode: "单机"}, []render.PanelRow{
		{Label: label, Value: value},
	})
	offset := rowsOffset("单机")
	label24 := got[offset : offset+24]
	labelEnd := bytes.IndexByte(label24, 0)
	if labelEnd < 0 {
		labelEnd = 24
	}
	wantLabel := "一二三四五六七八" // 8 个 CJK，恰好 24 字节
	if string(label24[:labelEnd]) != wantLabel {
		t.Fatalf("标签截断=%q, want %q", string(label24[:labelEnd]), wantLabel)
	}
	value24 := got[offset+24 : offset+48]
	valueEnd := bytes.IndexByte(value24, 0)
	if valueEnd < 0 {
		valueEnd = 24
	}
	if !utf8.Valid(value24[:valueEnd]) {
		t.Fatalf("值字段截断后不是合法 UTF-8: %x", value24[:valueEnd])
	}
	// 零填充：首个 NUL 之后必须全零。
	if !allZero(value24[valueEnd:]) || !allZero(label24[labelEnd:]) {
		t.Fatal("定宽字段首个 NUL 之后必须全零")
	}
}

func allZero(bytes []byte) bool {
	for _, b := range bytes {
		if b != 0 {
			return false
		}
	}
	return true
}

// TestEncodeDebugPanelSegmentEditState 锁住编辑态：行置 editing 位、edit_value 是
// 原值的截断播种、edit_cursor 是末位字节偏移（=len）。
func TestEncodeDebugPanelSegmentEditState(t *testing.T) {
	got := encodeDebugPanelSegment(true, true, render.PanelReadout{Mode: "单机"}, []render.PanelRow{
		{Label: "gravity", Value: "9.8", Selected: true},
	})
	offset := rowsOffset("单机")
	flags := binary.LittleEndian.Uint32(got[offset+48 : offset+52])
	if flags&8 == 0 {
		t.Fatalf("编辑行必须置 editing 位: flags=%#x", flags)
	}
	if flags&4 == 0 {
		t.Fatalf("编辑行必须同时置 editable 位: flags=%#x", flags)
	}
	editLen := binary.LittleEndian.Uint32(got[offset+52 : offset+56])
	if editLen != 3 {
		t.Fatalf("edit_value 长度=%d, want 3", editLen)
	}
	if value := string(got[offset+56 : offset+56+int(editLen)]); value != "9.8" {
		t.Fatalf("edit_value=%q, want 9.8", value)
	}
	cursor := binary.LittleEndian.Uint32(got[offset+56+int(editLen) : offset+60+int(editLen)])
	if cursor != editLen {
		t.Fatalf("edit_cursor=%d, want 末位 %d", cursor, editLen)
	}
}

// TestEncodeDebugPanelSegmentSelectedReadOnlyNotFlagged 锁住"selected→!readonly"：
// 联机时 physics 行被 rows() 标为只读，编码器绝不能在同一个行上同时置选中位
// （Rust 拒绝该行 flag 组合，整个段会被丢）。
func TestEncodeDebugPanelSegmentSelectedReadOnlyNotFlagged(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.selectFieldForTest(t, "physics.eyeHeight")
	rows := state.rows(true)
	got := encodeDebugPanelSegment(true, false, render.PanelReadout{Mode: "联机"}, rows)
	offset := rowsOffset("联机")
	for i := range rows {
		flags := binary.LittleEndian.Uint32(got[offset+i*52+48 : offset+i*52+52])
		if flags&2 != 0 && flags&1 != 0 {
			t.Fatalf("row[%d] 同时置 selected 与 readonly: flags=%#x", i, flags)
		}
	}
}

// TestEncodeDebugPanelSegmentHiddenIsNil 锁住"面板关闭时不产出段"（Rust 渲染
// 零工作，与 design §6.1 的整 pass 跳过同一条要求）。
func TestEncodeDebugPanelSegmentHiddenIsNil(t *testing.T) {
	if segment := encodeDebugPanelSegment(false, false, render.PanelReadout{Mode: "单机"},
		[]render.PanelRow{{Label: "x", Value: "1"}}); segment != nil {
		t.Fatalf("隐藏面板不得产出段: %x", segment)
	}
}

// TestEncodeDebugPanelSegmentWithinBudget 锁住段长上界：64 行参数（含段头）
// 与一个编辑行也不超过 MAX_UI_SEGMENT_BYTES。
func TestEncodeDebugPanelSegmentWithinBudget(t *testing.T) {
	rows := make([]render.PanelRow, 64)
	for i := range rows {
		rows[i] = render.PanelRow{Label: fmt.Sprintf("field %02d", i), Value: "123.456", Selected: i == 0}
	}
	segment := encodeDebugPanelSegment(true, true, render.PanelReadout{Mode: "benchmark"}, rows)
	if len(segment) > maxUISegmentBytes {
		t.Fatalf("段长=%d, 上界 %d", len(segment), maxUISegmentBytes)
	}
}

func TestPanelApplyEventsSelectMovesSelection(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.eyeHeight")
	state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionSelectNext}}, false)
	if state.selected != 1 {
		t.Fatalf("SELECT_NEXT 后 selected=%d, want 1", state.selected)
	}
	state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionSelectPrev}}, false)
	if state.selected != 0 {
		t.Fatalf("SELECT_PREV 后 selected=%d, want 0", state.selected)
	}
}

// TestPanelApplyEventsNavigationSkipsReadOnlyRows 锁住"方向键只落在可编辑行上"
// （联机时 physics/sim 全只读，导航直接跳过它们）。
func TestPanelApplyEventsNavigationSkipsReadOnlyRows(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selected = 0
	for i := 0; i < 200; i++ {
		state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionSelectNext}}, true)
		rows := dataRowsForTest(t, state.rows(true))
		if rows[state.selected].ReadOnly {
			t.Fatal("导航必须跳过只读行")
		}
	}
}

func TestPanelApplyEventsEnterEditConfirmWritesBack(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.gravity")
	initial := state.effective.Physics.Gravity

	changed := state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionEnterEdit}}, false)
	if changed || !state.editing {
		t.Fatalf("进入编辑不算配置变更: changed=%v editing=%v", changed, state.editing)
	}
	changed = state.applyPanelEvents([]debugPanelEvent{{
		action: client.DebugPanelActionConfirm, value: "12.5",
	}}, false)
	if !changed {
		t.Fatal("确认合法值必须报告变更")
	}
	if state.effective.Physics.Gravity != 12.5 {
		t.Fatalf("gravity=%v, want 12.5", state.effective.Physics.Gravity)
	}
	if state.editing {
		t.Fatal("确认后必须退出编辑态")
	}
	if state.effective.Physics.Gravity == initial {
		t.Fatal("编辑前重力值不应残留")
	}
}

func TestPanelApplyEventsConfirmClampsOutOfRange(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "sim.spawnRadius")
	state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionEnterEdit}}, false)
	changed := state.applyPanelEvents([]debugPanelEvent{{
		action: client.DebugPanelActionConfirm, value: "99999",
	}}, false)
	if !changed {
		t.Fatal("越界但可解析的值应钳制并报告变更")
	}
	if state.effective.Sim.SpawnRadius != 64 {
		t.Fatalf("spawnRadius=%v, want 钳到 64", state.effective.Sim.SpawnRadius)
	}
	if state.editing {
		t.Fatal("钳制后必须退出编辑态")
	}
}

func TestPanelApplyEventsInvalidValueRejected(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.gravity")
	before := state.effective.Physics.Gravity
	state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionEnterEdit}}, false)
	changed := state.applyPanelEvents([]debugPanelEvent{{
		action: client.DebugPanelActionConfirm, value: "not a number",
	}}, false)
	if changed {
		t.Fatal("非法值不得报告变更")
	}
	if state.effective.Physics.Gravity != before {
		t.Fatalf("非法值必须保持原值: %v -> %v", before, state.effective.Physics.Gravity)
	}
	if state.editing {
		t.Fatal("非法值确认后必须退出编辑态")
	}
}

func TestPanelApplyEventsCancelKeepsValueAndExitsEdit(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "render.fovDegrees")
	before := state.effective.Render.FovDegrees
	state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionEnterEdit}}, false)
	changed := state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionCancel}}, false)
	if changed {
		t.Fatal("取消不得报告变更")
	}
	if state.effective.Render.FovDegrees != before {
		t.Fatalf("取消必须保持原值: %v -> %v", before, state.effective.Render.FovDegrees)
	}
	if state.editing {
		t.Fatal("取消后必须退出编辑态")
	}
}

func TestPanelApplyEventsEditValueIsNoop(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.gravity")
	state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionEnterEdit}}, false)
	changed := state.applyPanelEvents([]debugPanelEvent{{
		action: client.DebugPanelActionEditValue, value: "99",
	}}, false)
	if changed || !state.editing {
		t.Fatalf("EDIT_VALUE 是 Rust 草稿通知, Go 无动作: changed=%v editing=%v", changed, state.editing)
	}
}

func TestPanelApplyEventsCloseHidesPanel(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionClose}}, false)
	if state.visible {
		t.Fatal("CLOSE 必须隐藏面板")
	}
}

func TestPanelApplyEventsIgnoredWhileEditing(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.gravity")
	state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionEnterEdit}}, false)
	state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionSelectNext}}, false)
	if state.selected != 7 {
		t.Fatalf("编辑期间不得移动选中行: selected=%d, want 7(gravity)", state.selected)
	}
}

func TestDecodeDebugPanelEventsFiltersAndPreservesOrder(t *testing.T) {
	got := decodeDebugPanelEvents([]client.UIEvent{
		{Kind: client.UIEventAction, ActionID: 7},
		{Kind: client.UIEventDebugAction, PanelAction: client.DebugPanelActionSelectNext},
		{Kind: client.UIEventDebugAction, PanelAction: client.DebugPanelActionConfirm, PanelValue: "12"},
	})
	want := []debugPanelEvent{
		{action: client.DebugPanelActionSelectNext},
		{action: client.DebugPanelActionConfirm, value: "12"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("筛选结果=%+v, want %+v", got, want)
	}
}

// editSeedForTest 从段字节里提取编辑行的 edit_value。定位 rows 被标记
// Selected 的行（rows() 不变量：恒有且仅有一个），其记录在 rowsOffset 之后
// 每行定长 52 字节（label 24 + value 24 + flags 4），编辑载荷紧随 flags：
// u32 len + value + u32 cursor。
func editSeedForTest(t *testing.T, segment []byte, rows []render.PanelRow) string {
	t.Helper()
	for i, row := range rows {
		if !row.Selected {
			continue
		}
		base := rowsOffset("单机") + i*52
		editLen := int(binary.LittleEndian.Uint32(segment[base+52 : base+56]))
		end := base + 56 + editLen
		if end > len(segment) {
			t.Fatalf("edit_value 越界: len=%d end=%d", len(segment), end)
		}
		return string(segment[base+56 : end])
	}
	t.Fatal("段中没有 Selected 行")
	return ""
}

// TestPanelEditSeedFullPrecisionDoesNotDrift 锁住 M-2 修法：编辑器向 Rust
// 播种的 edit_value 必须来自字段的全精度原始值，而不是 4 位有效数字的展示值
// ——用户不改文本直接 CONFIRM 时，确认事件原样回传播种文本，两者写回结果
// 必须一致。展示值 9.807 回写会令 9.80665 悄悄漂移为显示形式。测试走
// rows()→编码器→种子提取→applyPanelEvents 全链路，直击真实输入路径。
func TestPanelEditSeedFullPrecisionDoesNotDrift(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.gravity")
	state.effective.Physics.Gravity = 9.80665
	state.applyPanelEvents([]debugPanelEvent{{action: client.DebugPanelActionEnterEdit}}, false)
	rows := state.rows(false)
	seed := editSeedForTest(t, encodeDebugPanelSegment(true, true, render.PanelReadout{Mode: "单机"}, rows), rows)
	state.applyPanelEvents([]debugPanelEvent{{
		action: client.DebugPanelActionConfirm, value: seed,
	}}, false)
	if state.effective.Physics.Gravity != 9.80665 {
		t.Fatalf("确认播种值不得漂移有效值: got %v, want 9.80665", state.effective.Physics.Gravity)
	}
	if state.editing {
		t.Fatal("确认后必须退出编辑态")
	}
}

// TestPanelVisibleCapturesGameKeysAtFrameLoopLevel 锁住 spec 2.3 的核心断言
// 「面板可见时游戏键 MUST NOT 产生上行，位置与朝向不变」在帧循环层的落实：
// panelVisible() 的四条接线分支（Esc 不释放光标、相机旋转冻结、动作抑制、
// 移动归零）只有走 runGamePhase 才会被真实执行，unit 级 panelState 测试
// 覆盖不到。帧 0 面板可见时按 W+左键+E 并把光标移到 (200,150)：上行必须
// 全为中性输入，相机必须纹丝不动；帧 1 Esc 必须保持光标捕获；帧 2 F3
// 隐藏面板；帧 3 W 必须恢复移动上行。
func TestPanelVisibleCapturesGameKeysAtFrameLoopLevel(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式:重型测试由 CI 全量门禁运行")
	}
	drainInputs := func(messages []network.ClientMessage) []network.PlayerInput {
		var inputs []network.PlayerInput
		for _, message := range messages {
			input, ok := message.(network.PlayerInput)
			if !ok {
				t.Fatalf("意外上行消息 %#v，want PlayerInput", message)
			}
			inputs = append(inputs, input)
		}
		return inputs
	}
	var serverEndpoint network.ServerEndpoint
	var visiblePhase []network.PlayerInput
	app, endpoint, window := newChatLoopApplication(t, []chatWindowFrame{
		{
			delay:   55 * time.Millisecond,
			keys:    map[client.Key]bool{client.KeyW: true, client.KeyE: true},
			primary: true,
			cursorX: 200,
			cursorY: 150,
		},
		{
			delay: 55 * time.Millisecond,
			keys:  map[client.Key]bool{client.KeyEscape: true},
			onPoll: func() {
				visiblePhase = drainInputs(drainChatClientMessages(serverEndpoint))
			},
		},
		{delay: 55 * time.Millisecond, keys: map[client.Key]bool{client.KeyF3: true}},
		{delay: 55 * time.Millisecond, keys: map[client.Key]bool{client.KeyW: true}},
	})
	app.panel = newPanelState(config.Defaults())
	app.panel.visible = true
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	serverEndpoint = endpoint
	app.camera.Yaw, app.camera.Pitch = 0.5, -0.3
	app.render.MouseSensitivity = 1
	if err := runInteractive(app); err != nil {
		t.Fatal(err)
	}
	if app.panel.visible {
		t.Fatal("F3 必须隐藏面板")
	}
	if app.inventoryOpen {
		t.Fatal("面板可见期间 E 必须被抑制，背包不得打开")
	}
	if app.camera.Yaw != 0.5 || app.camera.Pitch != -0.3 {
		t.Fatalf("面板可见期间相机不得被鼠标旋转: yaw=%v pitch=%v", app.camera.Yaw, app.camera.Pitch)
	}
	if !window.captured {
		t.Fatal("面板可见期间 Esc 不得释放光标")
	}
	if len(window.captureHistory) != 1 || !window.captureHistory[0] {
		t.Fatalf("光标捕获历史=%v, want 仅入口捕获一次、绝无释放", window.captureHistory)
	}
	if len(visiblePhase) == 0 {
		t.Fatal("面板可见帧没有产生上行（中性输入本身也不该缺席）")
	}
	for _, input := range visiblePhase {
		if input.MoveX != 0 || input.MoveZ != 0 || input.Jump || input.Mining || input.Eating {
			t.Fatalf("面板可见时的上行必须全为中性输入: %+v", input)
		}
	}
	restored := false
	for _, input := range drainInputs(drainChatClientMessages(endpoint)) {
		if input.MoveZ != 0 {
			restored = true
		}
		if input.Mining || input.Eating {
			t.Fatalf("隐藏面板后的上行不得再带动作: %+v", input)
		}
	}
	if !restored {
		t.Fatal("隐藏面板后 W 必须恢复移动上行")
	}
}
