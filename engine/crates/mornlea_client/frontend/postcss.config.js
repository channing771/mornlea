// PostCSS 管线：vite 自动拾取本文件，vite.config.ts 不得重复声明 postcss
// 配置以免双源漂移。tailwindcss 先于 autoprefixer，保证前缀加在生成后的
// 工具类上。
export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
};
