// HudRoot：游戏相位常显 HUD 的组件根。`hud` 缺席（菜单相位/装配前）渲染
// null；单一比例 `--hud-scale` 只由 `hud.viewport`（桥下行 framebuffer 尺寸）
// 计算——`window` 尺寸不参与缩放，resize 的呈现更新由 Go 在下一权威 tick
// 随状态下行携带（与 GPU 侧 `hudScale` 同一窗口输入，容器面板与状态行才
// 不会按两份比例绘制而破坏左右缘对齐）。
//
// 构图（自下而上）：快捷栏贴条 → `--hud-status-hotbar-gap` 净空 → 主状态行
// → `--hud-status-bar-gap` 行距 → 氧气行 → `--hud-progress-track-gap` →
// 进度轨道（进食条；采掘进度条已退役，采掘反馈由世界空间裂纹承载） →
// `--hud-popup-track-gap` → 物品名弹条，整体底部锚定、水平居中，
// 与迁移前 `closedHUDHeight` 的自下而上记账逐项同序。容器打开态把两条状态行
// 翻转到快捷栏下方并向外堆叠（氧气继续向下），快捷栏贴条转为占位（空间
// 保留，不与 GPU 容器面板下段的快捷栏行重复呈现），底部改由
// `--hud-hotbar-bottom-margin` 预留——两份预留与偏移逐项抵消，状态栈的绝对
// 位置不随翻转记账移动。
//
// 纪律：零网络、零本地存储、零 transition/animation（权威 tick 下行即硬切
// 呈现），全部数值经 `src/tokens.css` 的 `--hud-*` 令牌消费。
import type { CSSProperties } from "react";
import type { HudEating, HudState } from "../bridge/client";
import { Crosshair, HitMarker } from "./Crosshair";
import { ChatLog } from "./ChatLog";
import { hudScale } from "./geometry";
import { Hotbar } from "./Hotbar";
import { ItemPopup } from "./ItemPopup";
import { ProgressTrack, type ProgressKind } from "./ProgressTrack";
import { StatusRow } from "./StatusRow";
import "./hud.css";

export interface HudRootProps {
  /** 桥下行的游戏相位 HUD 分节；菜单相位/未装配时缺席。 */
  readonly hud: HudState | undefined;
 readonly onSelect?: (index:number)=>void;
}

export function HudRoot({ hud, onSelect }: HudRootProps) {
  if (hud === undefined) {
    return null;
  }
  const scale = hudScale(hud.viewport);
  if (!(scale > 0)) {
    // 零尺寸/非法视口：安全降级为不呈现，不产生布局异常。
    return null;
  }
  const open = hud.containerOpen === true;
  const progress = resolveProgress(hud.eating);
  // 栈内子元素的 JSX 顺序即视觉顺序（DOM 序 = 构图序，便于断言与无障碍）：
  // 关闭态弹条/轨道在最上、贴条收底；容器打开态只交换「贴条」与「状态行」
  // 两段，行栈翻到贴条下方，氧气行继续向下堆叠。
  const statusRows = (
    <StatusRow health={hud.health} hunger={hud.hunger} oxygen={hud.oxygen} open={open} />
  );
  const hotbar = <Hotbar slots={hud.hotbar?.slots} selectedIndex={hud.hotbar?.selectedIndex} onSelect={onSelect} />;
  return (
    <div
      className={open ? "hud-root hud-root--open" : "hud-root"}
      style={{ "--hud-scale": scale } as CSSProperties}
    >
      <div className="hud-stack">
        {hud.popup !== undefined && !open ? <ItemPopup text={hud.popup.text} /> : null}
        {progress === null ? null : (
          <ProgressTrack kind={progress.kind} progress={progress.progress} />
        )}
        {open ? (
          <>
            {hotbar}
            {statusRows}
          </>
        ) : (
          <>
            {statusRows}
            {hotbar}
          </>
        )}
      </div>
      {hud.crosshair === true && !open ? <Crosshair /> : null}
      {hud.marker === true ? <HitMarker /> : null}
      {hud.chat !== undefined && hud.chat.lines.length > 0 ? (
        <ChatLog lines={hud.chat.lines} />
      ) : null}
    </div>
  );
}

/** 进度呈现输入：进食是唯一的屏幕进度语义（采掘条退役后不再有互斥裁决，
 * 采掘进度反馈由世界空间方块裂纹承载）。 */
function resolveProgress(eating: HudEating): { kind: ProgressKind; progress: number } | null {
  if (eating.active) {
    return { kind: "eating", progress: eating.progress };
  }
  return null;
}
