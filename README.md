# Mornlea

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8" alt="Go 1.26">
  <img src="https://img.shields.io/badge/Rust-1.97.1-f74c00" alt="Rust 1.97.1">
  <img src="https://img.shields.io/badge/platform-macOS-9cf" alt="macOS">
  <img src="https://img.shields.io/badge/protocol-v26-blue" alt="协议 v26">
  <img src="https://img.shields.io/badge/license-MIT-green" alt="MIT">
  <img src="https://github.com/channing771/mornlea/actions/workflows/ci.yml/badge.svg" alt="CI">
  <img src="https://img.shields.io/github/v/release/channing771/mornlea" alt="release">
</p>

> [English](README.en.md) · 简体中文（本文）

`Mornlea` 是一个使用 Go 编写的独立体素游戏实验项目。项目自研客户端、权威服务端、世界存储和 Rust wgpu 渲染管线，不追求兼容官方 Minecraft 的协议、存档或资源。

<details>
<summary>English overview</summary>

Mornlea is an original voxel game written from scratch in Go 1.26 — no Mojang assets, protocol, or saves. It ships a custom client, an authoritative server, persistent worlds, physics, and a Rust wgpu renderer. The current baseline uses protocol v26, player schema v7, chunk schema v9, world metadata v2, `companions.ai` schema v4, engine ABI v6, client ABI v9, and benchmark scenario v19. Up to four named, server-authoritative companions can plan and persist queued `go_to`/`follow`/`mine`/`place` tasks and speak through a separate bounded persona/dialogue path; the survival loop includes water, farming, hunger, and local confirmation audio. The ordinary local client starts at an egui main menu with a three-control Settings page. The graphical client is macOS-only; the headless Linux dedicated-server bundle uses CGO and an adjacent `libmornlea_engine.so`. MIT licensed.

```bash
git clone https://github.com/channing771/mornlea.git
cd mornlea
make run          # graphical client with a built-in authoritative server
```

Milestone history lives in [实现进度](docs/notes/progress.md); the LAN server guide is [docs/notes/lan-server.md](docs/notes/lan-server.md). Most docs are in Chinese.
</details>

项目仍处于早期开发阶段，已经具备程序化地形、GPU 地形渲染、玩家移动与碰撞、客户端预测、方块挖掘与放置、内置权威服务端、世界持久化、有界二进制协议、TCP 直连、无图形专用服务端与稳定玩家状态存档；已交付里程碑与协议/存档版本演进见[实现进度](docs/notes/progress.md)。

当前基线使用协议 v26、玩家 schema v7、区块 schema v9、世界 metadata v2、`companions.ai` schema v4、engine ABI v6、client ABI v9 与 benchmark scenario v19。最多四个具名伙伴由服务端权威执行并持久化 `go_to`/`follow`/`mine`/`place` 队列任务，性格台词走独立有界表达路径；生存循环已经包含水、农业、饥饿与本地确认音效。普通本地客户端从 egui 主菜单进入游戏，并提供只含三项的设置页。完整里程碑和版本演进见[实现进度](docs/notes/progress.md)。

## 截图

<p align="center"><img src="docs/demo.gif" width="640" alt="Mornlea 演示"></p>

以下静态截图取自无窗口视觉验证基线（640×360，`make visual-check` 生成），从左到右、从上到下依次为：正午地形、橡树林、方块光房间与材料展示。

<table>
  <tr>
    <td><img src="cmd/mornlea/testdata/golden/terrain-noon.png" width="380" alt="terrain-noon 正午地形"></td>
    <td><img src="cmd/mornlea/testdata/golden/oak-grove.png" width="380" alt="oak-grove 橡树林"></td>
  </tr>
  <tr>
    <td><img src="cmd/mornlea/testdata/golden/block-light-room.png" width="380" alt="block-light-room 方块光房间"></td>
    <td><img src="cmd/mornlea/testdata/golden/materials-showcase.png" width="380" alt="materials-showcase 材料展示"></td>
  </tr>
</table>

## 环境要求

- macOS；客户端入口目前使用 Darwin 构建约束，主要在 Apple Silicon 上验证；
- Go 1.26；
- 通过 rustup 安装的 Rust 1.97.1；
- 可用的 CGO 与 C 编译工具链，macOS 可通过 Xcode Command Line Tools 提供；
- Make。

如本机尚未安装命令行开发工具，可执行：

```bash
xcode-select --install
```

## 快速开始

```bash
git clone https://github.com/channing771/mornlea.git
cd mornlea
make run
```

首次启动需要生成并加载视距内的地形，耗时会明显长于后续运行。默认世界保存在 `worlds/default`。

使用独立存档目录启动：

```bash
make run ARGS="--world worlds/demo"
```

也可以先构建再运行：

```bash
make build
./bin/mornlea --world worlds/default
```

`make build` 会构建完整 Rust workspace，生成 `bin/mornlea`、`bin/mornlea-server`，并把 `libmornlea_engine.dylib` 复制到 `bin/`。图形客户端 `mornlea` 依赖同一次构建的 `mornlea_engine` 与 `mornlea_client`，专服 `mornlea-server` 只依赖同一次构建的 `mornlea_engine`；任一 binary 都不能与其他构建的依赖库混装。

## 常用命令

