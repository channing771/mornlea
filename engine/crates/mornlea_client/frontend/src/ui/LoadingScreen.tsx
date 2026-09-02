// 世界加载屏：loading 相位的全屏不透明遮罩 + 标题 + 进度条 + 区块计数行。
// 进度语义权威在 Go（loaded/total 与可选 meshed/meshTotal 分节下行）：进度条
// 按工作量单位合并两阶段——区块快照流送与初始网格化（后者是快照到齐后的
// 主要剩余工作量），网格字段缺席时退回纯区块比例。这里只做 clamp 比例换算
// 与文本格式化，不预测、不平滑进度——填充宽度随下行硬切，零 transition，
// `prefers-reduced-motion` 天然合规。分节缺席属防御性降级（Go 在 loading
// 相位恒下行分节）：空轨 0% 且不呈现计数行，遮罩与标题仍在，其下半成品
// 世界画面在任何输入形态下都不可见。
import type { CSSProperties } from "react";
import type { LoadingState } from "../bridge/client";
import { LOADING_COUNT_UNIT, LOADING_TITLE } from "./copy";

export interface LoadingScreenProps {
  /** 桥下行的 loading 分节；缺席时安全降级为 0% 空轨。 */
  readonly loading?: LoadingState;
}

/** 比例换算：网格字段齐备时 clamp((loaded+meshed)/(total+meshTotal), 0, 1)
 * （工作量单位合并，100% 恰好对齐完成判据收敛），否则退回
 * clamp(loaded/total, 0, 1)。total<=0 或分节缺席降级为 0（桥守卫已保证
 * total>=1、meshTotal>=1，防御分支只防组件直用时除零）。 */
function loadingRatio(loading: LoadingState | undefined): number {
  if (loading === undefined || loading.total <= 0) {
    return 0;
  }
  const meshed = loading.meshed ?? 0;
  const meshTotal =
    loading.meshed !== undefined && loading.meshTotal !== undefined && loading.meshTotal > 0
      ? loading.meshTotal
      : undefined;
  if (meshTotal === undefined) {
    return Math.min(Math.max(loading.loaded / loading.total, 0), 1);
  }
  return Math.min(Math.max((loading.loaded + meshed) / (loading.total + meshTotal), 0), 1);
}

export function LoadingScreen({ loading }: LoadingScreenProps = {}) {
  const ratio = loadingRatio(loading);
  return (
    <section className="loading-screen">
      <h1 className="loading-title">{LOADING_TITLE}</h1>
      <div className="loading-progress" style={{ "--loading-fill": ratio } as CSSProperties}>
        <span className="loading-progress-fill" />
      </div>
      {loading !== undefined && (
        <p className="loading-count">
          {LOADING_COUNT_UNIT} {loading.loaded} / {loading.total}
        </p>
      )}
    </section>
  );
}
