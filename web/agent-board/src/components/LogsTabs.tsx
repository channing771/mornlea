import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { EmptyHint } from '@/components/EmptyHint';

const NL = String.fromCharCode(10);

// LogsTabs 用 Tabs 分标签展示各日志文件末尾片段；标签名即文件名，内容展开/收起由 Radix 机制承载。
// 一个白色面板装 Tabs 与 <pre>；<pre> 为浅色终端-inset：纸底 + hairline 边框，mono 12px。
function LogsTabs({ logs }: { logs: Record<string, string[]> }) {
  const entries = Object.entries(logs || {});
  if (entries.length === 0) {
    return <EmptyHint>无日志</EmptyHint>;
  }
  return (
    <Tabs defaultValue={entries[0][0]} className="w-full">
      <div className="rounded-md border border-border bg-card p-3">
        <TabsList className="flex-wrap justify-start">
          {entries.map(([k]) => (
            <TabsTrigger key={k} value={k}>{k}</TabsTrigger>
          ))}
        </TabsList>
        {entries.map(([k, lines]) => (
          <TabsContent key={k} value={k}>
            <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md border border-border bg-background p-3 font-mono text-xs leading-relaxed text-foreground">
              {lines.join(NL)}
            </pre>
          </TabsContent>
        ))}
      </div>
    </Tabs>
  );
}

export { LogsTabs };
