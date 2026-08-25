import { AppShell } from '@/components/AppShell';
import { useStatus } from '@/api';

// App 顶层：驱动 useStatus 轮询并把状态分发给 AppShell。
export default function App() {
  const { status, loading, error, updatedAt, refresh } = useStatus();
  return <AppShell status={status} loading={loading} error={error} updatedAt={updatedAt} onRefresh={refresh} />;
}
