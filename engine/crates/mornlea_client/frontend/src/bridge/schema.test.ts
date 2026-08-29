// schema.json 单源钉值测试：用 ajv（draft 2020-12）校验合法/非法夹具，
// 保证 TS 类型、Go 组装与 Rust 消费共享同一份契约。非法用例重点覆盖
// 「未知相位/未知动作/未知事件类型必须拒绝」与越界值。
import Ajv2020 from "ajv/dist/2020";
import type { SchemaObject } from "ajv";
import { describe, expect, it } from "vitest";
import type { SettingsValues, UIState, UplinkEnvelope } from "./client";
import bridgeSchemaJson from "./schema.json";

// JSON 模块导入的推导类型不含 ajv 的 keyword 索引签名，经 SchemaObject
// 收口（非 any 逃逸）；schema 本身由 ajv strict 编译兜底。
const bridgeSchema = bridgeSchemaJson as unknown as SchemaObject;

const ajv = new Ajv2020({ strict: true });
ajv.compile(bridgeSchema);
const validateUiState = ajv.compile<UIState>({
  $ref: "mornlea://bridge/schema.json#/$defs/uiState",
});
const validateEnvelope = ajv.compile<UplinkEnvelope>({
  $ref: "mornlea://bridge/schema.json#/$defs/uplinkEnvelope",
});

const menuState: UIState = {
  phase: "menu",
  menu: {
    title: "Mornlea",
    version: "dev",
    error: "",
    buttons: [
      { id: "enter-game", label: "进入游戏", enabled: true },
      { id: "multiplayer", label: "多人游戏", enabled: false },
      { id: "open-settings", label: "设置", enabled: true },
      { id: "quit", label: "退出游戏", enabled: true },
    ],
  },
};

const savedDraft: SettingsValues = {
  audioVolume: 0.5,
  texturePackPath: "packs/local",
  windowSize: "960x540",
};

const settingsState: UIState = {
  phase: "settings",
  settings: {
    draft: { audioVolume: 0.37, texturePackPath: "", windowSize: "640x360" },
    saved: savedDraft,
    dirty: true,
    status: "",
    error: "",
  },
};

const pausedState: UIState = {
  phase: "paused",
  pause: { remote: true },
  debug: {
    visible: false,
    mode: "本地单机",
    rows: [
      { label: "── sim ──", value: "", kind: "section", readonly: true, selected: false, editing: false },
      { label: "gravity", value: "9.8", kind: "param", readonly: false, selected: true, editing: false },
    ],
  },
};

describe("schema：下行 uiState 合法夹具", () => {
  it("主菜单相位（四按钮、多人禁用）通过校验", () => {
    expect(validateUiState(menuState)).toBe(true);
  });

  it("starting 相位复用 menu 分节", () => {
    expect(validateUiState({ ...menuState, phase: "starting" })).toBe(true);
  });

  it("settings 相位（draft/saved/dirty/status/error）通过校验", () => {
    expect(validateUiState(settingsState)).toBe(true);
  });

  it("paused 相位 + debug 分节（含远程注明与调试行）通过校验", () => {
    expect(validateUiState(pausedState)).toBe(true);
  });

  it("game 相位仅 debug 可见（F3 叠加于游戏）通过校验", () => {
    expect(
      validateUiState({
        phase: "game",
        debug: {
          visible: true,
          mode: "本地单机",
          rows: [
            { label: "fps", value: "60", kind: "readout", readonly: true, selected: false, editing: false },
          ],
        },
      }),
    ).toBe(true);
  });
});

describe("schema：下行 uiState 非法用例一律拒绝", () => {
  it("未知 phase 拒绝", () => {
    expect(validateUiState({ phase: "loading" })).toBe(false);
  });

  it("缺 phase 拒绝", () => {
    expect(validateUiState({})).toBe(false);
  });

  it("未知按钮 id 拒绝", () => {
    expect(
      validateUiState({
        phase: "menu",
        menu: {
          title: "Mornlea",
          version: "dev",
          error: "",
          buttons: [{ id: "teleport", label: "传送", enabled: true }],
        },
      }),
    ).toBe(false);
  });

  it("按钮缺 enabled 拒绝", () => {
    expect(
      validateUiState({
        phase: "menu",
        menu: {
          title: "Mornlea",
          version: "dev",
          error: "",
          buttons: [{ id: "enter-game", label: "进入游戏" }],
        },
      }),
    ).toBe(false);
  });

  it("audioVolume 越界拒绝", () => {
    const bad = (volume: number): UIState => ({
      phase: "settings",
      settings: {
        draft: { audioVolume: volume, texturePackPath: "", windowSize: "640x360" },
        saved: savedDraft,
        dirty: false,
        status: "",
        error: "",
      },
    });
    expect(validateUiState(bad(1.01))).toBe(false);
    expect(validateUiState(bad(-0.01))).toBe(false);
  });

  it("未知 windowSize 预设拒绝", () => {
    expect(
      validateUiState({
        phase: "settings",
        settings: {
          draft: { audioVolume: 0.5, texturePackPath: "", windowSize: "800x600" },
          saved: savedDraft,
          dirty: false,
          status: "",
          error: "",
        },
      }),
    ).toBe(false);
  });

  it("材质路径含换行拒绝", () => {
    expect(
      validateUiState({
        phase: "settings",
        settings: {
          draft: { audioVolume: 0.5, texturePackPath: "a\nb", windowSize: "640x360" },
          saved: savedDraft,
          dirty: false,
          status: "",
          error: "",
        },
      }),
    ).toBe(false);
  });

  it("readonly 调试行被置位 selected 拒绝", () => {
    expect(
      validateUiState({
        phase: "game",
        debug: {
          visible: true,
          mode: "本地单机",
          rows: [
            { label: "fps", value: "60", kind: "param", readonly: true, selected: true, editing: false },
          ],
        },
      }),
    ).toBe(false);
  });

  it("参数行携带 editValue 编辑播种文本通过校验", () => {
    expect(
      validateUiState({
        phase: "game",
        debug: {
          visible: true,
          mode: "本地单机",
          rows: [
            {
              label: "gravity",
              value: "9.807",
              kind: "param",
              readonly: false,
              selected: true,
              editing: true,
              editValue: "9.80665",
            },
          ],
        },
      }),
    ).toBe(true);
  });

  it("editValue 含换行或非字符串拒绝", () => {
    const row = (editValue: unknown) => ({
      label: "gravity",
      value: "9.807",
      kind: "param",
      readonly: false,
      selected: true,
      editing: true,
      editValue,
    });
    expect(validateUiState({ phase: "game", debug: { visible: true, mode: "本地单机", rows: [row("9.80\n665")] } })).toBe(false);
    expect(validateUiState({ phase: "game", debug: { visible: true, mode: "本地单机", rows: [row(9.80665)] } })).toBe(false);
  });

  it("调试行超过 64 行拒绝", () => {
    const rows = Array.from({ length: 65 }, () => ({
      label: "x",
      value: "1",
      kind: "param" as const,
      readonly: false,
      selected: false,
      editing: false,
    }));
    expect(
      validateUiState({ phase: "game", debug: { visible: true, mode: "m", rows } }),
    ).toBe(false);
  });

  it("顶层未知属性拒绝", () => {
    expect(validateUiState({ phase: "menu", cheat: true })).toBe(false);
  });
});

