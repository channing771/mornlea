// 设置页屏：三字段（总音量/材质包目录/窗口大小）草稿编辑经 settings-change
// 上行回传，保存/取消/返回经 action 上行。草稿语义（脏判定、原子保存、生效
// 时机）由 Go 裁决，前端不比较持久化值；控件文案与 ui.rs 常量一致（copy.ts）。
import type { SettingsState, UplinkEvent } from "../bridge/client";
import {
  SETTINGS_AUDIO_LABEL,
  SETTINGS_BACK_LABEL,
  SETTINGS_CANCEL_LABEL,
  SETTINGS_DIRTY_HINT,
  SETTINGS_SAVE_LABEL,
  SETTINGS_TEXTURE_HINT,
  SETTINGS_TEXTURE_LABEL,
  SETTINGS_TEXTURE_PLACEHOLDER,
  SETTINGS_TITLE,
  SETTINGS_WINDOW_LABEL,
  WINDOW_SIZE_PRESETS,
} from "./copy";

export interface SettingsPanelProps {
  settings: SettingsState;
  onEvent: (event: UplinkEvent) => void;
}

export function SettingsPanel({ settings, onEvent }: SettingsPanelProps) {
  const { draft, dirty, status, error } = settings;
  return (
    <section className="settings-screen">
      <div className="settings-panel">
        <h1 className="settings-title">{SETTINGS_TITLE}</h1>
        <label className="settings-field">
          <span className="settings-field-label">{SETTINGS_AUDIO_LABEL}</span>
          <input
            type="range"
            min={0}
            max={100}
            step={1}
            value={Math.round(draft.audioVolume * 100)}
            onChange={(event) => {
              // 滑杆为 0..100 整数刻度，回传归一到 [0,1]（schema 口径）。
              onEvent({
                type: "settings-change",
                field: "audioVolume",
                value: Number(event.currentTarget.value) / 100,
              });
            }}
          />
        </label>
        <label className="settings-field">
          <span className="settings-field-label">{SETTINGS_TEXTURE_LABEL}</span>
          <input
            type="text"
            className="settings-text-input"
            value={draft.texturePackPath}
            placeholder={SETTINGS_TEXTURE_PLACEHOLDER}
            onChange={(event) => {
              // 路径单行语义：输入端的换行直接剥离，协议层另有拒绝兜底。
              onEvent({
                type: "settings-change",
                field: "texturePackPath",
                value: event.currentTarget.value.replace(/[\r\n]/g, ""),
              });
            }}
          />
        </label>
        <p className="settings-hint">{SETTINGS_TEXTURE_HINT}</p>
        <fieldset className="settings-window">
          <legend>{SETTINGS_WINDOW_LABEL}</legend>
          {WINDOW_SIZE_PRESETS.map((preset) => (
            <button
              key={preset.value}
              type="button"
              className="settings-window-button"
              aria-pressed={draft.windowSize === preset.value}
              onClick={() => {
                onEvent({
                  type: "settings-change",
                  field: "windowSize",
                  value: preset.value,
                });
              }}
            >
              {preset.label}
            </button>
          ))}
        </fieldset>
        {dirty && <p className="settings-dirty">{SETTINGS_DIRTY_HINT}</p>}
        {status !== "" && <p className="settings-status">{status}</p>}
        {error !== "" && (
          <p className="settings-error" role="alert">
            {error}
          </p>
        )}
        <div className="settings-actions">
          <button
            type="button"
            className="menu-button"
            onClick={() => {
              onEvent({ type: "action", id: "settings-save" });
            }}
          >
            {SETTINGS_SAVE_LABEL}
          </button>
          <button
            type="button"
            className="menu-button"
            onClick={() => {
              onEvent({ type: "action", id: "settings-cancel" });
            }}
          >
            {SETTINGS_CANCEL_LABEL}
          </button>
          <button
            type="button"
            className="menu-button"
            onClick={() => {
              onEvent({ type: "action", id: "settings-back" });
            }}
          >
            {SETTINGS_BACK_LABEL}
          </button>
        </div>
      </div>
    </section>
  );
}
