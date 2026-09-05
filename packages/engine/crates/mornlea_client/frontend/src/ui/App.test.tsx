// App 相位切换冒烟测试：四屏组件按下行 phase 切换（game 相位零 chrome），
// F3 调试面板按 debug.visible 叠加于任意相位；点击/输入经 onEvent 上行，
// 语义对齐 `internal/client` 的 UIAction* 动作清单。不接真 WKWebView，事件由注入的 spy 捕获。
import "@testing-library/jest-dom/vitest";
import { fireEvent } from "@testing-library/dom";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BridgeClient, type DebugRow, type MenuState, type UIState, type UplinkEvent } from "../bridge/client";
import { App } from "./App";
import { PAUSE_BACK_LABEL, PAUSE_QUIT_TO_MENU_LABEL } from "./copy";

afterEach(cleanup);

interface RenderResult {
  postMessage: ReturnType<typeof vi.fn>;
  events: UplinkEvent[];
}

function renderWithState(state: UIState, useEventSpy = false): RenderResult {
  const postMessage = vi.fn();
  const client = new BridgeClient({ postMessage });
  const events: UplinkEvent[] = [];
  const onEvent = useEventSpy ? (event: UplinkEvent) => events.push(event) : undefined;
  render(<App bridge={client} onEvent={onEvent} />);
  act(() => {
    client.handleState(state);
  });
  return { postMessage, events };
}

