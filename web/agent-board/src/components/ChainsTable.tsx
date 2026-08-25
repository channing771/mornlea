import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { ChainStatus } from '@/api';

// ChainsTable 展示接力链；存活绿、已死灰，stale 额外标记。
function ChainsTable({ chains }: { chains: ChainStatus[] }) {
  if (!chains || chains.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
        无接力链
      </div>
    );
  }
  return (
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
            <TableCell>{c.id}</TableCell>
            <TableCell className="font-mono text-xs">{c.pid || ''}</TableCell>
            <TableCell>
              {c.alive ? (
                <Badge variant="success" className="rounded-full">存活</Badge>
              ) : (
                <Badge variant="secondary" className="rounded-full">已死</Badge>
              )}
              {c.stale && <span className="ml-2 text-xs text-muted-foreground">(stale)</span>}
            </TableCell>
            <TableCell className="font-mono text-xs">{c.mtime || ''}</TableCell>
            <TableCell className="text-xs text-muted-foreground">{c.note || ''}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

export { ChainsTable };
