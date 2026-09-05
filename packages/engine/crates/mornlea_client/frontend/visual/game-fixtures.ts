import type { GameState } from "../src/bridge/game";
import type { HudSlot } from "../src/bridge/client";
import catalog from "./item-catalog.generated.json";

export const allItems: readonly HudSlot[] = catalog.items;
export const emptySlots = (n: number): HudSlot[] => Array.from({ length: n }, () => ({ item: 0, count: 0 }));
export function itemSlot(item: number, count = 1): HudSlot {
  const metadata = allItems.find(slot => slot.item === item);
  if (!metadata) throw new Error(`生产目录缺少物品 ${item}`);
  return { ...metadata, count: Math.min(count, metadata.count) };
}
const stone = itemSlot(1, 12);
export const workbenchRecipe = catalog.recipes[8]!;
export const gameFixture: GameState = {
  token: 1, kind: "inventory", cursorFree: true, confirmed: true,
  inventory: [stone, itemSlot(2, 8), ...emptySlots(34)],
  source: {area: "inventory", index: 0},
  grid: [stone, stone, stone, stone, ...emptySlots(5)], gridSize: 2,
  output: itemSlot(4, 4), chest: [stone, ...emptySlots(26)],
  furnace: [itemSlot(6, 8), itemSlot(5, 16), itemSlot(7, 2)],
  progress: 0.6, burn: 0.8, recipeIndex: -1,
  recipes: catalog.recipes.map(recipe => ({...recipe, size: 3 as const})),
};
