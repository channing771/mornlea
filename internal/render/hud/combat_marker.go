package hud

// appendCombatMarker 在已有 HUD 之后追加 4 个白色不透明无纹理 quad，构成十字命中标记。
func appendCombatMarker(dst *hotbarLayout, combatMarker bool, width, height float32) {
	if !combatMarker || width <= 0 || height <= 0 {
		return
	}
	scale := dst.scale
	if scale == 0 {
		scale = hudScale(dst.open, width, height)
	}
	cx := width * 0.5
	cy := height * 0.5
	offset := (4 + 8.0/2) * scale
	color := [4]float32{1, 1, 1, 1}
	// 上：2×8
	{
		w := 2 * scale
		h := 8 * scale
		x := cx - w*0.5
		y := cy - offset - h*0.5
		dst.quads = append(dst.quads, hotbarInstance{X: x, Y: y, Width: w, Height: h, Color: color})
	}
	// 下：2×8
	{
		w := 2 * scale
		h := 8 * scale
		x := cx - w*0.5
		y := cy + offset - h*0.5
		dst.quads = append(dst.quads, hotbarInstance{X: x, Y: y, Width: w, Height: h, Color: color})
	}
	// 左：8×2
	{
		w := 8 * scale
		h := 2 * scale
		x := cx - offset - w*0.5
		y := cy - h*0.5
		dst.quads = append(dst.quads, hotbarInstance{X: x, Y: y, Width: w, Height: h, Color: color})
	}
	// 右：8×2
	{
		w := 8 * scale
		h := 2 * scale
		x := cx + offset - w*0.5
		y := cy - h*0.5
		dst.quads = append(dst.quads, hotbarInstance{X: x, Y: y, Width: w, Height: h, Color: color})
	}
}
