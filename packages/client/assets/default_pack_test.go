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
	pastelcraftProjectPage = "https://modrinth.com/resourcepack/pastelcraft"
	pastelcraftAuthor      = "XradicalD"
	pastelcraftVersion     = "Pastelcraft 1.21.11 [R21]"
	pastelcraftVersionID   = "wE5aqTkH"
	pastelcraftDownloadURL = "https://cdn.modrinth.com/data/NmJMsiNC/versions/wE5aqTkH/Pastelcraft%201.21.11%20R21.zip"
	pastelcraftZIPSHA256   = "6cb32d1eb30fa7db4e683cda8f9f24de666b09f819badd8a609c66167a58ebcb"
	pastelcraftLicenseURL  = "https://opensource.org/licenses/MIT"
)

// pastelcraftSources 把内嵌槽位名映射到 ZIP 内文件名（`assets/minecraft/textures/block/`
// 前缀此处省略，`PROVENANCE.json` 逐条记录完整路径）。逐槽位照抄换肤映射表；
// `→ 回退` 的槽位不得用别图硬凑，因此不在此表出现。
var pastelcraftSources = map[string]string{
	"stone": "stone.png", "dirt": "dirt.png",
	"grass_side": "grass_block_side.png",
	"bedrock":    "bedrock.png", "stone_brick": "stone_bricks.png",
	"coal_ore": "coal_ore.png", "iron_ore": "iron_ore.png",
	"furnace": "furnace_front.png", "iron_block": "iron_block.png",
	"glass":       "glass.png",
	"cobblestone": "cobblestone.png", "smooth_stone": "smooth_stone.png",
	"sand": "sand.png", "gravel": "gravel.png",
	"oak_log_side": "oak_log.png", "oak_log_top": "oak_log_top.png",
	"oak_planks": "oak_planks.png", "brick": "bricks.png",
	"white_wool": "white_wool.png", "clay": "clay.png",
	"snow_top": "snow.png", "snow_side": "snow.png",
	"mossy_cobblestone": "mossy_cobblestone.png",
	"farmland_dry":      "farmland.png", "farmland_wet": "farmland_moist.png",
	"wheat_0": "wheat_stage0.png", "wheat_1": "wheat_stage1.png",
	"wheat_2": "wheat_stage2.png", "wheat_3": "wheat_stage3.png",
	"wheat_4": "wheat_stage4.png", "wheat_5": "wheat_stage5.png",
	"wheat_6": "wheat_stage6.png", "wheat_7": "wheat_stage7.png",
	"potato_0": "potatoes_stage0.png", "potato_1": "potatoes_stage1.png",
	"potato_2": "potatoes_stage2.png", "potato_3": "potatoes_stage3.png",
	"carrot_0": "carrots_stage0.png", "carrot_1": "carrots_stage1.png",
	"carrot_2": "carrots_stage2.png", "carrot_3": "carrots_stage3.png",
	"workbench_top": "crafting_table_top.png", "workbench_side": "crafting_table_side.png",
	"torch":   "torch.png",
	"crack_0": "destroy_stage_0.png", "crack_1": "destroy_stage_1.png",
	"crack_2": "destroy_stage_2.png", "crack_3": "destroy_stage_3.png",
	"crack_4": "destroy_stage_4.png", "crack_5": "destroy_stage_5.png",
	"crack_6": "destroy_stage_6.png", "crack_7": "destroy_stage_7.png",
	"crack_8": "destroy_stage_8.png", "crack_9": "destroy_stage_9.png",
}