const menu: MenuState = {
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

const menuState: UIState = { phase: "menu", menu };

describe("App 相位切换", () => {
  it("menu 相位渲染主菜单：标题、按钮列、版本行，多人游戏禁用", () => {
    renderWithState(menuState);
    expect(screen.getByRole("heading", { name: "Mornlea" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "进入游戏" }).hasAttribute("disabled")).toBe(false);
    expect(screen.getByRole("button", { name: "多人游戏" }).hasAttribute("disabled")).toBe(true);
    expect(screen.getByRole("button", { name: "设置" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "退出游戏" })).toBeTruthy();
    expect(screen.getByText("dev")).toBeTruthy();
  });

  it("menu 相位点击启用按钮发出对应 action 事件", () => {
    const { events } = renderWithState(menuState, true);
    fireEvent.click(screen.getByRole("button", { name: "进入游戏" }));
    expect(events).toEqual([{ type: "action", id: "enter-game" }]);
  });

  it("starting 相位仍渲染主菜单（装配中重复点击由 Go 忽略）", () => {
    renderWithState({ ...menuState, phase: "starting" });
    expect(screen.getByRole("heading", { name: "Mornlea" })).toBeTruthy();
  });

  it("menu 相位装配失败错误行以 alert 呈现", () => {
    renderWithState({
      ...menuState,
      menu: { ...menu, error: "世界装配失败" },
    });
    expect(screen.getByRole("alert")).toHaveTextContent("世界装配失败");
  });

  it("settings 相位渲染设置页：三字段、窗口预设选中态与三动作", () => {
    renderWithState({
      phase: "settings",
      settings: {
        draft: { audioVolume: 0.37, texturePackPath: "packs/local", windowSize: "640x360" },
        saved: { audioVolume: 0.5, texturePackPath: "", windowSize: "640x360" },
        dirty: true,
        status: "已保存",
        error: "路径不存在",
      },
    });
    expect(screen.getByRole("heading", { name: "设置" })).toBeTruthy();
    expect(screen.getByRole("slider")).toHaveValue("37");
    expect(screen.getByRole("textbox", { name: "材质包目录" })).toHaveValue("packs/local");
    expect(screen.getByRole("button", { name: "640 × 360" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button", { name: "960 × 540" }).getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByText("有未保存的更改")).toBeTruthy();
    expect(screen.getByText("材质包路径保存后将在下次启动生效")).toBeTruthy();
    expect(screen.getByText("已保存")).toBeTruthy();
    expect(screen.getByRole("alert")).toHaveTextContent("路径不存在");
    expect(screen.getByRole("button", { name: "保存" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "取消更改" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "返回" })).toBeTruthy();
  });

  it("settings 相位滑杆与路径输入发出 settings-change 事件", () => {
    const { events } = renderWithState(
      {
        phase: "settings",
        settings: {
          draft: { audioVolume: 0.37, texturePackPath: "", windowSize: "640x360" },
          saved: { audioVolume: 0.37, texturePackPath: "", windowSize: "640x360" },
          dirty: false,
          status: "",
          error: "",
        },
      },
      true,
    );
    fireEvent.change(screen.getByRole("slider"), { target: { value: "64" } });
    expect(events).toContainEqual({ type: "settings-change", field: "audioVolume", value: 0.64 });
    fireEvent.change(screen.getByRole("textbox", { name: "材质包目录" }), {
      target: { value: "packs/hi" },
    });
    expect(events).toContainEqual({
      type: "settings-change",
      field: "texturePackPath",
      value: "packs/hi",
    });
    fireEvent.click(screen.getByRole("button", { name: "1280 × 720" }));
    expect(events).toContainEqual({
      type: "settings-change",
      field: "windowSize",
      value: "1280x720",
    });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    expect(events).toContainEqual({ type: "action", id: "settings-save" });
  });

  it("paused 相位渲染暂停层：两按钮与远程注明行", () => {
    const { events } = renderWithState({ phase: "paused", pause: { remote: true } }, true);
    expect(screen.getByRole("heading", { name: "已暂停" })).toBeTruthy();
    expect(screen.getByRole("button", { name: PAUSE_BACK_LABEL })).toBeTruthy();
    expect(screen.getByRole("button", { name: PAUSE_QUIT_TO_MENU_LABEL })).toBeTruthy();
    expect(screen.getByText("远程世界不会暂停，服务端仍在推进")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: PAUSE_BACK_LABEL }));
    expect(events).toEqual([{ type: "action", id: "pause-back" }]);
  });

  it("paused 本地形态不渲染远程注明行", () => {
    renderWithState({ phase: "paused", pause: { remote: false } });
    expect(screen.queryByText("远程世界不会暂停，服务端仍在推进")).toBeNull();
  });

  it("loading 相位渲染加载屏：标题、进度轨道与区块计数，主菜单不呈现", () => {
    renderWithState({ phase: "loading", loading: { loaded: 1122, total: 4489 } });
    expect(screen.getByRole("heading", { name: "正在生成世界…" })).toBeTruthy();
    expect(screen.getByText("区块 1122 / 4489")).toBeTruthy();
    expect(document.querySelector(".loading-progress")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "进入游戏" })).toBeNull();
  });

  it("loading 相位 loading 分节缺席时安全降级：标题与空轨仍在、计数行不呈现", () => {
    renderWithState({ phase: "loading" });
    expect(screen.getByRole("heading", { name: "正在生成世界…" })).toBeTruthy();
    const track = document.querySelector<HTMLElement>(".loading-progress");
    expect(track).toBeTruthy();
    expect(track?.style.getPropertyValue("--loading-fill")).toBe("0");
    expect(screen.queryByText(/^区块 /)).toBeNull();
  });

  it("状态推送前的初始渲染为零 chrome（不闪出旧相位）", () => {
    const { container } = render(<App bridge={new BridgeClient(null)} />);
    expect(container.textContent).toBe("");
  });

  it("game 相位零 chrome（无 WebView 参与的语义等价）", () => {
    const postMessage = vi.fn();
    const client = new BridgeClient({ postMessage });
    const { container } = render(<App bridge={client} />);
    act(() => {
      client.handleState({ phase: "game" });
    });
    expect(container.textContent).toBe("");
  });

  it("debug.visible 时调试面板叠加呈现：模式、读数、段头与参数行", () => {
    const { events } = renderWithState(
      {
        phase: "game",
        debug: {
          visible: true,
          mode: "本地单机",
          rows: [
            { label: "fps", value: "60", kind: "readout", readonly: true, selected: false, editing: false },
            { label: "── sim ──", value: "", kind: "section", readonly: true, selected: false, editing: false },
            { label: "gravity", value: "9.8", kind: "param", readonly: false, selected: true, editing: false },
          ],
        },
      },
      true,
    );
    expect(screen.getByText("本地单机")).toBeTruthy();
    expect(screen.getByRole("listbox")).toBeTruthy();
    expect(screen.getByText("── sim ──")).toBeTruthy();
    expect(screen.getByText("gravity")).toBeTruthy();
    fireEvent.keyDown(screen.getByRole("listbox"), { key: "ArrowDown" });
    expect(events).toContainEqual({ type: "debug-edit", op: "select-next" });
  });

  it("编辑态参数行渲染输入框并上行 edit-value/confirm", () => {
    const { events } = renderWithState(
      {
        phase: "game",
        debug: {
          visible: true,
          mode: "本地单机",
          rows: [
            { label: "gravity", value: "9.8", kind: "param", readonly: false, selected: true, editing: true },
          ],
        },
      },
      true,
    );
    const input = screen.getByRole("textbox", { name: "gravity" });
    fireEvent.change(input, { target: { value: "12.5" } });
    expect(events).toContainEqual({ type: "debug-edit", op: "edit-value", value: "12.5" });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(events).toContainEqual({ type: "debug-edit", op: "confirm", value: "12.5" });
  });

  it("默认事件出口经桥 client 装信封投给 WKWebView messageHandlers", () => {
    const { postMessage } = renderWithState(menuState);
    fireEvent.click(screen.getByRole("button", { name: "退出游戏" }));
    expect(postMessage).toHaveBeenCalledWith({
      v: 1,
      events: [{ type: "action", id: "quit" }],
    });
  });

  it("禁用按钮点击不产生任何桥事件（多人游戏）", () => {
    const { events } = renderWithState(menuState, true);
    fireEvent.click(screen.getByRole("button", { name: "多人游戏" }));
    expect(events).toEqual([]);
  });
});

