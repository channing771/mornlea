import type { ReactNode } from 'react';

// EmptyHint 统一空态/降级提示：hairline 虚线边框 + 居中 muted 文本。
// 六个分区共用同一形状，避免每处手抄类名漂移。
function EmptyHint({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-md border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
      {children}
    </div>
  );
}

export { EmptyHint };
