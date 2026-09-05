import type { GameState } from "../src/bridge/game";
import type { HudSlot } from "../src/bridge/client";
const empty = (n: number): HudSlot[] => Array.from({ length: n }, () => ({ item: 0, count: 0 }));
const stone = { item: 1, count: 12, name: "石头" };
export const gameFixture: GameState = { token: 1, kind: "inventory", cursorFree: true, confirmed: true, inventory: [stone, { item: 3, count: 8, name: "泥土" }, ...empty(34)], grid: [stone, stone, stone, stone, ...empty(5)], gridSize: 2, output: { item: 4, count: 4, name: "石砖" }, chest: [stone, ...empty(26)], furnace: [{ item: 6, count: 8, name: "粗铁" }, { item: 5, count: 16, name: "煤炭" }, { item: 7, count: 2, name: "铁锭" }], progress: .6, burn: .8, recipeIndex: -1, recipes: ["石砖", "熔炉", "铁块", "石镐", "铁镐", "箱子", "橡木木板", "光源方块", "石锄", "铁锄"].map(name => ({ name, size: 3, slots: empty(9), output: { item: 1, count: 1, name } })) };
