## MODIFIED Requirements

### Requirement: 未受影响场景 golden 逐字节不变

自然短草改变默认方块 registry 与部分固定世界背景后，本变更 MAY 只更新经逐图归因确认确由自然短草可见性引起的既有正式 golden；`oak-grove` 的固定夹具 MUST 包含至少一株在相机中可辨识的短草，并 MUST 经既有四 quad alpha-cutout 植物路径呈现。共享相同固定世界全景且实际出现短草的 `main-menu.png` 或 `settings-menu.png` MAY 在逐图归因后更新；所有未受自然短草影响的正式场景 golden SHALL 保持逐字节不变。正式场景清单 MUST 继续恰好为既有 25 项并保持当前顺序，MUST NOT 新增 `natural-grass` 或任何第 26 个正式场景；全部更新与比对 MUST 继续使用既有双阈值，MUST NOT 通过放宽阈值接受差异。

#### Scenario: 非设置场景不受变更影响

- **GIVEN** 全部既有 25 个正式场景及变更前 golden
- **WHEN** 运行 capture 并逐图归因自然短草造成的像素差异
- **THEN** 只有画面中确实出现自然短草或共享自然短草世界背景的既有 PNG MAY 更新
- **AND** 其余每个场景的 PNG 字节 MUST 保持不变
- **AND** MUST NOT 新增任何第 26 个场景或 golden

#### Scenario: oak-grove 明确承重短草外观

- **GIVEN** `oak-grove` 的固定世界种子、区块、正午时间、相机与既有完整渲染链路
- **WHEN** 场景完成预热、网格收敛与上传并无窗口抓帧
- **THEN** 图像 MUST 包含至少一株可辨识的自然短草
- **AND** 短草 MUST 显示透明边缘与交叉植物轮廓，而不是实心立方体或不透明矩形

#### Scenario: 25 项场景顺序与阈值保持不变

- **WHEN** 检查 `captureScenes` 表并比较本变更允许更新的 golden
- **THEN** 场景数量 MUST 恰好为 `25` 且名称与顺序 MUST 与变更前一致
- **AND** 每张图 MUST 继续使用既有双阈值与差异图规则
