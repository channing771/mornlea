import { parseGame, validateGameAction, type GameState, type GameAction } from "./game";
// 类型化桥 client：下行 `window.mornlea.onState` 注入点 + 上行事件 postMessage
// 封装。协议形状以单源 `schema.json` 为权威（JSON Schema 草案 2020-12）；本文件
// 的 TS 类型与守卫函数按 schema 手写钉值，由 schema.test.ts 用 ajv 对同一份
// schema 校验合法/非法夹具来防漂移——生产 bundle 因此不引入 ajv 运行时。
//
// 下行：Go 组装状态 JSON，Rust 经 evaluateJavaScript 调用 `window.mornlea.onState`。
// 上行：`{v:1, events:[...]}` 信封经 `window.webkit.messageHandlers.mornlea.postMessage`
// 交给 WKScriptMessageHandler，由 Rust 排空后交 Go 依序消费。未知相位、未知动作、
// 未知事件类型在此一律抛 `BridgeProtocolError`（对应旧 ABI 的拒绝语义）。

export type Phase = "game" | "menu" | "starting" | "loading" | "settings" | "paused";

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

/** 世界加载屏分节：loaded 是已就绪区块列数，total 是与无头加载判据同源的
 * 目标列数；语义权威在 Go，前端只做比例换算与格式化。 */
