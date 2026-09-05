import type { HudSlot } from "../bridge/client";
/** `icon` 来自 Go 装配缓存；同一出口供快捷栏、背包和材料预览共享。 */
export function SlotIcon({ slot }: {
    readonly slot: HudSlot;
}) {
    if (slot.item === 0)
        return null;
    return slot.icon ? <img className="game-item-icon" src={slot.icon} alt="" draggable={false}/> : <span className="game-item-placeholder" aria-hidden="true">{slot.name?.slice(0, 1) || "◆"}</span>;
}
