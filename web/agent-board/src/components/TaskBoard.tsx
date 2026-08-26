import { StatusBadge } from '@/components/StatusBadge';
import { EmptyHint } from '@/components/EmptyHint';
import type { BacklogTask, WorktreeStatus } from '@/api';

// 分组展示顺序；不在枚举内的状态归入「其他」。
const groupOrder = ['开发中', '已认领', '待集成', '就绪', '排队', '设计候选', '已完成', '已取消', '其他'];

function findWorktree(worktrees: WorktreeStatus[], branch: string): WorktreeStatus | undefined {
  if (!branch) return undefined;
  const b = branch.toLowerCase();
  return worktrees.find((w) => w.branch && w.branch.toLowerCase().includes(b));
}

// TaskRow 单行（最多两行式 + ledger 摘录）：行 1 = mono 任务 id + feature + 状态徽章（行尾小号胶囊）；
// 行 2 = meta chips（认领人/分支/worktree/最近提交/脏文件/领先/change 进度），全部 mono 小号、truncate；
// 分类标签之间用 gap 空格分隔的 chips，不用 middle-dot 串。summary 与 ledger 摘录在 meta 之下。
function TaskRow({ task, worktrees }: { task: BacklogTask; worktrees: WorktreeStatus[] }) {
  const wt = findWorktree(worktrees, task.branch);
  const commitLine =
    wt && wt.lastCommit
      ? '最近提交：' + (wt.lastCommit.time || '') + '  ' + (wt.lastCommit.author || '') + '  ' + (wt.lastCommit.subject || '')
      : '';
  const ledgers =
    wt && wt.changes
      ? wt.changes.filter((ch) => ch.latestLedger).map((ch) => ({ name: ch.name, line: ch.latestLedger }))
      : [];

  return (
    <div className="px-4 py-2.5">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-sm">{task.id}</span>
        <span className="text-sm font-semibold">{task.feature}</span>
        <span className="ml-auto shrink-0">
          <StatusBadge status={task.status} />
        </span>
      </div>
      {task.summary && <p className="mt-1 text-xs text-muted-foreground">{task.summary}</p>}
      <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <span className="whitespace-nowrap">认领人：<span className="font-mono">{task.claimant || '-'}</span></span>
        <span className="whitespace-nowrap font-mono">分支：{task.branch || '-'}</span>
        {wt && (
          <span className="max-w-xs truncate font-mono" title={wt.path}>
            {wt.path}
          </span>
        )}
        {commitLine && (
          <span className="max-w-[36ch] truncate font-mono" title={commitLine}>
            {commitLine}
          </span>
        )}
        {wt && wt.dirtyCount > 0 && <span className="whitespace-nowrap font-mono">脏文件 {wt.dirtyCount}</span>}
        {wt && wt.hasAhead && <span className="whitespace-nowrap font-mono">领先 main {wt.aheadCount}</span>}
        {wt &&
          wt.changes &&
          wt.changes.length > 0 &&
          wt.changes.map((ch) => (
            <span key={ch.name} className="whitespace-nowrap font-mono">
              {ch.name} {ch.done}/{ch.total}
            </span>
          ))}
      </div>
      {ledgers.length > 0 && (
        <div className="mt-1.5 space-y-1">
          {ledgers.map((l, i) => (
            <div key={i} className="rounded-md border-l-2 border-border bg-muted/30 px-3 py-1.5 text-xs text-muted-foreground">
              <span className="font-medium">{l.name}</span> 最新 ledger：
              <span className="font-mono">{l.line}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// TaskBoard 按状态分组展示功能规划任务（cockpit 行列表）。
function TaskBoard({ tasks, worktrees }: { tasks: BacklogTask[]; worktrees: WorktreeStatus[] }) {
  if (!tasks || tasks.length === 0) {
    return <EmptyHint>任务表为空</EmptyHint>;
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
          /* 每个状态组一个白色面板：组头在面板内 + 组内 share divide-y 行分隔 */
          <div key={g.key} className="rounded-md border border-border bg-card">
            <div className="flex items-baseline gap-2 px-4 pt-3">
              <span className="text-sm font-semibold">{g.key}</span>
              <span className="font-mono text-xs text-muted-foreground">{items.length}</span>
            </div>
            <div className="mt-2 divide-y divide-border">
              {items.map((t, i) => (
                <TaskRow key={t.id + '-' + i} task={t} worktrees={worktrees} />
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
}

export { TaskBoard };