describe("schema：上行 uplinkEnvelope 合法夹具", () => {
  it("action 事件通过校验", () => {
    expect(validateEnvelope({ v: 1, events: [{ type: "action", id: "enter-game" }] })).toBe(true);
  });

  it("三类 settings-change 值形态各自通过校验", () => {
    expect(
      validateEnvelope({
        v: 1,
        events: [
          { type: "settings-change", field: "audioVolume", value: 0.37 },
          { type: "settings-change", field: "texturePackPath", value: "packs/local" },
          { type: "settings-change", field: "windowSize", value: "1280x720" },
        ],
      }),
    ).toBe(true);
  });

  it("debug-edit：edit-value/confirm 携带 value，通过校验", () => {
    expect(
      validateEnvelope({
        v: 1,
        events: [
          { type: "debug-edit", op: "edit-value", value: "12" },
          { type: "debug-edit", op: "confirm", value: "12" },
        ],
      }),
    ).toBe(true);
  });

  it("debug-edit：无 value 的 op（select-next 等）通过校验", () => {
    expect(
      validateEnvelope({
        v: 1,
        events: [
          { type: "debug-edit", op: "select-next" },
          { type: "debug-edit", op: "select-prev" },
          { type: "debug-edit", op: "enter-edit" },
          { type: "debug-edit", op: "cancel" },
          { type: "debug-edit", op: "close" },
        ],
      }),
    ).toBe(true);
  });
});

describe("schema：上行 uplinkEnvelope 非法用例一律拒绝", () => {
  it("未知事件类型拒绝（未知 type 落不进任何 oneOf 分支）", () => {
    expect(validateEnvelope({ v: 1, events: [{ type: "teleport" }] })).toBe(false);
  });

  it("未知动作 id 拒绝", () => {
    expect(validateEnvelope({ v: 1, events: [{ type: "action", id: "do-magic" }] })).toBe(false);
  });

  it("事件携带未知属性拒绝", () => {
    expect(
      validateEnvelope({ v: 1, events: [{ type: "action", id: "quit", extra: 1 }] }),
    ).toBe(false);
  });

  it("settings-change 的 audioVolume 值越界或类型错误拒绝", () => {
    expect(
      validateEnvelope({ v: 1, events: [{ type: "settings-change", field: "audioVolume", value: 1.5 }] }),
    ).toBe(false);
    expect(
      validateEnvelope({ v: 1, events: [{ type: "settings-change", field: "audioVolume", value: "loud" }] }),
    ).toBe(false);
  });

  it("settings-change 的 windowSize 值未知预设拒绝", () => {
    expect(
      validateEnvelope({
        v: 1,
        events: [{ type: "settings-change", field: "windowSize", value: "800x600" }],
      }),
    ).toBe(false);
  });

  it("debug-edit：edit-value 缺 value 拒绝", () => {
    expect(validateEnvelope({ v: 1, events: [{ type: "debug-edit", op: "edit-value" }] })).toBe(false);
  });

  it("debug-edit：select-next 携带 value 拒绝", () => {
    expect(
      validateEnvelope({ v: 1, events: [{ type: "debug-edit", op: "select-next", value: "x" }] }),
    ).toBe(false);
  });

  it("未知 debug-edit op 拒绝", () => {
    expect(validateEnvelope({ v: 1, events: [{ type: "debug-edit", op: "rewind" }] })).toBe(false);
  });

  it("信封版本非 1 拒绝", () => {
    expect(validateEnvelope({ v: 2, events: [{ type: "action", id: "quit" }] })).toBe(false);
  });

  it("空事件批与超 64 条事件批拒绝", () => {
    expect(validateEnvelope({ v: 1, events: [] })).toBe(false);
    const events = Array.from({ length: 65 }, () => ({ type: "action" as const, id: "quit" as const }));
    expect(validateEnvelope({ v: 1, events })).toBe(false);
  });

  it("缺 events 拒绝", () => {
    expect(validateEnvelope({ v: 1 })).toBe(false);
  });
});
