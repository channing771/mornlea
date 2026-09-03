// 后端 /api/status 契约的类型定义与轮询 hook。
// 字段逐一对应 packages/tools/agent-board/parse.go 的 Status 及各实体结构，保持稳定。

import { useCallback, useEffect, useRef, useState } from 'react';

export interface AgentStatus {
  tool: string;
  role: string;
  pid: string;
  ppid: string;
  uptime: string;
  cwd: string;
  cmd: string;
}

export interface ChainStatus {
  id: string;
  pid: string;
  alive: boolean;
  stale: boolean;
  guardFile: string;
  mtime: string;
  note: string;
}

export interface BacklogTask {
  id: string;
  feature: string;
  summary: string;
  status: string;
  statusRaw: string;
  claimant: string;
  branch: string;
  note: string;
}

export interface Commit {
  time: string;
  author: string;
  subject: string;
}

export interface ChangeStatus {
  name: string;
  done: number;
  total: number;
  latestLedger: string;
}

export interface WorktreeStatus {
  path: string;
  branch: string;
  head: string;
  isMain: boolean;
  lastCommit?: Commit;
  dirtyCount: number;
  aheadCount: number;
  hasAhead: boolean;
  changes: ChangeStatus[];
  error?: string;
}

export interface ConfirmCard {
  id: string;
  kind: string;
  title: string;
  category: string;
  question: string;
  design: string;
  waiting: boolean;
  waitSec: number;
  replyAction?: string;
  replyText?: string;
  status?: string;
  supersededBy?: string;
  createdAt: string;
}

export interface PRCheck {
  name: string;
  state: string;
}

export interface PRStatus {
  number: number;
  title: string;
  branch: string;
  mergeState: string;
  checks: PRCheck[];
  url: string;
}

export interface Status {
  generatedAt: string;
  root: string;
  agents: AgentStatus[];
  chains: ChainStatus[];
  tasks: BacklogTask[];
  worktrees: WorktreeStatus[];
  confirm: ConfirmCard[];
  prs: PRStatus[] | null;
  logs: Record<string, string[]>;
  errors: Record<string, string>;
}

// fetchStatus 拉取 /api/status；可传入 AbortSignal 以便取消。
export async function fetchStatus(signal?: AbortSignal): Promise<Status> {
  const res = await fetch('/api/status', { signal });
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`);
  }
  return (await res.json()) as Status;
}

// UseStatusState 是 useStatus 对外暴露的状态与控制。
export interface UseStatusState {
  status: Status | null;
  loading: boolean;
  error: string | null;
  updatedAt: Date | null;
  refresh: () => void;
}

// useStatus 以固定间隔（默认 5 秒）轮询 /api/status。
// 约定：上一请求未完成时不发起下一次（防叠发）；请求失败时保留旧数据并记录 error；
// 抛出的 AbortError 不视为失败。
export function useStatus(intervalMs = 5000): UseStatusState {
  const [status, setStatus] = useState<Status | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);
  const inFlightRef = useRef(false);
  const serialRef = useRef(0);
  const [refreshKey, setRefreshKey] = useState(0);

  const refresh = useCallback(() => setRefreshKey((k) => k + 1), []);

  useEffect(() => {
    let disposed = false;
    const controller = new AbortController();

    const run = async () => {
      if (inFlightRef.current) {
        return; // 防叠发：上一请求未完，跳过本轮。
      }
      inFlightRef.current = true;
      const serial = ++serialRef.current;
      setLoading(true);
      try {
        const st = await fetchStatus(controller.signal);
        if (!disposed && serial === serialRef.current) {
          setStatus(st);
          setError(null);
          setUpdatedAt(new Date());
        }
      } catch (e) {
        if (!disposed && serial === serialRef.current && !controller.signal.aborted) {
          // 保留旧数据，仅记录错误。
          setError(e instanceof Error ? e.message : String(e));
        }
      } finally {
        if (serial === serialRef.current) {
          inFlightRef.current = false;
          if (!disposed) setLoading(false);
        }
      }
    };

    void run();
    const id = setInterval(() => void run(), intervalMs);
    return () => {
      disposed = true;
      clearInterval(id);
      controller.abort();
    };
  }, [intervalMs, refreshKey]);

  return { status, loading, error, updatedAt, refresh };
}
