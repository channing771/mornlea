package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"os"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	pixelPerfectionRepository = "https://github.com/minetest-texture-packs/Pixel-Perfection"
	pixelPerfectionCommit     = "7935d064fc6f993d1b5038ed5ec17a615600cf0a"
	pixelPerfectionLicenseURL = "https://creativecommons.org/licenses/by-sa/4.0/legalcode.txt"
)

var pixelPerfectionSources = map[string]string{
	"stone": "default/default_stone.png", "dirt": "default/default_dirt.png",
	"grass_top": "default/default_grass.png", "grass_side": "default/default_grass_side.png",
	"bedrock": "bedrock/bedrock.png", "stone_brick": "default/default_stone_brick.png",
	"furnace": "default/default_furnace_front.png", "iron_block": "default/default_steel_block.png",
	"leaves": "default/default_leaves_simple.png", "glass": "default/default_glass.png",
	"cobblestone": "default/default_cobble.png", "sand": "default/default_sand.png",
	"gravel": "default/default_gravel.png", "oak_log_side": "default/default_tree.png",
	"oak_log_top": "default/default_tree_top.png", "oak_planks": "default/default_wood.png",
	"brick": "default/default_brick.png", "white_wool": "wool/wool_white.png",
	"clay": "default/default_clay.png", "snow_top": "default/default_snow.png",
	"snow_side": "default/default_snow.png", "mossy_cobblestone": "default/default_mossycobble.png",
	"farmland_dry": "farming/farming_soil.png", "farmland_wet": "farming/farming_soil_wet.png",
	"wheat_0": "farming/farming_wheat_1.png", "wheat_1": "farming/farming_wheat_2.png",
	"wheat_2": "farming/farming_wheat_3.png", "wheat_3": "farming/farming_wheat_4.png",
	"wheat_4": "farming/farming_wheat_5.png", "wheat_5": "farming/farming_wheat_6.png",
	"wheat_6": "farming/farming_wheat_7.png", "wheat_7": "farming/farming_wheat_8.png",
}

var pixelPerfectionLayers = map[string]uint16{
	"stone": LayerStone, "dirt": LayerDirt, "grass_top": LayerGrassTop, "grass_side": LayerGrassSide,
	"bedrock": LayerBedrock, "stone_brick": LayerStoneBrick, "furnace": LayerFurnace,
	"iron_block": LayerIronBlock, "leaves": LayerLeaves, "glass": LayerGlass,
	"cobblestone": LayerCobblestone, "sand": LayerSand, "gravel": LayerGravel,
	"oak_log_side": LayerOakLogSide, "oak_log_top": LayerOakLogTop, "oak_planks": LayerOakPlanks,
	"brick": LayerBrick, "white_wool": LayerWhiteWool, "clay": LayerClay, "snow_top": LayerSnowTop,
	"snow_side": LayerSnowSide, "mossy_cobblestone": LayerMossyCobblestone,
	"farmland_dry": LayerFarmlandDry, "farmland_wet": LayerFarmlandWet,
	"wheat_0": LayerWheat0, "wheat_1": LayerWheat1, "wheat_2": LayerWheat2, "wheat_3": LayerWheat3,
	"wheat_4": LayerWheat4, "wheat_5": LayerWheat5, "wheat_6": LayerWheat6, "wheat_7": LayerWheat7,
}

var proceduralFallbackLayers = []uint16{
	LayerCoalOre, LayerIronOre, LayerLightBlock, LayerRoofTile, LayerWater, LayerSmoothStone, LayerChest,
	LayerShortGrass, LayerCowHide, LayerCowHead,
}

// foodPackPage 是牛肉图标的上游页面：OpenGameArt 的 16x16 Food（作者
// ARoachIFoundOnMyPillow，CC0）。牛皮与牛头没有上游文件（原创程序化像素），
// 因此这里只登记生熟牛肉两层。
const (
	foodPackPage       = "https://opengameart.org/content/16x16-food"
	foodPackAuthor     = "ARoachIFoundOnMyPillow"
	foodPackLicenseID  = "CC0-1.0"
	foodPackLicenseURL = "https://creativecommons.org/publicdomain/zero/1.0/"
)

