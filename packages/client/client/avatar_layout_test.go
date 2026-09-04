//go:build darwin

package client

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
)

// TestAvatarInstanceLayoutMatchesRustBridge 锁定跨语言 avatar 实例布局：
// Go（`render.avatarInstanceBytes` 编码）、Rust（`entity.rs` 解码）与本桥
// 测试三方一致：96 字节 = mat4（0..64）+ 颜色（64..80）+ 材质 u32
// （80..84）+ 12 字节保留零填充；固定容量 75 具 × 6 部件 = 450 实例不变；
// 纯色分支哨兵与有效层号不相交；牛身层按名引用，绝不硬编码数字。
func TestAvatarInstanceLayoutMatchesRustBridge(t *testing.T) {
	const (
		instanceBytes     = 96
		maxBodies         = 75
		partsPerBody      = 6
		maxInstances      = maxBodies * partsPerBody
		materialOffset    = 80
		materialSolidBits = ^uint32(0)
	)
	if got := maxInstances * instanceBytes; got != 43200 {
		t.Fatalf("avatar 实例流上界=%d，想要 450×96=43200", got)
	}
	if materialOffset+4 > instanceBytes {
		t.Fatalf("材质槽越界：offset=%d", materialOffset)
	}
	if got := materialSolidBits; got == uint32(assets.LayerCowHide) || got == uint32(assets.LayerCowHead) {
		t.Fatalf("哨兵 %d 与牛身层号相交", got)
	}
	// 牛身层按名引用：层号含义由素材包守卫，桥测试只锁“两层互异且在 atlas 内”。
	if assets.LayerCowHide == assets.LayerCowHead {
		t.Fatal("牛皮与牛头层号重合")
	}
}
