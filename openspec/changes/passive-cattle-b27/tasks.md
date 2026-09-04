## 1. core 物品与食物链

- [ ] 1.1 `ItemRawBeef`/`ItemCookedBeef` 追加（`ItemIDMax` 哨兵前）+ 堆叠/放置/中文名 + 穷举测试同步（`packages/shared/core`；验证 `go test ./packages/shared/core -race -count=1`）
- [ ] 1.2 熔炼映射 1 生→1 熟 + `FoodValue` 生/熟定值（熟严格高于生）+ 单测（同包；验证同 1.1）

## 2. 权威模拟被动牛

- [ ] 2.1 `passiveSet` 定容升序集 + ID 派生 + 32/6 上限 + 昼间草地生成判定（`packages/server/sim/entity`；验证 `go test ./packages/server/sim/entity -race -count=1`）
- [ ] 2.2 漫游/受击逃跑/死亡掉 1 生牛肉环形尝试（同包；验证同 2.1）
- [ ] 2.3 `audit allowed` 被动边登记（`packages/audit`；验证 `go test ./packages/audit -count=1`）

## 3. 被动存档域

- [ ] 3.1 `storage/passive` codec（PMST/v1/32×72B/CRC/规范形）+ golden + 损坏矩阵（验证 `go test ./packages/server/storage/passive -race -count=1`）
- [ ] 3.2 Disk/Memory 编排 + 备份忽略临时文件 + 启动恢复（`packages/server/storage`、`packages/server/server/persistence`；验证 `go test ./packages/server/storage -race -count=1`）

## 4. 被动协议与镜像

- [ ] 4.1 `PassiveSpawn/State/Despawn` 值类型 + wire codec + 校验矩阵 + Memory/TCP parity（`packages/shared/network`；验证 `go test ./packages/shared/network/... -race -count=1`）
- [ ] 4.2 服务端订阅发布（进入视野 spawn/逐 tick state/离开死亡 despawn，每类每 tick ≤1 包）+ 双传输序列一致（`packages/server/server`；验证 `go test ./packages/server/server -run 'Passive' -race -count=1`）
- [ ] 4.3 客户端 latest-wins 镜像（未知 ID/过期 tick 丢弃；`packages/client/client`；验证 `go test ./packages/client/client -race -count=1`）

## 5. 贴图素材与注册表

- [ ] 5.1 外网 CC0 素材转制 16×16（牛 `mobs_animal` 首选、牛肉 OGA `16x16 Food` 首选）+ `PROVENANCE.json`/`ATTRIBUTION.md` + `applyPack` 门禁（`packages/client/assets/packs/pixel_perfection`；验证 `go test ./packages/client/assets -race -count=1`）
- [ ] 5.2 注册表末位 4 层 + `Material` 映射 + 植物区间不变断言 + 不透明/cutout 分类（同包；验证同 5.1）

## 6. 贴图化呈现与 ABI

- [ ] 6.1 Avatar 四足分支 + 材质槽 + 牛皮/牛头 UV（`packages/client/render`；验证 `go test ./packages/client/render -race -count=1`）
- [ ] 6.2 `avatar.wgsl` atlas 采样 + `entity.rs` 容量/布局 + `mornlea_client.h` + Go bridge + client ABI v14→v15 + 跨语言布局测试（`packages/engine` + bridge；验证 `make rust` 后 `go test ./packages/client/client -race -count=1` 与 `cargo test -p mornlea_client --locked`）
- [ ] 6.3 App 装配被动镜像→Avatar 管线（`packages/client/cmd/mornlea/app`；验证 `go test ./packages/client/cmd/mornlea/app -race -count=1`）

## 7. 视觉场景

- [ ] 7.1 `passive-herd` 场景（昼间草地 3 牛 + 1 生肉掉落，投影间距核算）+ golden 入库 + 旧景不变（`packages/client/cmd/mornlea/capture`；验证 `go test ./packages/client/cmd/mornlea/capture -race -count=1` 与 `make visual-check`）

## 8. 收尾门禁

- [ ] 8.1 `gofmt` + 六模块 `go vet` + `go test ./... -race -count=1`（分层见 `docs/notes/test-quickstart.md`）+ `go test ./packages/audit -count=1` + `openspec validate --all --strict --no-interactive` + `make visual-check`
