# passive-cattle-presentation Specification

## Purpose

定义被动牛的贴图化呈现契约：四足体型、写实贴图材质层、外网 CC0 素材转制与溯源纪律，以及 capture 视觉场景。牛 MUST NOT 使用纯色块呈现。

## Requirements

### Requirement: 牛使用四足贴图体型且不得为纯色

牛 SHALL 使用横向躯干 + 4 短腿 + 头部的四足体型，与夜行者直立骨架一眼可辨；身体各面 MUST 采样材质贴图（牛皮/牛头），MUST NOT 使用无纹理纯色块。玩家/伙伴/夜行者既有呈现 MUST 不变。

#### Scenario: 牛与夜行者轮廓可辨

- **GIVEN** 同一帧内存在 1 头牛与 1 只夜行者
- **WHEN** 客户端渲染该帧
- **THEN** 牛 MUST 呈现横向四足轮廓，夜行者 MUST 保持直立轮廓，两者 MUST 不相同

#### Scenario: 牛身无纯色块

- **GIVEN** 任意牛身体的任意可见面
- **WHEN** 客户端渲染该面
- **THEN** 该面 MUST 呈现贴图像素（斑点/噪点/明暗），MUST NOT 为整面单一 RGB

### Requirement: 4 个 16×16 材质层追加在枚举末位

系统 SHALL 新增 4 个 16×16 材质层：牛皮、牛头、生牛肉、熟牛肉；层号 MUST 追加在既有枚举末位，MUST NOT 扰动植物 31..54 连续区间与床/火把/裂纹冻结层号。牛身体六面 MUST 只采样牛皮/牛头层；生/熟牛肉 SHALL 用于掉落物与 HUD/容器图标。未映射层 MUST 仍以程序化像素回退。

#### Scenario: 层号追加不扰动植物区间

- **GIVEN** 新增 4 层后的注册表
- **WHEN** 查询小麦/马铃薯/胡萝卜各阶段层号
- **THEN** 植物层号 MUST 与变更前逐项相同，且 MUST 保持连续

#### Scenario: 生熟牛肉图标可辨

- **GIVEN** 掉落的 1 个生牛肉与熔出的 1 个熟牛肉
- **WHEN** 客户端呈现两者图标
- **THEN** 两者 MUST 像素不同且可辨（生偏红、熟偏棕），MUST NOT 共用同一层

### Requirement: 外网素材转制与溯源门禁

牛皮/牛头 SHALL 首选 `mobs_animal` 牛贴图 CC0（boxy UV），生/熟牛肉 SHALL 首选 OpenGameArt `16x16 Food` CC0；备选 MUST 同为免版税/CC0/CC-BY/CC-BY-SA 且 MUST NOT 为 Mojang 版权提取物。入库前 MUST 转制为 16×16（Pixel Perfection 明度/噪点统一），`PROVENANCE.json` MUST 逐文件记录来源、协议与 SHA，`ATTRIBUTION.md` MUST 署名；`applyPack` 16×16/≤64KiB 校验失败 MUST 全包拒绝。程序化像素 MUST 永久保留为回退。

#### Scenario: 溯源缺失拒绝入库

- **GIVEN** 一份无 `PROVENANCE.json` 条目的外网 PNG
- **WHEN** 评审该素材入库
- **THEN** MUST 拒绝入库，直到来源、协议与 SHA 补齐

#### Scenario: 非 16×16 素材拒绝覆盖

- **GIVEN** 一份 32×32 的外网牛肉图标
- **WHEN** 经材质包加载路径应用
- **THEN** 系统 MUST 拒绝该文件（尺寸门禁），MUST NOT 部分生效

### Requirement: capture 新增牛群场景

capture SHALL 新增 `passive-herd` 场景：昼间草地、3 头牛（不同朝向/位置互不遮挡）+ 1 个生牛肉掉落；抓帧 MUST 显示四足贴图牛身与可辨牛肉图标。既有 17+ 张 golden MUST 逐字节不变（牛景只追加）。

#### Scenario: 牛群场景一次通过

- **GIVEN** 固定牛群夹具
- **WHEN** 运行无窗口抓帧
- **THEN** 画面 MUST 同时显示 3 头贴图牛与 1 个生牛肉掉落，且与入库 golden 在双阈值内一致