var foodPackSources = map[string]string{
	"raw_beef": "food/steak_raw.png", "cooked_beef": "food/steak_grilled.png",
}

var foodPackLayers = map[string]uint16{
	"raw_beef": LayerRawBeef, "cooked_beef": LayerCookedBeef,
}

type pixelPerfectionProvenance struct {
	UpstreamRepository string `json:"upstream_repository"`
	UpstreamCommit     string `json:"upstream_commit"`
	License            struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"license"`
	Modification string                           `json:"modification"`
	Files        []pixelPerfectionProvenanceEntry `json:"files"`
}

type pixelPerfectionProvenanceEntry struct {
	Destination string `json:"destination"`
	Source      string `json:"source"`
	// SourceURL 与 License 只出现在非 Pixel Perfection 上游的文件条目上
	// （牛肉图标的 OpenGameArt 页面与 CC0 协议）：Pixel Perfection 子集的
	// 33 个条目沿用顶层 upstream/license，不逐文件重复。
	SourceURL string `json:"source_url,omitempty"`
	License   *struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"license,omitempty"`
	SHA256 string `json:"sha256"`
	// Derived 记录逐文件的派生合成信息。内嵌默认包中的 grass_side.png 不再是
	// 上游的原始 overlay，而是把草缘合成到 dirt 之上得到的完全不透明图——
	// 派生必须被记录，不能假装是上游原样拷贝（TestOpaqueLayersAreFullyOpaque 与
	// 这里的断言共同锁住这一点）。
	Derived *pixelPerfectionProvenanceDerived `json:"derived,omitempty"`
}

// pixelPerfectionProvenanceDerived 描述一个派生材质文件由哪些上游素材如何合成而来。
type pixelPerfectionProvenanceDerived struct {
	Sources []string `json:"sources"`
	Note    string   `json:"note"`
}