var pastelcraftLayers = map[string]uint16{
	"stone": LayerStone, "dirt": LayerDirt, "grass_side": LayerGrassSide,
	"bedrock": LayerBedrock, "stone_brick": LayerStoneBrick,
	"coal_ore": LayerCoalOre, "iron_ore": LayerIronOre,
	"furnace": LayerFurnace, "iron_block": LayerIronBlock,
	"glass":       LayerGlass,
	"cobblestone": LayerCobblestone, "smooth_stone": LayerSmoothStone,
	"sand": LayerSand, "gravel": LayerGravel,
	"oak_log_side": LayerOakLogSide, "oak_log_top": LayerOakLogTop, "oak_planks": LayerOakPlanks,
	"brick": LayerBrick, "white_wool": LayerWhiteWool, "clay": LayerClay,
	"snow_top": LayerSnowTop, "snow_side": LayerSnowSide, "mossy_cobblestone": LayerMossyCobblestone,
	"farmland_dry": LayerFarmlandDry, "farmland_wet": LayerFarmlandWet,
	"wheat_0": LayerWheat0, "wheat_1": LayerWheat1, "wheat_2": LayerWheat2, "wheat_3": LayerWheat3,
	"wheat_4": LayerWheat4, "wheat_5": LayerWheat5, "wheat_6": LayerWheat6, "wheat_7": LayerWheat7,
	"potato_0": LayerPotato0, "potato_1": LayerPotato1, "potato_2": LayerPotato2, "potato_3": LayerPotato3,
	"carrot_0": LayerCarrot0, "carrot_1": LayerCarrot1, "carrot_2": LayerCarrot2, "carrot_3": LayerCarrot3,
	"workbench_top": LayerWorkbenchTop, "workbench_side": LayerWorkbenchSide,
	"torch":   LayerTorch,
	"crack_0": LayerCrack0, "crack_1": LayerCrack1, "crack_2": LayerCrack2, "crack_3": LayerCrack3,
	"crack_4": LayerCrack4, "crack_5": LayerCrack5, "crack_6": LayerCrack6, "crack_7": LayerCrack7,
	"crack_8": LayerCrack8, "crack_9": LayerCrack9,
}

