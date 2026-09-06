package capture

// capture_scene_selection_test.go 钉住抓帧运行的显式场景子集选择与 GIF
// 生成门控：子集过滤必须按场景表固有顺序保序输出、缺省请求（nil/空清单）
// 等价全量、非法清单（未知、重复、空项）一律报错并点名问题项，GIF 门控
// 按运行模式矩阵取值。场景表是唯一事实源，这里的期望序列都从表推导或
// 选取表内已知名，不在测试里复制整张清单。

import (
	"slices"
	"strings"
	"testing"
)

// captureSceneNamesOf 把场景切片折叠成名字切片，便于与期望序列整体比较
// ——captureScene 含闭包字段不可整体比较，名字序列等价即场景选取等价。
func captureSceneNamesOf(scenes []captureScene) []string {
	names := make([]string, len(scenes))
	for index := range scenes {
		names[index] = scenes[index].Name
	}
	return names
}

// TestSelectScenesDefaultRunsFullTable 钉住缺省路径不变：未请求子集
// （nil 与空清单两种写法）必须等价于全量场景表的表序执行。
func TestSelectScenesDefaultRunsFullTable(t *testing.T) {
	for _, requested := range [][]string{nil, {}} {
		got, err := selectScenes(requested)
		if err != nil {
			t.Fatalf("selectScenes(%v) 报错: %v", requested, err)
		}
		if !slices.Equal(captureSceneNamesOf(got), captureSceneNamesOf(captureScenes)) {
			t.Fatalf("selectScenes(%v)=%v，想要全量表序 %v",
				requested, captureSceneNamesOf(got), captureSceneNamesOf(captureScenes))
		}
	}
}

// TestSelectScenesPreservesTableOrder 钉住保序语义：请求清单故意乱序
// （含倒序片段），输出必须按场景表固有顺序重排，且相邻景、倒数第二与
// 唯一末景等顺序 MUST 条款在子集路径原样成立。
func TestSelectScenesPreservesTableOrder(t *testing.T) {
	requested := []string{
		"water-underwater", "terrain-noon", "mining-crack-heavy",
		"main-menu", "mining-crack-early", "far-horizon",
	}
	got, err := selectScenes(requested)
	if err != nil {
		t.Fatalf("selectScenes(%v) 报错: %v", requested, err)
	}
	want := []string{
		"terrain-noon", "mining-crack-early", "mining-crack-heavy",
		"main-menu", "far-horizon", "water-underwater",
	}
	if !slices.Equal(captureSceneNamesOf(got), want) {
		t.Fatalf("selectScenes(%v)=%v，想要表序 %v",
			requested, captureSceneNamesOf(got), want)
	}
}

// TestSelectScenesRejectsUnknownName 钉住拒绝语义：未知名必须报错且错误
// 信息点名该名字，不得静默忽略后降级为部分或全量运行。
func TestSelectScenesRejectsUnknownName(t *testing.T) {
	requested := []string{"terrain-noon", "no-such-scene"}
	_, err := selectScenes(requested)
	if err == nil {
		t.Fatal("selectScenes 含未知名未报错")
	}
	if !strings.Contains(err.Error(), "no-such-scene") {
		t.Fatalf("错误信息 %q 必须点名未知名 no-such-scene", err.Error())
	}
}

// TestSelectScenesRejectsDuplicateName 钉住拒绝语义：重复请求同一场景
// 必须报错并点名重复项。
func TestSelectScenesRejectsDuplicateName(t *testing.T) {
	requested := []string{"terrain-noon", "torch-night", "terrain-noon"}
	_, err := selectScenes(requested)
	if err == nil {
		t.Fatal("selectScenes 含重复名未报错")
	}
	if !strings.Contains(err.Error(), "terrain-noon") {
		t.Fatalf("错误信息 %q 必须点名重复名 terrain-noon", err.Error())
	}
}

// TestSceneNamesMatchesSceneTable 钉住导出名单与场景表逐项一致且保持
// 表序——名单只是场景表的投影，不得演化为第二份独立清单。
func TestSceneNamesMatchesSceneTable(t *testing.T) {
	if got := SceneNames(); !slices.Equal(got, captureSceneNamesOf(captureScenes)) {
		t.Fatalf("SceneNames()=%v，想要场景表表序 %v",
			got, captureSceneNamesOf(captureScenes))
	}
}

// TestRunOptionsGifsEnabledMatrix 钉住 GIF 生成门控矩阵：纯比对缺省不
// 生成、显式请求生成、更新基线恒生成（update 行为不变的承重断言）。
func TestRunOptionsGifsEnabledMatrix(t *testing.T) {
	for _, test := range []struct {
		name string
		opts RunOptions
		want bool
	}{
		{name: "check 缺省不生成", opts: RunOptions{}, want: false},
		{name: "check 显式请求生成", opts: RunOptions{IncludeGIFs: true}, want: true},
		{name: "update 恒生成", opts: RunOptions{UpdateGolden: true}, want: true},
		{name: "update 叠加显式请求仍生成", opts: RunOptions{UpdateGolden: true, IncludeGIFs: true}, want: true},
	} {
		if got := test.opts.gifsEnabled(); got != test.want {
			t.Fatalf("%s: gifsEnabled()=%v，想要 %v", test.name, got, test.want)
		}
	}
}

// TestValidateSceneSelectionRejectsBadRequests 钉住校验入口的完整拒绝
// 契约：空项、未知名、重复名一律报错并点名问题项（空项的问题名是空串，
// 只断言报错本身）。
func TestValidateSceneSelectionRejectsBadRequests(t *testing.T) {
	for _, test := range []struct {
		name       string
		requested  []string
		wantSubstr string
	}{
		{name: "空项", requested: []string{"terrain-noon", ""}},
		{name: "未知名", requested: []string{"terrain-noon", "not-a-scene"}, wantSubstr: "not-a-scene"},
		{name: "重复名", requested: []string{"torch-night", "torch-night"}, wantSubstr: "torch-night"},
	} {
		err := ValidateSceneSelection(test.requested)
		if err == nil {
			t.Fatalf("%s: ValidateSceneSelection 未报错", test.name)
		}
		if test.wantSubstr != "" && !strings.Contains(err.Error(), test.wantSubstr) {
			t.Fatalf("%s: 错误信息 %q 必须点名 %q", test.name, err.Error(), test.wantSubstr)
		}
	}
}

// TestValidateSceneSelectionAcceptsValidRequests 钉住合法清单全放行：
// nil/空（缺省全量的等价输入）、乱序子集与全量名单都不得报错。
func TestValidateSceneSelectionAcceptsValidRequests(t *testing.T) {
	for _, requested := range [][]string{
		nil,
		{},
		{"water-underwater"},
		{"mining-crack-heavy", "mining-crack-early"},
		SceneNames(),
	} {
		if err := ValidateSceneSelection(requested); err != nil {
			t.Fatalf("ValidateSceneSelection(%v) 报错: %v", requested, err)
		}
	}
}
