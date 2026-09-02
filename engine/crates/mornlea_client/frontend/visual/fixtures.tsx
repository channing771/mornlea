// 视觉基线 fixture 注册表：每个 UI 部件一个命名呈现，由 harness 入口按
// `?fixture=<name>` 选择渲染。取舍：
//   - 四整屏直接以 fixture props 渲染生产面板组件（四面板的 props 面干净，
//     不与桥耦合），上行事件接 noEvent 空收集器——呈现与生产行为逐项一致，
//     桥协议零参与，不需要 App/桥桩。
//   - 组件态 fixture 只复用 ui.css 的既有语义 class（menu-button、settings-*、
//     debug-*），不引入 Tailwind 工具类：tailwind content 只扫描 src/** 与
//     retroui 产物，harness 侧新增的工具类不会被生成。
//   - 名称清单在 fixture-names.ts 单源（本文件经 `as const` 推导字面量联合，
//     visual.mjs 按严格格式提取同一数组），注册表键以
//     `Record<FixtureName, ReactElement>` 钉死：注册表多键/缺键都在编译期
//     报错，模块加载时另与清单运行期互钉兜底（防 .mjs 提取漂移）。
import type { ReactElement } from "react";
import type {
  DebugState,
  HudSlot,
  HudState,
  LoadingState,
  MenuState,
  PauseState,
  SettingsState,
  UplinkEvent,
} from "../src/bridge/client";
import { HudRoot } from "../src/hud/HudRoot";
import { ProgressTrack } from "../src/hud/ProgressTrack";
import { DebugPanel } from "../src/ui/DebugPanel";
import { LoadingScreen } from "../src/ui/LoadingScreen";
import { MainMenu } from "../src/ui/MainMenu";
import { PauseMenu } from "../src/ui/PauseMenu";
import { SettingsPanel } from "../src/ui/SettingsPanel";
import { PixelButton, PixelInput } from "../src/ui/pixel";
import { fixtureNames, type FixtureName } from "./fixture-names";

// noEvent：fixture 渲染的空上行收集器。回调签名与生产一致；截图场景无头、
// 无交互，回调不应被触发，留作防御性兜底。
const noEvent = (_event: UplinkEvent): void => {};

// 状态夹具取值与 Go 下行组装及 App.test.tsx 的形态对齐（标题、按钮表、设置
// 三字段、调试行），呈现面即生产真实值，不造演示数据。
const menuFixture: MenuState = {
  title: "Mornlea",
  version: "dev",
  error: "",
  buttons: [
    { id: "enter-game", label: "进入游戏", enabled: true },
    { id: "multiplayer", label: "多人游戏", enabled: false },
    { id: "open-settings", label: "设置", enabled: true },
    { id: "quit", label: "退出游戏", enabled: true },
  ],
};

const settingsFixture: SettingsState = {
  draft: { audioVolume: 0.25, texturePackPath: "packs/local", windowSize: "960x540" },
  saved: { audioVolume: 0.25, texturePackPath: "packs/local", windowSize: "960x540" },
  dirty: false,
  status: "",
  error: "",
};

const pauseFixture: PauseState = { remote: false };

// panel-loading：世界加载屏取中间进度（默认视距 4489 列的 1/4 档），标题、
// 进度轨道与计数行随生产 LoadingScreen 一并入基线；屏自身即全屏不透明遮罩，
// 无需舞台包裹。
const loadingFixture: LoadingState = { loaded: 1122, total: 4489, meshed: 26934, meshTotal: 107736 };

// panel-debug 用完整行集合（读数/段头/参数/选中/只读），debug-rows 只保留
// 规格点名的四种行态各一行，两张基线互不冗余。
const debugPanelFixture: DebugState = {
  visible: true,
  mode: "本地单机",
  rows: [
    { label: "fps", value: "60", kind: "readout", readonly: true, selected: false, editing: false },
    { label: "pos", value: "8, 64, -3", kind: "readout", readonly: true, selected: false, editing: false },
    { label: "── sim ──", value: "", kind: "section", readonly: true, selected: false, editing: false },
    { label: "gravity", value: "9.8", kind: "param", readonly: false, selected: true, editing: false },
    { label: "timescale", value: "1.0", kind: "param", readonly: false, selected: false, editing: false },
    { label: "seed", value: "1234567", kind: "param", readonly: true, selected: false, editing: false },
  ],
};

const debugRowsFixture: DebugState = {
  visible: true,
  mode: "本地单机",
  rows: [
    { label: "fps", value: "60", kind: "readout", readonly: true, selected: false, editing: false },
    { label: "── sim ──", value: "", kind: "section", readonly: true, selected: false, editing: false },
    { label: "gravity", value: "9.8", kind: "param", readonly: false, selected: false, editing: false },
    { label: "timescale", value: "1.0", kind: "param", readonly: false, selected: true, editing: false },
  ],
};

