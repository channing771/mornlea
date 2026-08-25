import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { fmtDur } from '@/lib/fmt';
import type { ConfirmCard } from '@/api';

// ConfirmList 展示待确认/已回复卡片；等待中的卡片加琥珀色边框。
function ConfirmList({ confirm }: { confirm: ConfirmCard[] }) {
  if (!confirm || confirm.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
        无待确认卡片
      </div>
    );
  }
  return (
    <div className="space-y-3">
      {confirm.map((c) => {
        const kind = c.kind === 'question' ? (
          <Badge variant="warning" className="rounded-full">question</Badge>
        ) : (
          <Badge variant="default" className="rounded-full">approval</Badge>
        );
        const wait = c.waiting ? (
          <Badge variant="warning" className="ml-2 rounded-full">等待中 {fmtDur(c.waitSec)}</Badge>
        ) : c.replyAction ? (
          <Badge variant="success" className="ml-2 rounded-full">已回复 {c.replyAction}</Badge>
        ) : null;
        return (
          <Card key={c.id} className={cn(c.waiting && 'border-status-develop')}>
            <CardHeader>
              <CardTitle className="flex flex-wrap items-center gap-2">
                {kind}
                <span className="font-mono">{c.id}</span>
                {c.category && <span className="font-normal text-muted-foreground">{c.category}</span>}
                {wait}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {c.title && <p className="text-sm">{c.title}</p>}
              {c.question && <p className="mt-1 text-sm text-muted-foreground">{c.question}</p>}
              {c.design && <p className="mt-1 text-xs text-muted-foreground">设计：{c.design}</p>}
              {c.replyAction && (
                <p className="mt-1 text-xs text-muted-foreground">回复动作：{c.replyAction} · {c.replyText || ''}</p>
              )}
              {c.supersededBy && (
                <p className="mt-1 text-xs text-muted-foreground">已被 {c.supersededBy} 取代</p>
              )}
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}

export { ConfirmList };
