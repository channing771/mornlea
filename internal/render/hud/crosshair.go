package hud

// crosshair.go 实现原创十字准星：viewport 几何中心锚定、投影加前景双层的
// 两臂矩形，全部由既有 instanced-quad 管线绘制，不新增 shader、GPU pass 或
// 上传格式。

const (
	// crosshairQuads 是准星的固定 quad 数：投影与前景各横竖两臂。
	crosshairQuads = 4
	// 十字两臂的 design px 尺寸：横臂 11×3、竖臂 3×11，随 `hudScale` 等比缩放。
	crosshairArmLength    = float32(11)
	crosshairArmThickness = float32(3)
	// crosshairShadowOffset 是投影层相对前景层的偏移（design px），与 HUD 文字
	// 阴影同向（右下），保证双层在任意背景上错位可辨。
	crosshairShadowOffset = float32(1)
)

// CrosshairOverlay 是准星的呈现开关。可见性由应用层按相位计算：主菜单、
// 设置页与暂停覆盖层等菜单相位必须不产生准星实例（呈现层只按 Visible 绘制，
// 不感知相位语义）。
type CrosshairOverlay struct {
	Visible bool
}

// appendCrosshair 在 framebuffer 几何中心追加十字准星。
//
// 调用顺序契约：准星实例必须先于快捷栏/容器面板追加——渲染按实例顺序覆盖，
// 面板后画才能遮住与准星的重叠区域（容器打开时准星被面板盖住是预期层叠）。
// 准星只消费 framebuffer 尺寸与 `dst.scale`，与物品镜像无关，因此在物品镜像
// 未确认、快捷栏不布局的帧里也照常绘制（HUD 可见即呈现）。
func appendCrosshair(dst *hotbarLayout, overlay CrosshairOverlay, width, height float32) {
	if !overlay.Visible || width <= 0 || height <= 0 {
		return
	}
	scale := dst.scale
	centerX := width * 0.5
	centerY := height * 0.5
	length := crosshairArmLength * scale
	thickness := crosshairArmThickness * scale
	offset := crosshairShadowOffset * scale
	horizontal := func(color [4]float32, shift float32) hotbarInstance {
		return hotbarInstance{
			X: centerX - length*0.5 + shift, Y: centerY - thickness*0.5 + shift,
			Width: length, Height: thickness, Color: color,
		}
	}
	vertical := func(color [4]float32, shift float32) hotbarInstance {
		return hotbarInstance{
			X: centerX - thickness*0.5 + shift, Y: centerY - length*0.5 + shift,
			Width: thickness, Height: length, Color: color,
		}
	}
	dst.quads = append(dst.quads,
		horizontal(crosshairShadow, offset),
		vertical(crosshairShadow, offset),
		horizontal(crosshairFg, 0),
		vertical(crosshairFg, 0),
	)
}
