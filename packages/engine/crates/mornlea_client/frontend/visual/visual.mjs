// UI 部件视觉基线管线（自包含脚本）：构建 visual harness → 本机静态服务 →
// Chrome headless 逐 fixture 截图 → pngjs 双阈值比对（check）或覆盖基线
// （update）。比对口径与 `cmd/mornlea/capture/visual_compare.go` 对齐：
// 任一像素任一通道差上限 2，且差异像素（任一通道差 ≥ 1）占比上限 0.0001，
// 超限即判漂移并把红标差异图写入 build/visual-ui/ 供人定位。
//
// 边界：本机开发工具，不进 CI 门禁；零网络（只访问本机回环的临时静态服务，
// 资产全部来自 visual-dist 构建产物）；缺基线不自动创建——check 报错列出，
// 只有显式 update 才写 golden（人工确认纪律）。
//
// 用法（在 frontend/ 内，或经 Make 目标）：
//   corepack pnpm visual-check    # 与 golden 比对，漂移/缺基线即非零退出
//   corepack pnpm visual-update   # 截图覆盖 testdata/visual-golden/ui/*.png（人工确认后用）
import { spawn } from "node:child_process";
import fs from "node:fs";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { PNG } from "pngjs";

const visualDir = path.dirname(fileURLToPath(import.meta.url));
const frontendDir = path.resolve(visualDir, "..");
// frontend 位于 packages/engine/crates/mornlea_client/frontend，向上五级即仓库根。
const repoRoot = path.resolve(frontendDir, "..", "..", "..", "..", "..");
const visualDistDir = path.join(frontendDir, "visual-dist");
const goldenDir = path.join(repoRoot, "testdata", "visual-golden", "ui");
// 候选实拍图与差异图统一留在仓库根 build/visual-ui/（根 /build/ 已 gitignored）。
const outDir = path.join(repoRoot, "build", "visual-ui");

// fixture 名称清单以 fixture-names.ts 的 `as const` 数组为单源（fixtures.tsx
// 经字面量联合获得编译期互钉），本脚本按该文件的严格格式约定提取数组字面量
// 驱动截图与比对；格式偏离立即报错，绝不静默漏拍。
const fixtureNamesPath = path.join(visualDir, "fixture-names.ts");
const namesMatch = fs
  .readFileSync(fixtureNamesPath, "utf8")
  .match(/export const fixtureNames = \[\n((?:  "[a-z0-9-]+",\n)+)\] as const;/);
if (namesMatch === null) {
  throw new Error(
    `fixture-names.ts 不符合格式约定（export const fixtureNames = [ 每行一个双引号 kebab-case 名称加逗号 ] as const;）：${fixtureNamesPath}`,
  );
}
// 去掉末项尾逗号后按 JSON 解析（数组字面量刻意保持 JSON 兼容写法）。
const fixtureNames = JSON.parse(`[\n${namesMatch[1].replace(/,\n$/, "\n")}]`);
if (
  !Array.isArray(fixtureNames) ||
  fixtureNames.length === 0 ||
  fixtureNames.some((name) => typeof name !== "string" || !/^[a-z0-9-]+$/.test(name))
) {
  throw new Error(`fixture 清单必须是非空的 kebab-case 名称数组：${fixtureNamesPath}`);
}

const VIEW_WIDTH = 1280;
const VIEW_HEIGHT = 720;
// 双阈值数值与 `cmd/mornlea/capture/capture_image.go` 的 captureThresholds
// 同值同义：通道差容忍 sRGB 编解码与光栅化的个位数 LSB 漂移；差异像素占比
// 上限挡住成片的布局/材质漂移。不要凭直觉放宽——放宽等于放弃门禁。
const MAX_CHANNEL_DELTA = 2;
const MAX_DIFF_PIXEL_RATIO = 0.0001;
// 单张截图的墙上时钟上限：virtual-time-budget 3s 已足够字体与首帧稳定，
// 正常一两秒内返回，超时按失败处理并强杀 Chrome。
const SCREENSHOT_TIMEOUT_MS = 60_000;