| 命令 | 说明 |
| --- | --- |
| `make help` | 显示 Makefile 帮助，也是默认目标 |
| `make run` | 运行客户端，可使用 `ARGS` 传递命令行参数 |
| `make build` | 构建两个 Go binary 与完整 Rust workspace，并复制 `bin/libmornlea_engine.dylib` |
| `make build-linux-server` | 构建 Linux amd64 `bin/mornlea-server` 与同目录 `bin/libmornlea_engine.so` |
| `make test` | 运行全部 Go 测试 |
| `make test-race` | 使用 race detector 运行全部 Go 测试 |
| `make test-multiplayer` | 运行 M3C 八玩家与 v6 报告测试 |
| `make bench-multiplayer` | 运行三组 M3C 多人微基准 |
| `make archcheck` | 验证依赖闭包与无图形服务端边界 |
| `make rust` | 构建固定 Rust 1.97.1 workspace 的两个 cdylib，`run`/`build`/`test` 等目标的前置依赖 |
| `make rust-check` | 运行 Rust 格式、clippy 与 workspace 单测 |
| `make fmt` | 格式化仓库内的 Rust 与 Go 源码 |
| `make clean` | 删除 `bin` 目录，不会删除世界存档 |

## 操作方式

| 输入 | 操作 |
| --- | --- |
| `W` / `A` / `S` / `D` | 移动 |
| 空格 | 跳跃 |
| 移动鼠标 | 转动视角 |
| 按住鼠标左键 | 持续发送 primary action；服务端优先命中三格内未被固体遮挡的最近玩家并结算 2 点近战伤害（目标有 10 tick 冷却），没有合法玩家目标时才按权威位置、视角和选中工具推进采掘 |
| 按住鼠标右键 | 作为「使用」键：命中熔炉/箱子时请求打开容器，锄头命中泥土/草时翻地，种子或方块物品执行放置；手持面包且饥饿未满时持续默认 32 tick 完成一次权威进食 |
| `1` … `9` | 选择快捷栏栏位，由服务端确认后生效 |
| `E` | 打开/关闭背包，或关闭已打开的熔炉/箱子；界面打开时释放鼠标并抑制游戏输入。每名玩家同时只能查看一个容器，打开另一个会结束原有查看关系 |
| `Q` | 把权威选中快捷栏中的**一个**物品丢到脚下方块处；按住不重复，容器打开或鼠标未捕获时不发送 |
| `Enter` | 背包/容器关闭且调试面板未消费 Enter 时打开聊天；聊天打开时提交有效非空输入并重新捕获鼠标 |
| `Backspace` | 聊天打开时删除一个完整 Unicode rune |
| 容器界面内左键 | 两次点击栏位可整堆移动；熔炉界面用统一栏位 `0..38`，箱子界面用统一栏位 `0..62`（`0..35` 玩家物品、`36..62` 箱子 27 格）；普通背包可点击石砖、熔炉、铁块、石镐、铁镐、箱子、橡木木板、发光方块、石锄和铁锄十条固定配方 |
| `Esc` | 聊天打开时取消并清空输入；否则关闭容器并恢复捕获，或释放鼠标指针 |
| 释放指针后单击窗口 | 重新捕获鼠标指针 |

关闭游戏窗口时，内置服务端会停止并刷新待保存的世界数据。运行时生成的世界目录已在 `.gitignore` 中排除。

准星命中六格内可显示的注册方块时，画面上会显示深度正确的细轮廓与方块中文名提示，作为本地即时反馈，不参与权威裁决。

### 资源与熔炉

新生成且未保存过的石头中，煤矿只出现在 `Y < 96`，约为 `1/2048`；铁矿只出现在 `Y < 48`，约为 `1/4096`，两者重合时铁矿优先。已保存区块不会被批量改写。

对准已放置的熔炉右键，收到服务端确认后会打开统一 `0..38` 栏位界面：`0..35` 是玩家物品，36 是原料输入，37 是煤炭燃料，38 是产物输出。熔炼固定三条映射：粗铁→铁锭、沙子→玻璃、黏土块→砖块。一个产物需要 200 个活动 tick，一个煤炭提供 1600 个燃烧 tick，可恰好支持 8 个产物；输入无效或输出已满时进度与燃料会一起暂停，输入种类切换时清零进度并保留燃烧时间。多名玩家同时查看的是同一份世界状态，客户端不预测物品或进度。

### 自然材料与橡树

新生成区块除煤矿、铁矿外还会按种子确定性生成自然材料：低处地表为沙子，其中噪声选中的黏土与浅层砾石穿插出现；高处地表为雪块。橡树按固定候选格生成在草方块上（树干高 4–6），产出橡木原木与树叶。已保存区块不会被批量改写。

旧世界可以在停服后离线迁移到同一套自然材料规则：

```bash
mornlea-server --world <世界目录> --migrate-materials --backup <备份目录>
```

迁移必须先创建可验证的完整备份，只重算石头、泥土、草、沙子、砾石、黏土和雪块七种自然值，矿石、容器、掉落物等其他状态保持不变；中断后可用相同参数从进度处续跑。

### 水与农业

`fluidEnabled` 默认开启；Rust worldgen 在海平面及以下注入水源，服务端用水源加七个流动等级在活动兴趣范围内权威推进。水没有碰撞体且不可作为物品放置；玩家浸没后使用水中物理，按住跳跃持续上浮，落水会消除本次摔落伤害。

石锄与铁锄可把上方为空气的泥土或草翻成耕地，成功翻地消耗一点耐久；小麦种子放在耕地上形成八阶段作物。活动区段内露天且位于湿耕地上的作物按固定预算生长；成熟小麦收获 1 个小麦与 2 颗种子，未成熟作物掉 1 颗种子。种子、小麦和面包进入权威背包与掉落物规则，伙伴明确不执行种植或收获。

### 箱子

