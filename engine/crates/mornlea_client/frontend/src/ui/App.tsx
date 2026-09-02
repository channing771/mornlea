// App：按下行 phase 切换五屏（menu/starting → 主菜单、loading → 加载屏、
// settings → 设置页、paused → 暂停层、game → 零 chrome），F3 调试面板按
// debug.visible 叠加于任意相位。前端不持任何菜单语义：相位机、草稿与装配
// 裁决都在 Go。
//
// 键盘路由（桥重组）：菜单相位下 WebView 是 firstResponder，winit 收不到
// 按键，全部面板键在此集中上行。Esc 优先级栈（调试面板 → 暂停层 → 设置页）
// 由这里的分支顺序体现：调试面板可见时 Esc/F3 归面板，不再下发到相位层；
// 游戏相位（WebView 隐藏）不注册任何路由，键盘仍由 winit 独占。
import { useEffect, useState } from "react";
import {
  bridge as defaultBridge,
  type BridgeClient,
  type UIState,
  type UplinkEvent,
} from "../bridge/client";
import { HudRoot } from "../hud/HudRoot";
import { DebugPanel } from "./DebugPanel";
import { LoadingScreen } from "./LoadingScreen";
import { MainMenu } from "./MainMenu";
import { PauseMenu } from "./PauseMenu";
import { SettingsPanel } from "./SettingsPanel";

export interface AppProps {
  /** 缺省用模块级单例；测试注入隔离实例。 */
  bridge?: BridgeClient;
  /** 缺省经桥 client 装信封发送；测试注入 spy。 */
  onEvent?: (event: UplinkEvent) => void;
}

export function App({ bridge = defaultBridge, onEvent }: AppProps = {}) {
  const [state, setState] = useState<UIState | null>(() => bridge.latest);
  useEffect(() => bridge.subscribe(setState), [bridge]);
  const emit = onEvent ?? ((event: UplinkEvent) => bridge.send(event));

  useEffect(() => {
    if (state === null) {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent): void => {
      routeKeyDown(event, state, emit);
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [state, emit]);

  if (state === null) {
    return null;
  }

  const debugOverlay = state.debug?.visible === true ? <DebugPanel debug={state.debug} onEvent={emit} /> : null;
  // 常显 HUD 叠加在最底层：菜单相位/装配前的 hud 分节缺席，组件自身渲染
  // null；游戏/暂停相位有已装配世界时由 Go 下行驱动，菜单面板仍绘制在
  // HUD 之上（暂停遮罩盖住世界与 HUD 是既有层叠语义）。
  const hudOverlay = <HudRoot hud={state.hud} />;

  switch (state.phase) {
    case "game":
      // 游戏相位零 chrome：除 HUD 外无菜单参与（Rust 侧另有 hidden 语义）。
      return <>{hudOverlay}{debugOverlay}</>;
    case "menu":
    case "starting":
      return (
        <>
          {hudOverlay}
          {debugOverlay}
          {state.menu && <MainMenu menu={state.menu} onEvent={emit} />}
        </>
      );
    case "settings":
      return (
        <>
          {hudOverlay}
          {debugOverlay}
          {state.settings && <SettingsPanel settings={state.settings} onEvent={emit} />}
        </>
      );
    case "loading":
      // 加载屏不透明整屏遮罩：无合法上行动作（Enter 不得重复触发装配），
      // 加载分节数据缺席时组件自身安全降级；hud 分节在 loading 相位缺席，
      // HudRoot 保持既有层叠位置并自渲染 null。
      return (
        <>
          {hudOverlay}
          {debugOverlay}
          <LoadingScreen loading={state.loading} />
        </>
      );
    case "paused":
      return (
        <>
          {hudOverlay}
          {debugOverlay}
          {state.pause && <PauseMenu pause={state.pause} onEvent={emit} />}
        </>
      );
  }
}

// routeKeyDown 是菜单相位的统一键盘路由表（优先级自上而下，命中即消费）：
//
// | 相位/叠加        | 键        | 上行                                   |
// |------------------|-----------|----------------------------------------|
// | 调试面板可见      | F3        | debug-edit close（关面板）             |
// | 调试面板可见      | Escape    | 编辑中 cancel，否则 close              |
// | 调试面板可见      | ↑/↓/Enter | select-next/prev/enter-edit（编辑中忽略，
// |                  |           | 且不 preventDefault 以保留文本光标移动）|
// | paused           | Escape    | action pause-back                      |
// | settings         | Escape    | action settings-back（脏草稿由 Go 裁决）|
// | menu/starting    | Enter     | action enter-game（默认按钮；禁用即忽略，
// |                  |           | 焦点在按钮上时交还原生激活语义）        |
// | menu             | Escape    | 无语义，不产生事件                     |
// | loading          | 任意键    | 无语义，不产生事件（加载期无合法上行，  |
// |                  |           | Enter 不得重复触发装配）               |
function routeKeyDown(event: KeyboardEvent, state: UIState, emit: (event: UplinkEvent) => void): void {
  const debug = state.debug;
  if (debug?.visible === true) {
    const editing = debug.rows.some((row) => row.editing);
    switch (event.key) {
      case "F3":
        emit({ type: "debug-edit", op: "close" });
        break;
      case "Escape":
        emit({ type: "debug-edit", op: editing ? "cancel" : "close" });
        break;
      case "ArrowDown":
      case "ArrowUp":
      case "Enter":
        // 编辑期间阻止行选中切换与再次进入编辑；不 preventDefault，
        // 让输入框内的文本光标移动保持原生行为。
        if (editing) {
          return;
        }
        emit({
          type: "debug-edit",
          op: event.key === "ArrowDown" ? "select-next" : event.key === "ArrowUp" ? "select-prev" : "enter-edit",
        });
        break;
      default:
        // 面板可见时其余键不产生任何上行（游戏键捕获语义的 WebView 侧等价）。
        return;
    }
    event.preventDefault();
    return;
  }

  switch (state.phase) {
    case "paused":
      if (event.key === "Escape") {
        emit({ type: "action", id: "pause-back" });
        event.preventDefault();
      }
      return;
    case "settings":
      if (event.key === "Escape") {
        // 脏草稿时 Go 忽略返回并重推带提示的状态；前端不自行判定。
        emit({ type: "action", id: "settings-back" });
        event.preventDefault();
      }
      return;
    case "menu":
    case "starting":
      if (event.key === "Enter" && !isButtonTarget(event)) {
        // 默认按钮=进入游戏；starting 相位按钮经下行禁用，这里同样不发事件。
        const start = state.menu?.buttons.find((button) => button.id === "enter-game");
        if (start?.enabled === true) {
          emit({ type: "action", id: "enter-game" });
          event.preventDefault();
        }
      }
      return;
    case "loading":
      // 加载期无任何合法上行动作：全部按键静默（Go 侧对 loading 相位的动作
      // 事件另有防御档，这里是前端侧的零上行保证）。
      return;
    case "game":
      // 游戏相位 WebView 隐藏（无 debug 分节时不会进入这里），键盘归 winit。
      return;
  }
}

// isButtonTarget 报告按键目标是否为按钮：聚焦按钮上的 Enter/Space 由原生
// 激活语义处理（产生该按钮自己的 click），中枢不得再发默认按钮事件。
function isButtonTarget(event: KeyboardEvent): boolean {
  return event.target instanceof Element && event.target.tagName === "BUTTON";
}
