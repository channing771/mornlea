import Ajv2020 from "ajv/dist/2020";
import type { SchemaObject } from "ajv";
import bridgeSchema from "./schema.json";
import { parseGame, type GameState } from "./game";
import { describe, it, expect } from "vitest";
import { createEnvelope, type UplinkEvent } from "./client";
describe("游戏桥拒绝边界", () => {
    it.each([
        { type: "game-action", token: 1, op: "slot", area: "inventory", index: 36 },
        { type: "game-action", token: 1, op: "slot", area: "output", index: 0 },
        { type: "game-action", token: 0, op: "close" },
        { type: "game-action", token: 1, op: "close", index: 0 },
        // 旧字段集（缺 button/shift）被新 schema 拒绝属预期，前端与 Go 同批发布。
        { type: "game-action", token: 1, op: "slot", area: "inventory", index: 0 },
        { type: "game-action", token: 1, op: "slot", area: "inventory", index: 0, button: "left" },
        { type: "game-action", token: 1, op: "slot", area: "inventory", index: 0, shift: false },
        { type: "game-action", token: 1, op: "slot", area: "inventory", index: 0, button: "middle", shift: false },
        { type: "game-action", token: 1, op: "slot", area: "inventory", index: 0, button: "left", shift: "false" },
        // Drops reject missing fields, unknown areas, and out-of-range indices.
        { type: "game-action", token: 1, op: "drop", area: "inventory" },
        { type: "game-action", token: 1, op: "drop", area: "output", index: 0 },
        { type: "game-action", token: 1, op: "drop", area: "inventory", index: 36 },
        { type: "game-action", token: 1, op: "drop", area: "inventory", index: 0, button: "left" },
        // Drag moves reject missing endpoints, unknown areas, and area-specific index overflow.
        { type: "game-action", token: 1, op: "dragMove", fromArea: "inventory", fromIndex: 0, toArea: "inventory" },
        { type: "game-action", token: 1, op: "dragMove", fromArea: "output", fromIndex: 0, toArea: "inventory", toIndex: 0 },
        { type: "game-action", token: 1, op: "dragMove", fromArea: "inventory", fromIndex: 36, toArea: "inventory", toIndex: 0 },
        { type: "game-action", token: 1, op: "dragMove", fromArea: "inventory", fromIndex: 0, toArea: "chest", toIndex: 27 },
    ])("拒绝非法游戏事件 %j", event => expect(() => createEnvelope([event as UplinkEvent])).toThrow());
    it.each([
        { type: "game-action", token: 1, op: "slot", area: "inventory", index: 0, button: "left", shift: false },
        { type: "game-action", token: 1, op: "slot", area: "furnace", index: 2, button: "right", shift: true },
        { type: "game-action", token: 1, op: "drop", area: "chest", index: 26 },
        { type: "game-action", token: 1, op: "dragMove", fromArea: "inventory", fromIndex: 35, toArea: "crafting", toIndex: 8 },
    ])("携带按键与修饰位的槽位事件通过 %j", event => expect(() => createEnvelope([event as UplinkEvent])).not.toThrow());
});

const emptySlots = (count: number) => Array.from({ length: count }, () => ({ item: 0, count: 0 }));
const gameState: GameState = {
    token: 1,
    kind: "inventory",
    cursorFree: true,
    confirmed: true,
    inventory: emptySlots(36),
    grid: emptySlots(9),
    gridSize: 2,
    output: { item: 0, count: 0 },
    chest: emptySlots(27),
    furnace: emptySlots(3),
    progress: 0,
    burn: 0,
    recipeIndex: -1,
    recipes: Array.from({ length: 10 }, () => ({
        name: "配方",
        size: 3,
        slots: emptySlots(9),
        output: { item: 1, count: 1 },
    })),
};
const ajv = new Ajv2020({ strict: true });
ajv.compile(bridgeSchema as unknown as SchemaObject);
const validateGame = ajv.compile({ $ref: "mornlea://bridge/schema.json#/$defs/gameState" });
const malformedKinds = [
    { kind: ["inventory"] },
    { kind: ["none"] },
    { kind: [["inventory"]] },
    { kind: null },
    { kind: 0 },
    { kind: true },
    { kind: {} },
];

describe("游戏视图类型保持严格字符串枚举", () => {
    it("合法状态在 schema 与解析器均通过", () => {
        expect(validateGame(gameState)).toBe(true);
        expect(parseGame(gameState).kind).toBe("inventory");
    });
    it.each(malformedKinds)("schema 拒绝非字符串 kind：%j", ({ kind }) => {
        expect(validateGame({ ...gameState, kind })).toBe(false);
    });
    it.each(malformedKinds)("解析器拒绝非字符串 kind：%j", ({ kind }) => {
        expect(() => parseGame({ ...gameState, kind })).toThrow();
    });
});
