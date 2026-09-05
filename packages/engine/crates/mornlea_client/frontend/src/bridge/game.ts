import { BridgeProtocolError, parseHudSlot, type HudSlot } from "./client";
export type GameKind = "none" | "inventory" | "character" | "workbench" | "chest" | "furnace";
export type SlotArea = "inventory" | "crafting" | "chest" | "furnace";
export interface GameSlotRef {
    readonly area: SlotArea;
    readonly index: number;
}
export interface GameRecipe {
    readonly name: string;
    readonly size: 2 | 3;
    readonly slots: readonly HudSlot[];
    readonly output: HudSlot;
}
export interface GameState {
    readonly token: number;
    readonly kind: GameKind;
    readonly cursorFree: boolean;
    readonly confirmed: boolean;
    readonly inventory: readonly HudSlot[];
    readonly grid: readonly HudSlot[];
    readonly gridSize: 2 | 3;
    readonly output: HudSlot;
    readonly chest: readonly HudSlot[];
    readonly furnace: readonly HudSlot[];
    readonly progress: number;
    readonly burn: number;
    readonly recipes: readonly GameRecipe[];
    readonly recipeIndex: number;
    readonly source?: GameSlotRef;
}
export type GameAction = {
    readonly type: "game-action";
    readonly token: number;
} & ({
    readonly op: "close" | "capture" | "inventory" | "character" | "take-output";
} | {
    readonly op: "hotbar" | "recipe";
    readonly index: number;
} | {
    readonly op: "slot";
    readonly area: SlotArea;
    readonly index: number;
});
const limits: Record<SlotArea, number> = { inventory: 35, crafting: 8, chest: 26, furnace: 2 };
function fail(): never { throw new BridgeProtocolError("非法游戏视图语义"); }
function object(raw: unknown, keys: readonly string[], optional: readonly string[] = []): Record<string, unknown> {
    if (typeof raw !== "object" || raw === null || Array.isArray(raw))
        return fail();
    const value = raw as Record<string, unknown>;
    if (keys.some(k => !(k in value)) || Object.keys(value).some(k => !keys.includes(k) && !optional.includes(k)))
        return fail();
    return value;
}
function integer(v: unknown, min: number, max: number): number { if (typeof v !== "number" || !Number.isInteger(v) || v < min || v > max)
    return fail(); return v; }
function ratio(v: unknown): number { if (typeof v !== "number" || !Number.isFinite(v) || v < 0 || v > 1)
    return fail(); return v; }
function bool(v: unknown): boolean { if (typeof v !== "boolean")
    return fail(); return v; }
function size(v: unknown): 2 | 3 { if (v !== 2 && v !== 3)
    return fail(); return v; }
function slots(v: unknown, n: number): readonly HudSlot[] { if (!Array.isArray(v) || v.length !== n)
    return fail(); return v.map((s, i) => parseHudSlot(s, `game.slots[${i}]`)); }
function slotRef(raw: unknown): GameSlotRef { const r = object(raw, ["area", "index"]); if (typeof r.area !== "string" || !Object.hasOwn(limits, r.area))
    return fail(); const area = r.area as SlotArea; return { area, index: integer(r.index, 0, limits[area]) }; }
export function validateGameAction(raw: unknown): GameAction {
    const r = raw as Record<string, unknown>;
    if (!r || r.type !== "game-action")
        return fail();
    integer(r.token, 1, Number.MAX_SAFE_INTEGER);
    const keys = ["type", "token", "op"];
    switch (r.op) {
        case "slot":
            object(r, [...keys, "area", "index"]);
            slotRef({ area: r.area, index: r.index });
            break;
        case "hotbar":
        case "recipe":
            object(r, [...keys, "index"]);
            integer(r.index, 0, r.op === "hotbar" ? 8 : 9);
            break;
        case "close":
        case "capture":
        case "inventory":
        case "character":
        case "take-output":
            object(r, keys);
            break;
        default: return fail();
    }
    return raw as GameAction;
}
export function parseGame(raw: unknown): GameState {
    const r = object(raw, ["token", "kind", "cursorFree", "confirmed", "inventory", "grid", "gridSize", "output", "chest", "furnace", "progress", "burn", "recipes", "recipeIndex"], ["source"]);
    if (typeof r.kind !== "string" || !["none", "inventory", "character", "workbench", "chest", "furnace"].includes(r.kind))
        return fail();
    if (!Array.isArray(r.recipes) || r.recipes.length !== 10)
        return fail();
    const recipes = r.recipes.map(raw => { const recipe = object(raw, ["name", "size", "slots", "output"]); if (typeof recipe.name !== "string" || [...recipe.name].length > 64)
        return fail(); return { name: recipe.name, size: size(recipe.size), slots: slots(recipe.slots, 9), output: parseHudSlot(recipe.output, "game.recipe.output") }; });
    return { token: integer(r.token, 1, Number.MAX_SAFE_INTEGER), kind: r.kind as GameKind, cursorFree: bool(r.cursorFree), confirmed: bool(r.confirmed), inventory: slots(r.inventory, 36), grid: slots(r.grid, 9), gridSize: size(r.gridSize), output: parseHudSlot(r.output, "game.output"), chest: slots(r.chest, 27), furnace: slots(r.furnace, 3), progress: ratio(r.progress), burn: ratio(r.burn), recipes, recipeIndex: integer(r.recipeIndex, -1, 9), ...(r.source === undefined ? {} : { source: slotRef(r.source) }) };
}
