// 快捷栏：固定九格贴条 + 选中格双层轮廓 + 物品 tile 占位/数量/耐久。
// 呈现数据只来自已确认的权威镜像（`HudHotbar`），镜像缺席即「尚未确认」，
// 此时贴条以占位形式保留构图空间（`--unconfirmed`），不产生任何像素。
import type { CSSProperties } from "react";
import type { HudSlot } from "../bridge/client";
import { DURABILITY_LOW_RATIO } from "./geometry";

export interface HotbarProps {
  /** 恰九格的权威镜像；缺席（未确认）时不呈现格内容但保留贴条空间。 */
  readonly slots: readonly HudSlot[] | undefined;
  readonly selectedIndex: number | undefined;
}

export function Hotbar({ slots, selectedIndex }: HotbarProps) {
  const confirmed = slots !== undefined && selectedIndex !== undefined;
  const cells = slots ?? [];
  return (
    <div className={confirmed ? "hud-hotbar" : "hud-hotbar hud-hotbar--unconfirmed"}>
      {cells.map((slot, index) => (
        <HotbarCell key={index} slot={slot} selected={index === selectedIndex} />
      ))}
    </div>
  );
}

function HotbarCell({ slot, selected }: { slot: HudSlot; selected: boolean }) {
  const className = selected ? "hud-slot hud-slot--selected" : "hud-slot";
  // 空格以 item=0 表达；数量只对多于一件的堆叠显示，耐久只对存在耐久上限
  // 且 `0 < ratio < 1` 的工具显示（与迁移前 `appendDurabilityBarScaled` 同口径，
  // 满耐久与单件堆叠不产生附加标记）。
  const showTile = slot.item !== 0;
  const showCount = slot.count > 1;
  const durability = slot.durability;
  const showDurability = durability !== undefined && durability > 0 && durability < 1;
  return (
    <div className={className}>
      {showTile ? <span className="hud-slot-tile" /> : null}
      {showDurability ? (
        <span className="hud-slot-durability">
          <span
            className={
              durability < DURABILITY_LOW_RATIO
                ? "hud-slot-durability-fill hud-slot-durability-fill--low"
                : "hud-slot-durability-fill"
            }
            style={{ "--hud-fill": durability } as CSSProperties}
          />
        </span>
      ) : null}
      {showCount ? <span className="hud-slot-count">{slot.count}</span> : null}
    </div>
  );
}
