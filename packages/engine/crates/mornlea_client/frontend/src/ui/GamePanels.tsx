import { useEffect, useRef, useState, type CSSProperties } from "react";
import type { HudSlot, HudState, UplinkEvent } from "../bridge/client";
import type { GameState, SlotArea, GameAction } from "../bridge/game";
import { SlotIcon } from "../hud/SlotIcon";
import "./game.css";
type Props = {
    readonly game: GameState;
    readonly hud?: HudState;
    readonly onEvent: (event: UplinkEvent) => void;
};
const titles = { none: "", inventory: "随身行囊", character: "旅人手记", workbench: "工作台", chest: "储物箱", furnace: "熔炉" };
const labels: Record<SlotArea, string> = { inventory: "背包", crafting: "合成", chest: "箱子", furnace: "熔炉" };
// Drag activation uses a squared pixel-distance threshold. Releasing below it
// preserves ordinary two-click semantics; crossing it alone reveals the ghost,
// so click and drag interactions do not steal each other.
const dragStartThresholdPixels = 3;
// An active drag retains the semantic source reference, a visual stack snapshot,
// and pointer position. Settlement always uses semantic references, while the
// snapshot serves only the ghost and authoritative refreshes still update panels.
type PanelDrag = {
    readonly from: { readonly area: SlotArea; readonly index: number };
    readonly stack: HudSlot;
    x: number;
    y: number;
};
export function GamePanels({ game, hud, onEvent }: Props) {
    const emit = (action: GameAction) => onEvent(action);
    const [drag, setDrag] = useState<PanelDrag | null>(null);
    // `dragRef` gives window listeners the latest active drag. `armedRef` holds
    // a press below threshold, and `suppressClickRef` consumes the click that
    // follows a completed drag.
    const dragRef = useRef<PanelDrag | null>(null);
    const armedRef = useRef<{ from: PanelDrag["from"]; stack: HudSlot; x: number; y: number } | null>(null);
    const suppressClickRef = useRef(false);
    // Listeners mount once and use a ref for the latest emitter and view token.
    const latestRef = useRef({ emit, token: game.token });
    useEffect(() => {
        latestRef.current = { emit, token: game.token };
    });
    useEffect(() => {
        const endDrag = () => {
            dragRef.current = null;
            armedRef.current = null;
            setDrag(null);
        };
        const onPointerMove = (event: PointerEvent) => {
            const armed = armedRef.current;
            if (armed) {
                const dx = event.clientX - armed.x;
                const dy = event.clientY - armed.y;
                if (dx * dx + dy * dy >= dragStartThresholdPixels * dragStartThresholdPixels) {
                    armedRef.current = null;
                    dragRef.current = { from: armed.from, stack: armed.stack, x: event.clientX, y: event.clientY };
                    setDrag(dragRef.current);
                }
                return;
            }
            if (dragRef.current) {
                dragRef.current = { ...dragRef.current, x: event.clientX, y: event.clientY };
                setDrag(dragRef.current);
            }
        };
        const onPointerDown = (event: PointerEvent) => {
            // A new press clears stale suppression; a non-primary press during a drag cancels it.
            suppressClickRef.current = false;
            if (event.button !== 0 && dragRef.current)
                endDrag();
        };
        const onPointerUp = (event: PointerEvent) => {
            const current = dragRef.current;
            if (!current) {
                // Releasing an unactivated press preserves two-click semantics for the following click.
                armedRef.current = null;
                return;
            }
            // Browsers emit a click after an active drag ends; consume it to avoid a false slot action.
            suppressClickRef.current = true;
            endDrag();
            const target = event.target instanceof Element ? event.target : null;
            const slot = target?.closest<HTMLElement>("[data-slot-area]") ?? null;
            if (slot) {
                const toArea = slot.dataset.slotArea as SlotArea;
                const toIndex = Number(slot.dataset.slotIndex);
                // Releasing on the source cancels without a message, matching same-slot two-click cancellation.
                if (toArea === current.from.area && toIndex === current.from.index)
                    return;
                latestRef.current.emit({
                    type: "game-action", token: latestRef.current.token, op: "dragMove",
                    fromArea: current.from.area, fromIndex: current.from.index, toArea, toIndex
                });
                return;
            }
            // Non-slot panel whitespace cancels; outside-panel background drops the whole stack.
            if (target?.closest(".game-panel"))
                return;
            latestRef.current.emit({ type: "game-action", token: latestRef.current.token, op: "drop", area: current.from.area, index: current.from.index });
        };
        const onKeyDown = (event: KeyboardEvent) => {
            // Escape cancels only the drag and does not set click suppression.
            // The pointer remains pressed, so its later release and click return
            // to ordinary two-click semantics, and keyboard clicks remain intact.
            // Only pointerup settlement sets suppression.
            if (event.key === "Escape" && dragRef.current)
                endDrag();
        };
        // System interruption or window focus loss may omit pointerup. Cancel so
        // the ghost cannot freeze and stale `dragRef` state cannot settle as
        // `dragMove` or `drop` during a later click. Neither path has a following
        // click, so neither sets suppression.
        const cancelDrag = () => {
            if (dragRef.current || armedRef.current)
                endDrag();
        };
        window.addEventListener("pointermove", onPointerMove);
        window.addEventListener("pointerdown", onPointerDown);
        window.addEventListener("pointerup", onPointerUp);
        window.addEventListener("pointercancel", cancelDrag);
        window.addEventListener("blur", cancelDrag);
        window.addEventListener("keydown", onKeyDown);
        return () => {
            window.removeEventListener("pointermove", onPointerMove);
            window.removeEventListener("pointerdown", onPointerDown);
            window.removeEventListener("pointerup", onPointerUp);
            window.removeEventListener("pointercancel", cancelDrag);
            window.removeEventListener("blur", cancelDrag);
            window.removeEventListener("keydown", onKeyDown);
        };
    }, []);
    const command = (op: "close" | "capture" | "inventory" | "character" | "take-output") => emit({ type: "game-action", token: game.token, op });
    if (game.kind === "none")
        return game.cursorFree ? <div className="game-free-cursor" onClick={() => command("capture")}>
        <p>Tab 或点击世界继续探索 · E 打开行囊</p>
        </div> : <p className="game-key-hint">Tab 自由光标 · E 行囊</p>;
    const personal = game.kind === "inventory" || game.kind === "character";
    const crafting = game.kind === "inventory" || game.kind === "workbench";
    // 槽位交互只透传语义字段（按键类型与 Shift 修饰位）：数量推导与落位
    // 全部由服务端权威完成，前端不自行计算分堆或快捷搬运结果。
    const slotAction = (area: SlotArea, index: number, button: "left" | "right", shift: boolean) => emit({ type: "game-action", token: game.token, op: "slot", area, index, button, shift });
    const slotButton = (area: SlotArea, slot: HudSlot, index: number) => <button key={index} type="button" data-slot-area={area} data-slot-index={index} className={`game-slot${area === "inventory" && index < 9 && hud?.hotbar?.selectedIndex === index ? " game-slot--selected" : ""}${drag?.from.area === area && drag.from.index === index ? " game-slot--drag-source" : ""}`} disabled={!game.confirmed} aria-label={`${labels[area]} ${index + 1}：${slot.name || "空"}`} aria-pressed={game.source?.area === area && game.source.index === index} onPointerDown={event => {
        // Only a primary press on a confirmed nonempty slot may arm a drag;
        // unconfirmed or empty slots leave later click semantics unchanged.
        if (event.button !== 0 || !game.confirmed || slot.item === 0)
            return;
        armedRef.current = { from: { area, index }, stack: slot, x: event.clientX, y: event.clientY };
    }} onClick={event => {
        if (suppressClickRef.current) {
            suppressClickRef.current = false;
            return;
        }
        slotAction(area, index, "left", event.shiftKey);
    }} onContextMenu={event => {
        // 右键在面板上只触发槽位语义操作，不弹出浏览器上下文菜单。
        event.preventDefault();
        slotAction(area, index, "right", event.shiftKey);
    }}>
    <SlotContents slot={slot}/>
    <span className="game-tooltip" role="tooltip">{slot.name || "空槽位"}{slot.count > 0 ? ` × ${slot.count}` : ""}{slot.durability !== undefined ? ` · 耐久 ${Math.round(slot.durability * 100)}%` : ""}</span>
    </button>;
    const grid = (area: SlotArea, slots: readonly HudSlot[], columns: number, offset = 0) => <div className="game-slot-grid" style={{ "--game-columns": columns } as CSSProperties}>{slots.map((slot, index) => slotButton(area, slot, index + offset))}</div>;
    // Render the drag ghost only for an active drag, outside recorded visual
    // baselines. It follows the pointer, ignores hit testing, and shows the stack snapshot.
    return <div className={`game-panel-overlay${drag ? " game-panel-overlay--dragging" : ""}`}>
    {drag && <div className="game-drag-ghost" style={{ left: drag.x, top: drag.y }} aria-hidden="true"><SlotContents slot={drag.stack}/></div>}
    {/* 容器级兜底：右键落在面板空白（页眉/页脚/留白）也只阻断原生菜单，
        不发射语义事件——槽位语义操作由槽位级 onContextMenu 独家承担。 */}
    <section className={`game-panel${crafting?"":" game-panel--compact"}`} role="dialog" aria-modal="true" aria-label={titles[game.kind]} onContextMenu={event => event.preventDefault()}>
  <header className="game-panel-header">
    <div>
    <p className="game-eyebrow">MORNLEA / 旅途日常</p>
    <h1>{titles[game.kind]}</h1>
    </div>
    <button type="button" className="game-close" aria-label="关闭面板" onClick={() => command("close")}>×</button>
    </header>
  {personal && <nav className="game-tabs" aria-label="个人面板">
        <button aria-pressed={game.kind === "inventory"} onClick={() => command("inventory")}>背包</button>
        <button aria-pressed={game.kind === "character"} onClick={() => command("character")}>人物</button>
        </nav>}
  {game.kind === "character" ? <CharacterSheet hud={hud}/> : <div className="game-panel-body">
        <div className="game-panel-main">
   {crafting && <section className="game-crafting">
            <div>
            <h2>{game.gridSize} × {game.gridSize} 合成</h2>{grid("crafting", game.grid.slice(0, game.gridSize ** 2), game.gridSize)}</div>
            <span className="game-crafting-arrow" aria-hidden="true">→</span>
            <div>
            <h2>产物</h2>
            <button className="game-slot game-output" disabled={!game.confirmed || game.output.item === 0} aria-label={`取出合成产物：${game.output.name || "空"}`} onClick={() => command("take-output")}>
            <SlotContents slot={game.output}/>
            </button>
            </div>
            </section>}
   {game.kind === "chest" && <section>
            <h2>箱内物品</h2>{grid("chest", game.chest, 9)}</section>}
   {game.kind === "furnace" && <section>

            <div className="game-furnace-flow">
            <div className="game-furnace-column">
            <span>原料</span>{slotButton("furnace", game.furnace[0]!, 0)}<div className="game-flame" role="progressbar" aria-label="余火" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(game.burn * 100)}>
            <svg viewBox="0 0 24 32" aria-hidden="true">
            <path className="game-flame-track" d="M12 1C17 10 7 11 16 17L20 10C27 26 20 31 12 31S-2 23 6 14C5 23 13 16 12 1Z"/>
            <path className="game-flame-fill" style={{ clipPath: `inset(${(1 - game.burn) * 100}% 0 0)` }} d="M12 1C17 10 7 11 16 17L20 10C27 26 20 31 12 31S-2 23 6 14C5 23 13 16 12 1Z"/>
            </svg>
            </div>{slotButton("furnace", game.furnace[1]!, 1)}<span>燃料</span>
            </div>
            <div className="game-furnace-arrow">
            <span aria-hidden="true">→</span>
            <div className="game-meter" role="progressbar" aria-label="熔炼" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(game.progress * 100)}>
            <span style={{ width: `${game.progress * 100}%` }}/>
            </div>
            <span>熔炼 {Math.round(game.progress * 100)}%</span>
            </div>
            <div className="game-furnace-column">{slotButton("furnace", game.furnace[2]!, 2)}<span>产物</span>
            </div>
            </div>
            </section>}
   <section className="game-inventory">
        <h2>随身物品</h2>{grid("inventory", game.inventory.slice(9), 9, 9)}<div className="game-inventory-hotbar">
        <h2>快捷栏 <span>1 — 9</span>
        </h2>{grid("inventory", game.inventory.slice(0, 9), 9)}</div>
        </section>
  </div>{crafting && <aside className="game-recipes">
            <h2>配方手册</h2>
            <div className="game-recipe-list">{game.recipes.map((recipe, index) => <button key={index} aria-pressed={game.recipeIndex === index} onClick={() => emit({ type: "game-action", token: game.token, op: "recipe", index })}>
                <SlotIcon slot={recipe.output}/>
                <span>{recipe.name}</span>
                </button>)}</div>{game.recipeIndex >= 0 && <div className="game-recipe-preview" aria-label="配方材料预览">
                <p>{game.recipes[game.recipeIndex]?.name} · 按图摆放材料</p>
                <div className="game-slot-grid" style={{ "--game-columns": 3 } as CSSProperties}>{game.recipes[game.recipeIndex]?.slots.map((slot, index) => <div className="game-slot game-preview-slot" key={index}>
                    <SlotIcon slot={slot}/>
                    </div>)}</div>
                </div>}</aside>}</div>}
  <footer className="game-panel-footer">{game.kind === "character" ? "" : game.confirmed ? "先选物品，再选目标位置；右键半组 / Shift+右键单件 / Shift+点击快速搬运" : "正在整理行囊…"}<span>E / Esc 关闭</span>
    </footer>
 </section>
    </div>;
}
function SlotContents({ slot }: {
    readonly slot: HudSlot;
}) {
    return <>
    <SlotIcon slot={slot}/>{slot.count > 1 && <span className="game-slot-count">{slot.count}</span>}{slot.durability !== undefined && <progress className="game-slot-durability" max={1} value={slot.durability}/>}</>;
}
function CharacterSheet({ hud }: {
    readonly hud?: HudState;
}) {
    return <div className="game-character">
    <svg role="img" aria-label="原创体素旅人肖像" viewBox="0 0 180 240">
    <path className="game-portrait-paper" d="M10 10h160v220H10z"/>
    <path className="game-portrait-shadow" d="M35 215h110v10H35z"/>
    <path className="game-portrait-skin" d="M64 30h52v56H64zM38 93h21v70H38zM121 93h21v70h-21z"/>
    <path className="game-portrait-hair" d="M59 25h62v28h-12V42H72v16H59z"/>
    <path className="game-portrait-shirt" d="M58 87h64v75H58zM33 87h25v30H33zM122 87h25v30h-25z"/>
    <path className="game-portrait-scarf" d="M64 86h52v13H64zM102 99h14v33h-14z"/>
    <path className="game-portrait-trousers" d="M59 162h27v47H59zM95 162h27v47H95z"/>
    <path className="game-portrait-hair" d="M55 205h32v12H55zM94 205h32v12H94zM75 61h6v6h-6zM101 61h6v6h-6z"/>
    <path className="game-portrait-scarf" d="M83 76h15v4H83z"/>
    </svg>
    <div>
    <p className="game-eyebrow">每一步，都是新的故事</p>
    <h2>体素旅人</h2>
    <p>沿着晨光，去发现新的风景。</p>
    <dl>
    <div>
    <dt>生命</dt>
    <dd>{hud?.health?.value ?? "—"} / 20</dd>
    </div>
    <div>
    <dt>饥饿</dt>
    <dd>{hud?.hunger?.value ?? "—"} / 20</dd>
    </div>
    </dl>
    </div>
    </div>;
}
