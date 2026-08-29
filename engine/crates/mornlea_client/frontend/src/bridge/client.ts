// 类型化桥 client：下行 `window.mornlea.onState` 注入点 + 上行事件 postMessage
// 封装。协议形状以单源 `schema.json` 为权威（JSON Schema 草案 2020-12）；本文件
// 的 TS 类型与守卫函数按 schema 手写钉值，由 schema.test.ts 用 ajv 对同一份
// schema 校验合法/非法夹具来防漂移——生产 bundle 因此不引入 ajv 运行时。
//
// 下行：Go 组装状态 JSON，Rust 经 evaluateJavaScript 调用 `window.mornlea.onState`。
// 上行：`{v:1, events:[...]}` 信封经 `window.webkit.messageHandlers.mornlea.postMessage`
// 交给 WKScriptMessageHandler，由 Rust 排空后交 Go 依序消费。未知相位、未知动作、
// 未知事件类型在此一律抛 `BridgeProtocolError`（对应旧 ABI 的拒绝语义）。

export type Phase = "game" | "menu" | "starting" | "settings" | "paused";

/** 菜单动作 id，与 Go `menuAction*` 常量及 Rust 既有 `UI_ACTION_*` 清单逐值互钉。 */
export type MenuActionId =
  | "enter-game"
  | "multiplayer"
  | "open-settings"
  | "quit"
  | "settings-save"
  | "settings-cancel"
  | "settings-back"
  | "pause-back"
  | "pause-quit-to-menu";

export type WindowSize = "640x360" | "960x540" | "1280x720";

export type SettingsField = "audioVolume" | "texturePackPath" | "windowSize";

export type DebugRowKind = "readout" | "section" | "param";

export type DebugEditOp =
  | "select-next"
  | "select-prev"
  | "enter-edit"
  | "edit-value"
  | "confirm"
  | "cancel"
  | "close";

export interface MenuButton {
  readonly id: MenuActionId;
  readonly label: string;
  readonly enabled: boolean;
}

export interface MenuState {
  readonly title: string;
  readonly version: string;
  readonly error: string;
  readonly buttons: readonly MenuButton[];
}

export interface SettingsValues {
  readonly audioVolume: number;
  readonly texturePackPath: string;
  readonly windowSize: WindowSize;
}

export interface SettingsState {
  readonly draft: SettingsValues;
  readonly saved: SettingsValues;
  readonly dirty: boolean;
  readonly status: string;
  readonly error: string;
}

export interface PauseState {
  readonly remote: boolean;
}

export interface DebugRow {
  readonly label: string;
  readonly value: string;
  readonly kind: DebugRowKind;
  /** 编辑播种文本（全精度），仅可编辑 param 行携带；缺席时以 value 播种。 */
  readonly editValue?: string;
  readonly readonly: boolean;
  readonly selected: boolean;
  readonly editing: boolean;
}

export interface DebugState {
  readonly visible: boolean;
  readonly mode: string;
  readonly rows: readonly DebugRow[];
}

export interface UIState {
  readonly phase: Phase;
  readonly menu?: MenuState;
  readonly settings?: SettingsState;
  readonly pause?: PauseState;
  readonly debug?: DebugState;
}

export interface ActionEvent {
  readonly type: "action";
  readonly id: MenuActionId;
}

/** settings-change 按字段判别值类型，避免 `number | string` 的宽化误用。 */
export type SettingsChangeEvent =
  | { readonly type: "settings-change"; readonly field: "audioVolume"; readonly value: number }
  | { readonly type: "settings-change"; readonly field: "texturePackPath"; readonly value: string }
  | { readonly type: "settings-change"; readonly field: "windowSize"; readonly value: WindowSize };

/** edit-value/confirm 必须携带文本，其余 op 不携带（与 schema 分支互钉）。 */
export type DebugEditEvent =
  | {
      readonly type: "debug-edit";
      readonly op: "select-next" | "select-prev" | "enter-edit" | "cancel" | "close";
    }
  | { readonly type: "debug-edit"; readonly op: "edit-value" | "confirm"; readonly value: string };

export type UplinkEvent = ActionEvent | SettingsChangeEvent | DebugEditEvent;

