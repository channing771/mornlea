## Purpose

视觉验收覆盖新增的 egui 主菜单：为主菜单建立无窗口 capture 场景并锁定其场景表位置，同时钉死既有场景 golden 像素不变，防止 egui 新 pass 影响既有画面。

## MODIFIED Requirements

### Requirement: 主菜单无窗口 capture 场景

视觉场景表 SHALL 新增 `main-menu` 场景：以既有 640×360 离屏渲染路径渲染含标题「Mornlea」、四个按钮与版本行的主菜单帧，回读像素与 golden 按既有双阈值比对；场景 MUST 排在 `far-horizon` 之前，`far-horizon` 仍为倒数第二、`water-underwater` 仍为最后。

#### Scenario: 场景表顺序与新场景产出

- **GIVEN** `captureScenes` 场景表
- **WHEN** 检查顺序
- **THEN** `main-menu` 存在且位于 `far-horizon` 之前
- **AND** `far-horizon` 仍为倒数第二、`water-underwater` 仍为最后
- **AND** 抓帧目录含 `main-menu.png`

#### Scenario: 新场景参与 golden 比对

- **GIVEN** 非更新模式运行 capture
- **WHEN** 执行到 `main-menu` 场景
- **THEN** `main-menu.png` 与 golden 逐像素比对（同既有阈值与差异图产出）

### Requirement: 既有场景 golden 逐字节不变

egui 集成 MUST NOT 改变不含 UI 段的任何既有 capture 场景的输出像素；既有场景的 golden 图像 SHALL 保持逐字节不变。

#### Scenario: 既有场景不受 egui 影响

- **GIVEN** 全部既有场景（不含 `main-menu`）
- **WHEN** 运行 capture 并与既有 golden 比对
- **THEN** 每个场景与各自 golden 的像素差异为零（同一比对管线下的既有阈值）
- **AND** 除新增 `main-menu.png` 外不产生任何新 golden 文件
