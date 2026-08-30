// 视觉基线 fixture 注册表：每个 UI 部件一个命名呈现，由 harness 入口按
// `?fixture=<name>` 选择渲染。取舍：
//   - 四整屏直接以 fixture props 渲染生产面板组件（四面板的 props 面干净，
//     不与桥耦合），上行事件接 noEvent 空收集器——呈现与生产行为逐项一致，
//     桥协议零参与，不需要 App/桥桩。
//   - 组件态 fixture 只复用 ui.css 的既有语义 class（menu-button、settings-*、
//     debug-*），不引入 Tailwind 工具类：tailwind content 只扫描 src/** 与
//     retroui 产物，harness 侧新增的工具类不会被生成。
//   - 名称清单在 fixture-names.json 单源（本文件与 visual.mjs 共读），注册表
//     与清单在模块加载时互钉，漂移立即抛错，避免部件静默缺基线。
import type { ReactElement } from "react";
import type {
  DebugState,
  MenuState,
  PauseState,
  SettingsState,
  UplinkEvent,
} from "../src/bridge/client";
import { DebugPanel } from "../src/ui/DebugPanel";
import { MainMenu } from "../src/ui/MainMenu";
import { PauseMenu } from "../src/ui/PauseMenu";
import { SettingsPanel } from "../src/ui/SettingsPanel";
import { PixelButton, PixelInput } from "../src/ui/pixel";
import fixtureNames from "./fixture-names.json";

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

// stage 把组件态 fixture 放进舞台：列宽与设置面板同款（min(460px, ...)），
// 控件在生产语境下的几何（全宽输入/滑杆、按钮组 flex）不因 harness 变形。
function stage(child: ReactElement): ReactElement {
  return (
    <div className="visual-stage">
      <div className="visual-column">{child}</div>
    </div>
  );
}

// 注册表：键集合与 fixture-names.json 完全一致（模块加载即互钉）。新增部件
// 先加清单条目再补注册，随后跑 update 入口把基线随部件一并入库。
const registry: Record<string, ReactElement> = {
  "panel-main-menu": <MainMenu menu={menuFixture} onEvent={noEvent} />,
  "panel-settings": <SettingsPanel settings={settingsFixture} onEvent={noEvent} />,
  "panel-pause": <PauseMenu pause={pauseFixture} onEvent={noEvent} />,
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
};

// 注册表与清单互钉：任一侧多/少条目都在模块加载时报错，而不是等到 check
// 才表现为「缺基线」或「静默漏拍」。
const registryKeys = Object.keys(registry).sort();
const manifestKeys = [...fixtureNames].sort();
if (
  registryKeys.length !== manifestKeys.length ||
  registryKeys.some((key, index) => key !== manifestKeys[index])
) {
  throw new Error(
    `fixture 注册表与 fixture-names.json 不一致：registry=[${registryKeys.join(", ")}] manifest=[${manifestKeys.join(", ")}]`,
  );
}

// resolveFixture 按名取呈现；未知名称直接抛错（截图会是空白页，宁可构建期
// 失败也不入库错误基线）。
export function resolveFixture(name: string | null): ReactElement {
  const element = name === null ? undefined : registry[name];
  if (element === undefined) {
    throw new Error(
      `未知 fixture：${name ?? "(URL 缺 ?fixture= 参数)"}，可用清单见 visual/fixture-names.json`,
    );
  }
  return element;
}
