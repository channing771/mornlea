package assets_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
)

// snowLayerBlocks 是四档雪层的稳定编号样本，按档位 1..4 排列。
var snowLayerBlocks = [...]core.BlockID{
	core.SnowLayer1BlockID,
	core.SnowLayer2BlockID,
	core.SnowLayer3BlockID,
	core.SnowLayer4BlockID,
}

// TestSnowLayerPresentationContract 锁定雪层四档的呈现契约：顶面高度原值逐档
// 1..4（呈现高度 (raw+1)/16 = 2/16..5/16，档间差 1/16 可辨；raw=1 是短方块域
// 1..=14 的最低值，雪层是该值的第一个消费者）、非不透明、零流体高度（不触
// 发 FluidHeight 与 BlockTopRaw 互斥校验）、材质按顶/侧两层映射且四档共用，
// 以及朝空气出面、朝透明邻居按透明剔除的可见性基准。
func TestSnowLayerPresentationContract(t *testing.T) {
	registry := assets.NewRegistry()
	for i, id := range snowLayerBlocks {
		// 档位 k 的 raw = k：1..4 档分别 1..4，呈现高度 (raw+1)/16 即 2/16..5/16
		//（spec 钉的可观察高度——第 k 档恰厚 (k+1)/16）。
		if got, want := registry.BlockTopRaw(id), uint8(i+1); got != want {
			t.Fatalf("雪层档位 %d 的 BlockTopRaw = %d，想要 %d", i+1, got, want)
		}
		if got := registry.Opaque(id); got {
			t.Fatalf("雪层档位 %d 判成不透明：不满格即不遮光", i+1)
		}
		if got := registry.FluidHeight(id); got != 0 {
			t.Fatalf("雪层档位 %d 的 FluidHeight = %d，想要哨兵 0（非流体）", i+1, got)
		}
		if got := registry.Emission(id); got != 0 {
			t.Fatalf("雪层档位 %d 的 Emission = %d，想要 0", i+1, got)
		}
		if got := registry.LightAttenuation(id); got != 0 {
			t.Fatalf("雪层档位 %d 的 LightAttenuation = %d，想要 0", i+1, got)
		}
		if got := registry.Model(id); got != 0 {
			t.Fatalf("雪层档位 %d 的 Model = %d，想要默认 0（普通短方块路径）", i+1, got)
		}
		// 材质映射：顶面用雪层顶、其余五面用雪层侧，四档共用（档位差异只由
		// BlockTopRaw 表达，与 8 个流体编号共用 LayerWater 同形）。
		if got := registry.Material(id, mesh.FacePosY); got != assets.LayerSnowLayerTop {
			t.Fatalf("雪层档位 %d 顶面材质层 = %d，想要 LayerSnowLayerTop(%d)",
				i+1, got, assets.LayerSnowLayerTop)
		}
		for _, face := range []mesh.Face{
			mesh.FaceNegX, mesh.FacePosX, mesh.FaceNegY, mesh.FaceNegZ, mesh.FacePosZ,
		} {
			if got := registry.Material(id, face); got != assets.LayerSnowLayerSide {
				t.Fatalf("雪层档位 %d 侧面（face %d）材质层 = %d，想要 LayerSnowLayerSide(%d)",
					i+1, face, got, assets.LayerSnowLayerSide)
			}
		}
	}
	// 可见性基准：朝空气出面；朝另一档雪层按透明剔除（同玻璃同类内部面剔除）；
	// 相邻不透明方块朝雪层的面仍可见（雪层不遮挡邻居）。
	for _, id := range snowLayerBlocks {
		if !registry.FaceVisible(id, core.AirID) {
			t.Fatalf("雪层 %d 朝向空气的面消失了", id)
		}
		if registry.FaceVisible(id, core.SnowLayer1BlockID) {
			t.Fatalf("雪层 %d 朝向另一档雪层的面未被透明剔除", id)
		}
		if !registry.FaceVisible(core.StoneID, id) {
			t.Fatalf("石头朝向雪层 %d 的面消失了：雪层不得遮挡邻居", id)
		}
		if registry.FaceVisible(id, core.StoneID) {
			t.Fatalf("雪层 %d 朝向不透明邻居的面未被遮挡", id)
		}
	}
}

