//go:build darwin

package app

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/sim/tuning"
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
	app := &Application{panel: newPanelState(config.Defaults())}
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
	state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionEnterEdit}}, false)
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
	originalSim := tuning.ActiveTunables()
	t.Cleanup(func() {
		physics.SetTunables(originalPhysics)
		tuning.SetTunables(originalSim)
	})

	app := &Application{panel: newPanelState(config.Defaults())}
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
		op: client.DebugPanelActionEnterEdit,
	}}, true)
	if state.editing {
		t.Fatal("联机时不得进入编辑态")
	}
	state.applyPanelEvents([]debugPanelEvent{{
		op: client.DebugPanelActionConfirm, value: "99",
	}}, false)
	if state.effective.Physics.Gravity == 99 {
		t.Fatalf("未进入编辑态的 confirm 不得改值：%v", state.effective.Physics.Gravity)
	}
}

// TestDebugUIStateRowsShape 锁定调试分节的行形状:读数行恒只读、段头行值
// 恒空、param 行唯一选中,editing 只落在选中可编辑行上——与旧 layout v3 的
// 单编辑态不变量与 Rust flags 语义逐条对应。
func TestDebugUIStateRowsShape(t *testing.T) {
	readout := render.PanelReadout{
		FrameMillis: 12.5, Position: mgl32.Vec3{10, 64, -3}, Yaw: 45, Pitch: -12,
		Tick: 1234, WorldTime: 42, LoadedChunks: 137, Mode: "单机",
	}
	rows := []render.PanelRow{
		{Label: "── physics ──", ReadOnly: true},
		{Label: "gravity", Value: "9.8", Selected: true},
		{Label: "fovDegrees", Value: "70"},
	}
	state := debugUIState(true, readout, rows)
	if !state.Visible || state.Mode != "单机" {
		t.Fatalf("分节头不符: %+v", state)
	}
	// 前 6 行是读数行(模式走顶部字段,不重复),随后 3 行是参数区。
	readouts := state.Rows[:6]
	for _, row := range readouts {
		if row.Kind != "readout" || !row.ReadOnly || row.Selected || row.Editing {
			t.Fatalf("读数行必须只读且不可选中: %+v", row)
		}
	}
	section := state.Rows[6]
	if section.Kind != "section" || section.Value != "" || !section.ReadOnly {
		t.Fatalf("段头行必须为 section 且值恒空: %+v", section)
	}
	gravity := state.Rows[7]
	if gravity.Kind != "param" || !gravity.Selected || !gravity.Editing || gravity.ReadOnly {
		t.Fatalf("选中参数行必须同时处于编辑态: %+v", gravity)
	}
	fov := state.Rows[8]
	if fov.Kind != "param" || fov.Selected || fov.Editing {
		t.Fatalf("未选中行不得置位 selected/editing: %+v", fov)
	}
	if gravity.Label != "gravity" || gravity.Value != "9.8" {
		t.Fatalf("param 行内容不符: %+v", gravity)
	}
	if got := readouts[0].Value; got != "12.5" {
		t.Fatalf("帧时读数 = %q, want 12.5", got)
	}
}

// TestDebugUIStateReadOnlyRowNeverSelectedOrEditing 锁住 readonly 行不变量:
// 联机时 physics/sim 行只读,组装绝不给同一行同时置 selected/editing(前端
// 解析层会拒绝该组合,整份状态被丢)。
func TestDebugUIStateReadOnlyRowNeverSelectedOrEditing(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.eyeHeight")
	rows := state.rows(true)
	got := debugUIState(true, render.PanelReadout{Mode: "联机"}, rows)
	for _, row := range got.Rows {
		if row.ReadOnly && (row.Selected || row.Editing) {
			t.Fatalf("readonly 行不得置位 selected/editing: %+v", row)
		}
		if !row.ReadOnly && row.Selected && row.Editing {
			t.Fatalf("唯一编辑态必须落在选中可编辑行: %+v", row)
		}
	}
}

