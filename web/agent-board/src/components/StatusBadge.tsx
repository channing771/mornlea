import { Badge } from '@/components/ui/badge';
import { statusClass } from '@/lib/fmt';
import { cn } from '@/lib/utils';

// StatusBadge 渲染任务状态徽章，小号胶囊（11px）；「开发中」带琥珀色脉冲点，沿用原看板语义。
// 注：视觉收紧为行内小号胶囊；「开发中」徽章文本类必须保持 text-status-develop（既有测试断言）。
function StatusBadge({ status }: { status: string }) {
  const cls = statusClass(status);
  return (
    <Badge className={cn('gap-1.5 px-2 py-0.5 text-[11px]', cls)}>
      {status === '开发中' && (
        <span aria-hidden className="h-1.5 w-1.5 animate-pulse rounded-full bg-status-develop motion-reduce:animate-none" />
      )}
      {status || '其他'}
    </Badge>
  );
}

export { StatusBadge };
