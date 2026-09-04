## Why

生存循环有产出端（农业）与消耗端（饥饿），但肉类食物链缺失：`docs/feature-backlog.md` B-27 要求交付一种被动家畜、原肉、熟肉与一条熔炉配方。当前唯一生物是夜行者（纯色 Avatar），用户已批准首批做牛且材质必须贴图写实、不得使用纯色。

## What Changes

- 新增被动牛权威实体：昼间草地漫游、受击逃跑不反击、全服与每玩家双上限、死亡掉落 1 个生牛肉。
- 新增食物链：`ItemRawBeef` / `ItemCookedBeef`、熔炼映射 1 生→1 熟、饥饿值联动、中文名。
- 新增存档域 `passive_mobs.bin`（PMST 信封、schema v1），缺失视空集合，损坏/未来版启动失败且不覆盖。
- 新增协议三消息 `PassiveSpawn` / `PassiveState` / `PassiveDespawn`（S→C，ID 升序、≤64 条/包），客户端 latest-wins 镜像不预测。
- 新增贴图化呈现：Avatar 四足分支 + 每面 UV，`assets` 末位追加牛皮/牛头/生/熟 4 个 16×16 层，素材取自 CC0 外网转制（`mobs_animal` 牛 CC0 首选、OGA `16x16 Food` CC0 首选），`PROVENANCE.json` + `ATTRIBUTION.md` 逐文件溯源，程序化像素保留为回退；Rust `avatar.wgsl` 加 atlas 采样。
- 新增 capture 场景 `passive-herd` 一景。
- 协议 v32→v33；client ABI v14→v15（Avatar 实例加材质槽，布局待 design 锁定）。

非目标：繁殖/挤奶/骑乘、第二被动生物、ECS 共享抽象（B-26 待 B-27 落地后另行评估）、旧 golden 语义变更、任何 Mojang 版权素材。

## Capabilities

### New Capabilities

- `passive-cattle`: 被动牛权威行为、生成上限、漫游/逃跑、死亡掉落与食物链（原肉/熟肉/熔炼/饥饿）。
- `passive-mob-persistence`: `passive_mobs.bin` 文件格式、校验矩阵、存储契约与重启恢复语义。
- `passive-mob-protocol`: 被动三消息值域、排序约束、订阅发布与客户端镜像语义。
- `passive-cattle-presentation`: 牛四足贴图呈现、4 个 16×16 材质层、外网 CC0 素材转制与溯源、capture 场景。

### Modified Capabilities

- 无。夜行者三份主规格（`authoritative-hostile-nightwalker` / `hostile-mob-persistence` / `hostile-mob-protocol`）行为不变；协议与 ABI 升版只追加不重排。

## Impact

- 影响包：`packages/shared/core`（物品/食物/熔炼/中文名）、`packages/server/sim/entity`（新被动集合）、`packages/server/storage/passive`（新域）、`packages/shared/network`（新消息与 codec）、`packages/client/client`（被动镜像）、`packages/client/render`（Avatar 四足贴图分支）、`packages/client/assets`（4 新层 + 默认包转制素材）、`packages/engine/crates/mornlea_client`（shader/ABI）、`packages/client/cmd/mornlea/capture`（新场景）、`packages/audit`（依赖边登记）。
- 兼容性：协议只追加；新存档文件独立，旧世界缺失即空集合；Avatar 实例布局变化走 client ABI 升版，旧客户端不混装。
- 并发/性能：每 tick 生成候选验证与上限裁决有界（类比夜行者每 tick ≤1 候选），权威 tick 无无界工作；atlas 层数 +4，mip 链与上传预算同批核算。
