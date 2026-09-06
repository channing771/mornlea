# selective-visual-capture 设计

## 现状与目标路径

`RunCapture(app, dir, updateGolden)` 单一入口顺序跑全部 `captureScenes`（24 景）后无条件跑 `RunPassiveDeathGIFs`。目标：同一入口支持显式场景子集与 GIF 生成门控，缺省路径行为不变。

## API 设计

```go
// capture 包新增（场景表仍是唯一事实源，不另立名单）：
func SceneNames() []string                 // 表序返回全部正式场景名
func ValidateSceneSelection([]string) error // 空项/重复/未知名报错
type RunOptions struct {
    UpdateGolden bool
    Scenes       []string // nil/空 = 全部场景（缺省路径）
    IncludeGIFs  bool     // check 模式显式请求 GIF；update 模式恒生成
}
func (o RunOptions) gifsEnabled() bool { return o.UpdateGolden || o.IncludeGIFs }
func RunCapture(app SceneApplication, dir string, opts RunOptions) error
```

- `RunCapture` 签名从三个裸参数改为 `RunOptions`：生产调用点只有 `main.go` 的 `runDependencies` 适配器一处，`RunPassiveDeathGIFs` 只有 `RunCapture` 内部一处，改造面最小。
- 子集过滤用内部纯函数 `selectScenes(requested []string) ([]captureScene, error)`：按成员集合过滤 `captureScenes`，天然保序（不重排），未知名报错（防御层；parse 层已先行校验）。
- 场景名在两层校验：`parseMainOptions` 调 `capture.ValidateSceneSelection` 在启动前拒绝（对齐 `--motion-scene` 拒未知值的既有风格）；`RunCapture` 内部再防御一次，供直接调用方（测试/未来入口）兜底。

## 依赖方向与并发边界

- 新增依赖方向：`package main`（options.go）→ `capture`，为既有方向的复用（main.go 已 import capture），无新边、无环。
- `capture` 包不新增任何 import；场景表仍为唯一事实源。
- 无新增 goroutine：子集仍是单 application 单线程顺序执行，既有顺序契约与「跨 goroutine 消息不可变」边界不受影响。

## 顺序语义

子集 = 保序过滤，不重排：`far-horizon` 倒数第二、`water-underwater` 唯一末景等全部顺序 MUST 条款在全量路径原样成立，子集路径按与完整清单一致的相对顺序解释。GIF runner 的干眼重钉是无条件执行、不依赖前置场景，子集路径下 GIF（仅在显式请求/update 时运行）不受影响。

## GIF 生成时机矩阵

| 路径 | GIF 生成 |
|---|---|
| check（缺省） | 否（打印一行跳过说明） |
| check + `--capture-gifs` | 是，写入输出目录供人工审查 |
| update（任意子集） | 是，写入 `.gif` 基线（既有行为不变） |

## CLI 与 Makefile

- `--capture-scenes "a,b"`：逗号切分，逐项 trim；空项/重复/未知名报错；未指定 `--capture` 时拒绝。
- `--capture-gifs`：bool；未指定 `--capture` 时拒绝；与 `--update-golden` 组合合法但冗余（update 恒生成）。
- `mainOptions` 新增 `CaptureScenes []string`、`CaptureGIFs bool`；`runWithDependencies` 在两个 `runCapture` 调用点（update/check）统一组装 `capture.RunOptions`。
- Makefile：`visual-check`/`visual-update` 追加 `$(if $(SCENES),--capture-scenes $(SCENES))`；`visual-check` 另追加 `$(if $(GIFS),--capture-gifs)`。

## 受影响文件

- `packages/client/cmd/mornlea/capture/capture.go`：`RunOptions`、`SceneNames`、`ValidateSceneSelection`、`selectScenes`、`gifsEnabled`、`RunCapture` 签名与 GIF 门控。
- `packages/client/cmd/mornlea/capture/capture_scene_selection_test.go`（新增）：子集选择与 GIF 门控单测。
- `packages/client/cmd/mornlea/main.go`：`runDependencies.runCapture` 签名与两处调用点组装 `RunOptions`。
- `packages/client/cmd/mornlea/options.go`：两个新 flag 与 parse 校验。
- `packages/client/cmd/mornlea/options_test.go`、`run_test.go`：parse 与接线测试（含 fake 适配）。
- `Makefile`、`docs/notes/visual-verification.md`、`.claude/skills/visual-baseline/SKILL.md`。

## 兼容/迁移与回退

- 无版本号（协议/存档/ABI/scenario）变化；golden 文件不动；缺省命令行为逐字节不变。
- 回退：revert 本分支即可，无数据迁移。

## 风险与验证

- 主要风险是子集运行的前置状态差异（场景间共享 application，存在如浸没标志的跨场景残留）。验证：真机等价性——同一 SHA 下先全量 `make visual-check`（必须全绿），再以子集重跑（`mining-crack-early,mining-crack-heavy`、`main-menu`、`water-underwater` 等覆盖首/中/尾与菜单相位），对同一 golden 同一双阈值也必须全绿；发现子集不安全的场景时，在过滤层强制带上前置或拒绝该子集，并记 ledger 与 spec delta。
- 单测：`selectScenes` 保序/未知/重复、`gifsEnabled` 矩阵、parse 层拒绝路径、缺省全量不变（既有 `capture_scene_order_test` 等锁定测试照常通过）。

## 被否决的替代方案

- **更换实现语言**：比对毫秒级、渲染已在 Rust/wgpu，瓶颈（GPU 帧、世界加载、GIF 浪费）与语言无关；重写整条 golden 管线零收益。
- **内容哈希缓存**：渲染输入等价于整个 client+engine 代码与内嵌资产，任何改动全量失效，退化为现状。
- **并行分片**：违背场景顺序契约，每分片重复一次世界加载，跨进程确定性不可控。
- **git diff 自动场景映射**：需维护"代码→场景"映射表，映射错误会静默漏检；用户裁决本期先做手动子集，映射可作为后续独立 change。