describe("App 键盘路由（WebView 是 firstResponder，菜单键经桥上行）", () => {
  it("主菜单 Enter 触发默认按钮（进入游戏）恰一次", () => {
    const { events } = renderWithState(menuState, true);
    fireEvent.keyDown(window, { key: "Enter" });
    expect(events).toEqual([{ type: "action", id: "enter-game" }]);
  });

  it("starting 相位进入游戏按钮禁用：Enter 不产生事件（防重经下行禁用态驱动）", () => {
    // Go 下行在 starting 相位把进入游戏按钮置为禁用；前端契约是禁用态
    // 既不响应点击，也不响应默认按钮 Enter。
    const startingMenu: MenuState = {
      ...menu,
      buttons: menu.buttons.map((button) =>
        button.id === "enter-game" ? { ...button, enabled: false } : button,
      ),
    };
    const { events } = renderWithState({ phase: "starting", menu: startingMenu }, true);
    expect(screen.getByRole("button", { name: "进入游戏" }).hasAttribute("disabled")).toBe(true);
    fireEvent.keyDown(window, { key: "Enter" });
    fireEvent.click(screen.getByRole("button", { name: "进入游戏" }));
    expect(events).toEqual([]);
  });

  it("主菜单 Escape 无语义：不产生事件", () => {
    const { events } = renderWithState(menuState, true);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(events).toEqual([]);
  });

  it("焦点在按钮上时 Enter 交给原生激活语义，中枢不重复发默认按钮", () => {
    const { events } = renderWithState(menuState, true);
    fireEvent.keyDown(screen.getByRole("button", { name: "设置" }), { key: "Enter" });
    expect(events).toEqual([]);
  });

  it("设置页 Escape = 返回（脏草稿由 Go 裁决阻止）", () => {
    const { events } = renderWithState(
      {
        phase: "settings",
        settings: {
          draft: { audioVolume: 0.5, texturePackPath: "", windowSize: "640x360" },
          saved: { audioVolume: 0.5, texturePackPath: "", windowSize: "640x360" },
          dirty: true,
          status: "",
          error: "",
        },
      },
      true,
    );
    fireEvent.keyDown(window, { key: "Escape" });
    expect(events).toEqual([{ type: "action", id: "settings-back" }]);
  });

  it("暂停层 Escape = 返回游戏（面板可见时 winit 收不到 Esc，经桥重组）", () => {
    const { events } = renderWithState({ phase: "paused", pause: { remote: false } }, true);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(events).toEqual([{ type: "action", id: "pause-back" }]);
  });

  it("loading 相位任意按键（Enter/Esc/W）零上行（Enter 不得重复触发装配）", () => {
    const { events } = renderWithState(
      { phase: "loading", loading: { loaded: 1, total: 4489 } },
      true,
    );
    fireEvent.keyDown(window, { key: "Enter" });
    fireEvent.keyDown(window, { key: "Escape" });
    fireEvent.keyDown(window, { key: "w" });
    expect(events).toEqual([]);
  });

  const debugState = (rows: readonly DebugRow[]): UIState => ({
    phase: "game",
    debug: { visible: true, mode: "本地单机", rows },
  });

  const paramRows = (overrides: Partial<{ selected: boolean; editing: boolean; readonly: boolean; editValue?: string }> = {}) => [
    {
      label: "gravity",
      value: "9.807",
      kind: "param" as const,
      readonly: false,
      selected: true,
      editing: false,
      ...overrides,
    },
  ];

  it("调试面板可见：F3 关闭面板（close op 上行）", () => {
    const { events } = renderWithState(debugState(paramRows()), true);
    fireEvent.keyDown(window, { key: "F3" });
    expect(events).toEqual([{ type: "debug-edit", op: "close" }]);
  });

  it("调试面板非编辑态：Esc 关闭、方向键移动选中、Enter 进入编辑", () => {
    const { events } = renderWithState(debugState(paramRows()), true);
    fireEvent.keyDown(window, { key: "ArrowDown" });
    fireEvent.keyDown(window, { key: "ArrowUp" });
    fireEvent.keyDown(window, { key: "Enter" });
    fireEvent.keyDown(window, { key: "Escape" });
    expect(events).toEqual([
      { type: "debug-edit", op: "select-next" },
      { type: "debug-edit", op: "select-prev" },
      { type: "debug-edit", op: "enter-edit" },
      { type: "debug-edit", op: "close" },
    ]);
  });

  it("调试面板编辑态：Esc 取消编辑，方向键与 Enter 被忽略（阻止选中切换）", () => {
    const { events } = renderWithState(debugState(paramRows({ editing: true })), true);
    fireEvent.keyDown(window, { key: "ArrowDown" });
    fireEvent.keyDown(window, { key: "ArrowUp" });
    fireEvent.keyDown(window, { key: "Enter" });
    fireEvent.keyDown(window, { key: "Escape" });
    expect(events).toEqual([{ type: "debug-edit", op: "cancel" }]);
  });

  it("编辑播种用下行 editValue 全精度文本：不改文本确认不漂移", () => {
    const { events } = renderWithState(
      debugState(paramRows({ editing: true, editValue: "9.80665" })),
      true,
    );
    const input = screen.getByRole("textbox", { name: "gravity" }) as HTMLInputElement;
    expect(input.value).toBe("9.80665");
    fireEvent.keyDown(input, { key: "Enter" });
    // confirm 携带输入框原文；编辑态 Enter 不得再尾随 enter-edit。
    expect(events).toEqual([{ type: "debug-edit", op: "confirm", value: "9.80665" }]);
  });

  it("无 editValue 时编辑播种回退为行展示值", () => {
    renderWithState(debugState(paramRows({ editing: true })), true);
    const input = screen.getByRole("textbox", { name: "gravity" }) as HTMLInputElement;
    expect(input.value).toBe("9.807");
  });

  it("只读参数行以 aria-disabled 呈现（禁止选中与编辑的呈现面）", () => {
    renderWithState(
      debugState([
        { label: "viewDistance", value: "8", kind: "param", readonly: true, selected: false, editing: false },
      ]),
      true,
    );
    expect(screen.getByText("viewDistance").closest(".debug-row")?.getAttribute("aria-disabled")).toBe("true");
  });
});

