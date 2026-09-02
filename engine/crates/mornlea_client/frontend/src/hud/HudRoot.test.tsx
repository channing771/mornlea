// HudRoot 挂载冒烟：菜单相位/装配前渲染 null、零尺寸视口安全降级、九格与
// 选中格双层轮廓、状态行逐槽解析与构图翻转、进食轨道呈现与叠加元素显隐。
// 规格语义的逐项断言矩阵（权威驱动/未确认隐藏/构图关系/缩放协调/饱和度归零
// 抖动）在 hud.assert.test.tsx，这里只钉住组件树能按桥下行状态正确挂载。
// 采掘进度条已退役（采掘进度反馈由世界空间裂纹承载），本文件不再出现采掘
// 轨道断言；旧形态的 mining 分节在桥层即被拒绝（client.test.ts）。
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { HudState, HudSlot } from "../bridge/client";
import { DESIGN_WIDTH } from "./geometry";
import { HudRoot } from "./HudRoot";

afterEach(cleanup);

function slot(overrides: Partial<HudSlot> = {}): HudSlot {
  return { item: 0, count: 0, ...overrides };
}

const nineSlots: readonly HudSlot[] = Array.from({ length: 9 }, (_, index) =>
  index === 2 ? { item: 7, count: 12, durability: 0.6 } : slot(),
);

const base: HudState = {
  viewport: { width: 1280, height: 720 },
  eating: { active: false, progress: 0 },
  hotbar: { slots: nineSlots, selectedIndex: 2 },
  health: { value: 7 },
  hunger: { value: 5, saturationZero: false },
  oxygen: { value: 300 },
};

function renderHud(hud: HudState | undefined) {
  const { container } = render(<HudRoot hud={hud} />);
  return container;
}