export interface UplinkEnvelope {
  readonly v: 1;
  readonly events: readonly UplinkEvent[];
}

/** 协议违约异常：未知相位/动作/事件类型、越界值与未知属性都从这里抛出。 */
export class BridgeProtocolError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "BridgeProtocolError";
  }
}

/** 上行事件接收口：与 `Window.webkit.messageHandlers.mornlea` 的 messageHandlers 结构同形。 */
export interface EventSink {
  postMessage(message: unknown): void;
}

/** `window.mornlea` 注入对象：Rust 宿主只依赖这一个入口。 */
export interface MornleaHostApi {
  onState(raw: unknown): void;
}

/** 安装目标的最小结构：jsdom 测试用普通对象，生产传 `window`。 */
export interface MornleaHostTarget {
  mornlea?: MornleaHostApi;
}

declare global {
  interface Window {
    mornlea?: MornleaHostApi;
    webkit?: { messageHandlers: { mornlea?: EventSink } };
  }
}

// 以下常量与 schema.json 的 enum/max 逐值对应；schema 变更时同步修改并跑钉值测试。
const PHASES: readonly Phase[] = ["game", "menu", "starting", "settings", "paused"];
const MENU_ACTIONS: readonly MenuActionId[] = [
  "enter-game",
  "multiplayer",
  "open-settings",
  "quit",
  "settings-save",
  "settings-cancel",
  "settings-back",
  "pause-back",
  "pause-quit-to-menu",
];
const WINDOW_SIZES: readonly WindowSize[] = ["640x360", "960x540", "1280x720"];
const DEBUG_ROW_KINDS: readonly DebugRowKind[] = ["readout", "section", "param"];
const MAX_MENU_BUTTONS = 8;
const MAX_DEBUG_ROWS = 64;
const MAX_EVENTS_PER_BATCH = 64;
const MAX_TITLE = 128;
const MAX_LABEL = 64;
const MAX_VERSION = 64;
const MAX_MESSAGE = 256;
const MAX_PATH = 1024;
const MAX_DEBUG_SIDE = 24;
const MAX_MODE = 64;
/** editValue 播种文本上界，与 schema `debugRow.editValue` maxLength 同值。 */
const MAX_DEBUG_SEED = 64;

type RecordLike = Record<string, unknown>;

function asRecord(value: unknown, context: string): RecordLike {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new BridgeProtocolError(`桥协议 ${context} 必须是对象`);
  }
  return value as RecordLike;
}

/** requireKeys 执行 additionalProperties:false 检查：record 的键必须全部落在
 * allowed∪optional 内，任何未知键都拒绝（可选键是否出现由字段校验决定）。 */
function requireKeys(record: RecordLike, allowed: readonly string[], context: string, optional: readonly string[] = []): void {
  const permitted = [...allowed, ...optional];
  for (const key of Object.keys(record)) {
    if (!permitted.includes(key)) {
      throw new BridgeProtocolError(`桥协议 ${context} 含未知属性「${key}」`);
    }
  }
}

function requireString(record: RecordLike, key: string, max: number, context: string): string {
  const value = record[key];
  if (typeof value !== "string") {
    throw new BridgeProtocolError(`桥协议 ${context}.${key} 必须是字符串`);
  }
  // 长度上界按码点计，与 schema maxLength 同口径；字节精确约束由 Go 组装侧维持。
  if (value.length > max) {
    throw new BridgeProtocolError(`桥协议 ${context}.${key} 超过 ${max} 码点上界`);
  }
  return value;
}

function requireBoolean(record: RecordLike, key: string, context: string): boolean {
  const value = record[key];
  if (typeof value !== "boolean") {
    throw new BridgeProtocolError(`桥协议 ${context}.${key} 必须是布尔`);
  }
  return value;
}

function requireEnum<T extends string>(
  record: RecordLike,
  key: string,
  allowed: readonly T[],
  context: string,
): T {
  const value = record[key];
  if (typeof value !== "string" || !(allowed as readonly string[]).includes(value)) {
    throw new BridgeProtocolError(`桥协议 ${context}.${key} 取值未知：「${String(value)}」`);
  }
  return value as T;
}