箱子是由区块拥有的固定容量共享存储，服务端固定配方 `8 石头 → 1 箱子`；箱子可放置、可挖掘，挖掘耗时与熔炉相同（裸手/普通物品 `30` tick、石镐 `15` tick、铁镐 `8` tick，至少石镐才能取得掉落）。每个区块最多同时存在 16 个活动箱子，每个箱子固定 27 格，接受任何已注册物品（含带耐久的工具）；第 17 个箱子会被原子拒绝且不消耗玩家物品。

对准已放置的箱子右键，收到服务端确认后会打开统一 `0..62` 栏位界面：`0..35` 是玩家物品，`36..62` 是箱子的 27 个格子，接受任何已注册物品且没有熔炉那样的物品类型限制。每名玩家同时只能查看一个容器：打开箱子会结束正在查看的熔炉，反之亦然；多名玩家可以同时查看同一个箱子并看到相同的最终状态。破坏箱子前，服务端会在掉落物副本上按箱子本体、再按 `36..62` 顺序预演全部非空格，只有全部堆都能完整放入所属区块的掉落物槽时才清除方块、停用槽位并一次性提交，容量不足时方块、箱子内容与掉落物都保持不变。

图形背包的固定合成面板有石砖、熔炉、铁块、石镐、铁镐、箱子、橡木木板、发光方块、石锄和铁锄十条可点击入口。服务端 `internal/core` 的稳定 `RecipeID` 表共有 11 条；第 11 条是 `3 小麦 → 1 面包`，不在图形背包的十行面板内。当前不支持大箱子合并、箱子命名、排序、快捷搬运、拆分堆、漏斗、比较器、潜行放置或自动化。

### 权威采掘与工具

按住左键时，客户端只发送持续 primary action；服务端每个 20 Hz tick 先按权威位置与视角寻找三格内的合法玩家近战目标，没有命中玩家时才用当前选中栏位和六格射线推进采掘。松键、换目标、换方块、换工具、超距、打开容器、断线或 reset 都会清除旧采掘进度；每名玩家的进度独立，不会共享或累加。HUD 只显示服务端确认的采掘进度：绿色表示完成后可掉落，橙色表示会破坏但不会掉落。

| 方块 | 空手/普通物品 | 石镐 | 铁镐 | 掉落条件 |
| --- | --- | --- | --- | --- |
| 泥土、草方块 | 5 tick | 5 tick | 5 tick | 任意手持状态均掉落 |
| 石头 | 30 tick | 15 tick | 8 tick | 仅空手、石镐或铁镐掉落；普通物品不是空手 |
| 石砖、熔炉、箱子、煤矿、铁矿、发光块 | 30 tick | 15 tick | 8 tick | 至少石镐才掉落方块或矿物；箱子掉落时本体与全部格子内容原子一起掉落，容量不足则整体拒绝且方块保持不变；错误工具破坏熔炉时仍保全内部物品 |
| 铁块 | 40 tick | 20 tick | 10 tick | 仅铁镐掉落 |
| 基岩 | 不推进 | 不推进 | 不推进 | 不可破坏 |

石质镐/锄最大耐久为 `131`，铁质镐/锄为 `250`。服务端只在工具确实完成一次有效作用后扣恰好 1 点耐久：成功破坏方块或成功翻地会磨损，所有拒绝路径均不磨损；完好锄头收获作物是唯一豁免，不扣耐久。最后一点耐久仍会完成本次作用，随后工具转为对应的损坏物品。

损坏工具采掘时等同空手，可以保留、在背包中移动或丢弃，但不能参与合成、熔炼、修复或回收；当前没有补充既有工具耐久的途径。服务端 11 条固定配方为：`4 石头 → 4 石砖`、`8 石头 → 1 熔炉`、`9 铁锭 → 1 铁块`、`3 石头 → 1 石镐`、`3 铁锭 → 1 铁镐`、`8 石头 → 1 箱子`、`1 橡木原木 → 4 橡木木板`、`4 玻璃 → 4 发光方块`、`2 石头 → 1 石锄`、`2 铁锭 → 1 铁锄`、`3 小麦 → 1 面包`。新合成的工具为满耐久；可用工具与对应损坏物品每格最多一个，其他当前物品每格最多 64 个。快捷栏只为磨损中且仍可用的工具显示耐久条。

### 权威生存状态与死亡

生命值与饥饿三层状态由服务端唯一权威并随玩家 schema v7 持久化；氧气是服务端权威的瞬态值。客户端不预测这些状态，只显示已确认值：生命心形位于快捷栏左侧，饥饿鸡腿位于右侧，氧气未满时显示气泡。

摔落安全高度为 3 格：系统追踪玩家离地后到达过的最高点，在"上一 tick 不在地面、这一 tick 在地面"的边沿结算一次伤害，公式为 `伤害 = floor(下落高度) − 3`，结果为负值时不扣血。例如从 4 格摔落扣 1 点，满血玩家从 23 格摔落会摔死（伤害恰为 20）；正常跳跃或落地后原地停留不会重复扣血；传送、重生和维度 reset 都会把峰值高度重置为当前高度。

自然回血还受饥饿门控：最后一次受伤后连续 `100` tick 未再受伤，且饥饿值至少为默认阈值 `18` 时，此后每 `40` tick 回 1 点直到满血；饥饿不足时计时照常推进但不产生回复，每回复一点会增加固定疲劳，任何伤害都会清零计时。

