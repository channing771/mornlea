// 视觉基线 harness 入口：按 `?fixture=<name>` 渲染注册表中的单个部件。
// 样式与生产同链路（tokens.css → ui.css → @font-face 像素字体），字体随
// harness 构建产物本地供给，不经 mornlea:// 也不产生任何网络请求。
import { createRoot } from "react-dom/client";
import "../src/tokens.css";
import "../src/ui/ui.css";
import "./visual.css";
import { resolveFixture } from "./fixtures";

const name = new URLSearchParams(window.location.search).get("fixture");
const element = resolveFixture(name);
const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("visual/index.html 缺少 #root 挂载点");
}
createRoot(rootElement).render(element);
