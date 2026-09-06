# selective-visual-capture 任务

基线 SHA：`76e47f56`（分支 `feat/selective-visual-capture`）。每项任务：先写失败测试再实现；完成后做一轮任务评审并把验证证据（命令 + 摘要 + SHA）记入 `ledger.md`，单独提交该任务文件。

## 1. capture 层：场景子集与 GIF 门控

- [x] 1.1 新增 `packages/client/cmd/mornlea/capture/capture_scene_selection_test.go`：钉住 `selectScenes` 保序过滤（乱序/部分输入按表序输出）、`nil/空` 等价全量、未知名与重复名报错、`SceneNames` 与场景表一致、`RunOptions.gifsEnabled` 矩阵（check 缺省否 / check+IncludeGIFs 是 / update 恒是）。先确认测试编译失败（red）。
- [x] 1.2 在 `capture/capture.go` 实现 `RunOptions`、`SceneNames`、`ValidateSceneSelection`、内部 `selectScenes`、`gifsEnabled`；`RunCapture` 签名改为 `(app, dir, opts RunOptions)`，场景循环改用过滤结果，GIF 段按门控执行（跳过时打印一行说明）；同步修改 `main.go` 的 `runDependencies.runCapture` 适配器保证编译。focused：`go test ./packages/client/cmd/mornlea/capture -run 'SelectScenes|SceneNames|GifsEnabled|SceneSelection' -count=1` 与 `go test ./packages/client/cmd/mornlea/capture -run 'TestCaptureSceneOrder' -count=1`（缺省路径不回归）。
- [x] 1.3 任务评审（全新评审者）：对照本 change 的 delta spec 抽查保序、拒绝与门控行为；结论与修复记入 `ledger.md`。

## 2. CLI flag、接线与 Makefile 透传

- [x] 2.1 在 `packages/client/cmd/mornlea/options_test.go` 增补：`--capture-scenes` 未搭配 `--capture` 拒绝、未知/重复/空场景名拒绝、合法子集解析成功、`--capture-gifs` 未搭配 `--capture` 拒绝（先 red）。
- [x] 2.2 在 `options.go` 实现两个 flag 与校验（场景名经 `capture.ValidateSceneSelection`，逗号切分逐项 trim）；`mainOptions` 增加 `CaptureScenes`/`CaptureGIFs`；`main.go` 两个 `runCapture` 调用点组装 `capture.RunOptions`；`run_test.go` 的 fake 适配签名；`Makefile` 的 `visual-check`/`visual-update` 透传 `SCENES=`、check 侧 `GIFS=1`。focused：`go test ./packages/client/cmd/mornlea -run 'ParseCapture|CaptureScenes|CaptureGifs|RunCapture' -count=1`。
- [x] 2.3 任务评审（全新评审者）：对照 spec delta 抽查 parse 拒绝路径与接线；结论记入 `ledger.md`。

## 3. 文档与路由说明

- [x] 3.1 更新 `docs/notes/visual-verification.md`：子集使用纪律（编辑环 `SCENES=` 子集 check、推送/提交前全量 `make visual-check` 为权威门禁、GIF 人工审查用 `make visual-check GIFS=1`）；核对 `.claude/skills/visual-baseline/SKILL.md` 与 `testdata/visual-golden/README.md` 中与 GIF 生成时机相关的陈述并同步。focused：人工核对引用的命令与 flag 名一致。

## 4. 真机等价性与性能验证

- [x] 4.1 同一工作区：`make rust` 后先跑全量 `make visual-check`（须全绿、记录墙钟耗时），再跑 `make visual-check SCENES=mining-crack-early,mining-crack-heavy`、`SCENES=main-menu`、`SCENES=water-underwater`（均须对同一 golden 全绿，记录耗时）；`make visual-check GIFS=1` 确认显式 GIF 生成仍可用。任何子集红灯即停下归因：若为跨场景状态残留，在过滤层强制带上前置或拒绝该子集，先更新 delta spec 再改码。全部数值记入 `ledger.md`。

## 5. 收尾门禁

- [ ] 5.1 `gofmt -l .` 无输出；六模块 `go vet`（`go vet ./packages/contracts/... ./packages/shared/... ./packages/server/... ./packages/client/... ./packages/tools/... ./packages/audit/...`）；`make test-race`；`openspec validate --all --strict --no-interactive`。结果记入 `ledger.md`。
