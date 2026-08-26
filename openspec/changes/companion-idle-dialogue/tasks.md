## 1. Idle Dialogue 节点契约

- [ ] 1.1 在 `internal/companion/dialogue_nodes_test.go` 与 `dialogue_client_test.go` 先写失败测试，覆盖 idle 零载荷合法矩阵、任何任务载荷拒绝及稳定 payload kind `"idle"`；运行 `go test ./internal/companion -run 'TestDialogueNodeValidateMatrix|TestDialogueClientIdleNodePayload' -count=1` 证明 RED。
- [ ] 1.2 在 `internal/companion/dialogue_nodes.go` 与 `dialogue_client.go` 追加枚举末尾的 `DialogueNodeIdle`、零载荷校验和稳定文本映射；不改系统提示、响应 schema 或任务节点数值。
- [ ] 1.3 运行 `go test ./internal/companion -run 'TestDialogueNodeValidateMatrix|TestDialogueClientIdleNodePayload' -count=1` 与 `go test ./internal/companion -race -count=1`；经独立 SPEC/QUALITY 双评审通过后记录 commit、RED/GREEN 与裁决到 `ledger.md`。

## 2. 确定性空闲期限与派发

- [ ] 2.1 新建 `internal/server/companion_idle_dialogue_test.go` 并先写失败测试，覆盖 FNV-1a golden `1369`、`1200..2400` 边界、`uint64` tick 回绕、水平 16 格边界、离线/inactive/恢复合成身份、current/pending 重置、槽满/单在途跳过及 idle 不消耗任务八次预算；另从无期限开始断言首个空闲 tick 只设置精确 deadline，晚于旧 deadline 执行时仍从旧 deadline 精确递推，并在任务重置后从再次空闲 tick 精确重建；运行 `go test ./internal/server -run 'TestIdleDialogueInterval|TestIdleDialogueDue|TestIdleDialogueAudience|TestIdleDialogueDispatch' -count=1` 证明 RED。
- [ ] 2.2 新建 `internal/server/companion_idle_dialogue.go`，在 `companion_manager.go` 增加两个运行期期限字段和 server-only issuer `restored` 标记，并把 `dispatchIdleDialogues` 固定接在 `dispatchPlanning` 之后；所有 tick 工作按 `orderedIDs` 扫描且最多 4 个 slot。
- [ ] 2.3 修改 `internal/server/companion_dialogue.go`，让 idle 复用既有 inactive、单在途、四槽、请求构造和 worker 路径，但不检查或递增 `dialogueRequests`；任何期限机会在派发资格判断前先安排下一期限。
- [ ] 2.4 运行 `gofmt -w internal/server/companion_idle_dialogue.go internal/server/companion_idle_dialogue_test.go internal/server/companion_manager.go internal/server/companion_dialogue.go`、上述定点测试、`go test ./internal/server -race -count=1` 与 `go test ./internal/archcheck -count=1`；经独立 SPEC/QUALITY 双评审通过后更新 `ledger.md`。

## 3. idle 结果重验、广播与 parity

- [ ] 3.1 在 `internal/server/companion_idle_dialogue_test.go` 先写失败测试，覆盖有效结果、pending/current/issuer/恢复身份/离线/超距/inactive 过时结果、摘要与事实不变，以及 idle 在途时新任务不取消且不发第二请求；generation 场景保持 queue 空闲而只让 outcome generation 不匹配，模型错误场景断言已经安排的下一 deadline 精确不变；运行 `go test ./internal/server -run 'TestIdleDialogueOutcome|TestIdleDialogueTaskStartDoesNotPreempt' -count=1` 证明 RED。
- [ ] 3.2 修改 `internal/server/companion_dialogue.go` 的 outcome switch，在 generation 检查后重验空队列、真实同一 issuer、active、在线与水平 16 格；有效结果只复用 `applyDialogueEffect` 广播 speech，非 terminal 语义保证 summary 不变。
- [ ] 3.3 在 `internal/server/companion_dialogue_wiring_test.go` 先补双在线玩家广播测试和受控 fake 模型的 Memory/TCP 业务事件投影 parity；不比较绝对落地 tick 或跨传输 EventID。
- [ ] 3.4 运行 `gofmt -w internal/server/companion_idle_dialogue_test.go internal/server/companion_dialogue.go internal/server/companion_dialogue_wiring_test.go`、`go test ./internal/server -run 'TestIdleDialogueOutcome|TestIdleDialogueTaskStartDoesNotPreempt|TestCompanionIdleDialogueBroadcastsToAllPlayers|TestCompanionIdleDialogueMemoryTCPParity' -count=1`、`go test ./internal/companion ./internal/server -race -count=1` 与 archcheck；经独立 SPEC/QUALITY 双评审通过后更新 `ledger.md`。

## 4. 整分支评审与门禁

- [ ] 4.1 派发独立整分支终审，逐项核对 proposal、delta spec、design、实现、测试与本任务表；修复循环最多 5 轮，把所有 finding、repair、Ruling 和复审结论写入 `ledger.md`。
- [ ] 4.2 对本 change 的 9 个计划内 Go 文件运行显式 `gofmt -w`，再运行 `test -z "$(gofmt -l internal/companion internal/server)"`、`git diff --check c60e8f69...HEAD`、工作树 `git diff --check` 与 `git diff --cached --check`；提交所有计划内修改后以 `test -z "$(git status --porcelain)"` 要求 clean worktree，再审核 `git diff --name-only c60e8f69...HEAD`，只允许计划内 Go 文件、`openspec/changes/companion-idle-dialogue/`、本实现计划和 `docs/feature-backlog.md` 的 C-08 行，任何 version/schema/ABI/scenario/capture/golden 路径均硬失败。
- [ ] 4.3 依次运行 `go test ./internal/companion ./internal/server -race -count=1`、`go test ./internal/archcheck -count=1`、`go vet ./...`、`go test ./... -race`、`make rust-check` 与 `scripts/agents/gates.sh`；任何失败修复根因，不调整正确性门禁或超时规避失败。
- [ ] 4.4 运行 `openspec validate --all --strict --no-interactive`；结合 4.2 的 changed-file 审核确认协议 v26、玩家 schema v7、区块 schema v9、world metadata v2、`companions.ai` schema v4、engine ABI v7、client ABI v9、benchmark scenario v19 与 capture golden 均未变化，在 fresh 证据齐全后完成任务状态。
