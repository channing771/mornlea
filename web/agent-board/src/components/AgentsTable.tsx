import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { AgentStatus } from '@/api';

// AgentsTable 展示执行中的 AI 进程。
function AgentsTable({ agents }: { agents: AgentStatus[] }) {
  if (!agents || agents.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
        当前没有 AI 在执行
      </div>
    );
  }
  return (
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
            <TableCell>{a.tool}</TableCell>
            <TableCell>{a.role}</TableCell>
            <TableCell className="font-mono text-xs">{a.pid}</TableCell>
            <TableCell>{a.uptime}</TableCell>
            <TableCell className="font-mono text-xs">{a.cwd || '—'}</TableCell>
            <TableCell className="max-w-[40ch] truncate font-mono text-xs">{a.cmd}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

export { AgentsTable };
