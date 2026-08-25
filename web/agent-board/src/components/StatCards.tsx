import type { Status } from '@/api';

// StatCards 顶部统计指标带（ops-console 风格）：白色面板外壳（rounded-md + hairline 边框），
// 内部 8 列 divide-x 垂直 hairline；数字一律 mono 定宽 ink 色。「开发中任务」数字用 amber 深字弱强调，
// 其余中性。gh 不可用时「打开 PR」显示 '-'。
function StatCards({ status }: { status: Status }) {
  const agents = (status.agents || []).length;
  const aliveChains = (status.chains || []).filter((c) => c.alive).length;
  const confirm = (status.confirm || []).filter((c) => c.waiting).length;
  const tasks = status.tasks || [];
  const count = (s: string) => tasks.filter((t) => t.status === s).length;
  const prs = Array.isArray(status.prs) ? status.prs.length : '-';
  const items = [
    { label: '执行中 AI', value: agents, strong: false },
    { label: '活跃接力链', value: aliveChains, strong: false },
    { label: '待确认卡片', value: confirm, strong: false },
    { label: '开发中任务', value: count('开发中'), strong: true },
    { label: '已认领任务', value: count('已认领'), strong: false },
    { label: '待集成任务', value: count('待集成'), strong: false },
    { label: '未认领任务', value: count('未认领'), strong: false },
    { label: '打开 PR', value: prs, strong: false },
  ];
  return (
    <div className="rounded-md border border-border bg-card">
      <div className="grid grid-cols-2 divide-x divide-border sm:grid-cols-4 xl:grid-cols-8">
        {items.map((it) => (
          <div key={it.label} className="px-4 py-3">
            <div className={'font-mono text-2xl font-semibold tabular-nums ' + (it.strong ? 'text-status-develop' : '')}>
              {it.value}
            </div>
            <div className="mt-0.5 text-xs text-muted-foreground">{it.label}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

export { StatCards };
