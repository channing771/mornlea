// HUD 组件断言矩阵：把常显 HUD 的规格语义逐项平移到组件级断言——权威驱动与
// 未确认隐藏、构图关系（生命左缘/饥饿右缘/氧气外堆叠/容器打开态翻转/净空令牌
// 消费）、形状差异（可采末端标记 vs 不可采固定警示缺口）、单一比例缩放协调，
// 以及饱和度归零抖动只落在饥饿行。挂载冒烟在 HudRoot.test.tsx，这里不复述。
//
// 断言手段：jsdom 不做布局、样式表也不注入文档，因此构图序用 DOM 序表达
// （组件把 JSX 顺序刻意保持为视觉序），间距的「类 → 令牌」消费关系在样式表
// 原文上核对；令牌数值与 design 常量的同值由 geometry.test.ts 互钉，像素级
// 构图由 visual/ 基线兜底。
import { cleanup, render } from "@testing-library/react";
import { readFileSync } from "node:fs";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import type { HudSlot, HudState } from "../bridge/client";
import { DESIGN_WIDTH, EDGE_MARGIN } from "./geometry";
import { HudRoot } from "./HudRoot";

afterEach(cleanup);

function slot(overrides: Partial<HudSlot> = {}): HudSlot {
  return { item: 0, count: 0, ...overrides };
}

const nineSlots: readonly HudSlot[] = Array.from({ length: 9 }, (_, index) =>
  index === 4 ? { item: 6, count: 3, durability: 0.6 } : slot(),
);

// 基准状态：九格已确认镜像 + 生命 7（三满一半）+ 饥饿 5（奇数，末格右半）+
// 满氧（按契约不呈现）+ 两条进度条未激活；叠加元素由各用例显式置位。
const base: HudState = {
  viewport: { width: 1280, height: 720 },
  mining: { active: false, progress: 0, harvestable: false },
  eating: { active: false, progress: 0 },
  hotbar: { slots: nineSlots, selectedIndex: 4 },
  health: { value: 7 },
  hunger: { value: 5, saturationZero: false },
  oxygen: { value: 300 },
};

function renderHud(hud: HudState | undefined): HTMLElement {
  const { container } = render(<HudRoot hud={hud} />);
  return container;
}

/** 栈内子元素的构图序：每个节点归一到关心的一枚类名，出现未知节点即抛错。 */
function stackOrder(root: HTMLElement): readonly string[] {
  const tokens = [
    "hud-popup",
    "hud-progress",
    "hud-status-row--primary",
    "hud-status-row--oxygen",
    "hud-hotbar",
  ];
  return [...root.querySelectorAll(".hud-stack > *")].map((element) => {
    const token = tokens.find((candidate) => element.classList.contains(candidate));
    if (token === undefined) {
      throw new Error(`状态栈出现未知节点：${element.className}`);
    }
    return token;
  });
}

/** 组内逐格的填充类序列：空格/半格/满格的 DOM 序，方向语义由组件的
 * `row-reverse`/`flex-start` 承担，这里钉住「DOM 序 = 填充推进序」。 */
function cellSequence(group: Element | null | undefined): readonly string[] {
  if (group === null || group === undefined) {
    return [];
  }
  return [...group.children].map(
    (cell) =>
      [...cell.classList].find((name) => name.startsWith("hud-cell--")) ??
      `(无填充类：${cell.className})`,
  );
}

/** 取第 index 个子元素；越界抛错而不是让断言落在 undefined 上恒真。 */
function childAt(parent: Element | null | undefined, index: number): Element {
  const child = parent?.children[index];
  if (child === undefined || parent === null || parent === undefined) {
    throw new Error(`子元素越界：index=${index}`);
  }
  return child;
}

// ---- 样式表层钉（jsdom 不计算样式，只能核对类 → 令牌的消费关系） ----

