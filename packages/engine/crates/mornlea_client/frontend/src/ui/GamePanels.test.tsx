import { cleanup, createEvent, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { GamePanels } from "./GamePanels";
import type { GameState } from "../bridge/game";
const slots = (n: number) => Array.from({ length: n }, () => ({
  item: 0,
  count: 0
}));
const state: GameState = {
  token: 3,
  kind: "inventory",
  cursorFree: true,
  confirmed: true,
  inventory: slots(36),
  grid: slots(9),
  gridSize: 2,
  output: {
    item: 1,
    count: 1,
    name: "石头"
  },
  chest: slots(27),
  furnace: slots(3),
  progress: 0,
  burn: 0,
  recipeIndex: -1,
  recipes: Array.from({ length: 10 }, () => ({
    name: "配方",
    size: 3,
    slots: slots(9),
    output: {
      item: 1,
      count: 1
    }
  }))
};
afterEach(cleanup);
it("背包和产物发出语义事件", () => {
  const emit = vi.fn();
  render(<GamePanels game={state} onEvent={emit} />);
  fireEvent.click(screen.getByRole("button", { name: "背包 36：空" }));
  expect(emit).toHaveBeenLastCalledWith({
    type: "game-action",
    token: 3,
    op: "slot",
    area: "inventory",
    index: 35,
    button: "left",
    shift: false
  });
  fireEvent.click(screen.getByRole("button", { name: "取出合成产物：石头" }));
  expect(emit).toHaveBeenLastCalledWith({
    type: "game-action",
    token: 3,
    op: "take-output"
  });
});

it.each(["chest", "furnace", "workbench"] as const)("%s 面板语义槽位", kind => {
  const emit = vi.fn();
  render(<GamePanels game={{
    ...state, kind,
    gridSize: 3
  }} onEvent={emit} />);
  const area = kind === "workbench" ? "crafting" : kind;
  const label = kind === "workbench" ? "合成" : kind === "chest" ? "箱子" : "熔炉";
  fireEvent.click(screen.getByRole("button", { name: `${label} 1：空` }));
  expect(emit).toHaveBeenLastCalledWith({
    type: "game-action",
    token: 3,
    op: "slot", area,
    index: 0,
    button: "left",
    shift: false
  });
});

it("未确认时槽位禁用且人物页只读", () => {
  const emit = vi.fn();
  const { rerender } = render(<GamePanels game={{
    ...state,
    confirmed: false
  }} onEvent={emit} />);
  fireEvent.click(screen.getByRole("button", { name: "背包 1：空" }));
  expect(emit).not.toHaveBeenCalled();
  rerender(<GamePanels game={{
    ...state,
    kind: "character"
  }} onEvent={emit} />);
  expect(screen.queryByRole("button", { name: "背包 1：空" })).toBeNull();
  expect(screen.getByLabelText("原创体素旅人肖像")).toBeTruthy();
});

it("关闭与人物切换携带当前视图token", () => {
  const emit = vi.fn();
  render(<GamePanels game={state} onEvent={emit} />);
  fireEvent.click(screen.getByRole("button", { name: "人物" }));
  expect(emit).toHaveBeenLastCalledWith({
    type: "game-action",
    token: 3,
    op: "character"
  });
  fireEvent.click(screen.getByRole("button", { name: "关闭面板" }));
  expect(emit).toHaveBeenLastCalledWith({
    type: "game-action",
    token: 3,
    op: "close"
  });
});

it("最后一个背包格和关闭按钮都可键盘聚焦", () => {
  render(<GamePanels game={state} onEvent={() => { }} />);
  const last = screen.getByRole("button", { name: "背包 36：空" });
  last.focus();
  expect(document.activeElement).toBe(last);
  const close = screen.getByRole("button", { name: "关闭面板" });
  close.focus();
  expect(document.activeElement).toBe(close)
});

it("背包与配方材料复用同一缓存图标", () => {
  const icon = "data:image/png;base64,c2FtZS1zb3VyY2U=";
  const inventory = state.inventory.map(slot => ({ ...slot }));
  inventory[0] = {
    item: 1,
    count: 2,
    name: "石头", icon
  };
  const recipes = state.recipes.map(recipe => ({
    ...recipe,
    slots: [...recipe.slots]
  }));
  recipes[0]!.slots[0] = {
    item: 1,
    count: 1,
    name: "石头", icon
  };
  const { container } = render(<GamePanels game={{
    ...state, inventory, recipes,
    recipeIndex: 0
  }} onEvent={() => { }} />);
  expect(container.querySelectorAll(`img.game-item-icon[src="${icon}"]`)).toHaveLength(2);
});

// 左/右键 × Shift × 有无来源的发射矩阵：组件只按交互形状透传语义字段，
// 来源是否在 Go 侧记录不影响事件形状（半组选中与整堆同高亮）。
it.each([
  { button: "left", shift: false, source: false },
  { button: "left", shift: false, source: true },
  { button: "left", shift: true, source: false },
  { button: "left", shift: true, source: true },
  { button: "right", shift: false, source: false },
  { button: "right", shift: false, source: true },
] as const)("槽位发射矩阵 %s+shift=%s+source=%s", ({ button, shift, source }) => {
  const emit = vi.fn();
  render(<GamePanels game={source ? { ...state, source: { area: "inventory", index: 0 } } : state} onEvent={emit} />);
  if (source) {
    expect(screen.getByRole("button", { name: "背包 1：空" }).getAttribute("aria-pressed")).toBe("true");
  }
  const slot = screen.getByRole("button", { name: "背包 2：空" });
  if (button === "left") {
    fireEvent.click(slot, { shiftKey: shift });
  } else {
    fireEvent.contextMenu(slot, { shiftKey: shift });
  }
  expect(emit).toHaveBeenCalledTimes(1);
  expect(emit).toHaveBeenCalledWith({
    type: "game-action",
    token: 3,
    op: "slot",
    area: "inventory",
    index: 1,
    button,
    shift
  });
});

it("右键槽位阻止浏览器上下文菜单并携带 Shift 单件档", () => {
  const emit = vi.fn();
  render(<GamePanels game={state} onEvent={emit} />);
  const slot = screen.getByRole("button", { name: "背包 1：空" });
  const event = createEvent.contextMenu(slot, { shiftKey: true });
  fireEvent(slot, event);
  expect(event.defaultPrevented).toBe(true);
  expect(emit).toHaveBeenLastCalledWith({
    type: "game-action",
    token: 3,
    op: "slot",
    area: "inventory",
    index: 0,
    button: "right",
    shift: true
  });
});

it("右键面板空白不弹菜单也不发射事件", () => {
  const emit = vi.fn();
  render(<GamePanels game={state} onEvent={emit} />);
  // 页脚属面板空白承载面：容器级兜底只阻断原生菜单，不产生槽位语义事件。
  const blank = screen.getByText("E / Esc 关闭");
  const event = createEvent.contextMenu(blank);
  fireEvent(blank, event);
  expect(event.defaultPrevented).toBe(true);
  expect(emit).not.toHaveBeenCalled();
});

it("页脚提示行说明半组、单件与快捷搬运", () => {
  render(<GamePanels game={state} onEvent={() => { }} />);
  expect(screen.getByText("先选物品，再选目标位置；右键半组 / Shift+右键单件 / Shift+点击快速搬运")).toBeTruthy();
});

// ===== 指针拖拽 =====
// jsdom 未实现 PointerEvent，Event 构造器会丢弃 clientX/button 等 init 键；
// 测试用显式属性注入派发同形指针事件。
const firePointer = (
  target: Window | Element,
  type: "pointerdown" | "pointermove" | "pointerup" | "pointercancel",
  init: { button?: number; clientX?: number; clientY?: number } = {}
) => {
  const event = new Event(type, { bubbles: true, cancelable: true });
  Object.defineProperty(event, "button", { value: init.button ?? 0 });
  Object.defineProperty(event, "clientX", { value: init.clientX ?? 0 });
  Object.defineProperty(event, "clientY", { value: init.clientY ?? 0 });
  fireEvent(target, event);
};
// 拖拽夹具：背包 1（索引 0）放一组石头供拖起，全部路径共用同一构造。
const dragState = (): GameState => {
  const inventory = state.inventory.map((slot, index) => index === 0 ? { item: 1, count: 5, name: "石头" } : { ...slot });
  return { ...state, inventory };
};
const dragStart = () => {
  firePointer(screen.getByRole("button", { name: "背包 1：石头" }), "pointerdown", { clientX: 10, clientY: 10 });
  firePointer(window, "pointermove", { clientX: 40, clientY: 40 });
};

it("拖起呈现浮层与让位源槽，落到其它槽位发 dragMove", () => {
  const emit = vi.fn();
  const { container } = render(<GamePanels game={dragState()} onEvent={emit} />);
  dragStart();
  // 拖拽呈现：浮层只在实际拖拽时渲染，源槽位让位。
  const ghost = container.querySelector(".game-drag-ghost");
  expect(ghost).toBeTruthy();
  expect(screen.getByRole("button", { name: "背包 1：石头" }).className).toContain("game-slot--drag-source");
  firePointer(screen.getByRole("button", { name: "背包 2：空" }), "pointerup", { clientX: 40, clientY: 40 });
  expect(emit).toHaveBeenCalledTimes(1);
  expect(emit).toHaveBeenCalledWith({
    type: "game-action",
    token: 3,
    op: "dragMove",
    fromArea: "inventory",
    fromIndex: 0,
    toArea: "inventory",
    toIndex: 1
  });
  // 松手后浮层消失。
  expect(container.querySelector(".game-drag-ghost")).toBeNull();
});

it("拖到面板空白处松手取消且不发消息", () => {
  const emit = vi.fn();
  render(<GamePanels game={dragState()} onEvent={emit} />);
  dragStart();
  // 页脚是面板内非槽位空白承载面。
  firePointer(screen.getByText("E / Esc 关闭"), "pointerup", { clientX: 40, clientY: 40 });
  expect(emit).not.toHaveBeenCalled();
});

it("拖到面板外松手发按槽位寻址的整组丢弃", () => {
  const emit = vi.fn();
  const { container } = render(<GamePanels game={dragState()} onEvent={emit} />);
  dragStart();
  // 覆盖层属面板外空白（页面背景/世界）：整组丢弃按源槽寻址。
  const overlay = container.querySelector(".game-panel-overlay");
  expect(overlay).toBeTruthy();
  firePointer(overlay!, "pointerup", { clientX: 5, clientY: 5 });
  expect(emit).toHaveBeenCalledTimes(1);
  expect(emit).toHaveBeenCalledWith({
    type: "game-action",
    token: 3,
    op: "drop",
    area: "inventory",
    index: 0
  });
});

it("松手回源槽是取消语义且吞掉后续点击", () => {
  const emit = vi.fn();
  render(<GamePanels game={dragState()} onEvent={emit} />);
  dragStart();
  const source = screen.getByRole("button", { name: "背包 1：石头" });
  firePointer(source, "pointerup", { clientX: 40, clientY: 40 });
  expect(emit).not.toHaveBeenCalled();
  // 真实 WebView 会在拖拽结束后补一次 click：必须被吞掉，不得误发槽位语义。
  fireEvent.click(source);
  expect(emit).not.toHaveBeenCalled();
});

it("Esc 取消拖拽且不发消息", () => {
  const emit = vi.fn();
  const { container } = render(<GamePanels game={dragState()} onEvent={emit} />);
  dragStart();
  fireEvent.keyDown(window, { key: "Escape" });
  expect(emit).not.toHaveBeenCalled();
  expect(container.querySelector(".game-drag-ghost")).toBeNull();
  // Esc 取消不残留 click 抑制位：其后的键盘激活 click 仍走普通槽位语义
  //（不会被误吞一次）。
  fireEvent.click(screen.getByRole("button", { name: "背包 1：石头" }));
  expect(emit).toHaveBeenCalledTimes(1);
  expect(emit).toHaveBeenCalledWith({
    type: "game-action",
    token: 3,
    op: "slot",
    area: "inventory",
    index: 0,
    button: "left",
    shift: false
  });
});

it("拖拽被 pointercancel 中断后取消且不误结算", () => {
  const emit = vi.fn();
  const { container } = render(<GamePanels game={dragState()} onEvent={emit} />);
  dragStart();
  // 系统中断（触摸取消/系统手势）只有 pointercancel、没有 pointerup：
  // 按取消收尾，浮层消失。
  firePointer(window, "pointercancel");
  expect(container.querySelector(".game-drag-ghost")).toBeNull();
  // 中断后残留的 stray pointerup 不得把已取消的拖拽误结算成 dragMove/drop。
  firePointer(screen.getByRole("button", { name: "背包 2：空" }), "pointerup", { clientX: 40, clientY: 40 });
  expect(emit).not.toHaveBeenCalled();
  // 后续普通点击不受残留抑制位影响，仍走两次点击语义。
  fireEvent.click(screen.getByRole("button", { name: "背包 1：石头" }));
  expect(emit).toHaveBeenCalledTimes(1);
  expect(emit).toHaveBeenCalledWith({
    type: "game-action",
    token: 3,
    op: "slot",
    area: "inventory",
    index: 0,
    button: "left",
    shift: false
  });
});

it("窗口失焦时取消拖拽且不发消息", () => {
  const emit = vi.fn();
  const { container } = render(<GamePanels game={dragState()} onEvent={emit} />);
  dragStart();
  // cmd-tab 等失焦场景收不到 pointerup：按取消收尾，浮层消失。
  fireEvent.blur(window);
  expect(container.querySelector(".game-drag-ghost")).toBeNull();
  firePointer(window, "pointerup", { clientX: 40, clientY: 40 });
  expect(emit).not.toHaveBeenCalled();
});

it("拖拽中按下次键取消且不发消息", () => {
  const emit = vi.fn();
  const { container } = render(<GamePanels game={dragState()} onEvent={emit} />);
  dragStart();
  firePointer(window, "pointerdown", { button: 2, clientX: 40, clientY: 40 });
  expect(container.querySelector(".game-drag-ghost")).toBeNull();
  firePointer(window, "pointerup", { button: 0, clientX: 40, clientY: 40 });
  expect(emit).not.toHaveBeenCalled();
});

it("未越过阈值松手保持两次点击语义", () => {
  const emit = vi.fn();
  const { container } = render(<GamePanels game={dragState()} onEvent={emit} />);
  const source = screen.getByRole("button", { name: "背包 1：石头" });
  firePointer(source, "pointerdown", { clientX: 10, clientY: 10 });
  // 未移动即松手：无浮层、无拖拽消息，随后的 click 仍走普通槽位语义。
  firePointer(source, "pointerup", { clientX: 11, clientY: 10 });
  expect(container.querySelector(".game-drag-ghost")).toBeNull();
  fireEvent.click(source);
  expect(emit).toHaveBeenCalledTimes(1);
  expect(emit).toHaveBeenCalledWith({
    type: "game-action",
    token: 3,
    op: "slot",
    area: "inventory",
    index: 0,
    button: "left",
    shift: false
  });
});

it("未确认或空槽不发起拖拽", () => {
  const emit = vi.fn();
  // 未确认：面板数据整体处于 unconfirmed。
  const { container: unconfirmedContainer } = render(<GamePanels game={{ ...dragState(), confirmed: false }} onEvent={emit} />);
  const disabledSource = screen.getByRole("button", { name: "背包 1：石头" });
  firePointer(disabledSource, "pointerdown", { clientX: 10, clientY: 10 });
  firePointer(window, "pointermove", { clientX: 40, clientY: 40 });
  expect(unconfirmedContainer.querySelector(".game-drag-ghost")).toBeNull();
  firePointer(window, "pointerup", { clientX: 40, clientY: 40 });
  cleanup();
  // 空槽：已确认但源槽无物品。
  const { container: emptyContainer } = render(<GamePanels game={state} onEvent={emit} />);
  firePointer(screen.getByRole("button", { name: "背包 1：空" }), "pointerdown", { clientX: 10, clientY: 10 });
  firePointer(window, "pointermove", { clientX: 40, clientY: 40 });
  expect(emptyContainer.querySelector(".game-drag-ghost")).toBeNull();
  firePointer(window, "pointerup", { clientX: 40, clientY: 40 });
  expect(emit).not.toHaveBeenCalled();
});

it("拖拽中的权威刷新照常应用且拖拽呈现保持", () => {
  const emit = vi.fn();
  const { rerender, container } = render(<GamePanels game={dragState()} onEvent={emit} />);
  dragStart();
  // 权威刷新：背包 2 被服务端确认填入泥土，源槽数量变化为 4。
  const refreshed = {
    ...dragState(),
    inventory: dragState().inventory.map((slot, index) => {
      if (index === 0) return { item: 1, count: 4, name: "石头" };
      if (index === 1) return { item: 2, count: 3, name: "泥土" };
      return { ...slot };
    })
  };
  rerender(<GamePanels game={refreshed} onEvent={emit} />);
  // 面板底层数据更新为确认状态。
  expect(screen.getByRole("button", { name: "背包 2：泥土" })).toBeTruthy();
  // 拖拽浮层保持跟随（快照呈现被拖组）。
  expect(container.querySelector(".game-drag-ghost")).toBeTruthy();
  // 松手落槽仍按语义引用结算。
  firePointer(screen.getByRole("button", { name: "背包 2：泥土" }), "pointerup", { clientX: 40, clientY: 40 });
  expect(emit).toHaveBeenCalledWith({
    type: "game-action",
    token: 3,
    op: "dragMove",
    fromArea: "inventory",
    fromIndex: 0,
    toArea: "inventory",
    toIndex: 1
  });
});
