package render

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
)

// TestPassiveEntityKeyUsesU64IdentityInDistinctDomain 锁定被动牛实体键：
// 独立于玩家/伙伴/目标名牌/夜行者的身份域，16-byte 键槽位以 little-endian
// 写入 u64 ID；不同 ID 产出不同键，与相同 bytes 的其他域不冲突。
func TestPassiveEntityKeyUsesU64IdentityInDistinctDomain(t *testing.T) {
	if EntityPassive != 5 {
		t.Fatalf("EntityPassive=%d，想要 5（1 玩家、2 伙伴、3 目标名牌、4 夜行者已被占用）", EntityPassive)
	}
	first := PassiveEntityKey(0x0102030405060708)
	second := PassiveEntityKey(0x0102030405060709)
	if first == second {
		t.Fatal("不同 u64 ID 产出了相同实体键")
	}
	if first.ID[0] != 0x08 || first.ID[7] != 0x01 {
		t.Fatalf("16-byte key 低段=%v，想要 little-endian u64", first.ID[:8])
	}
	for _, value := range first.ID[8:] {
		if value != 0 {
			t.Fatalf("16-byte key 高段=%v，想要零", first.ID[8:])
		}
	}
	for _, other := range []EntityKey{
		{Kind: EntityPlayer, ID: first.ID},
		{Kind: EntityCompanion, ID: first.ID},
		{Kind: EntityHostile, ID: first.ID},
	} {
		if compareEntityKeys(first, other) == 0 {
			t.Fatalf("被动牛键与相同 bytes 的 %v 键冲突", other.Kind)
		}
	}
}

// TestPassiveAvatarUsesQuadrupedTexturedProportions 锁定牛的四足体型：横向
// 躯干 + 4 短腿 + 头部共 6 部件，与夜行者直立骨架一眼可辨；身体锚定在站位
// Y（脚底），局部仍保持轴对齐。
func TestPassiveAvatarUsesQuadrupedTexturedProportions(t *testing.T) {
	key := PassiveEntityKey(7)
	parts := buildAvatarParts(nil, []Avatar{{Key: key, Position: mgl32.Vec3{4, 5, 6}}})
	if got, want := len(parts), avatarPartsPerBody; got != want {
		t.Fatalf("parts=%d want=%d", got, want)
	}
	sizes := make([]mgl32.Vec3, 0, avatarPartsPerBody)
	for _, part := range parts {
		bounds := transformedUnitCubeBounds(part.transform)
		sizes = append(sizes, bounds.max.Sub(bounds.min))
		assertAxisAligned(t, part.transform)
	}
	// 躯干横向：长度大于高度，与直立躯干（0.4 宽、0.7 高）不同构。
	if sizes[1][0] <= sizes[1][1] {
		t.Fatalf("牛躯干尺寸 %v 不是横向（长大于高）", sizes[1])
	}
	if sizes[1][0] <= 0.6 {
		t.Fatalf("牛躯干长度 %v 未大于玩家躯干宽度 0.4", sizes[1][0])
	}
	// 四条腿短于直立腿（玩家腿 0.7、夜行者腿 0.55）。
	for leg := 2; leg < avatarPartsPerBody; leg++ {
		if sizes[leg][1] >= 0.7 {
			t.Fatalf("牛腿 %d 高度 %v 未短于直立腿", leg, sizes[leg][1])
		}
	}
	// 全高低于直立体（玩家 1.8），轮廓一眼可辨。
	bounds := avatarPartsBounds(parts)
	if height := bounds.max[1] - bounds.min[1]; height >= 1.8 {
		t.Fatalf("牛全高 %v 未低于直立体 1.8", height)
	}
	if bounds.min[1] < 5-1e-4 || bounds.min[1] > 5+1e-4 {
		t.Fatalf("牛包围盒下缘=%v，想要站位 Y=5", bounds.min[1])
	}
}

