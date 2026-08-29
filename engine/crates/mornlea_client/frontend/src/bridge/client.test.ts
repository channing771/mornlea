// 桥 client 纯函数与订阅接口单测：parseState/createEnvelope 是纯函数，
// BridgeClient 只把「校验后的状态分发给订阅者、把事件装信封投给 sink」，
// 全程不接真 WKWebView（sink 由测试注入）。
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  BridgeClient,
  BridgeProtocolError,
  createEnvelope,
  installMornleaGlobal,
  parseState,
  type UIState,
  type UplinkEvent,
} from "./client";

const menuState: UIState = {
  phase: "menu",
  menu: {
    title: "Mornlea",
    version: "dev",
    error: "",
    buttons: [
      { id: "enter-game", label: "进入游戏", enabled: true },
      { id: "multiplayer", label: "多人游戏", enabled: false },
    ],
  },
};

describe("parseState", () => {
  it("合法主菜单状态原样通过", () => {
    expect(parseState(menuState)).toEqual(menuState);
  });

  it("game 相位可只带 phase", () => {
    expect(parseState({ phase: "game" })).toEqual({ phase: "game" });
  });

  it("未知 phase 抛 BridgeProtocolError", () => {
    expect(() => parseState({ phase: "loading" })).toThrow(BridgeProtocolError);
  });

  it("非对象输入（null/数组/标量）抛 BridgeProtocolError", () => {
    for (const raw of [null, "menu", 42, [], true]) {
      expect(() => parseState(raw)).toThrow(BridgeProtocolError);
    }
  });

  it("未知按钮 id 抛 BridgeProtocolError", () => {
    const raw = {
      phase: "menu",
      menu: {
        title: "Mornlea",
        version: "dev",
        error: "",
        buttons: [{ id: "teleport", label: "传送", enabled: true }],
      },
    };
    expect(() => parseState(raw)).toThrow(BridgeProtocolError);
  });

  it("按钮 enabled 非布尔抛 BridgeProtocolError", () => {
    const raw = {
      phase: "menu",
      menu: {
        title: "Mornlea",
        version: "dev",
        error: "",
        buttons: [{ id: "enter-game", label: "进入游戏", enabled: "yes" }],
      },
    };
    expect(() => parseState(raw)).toThrow(BridgeProtocolError);
  });

  it("audioVolume 越界抛 BridgeProtocolError", () => {
    const raw = {
      phase: "settings",
      settings: {
        draft: { audioVolume: 1.5, texturePackPath: "", windowSize: "640x360" },
        saved: { audioVolume: 0, texturePackPath: "", windowSize: "640x360" },
        dirty: false,
        status: "",
        error: "",
      },
    };
    expect(() => parseState(raw)).toThrow(BridgeProtocolError);
  });

  it("未知调试行 kind 抛 BridgeProtocolError", () => {
    const raw = {
      phase: "game",
      debug: {
        visible: true,
        mode: "本地单机",
        rows: [{ label: "fps", value: "60", kind: "sparkline", readonly: true, selected: false, editing: false }],
      },
    };
    expect(() => parseState(raw)).toThrow(BridgeProtocolError);
  });

  it("readonly 调试行置位 selected/editing 抛 BridgeProtocolError", () => {
    const raw = {
      phase: "game",
      debug: {
        visible: true,
        mode: "本地单机",
        rows: [{ label: "fps", value: "60", kind: "param", readonly: true, selected: true, editing: false }],
      },
    };
    expect(() => parseState(raw)).toThrow(BridgeProtocolError);
  });

  it("顶层未知属性抛 BridgeProtocolError", () => {
    expect(() => parseState({ phase: "menu", cheat: true })).toThrow(BridgeProtocolError);
  });
});

describe("createEnvelope", () => {
  it("包出版本化信封 {v:1, events:[...]}", () => {
    const event: UplinkEvent = { type: "action", id: "enter-game" };
    expect(createEnvelope([event])).toEqual({ v: 1, events: [event] });
  });

  it("空事件批拒绝", () => {
    expect(() => createEnvelope([])).toThrow(BridgeProtocolError);
  });

  it("超过 64 条事件批拒绝", () => {
    const events = Array.from({ length: 65 }, () => ({ type: "action" as const, id: "quit" as const }));
    expect(() => createEnvelope(events)).toThrow(BridgeProtocolError);
  });
});

describe("BridgeClient", () => {
  it("handleState 校验并分发给订阅者", () => {
    const client = new BridgeClient(null);
    const listener = vi.fn();
    client.subscribe(listener);
    client.handleState(menuState);
    expect(listener).toHaveBeenCalledWith(menuState);
  });

  it("晚到的订阅者立即重放最新状态", () => {
    const client = new BridgeClient(null);
    client.handleState(menuState);
    const listener = vi.fn();
    client.subscribe(listener);
    expect(listener).toHaveBeenCalledTimes(1);
    expect(listener).toHaveBeenCalledWith(menuState);
  });

  it("退订后不再收到状态推送", () => {
    const client = new BridgeClient(null);
    const listener = vi.fn();
    const unsubscribe = client.subscribe(listener);
    unsubscribe();
    client.handleState(menuState);
    expect(listener).not.toHaveBeenCalled();
  });

  it("非法状态抛错且不冲掉既有最新状态", () => {
    const client = new BridgeClient(null);
    client.handleState(menuState);
    const listener = vi.fn();
    client.subscribe(listener);
    listener.mockClear();
    expect(() => client.handleState({ phase: "bogus" })).toThrow(BridgeProtocolError);
    expect(listener).not.toHaveBeenCalled();
    expect(client.latest).toEqual(menuState);
  });

  it("send 把事件包成单事件信封投给 sink", () => {
    const postMessage = vi.fn();
    const client = new BridgeClient({ postMessage });
    client.send({ type: "action", id: "pause-back" });
    expect(postMessage).toHaveBeenCalledWith({
      v: 1,
      events: [{ type: "action", id: "pause-back" }],
    });
  });

  it("sink 为 null 时 send 安全空操作", () => {
    const client = new BridgeClient(null);
    expect(() => client.send({ type: "action", id: "quit" })).not.toThrow();
  });
});

describe("installMornleaGlobal", () => {
  afterEach(() => {
    delete (globalThis as { mornlea?: unknown }).mornlea;
  });

  it("把 onState 注入目标对象并接通 handleState", () => {
    const client = new BridgeClient(null);
    const listener = vi.fn();
    client.subscribe(listener);
    const target: { mornlea?: { onState: (raw: unknown) => void } } = {};
    installMornleaGlobal(client, target);
    expect(target.mornlea).toBeDefined();
    target.mornlea?.onState(menuState);
    expect(listener).toHaveBeenCalledWith(menuState);
  });

  it("注入点收到非法状态时抛 BridgeProtocolError", () => {
    const client = new BridgeClient(null);
    const target: { mornlea?: { onState: (raw: unknown) => void } } = {};
    installMornleaGlobal(client, target);
    expect(() => target.mornlea?.onState({ phase: "bogus" })).toThrow(BridgeProtocolError);
  });
});
