import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { EmptyHint } from '@/components/EmptyHint';
import type { AgentStatus } from '@/api';

// AgentsTable 展示执行中的 AI 进程；数据列（pid/时长/cwd/命令）一律 mono。
function AgentsTable({ agents }: { agents: AgentStatus[] }) {
  if (!agents || agents.length === 0) {
    return <EmptyHint>当前没有 AI 在执行</EmptyHint>;
  }
  return (
    <div className="overflow-hidden rounded-md border border-border bg-card">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>工具</TableHead>
          <TableHead>角色</TableHead>
          <TableHead>pid</TableHead>
          <TableHead>运行时长</TableHead>
          <TableHead>cwd</TableHead>
          <TableHead>命令摘要</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {agents.map((a, i) => (
          <TableRow key={a.pid || i}>
            <TableCell className="font-mono text-xs">{a.tool}</TableCell>
            <TableCell className="font-mono text-xs">{a.role}</TableCell>
            <TableCell className="font-mono text-xs">{a.pid}</TableCell>
            <TableCell className="font-mono text-xs">{a.uptime}</TableCell>
            <TableCell className="max-w-[36ch] truncate font-mono text-xs" title={a.cwd || ''}>
              {a.cwd || '-'}
            </TableCell>
            <TableCell className="max-w-[40ch] truncate font-mono text-xs" title={a.cmd}>
              {a.cmd}
            </TableCell>
          </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

export { AgentsTable };