var proceduralFallbackLayers = []uint16{
	LayerGrassTop, LayerLeaves,
	LayerChest, LayerLightBlock, LayerRoofTile, LayerWater,
	LayerPotato4, LayerPotato5, LayerPotato6, LayerPotato7,
	LayerCarrot4, LayerCarrot5, LayerCarrot6, LayerCarrot7,
	LayerDoor, LayerWorkbenchBottom, LayerShortGrass,
	LayerBedFootSouth, LayerBedFootWest, LayerBedFootNorth, LayerBedFootEast,
	LayerBedHeadSouth, LayerBedHeadWest, LayerBedHeadNorth, LayerBedHeadEast,
	LayerCowHide, LayerCowHead,
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

type pastelcraftProvenance struct {
	UpstreamProject   string `json:"upstream_project"`
	UpstreamAuthor    string `json:"upstream_author"`
	UpstreamVersion   string `json:"upstream_version"`
	UpstreamVersionID string `json:"upstream_version_id"`
	DownloadURL       string `json:"download_url"`
	ZIPSHA256         string `json:"zip_sha256"`
	License           struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"license"`
	Modification string                       `json:"modification"`
	Files        []pastelcraftProvenanceEntry `json:"files"`
}

type pastelcraftProvenanceEntry struct {
	Destination string `json:"destination"`
	Source      string `json:"source"`
	// SourceURL 与 License 只出现在非 Pastelcraft 上游的文件条目上
	// （牛肉图标的 OpenGameArt 页面与 CC0 协议）：Pastelcraft 子集的
	// 54 个条目沿用顶层 upstream/license，不逐文件重复。
	SourceURL string `json:"source_url,omitempty"`
	License   *struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"license,omitempty"`
	SHA256 string `json:"sha256"`
}

func TestEmbeddedDefaultPackProvenance(t *testing.T) {
	root := os.DirFS("packs/pastelcraft")
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
	if manifest.Format != 1 || manifest.Name != pastelcraftDefaultPackName {
		t.Fatalf("pack.json = %+v", manifest)
	}

	license, err := fs.ReadFile(root, "LICENSE.txt")
	if err != nil {
		t.Fatal(err)
	}
	licenseSum := sha256.Sum256(license)
	if got := hex.EncodeToString(licenseSum[:]); got != "8bf97eb82f6ae8f62059b8142d77fa8d7911c16eff29ea4c1de087324fb54b67" {
		t.Fatalf("LICENSE.txt SHA-256 = %s", got)
	}

	attribution, err := fs.ReadFile(root, "ATTRIBUTION.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		pastelcraftAuthor, "Square Dreams",
		pastelcraftProjectPage, pastelcraftVersion, pastelcraftVersionID,
		pastelcraftDownloadURL, pastelcraftZIPSHA256, pastelcraftLicenseURL,
		"without pixel transformations",
		"ZIP 内未附许可文本，许可依据 Modrinth 页面/API 的 MIT 声明",
	} {
		if !strings.Contains(string(attribution), required) {
			t.Errorf("ATTRIBUTION.md 缺少 %q", required)
		}
	}

	provenanceBytes, err := fs.ReadFile(root, "PROVENANCE.json")
	if err != nil {
		t.Fatal(err)
	}
	var provenance pastelcraftProvenance
	if err := json.Unmarshal(provenanceBytes, &provenance); err != nil {
		t.Fatalf("解析 PROVENANCE.json: %v", err)
	}
	if provenance.UpstreamProject != pastelcraftProjectPage || provenance.UpstreamAuthor != pastelcraftAuthor ||
		provenance.UpstreamVersion != pastelcraftVersion || provenance.UpstreamVersionID != pastelcraftVersionID {
		t.Errorf("upstream = %q %q %q@%q", provenance.UpstreamProject, provenance.UpstreamAuthor,
			provenance.UpstreamVersion, provenance.UpstreamVersionID)
	}
	if provenance.DownloadURL != pastelcraftDownloadURL || provenance.ZIPSHA256 != pastelcraftZIPSHA256 {
		t.Errorf("download = %q zip = %q", provenance.DownloadURL, provenance.ZIPSHA256)
	}
	if provenance.License.ID != "MIT" || provenance.License.URL != pastelcraftLicenseURL {
		t.Errorf("license = %+v", provenance.License)
	}
	if provenance.Modification != "Selected and renamed a subset without pixel transformations." {
		t.Errorf("modification = %q", provenance.Modification)
	}
	if len(provenance.Files) != len(pastelcraftSources)+len(foodPackSources) {
		t.Fatalf("provenance 文件数 = %d，想要 %d", len(provenance.Files), len(pastelcraftSources)+len(foodPackSources))
	}

	seen := make(map[string]bool, len(provenance.Files))
	for _, entry := range provenance.Files {
		logicalName := strings.TrimSuffix(path.Base(entry.Destination), ".png")
		wantSource, ok := pastelcraftSources[logicalName]
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
		// Pastelcraft 子集条目记录 ZIP 内完整路径，牛肉条目沿用旧包的相对路径。
		if !isFood {
			wantSource = "assets/minecraft/textures/block/" + wantSource
		}
		if entry.Source != wantSource {
			t.Errorf("%s source = %q，想要 %q", logicalName, entry.Source, wantSource)
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
			t.Errorf("%s 是 Pastelcraft 子集文件，不得携带逐文件来源/协议", logicalName)
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
	for logicalName := range pastelcraftSources {
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
	root, err := fs.Sub(defaultPackFS, "packs/pastelcraft")
	if err != nil {
		t.Fatalf("打开内嵌默认包: %v", err)
	}
	for logicalName, layer := range pastelcraftLayers {
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
	root, err := fs.Sub(defaultPackFS, "packs/pastelcraft")
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
	root, err := fs.Sub(defaultPackFS, "packs/pastelcraft")
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
	root := os.DirFS("packs/pastelcraft")
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
	"grass_side":   "4d7db76883c7cfbdb288c79d1e02a5527010f676651eef7a0402f4feb1815124",
	"farmland_dry": "c9e41957286704e581e4ef9d03f5e5e938ad1e34d2052b2c631fa5f99d66ad09",
	"farmland_wet": "c68ccbc0205433812fab9c536960addd5aa79bcb3a8ed8eddeac3d30dbab87fe",
	"sand":         "9f2ac41887cb36914d941c11f9482a3a8109ec2d8fb8db428690d55427be7450",
	"gravel":       "62ec6c921a393ee1fd9772b7668899bb975953abeab2482493e81714e49374de",
	"clay":         "dbf7f23818019190c1c3b87c3d66e0e0097a7d5fc68f57796914dc0466dc7181",
}

// pastelcraftDirtLayers 是换肤后必须来自新包的泥土系槽位集合（草顶为需
// 染色灰度图已退回程序化，不在此列）。
var pastelcraftDirtLayers = map[string]uint16{
	"dirt": LayerDirt, "grass_side": LayerGrassSide,
	"farmland_dry": LayerFarmlandDry, "farmland_wet": LayerFarmlandWet,
	"sand": LayerSand, "gravel": LayerGravel, "clay": LayerClay,
}

// TestEmbeddedDefaultPackIsPastelcraft 锁定内嵌默认包的产品外观基线：包名已换成
// 新包、泥土系 7 槽位既不再是程序化像素也不再是旧包字节；草顶是需染色灰度图
// 已退回程序化，这里反向断言它与程序化逐字节一致。
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
	if got, want := embedded.LayerRGBA(int(LayerGrassTop)), procedural.LayerRGBA(int(LayerGrassTop)); !bytes.Equal(got, want) {
		t.Errorf("grass_top 未退回程序化：需染色灰度图不得入库")
	}
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
