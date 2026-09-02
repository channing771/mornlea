// 进食进度轨道：同一轨道几何与锚点（快捷栏内容行居中、状态栈上沿之上一个
// 轨道间隙），暖金填充、不带任何形状标记。采掘进度条已退役（采掘进度反馈
// 由世界空间方块裂纹承载），轨道不再区分语义分支——保留组件与几何是为了
// 进食条继续占据 design 基准内的同一行。
//
// 填充钳制在轨道内：比例在 props 传入前已钳制，轨道再以 `overflow: hidden`
// 兜底，超额值不会把填充推出轨道边界。
import type { CSSProperties } from "react";

/** 轨道语义：进食是唯一的屏幕进度条（暖金填充，无标记）。 */
export type ProgressKind = "eating";

export interface ProgressTrackProps {
  readonly kind: ProgressKind;
  /** 已确认/预测的填充比例，呈现前钳制到 `0..1`。 */
  readonly progress: number;
}

export function ProgressTrack({ kind, progress }: ProgressTrackProps) {
  // 镜像 `appendEatingBar` 的分段：轨道随激活呈现，填充只在比例非零时追加
  // （零进度不产生填充）。
  const fraction = Math.min(Math.max(progress, 0), 1);
  const filled = fraction > 0;
  return (
    <div className="hud-progress" style={{ "--hud-fill": fraction } as CSSProperties}>
      {filled ? <span className={`hud-progress-fill hud-progress-fill--${kind}`} /> : null}
    </div>
  );
}
