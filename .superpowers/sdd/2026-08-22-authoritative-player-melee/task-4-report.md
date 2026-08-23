Task 4 report
红测：新增 benchmark 断言先因 `fixedBenchmarkPlayerInput` 缺失而编译失败。
结果：Memory/TCP 只经 wire 比较自身 health/reset 与远端位置；八人同 tick 和无玩家目标采掘已锁定。
验证：`make rust`。
验证：`go test ./internal/server -run 'Test(PlayerMeleeMemoryTCPWireParity|EightPlayersSameTickPrimaryInputKeepsSessionOrder|MultiplayerMemoryTCPMiningCompetition)' -race -count=1`。
验证：`go test ./internal/server -run TestPlayerMeleeMemoryTCPWireParity -race -count=10`。
验证：`go test ./internal/server -run 'Test(EightPlayersSameTickPrimaryInputKeepsSessionOrder|MultiplayerMemoryTCPMiningCompetition)' -race -count=10`。
验证：`go test ./cmd/mornlea -run 'Test.*Scenario' -count=1`、`go test ./internal/server -run 'Test(PlayerMelee|MemoryTCPParity|EightPlayer)' -race -count=10`、`git diff --check`。
SHA：87a9ad97f63a0c64e7088a75fbb31a8e614bd384。
文件：`internal/server/*melee*`、多人/采掘 parity 测试、benchmark 输入夹具与 OpenSpec tasks。
风险：无已知功能风险。
