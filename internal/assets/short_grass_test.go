package assets

import (
	"bytes"
	"errors"
	"image/color"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/mesh"
)

const (
	expectedShortGrassBlock core.BlockID = 84
	expectedShortGrassLayer uint16       = 68
)

func TestShortGrassUsesAppendedProceduralCutoutLayer(t *testing.T) {
	registry := NewRegistry()
	if got := registry.LayerCount(); got != int(LayerCrack9)+1 {
		t.Fatalf("LayerCount = %d，想要覆盖短草与裂纹追加层后的 %d", got, int(LayerCrack9)+1)
	}
	if got, want := len(registry.MeshSnapshot().Blocks), 85; got != want {
		t.Fatalf("mesh registry 条目 = %d，想要 %d", got, want)
	}
	if LayerDoor != 55 || LayerWorkbenchTop != 56 || LayerWorkbenchSide != 57 ||
		LayerWorkbenchBottom != 58 || LayerTorch != 59 ||
		LayerBedFootSouth != 60 || LayerBedHeadEast != 67 {
		t.Fatalf("既有材质层 55..67 被移动：door=%d workbench=%d/%d/%d torch=%d bed=%d..%d",
			LayerDoor, LayerWorkbenchTop, LayerWorkbenchSide, LayerWorkbenchBottom,
			LayerTorch, LayerBedFootSouth, LayerBedHeadEast)
	}
	if !mesh.PlantMaterial(expectedShortGrassLayer) {
		t.Fatalf("短草层 %d 未进入植物材质集合", expectedShortGrassLayer)
	}
	for layer := uint16(55); layer <= 67; layer++ {
		if mesh.PlantMaterial(layer) {
			t.Fatalf("既有非植物层 %d 被连续区间扩张误判为植物", layer)
		}
	}
	for face := mesh.Face(0); face < 6; face++ {
		if got := registry.Material(expectedShortGrassBlock, face); got != expectedShortGrassLayer {
			t.Fatalf("短草 face %d material = %d，想要 %d", face, got, expectedShortGrassLayer)
		}
	}
	for _, adjacent := range []core.BlockID{core.AirID, core.GlassID, core.WaterSourceID, core.StoneID} {
		if registry.FaceVisible(expectedShortGrassBlock, adjacent) {
			t.Fatalf("短草朝相邻方块 %d 产生了轴向面", adjacent)
		}
	}
	if registry.Opaque(expectedShortGrassBlock) {
		t.Fatal("短草不得作为完整遮光方块")
	}
	if !isCutoutLayer(int(expectedShortGrassLayer)) {
		t.Fatal("短草层未进入 cutout mip 路径")
	}

	pixels := registry.LayerRGBA(int(expectedShortGrassLayer))
	if len(pixels) != 16*16*4 {
		t.Fatalf("短草程序化纹理长度 = %d，想要 %d", len(pixels), 16*16*4)
	}
	opaque, transparent := 0, 0
	for index := 3; index < len(pixels); index += 4 {
		switch pixels[index] {
		case 0:
			transparent++
		case 255:
			opaque++
		default:
			t.Fatalf("短草 alpha[%d] = %d，想要 0 或 255", index/4, pixels[index])
		}
	}
	if opaque == 0 || transparent == 0 {
		t.Fatalf("短草程序化纹理不完整：不透明像素=%d，透明像素=%d", opaque, transparent)
	}
}

func TestShortGrassPackOverrideAndDefaultFallback(t *testing.T) {
	procedural := NewRegistry()
	defaults := NewDefaultRegistry()
	if got, want := defaults.LayerRGBA(int(expectedShortGrassLayer)), procedural.LayerRGBA(int(expectedShortGrassLayer)); !bytes.Equal(got, want) {
		t.Fatal("默认包缺少 short_grass.png 时未保留程序化 fallback")
	}
	root, err := fs.Sub(defaultPackFS, "packs/pixel_perfection")
	if err != nil {
		t.Fatalf("打开默认材质包: %v", err)
	}
	if _, err := fs.Stat(root, "textures/short_grass.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("默认包不得携带 textures/short_grass.png，stat error = %v", err)
	}

	want, encoded := solidPNG(t, 16, 16, color.NRGBA{R: 24, G: 180, B: 72, A: 255})
	overridden, err := NewRegistryWithOverride(fstest.MapFS{
		"pack.json":                {Data: manifest(t, "短草覆盖测试包")},
		"textures/short_grass.png": {Data: encoded},
	})
	if err != nil {
		t.Fatalf("NewRegistryWithOverride() error = %v", err)
	}
	if got := overridden.LayerRGBA(int(expectedShortGrassLayer)); !bytes.Equal(got, want) {
		t.Fatal("用户 textures/short_grass.png 未覆盖短草层")
	}
}
