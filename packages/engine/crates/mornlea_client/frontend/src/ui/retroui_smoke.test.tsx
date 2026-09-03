// pixel-retroui 接入冒烟测试：直接渲染上游 `Button` 组件，断言其产出真实
// `<button>` 元素与文本。该测试同时钉住两件事——依赖树内模块可被 Vite/
// vitest 正常解析导入，且 pixel-retroui 与 React 19 渲染器兼容（上游若
// 声明或实际不兼容 React 19，此处会先于任何面板改造红掉）。
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { Button } from "pixel-retroui";

afterEach(cleanup);

it("pixel-retroui Button 渲染真实 button 元素与文本", () => {
  render(<Button>进入游戏</Button>);
  const button = screen.getByRole("button", { name: "进入游戏" });
  expect(button.tagName).toBe("BUTTON");
  expect(button).toHaveTextContent("进入游戏");
});
