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
    // loading 相位随世界加载屏 change 合法化，未知相位夹具改用占位值。
    expect(() => parseState({ phase: "bogus" })).toThrow(BridgeProtocolError);
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

  it("editValue 播种键合法时被解析保留", () => {
    const raw = {
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
    };
    const state = parseState(raw);
    expect(state.debug?.rows[0]?.editValue).toBe("9.80665");
  });

  it("readonly 调试行携带 editValue 抛 BridgeProtocolError", () => {
    const raw = {
      phase: "game",
      debug: {
        visible: true,
        mode: "本地单机",
        rows: [
          {
            label: "viewDistance",
            value: "8",
            kind: "param",
            readonly: true,
            selected: false,
            editing: false,
            editValue: "8",
          },
        ],
      },
    };
    expect(() => parseState(raw)).toThrow(BridgeProtocolError);
  });

  it("顶层未知属性抛 BridgeProtocolError", () => {
    expect(() => parseState({ phase: "menu", cheat: true })).toThrow(BridgeProtocolError);
  });
});

// 世界加载屏分节夹具：loaded/total 与 Go 组装（len(loadedChunks) 与
// LoadedChunkTarget=(2*(ViewDistance+1)+1)^2，默认 4489）同源。
describe("parseState 的 loading 分节", () => {
  it("loading 分节携带可选网格字段通过并原样保留", () => {
    const state = parseState({
      phase: "loading",
      loading: { loaded: 4489, total: 4489, meshed: 53868, meshTotal: 107736 },
    });
    expect(state.loading).toEqual({ loaded: 4489, total: 4489, meshed: 53868, meshTotal: 107736 });
  });

  it("loading 分节网格字段越界拒绝（meshTotal=0、meshed 负数）", () => {
    expect(() =>
      parseState({ phase: "loading", loading: { loaded: 1, total: 9, meshed: 0, meshTotal: 0 } }),
    ).toThrow(BridgeProtocolError);
    expect(() =>
      parseState({ phase: "loading", loading: { loaded: 1, total: 9, meshed: -1, meshTotal: 9 } }),
    ).toThrow(BridgeProtocolError);
  });

  it("loading 相位携带 loading 分节原样通过", () => {
    const state = parseState({ phase: "loading", loading: { loaded: 1122, total: 4489 } });
    expect(state.phase).toBe("loading");
    expect(state.loading).toEqual({ loaded: 1122, total: 4489 });
  });

  it("loading 相位可缺席 loading 分节（schema 只要求 phase，守卫保持单字段校验）", () => {
    const state = parseState({ phase: "loading" });
    expect(state.loading).toBeUndefined();
  });

  it("loading 分节缺 loaded 或 total 抛 BridgeProtocolError", () => {
    expect(() => parseState({ phase: "loading", loading: { total: 4489 } })).toThrow(
      BridgeProtocolError,
    );
    expect(() => parseState({ phase: "loading", loading: { loaded: 0 } })).toThrow(
      BridgeProtocolError,
    );
  });

  it("loading 分节未知属性抛 BridgeProtocolError", () => {
    expect(() =>
      parseState({ phase: "loading", loading: { loaded: 1, total: 4489, eta: "12s" } }),
    ).toThrow(BridgeProtocolError);
  });

  it("loading loaded 负数或 total=0 抛 BridgeProtocolError", () => {
    expect(() => parseState({ phase: "loading", loading: { loaded: -1, total: 4489 } })).toThrow(
      BridgeProtocolError,
    );
    expect(() => parseState({ phase: "loading", loading: { loaded: 0, total: 0 } })).toThrow(
      BridgeProtocolError,
    );
  });

  it("loading 分节非整数字段抛 BridgeProtocolError", () => {
    expect(() => parseState({ phase: "loading", loading: { loaded: 1.5, total: 4489 } })).toThrow(
      BridgeProtocolError,
    );
  });
});

// 游戏相位 hud 分节守卫夹具：字段域与 Go 侧镜像同源（九格、生命 0..20、
// 氧气 0..300、聊天至多 6 行）。
const hudSlot = (item: number, count: number, durability?: number) =>
  durability === undefined ? { item, count } : { item, count, durability };

const hudSlots = [
  hudSlot(1, 64),
  hudSlot(2, 7),
  hudSlot(12, 1, 0.5),
  ...Array.from({ length: 6 }, () => hudSlot(0, 0)),
];

const hudState = {
  viewport: { width: 1280, height: 720 },
  hotbar: { slots: hudSlots, selectedIndex: 2 },
  health: { value: 17 },
  hunger: { value: 18, saturationZero: true },
  oxygen: { value: 210 },
  eating: { active: false, progress: 0 },
  popup: { text: "石镐" },
  chat: { lines: ["系统：格式应为 @伙伴名 指令", ""] },
  marker: true,
  crosshair: true,
  containerOpen: false,
};

const hudRaw = { phase: "game", hud: hudState };

