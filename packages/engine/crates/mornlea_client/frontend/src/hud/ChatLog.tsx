// 聊天呈现：最近聊天行的背衬栈（暖白文字 + 硬投影），锚在视口左缘、
// 状态栈上沿之上一个净空间隙。行序即呈现序（首行最旧、末行最新），
// 上限六行由 Go 的行缓冲裁剪后下行；聊天输入仍走 winit 采集路径，
// 这里不渲染任何输入控件。
export function ChatLog({ lines }: { lines: readonly string[] }) {
  return (
    <div className="hud-chat">
      {lines.map((line, index) => (
        // 空串是合法行且占用一个行槽（与迁移前同口径），渲染为空行占位。
        <div key={index} className="hud-chat-line">
          {line}
        </div>
      ))}
    </div>
  );
}
