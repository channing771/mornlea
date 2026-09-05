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
