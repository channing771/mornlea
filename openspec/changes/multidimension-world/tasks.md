## 1. 维度地基与世界生成

- [ ] 1.1 `packages/shared/core/block.go` 新增 `Depths DimensionID = 1`；`packages/server/sim/runtime/engine.go` 启动 `EnsureDimension(Depths)`；验证 `go test ./packages/server/sim/runtime -race -count=1`
- [ ] 1.2 `packages/shared/worldgen/generator.go` 支持按维派生种子（主世界原样，`Depths` 异或固定 salt）并新增 `GenerateChunk(dim,pos)`、`HeightAt(dim,…)`；`packages/server/server/generator.go` 接口同步加维度参数并 sweep 全部测试 helper 到新签名；验证 `go test ./packages/shared/worldgen ./packages/server/server -race -count=1`

## 2. 存档与迁移

- [ ] 2.1 `packages/server/storage/metadata.go` 实现 metadata v5（读 v1..v5，写只写 v5，旧档 `Depths` 锚点默认主世界锚点）+ `chunk_codec.go` 维度值域收紧为 0/1；验证 `go test ./packages/server/storage/... -race -count=1`

## 3. 协议

- [ ] 3.1 协议 v38→v39：`packet.go` 常量与历史注释，玩家/区块类消息放行 0/1（`message_player.go`、`message_command.go`、`codec_server.go`），伙伴系保持拒非零；同步根 `AGENTS.md` 与 `openspec/config.yaml` 版本矩阵；验证 `go test ./packages/shared/network/... -race -count=1` 与 `go test ./packages/audit -count=1`

## 4. 传送与出生

- [ ] 4.1 `packages/server/server/warp.go`（新建）：`/warp depths|overworld` 解析与单 tick 原子传送（冻结输入、旧维保存、跨维搬运、新维出生扫描、下发 `PlayerState{Dimension,Reset:true}` + `ForgetChunks` + `ChunkSnapshot`，失败 `CommandRejected` 留旧维）；`sim/entity/sleep.go` 床重生同维约束；验证 `go test ./packages/server/server ./packages/server/sim/... -race -count=1`
- [ ] 4.2 Memory/TCP 传送 parity 与重启双维重载集成测试；验证 `go test ./packages/server/server -race -count=1`

## 5. 收尾门禁

- [ ] 5.1 `gofmt`、六模块全量 `go test -race`（或 `make test-race`）、六模块 `go vet`（或 `make dev-check`）、`openspec validate --all --strict --no-interactive` 全绿
