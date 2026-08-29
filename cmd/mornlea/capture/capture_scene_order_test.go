package capture

// capture_scene_order_test.go 钉住正式 capture 场景表的完整顺序；既有测试入口
// 同时验证 ai-companion 夹具确定性，本次纯重组保留该入口的全部语义。

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/internal/assets"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
)

// TestCaptureSceneOrderAndAICompanionDeterminism 钉住整张场景表的顺序，并覆盖
// ai-companion 夹具的确定性。
//
// 原名是 ...AICompanionIsLast...，但 ai-companion 已不再是最后一个：水景两个
// 场景与远环 far-horizon 追加在它之后，而 water-underwater 另有必须排最后的
// 硬理由，见 `TestWaterUnderwaterCaptureSceneIsLast`。变基排序协调:far-horizon
// 插在 water-underwater 之前(倒数第二);其 `Apply` 显式清空 ai-companion 留下的
// 全部呈现状态,与前一场景互相独立。bed-night 按 spec delta 插在 torch-night
// 之后、ai-companion 之前（与 materials-showcase 之间）。
func TestCaptureSceneOrderAndAICompanionDeterminism(t *testing.T) {
	wantNames := []string{
		"terrain-noon", "hud-hotbar-health", "hud-survival-feedback", "hud-item-name-popup",
		"avatar-nametag", "inventory-crafting",
		"workbench-crafting", "chest-container", "furnace-container",
		"debug-panel", "skylight-tunnel", "block-light-room", "torch-night", "bed-night",
		"materials-showcase",
		"target-block-feedback", "oak-grove", "ai-companion",
		"hostile-mob", "water-surface-slope", "main-menu", "settings-menu", "far-horizon", "water-underwater",
	}
	gotNames := make([]string, len(captureScenes))
	for index, scene := range captureScenes {
		gotNames[index] = scene.Name
	}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("capture scenes=%v，想要 %v", gotNames, wantNames)
	}
	scene := captureSceneByName(t, "ai-companion")
	if scene.Prepare == nil || scene.Apply == nil || scene.WarmupFrames != 8 {
		t.Fatalf("ai-companion 场景不完整: %+v", scene)
	}

	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := newCaptureAICompanionState()
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)
	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 ai-companion: %v", err)
	}
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			chunk, ok := app.Mirror().Chunk(core.Overworld, core.ChunkPos{X: x, Z: z})
			wantRevision := uint64(1)
			if x == 0 && z == 0 {
				wantRevision = 2
			}
			if !ok || chunk.Revision != wantRevision {
				t.Fatalf("chunk (%d,%d) revision/loaded=%d/%v，想要 %d/true",
					x, z, chunk.Revision, ok, wantRevision)
			}
		}
	}
	for _, test := range []struct {
		position core.BlockPos
		want     core.BlockID
	}{
		{position: core.BlockPos{X: 5, Y: -1, Z: 4}, want: core.StoneID},
		{position: core.BlockPos{X: 5, Y: 0, Z: 4}, want: core.GrassID},
		{position: core.BlockPos{X: -1, Y: 0, Z: 4}, want: core.AirID},
	} {
		got, loaded := app.Mirror().BlockAt(core.Overworld, test.position)
		if !loaded || got != test.want {
			t.Fatalf("BlockAt(%+v)=%d/%v，想要 %d/true", test.position, got, loaded, test.want)
		}
	}
	if mesher.Stats().DirtySections == 0 {
		t.Fatal("ai-companion 夹具没有经 mirror 标记 dirty section")
	}

	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用 ai-companion: %v", err)
	}
	assertCaptureAICompanionState(t, app)
	overlay := app.ChatOverlay()
	if !overlay.Open || overlay.Input != "@阿木 挖石头" ||
		!slices.Equal(overlay.Lines, []string{"旅人 → 阿木：挖石头"}) {
		t.Fatalf("chat overlay=%+v", overlay)
	}
	uniqueRunes := map[rune]struct{}{}
	for _, text := range append([]string{"阿木", overlay.Input}, overlay.Lines...) {
		for _, value := range text {
			uniqueRunes[value] = struct{}{}
		}
	}
	if len(uniqueRunes) > 32 {
		t.Fatalf("ai-companion 独特 rune=%d，想要不超过 32", len(uniqueRunes))
	}
}

// TestTorchNightCaptureScenePosition 锁住 torch-night 的表内位置：紧随
// block-light-room 且先于 bed-night（bed-night 按 spec delta 插入其后；
// 两者都先于 materials-showcase）。
func TestTorchNightCaptureScenePosition(t *testing.T) {
	indexOf := func(name string) int {
		for index, scene := range captureScenes {
			if scene.Name == name {
				return index
			}
		}
		t.Fatalf("场景 %q 不存在", name)
		return -1
	}
	blockLight := indexOf("block-light-room")
	torchNight := indexOf("torch-night")
	bedNight := indexOf("bed-night")
	materials := indexOf("materials-showcase")
	if torchNight != blockLight+1 {
		t.Fatalf("torch-night=%d 必须紧随 block-light-room=%d", torchNight, blockLight)
	}
	if torchNight >= materials {
		t.Fatalf("torch-night=%d 必须在 materials-showcase=%d 之前", torchNight, materials)
	}
	if bedNight <= torchNight {
		t.Fatalf("bed-night=%d 必须在 torch-night=%d 之后", bedNight, torchNight)
	}
}

// TestBedNightCaptureScenePosition 锁住 bed-night 的表内位置：紧随
// torch-night 且在 ai-companion 之前（spec delta visual-verification
// 「bed-night 无窗口夜景场景」的排序要求）。
func TestBedNightCaptureScenePosition(t *testing.T) {
	indexOf := func(name string) int {
		for index, scene := range captureScenes {
			if scene.Name == name {
				return index
			}
		}
		t.Fatalf("场景 %q 不存在", name)
		return -1
	}
	torchNight := indexOf("torch-night")
	bedNight := indexOf("bed-night")
	aiCompanion := indexOf("ai-companion")
	farHorizon := indexOf("far-horizon")
	waterUnderwater := indexOf("water-underwater")
	if bedNight != torchNight+1 {
		t.Fatalf("bed-night=%d 必须紧随 torch-night=%d", bedNight, torchNight)
	}
	if bedNight >= aiCompanion {
		t.Fatalf("bed-night=%d 必须在 ai-companion=%d 之前", bedNight, aiCompanion)
	}
	// 既有顺序不变量随插入保持：far-horizon 倒数第二、water-underwater 唯一末位。
	if farHorizon != len(captureScenes)-2 {
		t.Fatalf("far-horizon=%d 必须是倒数第二（共 %d 项）", farHorizon, len(captureScenes))
	}
	if waterUnderwater != len(captureScenes)-1 {
		t.Fatalf("water-underwater=%d 必须是唯一末场景（共 %d 项）", waterUnderwater, len(captureScenes))
	}
}
