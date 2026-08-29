// App 相位切换冒烟测试：四屏组件按下行 phase 切换（game 相位零 chrome），
// F3 调试面板按 debug.visible 叠加于任意相位；点击/输入经 onEvent 上行，
// 语义对齐 ui.rs 既有动作清单。不接真 WKWebView，事件由注入的 spy 捕获。
import "@testing-library/jest-dom/vitest";
import { fireEvent } from "@testing-library/dom";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BridgeClient, type MenuState, type UIState, type UplinkEvent } from "../bridge/client";
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
});
