// 视觉基线 harness 的独立构建配置：root 锁在 visual/，产物输出到
// `frontend/visual-dist/`（gitignored），绝不写 `dist/`、不碰其字节门禁。
// 样式管线与生产同链路：这里内联声明与 `postcss.config.js` 相同的插件序
// （tailwindcss 先于 autoprefixer），tailwind 配置显式指向仓库的
// `tailwind.config.js`（content 扫描 src/** 与 retroui 产物，与生产一致），
// 不依赖 cwd 或配置上溯解析的隐式行为；`postcss.config.js` 本身零改动，
// 生产构建不受影响。
import react from "@vitejs/plugin-react";
import autoprefixer from "autoprefixer";
import tailwindcss from "tailwindcss";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

export default defineConfig({
  root: fileURLToPath(new URL(".", import.meta.url)),
  base: "./",
  plugins: [react()],
  css: {
    postcss: {
      plugins: [
        tailwindcss({ config: fileURLToPath(new URL("../tailwind.config.js", import.meta.url)) }),
        autoprefixer(),
      ],
    },
  },
  build: {
    outDir: "../visual-dist",
    emptyOutDir: true,
    assetsDir: "assets",
    // 固定产物文件名（无内容 hash），两次构建逐字节一致，便于 diff 排查。
    rollupOptions: {
      output: {
        entryFileNames: "assets/[name].js",
        chunkFileNames: "assets/[name].js",
        assetFileNames: "assets/[name][extname]",
      },
    },
  },
});
