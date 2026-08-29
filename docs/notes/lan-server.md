# 局域网专用服务端

`mornlea-server` 是无渲染的 TCP 权威专用服务端。它不加载 client、render、gfx 或窗口栈，但使用 Rust `mornlea_engine` 的 collision、raycast、physics 与 worldgen；发布时必须把服务端 binary 与同一次构建的相邻 `libmornlea_engine.so` 作为不可跨版本混装的 release unit：

```sh
make build-linux-server
./bin/mornlea-server --listen :25565 --world worlds/lan --seed 42 --max-players 8
```

源码 checkout 首次运行需先构建 Rust engine：

```sh
make rust
go run ./cmd/mornlea-server --listen :25565 --world worlds/lan --seed 42 --max-players 8
```

命令行默认值为 `--listen :25565`、`--world worlds/default`、`--seed 42` 和 `--max-players 8`。`--max-players` 只接受 `1..8`；`--seed` 只用于创建新世界，已有世界使用其 metadata 中的种子。`--config <path>` 可指定配置文件。停服后可用以下命令离线迁移自然材料，`--backup` 必须是世界目录之外的完整备份目录：

```sh
./bin/mornlea-server --world <世界目录> --migrate-materials --backup <备份目录>
```

## 配置与连接

默认配置路径是 `os.UserConfigDir()/mornlea/config.json`，配置 schema 为 v1。默认文件不存在时使用编译默认值且不自动创建；只有默认路径的新文件缺失时才会从旧 `minecraft-go` 目录迁移。显式 `--config` 只读取指定路径，不参与默认目录迁移。

专用服务端消费配置中的 `logging`、`physics`、`sim`、`fluidEnabled` 和可选 `ai` 组，不消费 `render`、`audioVolume`、`texturePackPath` 或 `windowSize`。`fluidEnabled` 默认开启；benchmark 与 capture 是独立的固定配置路径，不随本机配置漂移。

同一局域网中的客户端连接服务器私有地址：

```sh
go run ./cmd/mornlea --connect 192.168.x.x:25565 --name Chen
```

若只允许本机连接，请明确监听 loopback：

```sh
go run ./cmd/mornlea-server --listen 127.0.0.1:25565 --world worlds/local --max-players 8
```

客户端首次运行会在本机 profile 中创建稳定 UUIDv4。之后改变 `--name` 只更新该 PlayerID 的显示名，不会创建新玩家；同一 PlayerID 同时登录会被拒绝。不同 PlayerID 可以使用相同显示名，因此昵称不是身份凭据，也不保证唯一。Memory 与 TCP 复用同一登录、会话和权威模拟路径。

## AI 伙伴

`ai.companions` 缺失、为空或 `ai` 组不存在时，AI 关闭，服务端不会读取或改写已有 `companions.ai`。非空列表最多四个伙伴，每个定义包含 canonical UUIDv4 `id`、大小写敏感且唯一的 `name`，以及可选 `persona`。伙伴不占用八名玩家的登录容量，也不进入玩家的生命、伤害、掉落或重生语义。

非空伙伴列表必须同时提供 `endpoint` 与 `model`。endpoint 只能是无 userinfo/query/fragment 的 `https`，或 loopback IP 字面量的 `http`；远程 `https` 还必须提供 `apiKeyEnv`，密钥只从该环境变量读入内存，不写入配置、日志或存档。`taskTimeoutMinutes` 为 `1..60`，缺省为 10。人设可内联（最多 4,096 bytes）或放在配置文件同目录的 `personas/<canonical 名称>.txt`，内联优先；人设只影响 Dialogue，不进入 Planner。

```json
{
  "version": 1,
  "ai": {
    "endpoint": "http://127.0.0.1:8080/v1",
    "model": "local-model",
    "taskTimeoutMinutes": 10,
    "companions": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "name": "阿木",
        "persona": "沉稳、简短地回答。"
      }
    ]
  }
}
```

伙伴身体、位置、朝向、36 格背包和选中栏位由服务端权威维护。玩家在聊天中发送 `@阿木 去挖矿`，服务端会在 tick 边界按名称精确寻址：合法指令广播给全部在线玩家并进入该伙伴最多 16 条的 FIFO；格式错误、未知伙伴或队列已满只回复发令者。任务支持 `go_to`、`follow`、`mine` 和 `place`，Planner 输出必须是受限 JSON，模型请求在 worker 中异步执行，不阻塞权威 tick，最多同时占用四个模型请求槽。

`@阿木 停止` 是唯一绕过 FIFO 的控制指令，且只对正在运行的持续 `follow` 任务生效；它会停止跟随并广播 `TaskStopped`。其他任务或空闲伙伴收到该指令时只向发令者返回 `NotFollowing`。Dialogue 只广播不超过 256 bytes 的模型台词；任务事实事件仍由服务端产生，模型失败只跳过台词，不改变任务、FIFO 或世界状态。

`companions.ai` 使用 schema v4，active 与 inactive 身体记录合计最多 64 条。仍在配置中的伙伴恢复身体、任务和 FIFO；暂时移出配置的记录保留为 inactive，但不注册到模拟，且不保存名称、人设或对话摘要。当前配置重新启用同一 ID 时，名称和生效人设仍来自配置。

## 持久化与安全关服