function requireArray(record: RecordLike, key: string, context: string): readonly unknown[] {
  const value = record[key];
  if (!Array.isArray(value)) {
    throw new BridgeProtocolError(`桥协议 ${context}.${key} 必须是数组`);
  }
  return value;
}

function requireVolume(record: RecordLike, key: string, context: string): number {
  const value = record[key];
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0 || value > 1) {
    throw new BridgeProtocolError(`桥协议 ${context}.${key} 必须是 [0,1] 内的有限数值`);
  }
  return value;
}

function requireSingleLine(value: string, context: string): string {
  if (/[\r\n]/.test(value)) {
    throw new BridgeProtocolError(`桥协议 ${context} 不允许换行`);
  }
  return value;
}

function parseMenuButton(raw: unknown, context: string): MenuButton {
  const record = asRecord(raw, context);
  requireKeys(record, ["id", "label", "enabled"], context);
  return {
    id: requireEnum(record, "id", MENU_ACTIONS, context),
    label: requireString(record, "label", MAX_LABEL, context),
    enabled: requireBoolean(record, "enabled", context),
  };
}

function parseMenu(record: RecordLike): MenuState {
  requireKeys(record, ["title", "version", "error", "buttons"], "menu");
  const buttons = requireArray(record, "buttons", "menu");
  if (buttons.length > MAX_MENU_BUTTONS) {
    throw new BridgeProtocolError(`桥协议 menu.buttons 超过 ${MAX_MENU_BUTTONS} 个上界`);
  }
  return {
    title: requireString(record, "title", MAX_TITLE, "menu"),
    version: requireString(record, "version", MAX_VERSION, "menu"),
    error: requireString(record, "error", MAX_MESSAGE, "menu"),
    buttons: buttons.map((button, index) => parseMenuButton(button, `menu.buttons[${index}]`)),
  };
}

function parseSettingsValues(raw: unknown, context: string): SettingsValues {
  const record = asRecord(raw, context);
  requireKeys(record, ["audioVolume", "texturePackPath", "windowSize"], context);
  return {
    audioVolume: requireVolume(record, "audioVolume", context),
    texturePackPath: requireSingleLine(
      requireString(record, "texturePackPath", MAX_PATH, context),
      context,
    ),
    windowSize: requireEnum(record, "windowSize", WINDOW_SIZES, context),
  };
}

function parseSettings(record: RecordLike): SettingsState {
  requireKeys(record, ["draft", "saved", "dirty", "status", "error"], "settings");
  return {
    draft: parseSettingsValues(record.draft, "settings.draft"),
    saved: parseSettingsValues(record.saved, "settings.saved"),
    dirty: requireBoolean(record, "dirty", "settings"),
    status: requireString(record, "status", MAX_MESSAGE, "settings"),
    error: requireString(record, "error", MAX_MESSAGE, "settings"),
  };
}

function parseDebugRow(raw: unknown, context: string): DebugRow {
  const record = asRecord(raw, context);
  // editValue 是可选播种键：键集合按「必填 + 可选」一次验收（未知键仍拒绝），
  // 可选键出现时再按字段校验（单行有界字符串）。
  requireKeys(
    record,
    ["label", "value", "kind", "readonly", "selected", "editing"],
    context,
    ["editValue"],
  );
  const row: DebugRow = {
    label: requireString(record, "label", MAX_DEBUG_SIDE, context),
    value: requireString(record, "value", MAX_DEBUG_SIDE, context),
    kind: requireEnum(record, "kind", DEBUG_ROW_KINDS, context),
    readonly: requireBoolean(record, "readonly", context),
    selected: requireBoolean(record, "selected", context),
    editing: requireBoolean(record, "editing", context),
    editValue:
      "editValue" in record
        ? requireSingleLine(requireString(record, "editValue", MAX_DEBUG_SEED, context), context)
        : undefined,
  };
  // 与 Rust flags 语义互钉：EDITING 置位行必须可编辑，选中只落在可编辑行。
  if (row.readonly && (row.selected || row.editing)) {
    throw new BridgeProtocolError(`桥协议 ${context} readonly 行不得置位 selected/editing`);
  }
  // 只读行不可能进入编辑，播种文本只对可编辑行有意义。
  if (row.readonly && row.editValue !== undefined) {
    throw new BridgeProtocolError(`桥协议 ${context} readonly 行不得携带 editValue`);
  }
  return row;
}