// Chrome 路径解析：env `CHROME_BIN` 优先，其次 macOS 默认安装路径；两者皆缺
// 时给出稳定的中文错误（不猜测、不下载、不触网）。
const DEFAULT_CHROME_PATH = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";

function resolveChromeBin() {
  const fromEnv = process.env.CHROME_BIN?.trim();
  if (fromEnv !== undefined && fromEnv !== "") {
    return fromEnv;
  }
  if (fs.existsSync(DEFAULT_CHROME_PATH)) {
    return DEFAULT_CHROME_PATH;
  }
  throw new Error(
    `未找到 Google Chrome：请设置环境变量 CHROME_BIN 指向 Chrome 可执行文件（默认路径 ${DEFAULT_CHROME_PATH} 不存在）`,
  );
}

// runViteBuild 先跑 visual harness 的独立构建（visual-dist），保证 check 侧
// 截图永远对着当前源码而不是上一次的陈旧产物；构建失败即整条管线失败。
function runViteBuild() {
  const viteBin = path.join(frontendDir, "node_modules", "vite", "bin", "vite.js");
  if (!fs.existsSync(viteBin)) {
    throw new Error(`未找到 vite CLI（先运行 corepack pnpm install --frozen-lockfile）：${viteBin}`);
  }
  return new Promise((resolve, reject) => {
    const child = spawn(
      process.execPath,
      [viteBin, "build", "--config", path.join(visualDir, "visual.vite.config.ts")],
      { cwd: frontendDir, stdio: "inherit" },
    );
    child.on("error", reject);
    child.on("close", (code) => {
      if (code === 0) {
        resolve();
      } else {
        reject(new Error(`visual harness 构建失败（vite 退出码 ${code}）`));
      }
    });
  });
}

// startStaticServer 起一个只读静态文件服务：127.0.0.1 随机端口（并发安全），
// 请求路径限制在 visual-dist 内。管线对网络的全部需求仅此而已。
const MIME_TYPES = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".woff2": "font/woff2",
  ".png": "image/png",
};

function startStaticServer(rootDir) {
  return new Promise((resolveServer, rejectServer) => {
    const server = http.createServer((req, res) => {
      const url = new URL(req.url ?? "/", "http://127.0.0.1");
      let pathname;
      try {
        pathname = decodeURIComponent(url.pathname);
      } catch {
        res.writeHead(400);
        res.end("bad request");
        return;
      }
      const filePath = path.resolve(rootDir, pathname.replace(/^\/+/, ""));
      if (filePath !== rootDir && !filePath.startsWith(rootDir + path.sep)) {
        res.writeHead(403);
        res.end("forbidden");
        return;
      }
      fs.readFile(filePath, (err, data) => {
        if (err) {
          res.writeHead(404);
          res.end("not found");
          return;
        }
        res.writeHead(200, {
          "Content-Type": MIME_TYPES[path.extname(filePath).toLowerCase()] ?? "application/octet-stream",
        });
        res.end(data);
      });
    });
    server.on("error", rejectServer);
    server.listen(0, "127.0.0.1", () => {
      resolveServer({ server, port: server.address().port });
    });
  });
}

// captureScreenshot 对单个 fixture URL 调一次 Chrome headless CLI 截图。
// 参数口径：固定窗口 1280x720、强制 1x 缩放与 sRGB 色彩描述文件、隐藏滚动条，
// `--virtual-time-budget` 快进虚拟时间让字体加载与首帧动画收敛后再截图；
// 独立临时 user-data-dir 隔离配置，`--no-proxy-server` 与背景联网/组件更新
// 禁用共同压住 Chrome 自身的网络副作用。
//
// 退出策略：实测 Chrome 151（macOS）写完截图后主进程挂在退出路径上不退出
// （与 virtual-time-budget 无关），因此不依赖进程自愿退出——截图文件连续
// 多次轮询尺寸不变即认定写盘完成，主动结束整个进程组（detached 进程组 +
// SIGTERM → SIGKILL）；进程正常退出的机器/版本仍走 close 分支。截图完整性
// 由调用方用 pngjs 解码校验兜底（截断的 PNG 解码必失败）。
const STABLE_POLLS = 3;
const STABLE_POLL_INTERVAL_MS = 250;
const KILL_GRACE_MS = 2000;

