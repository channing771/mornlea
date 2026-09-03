package hud

// hunger.go 是 HUD 图集的鸡腿图标 painter：空槽与满格两枚 cell 的程序化像素源。
// 饥饿行的呈现已迁 WebView HUD 组件，但图集的列布局是固定上传契约的一部分
// （`AtlasPixels` 把整张贴图交给渲染器），这两枚 cell 因此继续随图集构建并上传，
// 列下标不得移动。

// paintHotbarDrumstick 把一格鸡腿画进 HUD 图集的指定列，full 区分满格与空槽。
//
// 程序化生成、不进 `packages/client/assets`：HUD 图集不在材质包的覆盖范围内（材质包
// 动的是方块 layer），把 HUD 图标放这里既避开了与它的冲突，也沿用了爱心已经
// 跑了几个里程碑的先例。
//
// 空槽画成同一轮廓的深色剪影而不是留白，这样十格底一直勾勒出满值刻度，玩家
// 一眼能读出「还差几格」。
func paintHotbarDrumstick(dst []byte, column int, full bool) {
	for y := range hotbarTextureSize {
		for x := range hotbarTextureSize {
			if !hotbarDrumstickPixel(x, y) {
				continue
			}
			border := !hotbarDrumstickPixel(x-1, y) || !hotbarDrumstickPixel(x+1, y) ||
				!hotbarDrumstickPixel(x, y-1) || !hotbarDrumstickPixel(x, y+1)
			// 空槽：深色剪影，边缘稍亮一档勾出轮廓。
			color := [4]byte{30, 22, 16, 255}
			if border {
				color = [4]byte{74, 52, 38, 255}
			}
			switch {
			case !full:
			case border:
				color = [4]byte{92, 48, 22, 255}
			case hotbarDrumstickBonePixel(x, y):
				color = [4]byte{240, 228, 196, 255}
			default:
				color = [4]byte{176, 96, 46, 255}
				// 左上一小片高光，让肉的体积在 16px 下也读得出来；窗口落在
				// 骨柄入肉处的下方，避免被骨色覆盖。
				if x <= 8 && y >= 9 && y <= 11 {
					color = [4]byte{214, 138, 74, 255}
				}
			}
			offset := (y*hotbarTextureWidth + column*hotbarTextureSize + x) * 4
			copy(dst[offset:offset+4], color[:])
		}
	}
}

// hotbarDrumstickPixel 报告鸡腿轮廓（骨头或肉）是否覆盖该像素。
// 空槽与满格共用同一轮廓，两者只差配色，因此填充半格时左右两半严丝合缝。
func hotbarDrumstickPixel(x, y int) bool {
	return hotbarDrumstickBonePixel(x, y) || hotbarDrumstickMeatPixel(x, y)
}

// hotbarDrumstickBonePixel 是左上角斜向的骨柄：一条 ±1 像素宽的对角线加一个圆骨节。
func hotbarDrumstickBonePixel(x, y int) bool {
	if x < 1 || y < 1 || x > 8 || y > 8 {
		return false
	}
	if diff := x - y; diff >= -1 && diff <= 1 {
		return true
	}
	dx, dy := x-2, y-2
	return dx*dx+dy*dy <= 2
}

// hotbarDrumstickMeatPixel 是右下角的肉：以 (10,10) 为心、半径约 4.7 的整数
// 圆盘。相比旧剪影收紧一圈并向右下挪一格，让骨柄以更短的入肉距离托住肉球，
// 右下留出的边距与心形剪影的收尖方向一致。
func hotbarDrumstickMeatPixel(x, y int) bool {
	dx, dy := x-10, y-10
	return dx*dx+dy*dy <= 22
}
