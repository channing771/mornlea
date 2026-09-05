// 快捷栏：Tiny Glade 粉彩手绘风九格贴条 + 选中暖橙外框 + 空槽线稿衬底。
// 呈现数据只来自已确认的权威镜像（`HudHotbar`），镜像缺席即「尚未确认」，
// 此时贴条以占位形式保留构图空间（`--unconfirmed`），不产生任何像素。
// 贴条透明悬浮（无深色底带）；每格面色由 `data-index` 逐格钉到粉彩表面令牌；
// 空槽（item=0）渲淡印线稿衬底，有物品时隐藏（与 tile 互斥）。
import type { CSSProperties } from "react";
import type { HudSlot } from "../bridge/client";
import { DURABILITY_LOW_RATIO } from "./geometry";
import { SlotIcon } from "./SlotIcon";
import { SlotDoodle } from "./slotDoodles";

export interface HotbarProps {
  /** 恰九格的权威镜像；缺席（未确认）时不呈现格内容但保留贴条空间。 */
  readonly slots: readonly HudSlot[] | undefined;
  readonly selectedIndex: number | undefined;
 readonly onSelect?: (index:number)=>void;
}

export function Hotbar({ slots, selectedIndex, onSelect }: HotbarProps) {
  const confirmed = slots !== undefined && selectedIndex !== undefined;
  const cells = slots ?? [];
  return (
    <div className={confirmed ? "hud-hotbar" : "hud-hotbar hud-hotbar--unconfirmed"}>
      {cells.map((slot, index) => (
        <button type="button" className="hud-slot-hit" key={index} disabled={!onSelect} aria-label={`快捷栏 ${index+1}：${slot.name||"空"}`} aria-pressed={index===selectedIndex} onClick={()=>onSelect?.(index)}><HotbarCell index={index} slot={slot} selected={index === selectedIndex} neighbor={selectedIndex!==undefined&&Math.abs(index-selectedIndex)===1}/></button>
      ))}
    </div>
  );
}

function HotbarCell({ index, slot, selected, neighbor }: { index: number; slot: HudSlot; selected: boolean; neighbor:boolean }) {
  const className = selected ? "hud-slot hud-slot--selected" : neighbor ? "hud-slot hud-slot--neighbor" : "hud-slot";
  // 空格以 item=0 表达；数量只对多于一件的堆叠显示，耐久只对存在耐久上限
  // 且 `0 < ratio < 1` 的工具显示（与迁移前 `appendDurabilityBarScaled` 同口径，
  // 满耐久与单件堆叠不产生附加标记）。
  const showTile = slot.item !== 0;
  const showCount = slot.count > 1;
  const durability = slot.durability;
  const showDurability = durability !== undefined && durability > 0 && durability < 1;
  return (
    <div className={className} data-index={index}>
      {showTile ? <span className="hud-slot-tile"><SlotIcon slot={slot}/></span> : null}
      {showTile ? null : (
        <span className="hud-slot-doodle">
          <SlotDoodle index={index} />
        </span>
      )}
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
