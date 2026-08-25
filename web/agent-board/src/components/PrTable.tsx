import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { EmptyHint } from '@/components/EmptyHint';
import type { PRStatus } from '@/api';

function checkSummary(checks: PRStatus['checks']): string {
  if (!checks || checks.length === 0) return '-';
  const ok = checks.filter((c) => c.state === 'success').length;
  const mark = ok === checks.length ? ' ✓' : '';
  return ok + '/' + checks.length + mark;
}

// PrTable 展示打开 PR；prs 为 null 表示 gh 不可用，展示降级说明。
function PrTable({ prs, errors }: { prs: PRStatus[] | null; errors: Record<string, string> }) {
  if (prs === null) {
    return (
      <EmptyHint>
        gh 未登录或不可用，PR 数据降级。{errors.prs ? ' ' + errors.prs : ''}
      </EmptyHint>
    );
  }
  if (prs.length === 0) {
    return <EmptyHint>暂无打开 PR</EmptyHint>;
  }
  return (
    <div className="overflow-hidden rounded-md border border-border bg-card">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>#</TableHead>
          <TableHead>标题</TableHead>
          <TableHead>分支</TableHead>
          <TableHead>mergeState</TableHead>
          <TableHead>检查</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {prs.map((p) => (
          <TableRow key={p.number}>
            <TableCell className="font-mono text-xs">#{p.number}</TableCell>
            <TableCell>
              {p.url ? (
                <a
                  href={p.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="rounded-md text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  {p.title}
                </a>
              ) : (
                p.title
              )}
            </TableCell>
            <TableCell className="font-mono text-xs">{p.branch || '-'}</TableCell>
            <TableCell className="font-mono text-xs">{p.mergeState || '-'}</TableCell>
            <TableCell className="font-mono text-xs">{checkSummary(p.checks)}</TableCell>
          </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

export { PrTable };
