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
    ])("拒绝非法游戏事件 %j", event => expect(() => createEnvelope([event as UplinkEvent])).toThrow());
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
