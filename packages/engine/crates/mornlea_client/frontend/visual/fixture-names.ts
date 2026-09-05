// fixture 名称清单单源：fixtures.tsx 经 `as const` 推导出字面量联合（注册表
// 键得到编译期互钉），visual.mjs 按严格格式提取本数组驱动截图与比对。格式
// 约定（visual.mjs 依赖，勿改结构）：`export const fixtureNames = [` 后每行
// 一个双引号 kebab-case 名称加逗号，闭于 `] as const;`。
export const fixtureNames = [
  "panel-inventory",
  "panel-inventory-empty",
  "panel-inventory-full",
  "items-all",
  "hud-hotbar-first",
  "hud-hotbar-last",
  "panel-workbench",
  "panel-chest",
  "panel-furnace",
  "panel-character",
  "panel-inventory-narrow",
  "panel-main-menu",
  "panel-settings",
  "panel-pause",
  "panel-loading",
  "panel-debug",
  "button-default",
  "button-disabled",
  "button-pressed",
  "input-text",
  "preset-group",
  "slider",
  "debug-rows",
  "error-line",
  "hud-hotbar",
  "hud-status",
  "hud-progress",
  "hud-popup-crosshair",
  "hud-chat",
  "hud-container-open",
] as const;

export type FixtureName = (typeof fixtureNames)[number];
