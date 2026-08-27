# M5E 再递延字面同源化（m5e-literal-single-source）

## Why

M5E（2026-08-18 归档）「延期与放弃」递延清单尚余两项未清偿：递延 4（`cmd/mornlea` 的
`[32]network.ChatEvent` 裸字面，当时判定「编译器类型匹配结构性锁定；彻底消字面留后续」）
与递延 5（`internal/network/codec_client.go` ChatCommand 编解码的 1024 字面与 payload 上限
守卫错误文案硬编码 1026，「字面同源化留后续 change 收敛」）。三个权威常量均已存在——
`client.ChatEventCapacity = 32`、`companion.MaxPlanCommandBytes`（= 1024）、
`chatCommandMaxWireBytes`（= `companion.MaxPlanCommandBytes + 2`，推导出 1026）——裸字面是
与它们失联的重复来源：常量演进时裸字面既不会跟着变也不会变红。本变更按规划表 E-12 行把
这两项一次清偿。2026-08-25 经确认通道获用户显式批准（bounded 短设计，飞书卡片 approve），
批准结论已落本目录 proposal/design。

## What Changes

- **L1（递延 4）**：`cmd/mornlea/capture_scene.go` 两处与
  `cmd/mornlea/capture_ai_companion_test.go` 一处的裸 `[32]network.ChatEvent{}` 改为
  `[client.ChatEventCapacity]network.ChatEvent{}`，与 `app.go` 字段声明及
  `app_lifecycle.go` 重置路径同源（后两者已在用该常量，本变更补齐漏改的三处）。
- **L2（递延 5）**：`internal/network/message_companion.go` 新增包级常量
  `chatCommandTextMaxBytes = companion.MaxPlanCommandBytes`（紧邻既有
  `chatCommandMaxWireBytes` 推导注释）；`codec_client.go` 编码端
  `e.string(message.Text, 1024)` 与解码端 `d.string(1024, 1024)` 改用该常量；payload 上限
  守卫的错误文案改由 `chatCommandMaxWireBytes` 以 `%d` 推导（先例：
  `internal/storage/player_codec.go` 的 `%w: player payload exceeds %d bytes`），格式化输出
  与原静态字符串逐字节相同。

零行为变化：所有替换点数值恒等，玩家可见、wire 可见、存档可见行为逐字节不变。

## 非目标

- 不改任何上限值、wire 格式、协议/存档 schema 版本、engine/client ABI、benchmark scenario、capture golden。
- 不动锁测试中独立钉死 wire 契约的 1024/1025/1026 字面（`TestChatCommandAccepts1024BytesAndRejects1025`
  与 `TestCompanionMessagesHaveFixedMaximumWireLengths` 的字面**故意**与被测常量不同源，
  是常量被误改时仍会变红的独立守卫；同源化反而使其失效）。
- 不动 `cmd/mornlea/app.go` 的 `chatLines [6]string`（HUD 显示行数有独立出处注释，不属
  递延 4/5 范围）。
- 不重开 M5E 放弃项三条。

## 用户可观察结果

无任何用户可观察变化。收益在维护性：ChatCommand 文本上限与其 wire 上限从此只有一处权威
来源，容量/上限演进不再依赖人工同步散落字面。

## Impact

- Go 代码：`cmd/mornlea/capture_scene.go`、`cmd/mornlea/capture_ai_companion_test.go`、
  `internal/network/message_companion.go`、`internal/network/codec_client.go`。
- 规格：零 delta（skip_specs）；无 Rust 面改动；无存档/协议迁移；无并发或性能契约变化。

## 延期与放弃

- 顺延①：`codec_client.go` 解码端「两参同值」注释的措辞打磨（双评审 QUALITY NIT）——
  双重否定可正向化并点明「rune 上限不会先于字节上限构成实际约束、调整须两参随动」；
  纯注释语感，不改语义，避免为它稀释已验证状态。
- 顺延②：`message_companion.go` 指令槽位仍直引 `companion.MaxPlanCommandBytes` 与
  `chatCommandTextMaxBytes` 别名并存（双评审 SPEC/QUALITY NIT）——既有风格的收口候选，
  在授权文件集外扩展即超范围，留待下一次触碰该常量块的行为性变更一并统一。
