// design 基准镜像断言：`src/hud/geometry.ts` 的常量必须与 `internal/render/hud`
// 的 Go 布局常量逐值一致（Go 侧是布局与命中几何的唯一权威）。任一侧漂移即红，
// 防止缩放分母与 CSS 令牌悄悄偏离 GPU 侧同源常量。
//
// 另含 `src/tokens.css` 的 `--hud-*` px 令牌 ↔ design 常量互钉：hud.css 与组件
// 只消费令牌、不 import 常量，令牌声明文本与常量的同值关系是「Go 布局权威 →
// design 基准 → CSS 呈现」链上唯一可能无声漂移的一环，这里读取原文逐值比对。
import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  CROSSHAIR_ARM_LENGTH,
  CROSSHAIR_ARM_THICKNESS,
  DESIGN_HEIGHT,
  DESIGN_WIDTH,
  DURABILITY_BAR_HEIGHT,
  DURABILITY_BAR_INSET,
  EDGE_MARGIN,
  HOTBAR_BOTTOM_MARGIN,
  HOTBAR_PANEL_PADDING,
  HOTBAR_ROW_WIDTH,
  HOTBAR_SLOT_GAP,
  HOTBAR_SLOT_SIZE,
  HOTBAR_SLOTS,
  ITEM_TILE_BORDER,
  ITEM_TILE_INSET,
  MARKER_LENGTH,
  MARKER_OFFSET,
  MARKER_THICKNESS,
  POPUP_ROW_HEIGHT,
  POPUP_TRACK_GAP,
  PROGRESS_CAP_WIDTH,
  PROGRESS_NOTCH_WIDTH,
  PROGRESS_TRACK_GAP,
  PROGRESS_TRACK_HEIGHT,
  PROGRESS_TRACK_WIDTH,
  SATURATION_ZERO_JITTER,
  SELECT_BORDER,
  SELECT_INSET,
  STATUS_BAR_GAP,
  STATUS_HOTBAR_GAP,
  STATUS_ICON_GAP,
  STATUS_ICON_SIZE,
  hudScale,
} from "./geometry";

describe("HUD design 基准镜像", () => {
  it("design 宽度 = 九格内容行宽 + 两侧贴条内边距（Go hudScale 宽度分母）", () => {
    expect(HOTBAR_ROW_WIDTH).toBe(HOTBAR_SLOTS * HOTBAR_SLOT_SIZE + (HOTBAR_SLOTS - 1) * HOTBAR_SLOT_GAP);
    expect(DESIGN_WIDTH).toBe(HOTBAR_ROW_WIDTH + 2 * HOTBAR_PANEL_PADDING);
    // 与 Go 常量逐值互钉（hotbarSlotSize/gap/panelPadding）。
    expect(HOTBAR_SLOT_SIZE).toBe(48);
    expect(HOTBAR_SLOT_GAP).toBe(4);
    expect(HOTBAR_PANEL_PADDING).toBe(6);
  });

  it("design 高度 = Go closedHUDHeight 的自下而上记账", () => {
    expect(DESIGN_HEIGHT).toBe(
      // hotbarBottomMargin + hotbarSlotSize + hotbarPanelPadding +
      // statusHotbarGap + 2*(statusBarGap+healthHeartSize) + miningBarGap +
      // miningBarHeight + popupTrackGap + popupRowHeight
      6 + HOTBAR_SLOT_SIZE + HOTBAR_PANEL_PADDING + STATUS_HOTBAR_GAP +
        2 * (STATUS_BAR_GAP + STATUS_ICON_SIZE) +
        PROGRESS_TRACK_GAP +
        PROGRESS_TRACK_HEIGHT +
        POPUP_TRACK_GAP +
        POPUP_ROW_HEIGHT,
    );
    expect(DESIGN_HEIGHT).toBe(160);
    expect(STATUS_HOTBAR_GAP).toBe(10);
    expect(STATUS_BAR_GAP).toBe(4);
    expect(STATUS_ICON_SIZE).toBe(16);
  });

  it("hudScale 镜像 Go 关闭态口径：单一比例、上限 1、扣两侧视口边距", () => {
    // 基准尺寸内不放大会回到 1。
    expect(hudScale({ width: 1280, height: 720 })).toBe(1);
    expect(hudScale({ width: 492, height: 176 })).toBe(1);
    // 宽度先触界：(492-16)/476 ≈ 1.0 → 高度界 (176-16)/160 = 1。
    expect(hudScale({ width: 400, height: 720 })).toBeCloseTo((400 - 16) / DESIGN_WIDTH, 12);
    expect(hudScale({ width: 1280, height: 120 })).toBeCloseTo((120 - 16) / DESIGN_HEIGHT, 12);
    // 不足一个像素的呈现空间降级为 0（零尺寸/非法视口由组件整体不呈现）。
    expect(hudScale({ width: 16, height: 720 })).toBe(0);
    expect(hudScale({ width: 0, height: 0 })).toBe(0);
  });
});