describe("HudRoot 挂载", () => {
  it("hud 缺席（菜单相位/装配前）与零尺寸视口都不呈现", () => {
    expect(renderHud(undefined).innerHTML).toBe("");
    expect(renderHud({ ...base, viewport: { width: 0, height: 0 } }).innerHTML).toBe("");
    expect(renderHud({ ...base, viewport: { width: 10, height: 720 } }).innerHTML).toBe("");
  });

  it("九格快捷栏与选中格双层轮廓：恰九格，选中格独享双层轮廓类", () => {
    const root = renderHud(base);
    expect(root.querySelectorAll(".hud-slot")).toHaveLength(9);
    expect(root.querySelectorAll(".hud-slot--selected")).toHaveLength(1);
    expect(root.querySelector(".hud-slot-count")?.textContent).toBe("12");
    expect(root.querySelectorAll(".hud-slot-durability")).toHaveLength(1);
  });

  it("状态行逐槽解析：生命七点=三满一半六空，饥饿五点=三填充（末格半格）", () => {
    const root = renderHud(base);
    expect(root.querySelectorAll(".hud-cell--heart-full")).toHaveLength(3);
    expect(root.querySelectorAll(".hud-cell--heart-half")).toHaveLength(1);
    expect(root.querySelectorAll(".hud-cell--heart-empty")).toHaveLength(6);
    expect(root.querySelectorAll(".hud-cell--hunger-full")).toHaveLength(2);
    expect(root.querySelectorAll(".hud-cell--hunger-half")).toHaveLength(1);
    expect(root.querySelectorAll(".hud-cell--hunger-empty")).toHaveLength(7);
    // 满氧不呈现任何气泡，但氧气行容器保留构图空间。
    expect(root.querySelectorAll(".hud-cell--bubble-full")).toHaveLength(0);
    expect(root.querySelector(".hud-status-row--oxygen")).not.toBeNull();
    // 图标是内联 SVG 像素剪影：每格携带非空 path（mask 生成不落空）。
    for (const path of root.querySelectorAll(".hud-cell path")) {
      expect((path.getAttribute("d") ?? "").length).toBeGreaterThan(0);
    }
  });

  it("耗损氧气按向上取整分段，未确认镜像不产生状态格", () => {
    const root = renderHud({ ...base, oxygen: { value: 90 } });
    expect(root.querySelectorAll(".hud-cell--bubble-full")).toHaveLength(3);
    expect(root.querySelectorAll(".hud-cell--bubble-empty")).toHaveLength(7);
    const unconfirmed = renderHud({
      ...base,
      health: undefined,
      hunger: undefined,
      oxygen: undefined,
    });
    expect(unconfirmed.querySelectorAll(".hud-cell")).toHaveLength(0);
    expect(unconfirmed.querySelector(".hud-status-row--oxygen")).not.toBeNull();
  });

  it("容器打开态翻转：状态行移到贴条下方并保持占位，弹条被抑制", () => {
    const closed = renderHud({ ...base, popup: { text: "橡木" } });
    const closedOrder = hudOrder(closed);
    expect(indexOfToken(closedOrder, "hud-status-row--primary")).toBeLessThan(
      indexOfToken(closedOrder, "hud-hotbar"),
    );

    const open = renderHud({ ...base, popup: { text: "橡木" }, containerOpen: true });
    expect(open.querySelector(".hud-root")?.className).toContain("hud-root--open");
    const openOrder = hudOrder(open);
    expect(indexOfToken(openOrder, "hud-hotbar")).toBeLessThan(
      indexOfToken(openOrder, "hud-status-row--primary"),
    );
    expect(indexOfToken(openOrder, "hud-status-row--primary")).toBeLessThan(
      indexOfToken(openOrder, "hud-status-row--oxygen"),
    );
    // 打开容器：弹条抑制、贴条仍在栈内（由 .hud-root--open 的 visibility
    // 规则让出像素、保留构图空间）、准星被面板遮挡语义下不呈现。
    expect(open.querySelector(".hud-popup")).toBeNull();
    expect(open.querySelector(".hud-hotbar")).not.toBeNull();
    expect(open.querySelector(".hud-hotbar--unconfirmed")).toBeNull();
    expect(open.querySelector(".hud-crosshair")).toBeNull();
  });

  it("进食轨道按激活呈现：eating 填充类唯一，不带任何形状标记", () => {
    const eating = renderHud({ ...base, eating: { active: true, progress: 0.8 } });
    expect(eating.querySelector(".hud-progress-fill--eating")).not.toBeNull();
    expect(eating.querySelectorAll(".hud-progress-fill")).toHaveLength(1);

    const inactive = renderHud(base);
    expect(inactive.querySelector(".hud-progress")).toBeNull();
  });

  it("零进度只呈现轨道：不产生填充（Go 分段同口径）", () => {
    const root = renderHud({ ...base, eating: { active: true, progress: 0 } });
    expect(root.querySelector(".hud-progress")).not.toBeNull();
    expect(root.querySelector(".hud-progress-fill")).toBeNull();
  });

  it("单一比例只由桥下行 viewport 计算（页面不读 window 尺寸参与缩放）", () => {
    const root = renderHud({ ...base, viewport: { width: 256, height: 720 } });
    const scale = Number(root.querySelector<HTMLElement>(".hud-root")?.style.getPropertyValue("--hud-scale"));
    expect(scale).toBeCloseTo((256 - 16) / DESIGN_WIDTH, 12);
  });

  it("叠加元素按窗口结果显隐：准星、marker 与聊天行栈", () => {
    const quiet = renderHud(base);
    expect(quiet.querySelector(".hud-crosshair")).toBeNull();
    expect(quiet.querySelector(".hud-marker")).toBeNull();
    expect(quiet.querySelector(".hud-chat")).toBeNull();

    const active = renderHud({
      ...base,
      crosshair: true,
      marker: true,
      chat: { lines: ["", "你好"] },
    });
    expect(active.querySelectorAll(".hud-crosshair-arm")).toHaveLength(2);
    expect(active.querySelectorAll(".hud-marker-arm")).toHaveLength(4);
    // 空串是合法行且占用一个行槽。
    expect(active.querySelectorAll(".hud-chat-line")).toHaveLength(2);
  });
});

/** 按栈内实际排布顺序取关键节点的类名序列（DOM 序 = 构图序，据此断言翻转）。 */
function hudOrder(root: HTMLElement): readonly (readonly string[])[] {
  return [...root.querySelectorAll(".hud-stack > *")].map((element) => element.className.split(" "));
}

function indexOfToken(order: readonly (readonly string[])[], token: string): number {
  return order.findIndex((classes) => classes.includes(token));
}
