// Tailwind 配置：为 pixel-retroui 组件类与后续像素组件提供工具类生成。
// `preflight` 必须保持关闭——tailwind 的全局 reset 会冲掉 tokens.css/ui.css
// 既有样式；retroui 的类名写死在其 dist 产物内，`content` 因此必须扫到
// node_modules 下的组件产物，否则组件样式对应的工具类不会进产物 CSS。
/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{ts,tsx}",
    "./node_modules/pixel-retroui/dist/**/*.{js,mjs}",
  ],
  corePlugins: {
    preflight: false,
  },
};