func TestEmbeddedDefaultPackProvenance(t *testing.T) {
	root := os.DirFS("packs/pixel_perfection")
	for _, name := range []string{"pack.json", "ATTRIBUTION.md", "LICENSE.txt", "PROVENANCE.json"} {
		if _, err := fs.ReadFile(root, name); err != nil {
			t.Fatalf("读取必需元数据 %s: %v", name, err)
		}
	}

	manifestBytes, err := fs.ReadFile(root, "pack.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Format int    `json:"format"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("解析 pack.json: %v", err)
	}
	if manifest.Format != 1 || manifest.Name != "Pixel Perfection for Mornlea" {
		t.Fatalf("pack.json = %+v", manifest)
	}

	license, err := fs.ReadFile(root, "LICENSE.txt")
	if err != nil {
		t.Fatal(err)
	}
	licenseSum := sha256.Sum256(license)
	if got := hex.EncodeToString(licenseSum[:]); got != "091d08965bb70d444daccb62c5bcc4345cd4d6a65267da1f06564c95d25d9abb" {
		t.Fatalf("LICENSE.txt SHA-256 = %s", got)
	}

	attribution, err := fs.ReadFile(root, "ATTRIBUTION.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Hugh “XSSheep” Rutland", "Toby109tt", "tacotexmex", "devurandom",
		pixelPerfectionRepository, pixelPerfectionCommit, pixelPerfectionLicenseURL,
		"without pixel transformations",
	} {
		if !strings.Contains(string(attribution), required) {
			t.Errorf("ATTRIBUTION.md 缺少 %q", required)
		}
	}

	provenanceBytes, err := fs.ReadFile(root, "PROVENANCE.json")
	if err != nil {
		t.Fatal(err)
	}
	var provenance pixelPerfectionProvenance
	if err := json.Unmarshal(provenanceBytes, &provenance); err != nil {
		t.Fatalf("解析 PROVENANCE.json: %v", err)
	}
	if provenance.UpstreamRepository != pixelPerfectionRepository || provenance.UpstreamCommit != pixelPerfectionCommit {
		t.Errorf("upstream = %q@%q", provenance.UpstreamRepository, provenance.UpstreamCommit)
	}
	if provenance.License.ID != "CC-BY-SA-4.0" || provenance.License.URL != pixelPerfectionLicenseURL {
		t.Errorf("license = %+v", provenance.License)
	}
	if provenance.Modification != "Selected and renamed a subset without pixel transformations." {
		t.Errorf("modification = %q", provenance.Modification)
	}
	if len(provenance.Files) != len(pixelPerfectionSources)+len(foodPackSources) {
		t.Fatalf("provenance 文件数 = %d，想要 %d", len(provenance.Files), len(pixelPerfectionSources)+len(foodPackSources))
	}

	seen := make(map[string]bool, len(provenance.Files))
	for _, entry := range provenance.Files {
		logicalName := strings.TrimSuffix(path.Base(entry.Destination), ".png")
		wantSource, ok := pixelPerfectionSources[logicalName]
		_, isFood := foodPackSources[logicalName]
		if isFood {
			wantSource, ok = foodPackSources[logicalName], true
		}
		if !ok || entry.Destination != "textures/"+logicalName+".png" {
			t.Errorf("未知 destination %q", entry.Destination)
			continue
		}
		if seen[logicalName] {
			t.Errorf("重复 destination %q", entry.Destination)
			continue
		}
		seen[logicalName] = true
		if entry.Source != wantSource {
			t.Errorf("%s source = %q，想要 %q", logicalName, entry.Source, wantSource)
		}
		if logicalName == "grass_side" {
			if entry.Derived == nil {
				t.Errorf("grass_side 缺少 derived 派生记录")
			} else if len(entry.Derived.Sources) != 2 ||
				entry.Derived.Sources[0] != "default/default_grass_side.png" ||
				entry.Derived.Sources[1] != "default/default_dirt.png" {
				t.Errorf("grass_side derived.sources = %v，想要 [default/default_grass_side.png default/default_dirt.png]", entry.Derived.Sources)
			} else if entry.Derived.Note == "" {
				t.Errorf("grass_side derived.note 为空")
			}
		} else if entry.Derived != nil {
			t.Errorf("%s 出现了非预期的 derived 记录", logicalName)
		}
		if isFood {
			// 牛肉条目必须逐文件记清外网来源与协议：缺来源即拒绝入库。
			if entry.SourceURL != foodPackPage {
				t.Errorf("%s source_url = %q，想要 %q", logicalName, entry.SourceURL, foodPackPage)
			}
			if entry.License == nil || entry.License.ID != foodPackLicenseID || entry.License.URL != foodPackLicenseURL {
				t.Errorf("%s license = %+v，想要 %s/%s", logicalName, entry.License, foodPackLicenseID, foodPackLicenseURL)
			}
		} else if entry.SourceURL != "" || entry.License != nil {
			t.Errorf("%s 是 Pixel Perfection 子集文件，不得携带逐文件来源/协议", logicalName)
		}

		file, err := root.Open(entry.Destination)
		if err != nil {
			t.Errorf("打开 %s: %v", entry.Destination, err)
			continue
		}
		config, err := png.DecodeConfig(file)
		file.Close()
		if err != nil {
			t.Errorf("解码 %s: %v", entry.Destination, err)
			continue
		}
		if config.Width != 16 || config.Height != 16 {
			t.Errorf("%s 尺寸 = %dx%d，想要 16x16", entry.Destination, config.Width, config.Height)
		}
		data, err := fs.ReadFile(root, entry.Destination)
		if err != nil {
			t.Errorf("读取 %s: %v", entry.Destination, err)
			continue
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != entry.SHA256 {
			t.Errorf("%s SHA-256 = %s，provenance 记录 %s", entry.Destination, got, entry.SHA256)
		}
	}

	var pngFiles []string
	if err := fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(name, ".png") {
			pngFiles = append(pngFiles, name)
		}
		return nil
	}); err != nil {
		t.Fatalf("遍历内嵌 PNG: %v", err)
	}
	var wantPNGFiles []string
	for logicalName := range pixelPerfectionSources {
		wantPNGFiles = append(wantPNGFiles, "textures/"+logicalName+".png")
	}
	for logicalName := range foodPackSources {
		wantPNGFiles = append(wantPNGFiles, "textures/"+logicalName+".png")
	}
	slices.Sort(pngFiles)
	slices.Sort(wantPNGFiles)
	if !slices.Equal(pngFiles, wantPNGFiles) {
		t.Errorf("PNG 文件 = %v，想要 %v", pngFiles, wantPNGFiles)
	}
}