// ---- 令牌互钉：`src/tokens.css` 的 `--hud-*` px 令牌 ↔ design 常量 ----

/** px 令牌与 design 常量的逐值互钉表：每项 `[令牌名, 同值常量]`。覆盖三组——
 * 组件/几何已在消费的常量、只为互钉而保留导出的常量（选中双层轮廓、物品 tile、
 * 轨道标记与耐久条几何、准星与 marker 几何、氧气行间隙）、以及缩放分母。 */
const TOKEN_PINNING: readonly (readonly [string, number])[] = [
  // 快捷栏贴条与物品格。
  ["--hud-hotbar-slot-size", HOTBAR_SLOT_SIZE],
  ["--hud-hotbar-slot-gap", HOTBAR_SLOT_GAP],
  ["--hud-hotbar-padding", HOTBAR_PANEL_PADDING],
  ["--hud-hotbar-bottom-margin", HOTBAR_BOTTOM_MARGIN],
  ["--hud-select-border", SELECT_BORDER],
  ["--hud-select-inset", SELECT_INSET],
  ["--hud-item-tile-inset", ITEM_TILE_INSET],
  ["--hud-item-tile-border", ITEM_TILE_BORDER],
  ["--hud-hotbar-row-width", HOTBAR_ROW_WIDTH],
  // 状态栈。
  ["--hud-status-hotbar-gap", STATUS_HOTBAR_GAP],
  ["--hud-status-bar-gap", STATUS_BAR_GAP],
  ["--hud-status-icon-size", STATUS_ICON_SIZE],
  ["--hud-status-icon-gap", STATUS_ICON_GAP],
  ["--hud-edge-margin", EDGE_MARGIN],
  ["--hud-saturation-jitter", SATURATION_ZERO_JITTER],
  // 进度轨道、耐久条与弹条。
  ["--hud-progress-track-width", PROGRESS_TRACK_WIDTH],
  ["--hud-progress-track-height", PROGRESS_TRACK_HEIGHT],
  ["--hud-progress-track-gap", PROGRESS_TRACK_GAP],
  ["--hud-progress-cap-width", PROGRESS_CAP_WIDTH],
  ["--hud-progress-notch-width", PROGRESS_NOTCH_WIDTH],
  ["--hud-durability-height", DURABILITY_BAR_HEIGHT],
  ["--hud-durability-inset", DURABILITY_BAR_INSET],
  ["--hud-popup-track-gap", POPUP_TRACK_GAP],
  ["--hud-popup-row-height", POPUP_ROW_HEIGHT],
  // 准星与命中 marker。
  ["--hud-crosshair-arm-length", CROSSHAIR_ARM_LENGTH],
  ["--hud-crosshair-arm-thickness", CROSSHAIR_ARM_THICKNESS],
  ["--hud-marker-length", MARKER_LENGTH],
  ["--hud-marker-thickness", MARKER_THICKNESS],
  ["--hud-marker-offset", MARKER_OFFSET],
  // 缩放分母。
  ["--hud-design-width", DESIGN_WIDTH],
  ["--hud-design-height", DESIGN_HEIGHT],
];

/** 有 px 值但刻意不与 geometry 常量互钉的令牌：字号、聊天行高/内距/净空、贴条
 * 投影缘、凹槽斜面与双层文字/准星阴影偏移。它们是 hud.css 的呈现口径（部分在
 * Go 侧有同值常量，由 Go 侧测试自行钉值），不镜像进 geometry.ts，导出面不因
 * 互钉而膨胀。新增 `--hud-*` px 令牌必须归入两张表之一，否则完备性断言即红。 */
