# 任务：权威护甲

> 每任务组先写失败测试再实现（red → green → refactor）；测试与被测代码同目录；验证命令在任务组内全部通过并记入 ledger（按基线 SHA 复用）后才勾选。

- [ ] 1. core 护甲域与物品/配方编号
  - 文件：`packages/shared/core/armor.go`（新：槽位、件→槽映射、点数/耐久表、`MaxArmorPoints`、`ArmorPoints` 求和、`ReducedDamage`）、`packages/shared/core/item.go`（追加 `ItemIronHelmet..ItemIronBoots` 58..61，哨兵 → 62，注释声明堆叠 1/无放置/非掉落物）、`packages/shared/core/recipe.go`（`RecipeIronHelmet/Chestplate/Leggings/Boots` 四值与形状、产物映射）、同包测试（`armor_test.go` 公式向量/点数求和/损坏件 0 点；`item_test.go` 哨兵穷举自动覆盖；`recipe_test.go` 四形状匹配与不匹配）。
  - 验证：`go test ./packages/shared/core -race -count=1`。

- [ ] 2. 协议 v42
  - 文件：`packages/shared/network/protocol/packet.go`（`ProtocolVersion = 42`）、`message_player.go`（`ArmorPoints uint8` 尾部追加 + `Validate` 上界 `core.MaxArmorPoints`）、`message_command.go`（`EquipArmor{Sequence}` + `RejectNotArmor`）、`registry.go`（C→S ID 18 ↔ `EquipArmor`；拒绝原因 ID 表追加 `not_armor`）、对应 codec 编码/解码（载荷尾部追加与命令编解码）与往返/越界拒绝测试（`ArmorPoints = 21` 三处一致拒绝、`EquipArmor` 编解码往返、未知命令 ID 拒绝不变）。
  - 验证：`go test ./packages/shared/network -race -count=1`。

- [ ] 3. 玩家 schema v9
  - 文件：`packages/server/storage/player/player_types.go`（`StoredPlayer`/`PlayerSave` 装备字段）、`player_codec.go`（尾部 4×5 字节装备区编码/解码，每槽沿用背包格同一 5 字节栈编码、版本常量 v8→v9、写侧只出 v9）、`player_migration.go`（v1..v8 只读迁移：装备空）、`testdata`（新增 v9 fixture；保留 v8 fixture 供迁移断言）、迁移与 round-trip 测试（v8 加载装备空→再保存 v9 且原字节段保留；v9 往返逐位；未来版本拒绝）。
  - 验证：`go test ./packages/server/storage/... -race -count=1`。

- [ ] 4. sim 装备、减免、耐久与死亡
  - 文件：`packages/server/sim/entity/player.go`（`armor [4]core.ItemStack` 字段、快照/恢复、`PlayerHash` 追加装备区）、`packages/server/sim/entity/armor.go`（新：`EquipArmor` 结算原子互换 + `not_armor` 判定、减免点数冻结与耐久消耗）、`packages/server/sim/entity/combat.go`（快照结构 `armorPoints`、`settleCombatIntent` 目标玩家分支调用 `core.ReducedDamage` 后再 `applyDamage`）、`packages/server/sim/entity/death.go`（死亡掉落含四槽护甲）、同包测试（互换三态：空槽/占用/非护甲拒绝；减免向量与击退不变；摔落不减免；受击全件耗 1 耐久与 0 点不耗；死亡掉装备；`PlayerHash` 含装备）。
  - 验证：`go test ./packages/server/sim/... -race -count=1`。

- [ ] 5. server 接线与 parity
  - 文件：`packages/server/server/session_ingress.go`（`EquipArmor` ingress）、同包 parity 与集成测试（Memory/TCP 装备互换与减免投影一致；穿戴下线→重启→装备/点数保值）。
  - 验证：`go test ./packages/server/server -race -count=1`。

- [ ] 6. 客户端镜像、桥与 HUD 组件
  - 文件：`packages/client/client`（镜像 `ArmorPoints`、`EquipArmor` 上行命令构造）、桥状态组装与 client ABI v18→v19（armor 分节三端钉值：Go 组装、Rust 中继、`packages/client/frontend` TS 类型；Rust 侧零行为仅测试清单同步）、前端状态行组件族（护甲条：10 档、半档粒度、0 点零渲染）+ vitest 组件断言、护甲四件的程序化快捷栏 sprite（清偿任务组 1 遗留的 21 例缺图标红测）、`packages/client/cmd/mornlea`（使用键上升沿手持护甲 → 上行 `EquipArmor`，不发 `PlaceBlock`）及输入判定测试。
  - 验证：`go test ./packages/client/... -race -count=1`；`make frontend-check`。

- [ ] 7. 穿甲 HUD 部件基线、golden 与 audit 守卫
  - 文件：`packages/engine/crates/mornlea_client/frontend/visual`（新 fixture `hud-armor`：合成 `HudState` 携权威点数 7 驱动真实 `HudRoot`，呈现 3 个完整图标 + 1 个半图标；常显 HUD 已全量迁 WebView、无头 capture 路径 hudPush 零值退化、画面零 HUD 像素，部件基线是护甲条像素的唯一载体；既有 fixture 清单顺序不变，追加于 `hud-status` 之后）、对应 golden（`testdata/visual-golden/ui/hud-armor.png` 仅新增 1 张，既有部件与世界场景零差异；`testdata/visual-golden/README.md` ui 段同步）、`packages/audit`（护甲域单一真源守卫：点数/耐久/公式只允许被 core 定义、其他包只读消费，沿 `TestDifficultyStaysASingleCoreDomain` 形态）、基线文档同步（根 `AGENTS.md` 版本矩阵、`openspec/config.yaml` 上下文、`docs/notes/progress.md`）。
  - 验证：`go test ./packages/audit -count=1`；`make visual-check`（世界 30 景零差异）与 `make frontend-visual-check`（部件既有 30 张零差异）后 `make frontend-visual-update` 仅新增 `hud-armor` 并复跑两者全绿。

- [ ] 8. 收尾门禁
  - 命令：`test -z "$(gofmt -l .)"`；`go vet ./packages/contracts/... ./packages/shared/... ./packages/server/... ./packages/client/... ./packages/tools/... ./packages/audit/...`；`make test-race`；`make rust`；`openspec validate --all --strict --no-interactive`。
  - 全部通过后按 ledger 记录证据，进入整分支终审与归档。
