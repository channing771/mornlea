// 视口中心的两类叠加元素：原创十字准星与权威命中 marker。
// 两者都锚定视口几何中心（容器为 0×0 的定位点，四向臂以负偏移自中心展开），
// 尺寸全部是 design px × `--hud-scale`，随 HUD 整体等比缩放。
// 准星是深色投影 + 亮色前景双层（`box-shadow` 复制一层偏移矩形，与迁移前
// `appendCrosshair` 的投影/前景两臂等价）；marker 是四向白色不透明短标记，
// 几何比例与迁移前 `appendCombatMarker` 逐项一致。

export function Crosshair() {
  return (
    <div className="hud-crosshair" aria-hidden="true">
      <span className="hud-crosshair-arm hud-crosshair-arm--horizontal" />
      <span className="hud-crosshair-arm hud-crosshair-arm--vertical" />
    </div>
  );
}

export function HitMarker() {
  return (
    <div className="hud-marker" aria-hidden="true">
      <span className="hud-marker-arm hud-marker-arm--up" />
      <span className="hud-marker-arm hud-marker-arm--down" />
      <span className="hud-marker-arm hud-marker-arm--left" />
      <span className="hud-marker-arm hud-marker-arm--right" />
    </div>
  );
}
