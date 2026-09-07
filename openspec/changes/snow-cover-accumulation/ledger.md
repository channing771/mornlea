# snow-cover-accumulation ledger

- 基线 SHA：待定（`seasonal-time-temperature` 合并 main 后重定基线再开工）。
- Ruling: 4 档雪层厚度 2/16..5/16（`block_top_raw` 2..5）而非 1/16 步进 — `block_top_raw` 0 为满格哨兵、1/16 不可表达，8 档视觉差异过细 — 档间 1/16 差与 4 档上限在可辨性与方块预算间最优。
- Ruling: 减速放 Go 共享物理落足采样而非 Rust step — Rust 只见碰撞盒不见方块 ID — 权威与客户端预测走同一 `physics.Step` 路径自动一致、零 ABI 变更。
- Ruling: 雪层不可被玩家放置（`PlaceableBlockAtFace` 拒绝） — 雪层是机制产生的装饰层，开放放置引入建造语义与放置面校验 — 超出本 change 范围。
- Ruling: 踢雪粒子复用 precip 实例流与 256 总上限（超限舍弃踢雪尘保天气粒子） — 不新增帧 TLV tag/client ABI 变更 — 粒子语义从属「降水的地面交互」。
- Ruling: 四项踩雪交互（脚印/音效/减速/粒子）与 4 档厚度、每季 3 游戏日均经用户显式选择（本会话 AskUserQuestion）。
- 版本：全部不变（协议 v37、metadata v4、engine ABI v10、client ABI v18、区块 schema v9、scenario v22）；Rust 零改动。

## 任务进度

（每任务完成后由控制会话回填验证证据与评审结论）
