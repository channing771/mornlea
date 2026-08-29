// 暂停层屏：半透明遮罩 + 固定标题 + 两枚固定按钮 + 远程注明行。按钮文案与
// ui.rs PAUSE_* 常量一致；「返回/退回」动作经桥上行，Esc 的开合由 Go 键位栈
// 裁决（Rust/前端不合成 Escape 动作），恢复或拆链的裁决权在 Go 相位机。
import type { PauseState, UplinkEvent } from "../bridge/client";
import {
  PAUSE_BACK_LABEL,
  PAUSE_QUIT_TO_MENU_LABEL,
  PAUSE_REMOTE_NOTE,
  PAUSE_TITLE,
} from "./copy";

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
          <button
            type="button"
            className="menu-button"
            onClick={() => {
              onEvent({ type: "action", id: "pause-back" });
            }}
          >
            {PAUSE_BACK_LABEL}
          </button>
          <button
            type="button"
            className="menu-button"
            onClick={() => {
              onEvent({ type: "action", id: "pause-quit-to-menu" });
            }}
          >
            {PAUSE_QUIT_TO_MENU_LABEL}
          </button>
        </div>
        {pause.remote && <p className="pause-remote-note">{PAUSE_REMOTE_NOTE}</p>}
      </div>
    </section>
  );
}