describe("parseState 的 hud 分节", () => {
  it("完整 hud 分节原样通过", () => {
    const state = parseState(hudRaw);
    expect(state.phase).toBe("game");
    expect(state.hud).toEqual(hudState);
  });

  it("可选分节缺席时保持缺席（组件据此隐藏）", () => {
    const state = parseState({
      phase: "game",
      hud: {
        viewport: hudState.viewport,
        eating: hudState.eating,
      },
    });
    expect(state.hud?.viewport).toEqual(hudState.viewport);
    expect(state.hud?.eating).toEqual(hudState.eating);
    expect(state.hud?.hotbar).toBeUndefined();
    expect(state.hud?.health).toBeUndefined();
    expect(state.hud?.hunger).toBeUndefined();
    expect(state.hud?.oxygen).toBeUndefined();
    expect(state.hud?.popup).toBeUndefined();
    expect(state.hud?.chat).toBeUndefined();
    expect(state.hud?.marker).toBeUndefined();
    expect(state.hud?.crosshair).toBeUndefined();
    expect(state.hud?.containerOpen).toBeUndefined();
  });

  it("进食未激活时恒携带的进度不触发呈现语义", () => {
    const state = parseState({
      phase: "game",
      hud: {
        ...hudState,
        eating: { active: false, progress: 0 },
      },
    });
    expect(state.hud?.eating).toEqual({ active: false, progress: 0 });
  });

  it("hud 未知属性抛 BridgeProtocolError（含已退役的 mining 分节）", () => {
    expect(() => parseState({ phase: "game", hud: { ...hudState, cheat: true } })).toThrow(
      BridgeProtocolError,
    );
    // 采掘进度条已退役：旧客户端形态里的 mining 分节按未知属性拒绝，
    // 解析器不对其新增宽松处理。
    expect(() =>
      parseState({
        phase: "game",
        hud: { ...hudState, mining: { active: true, progress: 0.25, harvestable: true } },
      }),
    ).toThrow(BridgeProtocolError);
  });

  it("hud 缺必填分节抛 BridgeProtocolError", () => {
    const { viewport: _viewport, ...noViewport } = hudState;
    expect(() => parseState({ phase: "game", hud: noViewport })).toThrow(BridgeProtocolError);
    const { eating: _eating, ...noEating } = hudState;
    expect(() => parseState({ phase: "game", hud: noEating })).toThrow(BridgeProtocolError);
  });

  it("hud 非对象输入抛 BridgeProtocolError", () => {
    for (const raw of [5, "hud", [], null]) {
      expect(() => parseState({ phase: "game", hud: raw })).toThrow(BridgeProtocolError);
    }
  });

  it("hotbar 格数与 selectedIndex 越界抛 BridgeProtocolError", () => {
    const eight = { slots: hudSlots.slice(1), selectedIndex: 0 };
    expect(() => parseState({ phase: "game", hud: { ...hudState, hotbar: eight } })).toThrow(
      BridgeProtocolError,
    );
    const overflow = { slots: [...hudSlots, hudSlot(0, 0)], selectedIndex: 0 };
    expect(() => parseState({ phase: "game", hud: { ...hudState, hotbar: overflow } })).toThrow(
      BridgeProtocolError,
    );
    const selected = { slots: hudSlots, selectedIndex: 9 };
    expect(() => parseState({ phase: "game", hud: { ...hudState, hotbar: selected } })).toThrow(
      BridgeProtocolError,
    );
  });

  it("slot 数值越界或非整数抛 BridgeProtocolError", () => {
    const cases = [
      [hudSlot(-1, 1), ...hudSlots.slice(1)],
      [hudSlot(65536, 1), ...hudSlots.slice(1)],
      [hudSlot(1, 65), ...hudSlots.slice(1)],
      [hudSlot(1, 1.5), ...hudSlots.slice(1)],
      [hudSlot(12, 1, 1.01), ...hudSlots.slice(1)],
    ];
    for (const slots of cases) {
      expect(() =>
        parseState({ phase: "game", hud: { ...hudState, hotbar: { slots, selectedIndex: 0 } } }),
      ).toThrow(BridgeProtocolError);
    }
  });

  it("生存数值越界抛 BridgeProtocolError", () => {
    expect(() => parseState({ phase: "game", hud: { ...hudState, health: { value: 21 } } })).toThrow(
      BridgeProtocolError,
    );
    expect(() => parseState({ phase: "game", hud: { ...hudState, hunger: { value: -1 } } })).toThrow(
      BridgeProtocolError,
    );
    expect(() =>
      parseState({ phase: "game", hud: { ...hudState, oxygen: { value: 301 } } }),
    ).toThrow(BridgeProtocolError);
  });

  it("hunger 缺 saturationZero 抛 BridgeProtocolError", () => {
    expect(() => parseState({ phase: "game", hud: { ...hudState, hunger: { value: 18 } } })).toThrow(
      BridgeProtocolError,
    );
  });

  it("进食进度缺字段或越界抛 BridgeProtocolError", () => {
    expect(() =>
      parseState({ phase: "game", hud: { ...hudState, eating: { active: true } } }),
    ).toThrow(BridgeProtocolError);
    expect(() =>
      parseState({ phase: "game", hud: { ...hudState, eating: { active: true, progress: 1.5 } } }),
    ).toThrow(BridgeProtocolError);
  });

  it("聊天行数与行宽越界抛 BridgeProtocolError", () => {
    expect(() =>
      parseState({
        phase: "game",
        hud: { ...hudState, chat: { lines: Array.from({ length: 7 }, () => "行") } },
      }),
    ).toThrow(BridgeProtocolError);
    expect(() =>
      parseState({ phase: "game", hud: { ...hudState, chat: { lines: ["一".repeat(33)] } } }),
    ).toThrow(BridgeProtocolError);
  });

  it("弹条文本越界抛 BridgeProtocolError", () => {
    expect(() =>
      parseState({ phase: "game", hud: { ...hudState, popup: { text: "一".repeat(33) } } }),
    ).toThrow(BridgeProtocolError);
  });

  it("非法 hud 不冲掉既有最新状态", () => {
    const client = new BridgeClient(null);
    client.handleState(hudRaw);
    expect(() => client.handleState({ phase: "game", hud: { ...hudState, cheat: 1 } })).toThrow(
      BridgeProtocolError,
    );
    expect(client.latest).toEqual(hudRaw);
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
