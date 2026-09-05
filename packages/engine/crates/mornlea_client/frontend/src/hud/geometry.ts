// HUD 呈现层 design 基准：与 `internal/render/hud` 的布局常量逐值同源
// （design px）。前端独占生产布局与语义命中，这里只承担两件事——
// 单一比例 `--hud-scale` 的缩放分母（`DESIGN_WIDTH`/`DESIGN_HEIGHT`），以及
// 状态格的语义解析（`resolve*Fill`，与 health.go/hunger.go/oxygen.go 的
// 逐槽判定逐项平移）。`src/hud/hud.css` 消费的、与 Go 布局常量对应的
// `--hud-*` px 令牌和这里的常量同值，两侧漂移由 geometry.test.ts 的镜像断言
// 与令牌互钉断言兜住（后者逐值比对 `src/tokens.css` 的声明文本）。
//
// 导出面刻意收敛：这里只保留「被组件消费、被 design 分母记账、或被令牌互钉
// 断言消费」的常量——互钉断言是 `--hud-*` 令牌值的真实消费方，组件自身的
// 尺寸全部经 CSS 令牌取得，不直接 import 这些数值。

/** 视口尺寸：桥下行 `hud.viewport` 是唯一权威窗口输入（窗口 `ContentSize` 的 CSS 逻辑像素口径），
 * 页面不读 `window` 尺寸参与缩放——resize 的呈现更新由 Go 随状态下行携带，
 * 高 DPI 下物理帧缓冲尺寸不会被误用为 CSS 布局尺寸。 */
export interface Viewport {
  readonly width: number;
  readonly height: number;
}

// ---- 快捷栏贴条（layout.go / panel.go） ----
export const HOTBAR_SLOTS = 9; // core.HotbarSlots
export const HOTBAR_SLOT_SIZE = 48; // hotbarSlotSize
export const HOTBAR_SLOT_GAP = 4; // hotbarSlotGap
export const HOTBAR_PANEL_PADDING = 6; // hotbarPanelPadding
export const HOTBAR_BOTTOM_MARGIN = 6; // hotbarBottomMargin（贴条下沿内边距）
export const SELECT_BORDER = 3; // hotbarSelectBorder（选中格外扩框）
export const SELECT_INSET = 3; // hotbarSelectInset（选中格内衬）
export const ITEM_TILE_INSET = 10; // hotbarSwatchInset（物品 tile 内缩）
export const ITEM_TILE_BORDER = 2; // hotbarSwatchBorder（物品 tile 暗边）
/** 贴条内容行宽：状态行与进食轨道的中轴锚点（panel.go `hotbarRowWidth`）。 */
export const HOTBAR_ROW_WIDTH = HOTBAR_SLOTS * HOTBAR_SLOT_SIZE + (HOTBAR_SLOTS - 1) * HOTBAR_SLOT_GAP;

// ---- 状态栈（layout.go / health.go） ----
export const STATUS_HOTBAR_GAP = 10; // statusHotbarGap（主行底与贴条外沿净空）
export const STATUS_BAR_GAP = 4; // statusBarGap（行堆叠行距）
export const STATUS_ICON_SIZE = 16; // healthHeartSize（心/鸡腿/气泡格尺寸）
export const STATUS_ICON_GAP = 1; // healthHeartGap（状态格间隙）
export const STATUS_SEGMENTS = 10; // healthSegmentCount / oxygenSegmentCount
export const EDGE_MARGIN = 8; // hudEdgeMargin（视口安全边距）

// ---- 进度轨道与弹条（layout.go / popup.go） ----
// 进食条沿用迁移前采掘轨道的几何常量（Go 布局常量名不变）；采掘条退役后
// 末端标记与警示缺口的常量已删，轨道仍占 design 基准内的同一行。
export const PROGRESS_TRACK_WIDTH = 240; // miningBarWidth
export const PROGRESS_TRACK_HEIGHT = 12; // miningBarHeight
export const PROGRESS_TRACK_GAP = 16; // miningBarGap
export const DURABILITY_BAR_HEIGHT = 3; // durabilityBarHeight
export const DURABILITY_BAR_INSET = 4; // durabilityBarInset
export const DURABILITY_LOW_RATIO = 0.25; // appendDurabilityBarScaled 的低耐久阈值
export const POPUP_TRACK_GAP = 6; // popupTrackGap
export const POPUP_ROW_HEIGHT = 16; // popupRowHeight
/** 饱和度归零抖动的垂直偏移（design px）：`hunger.go` 的 `y += scale` 分支，
 * 只动饥饿行的呈现原点，不改数值与格数；令牌为 `--hud-saturation-jitter`。 */
