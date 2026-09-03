// 状态行图标的内联 SVG 呈现：心形/鸡腿/气泡的剪影 mask 逐像素移植自本仓库
// HUD 图集 painter（`internal/render/hud/health.go` 的 `hotbarHeartPixel`/
// `hotbarBubblePixel` 与 `hunger.go` 的 `hotbarDrumstickPixel`），属本项目
// 原创像素资产，不临摹任何外部游戏图标——迁移只换呈现通道，不换美术来源。
//
// 呈现方式：同一份 16×16 mask 生成两条 path——silhouette（描边色）与
// 4 邻域侵蚀后的 interior（体色），与 painter 的 border/内部分色一致；
// 半格填充不用 clipPath，而是把 mask 与中线谓词求交再生成 path，避免逐实例
// 的 clip id 与层叠顺序问题。空/半/满三态共用同一轮廓，与 GPU 图集行为一致。
// 全部 path 在模块装载时算好（mask 是 16×16 常量网格），渲染路径零计算。
import type { CellFill } from "./geometry";

/** 16×16 像素网格：与 HUD 图集的 `hotbarTextureSize` 同一口径。 */
const GRID = 16;

type PixelMask = (x: number, y: number) => boolean;

function heartMask(x: number, y: number): boolean {
  // 左右镜像、上宽下尖：两枚圆叶各占两行后在 y=5 合流，底部逐行收尖。
  switch (y) {
    case 2:
      return (x >= 2 && x <= 5) || (x >= 10 && x <= 13);
    case 3:
    case 4:
      return (x >= 1 && x <= 6) || (x >= 9 && x <= 14);
    case 5:
      return x >= 1 && x <= 14;
    case 6:
    case 7:
      return x >= 2 && x <= 13;
    case 8:
      return x >= 3 && x <= 12;
    case 9:
      return x >= 4 && x <= 11;
    case 10:
      return x >= 5 && x <= 10;
    case 11:
      return x >= 6 && x <= 9;
    case 12:
      return x >= 7 && x <= 8;
    default:
      return false;
  }
}

function bubbleMask(x: number, y: number): boolean {
  // 满幅圆盘：每行左右界关于 x=7.5 镜像，短行把圆收圆。
  switch (y) {
    case 2:
      return x >= 5 && x <= 10;
    case 3:
      return x >= 3 && x <= 12;
    case 4:
    case 5:
      return x >= 2 && x <= 13;
    case 6:
    case 7:
    case 8:
    case 9:
      return x >= 1 && x <= 14;
    case 10:
    case 11:
      return x >= 2 && x <= 13;
    case 12:
      return x >= 3 && x <= 12;
    case 13:
      return x >= 5 && x <= 10;
    default:
      return false;
  }
}

function drumstickBonePixel(x: number, y: number): boolean {
  // 左上角斜向骨柄：±1 像素宽的对角线加一个圆骨节。
  if (x < 1 || y < 1 || x > 8 || y > 8) {
    return false;
  }
  const diff = x - y;
  if (diff >= -1 && diff <= 1) {
    return true;
  }
  const dx = x - 2;
  const dy = y - 2;
  return dx * dx + dy * dy <= 2;
}

function drumstickMeatPixel(x: number, y: number): boolean {
  // 右下角的肉：以 (10,10) 为心、半径约 4.7 的整数圆盘。
  const dx = x - 10;
  const dy = y - 10;
  return dx * dx + dy * dy <= 22;
}

function drumstickMask(x: number, y: number): boolean {
  return drumstickBonePixel(x, y) || drumstickMeatPixel(x, y);
}

/** 4 邻域侵蚀：与 painter 的 border 判定互补，得到轮廓内一圈以内的体色区。 */
function interiorOf(mask: PixelMask): PixelMask {
  return (x, y) =>
    mask(x, y) && mask(x - 1, y) && mask(x + 1, y) && mask(x, y - 1) && mask(x, y + 1);
}

/** mask → 单条 SVG path（逐行水平 run 合并成单位高度矩形）。 */
function pathFromMask(mask: PixelMask, limit?: PixelMask): string {
  let data = "";
  for (let y = 0; y < GRID; y += 1) {
    let x = 0;
    while (x < GRID) {
      if (!mask(x, y) || (limit !== undefined && !limit(x, y))) {
        x += 1;
        continue;
      }
      const start = x;
      while (x < GRID && mask(x, y) && (limit === undefined || limit(x, y))) {
        x += 1;
      }
      data += `M${start} ${y}h${x - start}v1h-${x - start}z`;
    }
  }
  return data;
}

const leftHalf: PixelMask = (x) => x < GRID / 2;
const rightHalf: PixelMask = (x) => x >= GRID / 2;

const heartInterior = interiorOf(heartMask);
const bubbleInterior = interiorOf(bubbleMask);
const drumstickInterior = interiorOf(drumstickMask);

/** 单层着色：一条 path 配一个令牌引用的填充色（icons.tsx 不出现裸色值）。 */
interface PaintedPath {
  readonly data: string;
  readonly fill: string;
}

