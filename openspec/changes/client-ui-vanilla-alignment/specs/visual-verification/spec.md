# visual-verification Delta

## ADDED Requirements

### Requirement: 选中弹条无窗口场景

无窗口 capture 场景表 SHALL 新增 `hud-item-name-popup` 场景，位于 `hud-survival-feedback` 之后、`avatar-nametag` 之前；`far-horizon` MUST 仍为倒数第二、`water-underwater` MUST 仍为唯一末场景。场景 MUST 装入确定性夹具：已确认镜像中选中栏位指向含已知中文名物品的格、确认变化所属 tick 与固定世界时间使弹条处于 40 tick 可见窗口内，且准星常显；场景 MUST 经与交互客户端相同的完整呈现链路无窗口抓取，MUST NOT 创建或聚焦前台游戏窗口，并 MUST 继续使用既有双阈值比对。本变更 SHALL 携带该场景的 golden PNG（场景总数 23→24 口径），并在归档时把新增场景同步进主规格；本变更同时 MUST 以显式基线更新重新生成受 HUD、容器面板与菜单换肤波及的全部既有 golden 并逐图人工复核，MUST NOT 通过放宽双阈值接受差异。

#### Scenario: 场景表顺序与导出

- **GIVEN** 完整 capture 场景表
- **WHEN** 检查 `hud-survival-feedback` 之后的场景
- **THEN** `hud-item-name-popup` MUST 位于 `hud-survival-feedback` 之后、`avatar-nametag` 之前，`far-horizon` MUST 为倒数第二，`water-underwater` MUST 为唯一末场景
- **AND** 抓帧运行 MUST 产出 `hud-item-name-popup` 图像

#### Scenario: 弹条与准星可审查且不污染后续场景

- **GIVEN** `hud-item-name-popup` 夹具已装入客户端镜像
- **WHEN** 场景完成预热、网格收敛与上传并抓帧
- **THEN** 图像 MUST 同时显示居中的物品名弹条（阴影加前景双层）与屏幕中心准星，并 MUST 与既有双阈值保持一致
- **AND** 场景结束后临时选中、tick 与世界时间夹具 MUST 一并恢复，使后续场景不继承任何夹具值
