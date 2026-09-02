# remove-mining-hud-bar Ledger

基线 SHA：`dca9197e`（main）。分支 `remove-mining-hud-bar`。

## 阶段 1：内容确认

- Ruling: 需求方在对话中直接裁决「破坏进度条可以去掉，使用裂纹替换」—
  与已交付的裂纹反馈构成明确取舍（屏幕条与裂纹并存重复）— 分类 bounded
  （既有呈现通道的删除，无新子系统）。设计决策（桥分节彻底删除、
  Harvestable 死字段一并删、进食条独立）见 design.md。

## 任务执行

### Task 1：三端删除采掘进度条

- 实现：commits `5a890919`（前端 17 文件：桥 schema/client、HudRoot/
  ProgressTrack、样式/令牌/几何、测试、visual 基线 hud-progress+
  hud-status 重拍、dist 重建）+ `735e028d`（Go 12 文件：`UIHudMining`
  删除、`assembleHUDState` 分节与互斥门控移除、`MiningOverlay.
  Harvestable` 死字段删除、镜像/夹具/测试同步）。
- 验证证据 @ `735e028d`：`make frontend-check` exit 0（146 vitest）；
  `go test ./internal/client ./cmd/mornlea/app ./cmd/mornlea/capture
  ./internal/render/... -race -count=1` 5 包全 ok；gofmt 无输出；
  TDD 双侧先红（前端 33 failed、Go 钉值/push/schema 四测红）后绿。
- SPEC 评审：**pass**（6/6；REMOVED 落实无死代码、三端桥逐字段一致、
  裂纹与协议 `PlayerState.MiningHarvestable` 未扰、Harvestable 客户端
  零残留）。QUALITY 评审：**pass**（删除质量 4/5、桥/Go/测试 5/5；
  提交分 2 个的编译耦合理由成立；中间提交 `5a890919` 单独 checkout Go
  契约红为已知 bisect 代价，备案接受）。
- Ruling: R1 修复三处 — `app_ui_state_test.go` 恒真死断言（载荷缺
  eating 短路拒绝归因，必修）、`visual.css:40`「三档轨道」注释残留、
  `cmd/mornlea/AGENTS.md:69` 采掘轨道提及过时 — 均评审明确非阻塞缺口。
- spec 标题矛盾由控制会话直接处理：MODIFIED 需求名须与主规格逐字一致
  （openspec 1.7 漂移守卫），按「完整场景顺序固定为 19 项」先例在 delta
  正文加历史名 blockquote 说明。
- R1 落地：commit `a9b39cfe`（死断言改为完整必填载荷 + 仅注入 mining 键，
  变异测试实证归因；visual.css 单轨注释；cmd/mornlea/AGENTS.md 同步）。
  focused race ok；frontend-check 146/146；gofmt 干净。Task 1 关闭。

### Task 2：收尾门禁（控制会话执行）

- 结果见下一条目（gofmt/vet/visual-check 24 场景/openspec strict/
  `make rust` 重建含新 dist 的 dylib）。
- 门禁结果 @ `a9b39cfe`：`go vet ./...` 干净、`gofmt -l .` 无输出、
  `make rust` 重建（新 dist 内嵌进 dylib）、`make visual-check` 24/24
  场景零差异（裂纹 golden 不动，实证不受扰）、`openspec validate
  --all --strict --no-interactive` 82/82。Task 2 关闭，change 就绪。
