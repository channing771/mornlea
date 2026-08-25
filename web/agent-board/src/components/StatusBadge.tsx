import { Badge } from '@/components/ui/badge';
import { statusClass } from '@/lib/fmt';
import { cn } from '@/lib/utils';

// StatusBadge 渲染任务状态徽章；「开发中」带琥珀色脉冲点，沿用原看板语义。
function StatusBadge({ status }: { status: string }) {
  const cls = statusClass(status);
  return (
    <Badge className={cn('gap-1.5', cls)}>
      {status === '开发中' && <span aria-hidden className="h-1.5 w-1.5 animate-pulse rounded-full bg-status-develop motion-reduce:animate-none" />}
      {status || '其他'}
    </Badge>
  );
}

export { StatusBadge };