眼睛浸没时，氧气从 `300` 每 tick 减 1；归零后默认每 `20` tick 经统一伤害入口扣 1 点生命，出水立即回满。饥饿值范围为 `0..20`，饱和度与疲劳使用整数千分位推进；面包是当前唯一食物，连续使用默认 `32` tick 后原子扣一件，饥饿增加 5 但钳到 20，饱和度增加 6000 千分位但钳到「新饥饿值 × 1000」。饥饿为 0 时默认每 `80` tick 扣 1 点生命，但停在 1 点而不致死。

玩家近战/PvP 复用左键 primary action：同维 active 玩家中取三格射线最近命中，固体方块遮挡、流体穿透，每次成功命中造成 2 点伤害并给目标 10 tick 冷却。同 tick 意图先冻结再结算，Memory 与 TCP 使用同一路径。

Darwin 图形客户端的权威确认 cue 包括采掘、放置、进食与受伤：采掘在已确认目标的空气增量实际应用后触发，进食在食物扣减与饥饿增加均确认后触发，受伤在本玩家权威生命值下降时触发（包括其他玩家造成的已确认伤害）。放置 cue 只由 v26 回给放置发起会话的新鲜 `PlaceBlockSucceeded(sequence)` 触发；重复或旧 sequence、放置拒绝、仅有方块/背包变化等无关状态，以及其他会话的放置确认都不会触发本地放置 cue。

背包与容器内的有效本地 UI 操作会即时播放 `CueUIClick`，包括点击可合成按钮、首次选择栏位来源、点击同格取消选择，以及成功发送栏位移动请求；面板空白区域、禁用或无效 UI 与发送失败保持静音。这类本地 UI cue 不等待权威确认，后续服务端拒绝不会反向撤销已经播放的声音。音频设备不可用时全部 cue 均无声降级，capture、benchmark 与专服不请求音频设备。

生命值降到 0 时，系统在同一权威 tick 内完成死亡结算：36 个物品栏格逐格掉进世界（放置成功才清空该格，物品不会被静默销毁）、玩家传回出生锚点、生命值回满、速度归零。死亡掉落的拾取延迟默认为 `40` 个 active tick。掉落物从死亡所在区块开始按半径 0、1、2… 环形外扩寻找空位，只写入已加载且 Ready 的区块；扫完全部已加载区块仍放不下的格会保留在背包中跟随重生。外部观察不到生命值为 0 的中间状态，发布的永远是重生后的满血状态。

当前仍无怪物，以及窒息、岩浆、火焰等伤害源；也没有床、自定义重生点或专门的死亡界面。确认生命值下降时会出现短暂红色边缘反馈并播放本地受伤提示音，死亡后直接在出生锚点满血重生。

## 局域网多人联机（M3 已完成）

专用服务端、两个客户端的人工验收命令、身份语义、断线/关服存档行为与安全边界见[局域网专用服务端指南](docs/notes/lan-server.md)。最简启动方式：

```bash
go run ./cmd/mornlea-server --listen :25565 --world worlds/lan --seed 42 --max-players 8
go run ./cmd/mornlea --connect 127.0.0.1:25565 --name 玩家甲
```

## 配置文件与调试面板

`mornlea`/`mornlea-server` 启动时读取同一份 JSON 配置文件，默认路径 `os.UserConfigDir()/mornlea/config.json`（与 `profile.json` 同目录），可用 `--config <path>` 覆盖。文件不存在时全部使用编译默认值，**不会自动创建文件**；字段缺失取默认值，只有声明为可钳制的数值调参越界时才会钳制并 `slog.Warn`，未知字段被忽略。JSON 语法错误、不认识的 `version`、字段类型错误、非法 `windowSize`，以及超长或含 CR/LF 的 `texturePackPath` 都会导致加载失败。旧默认目录的迁移规则见[改名迁移说明](docs/notes/mornlea-migration.md)。

本地材质覆盖的配置与 v1 目录格式见[材质包说明](docs/texture-packs.md)。

### 客户端设置

普通本地客户端的主菜单「设置」页只提供三项：总音量、材质包目录和窗口大小。窗口大小是固定的 16:9 预设，只能选择 `640x360`、`960x540` 或 `1280x720`；`windowSize` 缺失时默认使用 `1280x720`。对应的最小配置示例如下：

```json
{
  "version": 1,
  "audioVolume": 0.7,
  "texturePackPath": "packs/my-pack",
  "windowSize": "1280x720"
}
```

`audioVolume` 的范围是 `0..1`。设置页显示并保存 `texturePackPath` 原文；该值必须是单行、最多 1024 个 UTF-8 字节，相对路径按配置文件所在目录解析。保存成功后，音量和窗口大小立即应用于当前进程；发生变化且非空的材质包候选会在写入前完整校验，但当前 atlas 不会热替换，保存的材质包从下次启动起加载。保存失败会保留页面上的草稿，不会部分改写配置或当前运行状态。

除三项客户端设置外，配置还包含四组运行参数、一个可选 AI 组和顶层 `fluidEnabled`：

| 分组 | 内容 |
| --- | --- |
| `logging` | 全局日志等级 `default` 与按模块覆盖的 `modules`（键为包路径末段，如 `render`、`storage`），等级为 `debug`/`info`/`warn`/`error` |
| `physics` | 重力、行走/跳跃速度、加减速度、终端下落速度、视线高度等运动常量 |
| `sim` | 交互距离、掉落物寿命与拾取延迟、生命回复间隔、出生半径、熔炉冶炼/燃烧 tick 等权威模拟常量 |
| `render` | `viewDistance`（重启生效，仅配置文件可改）、`fovDegrees`、`mouseSensitivity` |
| `ai` | OpenAI-compatible `endpoint`/`model`、密钥环境变量名 `apiKeyEnv`、任务超时，以及 `0..4` 个带 canonical UUIDv4 `id`、唯一 `name` 和可选 `persona` 的伙伴定义；列表缺失或为空时 AI 关闭 |

