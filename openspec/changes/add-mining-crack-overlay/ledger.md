# add-mining-crack-overlay Ledger

基线 SHA：`d3e9fcbb`（main）。工作分支：`feat/mining-crack-overlay`
（worktree `.worktrees/mining-crack-overlay`）。

## 阶段 1：内容确认

- Ruling: 需求确认以需求方任务书为准 — 需求方在控制会话对话中给出了完整的
  目标、架构约束（真实 breakProgress 驱动 + 10 Stage + 单一可复用 Crack
  Overlay + Atlas/UV 切换 + 透明像素风）、禁止修改清单与 5 组验收 case，并
  明确指示"直接基于现有 Mornlea 项目代码完成修改"，构成显式设计批准；分类为
  `bounded`（仓库既有流程与渲染先例内的呈现层改动，无新子系统），设计结论
  全部落入本 change 的 proposal/design。— 依据：development-process 阶段 1
  "批准来源 = 用户或控制会话的显式确认"。— 无偏差。

## 任务执行

（每任务：implementer 派发、评审结论、验证证据按 SHA 记录于此）

### Task 1：程序化裂纹材质层

- 实现：commit `2c923b24`（fresh implementer）。出生阶段图实现增量生长；
  层号 68..77、cutout 分类、`crack_0..9` 绑定槽位；适配 `bed_test.go`
  （枚举末位哨兵改邻接断言，守卫语义等价）与 `pack_test.go`（固定打开顺序
  期望表追加）。
- 验证证据 @ `2c923b24`：`go test ./internal/assets -race -count=1` ok
  （66 tests）；`go test ./internal/mesh -run 'TestNativeOracleParity|
  TestPlantMaterial' -count=1` ok；`gofmt -l` 无输出；`go vet` 干净；
  `go test ./internal/archcheck -count=1` ok。
- SPEC 评审（fresh reviewer）：**pass**。层号/纹理七项/cutout/槽位/范围纪律
  全部 PASS；唯一缺口 = tasks 1.1 的"AtlasPixels 输出长度随 layerCount 走"
  无直接钉守（既有测试为双注册表相对比较）。
- QUALITY 评审（fresh reviewer）：**pass**。birth map 设计 5/5；视觉合理性
  4/5（stage 覆盖率 2%→35% 模拟确认）；两处 minor：`crack.go:46` 注释
  "约半数"与实测 25% 有效新增不符、`crack_test.go:40` 恒真断言循环。
- Ruling: R1 修复三处（AtlasPixels 位置性测试 + cutout 路径镜像断言、
  注释改门控概率描述、恒真循环改显式冻结表）— 均为评审明确的非阻塞缺口，
  修复成本低且加固 tasks 1.1 验收项 — 不构成范围漂移。
- R1 落地：commit `da5a219c`。`go test ./internal/assets -race -count=1` ok
  （68 tests，新增 `TestAtlasPixelsScalesWithLayerCount`、
  `TestCrackLayersTakeTheCutoutMipPath` 含双分支产物必须不同的夹具承重守卫
  与石头层平均路径对照）；gofmt/vet 干净。brief 中"layers×mips×1024"的字面
  公式与 mip 逐级减半的真实布局不符，implementer 按意图改为逐 mip 求和推导
  （78×1364），裁决接受。Task 1 关闭。
