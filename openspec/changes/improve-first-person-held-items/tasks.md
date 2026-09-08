## 1. 主手与持物呈现
- [ ] 1.1 在 assets 增加与材质刷新一致的有厚度图标缓存，先写 alpha、颜色、覆盖和预算失败测试，再实现；验证 `go test ./packages/client/assets -count=1`。
- [ ] 1.2 在 render 与 app 隐藏空闲左手，连接缓存，构建共同握持根、工具分档握点与分面方块；先写失败测试，验证 `go test ./packages/client/render ./packages/client/cmd/mornlea/app -count=1`。
- [ ] 1.3 同步 Go/Rust viewmodel 257 实例容量与超量错误测试，保留 ABI 布局；验证 `cargo test -p mornlea_client --locked viewmodel`，执行 `make rust` 并复验涉及 Go 包。
## 2. 视觉与收尾
- [ ] 2.1 在 capture 增加无头持物演示，覆盖空手、代表分面方块、全部工具及损坏状态、非方块物品；完整挖掘与攻击动作包含中立、峰值及恢复，输出审查物而不自动更新基线；验证 capture 定点测试与实际 motion demo。
- [ ] 2.2 完成逐图目检、16:9/4:3 投影、零分配与预算回归；记录 ledger 并完成任务审查、整分支终审。
- [ ] 2.3 收尾执行 gofmt、`make test-race`、`make dev-check`（含六模块 vet）、`make rust-check` 和 `openspec validate --all --strict --no-interactive`；记录证据，不推送或合并。