const TOKENS_WITHOUT_GEOMETRY_CONSTANT: readonly string[] = [
  "--hud-hotbar-rim",
  "--hud-slot-bevel",
  "--hud-digit-margin",
  "--hud-crosshair-shadow-offset",
  "--hud-text-shadow-offset",
  "--hud-chat-line-height",
  "--hud-chat-padding",
  "--hud-chat-clearance",
  "--hud-count-font-size",
  "--hud-popup-font-size",
  "--hud-chat-font-size",
];

/** 宽口径：所有以 px 结尾的 `--hud-*` 声明（含小数/负值/var() 等），只用来
 * 校验窄口径正则没有漏抓；互钉比对一律走窄口径的整数值。 */
const ANY_PX_DECLARATION = /(--hud-[a-z0-9-]+):\s*([^;{}]*px)\s*;/g;

/** 从 tokens.css 原文解析 `--hud-*` 的 px 声明值；解析口径刻意只识别**整数
 * px**（HUD design 基准全部是整数格），小数/负值/var() 会先被宽口径校验拦下
 * 报错，而不是被静默跳过造成漏钉。解析不出任何声明即视为文件结构漂移，
 * 同样直接红。vitest 的模块图里 `import.meta.url` 不是 file: URL，这里以
 * 运行根（frontend/，`pnpm test` 与 `make frontend-check` 的执行目录）解析
 * 同仓库源文件。 */
function readHudPxTokens(): Map<string, number> {
  const css = readFileSync(path.resolve(process.cwd(), "src/tokens.css"), "utf8");
  const tokens = new Map<string, number>();
  for (const match of css.matchAll(/(--hud-[a-z0-9-]+):\s*(\d+)px\s*;/g)) {
    const name = match[1];
    const value = match[2];
    if (name === undefined || value === undefined) {
      throw new Error(`tokens.css 声明解析失败：${match[0] ?? "(空匹配)"}`);
    }
    tokens.set(name, Number(value));
  }
  for (const match of css.matchAll(ANY_PX_DECLARATION)) {
    const name = match[1];
    const value = match[2]?.trim().replace(/px$/i, "");
    if (name === undefined || value === undefined) {
      continue;
    }
    if (!/^\d+$/.test(value)) {
      throw new Error(
        `tokens.css 令牌 ${name} 的 px 值「${value}」不在整数 px 解析口径内（HUD design 基准全为整数格），互钉断言不会覆盖它`,
      );
    }
  }
  return tokens;
}

describe("HUD 令牌与 design 常量互钉", () => {
  const tokens = readHudPxTokens();

  it("tokens.css 能解析出 px 令牌（原文结构漂移即红）", () => {
    expect(tokens.size).toBeGreaterThan(0);
  });

  it("px 令牌与 geometry 常量逐值同值（任一侧漂移即红）", () => {
    for (const [name, constant] of TOKEN_PINNING) {
      const declared = tokens.get(name);
      expect(declared, `tokens.css 缺少互钉令牌 ${name}`).not.toBeUndefined();
      expect(declared, `${name} 与 design 常量漂移`).toBe(constant);
    }
  });

  it("互钉表覆盖完备：每个 --hud-* px 令牌都归入两张表之一且不重复", () => {
    const pinned = TOKEN_PINNING.map(([name]) => name);
    const classified = new Set([...pinned, ...TOKENS_WITHOUT_GEOMETRY_CONSTANT]);
    const unclassified = [...tokens.keys()].filter((name) => !classified.has(name)).sort();
    expect(unclassified, `未归类的 --hud-* px 令牌：${unclassified.join("、")}`).toEqual([]);
    const stale = [...classified].filter((name) => !tokens.has(name)).sort();
    expect(stale, `互钉表引用了 tokens.css 已不存在的令牌：${stale.join("、")}`).toEqual([]);
    expect(new Set(pinned).size, "TOKEN_PINNING 内令牌重复登记").toBe(pinned.length);
  });
});