// removeDirSafe 清理临时 profile 目录：失败不阻断管线（残留在系统临时目录，
// 由系统清理策略兜底）。
function removeDirSafe(dir) {
  try {
    fs.rmSync(dir, { recursive: true, force: true });
  } catch {
    // 留给系统临时目录清理。
  }
}

function captureScreenshot({ chromeBin, url, destPath, profileDir }) {
  // Chrome 先写进 profile 私有的 shot.png，写稳后再发布到 outDir 的候选位，
  // 共享目录里永远不会出现半截文件。
  const shotPath = path.join(profileDir, "shot.png");
  return new Promise((resolve, reject) => {
    const args = [
      "--headless=new",
      `--screenshot=${shotPath}`,
      `--window-size=${VIEW_WIDTH},${VIEW_HEIGHT}`,
      "--force-device-scale-factor=1",
      "--force-color-profile=srgb",
      "--hide-scrollbars",
      "--virtual-time-budget=3000",
      "--no-first-run",
      "--no-default-browser-check",
      "--no-proxy-server",
      "--disable-background-networking",
      "--disable-component-update",
      `--user-data-dir=${profileDir}`,
      url,
    ];
    // detached 让 Chrome 及其 Helper 同属一个进程组，超时/完成时可整组结束，
    // 不给用户桌面留下孤儿 Helper。
    const child = spawn(chromeBin, args, { detached: true, stdio: ["ignore", "ignore", "pipe"] });
    let stderrTail = "";
    let settled = false;
    let lastSize = -1;
    let stableCount = 0;
    let stabilityTimer = null;
    let killTimer = null;
    const killTree = (signal) => {
      if (child.pid === undefined) {
        return;
      }
      try {
        process.kill(-child.pid, signal);
      } catch {
        // 进程组已退出，无需处理。
      }
    };
    const finish = (error) => {
      if (settled) {
        return;
      }
      settled = true;
      if (stabilityTimer !== null) {
        clearInterval(stabilityTimer);
      }
      clearTimeout(hardTimer);
      // 注意：这里刻意不取消 killTimer——SIGKILL 兜底必须在 resolve 之后仍能
      // 落到挂死在退出路径上的 Chrome，否则它会占着 profile 锁毒化后续截图。
      if (error !== undefined) {
        killTree("SIGKILL");
        removeDirSafe(profileDir);
        reject(error);
      } else {
        resolve();
      }
    };
    child.stderr?.on("data", (chunk) => {
      stderrTail = (stderrTail + chunk).slice(-2000);
    });
    child.on("error", (error) => {
      finish(error);
    });
    child.on("close", () => {
      // 先消费 shot.png 再清理 profile 目录：截图就落在 profileDir 内，
      // 顺序颠倒会让正常退出型 Chrome（Linux 或上游修复挂起后）每张必报失败。
      if (fs.existsSync(shotPath)) {
        fs.copyFileSync(shotPath, destPath);
        finish();
      } else {
        finish(new Error(`Chrome 截图失败：${url}${stderrTail === "" ? "" : `\n${stderrTail}`}`));
      }
      removeDirSafe(profileDir);
    });
    const hardTimer = setTimeout(() => {
      finish(
        new Error(
          `Chrome 截图超时（${SCREENSHOT_TIMEOUT_MS}ms 内未产出稳定截图）：${url}${stderrTail === "" ? "" : `\n${stderrTail}`}`,
        ),
      );
    }, SCREENSHOT_TIMEOUT_MS);
    stabilityTimer = setInterval(() => {
      let size = 0;
      try {
        size = fs.statSync(shotPath).size;
      } catch {
        return;
      }
      if (size > 0 && size === lastSize) {
        stableCount += 1;
      } else {
        stableCount = 1;
        lastSize = size;
      }
      if (stableCount < STABLE_POLLS) {
        return;
      }
      if (stabilityTimer !== null) {
        clearInterval(stabilityTimer);
        stabilityTimer = null;
      }
      // 文件已稳定：先发布候选图，再礼貌结束进程组，宽限期内不退再强杀
      // （SIGKILL 兜底不受 settle 影响）；close 分支被 settled 挡住，不会
      // 二次结算。
      fs.copyFileSync(shotPath, destPath);
      killTree("SIGTERM");
      killTimer = setTimeout(() => {
        killTree("SIGKILL");
        removeDirSafe(profileDir);
      }, KILL_GRACE_MS);
      finish();
    }, STABLE_POLL_INTERVAL_MS);
  });
}

