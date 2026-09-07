## 1. 天气权威与同步持久化（A 组）

- [x] 1.1 服务端天气时钟失败测试与最小实现（`packages/server/sim` 天气状态机、tick 递减与轮转分布；`go test ./packages/server/sim/... -race -count=1`）
- [x] 1.2 协议 v36 天气字段编解码与越界拒绝（`packages/shared/network/protocol`、`packages/shared/network/codec`；`go test ./packages/shared/network/... -race -count=1`）
- [x] 1.3 metadata v4 天气持久化与 v3 迁移默认晴天（`packages/server` 世界存储；`go test ./packages/server/... -race -count=1`，含原子保存失败可恢复断言）

## 2. 天气客户端表现（A 组）

- [x] 2.1 天气镜像消费与旧状态不回退（`packages/client/client`；`go test ./packages/client/client -race -count=1`）
- [x] 2.2 雨/雪粒子（按高度相对雪线选形）/灰天空/亮度压暗/雷暴闪光呈现（`packages/client/render` + Rust `mornlea_client` 天空与粒子，同步 client ABI 头文件与 bridge；`go test ./packages/client/... -race -count=1`，`cd packages/engine && cargo test -p mornlea_client --locked`）

## 3. 三态视角（B 组）

- [x] 3.1 F5 循环与本地持久化（`packages/client` 设置存储；`go test ./packages/client/... -race -count=1`）
- [x] 3.2 自身显隐与 viewmodel 互斥（`packages/client/client` + `packages/client/render`；`go test ./packages/client/... -race -count=1`，静态 capture 无 viewmodel 像素回归）
- [x] 3.3 第三人称后拉与防穿墙 pull-in，瞄准仍用眼睛射线（Go 相机 + Rust `camera.rs`/`ffi.rs`；`go test ./packages/client/client -race -count=1`，`cd packages/engine && cargo test -p mornlea_client --locked`）

## 4. 树多样性（C 组）

- [x] 4.1 Rust 橡树高度/冠形/珍异分杈扩展与单测（`packages/engine/crates/mornlea_engine/src/worldgen.rs`；`cd packages/engine && cargo test -p mornlea_engine --locked`）
- [x] 4.2 Go/Rust parity 与跨块一致、旧块不迁移断言（`packages/shared/worldgen` 测试；`go test ./packages/shared/worldgen -race -count=1`，更新区块 golden 并记录新旧差异只来自新生成块）
- [x] 4.3 覆盖半径复核与越界/上界回归（同上两包；`go test ./packages/shared/worldgen -race -count=1`，`cd packages/engine && cargo test -p mornlea_engine --locked`）

## 5. 收尾门禁

- [x] 5.1 全量验证与归档就绪（`gofmt -l` 相关包、`make dev-check` 或 `scripts/agents/gates.sh` 入口、`make test-race` 六模块全量 race、`openspec validate --all --strict --no-interactive`，记录 benchmark 数值只记录不改退出态）