// TestDebugUIStateAbsentWhenPanelHidden 锁住「面板关闭时零参与」:面板不可见
// 的组装不产出 debug 分节。
func TestDebugUIStateAbsentWhenPanelHidden(t *testing.T) {
	app := &Application{panel: newPanelState(config.Defaults())}
	if state := app.buildUIState(); state.Debug != nil {
		t.Fatalf("面板关闭不得产出 debug 分节: %+v", state.Debug)
	}
}

// TestDebugUIStateRowsCappedAtLimit 锁住行数上限:64 行参数(含段头)加上
// 读数行也不会超过 schema debug.rows maxItems。
func TestDebugUIStateRowsCappedAtLimit(t *testing.T) {
	rows := make([]render.PanelRow, 64)
	for i := range rows {
		rows[i] = render.PanelRow{Label: fmt.Sprintf("field %02d", i), Value: "123.456", Selected: i == 0}
	}
	got := debugUIState(true, render.PanelReadout{Mode: "benchmark"}, rows)
	if len(got.Rows) > debugPanelRowsMax {
		t.Fatalf("行数=%d, 上界 %d", len(got.Rows), debugPanelRowsMax)
	}
}

// TestTruncateDebugRunesKeepsRuneBoundary 锁住行文本的码点截断语义:超出
// 24 码点的 CJK 标签在 rune 边界截断,绝不切半(与旧定宽字段的截断同一语义)。
func TestTruncateDebugRunesKeepsRuneBoundary(t *testing.T) {
	label := strings.Repeat("一", 30) // 30 个 CJK > 24
	if got := truncateDebugRunes(label, debugRowMaxRunes); len([]rune(got)) != 24 {
		t.Fatalf("CJK 标签截断 = %d 码点, want 24", len([]rune(got)))
	}
	if got := truncateDebugRunes("short", debugRowMaxRunes); got != "short" {
		t.Fatalf("短文本不得截断: %q", got)
	}
	if got := truncateDebugRunes("starvationDamageIntervalTicks", debugRowMaxRunes); len([]rune(got)) != 24 {
		t.Fatalf("长字段名应截断到 24 码点: %q", got)
	}
}

// TestPanelConfirmFullPrecisionKeepsValue 锁住写回侧的全精度不变量:确认
// 事件携带的精确文本(编辑播种语义由前端呈现层承担)写回后不得漂移有效值。
func TestPanelConfirmFullPrecisionKeepsValue(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.gravity")
	state.effective.Physics.Gravity = 9.80665
	state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionEnterEdit}}, false)
	state.applyPanelEvents([]debugPanelEvent{{
		op: client.DebugPanelActionConfirm, value: "9.80665",
	}}, false)
	if state.effective.Physics.Gravity != 9.80665 {
		t.Fatalf("精确值写回不得漂移: got %v, want 9.80665", state.effective.Physics.Gravity)
	}
}

func TestPanelApplyEventsSelectMovesSelection(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.eyeHeight")
	state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionSelectNext}}, false)
	if state.selected != 1 {
		t.Fatalf("SELECT_NEXT 后 selected=%d, want 1", state.selected)
	}
	state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionSelectPrev}}, false)
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
		state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionSelectNext}}, true)
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

	changed := state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionEnterEdit}}, false)
	if changed || !state.editing {
		t.Fatalf("进入编辑不算配置变更: changed=%v editing=%v", changed, state.editing)
	}
	changed = state.applyPanelEvents([]debugPanelEvent{{
		op: client.DebugPanelActionConfirm, value: "12.5",
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
	state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionEnterEdit}}, false)
	changed := state.applyPanelEvents([]debugPanelEvent{{
		op: client.DebugPanelActionConfirm, value: "99999",
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
	state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionEnterEdit}}, false)
	changed := state.applyPanelEvents([]debugPanelEvent{{
		op: client.DebugPanelActionConfirm, value: "not a number",
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
	state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionEnterEdit}}, false)
	changed := state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionCancel}}, false)
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
	state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionEnterEdit}}, false)
	changed := state.applyPanelEvents([]debugPanelEvent{{
		op: client.DebugPanelActionEditValue, value: "99",
	}}, false)
	if changed || !state.editing {
		t.Fatalf("EDIT_VALUE 是 Rust 草稿通知, Go 无动作: changed=%v editing=%v", changed, state.editing)
	}
}

