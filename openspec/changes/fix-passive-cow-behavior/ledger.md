# fix-passive-cow-behavior ledger

- 基线 SHA：`b179fec1`（origin/main，2026-09-07 fast-forward 后）
- Ruling: 用户直报三处牛行为缺陷（打转/贴脸/横行），控制会话裁决跳过 backlog 排队直接开工，先例为 2026-09-04 B-27 裁决 — 用户在本会话内显式批准了「三 change + 牛修复先行」计划 — 规划表无对应行不构成阻塞。
- Ruling: 闲时看人改为「只看不靠近」，规格标题随之更名（REMOVED+ADDED 而非 MODIFIED） — OpenSpec delta 以标题匹配需求，行为语义已与标题矛盾 — 保留原标题会留下误导性契约。
- Ruling: 渲染朝向偏移放客户端装配层而非服务端发布 — 权威 yaw 语义与 wire 值保持正交，呈现映射不进协议 — 服务端换算会污染逃跑/引诱内部计算。

## 任务进度

（每任务完成后由控制会话回填验证证据与评审结论）
