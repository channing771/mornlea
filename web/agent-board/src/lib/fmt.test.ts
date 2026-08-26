import { describe, expect, it } from 'vitest';
import { fmtDur, statusClass } from '@/lib/fmt';

describe('fmtDur', () => {
  it.each([
    [0, '刚刚'],
    [59, '刚刚'],
    [60, '1 分钟前'],
    [3600, '1 小时前'],
    [86400, '1 天前'],
  ])('把 %i 秒格式化为 %s', (seconds, want) => {
    expect(fmtDur(seconds)).toBe(want);
  });
});

describe('statusClass', () => {
  it('未知状态回退到其他样式', () => {
    expect(statusClass('待确认')).toContain('text-status-other');
  });

  it('当前规划状态都有稳定样式', () => {
    expect(statusClass('就绪')).toContain('text-status-unclaimed');
    for (const status of ['排队', '设计候选', '已取消']) {
      expect(statusClass(status)).toContain('text-status-other');
    }
  });
});