export interface LoadingState {
  readonly loaded: number;
  readonly total: number;
  /** 初始网格化的单调完成估计；mesher 装配且完成计数开始后才携带。 */
  readonly meshed?: number;
  /** 网格化目标段数（目标列数 × 每区块段数）。 */
  readonly meshTotal?: number;
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

/** CSS 逻辑像素尺寸（窗口 `ContentSize`）：单一比例缩放变量 `--hud-scale` 的唯一窗口输入。 */
export interface HudViewport {
  readonly width: number;
  readonly height: number;
}

/** 快捷栏单格。空格以 item=0、count=0 表达；durability 只对部分磨损工具携带。 */
export interface HudSlot {
  readonly item: number;
  readonly count: number;
  readonly durability?: number;
  readonly name?: string;
  readonly icon?: string;
}

/** 快捷栏镜像：恰九格，格序即栏位序。 */
export interface HudHotbar {
  readonly slots: readonly HudSlot[];
  readonly selectedIndex: number;
}

export interface HudHealth {
  readonly value: number;
}

export interface HudHunger {
  readonly value: number;
  readonly saturationZero: boolean;
}

export interface HudOxygen {
  readonly value: number;
}

/** 进食进度：恒携带，active 表达本 tick 是否在进食；progress 是已钳制的填充比例。
 * 采掘进度条已退役，进食条是唯一的屏幕进度语义。 */
export interface HudEating {
  readonly active: boolean;
  readonly progress: number;
}

/** 物品名弹条：presence 即可见性，40 tick 窗口计时留在 Go 侧。 */
export interface HudPopup {
  readonly text: string;
}

/** 最近聊天行缓冲：行序即呈现序，空串是合法行且占用一个行槽。 */
export interface HudChat {
  readonly lines: readonly string[];
}

/**
 * 游戏相位常显 HUD 分节。viewport 与进食进度条恒携带，进度条以 active 表达
 * 是否呈现（采掘进度不再占用屏幕进度条，其反馈由世界空间裂纹承载）；可选
 * 分节缺席即「权威镜像尚未确认」或「呈现态不在结果窗口内」，组件据此隐藏。
 * 全部字段都是语义值（数值/比例/标志），不携带任何坐标矩形——布局由 CSS
 * 组件按 design 基准与 viewport 推导。
 */
export interface HudState {
  readonly viewport: HudViewport;
  readonly hotbar?: HudHotbar;
  readonly health?: HudHealth;
  readonly hunger?: HudHunger;
  readonly oxygen?: HudOxygen;
  readonly eating: HudEating;
  readonly popup?: HudPopup;
  readonly chat?: HudChat;
  readonly marker?: boolean;
  readonly crosshair?: boolean;
  readonly containerOpen?: boolean;
}

export interface UIState {
  readonly phase: Phase;
  readonly menu?: MenuState;
  readonly settings?: SettingsState;
  readonly pause?: PauseState;
  readonly loading?: LoadingState;
  readonly debug?: DebugState;
  readonly hud?: HudState;
 readonly game?: GameState;
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

export type UplinkEvent = ActionEvent | SettingsChangeEvent | DebugEditEvent | GameAction;

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
const PHASES: readonly Phase[] = ["game", "menu", "starting", "loading", "settings", "paused"];
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
// 以下为游戏相位 hud 分节的钉值：与 Go 侧镜像域（core.ItemID/MaxStackCount/
// MaxHealth/MaxHunger/MaxOxygenTicks、HotbarSlots、maxChatLines/maxChatRunes、
// maxPopupRunes）及 schema 的 integer/maxLength 上界逐值同源。
const HOTBAR_SLOTS = 9;
const SELECTED_INDEX_MAX = HOTBAR_SLOTS - 1;
const MAX_ITEM_ID = 65535;
const MAX_STACK_COUNT = 64;
const MAX_HEALTH = 20;
const MAX_HUNGER = 20;
const MAX_OXYGEN_TICKS = 300;
const MAX_CHAT_LINES = 6;
const MAX_CHAT_RUNES = 32;
const MAX_POPUP_RUNES = 32;
/** framebuffer 单边像素上界，与 Go 侧 uint32 域同界。 */
const MAX_VIEWPORT_SIDE = 4294967295;
/** loading 分节两整数的安全整数上界：schema 对 loaded/total 只钉下界
 * （0/1），上界以 `Number.MAX_SAFE_INTEGER` 表达「JSON 安全整数」语义，
 * 与 schema 的开放上界同口径，不另造数值契约。 */
const MAX_LOADING_COUNT = Number.MAX_SAFE_INTEGER;

type RecordLike = Record<string, unknown>;

/** 解析期中间形态：下行分节的字段按需出现，逐键校验后收口为只读契约。 */
type Writable<T> = { -readonly [K in keyof T]: T[K] };

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

/** 值形态的有界字符串读取：record 字段与数组元素（聊天行）共用同一码点上界。 */
function requireBoundedString(value: unknown, max: number, context: string): string {
  if (typeof value !== "string") {
    throw new BridgeProtocolError(`桥协议 ${context} 必须是字符串`);
  }
  // 长度上界按码点计，与 schema maxLength 同口径；字节精确约束由 Go 组装侧维持。
  if (value.length > max) {
    throw new BridgeProtocolError(`桥协议 ${context} 超过 ${max} 码点上界`);
  }
  return value;
}

function requireString(record: RecordLike, key: string, max: number, context: string): string {
  return requireBoundedString(record[key], max, `${context}.${key}`);
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

/** 整数值读取：hud 分节的物品编号、数量、生存数值与视口尺寸都按整数钉值，
 * 非整数（12.5）与越界一样拒绝。 */
function requireInteger(
  record: RecordLike,
  key: string,
  min: number,
  max: number,
  context: string,
): number {
  const value = record[key];
  if (typeof value !== "number" || !Number.isInteger(value)) {
    throw new BridgeProtocolError(`桥协议 ${context}.${key} 必须是整数`);
  }
  if (value < min || value > max) {
    throw new BridgeProtocolError(`桥协议 ${context}.${key} 超出 ${min}..${max} 区间`);
  }
  return value;
}

/** 0..1 比例读取：进度与耐久比例共用；NaN/Infinity 一并拒绝。 */
function requireRatio(record: RecordLike, key: string, context: string): number {
  const value = record[key];
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0 || value > 1) {
    throw new BridgeProtocolError(`桥协议 ${context}.${key} 必须是 [0,1] 内的有限比例`);
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

/** loading 分节守卫：loaded/total 都是整数且只钉下界（loaded>=0、total>=1），
 * 与 schema `$defs/loadingState` 同口径；分节缺席与否由 `parseState` 按
 * presence 收口，这里不做跨相位字段约束（schema 未定义，守卫不发明）。 */
/** loading 分节守卫：meshed/meshTotal 是可选键，出现时才按字段校验。 */
function parseLoading(record: RecordLike): LoadingState {
  requireKeys(record, ["loaded", "total"], "loading", ["meshed", "meshTotal"]);
  const loading: Writable<LoadingState> = {
    loaded: requireInteger(record, "loaded", 0, MAX_LOADING_COUNT, "loading"),
    total: requireInteger(record, "total", 1, MAX_LOADING_COUNT, "loading"),
  };
  if ("meshed" in record) {
    loading.meshed = requireInteger(record, "meshed", 0, MAX_LOADING_COUNT, "loading");
  }
  if ("meshTotal" in record) {
    loading.meshTotal = requireInteger(record, "meshTotal", 1, MAX_LOADING_COUNT, "loading");
  }
  return loading;
}

function parseHudViewport(raw: unknown, context: string): HudViewport {
  const record = asRecord(raw, context);
  requireKeys(record, ["width", "height"], context);
  return {
    width: requireInteger(record, "width", 0, MAX_VIEWPORT_SIDE, context),
    height: requireInteger(record, "height", 0, MAX_VIEWPORT_SIDE, context),
  };
}

export function parseHudSlot(raw: unknown, context: string): HudSlot {
  const record = asRecord(raw, context);
  // durability 是可选键：部分磨损工具才携带；未知键仍按 additionalProperties 拒绝。
  requireKeys(record, ["item", "count"], context, ["durability", "name", "icon"]);
  const slot: Writable<HudSlot> = {
    item: requireInteger(record, "item", 0, MAX_ITEM_ID, context),
    count: requireInteger(record, "count", 0, MAX_STACK_COUNT, context),
  };
  if ("durability" in record) {
    slot.durability = requireRatio(record, "durability", context);
  }
  if ("name" in record) slot.name = requireString(record,"name",64,context);
  if ("icon" in record) {
    const icon = requireString(record,"icon",65536,context);
    if (!/^data:image\/png;base64,[A-Za-z0-9+/]+={0,2}$/.test(icon)) throw new BridgeProtocolError("非法物品图像");
    slot.icon = icon;
  }
  return slot;
}

function parseHudHotbar(raw: unknown, context: string): HudHotbar {
  const record = asRecord(raw, context);
  requireKeys(record, ["slots", "selectedIndex"], context);
  const slots = requireArray(record, "slots", context);
  if (slots.length !== HOTBAR_SLOTS) {
    throw new BridgeProtocolError(`桥协议 ${context}.slots 必须恰为 ${HOTBAR_SLOTS} 格`);
  }
  return {
    slots: slots.map((slot, index) => parseHudSlot(slot, `${context}.slots[${index}]`)),
    selectedIndex: requireInteger(record, "selectedIndex", 0, SELECTED_INDEX_MAX, context),
  };
}

/** 进食进度：progress 恒携带，active 只表达是否呈现。 */
function parseHudEating(raw: unknown, context: string): HudEating {
  const record = asRecord(raw, context);
  requireKeys(record, ["active", "progress"], context);
  return {
    active: requireBoolean(record, "active", context),
    progress: requireRatio(record, "progress", context),
  };
}

/** hud 分节守卫：viewport 与进食进度条恒携带，其余分节按 presence 表达
 * 「镜像未确认」或「不在呈现窗口内」，缺席字段收口为 undefined 供组件隐藏。
 * 已退役的 mining 分节属未知属性，照 additionalProperties:false 语义拒绝。 */
function parseHud(record: RecordLike): HudState {
  requireKeys(
    record,
    ["viewport", "eating"],
    "hud",
    [
      "hotbar",
      "health",
      "hunger",
      "oxygen",
      "popup",
      "chat",
      "marker",
      "crosshair",
      "containerOpen",
    ],
  );
  const hud: Writable<HudState> = {
    viewport: parseHudViewport(record.viewport, "hud.viewport"),
    eating: parseHudEating(record.eating, "hud.eating"),
  };
  if ("hotbar" in record) {
    hud.hotbar = parseHudHotbar(asRecord(record.hotbar, "hud.hotbar"), "hud.hotbar");
  }
  if ("health" in record) {
    const health = asRecord(record.health, "hud.health");
    requireKeys(health, ["value"], "hud.health");
    hud.health = { value: requireInteger(health, "value", 0, MAX_HEALTH, "hud.health") };
  }
  if ("hunger" in record) {
    const hunger = asRecord(record.hunger, "hud.hunger");
    requireKeys(hunger, ["value", "saturationZero"], "hud.hunger");
    hud.hunger = {
      value: requireInteger(hunger, "value", 0, MAX_HUNGER, "hud.hunger"),
      saturationZero: requireBoolean(hunger, "saturationZero", "hud.hunger"),
    };
  }
  if ("oxygen" in record) {
    const oxygen = asRecord(record.oxygen, "hud.oxygen");
    requireKeys(oxygen, ["value"], "hud.oxygen");
    hud.oxygen = { value: requireInteger(oxygen, "value", 0, MAX_OXYGEN_TICKS, "hud.oxygen") };
  }
  if ("popup" in record) {
    const popup = asRecord(record.popup, "hud.popup");
    requireKeys(popup, ["text"], "hud.popup");
    hud.popup = { text: requireString(popup, "text", MAX_POPUP_RUNES, "hud.popup") };
  }
  if ("chat" in record) {
    const chat = asRecord(record.chat, "hud.chat");
    requireKeys(chat, ["lines"], "hud.chat");
    const lines = requireArray(chat, "lines", "hud.chat");
    if (lines.length > MAX_CHAT_LINES) {
      throw new BridgeProtocolError(`桥协议 hud.chat.lines 超过 ${MAX_CHAT_LINES} 行上界`);
    }
    hud.chat = {
      lines: lines.map((line, index) =>
        requireBoundedString(line, MAX_CHAT_RUNES, `hud.chat.lines[${index}]`),
      ),
    };
  }
  if ("marker" in record) {
    hud.marker = requireBoolean(record, "marker", "hud");
  }
  if ("crosshair" in record) {
    hud.crosshair = requireBoolean(record, "crosshair", "hud");
  }
  if ("containerOpen" in record) {
    hud.containerOpen = requireBoolean(record, "containerOpen", "hud");
  }
  return hud;
}

/** parseState 收口一行下行状态：违约即抛 `BridgeProtocolError`，不产出部分可信对象。 */
export function parseState(raw: unknown): UIState {
  const state = asRecord(raw, "uiState");
  requireKeys(state, ["phase", "menu", "settings", "pause", "loading", "debug", "hud", "game"], "uiState");
  return {
    phase: requireEnum(state, "phase", PHASES, "uiState"),
    menu: state.menu === undefined ? undefined : parseMenu(asRecord(state.menu, "menu")),
    settings:
      state.settings === undefined ? undefined : parseSettings(asRecord(state.settings, "settings")),
    pause: state.pause === undefined ? undefined : parsePause(asRecord(state.pause, "pause")),
    loading:
      state.loading === undefined ? undefined : parseLoading(asRecord(state.loading, "loading")),
    debug: state.debug === undefined ? undefined : parseDebug(asRecord(state.debug, "debug")),
    game: state.game === undefined ? undefined : parseGame(state.game),
    hud: state.hud === undefined ? undefined : parseHud(asRecord(state.hud, "hud")),
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
  for (const event of events) { if (event.type === "game-action") validateGameAction(event); }
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
