package render

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// TestAvatarCapacityAcceptsSeventyFiveBodies 锁定 75 体/450 实例的帧容量：
// 恰好 75 具身体必须全部编码（450 个 80-byte instance），上传缓冲的字节
// 布局与 Rust 侧 `AVATAR_MAX_INSTANCES` 同源同步。
func TestAvatarCapacityAcceptsSeventyFiveBodies(t *testing.T) {
	avatars := makeTestAvatars(maxAvatars)
	if maxAvatars != 75 {
		t.Fatalf("maxAvatars=%d，想要 75", maxAvatars)
	}
	parts := buildAvatarParts(nil, avatars)
	if got, want := len(parts), 450; got != want {
		t.Fatalf("parts=%d want=%d", got, want)
	}
	if got := len(parts) * avatarInstanceBytes; got != 36000 {
		t.Fatalf("实例流=%d bytes，想要 450×80=36000", got)
	}
	if got := avatarInstanceSize; got != 450*avatarInstanceBytes {
		t.Fatalf("avatarInstanceSize=%d，想要 450 实例的字节数", got)
	}
}

// TestHostileEntityKeyUsesU64IdentityInDistinctDomain 锁定夜行者实体键：
// 独立于玩家/伙伴/目标名牌的身份域，16-byte key 槽位以 little-endian 写入
// u64 ID；不同 ID 产出不同键，与相同 bytes 的其他域不冲突。
func TestHostileEntityKeyUsesU64IdentityInDistinctDomain(t *testing.T) {
	if EntityHostile != 4 {
		t.Fatalf("EntityHostile=%d，想要 4（1 玩家、2 伙伴、3 目标名牌已被占用）", EntityHostile)
	}
	first := HostileEntityKey(0x0102030405060708)
	second := HostileEntityKey(0x0102030405060709)
	if first == second {
		t.Fatal("不同 u64 ID 产出了相同实体键")
	}
	// 低 8 位 little-endian 写入 ID，高 8 位保持零。
	if first.ID[0] != 0x08 || first.ID[7] != 0x01 {
		t.Fatalf("16-byte key 低段=%v，想要 little-endian u64", first.ID[:8])
	}
	for _, value := range first.ID[8:] {
		if value != 0 {
			t.Fatalf("16-byte key 高段=%v，想要零", first.ID[8:])
		}
	}
	same := EntityKey{Kind: EntityPlayer, ID: first.ID}
	if compareEntityKeys(first, same) == 0 {
		t.Fatal("夜行者键与相同 bytes 的玩家键冲突")
	}
}

// TestHostileAvatarUsesOriginalPaletteAndProportions 锁定夜行者的原创呈现：
// 固定暗青基底 + 灰紫头部，6 cuboids 结构不变但头身比例与玩家不同，调色
// 与玩家/伙伴共用调色板的任何槽位都不重合。
func TestHostileAvatarUsesOriginalPaletteAndProportions(t *testing.T) {
	key := HostileEntityKey(7)
	parts := buildAvatarParts(nil, []Avatar{{Key: key, Position: mgl32.Vec3{4, 5, 6}}})
	if got, want := len(parts), avatarPartsPerBody; got != want {
		t.Fatalf("parts=%d want=%d", got, want)
	}
	base := avatarColor(key)

	// 暗青基底在 [0.2,0.9] 且蓝绿分量高于红分量。
	for channel, value := range base[:3] {
		if value < 0.2 || value > 0.9 {
			t.Fatalf("基底通道 %d=%f 越界", channel, value)
		}
	}
	if base[0] >= base[1] || base[0] >= base[2] {
		t.Fatalf("基底 %v 不是以蓝绿为主的暗青", base)
	}
	// 头部是灰紫：红蓝分量接近且都高于绿。
	head := parts[0].color
	if !(head[0] > head[1] && head[2] > head[1] && math.Abs(float64(head[0]-head[2])) < 0.15) {
		t.Fatalf("头部颜色 %v 不是灰紫", head)
	}
	// 躯干保留基底，四肢比基底暗。
	for channel := 0; channel < 3; channel++ {
		if parts[1].color[channel] != base[channel] {
			t.Fatalf("躯干通道 %d=%f 基底=%f", channel, parts[1].color[channel], base[channel])
		}
		if parts[2].color[channel] >= base[channel] {
			t.Fatalf("肢体通道 %d=%f 未比基底暗", channel, parts[2].color[channel])
		}
	}
	// 夜行者颜色域与玩家/伙伴调色板全部槽位不重合。
	for _, paletteColor := range avatarPalette {
		if base == paletteColor {
			t.Fatalf("夜行者基底 %v 与共用调色板槽位重合", base)
		}
	}

	// 头身比例与玩家不同：头部更大、身躯更短、手臂更长。
	hostileSizes := make([]mgl32.Vec3, 0, avatarPartsPerBody)
	for _, part := range parts {
		bounds := transformedUnitCubeBounds(part.transform)
		hostileSizes = append(hostileSizes, bounds.max.Sub(bounds.min))
	}
	if hostileSizes[0][0] <= 0.6 {
		t.Fatalf("夜行者头部宽度 %v 未大于玩家的 0.6", hostileSizes[0][0])
	}
	if hostileSizes[1][1] >= 0.7 {
		t.Fatalf("夜行者躯干高度 %v 未小于玩家的 0.7", hostileSizes[1][1])
	}
	if hostileSizes[2][1] <= 0.7 {
		t.Fatalf("夜行者手臂长度 %v 未大于玩家的 0.7", hostileSizes[2][1])
	}
	for index, part := range parts {
		assertAxisAligned(t, part.transform)
		_ = index
	}
	// 仍然锚定在脚底：全体部件的世界包围盒下缘落在站位 Y 附近。
	bounds := avatarPartsBounds(parts)
	if bounds.min[1] < 5-1e-4 || bounds.min[1] > 5+1e-4 {
		t.Fatalf("夜行者包围盒下缘=%v，想要站位 Y=5", bounds.min[1])
	}
	if !avatarColorRequiresKnownKindGuarded(key) {
		t.Fatal("夜行者键未通过 avatarColor 的身份域守卫")
	}
}

// avatarColorRequiresKnownKindGuarded 以不 panic 的方式确认 hostile 键能取色。
func avatarColorRequiresKnownKindGuarded(key EntityKey) (ok bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			ok = false
		}
	}()
	_ = avatarColor(key)
	return true
}

// TestHostileAvatarColorIsStableAcrossIDs 锁定夜行者取色与 ID 无关：固定
// 原创调色不随 ID 变化（区别于玩家/伙伴的 per-ID FNV 索引）。
func TestHostileAvatarColorIsStableAcrossIDs(t *testing.T) {
	first := avatarColor(HostileEntityKey(1))
	second := avatarColor(HostileEntityKey(0xffffffffffffffff))
	if first != second {
		t.Fatalf("夜行者取色随 ID 变化：%v vs %v", first, second)
	}
	// 与同 bytes 的玩家色域隔离。
	player := AvatarColor(core.PlayerID(HostileEntityKey(1).ID))
	if player == first {
		t.Fatal("夜行者颜色与相同 bytes 的玩家颜色重合")
	}
}