function readHudCss(): string {
  return readFileSync(path.resolve(process.cwd(), "src/hud/hud.css"), "utf8")
    .replace(/\/\*[\s\S]*?\*\//g, " ")
    .replace(/\s+/g, " ");
}

/** 取指定选择器全部规则块的正则化正文：同名选择器分块书写时按序合并，
 * 选择器不存在即抛错（防断言落在空串上恒真）。 */
function ruleBodies(css: string, selector: string): string {
  const bodies: string[] = [];
  for (const block of css.split("}")) {
    const brace = block.indexOf("{");
    if (brace < 0) {
      continue;
    }
    const selectors = block
      .slice(0, brace)
      .split(",")
      .map((part) => part.trim());
    if (selectors.includes(selector)) {
      bodies.push(block.slice(brace + 1));
    }
  }
  if (bodies.length === 0) {
    throw new Error(`hud.css 缺少选择器 ${selector}`);
  }
  return bodies.join(" ");
}

describe("权威驱动与未确认隐藏", () => {
  it("快捷栏镜像缺席：贴条保留构图占位，但不产生格、选中轮廓、数量或耐久", () => {
    const root = renderHud({ ...base, hotbar: undefined });
    expect(root.querySelector(".hud-hotbar--unconfirmed")).not.toBeNull();
    expect(root.querySelectorAll(".hud-slot")).toHaveLength(0);
    expect(root.querySelectorAll(".hud-slot--selected")).toHaveLength(0);
    expect(root.querySelectorAll(".hud-slot-tile")).toHaveLength(0);
    expect(root.querySelectorAll(".hud-slot-count")).toHaveLength(0);
    expect(root.querySelectorAll(".hud-slot-durability")).toHaveLength(0);
  });

  it("生命与饥饿缺席：组容器保留（零像素），但不产生任何状态格", () => {
    const root = renderHud({ ...base, health: undefined, hunger: undefined });
    expect(root.querySelector(".hud-status-group--health")).not.toBeNull();
    expect(root.querySelector(".hud-status-group--hunger")).not.toBeNull();
    expect(root.querySelectorAll(".hud-cell")).toHaveLength(0);
  });

  it("氧气满值与缺席：零气泡呈现，行容器保留构图空间（周边元素不跳动）", () => {
    for (const oxygen of [undefined, { value: 300 }]) {
      const root = renderHud({ ...base, oxygen });
      const bubbles = root.querySelectorAll(".hud-cell--bubble-full, .hud-cell--bubble-empty");
      expect(bubbles).toHaveLength(0);
      expect(root.querySelector(".hud-status-row--oxygen")).not.toBeNull();
    }
  });

  it("零值边界：氧气/饥饿/生命零值各呈现十格空刻度（氧气零值也是十个空气泡）", () => {
    const zero = renderHud({
      ...base,
      oxygen: { value: 0 },
      hunger: { value: 0, saturationZero: false },
      health: { value: 0 },
    });
    expect(cellSequence(zero.querySelector(".hud-status-row--oxygen"))).toEqual(
      Array<string>(10).fill("hud-cell--bubble-empty"),
    );
    expect(cellSequence(zero.querySelector(".hud-status-group--hunger"))).toEqual(
      Array<string>(10).fill("hud-cell--hunger-empty"),
    );
    expect(cellSequence(zero.querySelector(".hud-status-group--health"))).toEqual(
      Array<string>(10).fill("hud-cell--heart-empty"),
    );
  });

  it("生命上界：已确认 20（=MaxHealth）按钳制值呈现十个满心，不超十格", () => {
    const full = renderHud({ ...base, health: { value: 20 } });
    const hearts = full.querySelector(".hud-status-group--health");
    expect(cellSequence(hearts)).toEqual(Array<string>(10).fill("hud-cell--heart-full"));
    expect(hearts?.children).toHaveLength(10);
  });

  it("叠加元素按 presence 呈现：弹条/marker/准星/聊天缺席或空缓冲不产生节点", () => {
    const quiet = renderHud(base);
    expect(quiet.querySelector(".hud-popup")).toBeNull();
    expect(quiet.querySelector(".hud-crosshair")).toBeNull();
    expect(quiet.querySelector(".hud-marker")).toBeNull();
    expect(quiet.querySelector(".hud-chat")).toBeNull();

    const emptyChat = renderHud({ ...base, chat: { lines: [] } });
    expect(emptyChat.querySelector(".hud-chat")).toBeNull();

    const active = renderHud({
      ...base,
      popup: { text: "橡木原木" },
      crosshair: true,
      marker: true,
      chat: { lines: ["你好"] },
    });
    expect(active.querySelector(".hud-popup")?.textContent).toBe("橡木原木");
    expect(active.querySelectorAll(".hud-crosshair-arm")).toHaveLength(2);
    expect(active.querySelectorAll(".hud-marker-arm")).toHaveLength(4);
    expect(active.querySelectorAll(".hud-chat-line")).toHaveLength(1);
  });

  it("数量与耐久只随权威语义呈现：单件无数字、满/零耐久无条、低耐久独立档", () => {
    const root = renderHud({
      ...base,
      hotbar: {
        slots: [
          { item: 1, count: 1 },
          { item: 2, count: 64 },
          { item: 0, count: 0 },
          { item: 4, count: 1, durability: 1 },
          { item: 5, count: 1, durability: 0 },
          { item: 6, count: 1, durability: 0.6 },
          { item: 7, count: 1, durability: 0.25 },
          { item: 8, count: 1, durability: 0.2 },
          { item: 9, count: 1 },
        ],
        selectedIndex: 0,
      },
    });
    const slots = [...root.querySelectorAll(".hud-slot")];
    expect(slots).toHaveLength(9);
    // 单件堆叠：tile 呈现、数量不呈现；两位数量上界照实呈现。
    expect(childAt(slots[0], 0).classList.contains("hud-slot-tile")).toBe(true);
    expect(root.querySelectorAll(".hud-slot-count")).toHaveLength(1);
    expect(slots[1]?.querySelector(".hud-slot-count")?.textContent).toBe("64");
    // 空格：不产生物品 tile。
    expect(slots[2]?.querySelector(".hud-slot-tile")).toBeNull();
    // 满耐久与零耐久都不产生耐久条（呈现窗口是 0 < ratio < 1）。
    expect(slots[3]?.querySelector(".hud-slot-durability")).toBeNull();
    expect(slots[4]?.querySelector(".hud-slot-durability")).toBeNull();
    // 低耐久档的边界与 Go 一致：跌破四分之一才换档，恰在阈值仍是健康档。
    expect(slots[5]?.querySelector(".hud-slot-durability-fill--low")).toBeNull();
    expect(slots[6]?.querySelector(".hud-slot-durability-fill--low")).toBeNull();
    expect(slots[7]?.querySelector(".hud-slot-durability-fill--low")).not.toBeNull();
    // 选中格双层轮廓只落在 selectedIndex 一格。
    expect(root.querySelectorAll(".hud-slot--selected")).toHaveLength(1);
    expect(slots[0]?.classList.contains("hud-slot--selected")).toBe(true);
  });
});

describe("构图关系", () => {
  it("生命锚快捷栏左缘、饥饿锚右缘：主行两端组序固定，填充按各自方向推进", () => {
    const root = renderHud(base);
    const primary = root.querySelector(".hud-status-row--primary");
    expect(primary?.children).toHaveLength(2);
    expect(childAt(primary, 0).classList.contains("hud-status-group--health")).toBe(true);
    expect(childAt(primary, 1).classList.contains("hud-status-group--hunger")).toBe(true);
    // 生命自左向右推进：三满心、半心在第四格、其余空心。
    expect(cellSequence(childAt(primary, 0))).toEqual([
      "hud-cell--heart-full",
      "hud-cell--heart-full",
      "hud-cell--heart-full",
      "hud-cell--heart-half",
      "hud-cell--heart-empty",
      "hud-cell--heart-empty",
      "hud-cell--heart-empty",
      "hud-cell--heart-empty",
      "hud-cell--heart-empty",
      "hud-cell--heart-empty",
    ]);
    // 饥饿行自右向左排列：DOM 首格贴右缘，半鸡腿落在推进末端（第三个 DOM 格）。
    expect(cellSequence(childAt(primary, 1))).toEqual([
      "hud-cell--hunger-full",
      "hud-cell--hunger-full",
      "hud-cell--hunger-half",
      ...Array<string>(7).fill("hud-cell--hunger-empty"),
    ]);
  });

  it("氧气沿饥饿右缘向外堆叠：DOM 首格贴右缘，耗损段自右向左推进", () => {
    // ceil(90 × 10 / 300) = 3 个满气泡，其余七格是常驻空气泡刻度。
    const root = renderHud({ ...base, oxygen: { value: 90 } });
    expect(cellSequence(root.querySelector(".hud-status-row--oxygen"))).toEqual([
      "hud-cell--bubble-full",
      "hud-cell--bubble-full",
      "hud-cell--bubble-full",
      ...Array<string>(7).fill("hud-cell--bubble-empty"),
    ]);
  });

  it("容器打开态翻转：主行与氧气行移到贴条下方，弹条抑制而贴条占位保留", () => {
    const mining = { active: true, progress: 0.4, harvestable: true };
    const closed = renderHud({ ...base, popup: { text: "橡木原木" }, mining });
    expect(stackOrder(closed)).toEqual([
      "hud-popup",
      "hud-progress",
      "hud-status-row--oxygen",
      "hud-status-row--primary",
      "hud-hotbar",
    ]);

    const open = renderHud({
      ...base,
      popup: { text: "橡木原木" },
      mining,
      containerOpen: true,
    });
    expect(open.querySelector(".hud-root")?.classList.contains("hud-root--open")).toBe(true);
    expect(stackOrder(open)).toEqual([
      "hud-progress",
      "hud-hotbar",
      "hud-status-row--primary",
      "hud-status-row--oxygen",
    ]);
    // 弹条被抑制；贴条仍在栈内，让出像素由打开态样式规则承担。
    expect(open.querySelector(".hud-popup")).toBeNull();
    expect(open.querySelector(".hud-hotbar--unconfirmed")).toBeNull();
  });

  it("净空与行距经令牌由类消费：状态行-贴条净空不旁路、不裸写数值", () => {
    const css = readHudCss();
    const marginOf = (token: string) => `margin-top: calc(${token} * var(--hud-scale))`;
    // 关闭态自下而上：贴条净空 → 主行行距 → 氧气行 → 轨道间隙。
    expect(ruleBodies(css, ".hud-hotbar")).toContain(marginOf("var(--hud-status-hotbar-gap)"));
    expect(ruleBodies(css, ".hud-status-row--primary")).toContain(
      marginOf("var(--hud-status-bar-gap)"),
    );
    expect(ruleBodies(css, ".hud-status-row--oxygen")).toContain(
      marginOf("var(--hud-progress-track-gap)"),
    );
    expect(ruleBodies(css, ".hud-progress")).toContain(marginOf("var(--hud-popup-track-gap)"));
    // 打开态：主行贴到贴条外沿之下「净空 + 行距」，氧气行只隔一个行距。
    // （该声明在 hud.css 内跨行书写，正则化后 calc( 与 ( 之间留一个空格。）
    expect(ruleBodies(css, ".hud-root--open .hud-status-row--primary")).toContain(
      "margin-top: calc( (var(--hud-status-hotbar-gap) + var(--hud-status-bar-gap))" +
        " * var(--hud-scale) )",
    );
    expect(ruleBodies(css, ".hud-root--open .hud-status-row--oxygen")).toContain(
      marginOf("var(--hud-status-bar-gap)"),
    );
    // 打开态贴条让出像素（空间保留），底部预留贴条下沿内边距。
    expect(ruleBodies(css, ".hud-root--open .hud-hotbar")).toContain("visibility: hidden");
    expect(ruleBodies(css, ".hud-root--open .hud-stack")).toContain(
      "padding-bottom: calc(var(--hud-hotbar-bottom-margin) * var(--hud-scale))",
    );
    // 未确认镜像与「饥饿自右向左」「氧气沿右缘堆叠」的排列方向也是类 → 令牌
    // 之外的构图承诺，同样在样式表层钉住。
    expect(ruleBodies(css, ".hud-hotbar--unconfirmed")).toContain("visibility: hidden");
    expect(ruleBodies(css, ".hud-status-group--hunger")).toContain("flex-direction: row-reverse");
    expect(ruleBodies(css, ".hud-status-row--oxygen")).toContain("flex-direction: row-reverse");
  });
});

describe("形状差异", () => {
  it("可采与不可采几何序列不同：末端亮标记 vs 固定三处警示缺口", () => {
    const harvestable = renderHud({
      ...base,
      mining: { active: true, progress: 0.5, harvestable: true },
    });
    expect(
      harvestable
        .querySelector(".hud-progress-fill")
        ?.classList.contains("hud-progress-fill--mining-harvestable"),
    ).toBe(true);
    expect(harvestable.querySelectorAll(".hud-progress-cap")).toHaveLength(1);
    expect(harvestable.querySelectorAll(".hud-progress-notch")).toHaveLength(0);

    const blocked = renderHud({
      ...base,
      mining: { active: true, progress: 0.5, harvestable: false },
    });
    expect(
      blocked
        .querySelector(".hud-progress-fill")
        ?.classList.contains("hud-progress-fill--mining-blocked"),
    ).toBe(true);
    expect(blocked.querySelectorAll(".hud-progress-cap")).toHaveLength(0);
    // 固定位置（25%/50%/75%）与固定数量是形状差异的一半，不能只数节点。
    const notches = [...blocked.querySelectorAll<HTMLElement>(".hud-progress-notch")];
    expect(notches.map((notch) => notch.style.left)).toEqual(["25%", "50%", "75%"]);
  });

  it("双层轮廓与双层文字经类消费：选中格外框+内衬、数量与弹条的阴影+前景", () => {
    const css = readHudCss();
    // 选中格的两层都是几何标记：外扩 outline 画在格框之外，sage 内衬铺在格心
    // （::after），忽略颜色后仍可与未选中格区分。
    expect(ruleBodies(css, ".hud-slot--selected")).toContain("outline:");
    expect(ruleBodies(css, ".hud-slot--selected::after")).toContain(
      "background: var(--accent-sage)",
    );
    // 数量数字与物品名弹条都以「阴影 + 前景」双层呈现。
    const shadowLayer =
      "text-shadow: var(--hud-text-shadow-offset-scaled) var(--hud-text-shadow-offset-scaled)" +
      " 0 var(--text-shadow)";
    expect(ruleBodies(css, ".hud-slot-count")).toContain(shadowLayer);
    expect(ruleBodies(css, ".hud-popup")).toContain(shadowLayer);
  });

  it("进食轨道不带任何形状标记，采掘激活时优先呈现", () => {
    const eating = renderHud({ ...base, eating: { active: true, progress: 0.3 } });
    expect(eating.querySelector(".hud-progress-fill--eating")).not.toBeNull();
    expect(eating.querySelectorAll(".hud-progress-cap, .hud-progress-notch")).toHaveLength(0);

    const both = renderHud({
      ...base,
      mining: { active: true, progress: 0.4, harvestable: false },
      eating: { active: true, progress: 0.3 },
    });
    expect(both.querySelector(".hud-progress-fill--mining-blocked")).not.toBeNull();
    expect(both.querySelector(".hud-progress-fill--eating")).toBeNull();
  });

  it("进度比例钳制到 0..1：超额钉在满轨，零与负值不产生填充或标记", () => {
    const over = renderHud({
      ...base,
      mining: { active: true, progress: 2.5, harvestable: true },
    });
    const fill = over.querySelector<HTMLElement>(".hud-progress")?.style.getPropertyValue("--hud-fill");
    expect(fill).toBe("1");

    const under = renderHud({
      ...base,
      mining: { active: true, progress: -1, harvestable: true },
    });
    expect(under.querySelector(".hud-progress")).not.toBeNull();
    expect(under.querySelector(".hud-progress-fill")).toBeNull();
    expect(under.querySelectorAll(".hud-progress-cap, .hud-progress-notch")).toHaveLength(0);
  });
});

describe("缩放协调", () => {
  it("单一比例只随桥下行 viewport 变化，页面不读 window 尺寸参与缩放", () => {
    const scaleOf = (root: HTMLElement) =>
      Number(root.querySelector<HTMLElement>(".hud-root")?.style.getPropertyValue("--hud-scale"));
    const narrow = renderHud({ ...base, viewport: { width: 492, height: 720 } });
    expect(scaleOf(narrow)).toBeCloseTo((492 - 2 * EDGE_MARGIN) / DESIGN_WIDTH, 12);

    const smaller = renderHud({ ...base, viewport: { width: 256, height: 720 } });
    expect(scaleOf(smaller)).toBeCloseTo((256 - 2 * EDGE_MARGIN) / DESIGN_WIDTH, 12);
    expect(scaleOf(smaller)).toBeLessThan(scaleOf(narrow));
    // 缩放只改比例不改构图：窄视口下栈序与宽视口逐项一致。
    expect(stackOrder(smaller)).toEqual(stackOrder(narrow));

    // 同一桥下行 viewport 在不同 window 尺寸下产出同一比例（DPR/窗口独立）。
    window.innerWidth = 320;
    window.innerHeight = 240;
    const afterResize = renderHud({ ...base, viewport: { width: 256, height: 720 } });
    expect(scaleOf(afterResize)).toBe(scaleOf(smaller));
  });

  it("零尺寸或非法视口安全降级为不呈现，且不产生布局残留", () => {
    expect(renderHud({ ...base, viewport: { width: 0, height: 0 } }).innerHTML).toBe("");
    expect(renderHud({ ...base, viewport: { width: 0, height: 720 } }).innerHTML).toBe("");
    expect(renderHud({ ...base, viewport: { width: 1280, height: 0 } }).innerHTML).toBe("");
    expect(renderHud({ ...base, viewport: { width: 8, height: 720 } }).innerHTML).toBe("");
  });
});

describe("饱和度归零抖动", () => {
  it("saturationZero 置位只把抖动类落在饥饿行，复位与未确认都不携带", () => {
    const steady = renderHud({ ...base, hunger: { value: 5, saturationZero: false } });
    const steadyHunger = steady.querySelector(".hud-status-group--hunger");
    expect(steadyHunger?.classList.contains("hud-status-group--saturation-zero")).toBe(false);

    const jittered = renderHud({ ...base, hunger: { value: 5, saturationZero: true } });
    const hunger = jittered.querySelector(".hud-status-group--hunger");
    expect(hunger?.classList.contains("hud-status-group--saturation-zero")).toBe(true);
    // 生命行与氧气行不参与抖动（Go 侧只平移饥饿 quad 的呈现原点）。
    expect(
      jittered
        .querySelector(".hud-status-group--health")
        ?.classList.contains("hud-status-group--saturation-zero"),
    ).toBe(false);
    expect(
      jittered
        .querySelector(".hud-status-row--oxygen")
        ?.classList.contains("hud-status-group--saturation-zero"),
    ).toBe(false);
    // 抖动不改解析：同一数值的逐格类序与稳态逐项一致。
    expect(cellSequence(hunger)).toEqual(cellSequence(steadyHunger));

    const unconfirmed = renderHud({ ...base, hunger: undefined });
    expect(
      unconfirmed
        .querySelector(".hud-status-group--hunger")
        ?.classList.contains("hud-status-group--saturation-zero"),
    ).toBe(false);
  });

  it("抖动偏移经令牌与单一比例表达（1 design px × --hud-scale，样式表层钉）", () => {
    expect(ruleBodies(readHudCss(), ".hud-status-group--saturation-zero")).toContain(
      "transform: translateY(calc(var(--hud-saturation-jitter) * var(--hud-scale)))",
    );
  });
});
