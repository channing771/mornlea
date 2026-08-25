import { Card, CardContent } from '@/components/ui/card';
import type { Status } from '@/api';

// StatCards 顶部统计网格：执行中 AI/活跃接力链/待确认/开发中/已认领/待集成/未认领/打开 PR。
function StatCards({ status }: { status: Status }) {
  const agents = (status.agents || []).length;
  const aliveChains = (status.chains || []).filter((c) => c.alive).length;
  const confirm = (status.confirm || []).filter((c) => c.waiting).length;
  const tasks = status.tasks || [];
  const count = (s: string) => tasks.filter((t) => t.status === s).length;
  const prs = Array.isArray(status.prs) ? status.prs.length : '—';
  const items = [
    { label: '执行中 AI', value: agents },
    { label: '活跃接力链', value: aliveChains },
    { label: '待确认卡片', value: confirm },
    { label: '开发中任务', value: count('开发中') },
    { label: '已认领任务', value: count('已认领') },
    { label: '待集成任务', value: count('待集成') },
    { label: '未认领任务', value: count('未认领') },
    { label: '打开 PR', value: prs },
  ];
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-8">
      {items.map((it) => (
        <Card key={it.label}>
          <CardContent className="p-3">
            <div className="text-2xl font-bold">{it.value}</div>
            <div className="text-xs text-muted-foreground">{it.label}</div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

export { StatCards };
