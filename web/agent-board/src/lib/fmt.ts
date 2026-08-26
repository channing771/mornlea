// 展示格式化工具：时长与任务状态徽章映射。

export type TaskStatus = '就绪' | '已认领' | '开发中' | '待集成' | '排队' | '设计候选' | '已完成' | '已取消' | '其他';

// fmtDur 把秒数格式化为中文相对时长。
export function fmtDur(sec: number | null | undefined): string {
  if (sec == null) return '';
  if (sec < 0) sec = 0;
  if (sec < 60) return '刚刚';
  if (sec < 3600) return `${Math.floor(sec / 60)} 分钟前`;
  if (sec < 86400) return `${Math.floor(sec / 3600)} 小时前`;
  return `${Math.floor(sec / 86400)} 天前`;
}

// statusClassMap 把任务状态映射为徽章用的 Tailwind 类（背景 + 文字色）。
const statusClassMap: Record<TaskStatus, string> = {
  '就绪': 'bg-status-done/15 text-status-done',
  '已认领': 'bg-status-claimed/15 text-status-claimed',
  '开发中': 'bg-status-develop/15 text-status-develop',
  '待集成': 'bg-status-integrate/15 text-status-integrate',
  '排队': 'bg-status-other/15 text-status-other',
  '设计候选': 'bg-status-other/15 text-status-other',
  '已完成': 'bg-status-done/15 text-status-done',
  '已取消': 'bg-status-other/15 text-status-other',
  '其他': 'bg-status-other/15 text-status-other',
};

// statusClass 返回任务状态对应的徽章类；未知状态回退「其他」。
export function statusClass(status: string): string {
  return statusClassMap[status as TaskStatus] ?? statusClassMap['其他'];
}