// ---- 心形：空槽深剪影 + 满心红体 + 左上叶高光，半心只露左半 ----
const HEART_EMPTY: readonly PaintedPath[] = [
  { data: pathFromMask(heartMask), fill: "var(--hud-heart-empty-edge)" },
  { data: pathFromMask(heartInterior), fill: "var(--hud-heart-empty-face)" },
];
const HEART_SHINE: readonly PaintedPath[] = [
  {
    data: pathFromMask(heartInterior, (x, y) => x <= 4 && y >= 3 && y <= 4),
    fill: "var(--hud-heart-shine)",
  },
];
const HEART_FULL: readonly PaintedPath[] = [
  { data: pathFromMask(heartMask), fill: "var(--hud-heart-edge)" },
  { data: pathFromMask(heartInterior), fill: "var(--hud-heart-face)" },
  ...HEART_SHINE,
];
const HEART_LAYERS: Record<CellFill, readonly PaintedPath[]> = {
  empty: HEART_EMPTY,
  full: HEART_FULL,
  // 半心：painter 的 border 分支不按 x<8 判别，整圈轮廓都是满心描边色，
  // 只有体色按中线分成右暗左亮，高光落在左叶。
  half: [
    { data: pathFromMask(heartMask), fill: "var(--hud-heart-edge)" },
    { data: pathFromMask(heartInterior, rightHalf), fill: "var(--hud-heart-empty-face)" },
    { data: pathFromMask(heartInterior, leftHalf), fill: "var(--hud-heart-face)" },
    ...HEART_SHINE,
  ],
};

// ---- 鸡腿：斜骨柄 + 肉球 + 骨柄入肉处下方高光，半格只露右半 ----
const DRUMSTICK_EMPTY: readonly PaintedPath[] = [
  { data: pathFromMask(drumstickMask), fill: "var(--hud-hunger-empty-edge)" },
  { data: pathFromMask(drumstickInterior), fill: "var(--hud-hunger-empty-face)" },
];
const DRUMSTICK_FULL: readonly PaintedPath[] = [
  { data: pathFromMask(drumstickMask), fill: "var(--hud-hunger-edge)" },
  { data: pathFromMask(drumstickInterior, drumstickBonePixel), fill: "var(--hud-hunger-bone)" },
  {
    data: pathFromMask(drumstickInterior, (x, y) => !drumstickBonePixel(x, y)),
    fill: "var(--hud-hunger-face)",
  },
  {
    data: pathFromMask(
      drumstickInterior,
      (x, y) => !drumstickBonePixel(x, y) && x <= 8 && y >= 9 && y <= 11,
    ),
    fill: "var(--hud-hunger-shine)",
  },
];
const HUNGER_LAYERS: Record<CellFill, readonly PaintedPath[]> = {
  empty: DRUMSTICK_EMPTY,
  full: DRUMSTICK_FULL,
  // 半鸡腿：迁移前采样满格贴图的右半（U0 取中点、X 右移半格），因此描边、
  // 骨柄与肉都取右半，高光窗口 x<=8 在右半内只剩 x=8 一列（1×3 高光条）。
  half: [
    ...DRUMSTICK_EMPTY,
    { data: pathFromMask(drumstickMask, rightHalf), fill: "var(--hud-hunger-edge)" },
    {
      data: pathFromMask(drumstickInterior, (x, y) => drumstickBonePixel(x, y) && rightHalf(x, y)),
      fill: "var(--hud-hunger-bone)",
    },
    {
      data: pathFromMask(
        drumstickInterior,
        (x, y) => !drumstickBonePixel(x, y) && rightHalf(x, y),
      ),
      fill: "var(--hud-hunger-face)",
    },
    {
      data: pathFromMask(
        drumstickInterior,
        (x, y) => !drumstickBonePixel(x, y) && x <= 8 && y >= 9 && y <= 11,
      ),
      fill: "var(--hud-hunger-shine)",
    },
  ],
};

// ---- 气泡：空槽深剪影 + 满气泡青体 + 左上象限透气亮斑 ----
const BUBBLE_EMPTY: readonly PaintedPath[] = [
  { data: pathFromMask(bubbleMask), fill: "var(--hud-oxygen-empty-edge)" },
  { data: pathFromMask(bubbleInterior), fill: "var(--hud-oxygen-empty-face)" },
];
const BUBBLE_FULL: readonly PaintedPath[] = [
  { data: pathFromMask(bubbleMask), fill: "var(--hud-oxygen-empty-edge)" },
  { data: pathFromMask(bubbleInterior), fill: "var(--hud-oxygen-face)" },
  {
    data: pathFromMask(bubbleInterior, (x, y) => x <= 6 && y <= 7),
    fill: "var(--hud-oxygen-shine)",
  },
];
// 气泡没有半格语义（`resolveBubbleFill` 只产出 empty/full），half 兜底空槽
// 只为类型穷尽，不构成可到达的呈现分支。
const OXYGEN_LAYERS: Record<CellFill, readonly PaintedPath[]> = {
  empty: BUBBLE_EMPTY,
  full: BUBBLE_FULL,
  half: BUBBLE_EMPTY,
};

function PixelIcon({ layers }: { layers: readonly PaintedPath[] }) {
  return (
    <svg
      className="hud-icon"
      viewBox={`0 0 ${GRID} ${GRID}`}
      shapeRendering="crispEdges"
      aria-hidden="true"
      focusable="false"
    >
      {layers.map((layer, index) => (
        <path key={index} d={layer.data} fill={layer.fill} />
      ))}
    </svg>
  );
}

/** 心形状态格：empty/half/full 由 `resolveHeartFill` 按权威生命值解析。 */
export function HeartIcon({ fill }: { fill: CellFill }) {
  return <PixelIcon layers={HEART_LAYERS[fill]} />;
}

/** 鸡腿状态格：empty/half/full 由 `resolveHungerFill` 按权威饥饿值解析。 */
export function HungerIcon({ fill }: { fill: CellFill }) {
  return <PixelIcon layers={HUNGER_LAYERS[fill]} />;
}

/** 气泡状态格：empty/full 由 `resolveBubbleFill` 按权威氧气值解析。 */
export function OxygenIcon({ fill }: { fill: CellFill }) {
  return <PixelIcon layers={OXYGEN_LAYERS[fill]} />;
}
