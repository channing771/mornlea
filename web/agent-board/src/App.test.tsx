import { act, cleanup, render, renderHook, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Status } from '@/api';
import { useStatus } from '@/api';
import { AgentsTable } from '@/components/AgentsTable';
import { AppShell } from '@/components/AppShell';
import { LogsTabs } from '@/components/LogsTabs';
import { PrTable } from '@/components/PrTable';
import { StatusBadge } from '@/components/StatusBadge';

const emptyStatus: Status = {
  generatedAt: '2026-08-25T00:00:00Z',
  root: '/repo',
  agents: [],
  chains: [],
  tasks: [],
  worktrees: [],
  confirm: [],
  prs: [],
  logs: {},
  errors: {},
};

// okResponse 构造一个可被 fetchStatus 消费的 Response 假对象。
function okResponse(over: Partial<Status> = {}): Response {
  return { ok: true, status: 200, json: async () => ({ ...emptyStatus, ...over }) } as Response;
}

// StatusShell 把 useStatus 接到 AppShell 上，便于断言失败时的降级 Alert 与恢复。
function StatusShell() {
  const s = useStatus(5000);
  return <AppShell status={s.status} loading={s.loading} error={s.error} updatedAt={s.updatedAt} onRefresh={s.refresh} />;
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('看板状态呈现', () => {
  it('agents 为空时显示明确空态', () => {
    render(<AgentsTable agents={[]} />);
    expect(screen.getByText('当前没有 AI 在执行')).toBeInTheDocument();
  });

  it('开发中状态使用琥珀色语义映射', () => {
    render(<StatusBadge status="开发中" />);
    expect(screen.getByText('开发中')).toHaveClass('text-status-develop');
  });

  it('采集错误显示降级 Alert', () => {
    render(
      <AppShell
        status={{ ...emptyStatus, errors: { tasks: '读取任务表失败' } }}
        loading={false}
        error={null}
        updatedAt={null}
        onRefresh={vi.fn()}
      />,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('采集降级：tasks: 读取任务表失败');
  });

  it('prs 为 null 时显示 gh 降级说明', () => {
    render(<PrTable prs={null} errors={{ prs: 'gh 未登录' }} />);
    expect(screen.getByText(/PR 数据降级/)).toHaveTextContent('gh 未登录');
  });

  it('日志在空态和有内容间切换后仍显示内容', () => {
    const { rerender } = render(<LogsTabs logs={{ 'implementer.log': ['开始执行'] }} />);
    rerender(<LogsTabs logs={{}} />);
    expect(screen.getByText('无日志')).toBeInTheDocument();
    rerender(<LogsTabs logs={{ 'implementer.log': ['开始执行'] }} />);
    expect(screen.getByRole('tab', { name: 'implementer.log' })).toBeInTheDocument();
    expect(screen.getByText('开始执行')).toBeVisible();
  });

  it('点击其他日志标签可切换内容', async () => {
    render(<LogsTabs logs={{ 'alpha.log': ['甲'], 'beta.log': ['乙'] }} />);
    expect(screen.getByText('甲')).toBeVisible();
    expect(screen.queryByText('乙')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('tab', { name: 'beta.log' }));
    expect(screen.getByText('乙')).toBeVisible();
    expect(screen.queryByText('甲')).not.toBeInTheDocument();
  });
});

describe('useStatus', () => {
  it('上一轮请求未完成时不叠加轮询请求', () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn(() => new Promise<Response>(() => {}));
    vi.stubGlobal('fetch', fetchMock);

    const { unmount } = renderHook(() => useStatus(5000));
    expect(fetchMock).toHaveBeenCalledTimes(1);

    act(() => {
      vi.advanceTimersByTime(15_000);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    unmount();
  });

  it('上一轮结束后下一轮正常恢复轮询', async () => {
    vi.useFakeTimers();
    let resolveFirst!: (v: Response) => void;
    const first = new Promise<Response>((res) => { resolveFirst = res; });
    const fetchMock = vi
      .fn()
      .mockReturnValueOnce(first)
      .mockResolvedValue(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    const { result, unmount } = renderHook(() => useStatus(5000));
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // 第一轮未完成：轮询间隔不叠加请求。
    act(() => { vi.advanceTimersByTime(15_000); });
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // 完成第一轮后，下一个间隔应恢复拉取并更新数据。
    await act(async () => { resolveFirst(okResponse()); });
    expect(result.current.loading).toBe(false);
    await act(async () => { vi.advanceTimersByTime(5000); });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await act(async () => {});
    expect(result.current.status).toEqual(emptyStatus);
    unmount();
  });

  it('请求失败时保留旧数据并渲染降级 Alert，恢复后清除错误', async () => {
    vi.useFakeTimers();
    const agent = (tool: string) => ({ tool, role: 'planner', pid: '1', ppid: '0', uptime: '1h', cwd: '/x', cmd: tool + ' --role' });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(okResponse({ agents: [agent('claude')] }))
      .mockRejectedValueOnce(new Error('network'))
      .mockResolvedValue(okResponse({ agents: [agent('codex')] }));
    vi.stubGlobal('fetch', fetchMock);

    const { unmount } = render(<StatusShell />);
    // 第一轮成功：渲染数据区。
    await act(async () => {});
    expect(screen.getAllByText('执行中 AI').length).toBeGreaterThan(0);

    // 第二轮失败：保留旧数据、记录并渲染降级 Alert。
    await act(async () => { vi.advanceTimersByTime(5000); });
    await act(async () => {});
    expect(screen.getByRole('alert')).toHaveTextContent('后端不可用');
    expect(screen.getAllByText('执行中 AI').length).toBeGreaterThan(0);

    // 第三轮成功：错误清除、数据更新。
    await act(async () => { vi.advanceTimersByTime(5000); });
    await act(async () => {});
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByText('codex')).toBeInTheDocument();
    unmount();
  });
});
