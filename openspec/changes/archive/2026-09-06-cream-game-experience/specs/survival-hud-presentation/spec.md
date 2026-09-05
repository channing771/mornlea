## MODIFIED Requirements

### Requirement: 快捷栏固定为居中九格并明确标识选中格

系统 SHALL 在关闭容器界面时经 WebView HUD 组件显示固定九格、水平居中的快捷栏（呈现职责自 GPU HUD 迁移，语义逐项平移，见 `game-overlay-webview` capability）。快捷栏贴条 MUST 为透明悬浮排布（无深色长条底带）；每格 MUST 是独立粉彩方块——逐格不同的低饱和底色、深可可描边、大圆角、顶部浅高光加底部沉边加柔投影。选中格 MUST 具有暖橙赭石外扩外框加方块抬起阴影，使其在忽略颜色后仍可由外扩几何与阴影抬起与未选中格区分；呈现数据 MUST 仅来自已确认权威镜像。选中格 SHALL 轻微抬升，相邻格可轻柔跟随；动效必须有界、不改变槽位命中矩形且在减少动态效果时禁用过渡。槽内 MUST 显示实际物品图像而非统一色块。

#### Scenario: 九格快捷栏和选中格可由几何判定

- **GIVEN** 已确认的有效快捷栏镜像和任一合法选中下标
- **WHEN** HUD 组件在关闭容器界面时呈现快捷栏
- **THEN** 快捷栏 MUST 恰好包含九个等尺寸格并整体水平居中
- **AND** 只有选中格 MUST 具有暖橙外扩外框加抬起阴影，且该标识 MUST NOT 仅靠颜色区别于其他格

#### Scenario: 粉彩悬浮外观

- **GIVEN** 已确认的有效快捷栏镜像
- **WHEN** HUD 组件呈现快捷栏
- **THEN** 贴条 MUST 无深色底带背景与内缩缘阴影
- **AND** 每格 MUST 有圆角、深色描边、高光与沉边，且九格底色 MUST 逐格不同


### Requirement: 生存 HUD 保持固定资源和实例兼容性

容器与常显 HUD SHALL 全部由前端呈现，生产 GPU 面板在打开与关闭态均产生零 quad/glyph。48-byte 编码、256-byte 对齐、320 quad、768 glyph、15616 bytes glyph offset、52480 bytes 固定上传容量 MUST 保持兼容。严禁重启 GPU 面板 fallback 或放宽 overflow。

#### Scenario: 关闭和打开界面的合法最坏组合均有界
- **GIVEN** 本场景对应的合法 HUD 状态组合
- **WHEN** 准备生产渲染帧
- **THEN** 面板与 HUD MUST 仅由前端呈现，GPU 面板 MUST 为零实例，既有容量与 overflow 门禁 MUST 保留，稳定态不得新建 GPU 资源

#### Scenario: marker 只增加四个 quad 且不扩容
- **GIVEN** 本场景对应的合法 HUD 状态组合
- **WHEN** 准备生产渲染帧
- **THEN** 面板与 HUD MUST 仅由前端呈现，GPU 面板 MUST 为零实例，既有容量与 overflow 门禁 MUST 保留，稳定态不得新建 GPU 资源

#### Scenario: 关闭态保留面不产生实例
- **GIVEN** 本场景对应的合法 HUD 状态组合
- **WHEN** 准备生产渲染帧
- **THEN** 面板与 HUD MUST 仅由前端呈现，GPU 面板 MUST 为零实例，既有容量与 overflow 门禁 MUST 保留，稳定态不得新建 GPU 资源

#### Scenario: 稳定态不创建每帧动态 GPU 资源
- **GIVEN** 本场景对应的合法 HUD 状态组合
- **WHEN** 准备生产渲染帧
- **THEN** 面板与 HUD MUST 仅由前端呈现，GPU 面板 MUST 为零实例，既有容量与 overflow 门禁 MUST 保留，稳定态不得新建 GPU 资源

