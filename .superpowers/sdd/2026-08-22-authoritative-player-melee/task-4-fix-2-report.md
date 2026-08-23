Task 4 fix 2 report
结果：S2 在 S1 前连接，插入顺序与 `SessionID` 顺序相反；共享目标 wire 断言仍锁定 S1 抑制采掘、S2 冷却后采掘、S3 仅失 2 血。
验证：focused server race `-count=1`、新八人测试 `-count=10` 通过。
验证：`go test ./internal/server -run 'Test(PlayerMelee|MemoryTCPParity|EightPlayer)' -race -count=10` 通过。
验证：`gofmt`、`git diff --check` 通过。
SHA：30b09ed8e1e330d590f3200c86ef9ed5d1e81225。
风险：无已知功能风险。