func TestEmbeddedDefaultPackLayersAndFallbacks(t *testing.T) {
	procedural := NewRegistry()
	embedded := NewDefaultRegistry()
	root, err := fs.Sub(defaultPackFS, "packs/pixel_perfection")
	if err != nil {
		t.Fatalf("打开内嵌默认包: %v", err)
	}
	for logicalName, layer := range pixelPerfectionLayers {
		data, err := fs.ReadFile(root, "textures/"+logicalName+".png")
		if err != nil {
			t.Fatalf("读取 %s: %v", logicalName, err)
		}
		if got, want := embedded.LayerRGBA(int(layer)), normalizePNGForTest(t, data); !bytes.Equal(got, want) {
			t.Errorf("%s layer %d 未使用内嵌 PNG", logicalName, layer)
		}
	}
	for logicalName, layer := range foodPackLayers {
		data, err := fs.ReadFile(root, "textures/"+logicalName+".png")
		if err != nil {
			t.Fatalf("读取 %s: %v", logicalName, err)
		}
		if got, want := embedded.LayerRGBA(int(layer)), normalizePNGForTest(t, data); !bytes.Equal(got, want) {
			t.Errorf("%s layer %d 未使用内嵌 PNG", logicalName, layer)
		}
	}
	for _, layer := range proceduralFallbackLayers {
		if got, want := embedded.LayerRGBA(int(layer)), procedural.LayerRGBA(int(layer)); !bytes.Equal(got, want) {
			t.Errorf("procedural fallback layer %d 被替换", layer)
		}
	}
	for _, registry := range []*Registry{procedural, embedded} {
		assertBinaryAlpha(t, registry.LayerRGBA(int(LayerLeaves)), "leaves")
		assertBinaryAlpha(t, registry.LayerRGBA(int(LayerGlass)), "glass")
	}
}

func TestDefaultRegistryAtlasIsStable(t *testing.T) {
	procedural := NewRegistry()
	first, second := NewDefaultRegistry(), NewDefaultRegistry()
	proceduralLayers, proceduralAtlas := atlasPixelsForTest(t, procedural)
	firstLayers, firstAtlas := atlasPixelsForTest(t, first)
	secondLayers, secondAtlas := atlasPixelsForTest(t, second)
	if firstLayers != proceduralLayers || len(firstAtlas) != len(proceduralAtlas) {
		t.Fatalf("default atlas = %d layers/%d bytes，procedural = %d layers/%d bytes",
			firstLayers, len(firstAtlas), proceduralLayers, len(proceduralAtlas))
	}
	if secondLayers != firstLayers || !bytes.Equal(secondAtlas, firstAtlas) {
		t.Fatal("两次 NewDefaultRegistry() 的 atlas 不一致")
	}
}

func atlasPixelsForTest(t *testing.T, registry *Registry) (int, []byte) {
	t.Helper()
	exporter, ok := any(registry).(interface{ AtlasPixels() (int, []byte) })
	if !ok {
		t.Skip("AtlasPixels 只存在于 darwin 客户端构建")
	}
	return exporter.AtlasPixels()
}

func TestEmbeddedDefaultPackMetadata(t *testing.T) {
	root, err := fs.Sub(defaultPackFS, "packs/pixel_perfection")
	if err != nil {
		t.Fatalf("打开内嵌默认包: %v", err)
	}
	for _, name := range []string{"pack.json", "ATTRIBUTION.md", "LICENSE.txt", "PROVENANCE.json"} {
		if data, err := fs.ReadFile(root, name); err != nil || len(data) == 0 {
			t.Errorf("内嵌元数据 %s: len=%d err=%v", name, len(data), err)
		}
	}
}