// assertReadablePng 校验截图完整且为预期尺寸：pngjs 解码能挡住截断/损坏文件
// （chunk 结构不完整即抛错），尺寸校验挡住窗口参数漂移——两者都不给「半张
// 基线」入库的机会。
function assertReadablePng(filePath) {
  const png = PNG.sync.read(fs.readFileSync(filePath));
  if (png.width !== VIEW_WIDTH || png.height !== VIEW_HEIGHT) {
    throw new Error(
      `截图尺寸 ${png.width}x${png.height} 与预期 ${VIEW_WIDTH}x${VIEW_HEIGHT} 不符：${filePath}`,
    );
  }
}

// compareWithGolden 把候选实拍与基线逐像素比对（口径注释见顶部常量），并
// 生成可视化差异图：相同像素压暗、差异像素红标，供人眼直接定位问题区域。
// 与 Go 版一致只比 RGB（截图 alpha 恒为 255，比它没有信息量）。
function compareWithGolden(candidatePath, goldenPath) {
  const got = PNG.sync.read(fs.readFileSync(candidatePath));
  const want = PNG.sync.read(fs.readFileSync(goldenPath));
  if (got.width !== want.width || got.height !== want.height) {
    throw new Error(
      `图像尺寸不匹配：实拍 ${got.width}x${got.height}，基线 ${want.width}x${want.height}（${path.basename(candidatePath)}）`,
    );
  }
  const totalPixels = got.width * got.height;
  const vis = new PNG({ width: got.width, height: got.height });
  let maxChannelDelta = 0;
  let diffPixels = 0;
  let firstDiffX = -1;
  let firstDiffY = -1;
  for (let y = 0; y < got.height; y += 1) {
    for (let x = 0; x < got.width; x += 1) {
      const i = (got.width * y + x) << 2;
      let maxDelta = 0;
      for (let c = 0; c < 3; c += 1) {
        const delta = Math.abs(got.data[i + c] - want.data[i + c]);
        if (delta > maxDelta) {
          maxDelta = delta;
        }
      }
      if (maxDelta > maxChannelDelta) {
        maxChannelDelta = maxDelta;
      }
      if (maxDelta > 0) {
        if (firstDiffX < 0) {
          firstDiffX = x;
          firstDiffY = y;
        }
        diffPixels += 1;
        vis.data[i] = 255;
        vis.data[i + 1] = 0;
        vis.data[i + 2] = 0;
      } else {
        // 相同像素压暗（基线红通道 / 4），与 Go compareImages 的可视化一致。
        const dim = want.data[i] >> 2;
        vis.data[i] = dim;
        vis.data[i + 1] = dim;
        vis.data[i + 2] = dim;
      }
      vis.data[i + 3] = 255;
    }
  }
  return {
    maxChannelDelta,
    diffPixels,
    totalPixels,
    ratio: totalPixels === 0 ? 0 : diffPixels / totalPixels,
    firstDiffX,
    firstDiffY,
    vis,
    drifted: maxChannelDelta > MAX_CHANNEL_DELTA || diffPixels / (totalPixels || 1) > MAX_DIFF_PIXEL_RATIO,
  };
}

