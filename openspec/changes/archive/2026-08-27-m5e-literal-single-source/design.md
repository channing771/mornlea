# Design — m5e-literal-single-source

## 裁决

- **分类**：bounded（内容确认通道 2026-08-25 飞书 approve）。零行为字面同源化，无 delta specs（skip_specs）。
- **L1 同源目标**：`client.ChatEventCapacity`（`internal/client/chat.go:15`，容量出处注释已写明「事件环最多回放 32 条」）。替换点共三处裸 `[32]network.ChatEvent{}`：`capture_scene.go` 场景重置两处、`capture_ai_companion_test.go` 断言一处。同值替换，编译期可证等价；数组零值比较语义不变。
- **L2 同源目标**：新增 `chatCommandTextMaxBytes = companion.MaxPlanCommandBytes`，与既有 `validateCommandText`（校验上界）、`chatCommandMaxWireBytes = companion.MaxPlanCommandBytes + 2`（wire 推导）三点收敛到同一来源。

## 被否决的替代方案

1. **编解码调用点直接引用 `companion.MaxPlanCommandBytes`**：少一层间接，但 `network` 包内该值的语境是「ChatCommand 文本槽位上限」，直接引用外部包常量会让 codec 端丢失语义锚点；命名包级常量紧邻 `chatCommandMaxWireBytes` 注释块，推导关系自文档化。选命名常量。
2. **解码端 `d.string(maxBytes, maxRunes)` 拆成两个常量**：现状两参同值 1024，拆分是为不存在的需求引入区分度；沿用同值并注释「rune 上限与字节上限同值系现状保持」。
3. **错误文案保留静态字符串、只加注释**：注释挡不住漂移（archcheck 注释门禁只抓标识符删除/改名）；`fmt.Errorf("%d", chatCommandMaxWireBytes)` 与仓库先例 `storage/player_codec.go` 同型。格式化仅发生在超限拒绝路径，不在热路径。
4. **锁测试字面一并同源化**：否决——锁测试的价值恰在独立于被测常量，见 proposal 非目标。

## 不变量与验证口径

- wire 可观察行为逐字节不变：编码产物 hex 不变、解码接受集不变、错误类别与消息文本不变（`%d` 输出 `1026` 与原静态串逐字节相同）。
- 验证以既有锁测试为主：`TestChatCommandAccepts1024BytesAndRejects1025`（1024 收/1025 拒/raw wire 拒收集）、
  `TestCompanionMessagesHaveFixedMaximumWireLengths`（ChatCommand 1026 上限）、cmd/mornlea capture 相关测试（场景重置路径）。
- 无新测试：本变更零行为，编译器 + 既有行为级锁即红线；不为同源化制造同义反复断言。

## 受影响文件

`cmd/mornlea/capture_scene.go`、`cmd/mornlea/capture_ai_companion_test.go`、
`internal/network/message_companion.go`、`internal/network/codec_client.go`
（均在本行认领声明的独占文件集内）。
