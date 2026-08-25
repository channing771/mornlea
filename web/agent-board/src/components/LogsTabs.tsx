import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';

const NL = String.fromCharCode(10);

// LogsTabs 用 Tabs 分标签展示各日志文件末尾片段；内容置于等宽 <pre> 中可滚动。
function LogsTabs({ logs }: { logs: Record<string, string[]> }) {
  const entries = Object.entries(logs || {});
  if (entries.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
        无日志
      </div>
    );
  }
  return (
    <Tabs defaultValue={entries[0][0]} className="w-full">
      <TabsList className="h-auto flex-wrap justify-start">
        {entries.map(([k]) => (
          <TabsTrigger key={k} value={k}>{k}</TabsTrigger>
        ))}
      </TabsList>
      {entries.map(([k, lines]) => (
        <TabsContent key={k} value={k}>
          <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-lg border bg-card p-3 font-mono text-xs leading-relaxed text-foreground">
            {lines.join(NL)}
          </pre>
        </TabsContent>
      ))}
    </Tabs>
  );
}

export { LogsTabs };