`fluidEnabled` 默认 `true`，控制新生成世界是否注水；benchmark 固定不注水，capture 固定使用编译默认值，两条自动化路径都不随本机配置漂移。

**`mouseSensitivity` 是无量纲倍率**，默认 `1`，区间 `[0.1, 5]`；实际弧度/像素系数是代码内基线常量 `baseMouseSensitivity = 0.002`（`cmd/mornlea/main.go`），运行时灵敏度 = 该基线 × 配置里的倍率。

`--dev` 只控制游戏内调试面板是否可用，**不控制配置文件是否生效**：配置文件里调过的值无论是否加 `--dev` 都会生效；不加 `--dev` 时只是看不到、也改不了面板。

加 `--dev` 后按 `F3` 切换面板显隐，面板按分组显示段头（如 `── physics ──`）加裸字段名（如 `gravity`，而非 `physics.gravity`）；方向键选行/步进，`Shift` 粗调 ×10，`Alt` 细调 ×0.1，`Enter` 重置当前行，`F5` 保存到配置文件，`F6` 全部重置。联机（`--connect`）时 `physics`/`sim` 两组灰显只读并标注服务端控制，`render` 组仍可写；`viewDistance` 无论是否联机都只读，只能通过配置文件调整并重启生效。

普通本地模式的内置服务端和 `mornlea-server` 都消费同一配置中的 `logging`、`physics`、`sim`、`fluidEnabled` 与可选 `ai`；专用服务端不消费 `render`、`audioVolume`、`texturePackPath` 或 `windowSize`。`--connect` 客户端不会按本机 `ai` 配置创建伙伴，只呈现远端服务端通过协议 v26 发布的伙伴。

**联机时本机配置文件里的 `physics`/`sim` 必须与服务端所用的一致**，否则客户端预测会与权威模拟持续分歧（位置回弹）。面板在联机时锁住这两组，但配置文件不受该锁约束——它始终生效。局域网下让 `mornlea` 与 `mornlea-server` 读同一份配置文件即可满足这条要求；`mornlea` 检测到"`--connect` + 这两组偏离默认值"时会打印一条 `slog.Warn` 提醒。

## 视觉验证