export const SATURATION_ZERO_JITTER = 1;

// ---- 准星与命中标记（crosshair.go / combat_marker.go） ----
export const CROSSHAIR_ARM_LENGTH = 11; // 横臂 11×3、竖臂 3×11
export const CROSSHAIR_ARM_THICKNESS = 3;
export const MARKER_LENGTH = 8; // 命中标记长臂 8、短边 2
export const MARKER_THICKNESS = 2;
/** 标记中心到准星中心的偏移：`appendCombatMarker` 的 `(4 + 8/2)`。 */
export const MARKER_OFFSET = 4 + MARKER_LENGTH / 2;

// ---- 权威域上界（与 `src/bridge/client.ts` 的钉值及 core 常量同源） ----
export const MAX_HEALTH = 20; // core.MaxHealth
export const MAX_HUNGER = 20; // core.MaxHunger
export const MAX_OXYGEN_TICKS = 300; // core.MaxOxygenTicks

/** 缩放宽度分母：Go `hudScale` 的 `hotbarContentWidth`（贴条含两侧内边距）。 */
export const DESIGN_WIDTH = HOTBAR_ROW_WIDTH + 2 * HOTBAR_PANEL_PADDING;
/** 缩放高度分母：Go `closedHUDHeight`（关闭态联合高度，弹条行纳入防裁剪）。 */
export const DESIGN_HEIGHT =
  HOTBAR_BOTTOM_MARGIN +
  HOTBAR_SLOT_SIZE +
  HOTBAR_PANEL_PADDING +
  STATUS_HOTBAR_GAP +
  2 * (STATUS_BAR_GAP + STATUS_ICON_SIZE) +
  PROGRESS_TRACK_GAP +
  PROGRESS_TRACK_HEIGHT +
  POPUP_TRACK_GAP +
  POPUP_ROW_HEIGHT;

/**
 * hudScale 镜像 Go 关闭态的 `hudScale(false, width, height)`：可用宽高各扣
 * 去两侧视口安全边距后对 design 基准取比例，上限 1（不放大道超过基准尺寸），
 * 下限 0。返回 0 表示视口不足以容纳任何呈现（含零尺寸/非法视口），调用方
 * 据此整体降级为不呈现。
 */
export function hudScale(viewport: Viewport): number {
  const availableWidth = viewport.width - 2 * EDGE_MARGIN;
  const availableHeight = viewport.height - 2 * EDGE_MARGIN;
  if (availableWidth <= 0 || availableHeight <= 0) {
    return 0;
  }
  return Math.min(availableWidth / DESIGN_WIDTH, availableHeight / DESIGN_HEIGHT, 1);
}

/** 状态格填充态。`half` 的朝向由图标语义决定：心形露左半、鸡腿露右半，
 * 与迁移前 GPU 图集的半格方向逐项一致（cells 的调用方不需要再区分）。 */
export type CellFill = "empty" | "half" | "full";

/**
 * resolveHeartFill 镜像 `appendHealthBar` 的逐槽判定：`value/2` 向下取整为
 * 满心数，奇数余量落在下一格的半心。调用方已保证 `value` 落在 `0..MAX_HEALTH`。
 */
export function resolveHeartFill(segment: number, value: number): CellFill {
  const full = Math.floor(value / 2);
  if (segment < full) {
    return "full";
  }
  if (segment === full && value % 2 !== 0) {
    return "half";
  }
  return "empty";
}

/**
 * resolveHungerFill 镜像 `appendHungerBar`：填充数 = `ceil(value/2)`，奇数值
 * 的最后一格只露鸡腿右半边。空槽是常驻刻度，`empty` 同样要呈现。
 */
export function resolveHungerFill(segment: number, value: number): CellFill {
  const filled = Math.ceil(value / 2);
  if (segment >= filled) {
    return "empty";
  }
  if (segment === filled - 1 && value % 2 !== 0) {
    return "half";
  }
  return "full";
}

/**
 * resolveBubbleFill 镜像 `appendOxygenBar`：满段数 = `ceil(value*10/max)`，
 * 其余为空气泡。氧气未耗损（满值）时整行不呈现，由 `StatusRow` 在调用前
 * 判定，这里不做满值短路。
 */
export function resolveBubbleFill(segment: number, value: number, max: number): CellFill {
  const filled = Math.ceil((value * STATUS_SEGMENTS) / max);
  return segment < filled ? "full" : "empty";
}
