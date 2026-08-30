## MODIFIED Requirements

### Requirement: 生存 HUD 保持固定资源和实例兼容性

生存 HUD SHALL 继续使用 benchmark scenario v19 已锁定的固定容量预分配资源、同一个既有 HUD pass 和相同的 48-byte instance 编码。`maxHotbarQuads` MUST 保持 267，`maxHotbarGlyphs` MUST 保持 700，glyph offset MUST 保持 13312 bytes，固定上传总容量 MUST 保持 46912 bytes，所有固定区间 offset MUST 保持 256-byte 对齐。当前不含 hit marker 的合法最大关闭/打开态 MUST 分别恰好为 96/257 quad；显示 marker 后 MUST 分别恰好为 100/261 quad，仍不超过固定 267 上限。稳定态 warmed `Prepare` MUST 保持零堆分配、零每帧动态 GPU 资源，且不得新增 HUD shader、GPU pass、上传格式、API、ABI、配置项或依赖。

#### Scenario: 关闭和打开界面的合法最坏组合均有界
- **GIVEN** 分别构造关闭界面的最坏快捷栏、状态与采掘组合，以及打开界面的最大背包、容器、状态与聊天组合
- **WHEN** 系统在 marker 不可见时准备两种 HUD
- **THEN** 关闭和打开合法最坏组合的 quad 数量 MUST 分别恰好为 96 和 257，且都 MUST 不超过固定上限 267
- **AND** 合法最大 glyph 组合 MUST 不超过固定上限 700

#### Scenario: marker 只增加四个 quad 且不扩容
- **GIVEN** 合法最大关闭态和打开态各自启用 combat marker
- **WHEN** 系统准备 HUD
- **THEN** quad 数量 MUST 分别恰好为 100 和 261，固定上限 MUST 保持 267
- **AND** glyph offset、固定上传总容量、instance 大小和 256-byte 对齐 MUST 保持不变

#### Scenario: 稳定态准备保持零分配
- **GIVEN** HUD atlas、layout 与固定上传资源已经预热
- **WHEN** 系统反复以 marker 可见和不可见两种状态调用 `Prepare`
- **THEN** warmed `Prepare` MUST 产生零堆分配，也 MUST 不创建每帧动态 GPU 资源、HUD pass 或 shader

## ADDED Requirements

### Requirement: 权威命中 marker 使用四个 quad 并按成功呈现帧计时

图形客户端 SHALL 只由严格递增的合法 `CombatHit` 武装 hit marker。marker MUST 在准星中心上、下、左、右各显示一个白色不透明 untextured quad，共 4 quad；每条设计长度 MUST 为 8 px、厚度 MUST 为 2 px、内缘距中心 MUST 为 4 px，并随既有 `hudScale` 等比缩放与裁剪。每条新鲜确认 MUST 把剩余时间重置为 6 个成功呈现帧；只有 renderer 实际成功返回 true 后才可消耗一帧。零 framebuffer、HUD prepare 失败、entity overflow 或 renderer 返回 false MUST NOT 消耗。断线、退回主菜单、建立新会话、权威 reset 和 capture 场景切换 MUST 清零独立 combat tick 与 marker 帧数；权威 reset 后同 tick 随后的新鲜确认 MUST 仍可重新武装。

#### Scenario: marker 几何恰好为四个 quad
- **GIVEN** framebuffer 为正尺寸且 marker 已武装
- **WHEN** 系统准备 HUD
- **THEN** MUST 在中心四向各产生一个白色不透明 quad，上下尺寸 MUST 为 `2×8`，左右尺寸 MUST 为 `8×2`，每条内缘 MUST 距中心 `4*hudScale`

#### Scenario: 六次成功呈现后消失
- **GIVEN** 一条新鲜确认把 marker 剩余帧重置为 6
- **WHEN** renderer 连续六次成功返回 true
- **THEN** 前六个成功帧 MUST 可见，第六次之后 marker MUST 不再可见

#### Scenario: 失败呈现不消耗帧
- **GIVEN** marker 仍有 6 帧
- **WHEN** 遇到零 framebuffer、HUD prepare 失败、entity overflow 或 renderer 返回 false
- **THEN** 剩余帧数 MUST 保持 6，下一次成功呈现 MUST 仍显示 marker

#### Scenario: 新确认重置窗口且生命周期 reset 清空
- **GIVEN** marker 只剩 1 帧
- **WHEN** 收到更大 `ServerTick` 的合法确认
- **THEN** 剩余帧数 MUST 重置为 6
- **WHEN** 随后断线、返回菜单、建立新 session、收到权威 reset 或切换 capture 场景
- **THEN** combat 去重状态与 marker 帧数 MUST 一并清零
