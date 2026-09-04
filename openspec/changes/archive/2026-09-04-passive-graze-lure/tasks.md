## 1. 权威吃草行为

- [x] 1.1 吃草事件推进：splitmix64 抽选 + 站草判定 + 20 tick 低头 + 中断语义（受击/移动/换块）+ 结算单格草→泥土（`packages/server/sim/entity`；验证 `go test ./packages/server/sim/entity -race -count=1`）
- [x] 1.2 吃草边界与预算测试：非草/未加载/满负载/重启瞬态丢失（同包；验证同 1.1）

## 2. 权威引诱行为

- [x] 2.1 引诱转向：最近玩家 ≤8 格 + 选中格小麦判定 + 2.5 格止步 + 逃跑＞引诱＞漫游优先级（同包；验证同 1.1）
- [x] 2.2 引诱边界测试：切走恢复漫游、超距恢复、异维/离线排除（同包；验证同 1.1）

## 3. 协议放牧位与版本

- [x] 3.1 `PassiveState` +1 字节放牧位：值类型、wire 编解码、拒绝矩阵（`grazing` 非 0/1 拒绝）、字节推导测试更新、协议 v33→v34、基线矩阵同步（`packages/shared/network` + `packages/audit`；验证 `go test ./packages/shared/network/... -race -count=1` 与 `go test ./packages/audit -count=1`）
- [x] 3.2 服务端发布携带放牧位 + Memory/TCP 一致（`packages/server/server`；验证 `go test ./packages/server/server -run 'Passive' -race -count=1`）

## 4. 客户端呈现与场景

- [x] 4.1 放牧位镜像 + 牛头 Pitch 下压映射 + 角度锁定测试（`packages/client/client`、`packages/client/render`；验证 `go test ./packages/client/client ./packages/client/render -race -count=1`）
- [x] 4.2 `passive-graze` capture 场景（低头牛 + 草→泥土前后）+ golden 入库 + 旧景不变（`packages/client/cmd/mornlea/capture`；验证 `go test ./packages/client/cmd/mornlea/capture -race -count=1` 与 `make visual-check`）

## 5. 收尾门禁

- [x] 5.1 `gofmt` + `go vet` + 受影响包全量 race + `go test ./packages/audit -count=1` + `openspec validate --all --strict --no-interactive` + `make visual-check`