// stage 把组件态 fixture 放进舞台：列宽与设置面板同款（见 visual.css
// `.visual-column` 的交叉引用注释），控件在生产语境下的几何（全宽输入/滑杆、
// 按钮组 flex）不因 harness 变形。
function stage(child: ReactElement): ReactElement {
  return (
    <div className="visual-stage">
      <div className="visual-column">{child}</div>
    </div>
  );
}

// ---- 游戏 HUD 部件（`hud-` 前缀）：真实组件 + 合成 HudState 夹具 ----

// 视口夹具：取实拍窗口尺寸（1280x720），单一比例 `--hud-scale` 因此落在 1，
// 部件按 design 基准原尺寸呈现。
const hudViewport = { width: 1280, height: 720 };

// hudState 补齐恒携带字段（viewport 与进食进度条的未激活缺省），各 fixture
// 只写关心分节；进食进度条仍可按 fixture 覆写（如状态栈内的进食轨道）。
function hudState(overrides: Partial<Omit<HudState, "viewport">>): HudState {
  return {
    viewport: hudViewport,
    eating: { active: false, progress: 0 },
    ...overrides,
  };
}

// hudStage 把 HUD 部件放进模拟世界底色的舞台：生产 HUD 的根是全屏透明层、
// 直接叠在 wgpu 世界画面上，截图没有世界画面，这里以既有暖深棕面板投影令牌
// 充当背景，暖白前景文字与暗色贴条才可辨（呈现面仍是生产组件与令牌）。
function hudStage(child: ReactElement): ReactElement {
  return <div className="visual-hud-stage">{child}</div>;
}

// hud-hotbar 的九格镜像：覆盖选中格双层轮廓、两位数量、单件不显示数量、
// 耐久三档（部分磨损/满耐久无条/跌破四分之一的低耐久档）与空格。
const hudHotbarSlots: readonly HudSlot[] = [
  { item: 1, count: 1 },
  { item: 2, count: 64 },
  { item: 7, count: 12, durability: 0.6 },
  { item: 0, count: 0 },
  { item: 4, count: 3 },
  { item: 9, count: 1, durability: 1 },
  { item: 11, count: 1, durability: 0.15 },
  { item: 0, count: 0 },
  { item: 5, count: 2 },
];

// 状态栈/弹条/聊天/容器各 fixture 共用的九格镜像：空格与少量物品混合，
// 状态行两缘按这份镜像的内容行宽对齐。
const hudInventorySlots: readonly HudSlot[] = [
  { item: 1, count: 1 },
  { item: 0, count: 0 },
  { item: 3, count: 24 },
  { item: 0, count: 0 },
  { item: 6, count: 1, durability: 0.8 },
  { item: 0, count: 0 },
  { item: 12, count: 5 },
  { item: 0, count: 0 },
  { item: 2, count: 1 },
];

// hud-hotbar：只携带快捷栏镜像，生命/饥饿/氧气缺席顺带见证「镜像未确认
// 不产生状态格」。
const hudHotbarFixture: HudState = hudState({
  hotbar: { slots: hudHotbarSlots, selectedIndex: 2 },
});

// hud-status：完整状态栈——生命 7（三满一半）、饥饿 5（奇数，末格露右半）、
// 耗损氧气 90（三个满气泡沿饥饿外缘堆叠）、进食轨道居状态栈上方；饥饿行携带
// `saturationZero`，抖动态（下移 1 design px）随之入基线见证。
const hudStatusFixture: HudState = hudState({
  hotbar: { slots: hudInventorySlots, selectedIndex: 0 },
  health: { value: 7 },
  hunger: { value: 5, saturationZero: true },
  oxygen: { value: 90 },
  eating: { active: true, progress: 0.45 },
});

// hud-popup-crosshair：物品名弹条 + 十字准星 + 权威命中 marker 同帧共存。
const hudPopupCrosshairFixture: HudState = hudState({
  hotbar: { slots: hudInventorySlots, selectedIndex: 0 },
  health: { value: 7 },
  hunger: { value: 5, saturationZero: false },
  oxygen: { value: 90 },
  popup: { text: "橡木原木" },
  crosshair: true,
  marker: true,
});

// hud-chat：多行聊天（含一个空行占位），锚在视口左缘、状态栈上沿之上。
const hudChatFixture: HudState = hudState({
  hotbar: { slots: hudInventorySlots, selectedIndex: 0 },
  health: { value: 7 },
  hunger: { value: 5, saturationZero: false },
  oxygen: { value: 90 },
  chat: { lines: ["你加入了世界", "矿洞入口在东南方向", "", "小心夜晚的敌对生物"] },
});

// hud-container-open：容器打开态翻转构图——两条状态行翻到快捷栏下方、贴条
// 让出像素保留构图空间，弹条被抑制（夹具仍携带弹条以见证抑制）。
const hudContainerOpenFixture: HudState = hudState({
  hotbar: { slots: hudInventorySlots, selectedIndex: 0 },
  health: { value: 4 },
  hunger: { value: 5, saturationZero: false },
  oxygen: { value: 90 },
  popup: { text: "橡木原木" },
  containerOpen: true,
});

