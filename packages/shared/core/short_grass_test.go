package core_test

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

const expectedShortGrassID core.BlockID = 84

func TestShortGrassStableBlockIdentityHasNoItem(t *testing.T) {
	if got := core.ShortGrassID; got != expectedShortGrassID {
		t.Fatalf("ShortGrassID = %d，想要 %d", got, expectedShortGrassID)
	}
	if got, want := core.BlockIDMax, core.BlockID(85); got != want {
		t.Fatalf("BlockIDMax = %d，想要只追加短草后的 %d", got, want)
	}
	if !core.RegisteredBlock(expectedShortGrassID) {
		t.Fatalf("短草编号 %d 未注册", expectedShortGrassID)
	}
	if name, ok := core.BlockDisplayName(expectedShortGrassID); !ok || name != "短草" {
		t.Fatalf("BlockDisplayName(%d) = (%q,%v)，想要 (短草,true)", expectedShortGrassID, name, ok)
	}
	if core.IsCrop(expectedShortGrassID) {
		t.Fatal("短草不得被归类为作物")
	}
	if got, ok := core.BlockDrop(expectedShortGrassID); ok || got != core.ItemNone {
		t.Fatalf("BlockDrop(短草) = (%d,%v)，短草不得登记通用掉落", got, ok)
	}
	if got, want := core.ItemIDMax, core.ItemID(53); got != want {
		t.Fatalf("ItemIDMax = %d，想要保持 %d：短草不得追加物品编号", got, want)
	}
	faces := [...]core.BlockFace{
		core.BlockFaceNegX, core.BlockFacePosX, core.BlockFaceNegY,
		core.BlockFacePosY, core.BlockFaceNegZ, core.BlockFacePosZ,
	}
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		if block, ok := core.ItemPlacement(item); ok && block == expectedShortGrassID {
			t.Fatalf("ItemPlacement(%d) 可以放置短草", item)
		}
		for _, face := range faces {
			if block, ok := core.PlaceableBlockAtFace(item, face); ok && block == expectedShortGrassID {
				t.Fatalf("PlaceableBlockAtFace(%d,%d) 可以放置短草", item, face)
			}
		}
	}
}

func TestPlantPredicatesKeepCropsAndWildGrassDistinct(t *testing.T) {
	for id := core.BlockID(0); id < core.BlockIDMax; id++ {
		wantCrop := core.IsCrop(id)
		wantWildGrass := id == core.ShortGrassID
		if got := core.IsWildGrass(id); got != wantWildGrass {
			t.Fatalf("IsWildGrass(%d) = %v，想要 %v", id, got, wantWildGrass)
		}
		if got, want := core.IsPlant(id), wantCrop || wantWildGrass; got != want {
			t.Fatalf("IsPlant(%d) = %v，想要 %v", id, got, want)
		}
	}
}

func TestShortGrassIsTransparentButRaycastable(t *testing.T) {
	if core.BlockOpaque(expectedShortGrassID) {
		t.Fatal("短草不得作为完整遮光方块")
	}
	if !core.InteractionTarget(expectedShortGrassID) {
		t.Fatal("短草必须是权威交互射线目标")
	}

	want := core.BlockPos{X: 2, Y: 3, Z: 4}
	hit, ok, err := core.RaycastBlocks(
		mgl32.Vec3{0.5, 3.5, 4.5},
		mgl32.Vec3{1, 0, 0},
		4,
		func(position core.BlockPos) (bool, error) {
			block := core.AirID
			if position == want {
				block = expectedShortGrassID
			}
			return core.InteractionTarget(block), nil
		},
	)
	if err != nil || !ok || hit.Block != want {
		t.Fatalf("短草射线命中 = (%+v,%v,%v)，想要命中 %+v", hit, ok, err, want)
	}
}
