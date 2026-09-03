// 暂停层屏：半透明遮罩 + 固定标题 + 两枚固定按钮 + 远程注明行。按钮文案与
// 退役 egui 菜单的 PAUSE_* 常量沿用值一致；「返回/退回」动作经桥上行。覆盖层可见时 WebView 是
// firstResponder，winit 收不到 Esc——Esc=返回游戏经 App 的窗口级键盘路由上行
// pause-back（开层的 Esc 在游戏相位仍由 winit 捕获，两侧不会同帧回声），
// 恢复或拆链的裁决权在 Go 相位机（防重入哨兵保证重复事件只生效一次）。
import type { PauseState, UplinkEvent } from "../bridge/client";
import {
  PAUSE_BACK_LABEL,
  PAUSE_QUIT_TO_MENU_LABEL,
  PAUSE_REMOTE_NOTE,
  PAUSE_TITLE,
} from "./copy";
import { PixelButton } from "./pixel";

export interface PauseMenuProps {
  pause: PauseState;
  onEvent: (event: UplinkEvent) => void;
}

export function PauseMenu({ pause, onEvent }: PauseMenuProps) {
  return (
    <section className="pause-screen">
      <div className="pause-content">
        <h1 className="pause-title">{PAUSE_TITLE}</h1>
        <div className="menu-buttons">
          <PixelButton
            type="button"
            className="menu-button"
            onClick={() => {
              onEvent({ type: "action", id: "pause-back" });
            }}
          >
            {PAUSE_BACK_LABEL}
          </PixelButton>
          <PixelButton
            type="button"
            className="menu-button"
            onClick={() => {
              onEvent({ type: "action", id: "pause-quit-to-menu" });
            }}
          >
            {PAUSE_QUIT_TO_MENU_LABEL}
          </PixelButton>
        </div>
        {pause.remote && <p className="pause-remote-note">{PAUSE_REMOTE_NOTE}</p>}
      </div>
    </section>
  );
}