// TestPassiveAvatarBodySamplesHideAndHead 锁定牛身六面的材质采样：头部采
// 牛头层，其余五面采牛皮层；层号按名引用，绝不硬编码数字。
func TestPassiveAvatarBodySamplesHideAndHead(t *testing.T) {
	parts := buildAvatarParts(nil, []Avatar{{Key: PassiveEntityKey(3)}})
	if got, want := len(parts), avatarPartsPerBody; got != want {
		t.Fatalf("parts=%d want=%d", got, want)
	}
	if parts[0].material != uint32(assets.LayerCowHead) {
		t.Fatalf("牛头材质=%d，想要按名引用的牛头层 %d", parts[0].material, uint32(assets.LayerCowHead))
	}
	for index := 1; index < len(parts); index++ {
		if parts[index].material != uint32(assets.LayerCowHide) {
			t.Fatalf("牛身部件 %d 材质=%d，想要按名引用的牛皮层 %d", index, parts[index].material, uint32(assets.LayerCowHide))
		}
	}
}

// TestAvatarSolidBranchesUseSentinelMaterial 锁定纯色分支的哨兵材质：玩家/
// 伙伴/夜行者各部件传哨兵走原纯色路径，像素逐字节不变的新测试锁。
func TestAvatarSolidBranchesUseSentinelMaterial(t *testing.T) {
	avatars := []Avatar{
		{Key: testEntityKey(testAvatarID(1))},
		{Key: EntityKey{Kind: EntityCompanion, ID: [16]byte(testAvatarID(2))}},
		{Key: HostileEntityKey(9)},
	}
	parts := buildAvatarParts(nil, avatars)
	if got, want := len(parts), 3*avatarPartsPerBody; got != want {
		t.Fatalf("parts=%d want=%d", got, want)
	}
	for index, part := range parts {
		if part.material != avatarMaterialSolid {
			t.Fatalf("纯色部件 %d 材质=%d，想要哨兵 %d", index, part.material, avatarMaterialSolid)
		}
	}
	if avatarMaterialSolid != ^uint32(0) {
		t.Fatalf("哨兵=%d，想要与有效层号不相交的 ^uint32(0)", avatarMaterialSolid)
	}
}

// TestAvatarInstanceLayoutIs96Bytes 锁定跨语言实例布局：mat4（0..64）+
// 颜色（64..80）+ 材质 u32（80..84）+ 12 字节保留零填充，总 96 字节；
// 固定容量 75 具身体 × 6 部件 = 450 实例不变。
func TestAvatarInstanceLayoutIs96Bytes(t *testing.T) {
	if avatarInstanceBytes != 96 {
		t.Fatalf("avatarInstanceBytes=%d，想要 96（64 mat4 + 16 color + 4 material + 12 保留）", avatarInstanceBytes)
	}
	if maxAvatars != 75 {
		t.Fatalf("maxAvatars=%d，想要 75", maxAvatars)
	}
	parts := buildAvatarParts(nil, makeTestAvatars(maxAvatars))
	if got, want := len(parts), 450; got != want {
		t.Fatalf("parts=%d want=%d", got, want)
	}
	stream := avatarPartBytes(parts)
	if got, want := len(stream), 450*96; got != want {
		t.Fatalf("实例流=%d bytes，想要 450×96=%d", got, want)
	}
	// 首个纯色实例：颜色逐字节可解，材质为哨兵，保留区全零。
	base := AvatarColor(testAvatarID(1))
	head := avatarShade(base, 1.12)
	for index, want := range head {
		if got := math.Float32frombits(binary.LittleEndian.Uint32(stream[64+index*4:])); got != want {
			t.Fatalf("首实例颜色通道 %d=%v，想要 %v", index, got, want)
		}
	}
	if got := binary.LittleEndian.Uint32(stream[80:]); got != avatarMaterialSolid {
		t.Fatalf("首实例材质=%d，想要哨兵 %d", got, avatarMaterialSolid)
	}
	for offset := 84; offset < 96; offset++ {
		if stream[offset] != 0 {
			t.Fatalf("首实例保留字节 %d=%d，想要零", offset, stream[offset])
		}
	}
	// 牛实例：头部材质为牛头层，保留区同样全零。
	cattle := avatarPartBytes(buildAvatarParts(nil, []Avatar{{Key: PassiveEntityKey(1)}}))
	if got := binary.LittleEndian.Uint32(cattle[80:]); got != uint32(assets.LayerCowHead) {
		t.Fatalf("牛头实例材质=%d，想要牛头层 %d", got, uint32(assets.LayerCowHead))
	}
	for offset := 84; offset < 96; offset++ {
		if cattle[offset] != 0 {
			t.Fatalf("牛头实例保留字节 %d=%d，想要零", offset, cattle[offset])
		}
	}
}