// 注册表：键集合被 `Record<FixtureName, ReactElement>` 与 fixture-names.ts 的
// 字面量联合双向钉死（新增部件先加清单条目，否则这里编译不过），随后跑
// update 入口把基线随部件一并入库。
const registry: Record<FixtureName, ReactElement> = {
  "panel-main-menu": <MainMenu menu={menuFixture} onEvent={noEvent} />,
  "panel-settings": <SettingsPanel settings={settingsFixture} onEvent={noEvent} />,
  "panel-pause": <PauseMenu pause={pauseFixture} onEvent={noEvent} />,
  "panel-loading": <LoadingScreen loading={loadingFixture} />,
  "panel-debug": <DebugPanel debug={debugPanelFixture} onEvent={noEvent} />,
  "button-default": stage(
    <PixelButton type="button" className="menu-button">进入游戏</PixelButton>,
  ),
  "button-disabled": stage(
    <PixelButton type="button" className="menu-button" disabled>多人游戏</PixelButton>,
  ),
  // aria-pressed 的琥珀选中面在生产中只出现在窗口预设按钮上，这里沿用同一
  // class 保证选中态呈现与生产逐像素同源。
  "button-pressed": stage(
    <fieldset className="settings-window">
      <PixelButton type="button" className="settings-window-button" aria-pressed={true}>
        960 × 540
      </PixelButton>
    </fieldset>,
  ),
  "input-text": stage(
    <PixelInput
      type="text"
      className="settings-text-input"
      value="packs/local"
      readOnly
      aria-label="材质包目录"
    />,
  ),
  "preset-group": stage(
    <fieldset className="settings-window">
      <legend>窗口大小</legend>
      <PixelButton type="button" className="settings-window-button" aria-pressed={false}>
        640 × 360
      </PixelButton>
      <PixelButton type="button" className="settings-window-button" aria-pressed={true}>
        960 × 540
      </PixelButton>
      <PixelButton type="button" className="settings-window-button" aria-pressed={false}>
        1280 × 720
      </PixelButton>
    </fieldset>,
  ),
  "slider": stage(
    <input
      type="range"
      className="settings-slider"
      min={0}
      max={100}
      step={1}
      value={25}
      readOnly
      aria-label="总音量"
    />,
  ),
  "debug-rows": <DebugPanel debug={debugRowsFixture} onEvent={noEvent} />,
  "error-line": stage(<p className="menu-error" role="alert">世界装配失败</p>),
  // 游戏 HUD 部件：合成 HudState 夹具驱动真实 HudRoot（呈现面与生产一致）。
  "hud-hotbar": hudStage(<HudRoot hud={hudHotbarFixture} />),
  "hud-status": hudStage(<HudRoot hud={hudStatusFixture} />),
  // 进食条是唯一的屏幕进度语义（采掘条退役），单条轨道以生产 ProgressTrack
  // 组件按常用填充比例入基线（轨道几何与生产逐项同源）。
  "hud-progress": hudStage(
    <div className="visual-hud-progress-row">
      <ProgressTrack kind="eating" progress={0.62} />
    </div>,
  ),
  "hud-popup-crosshair": hudStage(<HudRoot hud={hudPopupCrosshairFixture} />),
  "hud-chat": hudStage(<HudRoot hud={hudChatFixture} />),
  "hud-container-open": hudStage(<HudRoot hud={hudContainerOpenFixture} />),
};

// 注册表与清单运行期互钉兜底（编译期已由 Record<FixtureName, …> 钉死注册表
// 侧；这段防的是 visual.mjs 对 fixture-names.ts 的严格格式提取漂移）。
const registryKeys = Object.keys(registry).sort();
const manifestKeys = [...fixtureNames].sort();
if (
  registryKeys.length !== manifestKeys.length ||
  registryKeys.some((key, index) => key !== manifestKeys[index])
) {
  throw new Error(
    `fixture 注册表与 fixture-names.ts 不一致：registry=[${registryKeys.join(", ")}] manifest=[${manifestKeys.join(", ")}]`,
  );
}

// isFixtureName 收窄 URL 查询参数到清单内的合法名称。
function isFixtureName(value: string): value is FixtureName {
  return value in registry;
}

// resolveFixture 按名取呈现；未知名称直接抛错（截图会是空白页，宁可构建期
// 失败也不入库错误基线）。
export function resolveFixture(name: string | null): ReactElement {
  const element = name !== null && isFixtureName(name) ? registry[name] : undefined;
  if (element === undefined) {
    throw new Error(
      `未知 fixture：${name ?? "(URL 缺 ?fixture= 参数)"}，可用清单见 visual/fixture-names.ts`,
    );
  }
  return element;
}