// formatDiff 与 Go imageDiff.String() 同构：报数字也报首个差异坐标，
// 避免只给比例让人盲修。
function formatDiff(result) {
  let text = `最大通道差 ${result.maxChannelDelta}，差异像素 ${result.diffPixels}/${result.totalPixels}（${(result.ratio * 100).toFixed(4)}%）`;
  if (result.diffPixels > 0) {
    text += `，首个差异像素在 (${result.firstDiffX},${result.firstDiffY})`;
  }
  return text;
}

async function main() {
  const mode = process.argv[2];
  if (mode !== "check" && mode !== "update") {
    console.error(`用法：node visual/visual.mjs <check|update>（收到：${mode ?? "(缺席)"}）`);
    process.exitCode = 2;
    return;
  }
  await runViteBuild();
  fs.mkdirSync(outDir, { recursive: true });
  const chromeBin = resolveChromeBin();
  const { server, port } = await startStaticServer(visualDistDir);
  try {
    const captured = [];
    for (const name of fixtureNames) {
      const url = `http://127.0.0.1:${port}/index.html?fixture=${encodeURIComponent(name)}`;
      const destPath = path.join(outDir, `${name}.png`);
      // 每个 fixture 独立临时 profile：任一 Chrome 残留都不会用 profile 锁
      // 卡住下一张截图。
      const shotProfileDir = fs.mkdtempSync(path.join(os.tmpdir(), "mornlea-visual-shot-"));
      try {
        await captureScreenshot({ chromeBin, url, destPath, profileDir: shotProfileDir });
      } finally {
        removeDirSafe(shotProfileDir);
      }
      assertReadablePng(destPath);
      console.log(`已截取 ${name} → ${path.relative(repoRoot, destPath)}`);
      captured.push({ name, destPath });
    }

    if (mode === "update") {
      // 显式 update 才写基线：截图全部成功后一次性覆盖，避免半批基线。
      fs.mkdirSync(goldenDir, { recursive: true });
      for (const { name, destPath } of captured) {
        fs.copyFileSync(destPath, path.join(goldenDir, `${name}.png`));
      }
      console.log(
        `基线已覆盖 ${fixtureNames.length} 张 → ${path.relative(repoRoot, goldenDir)}/（请人工目检确认呈现正确后再提交入库）`,
      );
      return;
    }

    // check：缺基线只报错、绝不自动创建；漂移写差异图并整体非零退出。
    const missing = [];
    const drifted = [];
    for (const { name, destPath } of captured) {
      const goldenPath = path.join(goldenDir, `${name}.png`);
      if (!fs.existsSync(goldenPath)) {
        missing.push(name);
        continue;
      }
      const result = compareWithGolden(destPath, goldenPath);
      if (result.drifted) {
        const diffPath = path.join(outDir, `${name}-diff.png`);
        fs.writeFileSync(diffPath, PNG.sync.write(result.vis));
        drifted.push({ name, message: formatDiff(result), diffPath });
      } else {
        console.log(`通过 ${name}：${formatDiff(result)}`);
      }
    }
    if (missing.length > 0) {
      console.error(
        `缺少基线（首次建立基线请运行 corepack pnpm visual-update）：${missing.join("、")}`,
      );
    }
    for (const item of drifted) {
      console.error(
        `视觉漂移 ${item.name} 超出双阈值：${item.message}（实拍与差异图见 ${path.relative(repoRoot, outDir)}/，差异图 ${path.relative(repoRoot, item.diffPath)}）`,
      );
    }
    if (missing.length > 0 || drifted.length > 0) {
      process.exitCode = 1;
    } else {
      console.log(`全部 ${fixtureNames.length} 个部件与基线一致`);
    }
  } finally {
    server.close();
    // 截图用的 keep-alive 连接可能还挂着，不主动断会让进程退出被拖住。
    server.closeAllConnections();
  }
}

main().catch((error) => {
  console.error(`visual 管线失败：${error instanceof Error ? error.message : String(error)}`);
  process.exitCode = 1;
});