function parseDebug(record: RecordLike): DebugState {
  requireKeys(record, ["visible", "mode", "rows"], "debug");
  const rows = requireArray(record, "rows", "debug");
  if (rows.length > MAX_DEBUG_ROWS) {
    throw new BridgeProtocolError(`桥协议 debug.rows 超过 ${MAX_DEBUG_ROWS} 行上界`);
  }
  return {
    visible: requireBoolean(record, "visible", "debug"),
    mode: requireString(record, "mode", MAX_MODE, "debug"),
    rows: rows.map((row, index) => parseDebugRow(row, `debug.rows[${index}]`)),
  };
}

function parsePause(record: RecordLike): PauseState {
  requireKeys(record, ["remote"], "pause");
  return { remote: requireBoolean(record, "remote", "pause") };
}

/** 校验并收口一行下行状态：违约即抛 `BridgeProtocolError`，不产出部分可信对象。 */
export function parseState(raw: unknown): UIState {
  const state = asRecord(raw, "uiState");
  requireKeys(state, ["phase", "menu", "settings", "pause", "debug"], "uiState");
  return {
    phase: requireEnum(state, "phase", PHASES, "uiState"),
    menu: state.menu === undefined ? undefined : parseMenu(asRecord(state.menu, "menu")),
    settings:
      state.settings === undefined ? undefined : parseSettings(asRecord(state.settings, "settings")),
    pause: state.pause === undefined ? undefined : parsePause(asRecord(state.pause, "pause")),
    debug: state.debug === undefined ? undefined : parseDebug(asRecord(state.debug, "debug")),
  };
}

/** 把上行事件包成版本化信封；空批与超上界批是调用方编程错误。 */
export function createEnvelope(events: readonly UplinkEvent[]): UplinkEnvelope {
  if (events.length === 0) {
    throw new BridgeProtocolError("上行事件批为空，不构成可发送信封");
  }
  if (events.length > MAX_EVENTS_PER_BATCH) {
    throw new BridgeProtocolError(`上行事件批超过 ${MAX_EVENTS_PER_BATCH} 条上界`);
  }
  return { v: 1, events: [...events] };
}

/**
 * 桥 client：把「校验后的下行状态」分发给订阅者，把上行事件装信封投给 sink。
 * 订阅即重放最新状态，保证晚挂载的 React 树不漏掉装配前的首帧状态。
 */
export class BridgeClient {
  private latestState: UIState | null = null;
  private readonly listeners = new Set<(state: UIState) => void>();

  constructor(private readonly sink: EventSink | null) {}

  get latest(): UIState | null {
    return this.latestState;
  }

  /** 宿主注入点 `window.mornlea.onState` 的实际落点。 */
  handleState(raw: unknown): void {
    const state = parseState(raw);
    this.latestState = state;
    // 拷贝后遍历：订阅者在回调里退订不得扰动本次分发。
    for (const listener of [...this.listeners]) {
      listener(state);
    }
  }

  subscribe(listener: (state: UIState) => void): () => void {
    this.listeners.add(listener);
    const latest = this.latestState;
    if (latest !== null) {
      listener(latest);
    }
    return () => {
      this.listeners.delete(listener);
    };
  }

  /** 单事件即批发出；sink 缺席（非 WebView 环境）时安全空操作。 */
  send(event: UplinkEvent): void {
    this.sink?.postMessage(createEnvelope([event]));
  }
}

/** 解析宿主提供的上行出口；非 WebView 环境（测试、纯浏览器预览）返回 null。 */
function resolveDefaultSink(): EventSink | null {
  if (typeof window === "undefined") {
    return null;
  }
  return window.webkit?.messageHandlers.mornlea ?? null;
}

/** 把 onState 注入目标（生产为 `window`），Rust 宿主据此调用。 */
export function installMornleaGlobal(bridge: BridgeClient, target: MornleaHostTarget = window): void {
  target.mornlea = {
    onState: (raw: unknown) => {
      bridge.handleState(raw);
    },
  };
}

/** 应用级单例：sink 在模块装配时解析一次。 */
export const bridge: BridgeClient = new BridgeClient(resolveDefaultSink());
