## MODIFIED Requirements

### Requirement: 容器像素换肤保持固定 HUD 资源契约

系统 SHALL 复用既有 hotbar atlas、layout、48-byte instance 编码和 HUD GPU pass。三类 overlay 各自只比换肤前组成增加一个标题 quad且零 glyph；代码事实中的最大打开态 MUST 为 257 quad，而不是过时的 266 quad。combat marker 可见时最大打开态 MUST 只增加 4 个 quad 至 261，仍不超过 scenario v19 固定的 267 quad。固定 glyph 上限 MUST 仍为 700，glyph offset MUST 仍为 13312 bytes，总容量 MUST 仍为 46912 bytes，所有固定区间 MUST 继续按 256 bytes 对齐。稳定态 MUST 只更新既有固定上传资源的实际实例前缀，不得创建每帧动态 GPU 资源或扩充固定布局。

#### Scenario: 最大打开态仍装入 scenario v19 固定缓冲
- **GIVEN** 打开态同时取合法 overlay、物品数量、来源轮廓、生存状态与聊天的最坏互斥组合，combat marker 不可见
- **WHEN** HUD 准备 quad 与 glyph 实例
- **THEN** quad 数量 MUST 为 257 且不超过固定上限 267
- **AND** glyph 上限、glyph offset、总容量、instance 大小和对齐 MUST 分别保持 700、13312 bytes、46912 bytes、48 bytes 和 256 bytes
- **AND** 标题 MUST 只占一个 quad 且不占 glyph

#### Scenario: 最大打开态加入 marker 仍不扩容
- **GIVEN** 上述合法最大打开态收到新鲜 combat 确认
- **WHEN** HUD 同时准备 4-quad marker
- **THEN** quad 数量 MUST 恰好为 261，固定上限 MUST 仍为 267，所有 buffer offset 与总容量 MUST 不变化

#### Scenario: 零尺寸与窄窗口不引入资源或边界例外
- **GIVEN** framebuffer 为零尺寸或现有支持的窄正尺寸
- **WHEN** 任一容器 overlay 与可选 marker 准备布局
- **THEN** 零尺寸 MUST 不发出实例，正尺寸的全部实例 MUST 有限且严格位于 framebuffer 内
- **AND** 标题、header、marker 与所有命中矩形 MUST 保持不相交
- **AND** 系统 MUST NOT 分配动态 GPU 资源或放宽固定容量
