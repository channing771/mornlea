// App：按下行 phase 切换四屏（menu/starting → 主菜单、settings → 设置页、
// paused → 暂停层、game → 零 chrome），F3 调试面板按 debug.visible 叠加于
// 任意相位。前端不持任何菜单语义：相位机、草稿与装配裁决都在 Go。
import { useEffect, useState } from "react";
import {
  bridge as defaultBridge,
  type BridgeClient,
  type UIState,
  type UplinkEvent,
} from "../bridge/client";
import { DebugPanel } from "./DebugPanel";
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

  if (state === null) {
    return null;
  }

  const debugOverlay = state.debug?.visible === true ? <DebugPanel debug={state.debug} onEvent={emit} /> : null;

  switch (state.phase) {
    case "game":
      // 游戏相位零 chrome：WebView 侧无菜单参与（Rust 侧另有 hidden 语义）。
      return debugOverlay;
    case "menu":
    case "starting":
      return (
        <>
          {debugOverlay}
          {state.menu && <MainMenu menu={state.menu} onEvent={emit} />}
        </>
      );
    case "settings":
      return (
        <>
          {debugOverlay}
          {state.settings && <SettingsPanel settings={state.settings} onEvent={emit} />}
        </>
      );
    case "paused":
      return (
        <>
          {debugOverlay}
          {state.pause && <PauseMenu pause={state.pause} onEvent={emit} />}
        </>
      );
  }
}
