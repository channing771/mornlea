// 采掘/进食共用的进度轨道：同一轨道几何与锚点（快捷栏内容行居中、状态栈
// 上沿之上一个轨道间隙），只以填充色与标记形状区分语义。
//
// 颜色无关可辨性（迁移前 `appendMiningBar` 的形状契约）：
//   - 可采 → sage 绿填充 + 随填充末端移动的亮色末端标记；
//   - 不可采 → 橙色填充 + 固定数量与固定位置的警示缺口；
//   - 进食 → 无任何标记（不与采掘两种形状混淆）。
// 填充与标记都钳制在轨道内：比例在 props 传入前已钳制，轨道再以
// `overflow: hidden` 兜底，超额值不会把任何标记推出轨道边界。
import type { CSSProperties } from "react";
import { PROGRESS_NOTCH_POSITIONS } from "./geometry";

/** 轨道语义：采掘按 `harvestable` 二分，进食是独立的暖金填充。 */
export type ProgressKind = "mining-harvestable" | "mining-blocked" | "eating";

export interface ProgressTrackProps {
  readonly kind: ProgressKind;
  /** 已确认/预测的填充比例，呈现前钳制到 `0..1`。 */
  readonly progress: number;
}

export function ProgressTrack({ kind, progress }: ProgressTrackProps) {
  // 镜像 `appendMiningBar`/`appendEatingBar` 的分段：轨道随激活呈现，填充与
  // 形状标记只在比例非零时追加（零进度不产生末端标记或警示缺口）。
  const fraction = Math.min(Math.max(progress, 0), 1);
  const filled = fraction > 0;
  return (
    <div className="hud-progress" style={{ "--hud-fill": fraction } as CSSProperties}>
      {filled ? <span className={`hud-progress-fill hud-progress-fill--${kind}`} /> : null}
      {filled && kind === "mining-harvestable" ? <span className="hud-progress-cap" /> : null}
      {filled && kind === "mining-blocked"
        ? PROGRESS_NOTCH_POSITIONS.map((position) => (
            <span
              key={position}
              className="hud-progress-notch"
              style={{ left: `${position * 100}%` }}
            />
          ))
        : null}
    </div>
  );
}