const gameSlots = (count:number)=>Array.from({length:count},()=>({item:0,count:0}));
const gameState: NonNullable<UIState["game"]> = {
 token:42,kind:"inventory",cursorFree:true,confirmed:true,inventory:gameSlots(36),grid:gameSlots(9),gridSize:2,output:{item:0,count:0},chest:gameSlots(27),furnace:gameSlots(3),progress:0,burn:0,recipes:Array.from({length:10},()=>({name:"石砖",size:3,slots:gameSlots(9),output:{item:4,count:4}})),recipeIndex:-1,
};
it.each(["e","Escape"])("面板 %s 关闭按键返回语义事件",key=>{
 const {events}=renderWithState({phase:"game",game:gameState},true);
 fireEvent.keyDown(window,{key});expect(events).toEqual([{type:"game-action",token:42,op:"close"}]);
});
it("自由光标 Tab / 世界点击捕获，数字键请求权威选中",()=>{
 const {events}=renderWithState({phase:"game",game:{...gameState,kind:"none"}},true);
 fireEvent.keyDown(window,{key:"9"});fireEvent.keyDown(window,{key:"Tab"});
 expect(events).toEqual([{type:"game-action",token:42,op:"hotbar",index:8},{type:"game-action",token:42,op:"capture"}]);
 fireEvent.click(document.querySelector('.game-free-cursor')!);expect(events.at(-1)).toEqual({type:"game-action",token:42,op:"capture"});
});
it("捕获和暂停态不产生游戏操作",()=>{
 const {events}=renderWithState({phase:"game",game:{...gameState,kind:"none",cursorFree:false}},true);
 fireEvent.keyDown(window,{key:"e"});fireEvent.keyDown(window,{key:"9"});expect(events).toEqual([]);
 cleanup();const paused=renderWithState({phase:"paused",pause:{remote:false},game:gameState},true);
 fireEvent.keyDown(window,{key:"9"});expect(paused.events).toEqual([]);expect(screen.queryByRole('dialog')).toBeNull();
});
it("下行游戏状态拒绝越界来源与额外字段",()=>{
 const bridge=new BridgeClient(null);
 expect(()=>bridge.handleState({phase:"game",game:{...gameState,source:{area:"furnace",index:3}}})).toThrow();
 expect(()=>bridge.handleState({phase:"game",game:{...gameState,extra:true}})).toThrow();
 expect(()=>bridge.handleState({phase:"game",game:{...gameState,inventory:gameSlots(35)}})).toThrow();
});
