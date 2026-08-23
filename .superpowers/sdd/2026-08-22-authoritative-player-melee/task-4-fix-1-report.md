Task 4 fix 1 report
红测：旧八人夹具中 Session 2 收到 18 血且未采掘，违反新共享目标断言。
结果：Session 1/2 同 tick 瞄准 Session 3；wire 状态锁定前者抑制采掘、后者冷却后采掘、目标只扣 2 血。
验证：focused server race `-count=1` 与新八人测试 `-count=10` 通过。
验证：`go test ./internal/server -run 'Test(PlayerMelee|MemoryTCPParity|EightPlayer)' -race -count=10` 通过。
验证：`gofmt`、`git diff --check` 通过。
SHA：c451e86142b3a17ab5c841f6ab91735ffc2316b0。
风险：无已知功能风险。