func TestPanelApplyEventsCloseHidesPanel(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionClose}}, false)
	if state.visible {
		t.Fatal("CLOSE 必须隐藏面板")
	}
}

func TestPanelApplyEventsIgnoredWhileEditing(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "physics.gravity")
	state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionEnterEdit}}, false)
	state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionSelectNext}}, false)
	if state.selected != 7 {
		t.Fatalf("编辑期间不得移动选中行: selected=%d, want 7(gravity)", state.selected)
	}
}

func TestDecodeDebugPanelEventsFiltersAndPreservesOrder(t *testing.T) {
	got := decodeDebugPanelEvents([]client.UIEvent{
		{Kind: client.UIEventAction, ActionID: client.UIActionQuit},
		{Kind: client.UIEventDebugAction, PanelAction: client.DebugPanelActionSelectNext},
		{Kind: client.UIEventDebugAction, PanelAction: client.DebugPanelActionConfirm, PanelValue: "12"},
	})
	want := []debugPanelEvent{
		{op: client.DebugPanelActionSelectNext},
		{op: client.DebugPanelActionConfirm, value: "12"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("筛选结果=%+v, want %+v", got, want)
	}
}

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
	if err := RunInteractive(app); err != nil {
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

// TestPanelF3ToggleFromHiddenCapturesAtFrameLoopLevel 是 2.3 的对称路径：
// TestPanelVisibleCapturesGameKeysAtFrameLoopLevel 从「面板已可见」开始，只覆盖
// 可见→隐藏→恢复；本测试从隐藏开始，锁住 F3 上升沿在同一帧内切换可见并立即
// 整帧捕获（F3 与游戏键同帧时游戏键 MUST NOT 产生上行、相机 MUST NOT 旋转）。
// 帧 0 W 必须产生移动上行；帧 1 F3 打开面板；帧 2 W+鼠标移动被捕获（中性
// 上行+相机不动）；帧 3 F3 关闭；帧 4 W 恢复移动上行。
func TestPanelF3ToggleFromHiddenCapturesAtFrameLoopLevel(t *testing.T) {
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
	var beforeToggle []network.PlayerInput
	var duringVisible []network.PlayerInput
	app, endpoint, window := newChatLoopApplication(t, []chatWindowFrame{
		{delay: 55 * time.Millisecond, keys: map[client.Key]bool{client.KeyW: true}},
		{
			delay: 55 * time.Millisecond,
			keys:  map[client.Key]bool{client.KeyF3: true},
			onPoll: func() {
				beforeToggle = drainInputs(drainChatClientMessages(serverEndpoint))
			},
		},
		{
			delay:   55 * time.Millisecond,
			keys:    map[client.Key]bool{client.KeyW: true},
			cursorX: 200,
			cursorY: 150,
			onPoll: func() {
				// 帧 1（F3 打开）的上行：无按键位移，必为中性。
				drainInputs(drainChatClientMessages(serverEndpoint))
			},
		},
		{
			delay: 55 * time.Millisecond,
			keys:  map[client.Key]bool{client.KeyF3: true},
			onPoll: func() {
				// 帧 2（W+鼠标移动）的正加上行：整帧捕获后必为中性。
				duringVisible = drainInputs(drainChatClientMessages(serverEndpoint))
			},
		},
		{delay: 55 * time.Millisecond, keys: map[client.Key]bool{client.KeyW: true}},
	})
	serverEndpoint = endpoint
	app.panel = newPanelState(config.Defaults())
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	app.camera.Yaw, app.camera.Pitch = 0.5, -0.3
	app.render.MouseSensitivity = 1
	if err := RunInteractive(app); err != nil {
		t.Fatal(err)
	}
	if app.panel.visible {
		t.Fatal("最后一帧 F3 必须关闭面板")
	}
	if len(beforeToggle) == 0 {
		t.Fatal("打开面板前的 W 帧必须产生上行")
	}
	for _, input := range beforeToggle {
		if input.MoveZ == 0 {
			t.Fatalf("打开面板前的上行必须携带移动: %+v", input)
		}
	}
	if len(duringVisible) == 0 {
		t.Fatal("面板可见帧没有产生上行（中性输入本身也不该缺席）")
	}
	for _, input := range duringVisible {
		if input.MoveX != 0 || input.MoveZ != 0 || input.Jump || input.Mining || input.Eating {
			t.Fatalf("面板可见时的上行必须全为中性输入: %+v", input)
		}
	}
	if app.camera.Yaw != 0.5 || app.camera.Pitch != -0.3 {
		t.Fatalf("面板可见期间相机不得被鼠标旋转: yaw=%v pitch=%v", app.camera.Yaw, app.camera.Pitch)
	}
	if !window.captured {
		t.Fatal("面板可见期间光标必须保持捕获")
	}
	if len(window.captureHistory) != 1 || !window.captureHistory[0] {
		t.Fatalf("光标捕获历史=%v, want 仅入口捕获一次、绝无打开时释放再重捕获", window.captureHistory)
	}
	restored := false
	for _, input := range drainInputs(drainChatClientMessages(endpoint)) {
		if input.MoveZ != 0 {
			restored = true
		}
	}
	if !restored {
		t.Fatal("面板关闭后 W 必须恢复移动上行")
	}
}

// TestPanelFrameInputReopenStartsFreshFrameTimer 锁住「面板隐藏复位」：关闭期间
// panelLastFrameAt 被清零，重开后的第一帧帧时从 0 重新起算，而不是把整段关闭
// 时长记成帧时（面板重开第一帧会显示数十秒的帧时）。
func TestPanelFrameInputReopenStartsFreshFrameTimer(t *testing.T) {
	app := &Application{panel: newPanelState(config.Defaults())}
	start := time.Now()
	app.panel.visible = true
	if readout, _ := app.panelFrameInput(start); readout.FrameMillis != 0 {
		t.Fatalf("打开面板的第一帧帧时=%v, want 0", readout.FrameMillis)
	}
	app.panel.visible = false
	if readout, _ := app.panelFrameInput(start.Add(time.Hour)); readout.FrameMillis != 0 {
		t.Fatalf("关闭面板不得产出读数: %v", readout.FrameMillis)
	}
	app.panel.visible = true
	if readout, _ := app.panelFrameInput(start.Add(time.Hour + time.Millisecond)); readout.FrameMillis != 0 {
		t.Fatalf("重开面板第一帧帧时=%v, want 0（关闭时长不得计入）", readout.FrameMillis)
	}
}

// TestPanelRemoteAllowsRenderEdit 锁住 ruling 修正后的 spec「联机呈现组可编辑」
// 场景：remote() 只禁 physics/sim 权威组（避免本地预测偏离服务端），render.*
// 纯本地呈现（FOV、灵敏度）在联机时仍可编辑并在当前进程生效。
func TestPanelRemoteAllowsRenderEdit(t *testing.T) {
	state := newPanelState(config.Defaults())
	state.visible = true
	state.selectFieldForTest(t, "render.fovDegrees")
	if state.applyPanelEvents([]debugPanelEvent{{op: client.DebugPanelActionEnterEdit}}, true); !state.editing {
		t.Fatal("联机时 render 行必须可进入编辑态")
	}
	changed := state.applyPanelEvents([]debugPanelEvent{{
		op: client.DebugPanelActionConfirm, value: "42",
	}}, true)
	if !changed {
		t.Fatal("联机时 render 分组确认合法值必须报告变更")
	}
	if state.effective.Render.FovDegrees != 42 {
		t.Fatalf("fovDegrees=%v, want 42（仅本地呈现，不产生网络上行）", state.effective.Render.FovDegrees)
	}
	if state.editing {
		t.Fatal("确认后必须退出编辑态")
	}
}
