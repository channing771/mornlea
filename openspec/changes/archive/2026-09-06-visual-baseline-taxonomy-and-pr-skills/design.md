## Context

动机见 proposal.md。现状证据：`testdata/visual-golden/` 下 `world/` 27 张 PNG（`captureScenes` 表驱动、无头 640×360、双阈值进 `make visual-check`）、`ui/` 19 张 PNG（`fixtureNames` 单源、本机 Chrome、`frontend-visual-check` 不进 CI）、`motion/break-burst.gif` 50 帧采掘全流程演示（只验呈现、不进比对、`--motion-demo` 独立入口）、`passive-death/` 4 个 GIF（tick 步进、逐帧双阈值、帧预算有界、进 `make visual-check`）。PR 链现状：`docs/development-process.md` 阶段 5 第 5 步（默认 `AGENT_MODE=pr`）、`scripts/agents/confirm/confirm.sh`（飞书优先、Discussion 降级）、`scripts/agents/pr-finalize.sh`（detached 监听、失败重跑最多 3 轮、全绿才合并）、PR 标题正文规范（单行英文 + 模板）。

## Goals / Non-Goals

**Goals:**

- 三类边界成文且可执行：按渲染源与时间维度路由，消灭无理由的 PNG + GIF 双存。
- 两条高频工作流变成 skill：调用方只记 skill 名，不再拼凑文档与脚本路径。
- skill 只引用权威源（常量、命令、模板路径），不复制会漂移的阈值、清单与数量。

**Non-Goals:**

- 不增删改任何基线图片，不改场景表与夹具，不调阈值。
- 不碰 `visual-verification` 主规格（24 与 27 的数量差由 `grass-closeup-scene` change 负责对齐）。
- 不新增自动化脚本，只编排既有脚本与命令。

## Decisions

- **三类按“渲染源 × 时间维度”划分**：`ui` = 窗口/WebView 层（Chrome 夹具截图）；`world` = 无头世界单帧（离屏渲染收敛后抓帧）；GIF = 跨 tick 过程（tick 步进连帧）。备选“按文件后缀划分”被否决——后缀相同但门禁语义不同（`motion` 演示不进比对、`passive-death` 进比对），必须按语义分子类。
- **GIF 内分演示与门禁两小类**：演示（`motion`）只供人眼审查、不进任何比对；门禁（`passive-death`）逐帧双阈值、全帧通过方为通过、帧预算有界。备选“统一为门禁”被否决——演示的 50 帧全流程若进门禁会把人工审查物变成硬门禁，与其“只验呈现”定位冲突。
- **静态与全流程互斥**：单帧稳定态必须 PNG；跨 tick 状态迁移必须 GIF 且覆盖触发前、结算、收敛全流程；同一行为禁双存。现有重叠（吃草静态对照与 `graze.gif`、裂纹双帧与 `break-burst` 演示）按“门禁采样 vs 全流程呈现”区分：裂纹双帧是门禁采样点、演示是人工审查物，允许并存但必须在 README 注明理由；吃草静态的泥土对照格与 GIF 的草变泥土同镜若语义重复，后续由所属玩法 change 收敛，本 change 只立规则不动基线。
- **skill 双目录同文**：`.claude/skills/` 与 `.codex/skills/` 内容一致（仅当引用斜杠命令时才按目录改写，本次两 skill 均无斜杠引用）。备选“只写一处”被否决——两目录既有 6 个 openspec skill 均为双写，单写会造成调用方按客户端找不到 skill。
- **PR skill 把确认做成硬门禁**：`confirm.sh ask/wait` 未收到 approve 不得 `gh pr create`；`pr-finalize.sh` 未全绿不得合并；上限轮次内仍红则停在 OPEN 并交人工。备选“确认可选”被否决——用户明确要求提交前先确认。

## Risks / Trade-offs

- [规则立后旧重叠仍在] → 本 change 只立规则不动基线，避免与 `grass-closeup-scene` 及玩法 change 冲突；收敛旧重叠由后续各所属 change 按规则执行。
- [skill 与文档漂移] → skill 正文只引用文档与脚本路径，不复制命令全文与阈值；权威仍是 `docs/development-process.md` 与源码常量。
- [双写不一致] → 两目录文件逐字节一致（本次无客户端差异点），由任务中的 diff 断言钉住。

## Migration Plan

- 落地：README 规则节 + 4 个 `SKILL.md`；`openspec validate --all --strict --no-interactive` 通过。
- 回退：删除规则节与新增 skill 目录，重跑同一校验。

## Open Questions

无。
