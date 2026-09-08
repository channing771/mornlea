## ADDED Requirements

### Requirement: 树苗与橡树再生基线场景

`sapling-growth` SHALL 为树苗与运行时生长的橡树提供图片基线：确定性手工夹具在固定地形上同时立一株树苗与一棵由运行时树形几何长成的橡树，固定正午与固定机位，画面 MUST 呈现至少一株可辨识的树苗（交叉斜面、四 quad cutout、上缘透空、贴地生长）与至少一株可辨识的橡树（原木树干与树叶树冠）。在场景清单中 `sapling-growth` MUST 紧随 `oak-grove` 且先于 `ai-companion`，场景总数 MUST 由 `29` 变为 `30`。该场景 MUST 经与交互客户端相同的完整呈现链路收敛后无窗口抓取，MUST NOT 创建或聚焦前台游戏窗口，且既有双阈值 MUST 保持不变。

#### Scenario: 场景呈现可辨识树苗与橡树

- **GIVEN** `sapling-growth` 的确定性夹具已装入客户端镜像（树苗与运行时橡树、固定正午、固定机位）
- **WHEN** 场景完成预热、网格收敛和上传并抓帧
- **THEN** 图像 MUST 显示至少一株树苗，其屏幕矩形内 MUST 存在 cutout 透空与贴地叶片的可辨识差异
- **AND** 图像 MUST 显示至少一株橡树，其树干与树冠 MUST 可辨识

#### Scenario: 场景顺序与数量锁定

- **GIVEN** 场景清单与顺序断言
- **WHEN** 枚举 capture 场景
- **THEN** `sapling-growth` MUST 位于 `oak-grove` 之后、`ai-companion` 之前
- **AND** 场景总数 MUST 等于 `30`，既有场景的名称与相对顺序 MUST 保持不变

#### Scenario: 既有 golden 与阈值不变

- **GIVEN** 除 `sapling-growth` 外的既有场景
- **WHEN** 运行视觉比对
- **THEN** 既有 golden MUST 逐字节不变或保持在既有双阈值内
- **AND** 阈值 MUST NOT 被放宽
