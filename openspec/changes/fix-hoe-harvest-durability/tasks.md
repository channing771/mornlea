# fix-hoe-harvest-durability tasks

## 1. 作物 × 锄头耐久豁免（TDD）

- [x] 1.1 RED：在 `internal/sim/mining_test.go` 耐久簇新增用例：①完好石锄/铁锄收获成熟作物（`WheatStage7ID`）耐久不变、掉落 1 小麦 + 2 种子照常；②完好锄头收获未成熟作物（如 `WheatStage3ID`）耐久不变、掉落 1 种子照常；③对照：石镐收获成熟作物耐久减 1（豁免不外溢到非锄头）；④对照：石锄破坏泥土耐久减 1（豁免不外溢到非作物）。验证：`go test ./internal/sim -run 'TestMiningHoeHarvest' -race -count=1` 先失败（①②红、③④绿）。
- [x] 1.2 GREEN：`internal/sim/mining.go` 新增 `hoeHarvestDurabilityExempt(block core.BlockID, item core.ItemID) bool`（`core.IsCrop` × `core.TillingTool`，中文 GoDoc 说明豁免语义与「表」的扩展点），并在 `advanceMining` 玩家完成分叉的 `consumeToolDurability` 调用前按完成时选中物守卫。验证：1.1 全部转绿。
- [x] 1.3 回归：`go test ./internal/sim -race -count=1`（翻地扣耐久、既有耐久与采掘测试零改动通过）。

## 2. 收尾

- [ ] 2.1 `gofmt -l .` 无输出；`go vet ./...`；`go test ./internal/archcheck -count=1`。
- [ ] 2.2 `openspec validate fix-hoe-harvest-durability --strict --no-interactive`。
- [ ] 2.3 `scripts/agents/gates.sh` 全量门禁；基线文档（`AGENTS.md` + `CLAUDE.md` 判据句补豁免注记，逐字节相同）与 `docs/notes/progress.md` 基线段落同步；整分支终审；结论记入 `ledger.md`。
