// 离线导出生产装配目录；失败立即终止，禁止回退到旧图片或占位符。
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
const output = fileURLToPath(new URL("./item-catalog.generated.json", import.meta.url));
const root = fileURLToPath(new URL("../../../../../../", import.meta.url));
execFileSync("go", ["test", "./packages/client/cmd/mornlea/app", "-run", "^TestExportUIVisualCatalog$", "-count=1"], {
  cwd: root,
  env: { ...process.env, MORNLEA_UI_VISUAL_CATALOG: output },
  stdio: "inherit",
});