func TestRegistryWithOverrideUsesEmbeddedFallbackAndKeepsMapping(t *testing.T) {
	defaultRegistry := NewDefaultRegistry()
	pixels, encoded := solidPNG(t, 16, 16, color.NRGBA{R: 31, G: 79, B: 127, A: 128})
	registry, err := NewRegistryWithOverride(fstest.MapFS{
		"pack.json":           {Data: manifest(t, "用户覆盖")},
		"textures/leaves.png": {Data: encoded},
	})
	if err != nil {
		t.Fatalf("NewRegistryWithOverride() error = %v", err)
	}
	if registry == nil {
		t.Fatal("NewRegistryWithOverride() 返回 nil registry")
	}
	if got := registry.LayerRGBA(int(LayerLeaves)); !bytes.Equal(got, pixels) || got[3] != 128 {
		t.Fatal("用户中间 alpha override 未替换 leaves")
	}
	if got, want := registry.LayerRGBA(int(LayerStone)), defaultRegistry.LayerRGBA(int(LayerStone)); !bytes.Equal(got, want) {
		t.Fatal("用户缺失 stone 时未保留内嵌默认")
	}
	if got := registry.Material(core.LeavesID, mesh.FacePosX); got != LayerLeaves {
		t.Fatalf("leaves material = %d，想要 %d", got, LayerLeaves)
	}
	if registry.Opaque(core.LeavesID) || !isCutoutLayer(int(LayerLeaves)) {
		t.Fatal("用户像素改变了 leaves 的透明分类")
	}
	if !reflect.DeepEqual(registry.MeshSnapshot(), defaultRegistry.MeshSnapshot()) {
		t.Fatal("用户像素改变了 mesh registry snapshot")
	}
}

func TestRegistryWithOverrideRejectsInvalidPackWithoutRegistry(t *testing.T) {
	registry, err := NewRegistryWithOverride(fstest.MapFS{
		"pack.json":          {Data: manifest(t, "损坏覆盖")},
		"textures/stone.png": {Data: []byte("not a png")},
	})
	if err == nil {
		t.Fatal("NewRegistryWithOverride() error = nil")
	}
	if registry != nil {
		t.Fatal("NewRegistryWithOverride() 在失败时暴露了部分 registry")
	}
}

func normalizePNGForTest(t *testing.T, data []byte) []byte {
	t.Helper()
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码 PNG: %v", err)
	}
	rgba := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	draw.Draw(rgba, rgba.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	return rgba.Pix
}

func assertBinaryAlpha(t *testing.T, pixels []byte, name string) {
	t.Helper()
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] != 0 && pixels[i] != 255 {
			t.Fatalf("%s alpha[%d] = %d，想要 0 或 255", name, i/4, pixels[i])
		}
	}
}

// TestOpaqueLayersAreFullyOpaque 锁定"非 cutout 层必须全图不透明、cutout 层 alpha 必须二值"这条契约。
//
// 判据来源：packages/engine/crates/mornlea_client/shaders/terrain.wgsl 的片段着色器对
// alpha<0.5 的片段判 discard。不透明层（不在 `isCutoutLayer` 集合里）一旦带有透明
// 像素，这一整块面就会被着色器丢弃，方块面出现看穿/破洞（历史上 grass_side.png
// 就因此把草方块侧面的下半部整段丢弃）。cutout 层（leaves、glass、wheat_0..7）
// 允许 0 与 255 二值 alpha，由 mip 链的 `downsampleCutout` 保住覆盖率。
//
// 唯一的例外是 `LayerWater`：它不带进不透明 terrain pass，而是被按 material 分流到
// 半透明 water pass（见 render 的区段调度），在那里走 alpha blend 而非 cutout
// discard，因此允许固定的中间 alpha（`waterAlpha`=160），不适用本测试的"不透明层必须
// 全图不透明"约束，这里显式跳过。
func TestOpaqueLayersAreFullyOpaque(t *testing.T) {
	for _, tc := range []struct {
		name     string
		registry *Registry
	}{
		{"程序化", NewRegistry()},
		{"内嵌默认", NewDefaultRegistry()},
	} {
		for layer := 0; layer < tc.registry.LayerCount(); layer++ {
			if layer == int(LayerWater) {
				continue
			}
			pixels := tc.registry.LayerRGBA(layer)
			if len(pixels) != 16*16*4 {
				t.Fatalf("%s layer %d: 像素长度 = %d", tc.name, layer, len(pixels))
			}
			for i := 3; i < len(pixels); i += 4 {
				switch {
				case isCutoutLayer(layer):
					if pixels[i] != 0 && pixels[i] != 255 {
						t.Fatalf("%s layer %d (cutout): alpha[%d] = %d，想要 0 或 255", tc.name, layer, i/4, pixels[i])
					}
				default:
					if pixels[i] != 255 {
						t.Fatalf("%s layer %d (不透明): alpha[%d] = %d，想要 255", tc.name, layer, i/4, pixels[i])
					}
				}
			}
		}
	}
}

