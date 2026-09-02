// 世界加载屏：loading 相位的全屏不透明遮罩 + 标题 + 进度条 + 区块计数行。
// 进度语义权威在 Go（loaded/total 分节下行），这里只做 clamp 比例换算与
// 文本格式化，不预测、不平滑进度——填充宽度随下行硬切，零 transition，
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

/** 比例换算：clamp(loaded/total, 0, 1)；total<=0 或分节缺席降级为 0
 * （桥守卫已保证 total>=1，0/0 只防组件直用时除零）。 */
function loadingRatio(loading: LoadingState | undefined): number {
  if (loading === undefined || loading.total <= 0) {
    return 0;
  }
  return Math.min(Math.max(loading.loaded / loading.total, 0), 1);
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