玩家存档为 schema v8，区块存档为 schema v9，世界 metadata 为 v3。当前玩家的最终位置、安全位置、个人重生点（如已设置）、生命值、饥饿值、饱和度、疲劳值以及完整 36 格物品状态（含选中栏位和工具耐久）会跨断线、重启恢复；合成网格是瞬态状态，关闭、断线或死亡前会先无损回收。区块同时保存方块、容器和掉落物状态；天空光与静态方块光由客户端从权威方块镜像派生，不写入存档。

支持的旧存档按既有迁移链只读加载并在后续保存时写出当前 schema；未来 schema、损坏文件、CRC 错误和超出上限的数据必须拒绝，不能覆盖正式文件。伙伴存档独立位于世界根目录 `companions.ai`，只在 AI 启用时加载和保存。

按 `Ctrl-C`（SIGINT）或向进程发送 SIGTERM 可正常关闭服务端。关闭会停止接纳连接，关闭待登录流，断开已登录会话，取消在途模型请求，完成最后一次权威 step，然后依次刷写玩家、区块、世界 metadata 和伙伴，最后同步并关闭世界存储、释放 `world.lock`。持久化失败会保留可重试状态，后续可再次执行关服；不要用强制杀进程替代正常关服。备份前必须等待进程退出，再复制整个世界目录。

## 当前玩法

所有世界写入和玩家状态由服务端在 20 Hz 权威 tick 中裁决。左键持续发送 primary action：服务端按统一战斗规则从三格内同维、存活且非自身的 player 或 hostile（夜行者）中选择目标；空手、普通物品和损坏剑命中造成 2 点伤害，木剑、石剑和铁剑分别造成 4/5/6 点伤害，玩家攻击受 10 tick 冷却约束，夜行者在 1.8 格内近战造成 3 点伤害。只有未成功实体命中时才按六格射线推进采掘。右键用于容器交互、翻地、放置和进食。服务端会重新验证位置、视角、触及距离、区块 Ready 状态、工具和背包，客户端不预先提交世界结果。

- 掉落物属于所属区块，每区块最多 32 堆；玩家只接收脚下区块半径 2 内的掉落物，单个玩家镜像最多 800 堆。快捷栏固定 9 格，连同 27 格背包共 36 格；普通物品每格最多 64 个，可用工具及损坏工具每格最多 1 个。
- 熔炉是区块共享状态，每区块最多 32 个；箱子每区块最多 16 个、每个 27 格。打开熔炉、箱子或工作台都经过服务端射线与触及距离校验，每名玩家同时只能查看一个容器。
- 挖掘、工具耐久、容器和作物掉落均走原子权威结算。石镐和石锄最大耐久为 `131`，铁镐和铁锄为 `250`；成功采掘或翻地通常扣 1 点耐久，最后一点耐久完成本次作用后转为损坏形态。
- 固定配方包括石砖、熔炉、铁块、石镐、铁镐、箱子、橡木木板、发光方块、石锄、铁锄、面包、木棍和工作台。个人合成网格是 2×2，打开工作台后为独立的 3×3 瞬态网格，网格内容不写入玩家存档。
- `fluidEnabled` 开启时，新世界会生成水源和七级流体。水没有碰撞体且不能作为物品放置；水面、流动、天空光衰减、水中物理和水下交互由服务端与 Rust 客户端共同呈现。
- 锄头可把泥土或草翻成干/湿耕地；小麦、马铃薯和胡萝卜各有八个阶段，可在露天且湿润时生长。成熟作物有确定性哈希掉落，未成熟作物至少返还一份自身种子；骨粉可让未成熟作物推进一阶段并消耗一份骨粉。
- 生命值上限为 20，饥饿值范围为 `0..20`，饱和度和疲劳由服务端整数结算并持久化；氧气从 `300` 开始但不写入存档。摔落、溺水、饥饿、玩家与夜行者近战和自然回血均受权威状态控制，饥饿为零时伤害最低停在 1 点；夜行者是当前唯一敌怪，会在夜间暗处生成并追逐近战，白昼露天时灼烧；床可设置个人重生点，重生时床失效则回落到世界出生锚点；当前仍没有火焰、岩浆、窒息或专门的死亡界面。

## 版本与限制

当前线上协议为 v32，服务端和客户端必须成套升级。v31 及其他不匹配版本会在握手阶段、进入 Play 前被稳定拒绝；不提供版本协商或降级解码。协议 v32 在 Play S→C registry 尾部追加 ID 25 的固定 10-byte 私有 `CombatHit`，携带最终 `ServerTick`、伤害和目标类型，只发送给成功玩家攻击的发起者。

版本矩阵：协议 v32、玩家 schema v8、区块 schema v9、世界 metadata v3、`companions.ai` schema v4、`hostile_mobs` schema v1、engine ABI v8、client ABI v11、benchmark scenario v20。升级前必须正常关服并备份完整世界目录；不要让旧程序直接打开已升级目录后继续写入。异常退出时各文件分别原子更新，但文件之间没有跨文件事务。

当前没有服务器发现、游戏内连接菜单或断线自动重连；图形客户端仅支持 macOS，Linux amd64 专服使用 CGO 并依赖相邻 `libmornlea_engine.so`。

> 安全警告：当前 TCP 没有认证，也没有加密。PlayerID 与昵称都由客户端声明，任何能连到监听地址的人都可尝试冒用身份或读取/篡改明文流量。仅可用于可信局域网；不要在路由器上做端口映射，也不要暴露到公网。
