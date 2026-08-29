// F3 调试面板屏：读数区（mode + readout 行）、段头行与参数行列表。行内容全部
// 由 Go 下行驱动；选中移动/进入编辑/值输入/确认/取消/关闭经 debug-edit 上行，
// 由 Go 维护选中下标与编辑态裁决。联机只读、字节上限等语义在 Go 组装侧维持。
import { useState, type KeyboardEvent } from "react";
import type { DebugRow, DebugState, UplinkEvent } from "../bridge/client";

export interface DebugPanelProps {
  debug: DebugState;
  onEvent: (event: UplinkEvent) => void;
}

export function DebugPanel({ debug, onEvent }: DebugPanelProps) {
  const readouts = debug.rows.filter((row) => row.kind === "readout");
  const listRows = debug.rows.filter((row) => row.kind !== "readout");

  // 键盘语义：上下移动选中、Enter 进入编辑、Esc 取消编辑/关闭面板。选中下标
  // 的合法性（跳过只读行等）由 Go 裁决，前端只回传意图。
  const handleListKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    switch (event.key) {
      case "ArrowDown":
        onEvent({ type: "debug-edit", op: "select-next" });
        break;
      case "ArrowUp":
        onEvent({ type: "debug-edit", op: "select-prev" });
        break;
      case "Enter":
        onEvent({ type: "debug-edit", op: "enter-edit" });
        break;
      case "Escape":
        onEvent({ type: "debug-edit", op: listRows.some((row) => row.editing) ? "cancel" : "close" });
        break;
      default:
        return;
    }
    event.preventDefault();
  };

  return (
    <section className="debug-screen">
      <div className="debug-panel">
        <div className="debug-readout">
          <span className="debug-mode">{debug.mode}</span>
          {readouts.map((row) => (
            <p key={row.label} className="debug-readout-row">
              <span className="debug-label">{row.label}</span>
              <span className="debug-value">{row.value}</span>
            </p>
          ))}
        </div>
        <div
          className="debug-rows"
          role="listbox"
          aria-label="调试面板参数行"
          tabIndex={0}
          onKeyDown={handleListKeyDown}
        >
          {listRows.map((row) =>
            row.kind === "section" ? (
              <div key={row.label} className="debug-section">
                {row.label}
              </div>
            ) : (
              <div
                key={row.label}
                role="option"
                aria-selected={row.selected}
                className={row.selected ? "debug-row is-selected" : "debug-row"}
              >
                <span className="debug-label">{row.label}</span>
                {row.editing ? <DebugRowEditor row={row} onEvent={onEvent} /> : <span className="debug-value">{row.value}</span>}
              </div>
            ),
          )}
        </div>
      </div>
    </section>
  );
}

// 编辑态输入框：草稿留在呈现层（对齐 egui「文本草稿不进 Go 下行」的语义）。
// 挂载即以行值初始化；onChange 流式上行 edit-value，Enter 以本地草稿确认。
// 组件按编辑会话挂载/卸载，草稿不会跨会话泄漏。
function DebugRowEditor({ row, onEvent }: { row: DebugRow; onEvent: (event: UplinkEvent) => void }) {
  const [draft, setDraft] = useState(row.value);
  return (
    <input
      className="debug-edit-input"
      aria-label={row.label}
      value={draft}
      onChange={(event) => {
        const next = event.currentTarget.value.replace(/[\r\n]/g, "");
        setDraft(next);
        onEvent({ type: "debug-edit", op: "edit-value", value: next });
      }}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          onEvent({ type: "debug-edit", op: "confirm", value: draft });
        }
      }}
    />
  );
}
