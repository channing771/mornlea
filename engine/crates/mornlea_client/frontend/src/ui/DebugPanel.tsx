// F3 调试面板屏：读数区（mode + readout 行）、段头行与参数行列表。行内容全部
// 由 Go 下行驱动；选中移动/进入编辑/值输入/确认/取消/关闭经 debug-edit 上行，
// 由 Go 维护选中下标与编辑态裁决。联机只读、字节上限等语义在 Go 组装侧维持。
// 面板键（F3/Esc/方向键/Enter）由 App 的窗口级路由统一上行，本组件只承载
// 编辑输入框：Enter 就地确认（拦截冒泡避免中枢再解释），Esc 冒泡给中枢取消。
import { useState } from "react";
import type { DebugRow, DebugState, UplinkEvent } from "../bridge/client";
import { PixelInput } from "./pixel";

export interface DebugPanelProps {
  debug: DebugState;
  onEvent: (event: UplinkEvent) => void;
}

export function DebugPanel({ debug, onEvent }: DebugPanelProps) {
  const readouts = debug.rows.filter((row) => row.kind === "readout");
  const listRows = debug.rows.filter((row) => row.kind !== "readout");

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
        <div className="debug-rows" role="listbox" aria-label="调试面板参数行">
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
                aria-disabled={row.readonly || undefined}
                className={readOnlyRowClassName(row)}
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

// readOnlyRowClassName 给只读参数行附加几何可辨的禁用态标记：只读行不可
// 选中与编辑（Go 导航天然跳过，aria-disabled 同步呈现该语义）。
function readOnlyRowClassName(row: DebugRow): string {
  const base = row.selected ? "debug-row is-selected" : "debug-row";
  return row.readonly ? `${base} is-readonly` : base;
}

// 编辑态输入框：草稿留在呈现层（对齐「文本草稿不进 Go 下行」的语义）。
// 挂载即以行值初始化；onChange 流式上行 edit-value，Enter 以本地草稿确认。
// 组件按编辑会话挂载/卸载，草稿不会跨会话泄漏。
//
// 播种精度：展示值只有 4 位有效数字，播种必须优先用下行的 editValue
// （全精度文本）——「不改文本直接确认」携带的播种原文写回才不漂移有效值；
// 旧状态无 editValue 时回退展示值（Go 侧自该版本起恒携带）。
//
// autoFocus：编辑会话开启时 WebView 是 firstResponder，焦点必须落进输入框
// 才能接收文本；confirm 事件 stopPropagation，避免冒泡到 App 的窗口级路由
// 被再次解释（编辑态中枢本就忽略 Enter，这里双保险）。呈现面为 PixelInput
// （像素描边框），aria-label/键盘/流式上行的语义保持原生零改动。
function DebugRowEditor({ row, onEvent }: { row: DebugRow; onEvent: (event: UplinkEvent) => void }) {
  const [draft, setDraft] = useState(row.editValue ?? row.value);
  return (
    <PixelInput
      className="debug-edit-input"
      aria-label={row.label}
      autoFocus
      value={draft}
      onChange={(event) => {
        const next = event.currentTarget.value.replace(/[\r\n]/g, "");
        setDraft(next);
        onEvent({ type: "debug-edit", op: "edit-value", value: next });
      }}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          event.stopPropagation();
          event.preventDefault();
          onEvent({ type: "debug-edit", op: "confirm", value: draft });
        }
      }}
    />
  );
}
