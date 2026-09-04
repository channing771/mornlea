import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { EmptyHint } from '@/components/EmptyHint';
import type { ChainStatus } from '@/api';

// ChainsTable 展示接力链；存活绿（emerald）、已死中性 slate，stale 以小号 muted 副标呈现。
function ChainsTable({ chains }: { chains: ChainStatus[] }) {
  if (!chains || chains.length === 0) {
    return <EmptyHint>无接力链</EmptyHint>;
  }
  return (
    <div className="overflow-hidden rounded-md border border-border bg-card">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>链</TableHead>
          <TableHead>pid</TableHead>
          <TableHead>存活</TableHead>
          <TableHead>守卫修改时间</TableHead>
          <TableHead>注记</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {chains.map((c, i) => (
          <TableRow key={c.id || i}>
            <TableCell className="font-mono text-xs">{c.id}</TableCell>
            <TableCell className="font-mono text-xs">{c.pid || '-'}</TableCell>
            <TableCell>
              {c.alive ? (
                <Badge variant="success">存活</Badge>
              ) : (
                <Badge variant="secondary">已死</Badge>
              )}
              {c.stale && <span className="ml-2 text-xs text-muted-foreground">(stale)</span>}
            </TableCell>
            <TableCell className="font-mono text-xs">{c.mtime || '-'}</TableCell>
            <TableCell className="text-xs text-muted-foreground">{c.note || '-'}</TableCell>
          </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

export { ChainsTable };