// TestEmbeddedBeefLayersAreDistinguishable 锁定内嵌牛肉图标的呈现契约：生熟
// 两层来自 OpenGameArt `16x16 Food` CC0 的不同文件，像素不同且可辨（生偏红、
// 熟偏棕），不得共用同一层。
func TestEmbeddedBeefLayersAreDistinguishable(t *testing.T) {
	embedded := NewDefaultRegistry()
	raw := embedded.LayerRGBA(int(LayerRawBeef))
	cooked := embedded.LayerRGBA(int(LayerCookedBeef))
	if bytes.Equal(raw, cooked) {
		t.Fatal("内嵌生熟牛肉逐像素相同：两者必须可辨")
	}
	rawAvg, cookedAvg := opaqueAverageForTest(raw), opaqueAverageForTest(cooked)
	if rawAvg[0] < rawAvg[2]+40 {
		t.Fatalf("内嵌生牛肉平均色 R=%d B=%d，想要偏红（R-B>=40）", rawAvg[0], rawAvg[2])
	}
	if cookedAvg[0]-cookedAvg[2] >= rawAvg[0]-rawAvg[2] {
		t.Fatalf("内嵌熟牛肉 R-B=%d 未低于生牛肉 R-B=%d：熟肉应偏棕",
			cookedAvg[0]-cookedAvg[2], rawAvg[0]-rawAvg[2])
	}
}

// TestEmbeddedCowLayersKeepProceduralFallback 锁定牛身两层的回退语义：默认包
// 不携带牛皮与牛头文件，程序化像素原样生效；一旦有人往包里塞同名文件，这里
// 会先于呈现变红。
func TestEmbeddedCowLayersKeepProceduralFallback(t *testing.T) {
	procedural := NewRegistry()
	embedded := NewDefaultRegistry()
	for _, layer := range []uint16{LayerCowHide, LayerCowHead} {
		if got, want := embedded.LayerRGBA(int(layer)), procedural.LayerRGBA(int(layer)); !bytes.Equal(got, want) {
			t.Fatalf("牛身层 %d 被默认包覆盖：牛身纹理必须只来自程序化生成路径", layer)
		}
	}
	root, err := fs.Sub(defaultPackFS, "packs/pixel_perfection")
	if err != nil {
		t.Fatalf("打开内嵌默认包: %v", err)
	}
	for _, name := range []string{"textures/cow_hide.png", "textures/cow_head.png"} {
		if _, err := fs.Stat(root, name); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("默认包不得携带 %s，stat error = %v", name, err)
		}
	}
}

// TestEmbeddedBeefAttribution 锁定牛肉图标的署名：作者、上游页面与 CC0 协议
// 缺一不可。
func TestEmbeddedBeefAttribution(t *testing.T) {
	root := os.DirFS("packs/pixel_perfection")
	attribution, err := fs.ReadFile(root, "ATTRIBUTION.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{foodPackAuthor, foodPackPage, foodPackLicenseURL, "raw_beef", "cooked_beef"} {
		if !strings.Contains(string(attribution), required) {
			t.Errorf("ATTRIBUTION.md 缺少 %q", required)
		}
	}
}

