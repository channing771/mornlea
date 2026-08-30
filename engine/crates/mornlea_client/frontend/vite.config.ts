import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// 构建输出必须确定：dist 由 `git diff --exit-code` 门禁守护入库一致性，
// 文件名因此不带内容 hash（固定命名），两次构建逐字节一致由
// `make frontend-check` 与专项验证共同保证。
export default defineConfig({
  base: "./",
  plugins: [react()],
  build: {
    outDir: "dist",
    assetsDir: "assets",
    rollupOptions: {
      output: {
        entryFileNames: "assets/[name].js",
        chunkFileNames: "assets/[name].js",
        assetFileNames: "assets/[name][extname]",
      },
    },
  },
  test: {
    environment: "jsdom",
  },
});
