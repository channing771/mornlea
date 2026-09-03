// 入口：装配桥（window.mornlea 注入点）并挂载 App。资产经 mornlea:// scheme
// 由 Rust 内嵌字节供给，本文件不引入任何网络资源。
import { createRoot } from "react-dom/client";
import { bridge, installMornleaGlobal } from "./bridge/client";
import "./tokens.css";
import { App } from "./ui/App";
import "./ui/ui.css";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("index.html 缺少 #root 挂载点");
}
installMornleaGlobal(bridge);
createRoot(rootElement).render(<App />);