// pastelcraftDefaultPackName 是内嵌默认包在换肤后的产品名，与旧包
// `Pixel Perfection for Mornlea` 保持同形的命名习惯。
const pastelcraftDefaultPackName = "Pastelcraft for Mornlea"

// oldDefaultPackLayerSHA256 记录换肤前旧默认包在泥土系槽位的像素哈希：
// 换肤后这些槽位必须不再等于旧字节，测试据此变绿。
var oldDefaultPackLayerSHA256 = map[string]string{
	"dirt":         "eee24a5d59308ddce118f1639ce522cd9ce742f5c2408399d60a2513a65a8025",
	"grass_top":    "e966bdf4896a46bbcf32d79fb58320d82ed5fce102f8bf58a2a8ea3ceb337e0d",
	"grass_side":   "4d7db76883c7cfbdb288c79d1e02a5527010f676651eef7a0402f4feb1815124",
	"farmland_dry": "c9e41957286704e581e4ef9d03f5e5e938ad1e34d2052b2c631fa5f99d66ad09",
	"farmland_wet": "c68ccbc0205433812fab9c536960addd5aa79bcb3a8ed8eddeac3d30dbab87fe",
	"sand":         "9f2ac41887cb36914d941c11f9482a3a8109ec2d8fb8db428690d55427be7450",
	"gravel":       "62ec6c921a393ee1fd9772b7668899bb975953abeab2482493e81714e49374de",
	"clay":         "dbf7f23818019190c1c3b87c3d66e0e0097a7d5fc68f57796914dc0466dc7181",
}

// pastelcraftDirtLayers 是换肤后必须来自新包的泥土系槽位集合。
var pastelcraftDirtLayers = map[string]uint16{
	"dirt": LayerDirt, "grass_top": LayerGrassTop, "grass_side": LayerGrassSide,
	"farmland_dry": LayerFarmlandDry, "farmland_wet": LayerFarmlandWet,
	"sand": LayerSand, "gravel": LayerGravel, "clay": LayerClay,
}

// TestEmbeddedDefaultPackIsPastelcraft 锁定内嵌默认包的产品外观基线：包名已换成
// 新包、泥土系槽位既不再是程序化像素也不再是旧包字节。换肤实现落地前保持红色。
func TestEmbeddedDefaultPackIsPastelcraft(t *testing.T) {
	root, err := fs.Sub(defaultPackFS, "packs/pastelcraft")
	if err != nil {
		t.Fatalf("打开内嵌默认包 packs/pastelcraft: %v", err)
	}
	manifestBytes, err := fs.ReadFile(root, "pack.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Format int    `json:"format"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("解析 pack.json: %v", err)
	}
	if manifest.Format != 1 || manifest.Name != pastelcraftDefaultPackName {
		t.Fatalf("pack.json = %+v，想要 format=1 name=%q", manifest, pastelcraftDefaultPackName)
	}
	procedural := NewRegistry()
	embedded := NewDefaultRegistry()
	for logicalName, layer := range pastelcraftDirtLayers {
		if got, want := embedded.LayerRGBA(int(layer)), procedural.LayerRGBA(int(layer)); bytes.Equal(got, want) {
			t.Errorf("%s 仍是程序化像素：换肤后必须来自内嵌新包", logicalName)
		}
		sum := sha256.Sum256(embedded.LayerRGBA(int(layer)))
		if got := hex.EncodeToString(sum[:]); got == oldDefaultPackLayerSHA256[logicalName] {
			t.Errorf("%s 仍是旧包字节 %s：换肤后必须变化", logicalName, got)
		}
	}
}

// opaqueAverageForTest 返回一层材质不透明像素的平均 RGB，用于比较色相。
func opaqueAverageForTest(px []byte) [3]int {
	var sum [3]int
	count := 0
	for i := 0; i < len(px); i += 4 {
		if px[i+3] == 0 {
			continue
		}
		sum[0] += int(px[i])
		sum[1] += int(px[i+1])
		sum[2] += int(px[i+2])
		count++
	}
	if count == 0 {
		return sum
	}
	for i := range sum {
		sum[i] /= count
	}
	return sum
}
