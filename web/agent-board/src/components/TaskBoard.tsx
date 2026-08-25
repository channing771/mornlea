import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { StatusBadge } from '@/components/StatusBadge';
import type { BacklogTask, WorktreeStatus } from '@/api';

// 分组展示顺序；不在枚举内的状态归入「其他」。
const groupOrder = ['开发中', '已认领', '待集成', '未认领', '已完成', '其他'];

function findWorktree(worktrees: WorktreeStatus[], branch: string): WorktreeStatus | undefined {
  if (!branch) return undefined;
  const b = branch.toLowerCase();
  return worktrees.find((w) => w.branch && w.branch.toLowerCase().includes(b));
}

function TaskCard({ task, worktrees }: { task: BacklogTask; worktrees: WorktreeStatus[] }) {
  const wt = findWorktree(worktrees, task.branch);
  const commit =
    wt && wt.lastCommit
      ? '最近提交：' + (wt.lastCommit.time || '') + ' · ' + (wt.lastCommit.author || '') + ' · ' + (wt.lastCommit.subject || '')
      : '';
  const dirty = wt && wt.dirtyCount > 0 ? ' · 脏文件 ' + wt.dirtyCount : '';
  const ahead = wt && wt.hasAhead ? ' · 领先 main ' + wt.aheadCount : '';
  const progress =
    wt && wt.changes && wt.changes.length > 0
      ? wt.changes.map((ch) => ch.name + ' ' + ch.done + '/' + ch.total).join(' · ')
      : '';
  const ledgers =
    wt && wt.changes
      ? wt.changes.filter((ch) => ch.latestLedger).map((ch) => ({ name: ch.name, line: ch.latestLedger }))
      : [];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          <span className="font-mono">{task.id}</span>
          <span>{task.feature}</span>
          <StatusBadge status={task.status} />
        </CardTitle>
      </CardHeader>
      <CardContent>
        {task.summary && <p className="text-sm text-foreground">{task.summary}</p>}
        <p className="mt-1 text-xs text-muted-foreground">
          认领人：{task.claimant || '—'} · 分支：{task.branch || '—'}
          {wt ? ' · ' + wt.path : ''}
        </p>
        {commit && <p className="mt-1 text-xs text-muted-foreground">{commit}</p>}
        {(dirty || ahead) && <p className="mt-1 text-xs text-muted-foreground">{dirty}{ahead}</p>}
        {progress && <p className="mt-1 text-xs text-muted-foreground">{progress}</p>}
        {ledgers.length > 0 && (
          <div className="mt-2 space-y-1">
            {ledgers.map((l, i) => (
              <div key={i} className="rounded border-l-2 border-border bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground">
                <span className="font-medium">{l.name}</span> 最新 ledger：{l.line}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// TaskBoard 按状态分组展示功能规划任务。
function TaskBoard({ tasks, worktrees }: { tasks: BacklogTask[]; worktrees: WorktreeStatus[] }) {
  if (!tasks || tasks.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
        任务表为空
      </div>
    );
  }
  const groups = groupOrder.map((k) => ({ key: k, items: tasks.filter((t) => t.status === k) }));
  // 不在枚举内的状态归入「其他」。
  const known = new Set(groupOrder);
  const other = tasks.filter((t) => !known.has(t.status));
  return (
    <div className="space-y-4">
      {groups.map((g) => {
        const items = g.key === '其他' ? [...g.items, ...other] : g.items;
        if (items.length === 0) return null;
        return (
          <div key={g.key}>
            <h3 className="mb-2 text-sm font-semibold text-muted-foreground">
              {g.key}（{items.length}）
            </h3>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {items.map((t, i) => (
                <TaskCard key={t.id + '-' + i} task={t} worktrees={worktrees} />
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
}

export { TaskBoard };
