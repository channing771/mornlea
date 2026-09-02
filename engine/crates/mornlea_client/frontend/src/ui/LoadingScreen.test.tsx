// 加载屏组件单测：比例钳制（clamp(loaded/total, 0, 1)、total<=0 与分节缺席
// 安全降级为 0）与计数行格式「区块 x / y」。进度比例经 `--loading-fill`
// CSS 变量下发轨道，这里直接钉轨道上的变量值。
import "@testing-library/jest-dom/vitest";
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LoadingScreen } from "./LoadingScreen";

afterEach(cleanup);

function fillValue(container: HTMLElement): string {
  const track = container.querySelector<HTMLElement>(".loading-progress");
  if (track === null) {
    throw new Error("加载屏未渲染进度轨道");
  }
  return track.style.getPropertyValue("--loading-fill");
}

describe("LoadingScreen", () => {
  it("中间进度按 loaded/total 换算并呈现计数行", () => {
    const { container, getByRole, getByText } = render(
      <LoadingScreen loading={{ loaded: 1122, total: 4489 }} />,
    );
    expect(fillValue(container)).toBe(String(1122 / 4489));
    expect(getByText("区块 1122 / 4489")).toBeTruthy();
    expect(getByRole("heading", { name: "正在生成世界…" })).toBeTruthy();
  });

  it("loaded>total 钳制到 1，loaded=0 呈现 0", () => {
    const over = render(<LoadingScreen loading={{ loaded: 5000, total: 4489 }} />);
    expect(fillValue(over.container)).toBe("1");
    cleanup();
    const empty = render(<LoadingScreen loading={{ loaded: 0, total: 4489 }} />);
    expect(fillValue(empty.container)).toBe("0");
  });

  it("total<=0 与分节缺席安全降级为 0%（轨道仍在、计数行只随分节出现）", () => {
    // total 下界由桥守卫保证 >=1；0/0 属组件契约的防御分支，不得除零。
    const zero = render(<LoadingScreen loading={{ loaded: 0, total: 0 }} />);
    expect(fillValue(zero.container)).toBe("0");
    expect(zero.getByText("区块 0 / 0")).toBeTruthy();
    cleanup();
    const absent = render(<LoadingScreen />);
    expect(fillValue(absent.container)).toBe("0");
    expect(absent.queryByText(/^区块 /)).toBeNull();
  });

  it("计数数字用 Go 下发整数原样呈现（不格式化千分位）", () => {
    const { getByText } = render(<LoadingScreen loading={{ loaded: 4489, total: 4489 }} />);
    expect(getByText("区块 4489 / 4489")).toBeTruthy();
  });
});