// TestSnowLayerEntersMeshSnapshot 锁定「快照自动跟随」：四档雪层必须逐条进入
// mesh registry 快照（条目数覆盖全部已注册方块），且冻结字段与活体 registry
// 逐点一致——Rust 侧只消费快照，漏登记等于雪层永远不出面。
func TestSnowLayerEntersMeshSnapshot(t *testing.T) {
	registry := assets.NewRegistry()
	snapshot := registry.MeshSnapshot()
	if got, want := len(snapshot.Blocks), int(core.BlockIDMax); got != want {
		t.Fatalf("snapshot block 数 = %d，想要覆盖全部已注册方块的 %d", got, want)
	}
	for _, id := range snowLayerBlocks {
		block := snapshot.Blocks[int(id)]
		if block.ID != id || block.Opaque || block.Emission != 0 ||
			block.FluidHeight != 0 || block.LightAttenuation != 0 || block.Model != 0 {
			t.Fatalf("雪层 %d 的快照字段 = %+v", id, block)
		}
		if want := registry.BlockTopRaw(id); block.BlockTopRaw != want {
			t.Fatalf("快照中雪层 %d 的 BlockTopRaw = %d，想要 %d", id, block.BlockTopRaw, want)
		}
		if got := snapshot.FaceVisible(id, core.AirID); !got {
			t.Fatalf("快照中雪层 %d 朝向空气的面不可见", id)
		}
	}
}

// TestSnowLayerLayersAppendAfterHumanLayers 锁定材质层追加纪律：两张雪层只能
// 追加在人物层之后（后续追加层继续排在它们之后，`layerCount` 由末位层决定），
// 既有冻结层号（植物 31..54、火把 59、床 60..67、短草 68、裂纹 69..78）一律
// 不动，也不得落进植物 material 区间（否则会被渲染成交叉斜面）。
func TestSnowLayerLayersAppendAfterHumanLayers(t *testing.T) {
	registry := assets.NewRegistry()
	if assets.LayerSnowLayerTop != assets.LayerSnowLayerSide-1 {
		t.Fatalf("雪层两层必须相邻：top=%d side=%d", assets.LayerSnowLayerTop, assets.LayerSnowLayerSide)
	}
	if assets.LayerSnowLayerTop <= assets.LayerHumanClayLeg {
		t.Fatalf("雪层两层必须追加在人物层 %d 之后，实际 top=%d",
			assets.LayerHumanClayLeg, assets.LayerSnowLayerTop)
	}
	if got, want := registry.LayerCount(), int(assets.LayerSapling)+1; got != want {
		t.Fatalf("LayerCount = %d，想要覆盖雪层与后续追加层后的 %d", got, want)
	}
	if mesh.PlantMaterial(assets.LayerSnowLayerTop) || mesh.PlantMaterial(assets.LayerSnowLayerSide) {
		t.Fatal("雪层材质层落进了植物区间：会被渲染成交叉斜面")
	}
}

// TestSnowLayerTexturesAreSnowFamilyButDistinct 锁定两张雪层的程序化像素：全长
// 16×16×4、全不透明（固体层不得带透明像素，否则被 cutout discard 打洞）、与
// 彼此及既有雪块两层（LayerSnowTop/LayerSnowSide）逐像素不同——同族雪白但
// 独立层号各有像素。
func TestSnowLayerTexturesAreSnowFamilyButDistinct(t *testing.T) {
	registry := assets.NewRegistry()
	layers := []struct {
		name  string
		layer uint16
	}{
		{"雪层顶", assets.LayerSnowLayerTop},
		{"雪层侧", assets.LayerSnowLayerSide},
	}
	neighbors := []struct {
		name  string
		layer uint16
	}{
		{"雪块顶", assets.LayerSnowTop},
		{"雪块侧", assets.LayerSnowSide},
	}
	for _, tt := range layers {
		px := registry.LayerRGBA(int(tt.layer))
		if len(px) != 16*16*4 {
			t.Fatalf("%s材质长度 = %d，想要 %d", tt.name, len(px), 16*16*4)
		}
		for i := 3; i < len(px); i += 4 {
			if px[i] != 255 {
				t.Fatalf("%s是不透明固体层，像素 alpha 必须全为 255", tt.name)
			}
		}
		// 雪白族：亮度偏高。
		bright, total := 0, 0
		for i := 0; i < len(px); i += 4 {
			bright += (int(px[i]) + int(px[i+1]) + int(px[i+2])) / 3
			total++
		}
		if avg := bright / total; avg < 190 {
			t.Fatalf("%s平均亮度 = %d，雪白族想要 >= 190", tt.name, avg)
		}
		for _, other := range neighbors {
			if string(px) == string(registry.LayerRGBA(int(other.layer))) {
				t.Fatalf("%s与%s逐像素相同：独立层号必须各有像素", tt.name, other.name)
			}
		}
	}
	if string(registry.LayerRGBA(int(assets.LayerSnowLayerTop))) ==
		string(registry.LayerRGBA(int(assets.LayerSnowLayerSide))) {
		t.Fatal("雪层顶/侧两层逐像素相同：必须可区分")
	}
}
