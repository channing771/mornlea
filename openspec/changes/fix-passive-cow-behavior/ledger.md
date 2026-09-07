# fix-passive-cow-behavior ledger

- 基线 SHA：`b179fec1`（origin/main，2026-09-07 fast-forward 后）
- Ruling: 用户直报三处牛行为缺陷（打转/贴脸/横行），控制会话裁决跳过 backlog 排队直接开工，先例为 2026-09-04 B-27 裁决 — 用户在本会话内显式批准了「三 change + 牛修复先行」计划 — 规划表无对应行不构成阻塞。
- Ruling: 闲时看人改为「只看不靠近」，规格标题随之更名（REMOVED+ADDED 而非 MODIFIED） — OpenSpec delta 以标题匹配需求，行为语义已与标题矛盾 — 保留原标题会留下误导性契约。
- Ruling: 渲染朝向偏移放客户端装配层而非服务端发布 — 权威 yaw 语义与 wire 值保持正交，呈现映射不进协议 — 服务端换算会污染逃跑/引诱内部计算。

## 任务进度

- 1.1+1.2 漫游分段稳定：提交 `7568deea`（基线 e584916e）。验证：`go test ./packages/server/sim/entity -race -count=1 -run 'Passive'` ok（2.2s）、`go test ./packages/server/sim/... -race -count=1` 全 ok（entity 4.9s / realm 5.3s / runtime 21.2s）、`gofmt -l` 无输出。评审 APPROVE：段内稳定/段间 ≤0.2 rad 契约成立、新测试对旧实现必红（评审者独立复刻哈希验证 192/199 步超限量化的打转缺陷）、哈希分箱无系统性偏置、邻域回滚与逃跑优先守护未削弱；抽查 17 测试全 PASS。备注：实现与测试同 commit，红→绿时序以区分度实证替代。
- 2.1 闲时只看不靠近：提交 `533c9333` + 修复提交 `5c90e9a6`（基线 7568deea）。验证：`go test ./packages/server/sim/... -race -count=1` 全 ok（entity 5.3s / runtime 22.3s）、`gofmt -l` 无输出。评审 REQUEST_CHANGES→修复后 APPROVE：唯一阻塞为 `TestPassiveMovementNeverPassesThroughWalls` 被闲时冻结掏空（玩家距牛 2.83 格 <6 格圈、100 tick 位移 0；父实现 1.43 格守护是活的），评审者以 go test -overlay 预验证修法；实现者按建议落地（玩家摆 30.5 格 + moved 位移断言 + 因果注释），红色先行复现「位移 0」后转绿。Ruling: 穿墙测试摆位补强属任务 2.1 范围 — 冻结化回归会静默掏空全仓唯一的被动牛×实体碰撞守护 — 守护空转化是行为变更的直接后果而非无关顺手改。
- 3.1 渲染朝向装配层对齐：提交 `325f0d11`（基线 5c90e9a6）。验证：`go test ./packages/client/... -race -count=1` 12 包全 ok（app 74.7s 含装配依赖）、`gofmt -l` 无输出。评审 APPROVE：θ=yaw+π/2 映射经 mathgl v1.2.0 源码独立复算成立（yaw=0 面向 -Z、yaw=π/2 两例手工验证）、归一化缝仅出现在 yaw≈π/2 且下游 2π 周期矩阵吸收、插值在偏移前空间闭合不受常数平移破坏、玩家/伙伴/夜行者直传与低头/点头/死亡通道未动；红→绿经 go test -overlay 实证（父实现 yaw=0 偏差 √2 即 90° 横行）。两项自报偏差（绝对 L2 替代 mathgl 相对语义阈值、测试拆 app/render 两包）理由均核实成立。
- Ruling: 被动牛装配 yaw 与模型局部 +X 姿态解耦断言 — 装配数值契约在 app 包、几何基向量在 render 包 — render 不可反向导入 app — 单侧断言无法同时锁「装配值」与「矩阵语义」。
- 4.1 收尾门禁（控制会话执行，HEAD 49eb0f50）：`gofmt -l packages/` 无输出；`go test ./packages/audit -count=1` ok（8.9s，含注释标识符门禁修复提交 `49eb0f50`：外部 mathgl 标识符去反引号）；`make dev-check` 退出码 0；`make visual-check` 首跑仅 passive-herd（4.83%）/passive-graze（2.95%）两景超出阈值（牛朝向修正的预期视觉变化，实拍经人工逐图确认：头在躯干前端、低头吃草姿态正确、无渲染异常），`SCENES=passive-herd,passive-graze make visual-update` 重录 2 张 PNG + 3 张 GIF（graze/kill/lure）基线后复跑 27/27 全部零差异；`make test-race` 退出码 0（45 包 ok 零失败）；`openspec validate --all --strict --no-interactive` 100 项全过。Ruling: 被动 GIF 基线随两景 visual-update 一并重录 — GIF 剧本同样呈现牛朝向 — 保留旧基线会在 motion 校验上报假差异。
