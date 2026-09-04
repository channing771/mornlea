// 空槽淡印手绘线稿：九格各一枚 16×16 线稿衬底，有物品时隐藏（与 tile
// 互斥由 `Hotbar` 保证）。线形仿 `icons.tsx` 的 mask→path 内联 SVG 模式——
// 全部 path 在模块装载时由像素 mask 算好，渲染路径零计算；描边只走
// `currentColor`（实际取 `hud.css` 的 `.hud-slot-doodle` 指定的线稿令牌），
// 本文件零裸色值，不新增二进制资产。
const GRID = 16;

type PixelMask = (x: number, y: number) => boolean;

/** mask → 单条 SVG path（逐行水平 run 合并成单位高度矩形，与 `icons.tsx` 同口径）。 */
function pathFromMask(mask: PixelMask): string {
  let data = "";
  for (let y = 0; y < GRID; y += 1) {
    let x = 0;
    while (x < GRID) {
      if (!mask(x, y)) {
        x += 1;
        continue;
      }
      const start = x;
      while (x < GRID && mask(x, y)) {
        x += 1;
      }
      data += `M${start} ${y}h${x - start}v1h-${x - start}z`;
    }
  }
  return data;
}

/** 竖线段：`x` 列上 `y` 自 `from` 到 `to`（闭区间）的 1px 线。 */
function vertical(x: number, from: number, to: number): PixelMask {
  return (px, py) => px === x && py >= from && py <= to;
}

/** 横线段：`y` 行上 `x` 自 `from` 到 `to`（闭区间）的 1px 线。 */
function horizontal(y: number, from: number, to: number): PixelMask {
  return (px, py) => py === y && px >= from && px <= to;
}

/** 若干线稿 mask 的并集：把多笔 1px 线合成一枚衬底。 */
function union(...masks: readonly PixelMask[]): PixelMask {
  return (x, y) => masks.some((mask) => mask(x, y));
}

/** 圆环轮廓：中心 (`cx`,`cy`)、半径平方落在 [`lo`,`hi`] 的 1px 环带。 */
function ring(cx: number, cy: number, lo: number, hi: number): PixelMask {
  return (x, y) => {
    const d = (x - cx) * (x - cx) + (y - cy) * (y - cy);
    return d >= lo && d <= hi;
  };
}

/** 斜线段：自 (`x0`,`y0`) 沿 (`dx`,`dy`) 走 `steps` 步的 1px 对角线。 */
function diagonal(x0: number, y0: number, dx: number, dy: number, steps: number): PixelMask {
  return (x, y) => {
    for (let i = 0; i < steps; i += 1) {
      if (x === x0 + dx * i && y === y0 + dy * i) {
        return true;
      }
    }
    return false;
  };
}

// 0 格：小草芽——中央竖茎 + 左右两片叶弧 + 地面线。
const DOODLE_SPROUT: PixelMask = union(
  vertical(8, 6, 12),
  diagonal(8, 10, -1, -1, 4),
  diagonal(8, 11, 1, -1, 4),
  horizontal(13, 5, 11),
);

// 1 格：小花——花环 + 花心 + 短茎。
const DOODLE_FLOWER: PixelMask = union(
  ring(8, 6, 7, 12),
  (x, y) => x === 8 && y === 6,
  vertical(8, 10, 13),
  diagonal(8, 12, 1, -1, 3),
);

// 2 格：蘑菇——半圆帽轮廓 + 帽沿线 + 菌柄。
const DOODLE_MUSHROOM: PixelMask = union(
  (x, y) => {
    const d = (x - 8) * (x - 8) + (y - 8) * (y - 8);
    return y <= 8 && d >= 20 && d <= 30;
  },
  horizontal(8, 3, 13),
  vertical(6, 9, 12),
  vertical(10, 9, 12),
  horizontal(12, 6, 10),
);

// 3 格：叶子——菱形叶轮廓 + 中脉。
const DOODLE_LEAF: PixelMask = (x, y) => {
  if (y < 3 || y > 13) {
    return false;
  }
  const half = Math.round(4 * Math.sin((Math.PI * (y - 3)) / 10));
  return Math.abs(x - 8) === half || x === 8;
};

// 4 格：陶杯——杯口 + 两壁 + 杯底。
const DOODLE_CUP: PixelMask = union(
  horizontal(5, 5, 11),
  vertical(5, 5, 11),
  vertical(11, 5, 11),
  horizontal(11, 5, 11),
  horizontal(7, 4, 9),
);

// 5 格：三叶草——三枚小环 + 茎。
const DOODLE_CLOVER: PixelMask = union(
  ring(6, 6, 3, 6),
  ring(10, 6, 3, 6),
  ring(8, 9, 3, 6),
  vertical(8, 11, 13),
);

// 6 格：水滴——下圆环 + 顶部两笔斜线 + 底波。
const DOODLE_DROP: PixelMask = union(
  (x, y) => {
    const d = (x - 8) * (x - 8) + (y - 10) * (y - 10);
    return y >= 8 && d >= 7 && d <= 12;
  },
  diagonal(5, 9, 1, -1, 4),
  diagonal(11, 9, -1, -1, 4),
);

// 7 格：四角星——长竖 + 长横 + 短对角。
const DOODLE_SPARK: PixelMask = union(
  vertical(8, 3, 13),
  horizontal(8, 3, 13),
  diagonal(6, 6, 1, 1, 5),
  diagonal(10, 6, -1, 1, 5),
);

// 8 格：云纹——顶弧 + 两侧 + 底波浪。
const DOODLE_CLOUD: PixelMask = union(
  (x, y) => {
    const d = (x - 8) * (x - 8) + (y - 8) * (y - 8);
    return y <= 8 && d >= 10 && d <= 15;
  },
  vertical(4, 8, 10),
  vertical(12, 8, 10),
  (x, y) => y === 11 && x >= 4 && x <= 12 && (x + y) % 2 === 0,
  horizontal(10, 5, 11),
);

const DOODLE_MASKS: readonly PixelMask[] = [
  DOODLE_SPROUT,
  DOODLE_FLOWER,
  DOODLE_MUSHROOM,
  DOODLE_LEAF,
  DOODLE_CUP,
  DOODLE_CLOVER,
  DOODLE_DROP,
  DOODLE_SPARK,
  DOODLE_CLOUD,
];

/** 九枚线稿 path：模块装载时由 mask 算好，渲染路径只按格序号取用。 */
const DOODLE_PATHS: readonly string[] = DOODLE_MASKS.map(pathFromMask);

/** 空槽线稿衬底：`index` 为格序号（0..8），描边经 `currentColor` 取用方格的线稿色。 */
export function SlotDoodle({ index }: { readonly index: number }) {
  const safeIndex = ((index % DOODLE_PATHS.length) + DOODLE_PATHS.length) % DOODLE_PATHS.length;
  const data = DOODLE_PATHS[safeIndex] ?? "";
  return (
    <svg viewBox={`0 0 ${GRID} ${GRID}`} aria-hidden="true" focusable="false">
      <path d={data} fill="currentColor" />
    </svg>
  );
}
