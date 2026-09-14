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
