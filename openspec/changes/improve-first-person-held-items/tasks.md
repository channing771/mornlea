## 1. 主手与持物呈现
- [x] 1.1 在 assets 增加与材质刷新一致的有厚度图标缓存，先写 alpha、颜色、覆盖和预算失败测试，再实现；验证 `go test ./packages/client/assets -count=1`。
- [x] 1.2 在 render 与 app 隐藏空闲左手，连接缓存，构建共同握持根、工具分档握点与分面方块；先写失败测试，验证 `go test ./packages/client/render ./packages/client/cmd/mornlea/app -count=1`。
- [x] 1.3 同步 Go/Rust viewmodel 257 实例容量与超量错误测试，保留 ABI 布局；验证 `cargo test -p mornlea_client --locked viewmodel`，执行 `make rust` 并复验涉及 Go 包。
## 2. 视觉与收尾
- [x] 2.1 在 capture 增加无头持物演示，覆盖空手、代表分面方块、全部工具及损坏状态、非方块物品；完整挖掘与攻击动作包含中立、峰值及恢复，输出审查物而不自动更新基线；验证 capture 定点测试与实际 motion demo。
- [x] 2.2 完成逐图目检、16:9/4:3 投影、零分配与预算回归；记录 ledger 并完成任务审查、整分支终审。
- [x] 2.3 收尾执行 gofmt、`make test-race`、`make dev-check`（含六模块 vet 及 `make rust-check` 等价的 Rust fmt/clippy/test） 和 `openspec validate --all --strict --no-interactive`；记录证据，不推送或合并。

## 3. 用户反馈：立体结构、HUD 与即时挥动
- [ ] 3.1 工具改为分件立体模型，当前材质和损坏差异保持；先失败测试再实现并定点验证。
- [ ] 3.2 适配逻辑窗口、FOV 与完整 HUD 安全区，验证窄屏、常用窗口及完整动作。
- [ ] 3.3 接入有效本地主键动作，空挥即时、时间稳定、界面门控和会话重置正确，权威行为不变。
- [ ] 3.4 用户授权的隔离交互窗口同时展示真实 HUD、主手与持物，截图/录制空手和工具点击，完成实际目检与必要修整。
- [ ] 3.5 任务审查、整分支终审、相称测试与收尾门禁，记录新证据；不推送或合并。