`--capture <目录>` 让 `mornlea` 走无头 offscreen 路径，依次跑完 `cmd/mornlea/capture.go` 里表驱动的 19 个固定场景：`terrain-noon`、`hud-hotbar-health`、`hud-survival-feedback`、`avatar-nametag`、`inventory-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`water-surface-slope`、`main-menu`、`settings-menu`、`far-horizon`、`water-underwater`。每张 640×360 PNG 都与 `cmd/mornlea/testdata/golden/` 下的基线比对；双阈值是单像素最大通道差与差异像素占比（定义见 `cmd/mornlea/visual_compare.go`），两项都在阈值内才算通过。具体数值与实测漂移分布见[视觉验证设计文档](docs/superpowers/specs/2026-08-07-visual-verification-design.md) §6。

```bash
make visual-check              # 抓帧并与基线比对，输出目录默认 build/visual
VISUAL_OUT=/tmp/shots make visual-check   # 自定义输出目录
make visual-update             # 重新生成基线，写入 cmd/mornlea/testdata/golden/
```

比对失败时，实拍图与差异图（差异像素涂红，其余像素按基线压暗）会写进输出目录的 `<场景>-actual.png` 与 `<场景>-diff.png`，供人眼定位问题区域。

**什么时候该跑 `make visual-update`**：只在渲染行为**有意**改变、且已经人工打开新产出的 PNG 确认画面正确之后。golden 基线一旦冻错，后续所有比对都在维护一个错误的基线。

**什么时候不该跑**：比对红了但看不出改动位置或原因、又或者只是想让门禁通过。红灯是要查的信号，不是要覆盖的噪声——`internal/render`、Rust renderer 或 shader 相关改动的评审必须实际打开差异图，不能只看比对器的数值结论。视觉验证暂不接入 `go test ./...` 或 CI（需要 GPU，且尚未在共享 runner 上积累稳定性数据），只作为本地 make target 与人工评审步骤存在。

## 项目结构

```text
.
├── cmd/
│   ├── mornlea/       游戏客户端与内置服务端装配
│   ├── mornlea-server/ 无图形 TCP 专用服务端
│   ├── gfxspike/      Rust renderer 地形渲染验证程序
│   └── perfcheck/     性能报告比较工具
├── engine/
│   └── crates/
│       ├── mornlea_engine/ 固定 Rust 1.97.1 cdylib：mesh/light/collision/raycast/physics/worldgen
│       └── mornlea_client/ Darwin 窗口、事件循环与全部 GPU 渲染
├── internal/
│   ├── core/          公共领域类型与 native raycast batch 驱动
│   ├── companion/     独立伙伴身份、静态定义与身体类型
│   ├── profile/       本机稳定玩家身份与档案
│   ├── config/        共享 JSON 配置加载与校验
│   ├── logging/       模块化日志
│   ├── audio/         Darwin 本地程序化提示音
│   ├── fluid/         有界权威流体更新队列
│   ├── lod/           远环 tile 调度与 CPU 编码
│   ├── world/         区块和世界数据模型
│   ├── worldgen/      worldgen seed→perm 播种、Rust 调用与区块回写
│   ├── physics/       玩家运动与碰撞
│   ├── sim/           权威世界模拟
│   ├── server/        服务端 Host、会话、发布与玩家持久化
│   ├── network/       二进制协议、登录状态机与 Memory/TCP 传输
│   ├── storage/       世界、区域文件与玩家状态持久化
│   ├── client/        输入、相机、预测、窗口/client ABI 与客户端镜像
│   ├── mesh/          区块网格生产 API（实现位于 Rust cdylib）
│   ├── nativeabi/     engine C ABI 的唯一 Go bridge
│   ├── render/        渲染 CPU 半部：布局、编码与上传调度
│   ├── assets/        方块定义与程序化材质
│   └── archcheck/     内部包依赖方向门禁测试
├── scripts/agent-hooks/  Claude Code 与 Codex 共用的自动 Hook 守卫
└── docs/              设计、实施计划、性能记录与实现进度
```

整体架构与技术选型见[项目设计文档](docs/superpowers/specs/2026-07-26-minecraft-go-design.md)。其余文档导览：

- [实现进度](docs/notes/progress.md)：已交付里程碑、当前基线与下一步方向；
- [局域网专用服务端](docs/notes/lan-server.md)：`mornlea-server` 的启动、身份、存档与安全边界；
- [改名迁移说明](docs/notes/mornlea-migration.md)：M4Q 改名后的名称与本机数据迁移；
- [性能基线](docs/notes/perf-baseline.md)（M2）与 [Apple M5 基线](docs/notes/perf-baseline-m5.md)：性能证据链；
- [OpenSpec 工作流](docs/openspec.md)：OpenSpec 使用与自动 Hook 约束；
- `docs/superpowers/specs/`、`docs/superpowers/plans/`：历史设计背景与决策记录，不代表当前实现。

## Rust 与 Go 的职责划分

固定 Rust 1.97.1 workspace 同时包含 `mornlea_engine` 与 `mornlea_client` 两个 `cdylib`，形成两条独立的 C ABI 与 release-unit 边界：`engine/include/mornlea_engine.h` 定义 engine ABI v6，`engine/include/mornlea_client.h` 定义 client ABI v9。macOS 图形 `mornlea` 必须配套同一次构建的 engine 与 client 库；`mornlea-server` 只配套同一次构建的 engine 库。任一 ABI 均不可跨版本混装。

| 语言 / 组件 | 职责 |
| --- | --- |
| Rust `mornlea_engine` | mesh/light、collision resolver、raycast、physics tick 积分与 worldgen（地形、矿石、橡树、海平面注水和远环壳）的**唯一生产实现**；panic 不穿过 ABI，非法输入在发布结果前拒绝。 |
| Rust `mornlea_client` | Darwin 客户端窗口、事件循环与全部 GPU 渲染（terrain、sky、云、culling、HiZ、实体、文本、HUD 与 egui；窗口 surface 与离屏）的**唯一生产实现**。 |
| Go | 拥有 app、world、sim、network、storage，以及 render 的 CPU 半部（布局、编码与上传调度）、客户端镜像与预测、资产与配置、物理 state/input/tunable/snapshot 编码、yaw 三角与 prism 构建、worldgen seed→perm 播种和 `world.Chunk` 回写。Go 不接触 GPU API，也没有生产 fallback。 |

边界规则：

- `internal/nativeabi` 是 engine C ABI 的唯一 Go bridge；`internal/mesh`、`internal/physics` 与 `internal/core` 通过领域 API 调用它。client C ABI 则只由 `internal/client` 接触，其他 Go 层只使用其 `Window`、`Renderer` 等领域接口；
- 调用结束后任何语言都不得保留对方指针；旧 Go 积分、worldgen、网格、光照、碰撞与 raycast 实现仅作测试 oracle，不是生产 fallback；
- 网格与光照结果不进入网络协议或存档；collision 被客户端预测和服务端权威模拟共用。专用服务端因此使用 `mornlea_engine`，但不链接 `mornlea_client` 或图形栈。

构建：`make run`/`build`/`test` 等目标先自动构建完整 Rust workspace（`rust-toolchain.toml` 固定 1.97.1）。macOS 图形客户端同时跨 engine ABI v6 与 client ABI v9 两条边界；`make build` 生成两个 Go binary，并把 `libmornlea_engine.dylib` 复制到 `bin/`，图形 binary 同时链接 workspace 构建的 `mornlea_client`，专服 binary 只链接 `mornlea_engine`。`make build-linux-server` 只打包 Linux amd64 专服与相邻 `libmornlea_engine.so`，通过 `$ORIGIN` 加载。两条 ABI 各自都是不可跨版本混装的 release-unit 边界；专服依赖闭包不含 client/render（`make archcheck` 验证）。`make rust-check` 运行 Rust 格式、clippy 与单测。

## 当前限制

- 可运行客户端目前仅支持 macOS；
- TCP 多人联机仅面向可信局域网，协议没有认证或加密，不能暴露到公网；
- 尚无服务器发现、游戏内连接菜单或断线自动重连；
- 图形客户端默认在完整程序化注册表上应用内嵌的 Pixel Perfection 子集，未映射 layer 回退到程序化材质；仓库不包含官方 Minecraft 美术资源；
- 快捷栏固定 9 格；可用工具与对应损坏物品每格最多 1 个，其他当前物品每格最多 64 个；快捷栏装满时仍可挖掘，物品留在地面；
- 掉落物属于所属区块：每区块最多 32 堆；可堆叠物品在同位置最多合并到 64，工具与损坏形态每格上限为 1、不会合并；区块掉落物已满时可采集方块的采掘会被拒绝且方块保持不变；
- 采掘与方块破坏掉落的拾取延迟编译默认值为 `10` 个 active tick，主动丢弃与死亡掉落则默认为 `40` 个 active tick；延迟结束后，默认拾取半径 `1.25` 格内的玩家可以拾取。掉落物的编译默认寿命为 `6000` 个 active tick，且只有玩家附近（区块半径 2）的掉落物才会推进寿命；
- 拾取先填快捷栏再填背包（同类未满格优先，其次最低空格），两者都满时剩余物品留在地面；
- 背包界面只支持整堆移动：空目标接收整堆、同物品合并到 64 并保留余量、不同物品交换；尚无拆分堆、拖拽与快捷搬运；
- 服务端固定配方表有 11 条单输入配方，图形背包显示其中前 10 条，服务端第 11 条为 `3 小麦 → 1 面包`；仍无多种原料组合的配方、合成网格、工作台、批量合成或队列；
- 熔炉固定三条熔炼映射（粗铁→铁锭、沙子→玻璃、黏土块→砖块）且只接受煤炭燃料，每区块最多 32 个；工具尚无修复或损坏物品回收，采掘尚无多人共享进度和裂纹贴图，熔炉尚无多燃料、经验、自动化或离线进度补算；
- 每区块最多 16 个箱子，每个箱子固定 27 格且接受任何已注册物品；每名玩家同时只能查看一个容器，打开箱子会结束熔炉查看关系，反之亦然；尚无大箱子合并、命名、排序、快捷搬运、拆分堆、漏斗、比较器、潜行放置或任何自动化；
- 世界时间由服务端权威推进，一个昼夜固定为 `24000` tick，所有客户端从权威玩家状态观察同一相位；地形、远端玩家、掉落物和天空背景按固定曲线随昼夜变化，HUD 与昵称不受世界明暗影响；
- 天空光和静态方块光都由客户端从权威方块镜像派生：天空光直射起点为 `15`，穿过流体每格额外衰减 1、穿过植物不额外衰减；方块光从发光块以 `15` 起步，模型保持不穿过水或玻璃，未知邻区按阻挡和黑暗处理。发光块可放置、挖回并通过 `4 玻璃 → 4 发光方块` 合成；尚无真实火把、彩色或动态光、动态阴影与天气；
- 水默认由 worldgen 注入，并以水源加七级流体、斜水面、半透明 water pass、水中物理、氧气与溺水形成权威闭环；水不进物品表，尚无水桶或玩家放置流体；
- 农业已有锄头翻地、耕地干湿、八阶段小麦、固定预算生长、种植/收获与面包服务端配方；伙伴不种植或收获，图形背包也不显示第 11 条面包配方；
- 圆石、平滑石、沙子、砾石、橡木原木、橡木木板、树叶、玻璃、砖块、白色羊毛、红色瓦块、黏土、雪块和苔藓圆石均可权威放置、采掘并掉落自身；沙子、砾石、黏土、雪块会按种子在新生成区块自然出现，橡树确定性生成橡木原木与树叶，旧世界可用 `mornlea-server --migrate-materials --backup` 离线迁移；存档缺失的新玩家会在背包前 14 格各获得 64 个材料，并在第 15 格获得 64 颗小麦种子，已有玩家不会补发。圆石、平滑石、玻璃、砖块、羊毛、红瓦、苔藓圆石等其余材料不自然生成；材料尚无重力、腐烂或融化行为；
- 主动丢弃只支持单件原地转移：服务端在同一权威 tick 内校验玩家、选中栏位、脚底区块与每区块 32 个掉落物槽后原子扣除一件并在脚下创建掉落物，默认 `40` 个活动 tick 内不可拾取（采掘与方块破坏产生的掉落物默认为 `10`）。合并到同位置旧堆时保留其 ID 与寿命时间线，只把拾取禁止窗口延长到较长的来源延迟；所有已注册物品都可作为掉落物传播；
- 尚无整组丢弃、丢弃数量选择、投掷速度、重力与水平移动、所有者专属拾取与客户端预测；
- 生命值、氧气、饥饿/饱和/疲劳、面包进食、自然回血门控、饥饿伤害、溺水、摔落与玩家近战/PvP 均由服务端权威结算；Darwin 客户端为采掘/放置/进食/受伤播放权威确认 cue，并为有效的背包/容器本地 UI 操作即时播放 `CueUIClick`。当前仍无怪物、窒息/岩浆/火焰伤害、床、自定义重生点或专门的死亡界面。

## 兼容性与升级

- 线上协议为 v26；所有不匹配版本都会在握手阶段、进入 Play 前被稳定拒绝，不提供版本协商或降级解码。最近一次 v25→v26 迁移新增仅回发成功放置发起会话的 `PlaceBlockSucceeded(sequence)` 序号确认；
- 世界 metadata 保持 v2，记录绝对世界时间。既有 v1 世界可直接打开，世界时间从 `0` 开始，并在下一次正常自动保存或关服时写为 v2；只认识旧版本的程序遇到未来 metadata 必须稳定拒绝且不得覆盖原文件；
- 玩家存档保持 schema v7，区块存档保持 schema v9；受支持的旧版本沿既有迁移链读取，未来版本必须稳定拒绝且不得覆盖。伙伴状态独立写入世界根目录的 `companions.ai` schema v4，active 与 inactive 身体记录合计最多 64 条；active 记录可持久化当前任务、FIFO 与近期对话摘要，名称和生效 persona 始终来自当前配置；
- 列顶高度表、天空光和静态方块光仍只从权威方块镜像取得，不写入区块、玩家或伙伴存档，也不进入网络 payload；程序化天空仍只消费既有权威世界时间；
- **备份与回退**：升级前必须正常关服，等待玩家、伙伴与世界存储刷写完成并备份完整世界目录，再启动 v26 程序。回退时必须先停服，再恢复升级前的完整备份；不承诺把 schema v9 区块、schema v7 玩家档、`companions.ai` v4 或新物品降级写回，不能让旧程序直接打开已升级目录后继续写入。异常退出时玩家、伙伴与区块文件各自原子，但它们之间没有跨文件事务；
- benchmark producer 为 scenario v19，固定输入仍是七名远端玩家、零伙伴，且 benchmark 世界仍显式钉死不注水、不含农业方块；版本变化记录的是被测进程本身的改变（HUD 新增饥饿条使 Hotbar HUD 固定上传布局再次移动——quad 容量 247→267、glyph offset 12288→13312、总容量 45888→46912 bytes，HUD 图集新增空/满两列鸡腿，权威 tick 多出饥饿三层状态的推进与结算）。当前唯一显式迁移是 `18:19`，v6..v18 历史报告仍可同版本读取。M5A v16 Memory/TCP 报告仅为 record-only 证据，M2 v15 与 M5 v14 baseline JSON 未提升；性能数值只记录，报告结构、身份、真实 overflow、数据丢失和 I/O 错误仍失败。跨 transport 比较只在显式请求时执行。

## 使用 OpenSpec 开发

本项目使用 OpenSpec 管理复杂变更。一次变更通常包含：

- `proposal.md`：为什么做、范围和非目标；
- `specs/<capability>/spec.md`：可观察行为和验收场景；
- `design.md`：技术方案、边界和取舍；
- `tasks.md`：可执行、可验证的任务清单。

新功能、跨包重构、网络协议或存档格式变化，以及影响架构和性能契约的修改，默认使用 OpenSpec。拼写、格式等低风险小改动可以直接完成。

### 1. 安装

需要 Node.js 20.19 或更高版本：

```bash
npm install -g @fission-ai/openspec@1.7.0
openspec --version
```

仓库已经为 Claude Code 和 Codex 生成集成文件，不需要再次运行 `openspec init`。

### 2. 探索和提案

命令输入在 AI 对话中，不是在 shell 中执行：

```text
Claude Code: /opsx:explore
Claude Code: /opsx:propose add-example-feature

