package assets_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// TestFarmlandLayerNumbersMatchClientShaderContract 是耕地 material 区间在
// Go 侧的机械守卫（farmland-mesh-top-sink D2a）。
//
// 区间的数值真值源是本包 blocks.go 的层枚举，但 Rust 客户端在
// packages/engine/crates/mornlea_client 的 src/render/shaders.rs
// （FARMLAND_MATERIAL_FIRST/FARMLAND_MATERIAL_LAST = 29/30）与其 terrain.wgsl
// 的 farmland_material 各硬编码了一份——WGSL 没有常量注入机制，三处没有共享
// 定义也没有生成步骤，只能人手同步。在 `LayerFarmlandDry` **之前**插层会整体
// 平移这段区间，客户端会把耕地当普通满格方块渲染（顶面不下沉）、甚至把相邻
// 层误判成短方块而误读尺寸位；本条与 Rust 侧 render/farmland_tests.rs 的
// farmland_range_constants_match_go_layer_enum 是仅有的两处报警点。
//
// 反引号纪律：本注释只对 Go 声明用反引号——archcheck 的
// TestCommentBacktickIdentifiersExist 把反引号名按全仓 Go 声明核对存在性，
// wgsl 函数与 Rust 测试名不是 Go 声明，一律纯文本提及。
func TestFarmlandLayerNumbersMatchClientShaderContract(t *testing.T) {
	// 字面量与 Rust 客户端 shaders.rs 的两个常量逐一对应，改任何一侧必须同步。
	if want := uint16(29); assets.LayerFarmlandDry != want {
		t.Fatalf("客户端耕地区间下界应为 %d（Rust 客户端 FARMLAND_MATERIAL_FIRST），实测 LayerFarmlandDry=%d", want, assets.LayerFarmlandDry)
	}
	if want := uint16(30); assets.LayerFarmlandWet != want {
		t.Fatalf("客户端耕地区间上界应为 %d（Rust 客户端 FARMLAND_MATERIAL_LAST），实测 LayerFarmlandWet=%d", want, assets.LayerFarmlandWet)
	}
	// 区间必须恰好覆盖干湿两层、且与两侧邻居紧贴：水层在前、小麦层在后，
	// 插层必然撞上这三条断言之一。
	if assets.LayerWater != assets.LayerFarmlandDry-1 {
		t.Fatalf("LayerWater=%d 不紧贴耕地区间下界，插层检测失效", assets.LayerWater)
	}
	if assets.LayerWheat0 != assets.LayerFarmlandWet+1 {
		t.Fatalf("LayerWheat0=%d 不紧贴耕地区间上界，插层检测失效", assets.LayerWheat0)
	}

	// 游戏编号 → 材质层的映射是客户端判别的实际输入：干/湿耕地六面的
	// material 都必须落在本区间内，否则 shader 不会走角高度路径。
	r := assets.NewRegistry()
	for _, id := range []world.BlockID{core.FarmlandDryID, core.FarmlandWetID} {
		for face := mesh.Face(0); face < 6; face++ {
			mat := r.Material(id, face)
			if mat != assets.LayerFarmlandDry && mat != assets.LayerFarmlandWet {
				t.Fatalf("耕地 %d 的面 %d material=%d 落在客户端耕地区间 [29,30] 之外", id, face, mat)
			}
		}
	}
}
