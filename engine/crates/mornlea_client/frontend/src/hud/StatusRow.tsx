// 状态行：生命/饥饿主行锚定快捷栏两缘，耗损氧气沿饥饿右缘向外堆叠。
// 生命起点对齐快捷栏左缘（`flex-start`），饥饿终点对齐右缘并自右向左排列
// （`row-reverse`），氧气行沿同一右缘从右向左堆叠——三者共用快捷栏内容行宽，
// 与迁移前 `statusBarBounds` 的共享锚点一致。
//
// 未确认/满值隐藏：生命与饥饿缺席（镜像未确认）时不产生任何格；氧气未确认
// 或满值时整行不产生气泡，但行容器恒在（固定行高），保证周边元素不随氧气
// 显隐跳动。行自身不绘制任何背景面板。
import type { HudHealth, HudHunger, HudOxygen } from "../bridge/client";
import {
  MAX_HEALTH,
  MAX_HUNGER,
  MAX_OXYGEN_TICKS,
  STATUS_SEGMENTS,
  resolveBubbleFill,
  resolveHeartFill,
  resolveHungerFill,
  type CellFill,
} from "./geometry";
import { HeartIcon, HungerIcon, OxygenIcon } from "./icons";

export interface StatusRowProps {
  readonly health: HudHealth | undefined;
  readonly hunger: HudHunger | undefined;
  readonly oxygen: HudOxygen | undefined;
  /** 容器打开态：行栈向快捷栏下方避让，主行先于氧气行（氧气向下堆叠）。 */
  readonly open: boolean;
}

export function StatusRow({ health, hunger, oxygen, open }: StatusRowProps) {
  const oxygenRow = (
    <div className="hud-status-row hud-status-row--oxygen">{oxygenCells(oxygen)}</div>
  );
  const primaryRow = (
    <div className="hud-status-row hud-status-row--primary">
      <div className="hud-status-group hud-status-group--health">{heartCells(health)}</div>
      <div className={hungerGroupClass(hunger)}>{hungerCells(hunger)}</div>
    </div>
  );
  return (
    <>
      {open ? (
        <>
          {primaryRow}
          {oxygenRow}
        </>
      ) : (
        <>
          {oxygenRow}
          {primaryRow}
        </>
      )}
    </>
  );
}

/** 饥饿行的类名：`saturationZero` 置位即追加抖动类（饥饿行整体下移 1 design px
 * × 比例，镜像 `hunger.go` 的呈现分支），未确认镜像与复位态都不携带该类。 */
function hungerGroupClass(hunger: HudHunger | undefined): string {
  const base = "hud-status-group hud-status-group--hunger";
  return hunger?.saturationZero === true ? `${base} hud-status-group--saturation-zero` : base;
}

/** 生命：十槽各自解析为空心/半心/满心；值先钳制到 `0..MAX_HEALTH`。 */
function heartCells(health: HudHealth | undefined) {
  if (health === undefined) {
    return null;
  }
  const value = Math.min(health.value, MAX_HEALTH);
  return sequence().map((segment) => {
    const fill = resolveHeartFill(segment, value);
    return (
      <span key={segment} className={`hud-cell hud-cell--heart-${fill}`}>
        <HeartIcon fill={fill} />
      </span>
    );
  });
}

/** 饥饿：十格常驻空槽 + `ceil(value/2)` 个填充，奇数值末格只露右半。 */
function hungerCells(hunger: HudHunger | undefined) {
  if (hunger === undefined) {
    return null;
  }
  const value = Math.min(hunger.value, MAX_HUNGER);
  return sequence().map((segment) => {
    const fill = resolveHungerFill(segment, value);
    return (
      <span key={segment} className={`hud-cell hud-cell--hunger-${fill}`}>
        <HungerIcon fill={fill} />
      </span>
    );
  });
}

/** 氧气：满氧/未确认整行不呈现，耗损时按向上取整分段解析十段气泡。 */
function oxygenCells(oxygen: HudOxygen | undefined) {
  if (oxygen === undefined || oxygen.value >= MAX_OXYGEN_TICKS) {
    return null;
  }
  const value = Math.min(oxygen.value, MAX_OXYGEN_TICKS);
  return sequence().map((segment) => {
    const fill: CellFill = resolveBubbleFill(segment, value, MAX_OXYGEN_TICKS);
    return (
      <span key={segment} className={`hud-cell hud-cell--bubble-${fill}`}>
        <OxygenIcon fill={fill} />
      </span>
    );
  });
}

/** 十个槽位的下标序列：与 `healthSegmentCount`/`oxygenSegmentCount` 同值。 */
function sequence(): readonly number[] {
  return Array.from({ length: STATUS_SEGMENTS }, (_, index) => index);
}