Codex:       $openspec-explore
Codex:       $openspec-propose add-example-feature
```

提案生成到 `openspec/changes/add-example-feature/`。编码前应人工检查目标、非目标、Requirement/Scenario、技术方案和任务拆分；所有产物都是普通 Markdown，可以直接修改。

### 3. 按规格实现

```text
Claude Code: /opsx:apply
Codex:       $openspec-apply-change
```

AI 会按照 `tasks.md` 实现并验证。需求或设计发生变化时，先更新 change 产物，保持规格与实现一致：

```text
Claude Code: /opsx:update
Codex:       $openspec-update-change
```

### 4. 校验和归档

实现完成后在终端运行：

```bash
openspec status --change add-example-feature
openspec validate --all --strict --no-interactive
go test ./... -race
go vet ./...
```

确认实现、规格和任务状态一致后，在 AI 对话中归档：

```text
Claude Code: /opsx:archive
Codex:       $openspec-archive-change
```

归档会把 delta specs 合入 `openspec/specs/`，并把完整变更移至 `openspec/changes/archive/`。这些文件应和代码一起提交到版本控制。

`sync` 用于长期变更在归档前提前同步主规格：

```text
Claude Code: /opsx:sync
Codex:       $openspec-sync-specs
```

| 阶段 | Claude Code | Codex |
| --- | --- | --- |
| 探索 | `/opsx:explore` | `$openspec-explore` |
| 提案 | `/opsx:propose <change>` | `$openspec-propose <change>` |
| 实现 | `/opsx:apply` | `$openspec-apply-change` |
| 更新产物 | `/opsx:update` | `$openspec-update-change` |
| 提前同步主规格 | `/opsx:sync` | `$openspec-sync-specs` |
| 归档 | `/opsx:archive` | `$openspec-archive-change` |

项目上下文和产物约束见 [`openspec/config.yaml`](openspec/config.yaml)，AI 项目规则见 [`AGENTS.md`](AGENTS.md)，更完整的工作流说明见 [`docs/openspec.md`](docs/openspec.md)。

### 自动 Hooks 约束

Claude Code 与 Codex 共用 [`scripts/agent-hooks/guard.mjs`](scripts/agent-hooks/guard.mjs)，分别由 `.claude/settings.json` 和 `.codex/hooks.json` 加载：

- `PreToolUse`：阻止高破坏性 Git 命令、强制推送和宽范围递归删除；
- `PostToolUse`：编辑后检查改动中的 Go 文件是否已经 `gofmt`；
- `Stop`：检查 diff、OpenSpec、架构依赖、受影响包 race 测试和 `go vet`；
- 协议、存档、性能基线、架构边界或跨组件实现变更，没有完整 active OpenSpec change 时不得结束任务。

首次进入仓库后，在 Codex 或 Claude Code 中打开 `/hooks` 检查配置。Codex 会要求审查并信任项目 Hook；Hook 文件发生变化后需要重新审查。

Hook 策略测试：

```bash
node --test scripts/agent-hooks/guard.test.mjs
```

Hook 是机械护栏，不能替代 CI 和人工评审。详细触发规则与显式例外方式见 [`docs/openspec.md`](docs/openspec.md#自动-hook-约束)。
