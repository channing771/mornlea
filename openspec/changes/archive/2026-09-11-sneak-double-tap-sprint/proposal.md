# Change: sneak-double-tap-sprint（B-42 潜行与双击疾跑）

## Why
`archive/2026-08-27-sprint/design.md` 非目标（「潜行/鞘翅等互斥、水中冲刺」）与 `archive/2026-08-06-m4k-authoritative-chests/proposal.md` 非目标（「潜行放置」）共同留下潜行机制缺口；B-30 只交付了疾跑加速而未建互斥半边。需要以一个最小闭环交付：`Shift` 按住即潜行（慢速移动、防坠落边缘保护、对交互方块潜行放置），疾跑改双击 `W` 触发并锁存，`Ctrl` 退役。

## What Changes
- `Shift` 按住 = 潜行意图：慢速移动（默认 `0.3x`）、站立边缘防坠落、对交互方块潜行放置（压交互、直发放置）。
- 疾跑改双击 `W` 触发并锁存：第二次 `W` 上升沿距上次 `W` 释放 `≤300ms` 且 `W` 保持按住即置位 `Sprinting`，直到松开 `W`、按下 `Shift`、打开界面/聊天/暂停时清零；饥饿/地面/浸没仍由服务端权威门控。
- `Ctrl` 不再触发疾跑，直接退役；`Alt` 保持空闲；键位重绑留给 D-15。
- 互斥裁决（潜行优先）：`Sneaking` 置位时疾跑加速与疾跑疲劳一律跳过；潜行减速只在站立 + 非浸没时生效（空中/水中不减速，但 `Sneaking` 位仍保留给放置逻辑）。
- 协议 `v40→v41`：`PlayerInput` 尾部追加 `Sneaking bool`（紧跟 `Sprinting` 之后），纯追加；Step header 布局 `v3→v4`（总长保持 160B：`bytes[130]=Sneaking`，`bytes[152:156]=SneakSpeedMultiplier`，`156..160` 继续置零）。

## Impact
- 协议 `v40→v41`（仅 `PlayerInput` 尾部 1 字节，不新增消息类型、不新增 `RejectReason`）；旧版登录拒绝，header 布局版本校验 fail-fast。
- Step header 布局 `v3→v4`，总长保持 160 字节，不升 engine ABI（v11）、client ABI（v18）与全部存档 schema（玩家 v8、区块 v9、metadata v5、companions v5、hostile v1、passive v1）、benchmark scenario（v23）不变。
- 回退 = 整 change revert（纯追加，无迁移）。

## Non-Goals
- 门/床交互的客户端 wiring（本行只在服务端留潜行分流位，不新增网络消息）。
- 锄地/骨粉/水桶在潜行下的行为变更（保持原样，不压制）。
- 潜行视线高度降低与姿态呈现（留 D-09 第三人称/姿态）。
- 潜行耐力/疲劳新增行、FOV/HUD/音效、设置页键位重绑（D-15）。
- 新 capture 场景；既有 30 景零漂为门禁。
