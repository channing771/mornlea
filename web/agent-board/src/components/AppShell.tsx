import { useEffect, useState, type ReactNode } from 'react';
import { RefreshCw } from 'lucide-react';
import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { StatCards } from '@/components/StatCards';
import { AgentsTable } from '@/components/AgentsTable';
import { ChainsTable } from '@/components/ChainsTable';
import { TaskBoard } from '@/components/TaskBoard';
import { ConfirmList } from '@/components/ConfirmList';
import { PrTable } from '@/components/PrTable';
import { LogsTabs } from '@/components/LogsTabs';
import type { Status } from '@/api';

interface AppShellProps {
  status: Status | null;
  loading: boolean;
  error: string | null;
  updatedAt: Date | null;
  onRefresh: () => void;
}

function pad(n: number): string {
  return n < 10 ? '0' + n : String(n);
}

function fmtClock(d: Date): string {
  return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
}

function fmtTime(d: Date): string {
  return pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
}

// AppShell 顶栏 + 错误提示 + 统计卡片 + 各分区，是看板整体布局。
function AppShell({ status, loading, error, updatedAt, onRefresh }: AppShellProps) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(id);
  }, []);
  const errors = (status && status.errors) || {};
  const errorKeys = Object.keys(errors).filter((k) => errors[k]);

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="sticky top-0 z-10 border-b border-border bg-background/90 backdrop-blur">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-3 px-4 py-3">
          <h1 className="text-lg font-semibold">Mornlea Agent 执行看板</h1>
          <span className="text-sm tabular-nums text-muted-foreground">{fmtClock(now)}</span>
          <span className="ml-auto text-xs text-muted-foreground">
            {updatedAt ? '上次更新 ' + fmtTime(updatedAt) : '尚未更新'}
          </span>
          <Button size="sm" variant="outline" onClick={onRefresh}>
            <RefreshCw className="h-4 w-4" /> 刷新
          </Button>
        </div>
      </header>
      <main className="mx-auto max-w-7xl space-y-6 px-4 py-6">
        {error && (
          <Alert variant="destructive">
            后端不可用：{error}（5 秒后自动重试…）
          </Alert>
        )}
        {errorKeys.length > 0 && (
          <Alert variant="destructive">
            采集降级：{errorKeys.map((k) => k + ': ' + errors[k]).join(' ｜ ')}
          </Alert>
        )}
        {loading && !status ? (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-8">
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className="h-16" />
            ))}
          </div>
        ) : status ? (
          <>
            <StatCards status={status} />
            <Section title="执行中 AI" sub="ps 采样的 claude/codex/run-agent/relay/feishu-listener/pr-finalize 进程">
              <AgentsTable agents={status.agents} />
            </Section>
            <Section title="接力链" sub="~/.mornlea/loop.guard*；注：pid 可能为会话临时 shell 的已知缺陷">
              <ChainsTable chains={status.chains} />
            </Section>
            <Section title="任务看板" sub="docs/feature-backlog.md 按状态分组；worktree 按分支名关联">
              <TaskBoard tasks={status.tasks} worktrees={status.worktrees} />
            </Section>
            <Section title="待确认" sub="~/.mornlea/confirm 卡片（approval / question）">
              <ConfirmList confirm={status.confirm} />
            </Section>
            <Section title="PR 与 CI" sub="gh pr list --state open（尽力采集）">
              <PrTable prs={status.prs} errors={status.errors} />
            </Section>
            <Section title="最近动态" sub="各日志末尾片段">
              <LogsTabs logs={status.logs} />
            </Section>
          </>
        ) : null}
      </main>
    </div>
  );
}

function Section({ title, sub, children }: { title: string; sub?: string; children: ReactNode }) {
  return (
    <section className="space-y-2">
      <h2 className="flex flex-wrap items-baseline gap-2 text-sm font-semibold">
        {title}
        {sub && <span className="text-xs font-normal text-muted-foreground">{sub}</span>}
      </h2>
      {children}
    </section>
  );
}

export { AppShell };
