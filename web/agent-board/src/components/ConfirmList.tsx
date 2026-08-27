import { Badge } from '@/components/ui/badge';
import { EmptyHint } from '@/components/EmptyHint';
import { cn } from '@/lib/utils';
import { fmtDur } from '@/lib/fmt';
import type { ConfirmCard } from '@/api';

// ConfirmList 展示待确认/已回复卡片。等待中的卡片是「最需要注意力」的面：左侧 3px amber rail（+微亮的边框底色），
// 非等待（已回复）用 hairline 行样式。分类标签之间用 gap 分隔，不串 middle-dot。
function ConfirmList({ confirm }: { confirm: ConfirmCard[] }) {
  if (!confirm || confirm.length === 0) {
    return <EmptyHint>无待确认卡片</EmptyHint>;
  }
  return (
    /* 整个待确认区装在一个白色面板内：等待中 = 白卡 + 3px amber 左轨（浅底微暖底），已回复 = 面板内 hairline 行 */
    <div className="rounded-md border border-border bg-card">
      {confirm.map((c) => {
        const kind = c.kind === 'question' ? (
          <Badge variant="warning">question</Badge>
        ) : (
          <Badge variant="default">approval</Badge>
        );
        const wait = c.waiting ? (
          <Badge variant="warning" className="ml-2">等待中 <span className="font-mono">{fmtDur(c.waitSec)}</span></Badge>
        ) : c.replyAction ? (
          <Badge variant="success" className="ml-2">已回复 <span className="font-mono">{c.replyAction}</span></Badge>
        ) : null;
        return (
          <div
            key={c.id}
            className={cn(
              c.waiting
                ? 'm-3 rounded-md border border-border border-l-[3px] border-l-status-develop bg-status-develop/10 px-3 py-3'
                : 'border-b border-border px-3 py-3 last:border-b-0',
            )}
          >
            <div className="flex flex-wrap items-center gap-2">
              {kind}
              <span className="font-mono text-sm">{c.id}</span>
              {c.category && <span className="text-xs text-muted-foreground">{c.category}</span>}
              {wait}
            </div>
            {c.title && <p className="mt-1 text-sm">{c.title}</p>}
            {c.question && <p className="mt-1 text-sm text-muted-foreground">{c.question}</p>}
            <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              {c.design && <span>设计：{c.design}</span>}
              {c.replyAction && <span className="whitespace-nowrap">回复动作：<span className="font-mono">{c.replyAction}</span></span>}
              {c.replyText && <span className="max-w-[40ch] truncate" title={c.replyText}>{c.replyText}</span>}
              {c.supersededBy && <span className="whitespace-nowrap">已被 {c.supersededBy} 取代</span>}
            </div>
          </div>
        );
      })}
    </div>
  );
}

export { ConfirmList };
