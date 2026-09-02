// 物品名弹条：选中栏位确认变化时于快捷栏上方居中呈现的物品中文名。
// 文本由 Go 截断并按 40 tick 窗口下行（presence 即可见性），容器打开态由
// `HudRoot` 抑制——这里只做「阴影 + 前景」双层文字呈现。
export function ItemPopup({ text }: { text: string }) {
  return <div className="hud-popup">{text}</div>;
}
