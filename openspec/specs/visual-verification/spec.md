# visual-verification Specification

## Purpose
为渲染结果提供像素级验证：把 GPU 上渲染出的帧读回主存、以确定性的固定场景抓帧存档，并与冻结的基线图做有容差的比对，使"屏幕上画错了"这类问题在自动门禁中可见，而不是只能靠人眼偶然发现。
## Requirements
### Requirement: 渲染目标的像素可被读回且行距紧凑

系统 SHALL 支持把渲染目标纹理的像素读回主存。读回结果 MUST 按"宽 × 每像素字节"紧凑排列，不得包含底层图形 API 为满足行距对齐而插入的填充字节。读回能力 MUST 由纹理在创建时显式声明的用途位启用，未声明该用途的纹理不得被读回。

#### Scenario: 非对齐行距的读回保持紧凑

- **GIVEN** 一张宽度使得每行字节数不是图形 API 行距对齐倍数的纹理（例如每行 400 字节，而对齐要求为 256）
- **WHEN** 写入一组已知像素后读回该纹理
- **THEN** 读回的字节序列 MUST 与写入的字节序列逐字节相等，且长度 MUST 等于 宽 × 高 × 每像素字节

#### Scenario: 恰好对齐行距的读回不丢行

- **GIVEN** 一张每行字节数恰好等于行距对齐边界的纹理
- **WHEN** 写入一组已知像素后读回该纹理
- **THEN** 读回的字节序列 MUST 与写入的字节序列逐字节相等

#### Scenario: 越界的层或 mip 被拒绝

- **GIVEN** 一张已创建的纹理
- **WHEN** 以超出其层数或 mip 级数的下标请求读回
- **THEN** 系统 MUST 拒绝该请求，且不得返回内容不确定的数据

### Requirement: 抓帧模式产出确定性的视觉场景图像

系统 SHALL 提供一个无头抓帧模式，按固定的视觉场景清单产出图像文件。每个视觉场景 MUST 由确定性的世界状态、固定的相机位姿与固定的抓帧时机共同定义，三者 MUST 全部是常量而非运行时输入。抓帧 MUST NOT 创建或聚焦任何前台游戏窗口。抓帧渲染 MUST 复用与交互式客户端相同的渲染调用链，不得使用专供抓帧的旁路。

#### Scenario: 抓帧产出全部场景图像

- **WHEN** 以抓帧模式指定一个输出目录运行
- **THEN** 该目录中 MUST 为场景清单里的每个场景产出一份图像文件，文件名 MUST 与场景名一致

#### Scenario: 同一提交上重复抓帧结果稳定

- **GIVEN** 同一份代码与同一台机器
- **WHEN** 连续两次以抓帧模式运行
- **THEN** 两次产出的图像之间的差异 MUST 落在既定的比对阈值以内

#### Scenario: 图像的颜色通道顺序正确

- **GIVEN** 渲染目标采用与图像文件不同的颜色通道顺序
- **WHEN** 抓帧写出图像文件
- **THEN** 图像文件中每个像素的红、绿、蓝分量 MUST 与渲染目标中该像素的对应分量一致，不得整体偏色

### Requirement: 抓帧模式与其他运行模式互斥

抓帧模式与性能基准模式 MUST NOT 同时启用：两者都独占无头渲染路径并各自驱动场景，同时启用的语义无法定义。抓帧模式与连接远程服务端 MUST NOT 同时启用。系统遇到互斥组合时 MUST 直接拒绝启动并报错，不得让其中一方静默胜出。

#### Scenario: 抓帧与性能基准同时指定被拒绝

- **WHEN** 同时指定抓帧模式与性能基准模式
- **THEN** 系统 MUST 拒绝启动并给出说明原因的错误

#### Scenario: 抓帧与远程连接同时指定被拒绝

- **WHEN** 同时指定抓帧模式与远程服务端地址
- **THEN** 系统 MUST 拒绝启动并给出说明原因的错误

#### Scenario: 未指定抓帧时行为不变

- **WHEN** 不指定抓帧模式运行
- **THEN** 系统 MUST 保持既有行为，且不得因抓帧能力的存在产生任何额外的渲染或读回开销

### Requirement: 视觉比对采用双阈值而非逐字节相等

系统 SHALL 用两项独立指标裁决一次视觉比对：全图范围内单个像素任一颜色通道的最大差值，以及存在任何差异的像素占全图的比例。两项均在阈值内方为通过。系统 MUST NOT 要求实拍图与基线图逐字节相等——颜色空间编解码、光栅化的 tie-break 与驱动差异会造成个位数最低有效位漂移，逐字节相等会使门禁在共享运行环境下持续假失败。

两项指标各自拦截一类形态：最大差值拦截少数像素上的显著偏差（如接缝漏光），占比拦截波及大片画面的整体偏移（如色阶错误）。

两项阈值的取值 MUST 来自同一环境重复抓帧测得的实际漂移分布，并 MUST 与测量结果一同记录在变更产物中。

#### Scenario: 稀疏微小漂移被放过

- **GIVEN** 实拍图与基线图仅有极少数像素相差 1，其余像素完全相同
- **WHEN** 执行视觉比对
- **THEN** 比对 MUST 通过

（此处刻意描述"稀疏"而非"大面积"：实测同机连续两次抓帧的漂移形态是 230400 像素中 2 个相差 1，GPU 浮点非结合性造成的抖动是稀疏的。要求"整图每像素都偏移 1 也必须通过"会与占比指标直接冲突，且描述的是一种真实硬件不会产生的形态。）

#### Scenario: 局部高差值被拦下

- **GIVEN** 实拍图与基线图仅有极少数像素不同，但这些像素的通道差值远超噪声水平
- **WHEN** 执行视觉比对
- **THEN** 比对 MUST 失败

#### Scenario: 大面积中等差值被拦下

- **GIVEN** 实拍图整体相对基线图偏移了一个明显的色阶
- **WHEN** 执行视觉比对
- **THEN** 比对 MUST 失败

#### Scenario: 尺寸不匹配直接失败

- **GIVEN** 实拍图与基线图的尺寸不同
- **WHEN** 执行视觉比对
- **THEN** 系统 MUST 直接判定失败并报出双方尺寸，MUST NOT 缩放后再比对

### Requirement: 基线更新必须显式

系统 MUST NOT 在基线图缺失时静默创建基线。只有在调用方显式请求更新基线时，系统才 SHALL 写入基线文件。

#### Scenario: 基线缺失且未请求更新时失败

- **GIVEN** 某个视觉场景没有对应的基线图
- **WHEN** 在未请求更新基线的情况下执行视觉比对
- **THEN** 系统 MUST 报错，MUST NOT 把本次抓帧结果写成基线

#### Scenario: 显式请求时写入基线

- **WHEN** 显式请求更新基线并执行抓帧
- **THEN** 系统 MUST 把本次抓帧结果写入基线文件

### Requirement: 比对失败必须产出可供人眼定位的材料

比对失败时，系统 SHALL 在输出目录中同时产出本次实拍图与一张差异可视化图，使人能直接看到差异发生在画面的哪个位置。系统 MUST NOT 只给出数值结论。

#### Scenario: 失败时产出实拍图与差异图

- **GIVEN** 某个视觉场景的比对超出阈值
- **WHEN** 视觉比对结束
- **THEN** 输出目录中 MUST 同时存在该场景的实拍图与差异可视化图
- **AND** 差异可视化图 MUST 以与无差异区域可区分的方式标出差异像素的位置

### Requirement: 视觉基线固定使用内嵌默认材质

无窗口 capture 与其 golden SHALL 使用内嵌默认材质，MUST NOT 应用本机用户材质覆盖。受默认材质替换影响的场景 MUST 经完整渲染链路重新生成并逐图复核；既有双阈值比较规则 MUST 保持不变。

#### Scenario: 本机覆盖不影响视觉基线

- **GIVEN** 本机配置了一个有效用户材质目录
- **WHEN** 生成或比对 capture 场景
- **THEN** 输出 MUST 使用内嵌默认材质
- **AND** 用户目录内容 MUST NOT 改变任何 golden 或比较结果

#### Scenario: 默认视觉变化仍使用既有阈值

- **GIVEN** 内嵌默认材质改变了多个已映射 layer 的像素
- **WHEN** 显式更新并复核受影响的视觉基线
- **THEN** 更新后的场景 MUST 继续使用既有双阈值比较
- **AND** MUST NOT 通过放宽阈值接受材质或渲染缺陷

### Requirement: 远环与水下场景顺序及近环保护保持不变

抓帧场景清单 MUST 保留 `far-horizon` 为倒数第二个场景，并 MUST 保留 `water-underwater` 为唯一末场景。重建材质视觉基线时，系统 MUST 在写入任何 golden 前，以两个 disposable application 和相同生效 registry、世界种子、场景状态、相机及渲染配置分别抓取启用与禁用 LOD 的 `far-horizon`；两次 control 除 `lodEnabled` 外 MUST 等价。系统 MUST 复用既有几何推导的顶部与底部受保护行，对两张当前帧执行逐像素近环比较；任一受保护行不同 MUST 拒绝整次更新且不得覆盖任何 golden。每个已经成功构造的 control application MUST 在成功、后续构造失败或 guard 失败路径关闭；guard 通过并关闭两者后，系统 MUST 再构造一个 fresh LOD-on application，且只有该 application MAY 按正常完整场景顺序执行正式 capture 与写盘。该 control MUST NOT 依赖旧 golden 是否存在，既有视觉比较阈值 MUST 保持不变。

#### Scenario: 远环紧邻末尾水下场景

- **GIVEN** 完整 capture 场景清单
- **WHEN** 检查其末尾顺序
- **THEN** `far-horizon` MUST 位于 `water-underwater` 之前
- **AND** `far-horizon` MUST 是倒数第二个场景，`water-underwater` MUST 是唯一末场景

#### Scenario: 重立默认材质基线先执行材质无关近环 control

- **GIVEN** 调用方显式请求为新的内嵌默认材质更新整套 golden
- **WHEN** 系统准备覆盖第一张 golden
- **THEN** 系统 MUST 先用同一生效 registry 和两个 disposable application 完成 LOD on/off `far-horizon` 成对抓帧并执行受保护行比较
- **AND** 该 control MUST 在旧 golden 缺失时仍执行
- **AND** 任一近环差异 MUST 使整次更新失败且所有既有 golden 保持不变

#### Scenario: 正式 capture 从 fresh application 开始

- **GIVEN** LOD on/off control 已通过
- **WHEN** 系统开始正式完整 capture
- **THEN** 两个 control application MUST 已关闭
- **AND** 正式 `runCapture` MUST 接收一个未执行过 `far-horizon` control scene 的 fresh LOD-on application
- **AND** 正式场景 MUST 按普通 capture 的既有完整顺序运行

#### Scenario: control 生命周期失败时全部关闭

- **GIVEN** 任一 control application 构造失败、近环 guard 失败，或 fresh 正式 application 构造失败
- **WHEN** 更新路径返回错误
- **THEN** 每个已经成功构造的 application MUST 被关闭
- **AND** 正式 capture MUST NOT 在 guard 失败或 control application 尚未关闭时开始

#### Scenario: 真正的远景带差异不阻止材质基线更新

- **GIVEN** LOD on/off 成对抓帧只在几何推导的远景带存在差异，受保护的顶部与底部行逐像素一致
- **WHEN** 系统执行材质 golden 更新
- **THEN** 近环 control MUST 通过
- **AND** 系统 MAY 在继续使用既有双阈值的前提下写入经复核的内嵌默认材质 golden

### Requirement: 视觉基线覆盖统一方块与 HUD 风格

系统 SHALL 通过既有无窗口固定场景记录并比对当前产品默认方块材质与世界呈现。地形场景 MUST 覆盖内嵌默认 layer 与没有内嵌映射时的程序化回退。常显 HUD（快捷栏贴条与选中框、状态行图标、氧气气泡、采掘/进食轨道、物品名弹条、准星、聊天呈现与权威命中 marker）的 GPU 呈现已退役，无头抓帧路径 MUST NOT 产生这部分像素；它们的呈现验收 SHALL 由 `game-overlay-webview` capability 的前端组件断言与 `frontend/visual` 部件基线承接（本机 Chrome 截图、既有双阈值），MUST NOT 再由 capture golden 承接。GPU 保留面（容器浮动面板、容器悬停 tooltip 与 HUD atlas）MUST 由 `inventory-crafting`、`workbench-crafting`、`chest-container` 与 `furnace-container` 四景继续做像素验收。世界类场景 golden 中常显 HUD 条带与准星的消失属合法波及，随本 change 经既有显式更新路径重新生成并逐图复核。更新基线时 MUST 继续执行既有显式更新、无窗口完整渲染链路和双阈值规则；不得创建或聚焦前台游戏窗口，不得导入、临摹或复制 Mojang 像素。

`materials-showcase` MUST 保持既有固定正午、固定相机和确定性夹具，并经与交互客户端相同的完整呈现链路收敛后无窗口抓取，不得创建或聚焦前台游戏窗口。夹具 MUST 同时覆盖 14 种新材料、八格连续草地、相邻玻璃、相邻树叶、原木顶面年轮与侧面树皮，以及干耕地与湿耕地各至少一个可见列（含下沉顶面的完整几何）。既有双阈值 MUST 保持不变。

抓帧场景清单 MUST 按以下完整顺序运行（24 景）：`terrain-noon`、`avatar-nametag`、`inventory-crafting`、`workbench-crafting`、`chest-container`、`furnace-container`、`debug-panel`、`skylight-tunnel`、`block-light-room`、`torch-night`、`bed-night`、`materials-showcase`、`target-block-feedback`、`oak-grove`、`ai-companion`、`sword-combat`、`hostile-mob`、`water-surface-slope`、`mining-crack-early`、`mining-crack-heavy`、`main-menu`、`settings-menu`、`far-horizon`、`water-underwater`。`hud-hotbar-health`、`hud-survival-feedback` 与 `hud-item-name-popup` 三景随常显层 GPU 呈现退役从清单移除，清单 MUST NOT 再包含任何只承载常显 HUD 像素的场景。清单 MUST 保留 `target-block-feedback`、`oak-grove` 与 `ai-companion` 的既有名称及相对顺序，`ai-companion` MUST 继续紧随 `oak-grove`，并 MUST 保持 `sword-combat`、`hostile-mob`、`water-surface-slope` 的相邻顺序，`mining-crack-early` 与 `mining-crack-heavy` MUST 依次紧随 `water-surface-slope` 且先于 `main-menu`，`settings-menu` MUST 紧随 `main-menu`，`far-horizon` MUST 为倒数第二，`water-underwater` MUST 为唯一末场景。所有场景 MUST 使用与交互客户端相同的完整呈现链路收敛后无窗口抓取，且不得创建或聚焦前台游戏窗口。

#### Scenario: 地形与 HUD 风格变化产生可审查基线

- **GIVEN** 既有固定场景与渲染链路可用
- **WHEN** 显式更新本变更影响的视觉基线
- **THEN** `terrain-noon` MUST 包含当前内嵌默认材质及没有内嵌映射 layer 的程序化回退
- **AND** `terrain-noon` 的画面 MUST NOT 出现快捷栏、状态行、氧气、采掘/进食轨道、弹条、准星、聊天或命中 marker 像素
- **AND** 该图 MUST 由无窗口完整渲染链路产出并继续使用既有双阈值

#### Scenario: 常显 HUD 像素退出无头抓帧

- **GIVEN** 常显 HUD 的 GPU 呈现已退役且容器界面关闭
- **WHEN** 抓取任一非菜单相位的固定场景
- **THEN** 画面 MUST NOT 出现任何常显 HUD 像素，与 `survival-hud-presentation`「容器保留面 GPU 资源契约重钉」的关闭态 0 quad/0 glyph 一致
- **AND** 快捷栏、状态行、氧气、采掘/进食轨道、弹条、准星、聊天与 marker 的呈现验收 MUST 由 `game-overlay-webview` 的前端组件断言与 `frontend/visual` 部件基线承接

#### Scenario: 完整场景顺序收缩为 24 项

- **GIVEN** 完整无窗口 capture 场景清单
- **WHEN** 检查全部场景名称与顺序
- **THEN** 清单 MUST 恰好包含本 requirement 列出的 24 项，且顺序与之逐项一致
- **AND** 清单 MUST NOT 包含 `hud-hotbar-health`、`hud-survival-feedback` 或 `hud-item-name-popup`
- **AND** `far-horizon` MUST 是倒数第二个场景，`water-underwater` MUST 是唯一末场景

#### Scenario: 打开背包场景验证容器 GPU 保留面

- **GIVEN** `inventory-crafting` 装入固定背包、个人 2×2 网格中一条已匹配的真实原料形状、非空产物格和一个已选来源格
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 画面 MUST 同时呈现原创像素框、36 个凹槽、2×2 网格、产物格与背包/合成标题，全部属于容器面板保留面
- **AND** 画面 MUST NOT 出现生命/饥饿状态行、氧气气泡或快捷栏贴条，容器面板与 tooltip 保留面 MUST NOT 因常显层退役而缺失
- **AND** 打开态保留面最坏组合 MUST 继续由 `survival-hud-presentation` 的 218 quad/268 glyph 契约钉住

#### Scenario: 远端玩家场景只继承地形背景变化

- **GIVEN** 远端玩家与名牌的渲染逻辑没有变化，但场景共享的当前产品默认地形背景发生变化
- **WHEN** 更新本变更影响的视觉基线
- **THEN** `avatar-nametag` MUST 继承当前地形背景
- **AND** 远端玩家轮廓、颜色与名牌文字 MUST 保持既有可观察语义（名牌属世界呈现，不随常显层退役消失）

#### Scenario: 材料展示保持既有验收夹具

- **GIVEN** `materials-showcase` 的固定夹具已装入客户端镜像
- **WHEN** `materials-showcase` 完成网格和上传收敛并抓帧
- **THEN** 图像 MUST 同时显示 14 种新材料各一个近景样本及多方块表面、跨至少一个 AO 或天空光拆分边界的八格连续草地、两个相邻玻璃方块、两个相邻树叶方块，以及原木顶面年轮与侧面树皮、干耕地与湿耕地各一个可见列（两列顶面呈现在下沉高度而非整格顶面）
- **AND** 玻璃后方方块 MUST 可见，树叶孔洞和光照 MUST 可辨认，相同 cutout 方块的内部面 MUST 不可见

#### Scenario: 材料展示只走无窗口完整链路

- **GIVEN** `materials-showcase` 使用固定正午与固定相机
- **WHEN** 生成或比对 `materials-showcase`
- **THEN** 抓帧 MUST 使用与交互客户端相同的完整呈现链路
- **AND** MUST NOT 创建或聚焦前台游戏窗口，且 MUST 继续使用现有双阈值

#### Scenario: 全部正式基线需重新生成并完整复核

- **GIVEN** 常显 HUD 的 GPU 呈现退役与 WebView HUD 组件承接已经落地
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 按既有完整顺序重新生成全部 24 张正式 golden
- **AND** 调用方 MUST 逐张人工复核全部 24 张图像后才能接受更新，且既有双阈值 MUST 保持不变

#### Scenario: 伙伴场景与当前末尾顺序并存

- **GIVEN** 完整无窗口场景清单
- **WHEN** 检查 `target-block-feedback` 之后的场景名称与顺序
- **THEN** `oak-grove` 与 `ai-companion` MUST 保持既有名称，且 `ai-companion` MUST 紧随 `oak-grove`
- **AND** `water-surface-slope` MUST 位于 `ai-companion` 之后，`main-menu` 与 `settings-menu` MUST 依次相邻并位于 `far-horizon` 之前，`far-horizon` MUST 是倒数第二个场景，`water-underwater` MUST 是唯一末场景

#### Scenario: 橡树林通过正常渲染链路抓取

- **GIVEN** `oak-grove` 的固定世界种子、生成区块、正午时间与相机已经装入客户端镜像
- **WHEN** 场景完成预热、网格收敛和上传并抓帧
- **THEN** 图像 MUST 由与交互客户端相同的完整呈现链路产出，且 MUST 显示固定橡树地貌
- **AND** 抓帧 MUST NOT 创建或聚焦前台游戏窗口，且 MUST 继续使用既有双阈值

#### Scenario: AI 伙伴通过统一呈现链路抓取

- **GIVEN** `ai-companion` 已重置前一场景的 remote、companion、chat、inventory、panel、container、mining、damage 和 item-drop 状态，并装入固定伙伴和聊天夹具
- **WHEN** 场景完成预热和上传并抓帧
- **THEN** 图像 MUST 由统一的人形与名牌呈现链路产出，且 MUST 同时显示伙伴人形与中文名牌“阿木”
- **AND** accepted 事件与 `@阿木 挖石头` 输入属聊天呈现，已迁 WebView HUD 组件，画面 MUST NOT 出现聊天行或聊天输入框像素；其验收由前端组件断言承接
- **AND** 抓帧 MUST NOT 创建或聚焦前台游戏窗口，且 MUST 继续使用既有双阈值

#### Scenario: 目标反馈通过正常渲染链路验证遮挡

- **GIVEN** `target-block-feedback` 的固定夹具命中一个已注册材料方块
- **WHEN** 场景完成预热、网格收敛和上传并抓帧
- **THEN** 图像 MUST 同时显示该方块的细轮廓、中文名称和被地形正确遮挡的边
- **AND** 场景 MUST 使用与交互客户端相同的完整呈现链路并保持正确遮挡，不得创建或聚焦前台游戏窗口

#### Scenario: 打开背包的基线不受目标提示影响

- **GIVEN** `inventory-crafting` 场景打开背包
- **WHEN** 显式更新所有视觉基线
- **THEN** `inventory-crafting` MUST 不显示目标轮廓或名称
- **AND** 背包与合成区域的容器保留面语义 MUST 保持不变
- **AND** 只有经逐图复核确认由常显层退役、当前产品默认材质或共享地形背景变化引起时，它的 golden MAY 更新

#### Scenario: 基线更新不改变阈值或场景尾序

- **GIVEN** 调用方在常显层退役后的基线上更新全部正式 golden
- **WHEN** 检查生成结果和比较配置
- **THEN** `water-surface-slope`、`main-menu`、`settings-menu`、倒数第二的 `far-horizon` 与唯一末场景 `water-underwater` 的尾序 MUST 保持不变
- **AND** 既有双阈值 MUST 保持不变，任何差异 MUST 经逐图人工复核而不得通过放宽阈值接受

#### Scenario: 完整场景顺序加入生存反馈

- **GIVEN** 常显 HUD 像素已迁 WebView 组件（本 change 退役 `hud-survival-feedback` 景）
- **WHEN** 检查 capture 场景清单
- **THEN** 生存反馈的呈现验收 MUST 由 `game-overlay-webview` 的前端组件断言与 `frontend/visual` 部件基线承接
- **AND** 场景顺序约束由「完整场景顺序收缩为 24 项」承载

#### Scenario: 生存反馈场景固定且不污染后续场景

- **GIVEN** 退役场景的状态恢复纪律（临时 predictor、生命、氧气、饥饿和采掘状态在场景结束一并恢复）
- **WHEN** 保留场景依次运行
- **THEN** 该纪律 MUST 由保留场景与呈现状态机继续遵守，后续场景 MUST NOT 继承任何夹具值

#### Scenario: 打开背包场景复用同一向外状态栈

- **GIVEN** `inventory-crafting` 保留景验证容器 GPU 保留面
- **WHEN** 场景呈现打开的背包与状态栈构图
- **THEN** 状态栈（生命/饥饿/氧气）呈现已迁 WebView，GPU 画面 MUST 只包含容器保留面
- **AND** 保留面与 WebView 状态栈互不相交的构图由前端组件断言承接

#### Scenario: 合并后的全部正式基线需重新生成并完整复核

- **GIVEN** 常显层退役与容器保留面钉值已落地
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 按既有完整顺序重新生成全部 24 张正式 golden
- **AND** 调用方 MUST 逐张人工复核全部 24 张图像后才能接受更新，且既有双阈值 MUST 保持不变

#### Scenario: 合并基线更新不改变阈值或场景尾序

- **GIVEN** 常显层退役后的基线重生成
- **WHEN** 检查比较配置与场景尾序
- **THEN** 既有双阈值 MUST 保持不变，任何差异 MUST 经逐图人工复核而不得通过放宽阈值接受
- **AND** `far-horizon` MUST 保持倒数第二，`water-underwater` MUST 保持唯一末场景

### Requirement: 视觉基线覆盖三类容器像素界面

系统 SHALL 具有恰好 24 个正式无窗口场景，`workbench-crafting` MUST 紧随 `inventory-crafting`，`chest-container` 与 `furnace-container` MUST 依次紧随 `workbench-crafting`，`torch-night` MUST 紧随 `block-light-room` 且先于 `bed-night`，`sword-combat` MUST 紧随 `ai-companion` 且先于 `hostile-mob`。完整顺序 MUST 与当前 `captureScenes` 表一致，`far-horizon` MUST 为倒数第二且 `water-underwater` MUST 为唯一末场景。既有显式更新、无窗口完整渲染链路和双阈值 MUST 保持不变；两张 far-horizon diagnostic controls MUST 继续不计入正式场景或 golden。golden 基线 SHALL 恰好为 24 张；四类容器场景验证的是容器面板与 tooltip 的 GPU 保留面，打开态保留面最坏组合 MUST 继续满足 `survival-hud-presentation` 的 218 quad/268 glyph 契约，关闭态 MUST 保持 0 quad/0 glyph。本变更 MUST NOT 借机放宽任何阈值。

#### Scenario: 完整场景顺序固定为 19 项

> 标题沿用历史名（openspec 1.7 的 MODIFIED 漂移守卫不支持 Scenario 改名）；当前正式清单为 24 项，语义以下述断言为准。

- **GIVEN** 完整正式 capture 场景清单
- **WHEN** 检查场景数量、名称与顺序
- **THEN** 清单 MUST 恰好包含上述 24 项
- **AND** `workbench-crafting` MUST 紧随 `inventory-crafting` 且在 `chest-container` 之前
- **AND** `torch-night` MUST 紧随 `block-light-room` 且在 `bed-night` 之前
- **AND** `sword-combat` MUST 紧随 `ai-companion` 且在 `hostile-mob` 之前
- **AND** `mining-crack-early` 与 `mining-crack-heavy` MUST 依次紧随 `water-surface-slope` 且在 `main-menu` 之前
- **AND** `far-horizon` MUST 保持倒数第二，`water-underwater` MUST 保持唯一末项

#### Scenario: 背包与合成场景覆盖普通容器皮肤

- **GIVEN** `inventory-crafting` 装入固定背包、个人 2×2 网格中一条已匹配的真实原料形状、非空产物格和一个已选来源格
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 场景构造 MUST 同时呈现原创像素框、36 个凹槽、2×2 网格、产物格、背包/合成标题与来源轮廓
- **AND** 画面 MUST NOT 出现常显 HUD 的状态行或快捷栏贴条，命中区域与目标提示隐藏语义 MUST 保持不变

#### Scenario: 工作台场景覆盖 3×3 网格与镜像不对称配方

- **GIVEN** `workbench-crafting` 装入固定背包、已打开的工作台 3×3 网格、至少一条水平镜像不对称配方的合法摆放与合法产物
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 场景构造 MUST 同时呈现 3×3 网格、非空产物格与统一凹槽风格
- **AND** 场景 MUST NOT 依赖前一场景留下的容器或网格状态

#### Scenario: 箱子场景覆盖 63 格

- **GIVEN** `chest-container` 装入固定玩家背包、27 格箱子内容和一个已选来源栏位
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 图像 MUST 同时显示箱子标题、统一像素框、63 个栏位凹槽与来源轮廓
- **AND** 场景不得依赖前一场景留下的熔炉或箱子状态

#### Scenario: 熔炉场景覆盖 39 格和流程图示

- **GIVEN** `furnace-container` 装入固定玩家背包、已确认熔炉三格、部分燃烧/熔炼进度和一个已选来源栏位
- **WHEN** 场景经完整链路收敛并无窗口抓取
- **THEN** 图像 MUST 同时显示熔炉标题、统一凹槽、来源轮廓、输入/燃料/输出与可辨认的火焰/箭头图示
- **AND** 39 个统一栏位的布局 MUST 完整可审查且场景不得依赖前一场景留下的容器状态

#### Scenario: 全部正式 golden 重新生成并逐图复核

- **GIVEN** 容器保留面、火把纹理层与全部 overlay 的最终实现已经通过聚焦测试
- **WHEN** 显式更新视觉基线
- **THEN** 系统 MUST 重新生成全部 24 张正式 golden，并只提交实际场景文件
- **AND** 调用方 MUST 逐张人工复核 24 张图像后才能接受，且 MUST NOT 通过放宽双阈值接受差异
- **AND** 抓帧 MUST NOT 创建或聚焦前台游戏窗口，MUST NOT 导入、临摹或复制 Mojang 像素

#### Scenario: torch-night 纳入 golden 比对

- **GIVEN** 非更新模式运行 capture
- **WHEN** 执行到 `torch-night`
- **THEN** 该场景 MUST 与对应 golden 按既有双阈值比对，差异图规则与其它场景一致
- **AND** golden 目录 MUST 存在 `torch-night.png`，正式 golden 总数 MUST 恰好为 24 张

#### Scenario: 未受影响场景 golden 逐字节不变

- **GIVEN** 常显层退役的显式基线更新只波及携带常显 HUD 像素或共享世界背景的场景
- **WHEN** 运行 capture 并与本变更合入前的 golden 比对
- **THEN** `main-menu.png` 与 `settings-menu.png` 的 PNG 字节 MUST 逐字节不变
- **AND** 退役的 `hud-hotbar-health.png`、`hud-survival-feedback.png` 与 `hud-item-name-popup.png` MUST 从 golden 目录移除，golden 目录 MUST 恰好有 24 张 PNG
- **AND** 集成后全部 24 张 golden 在 compare 模式下 MUST 全部通过既有双阈值

### Requirement: 视觉基线覆盖调试面板

调试面板的呈现（读数区、参数分组段头、可编辑行与只读行对比、选中行高亮）SHALL 由 WebView 组件承担，其结构、可编辑语义与像素验收由 `game-overlay-webview` 的前端组件断言与 `frontend/visual` 部件基线承接。无头抓帧路径的程序化面板渲染路径 MUST NOT 保留：`debug-panel` 场景 MUST 继续存在并装入面板可见态，用于钉住「面板可见不产生任何无头面板像素」这一边界，其 golden SHALL 为同一相位与相机下的纯世界底图。既有双阈值 MUST 保持不变。

#### Scenario: 面板场景产出可审查基线

- **WHEN** 显式更新视觉基线
- **THEN** `debug-panel` 的 golden MUST 为固定正午、固定相机的纯世界底图
- **AND** 画面 MUST NOT 出现读数区、参数分组、可编辑行高亮或任何面板 chrome 像素
- **AND** 该图 MUST 由无窗口完整渲染链路产出

#### Scenario: 面板默认隐藏不影响其余场景

- **GIVEN** 抓帧路径为该场景装入面板可见态
- **WHEN** 抓取其余不涉及面板的场景
- **THEN** 那些场景的画面 MUST NOT 出现面板
- **AND** 它们的基线 MUST NOT 因面板状态机的存在而改变

#### Scenario: 面板可见不产生无头像素

- **GIVEN** `debug-panel` 场景已装入面板可见态
- **WHEN** 该场景与同一相机、同一世界时间的非面板世界帧比较
- **THEN** 两者 MUST 不存在面板像素差异
- **AND** 面板读数与参数行的呈现验收 MUST 由前端组件断言承接

### Requirement: 天空光通道场景只在收敛后无窗口抓取

抓帧场景清单 MUST 保留既有 `skylight-tunnel` 和 `block-light-room`，并 MUST 保留 `far-horizon` 为倒数第二个场景、`water-underwater` 为整个清单的唯一末场景。两个光照场景 MUST 通过与交互客户端相同的世界状态与呈现链路装入固定夹具，并在全部有限后台工作收敛后才抓取；抓帧 MUST NOT 创建或聚焦游戏窗口，既有视觉差异阈值 MUST 保持不变。设备型号 MUST NOT 作为该视觉语义验收的额外条件。`skylight-tunnel` MUST 保持露天入口、半遮蔽过渡区、超过传播距离的深处以及固定正午时间、相机位置和朝向。`block-light-room` MUST 是午夜的完整封闭房间，唯一照明源 MUST 是一个发光块；图像 MUST 显示室内由近到远的方块光衰减，房外和缺失邻区边界 MUST 无漏光。

#### Scenario: 收敛后的通道抓帧显示梯度

- **GIVEN** `skylight-tunnel` 的固定夹具已装入客户端镜像
- **WHEN** 无窗口抓帧完成预热、网格收敛和上传
- **THEN** 输出目录 MUST 包含名为 `skylight-tunnel` 的图像，且图像 MUST 显示从洞口到深处可辨认的递减天空光梯度

#### Scenario: 收敛后的封闭房间只由发光块照亮

- **GIVEN** `block-light-room` 的封闭房间夹具已装入客户端镜像，世界时间为午夜且房间内只有一个发光块
- **WHEN** 无窗口抓帧完成预热、网格收敛和上传
- **THEN** 输出目录 MUST 包含名为 `block-light-room` 的图像，图像 MUST 显示室内从光源由近到远的衰减，房外与边界 MUST 无亮缝

#### Scenario: 未收敛时拒绝抓取半成品

- **GIVEN** `skylight-tunnel` 或 `block-light-room` 的有限后台工作在有界预热内未收敛
- **WHEN** 抓帧程序准备写出该场景
- **THEN** 程序 MUST 返回包含场景名的错误，且 MUST NOT 写入该场景的新 golden

#### Scenario: 房间边界漏光超过阈值时失败

- **GIVEN** `block-light-room` 的实拍图在房外或缺失邻区边界出现非预期亮区
- **WHEN** 图像与已接受 golden 执行既有双阈值比对
- **THEN** 任一局部高差值或大面积中等差值超过既有阈值 MUST 使验证失败，且不得自动放宽阈值

#### Scenario: 新 golden 需完整场景复核

- **GIVEN** 调用方显式请求更新视觉基线
- **WHEN** 受支持设备的无窗口抓帧生成包含 `skylight-tunnel`、`block-light-room`、`target-block-feedback`、`oak-grove`、`ai-companion`、倒数第二的 `far-horizon` 与末尾 `water-underwater` 的新图像集
- **THEN** 调用方 MUST 人工复核全部场景图像后才能接受新 golden，且任一场景未收敛、方块光、伙伴、水下或远环语义不成立，或比对超过既有阈值 MUST 失败
- **AND** 由当前内嵌默认材质引起的既有 golden 变化 MAY 在完整复核后接受，设备型号 MUST NOT 成为额外的接受或拒绝条件

### Requirement: 主菜单与设置菜单无窗口 capture 场景

视觉场景表 SHALL 包含 `main-menu` 与紧随其后的 `settings-menu`：两者均以既有 `640x360` 离屏渲染路径运行，回读像素并与 golden 按既有双阈值比对。菜单 chrome 由 WebView 呈现且 MUST NOT 参与无头 capture——两张 golden 的内容 SHALL 为对应相位的世界全景底图（纯 wgpu 渲染、确定性像素），WebView 层的结构与视觉 MUST 由前端组件断言测试覆盖而非像素 golden。两场景 MUST 依次排在 `far-horizon` 之前，`far-horizon` 仍为倒数第二、`water-underwater` 仍为最后。

#### Scenario: 场景表顺序与两张 UI 图产出

- **GIVEN** `captureScenes` 场景表
- **WHEN** 检查 UI 场景与尾序
- **THEN** `main-menu` 与 `settings-menu` MUST 依次相邻并位于 `far-horizon` 之前
- **AND** `far-horizon` MUST 仍为倒数第二、`water-underwater` MUST 仍为最后
- **AND** 抓帧目录 MUST 含 `main-menu.png` 与 `settings-menu.png`

#### Scenario: 两个 UI 场景参与 golden 比对

- **GIVEN** 非更新模式运行 capture
- **WHEN** 执行到 `main-menu` 与 `settings-menu`
- **THEN** 两张 PNG MUST 分别与对应 golden 逐像素比对
- **AND** MUST 继续使用既有阈值与差异图产出规则
- **AND** 比对对象 MUST 为纯 wgpu 全景底图，无头路径 MUST NOT 初始化 WebView、产生任何菜单 chrome 像素或网络请求

### Requirement: 未受影响场景 golden 逐字节不变

菜单相位场景 `main-menu` 与紧随其后的 `settings-menu` 的 golden SHALL 为纯 wgpu 全景底图，不携带常显 HUD 像素与菜单 chrome（WebView 层由前端组件断言覆盖）。凡不影响全景渲染路径的呈现层变更（含常显 HUD 的 GPU 呈现退役）MUST NOT 改变这两张 golden 的字节。自然短草改变默认方块 registry 与共享世界背景后，本变更 MAY 只更新经逐图归因确认确由自然短草可见性引起的既有正式 golden；`oak-grove` 的固定夹具 MUST 包含至少一株在相机中可辨识的短草，并 MUST 经既有四 quad alpha-cutout 植物路径呈现。共享相同固定世界全景且实际出现短草的 `main-menu.png` 或 `settings-menu.png` MAY 在逐图归因后更新；所有未受自然短草影响的正式场景 golden SHALL 保持逐字节不变。正式场景清单 MUST 继续恰好为既有 24 项并保持当前顺序（常显 HUD 三场景已退役、`mining-crack` 对已加入），MUST NOT 新增 `natural-grass` 或任何第 25 个正式场景；全部更新与比对 MUST 继续使用既有双阈值，MUST NOT 通过放宽阈值接受差异。其余场景的 golden MUST 只经既有显式更新路径变化，且每一处差异 MUST 可归因到已声明的呈现层或共享世界背景变化，不得以放宽双阈值吸收。

#### Scenario: 非设置场景不受变更影响

- **GIVEN** 全部既有 24 个正式场景及变更前 golden
- **WHEN** 运行 capture 并逐图归因自然短草造成的像素差异
- **THEN** 只有画面中确实出现自然短草或共享自然短草世界背景的既有 PNG MAY 更新
- **AND** 其余每个场景的 PNG 字节 MUST 保持不变，且不影响全景渲染路径的呈现层差异 MUST NOT 单独改变菜单相位 golden 的字节
- **AND** MUST NOT 新增任何第 25 个场景或 golden

#### Scenario: oak-grove 明确承重短草外观

- **GIVEN** `oak-grove` 的固定世界种子、区块、正午时间、相机与既有完整渲染链路
- **WHEN** 场景完成预热、网格收敛与上传并无窗口抓帧
- **THEN** 图像 MUST 包含至少一株可辨识的自然短草
- **AND** 短草 MUST 显示透明边缘与交叉植物轮廓，而不是实心立方体或不透明矩形

#### Scenario: 24 项场景顺序与阈值保持不变

- **WHEN** 检查 `captureScenes` 表并比较本变更允许更新的 golden
- **THEN** 场景数量 MUST 恰好为 `24` 且名称与顺序 MUST 与变更前一致
- **AND** 每张图 MUST 继续使用既有双阈值与差异图规则

### Requirement: 火把获得原创程序化纹理层

五种火把 MUST 共用一个 16×16 原创程序化 alpha-cutout 材质层，图案为窄木柄加暖色火芯；像素 MUST 全部来自现有程序化生成路径、alpha 值 MUST 仅为 0/255、图层 MUST 非空且与既有全部层的像素不同。MUST NOT 引入任何外部 PNG / Mojang 版权材质。

#### Scenario: 层唯一性与 alpha

- **GIVEN** 程序化 material builder 输出的火把层
- **WHEN** 与既有全部层逐个像素比较
- **THEN** 该层 MUST 非空、alpha 仅 0/255、不与任何既有层逐像素相同

### Requirement: `torch-night` 无窗口夜景场景

无窗口 capture MUST 提供 `torch-night` 场景（位于 `block-light-room` 之后、`materials-showcase` 之前）：固定夜晚封闭暗室，同时出现落地与至少两种墙面火把，并用像素差证明火把附近亮度高于远处、透明边缘不是实心矩形。场景 MUST 经与交互客户端相同的完整呈现链路收敛后抓取，MUST NOT 创建或聚焦任何前台窗口；其 golden 由本变更经显式基线更新写入并逐图人工复核，MUST NOT 通过放宽双阈值接受差异。

#### Scenario: 场景可构造且包含多形态

- **GIVEN** `torch-night` 场景构造代码
- **WHEN** 无窗口运行该场景
- **THEN** 场景 MUST 至少包含一朵落地与两朵不同方向的墙面火把
- **AND** 运行不得创建或聚焦前台窗口

#### Scenario: 近亮远暗证明

- **GIVEN** 场景渲染完成后的图像
- **WHEN** 比较火把附近与远处的同材质表面像素
- **THEN** 火把附近 MUST 明显更亮

#### Scenario: 透明边缘

- **GIVEN** 渲染后的火把本体像素
- **WHEN** 检查其外接矩形边缘
- **THEN** 边缘 MUST 存在 alpha=0 的透明像素，证明不是实心矩形精灵

### Requirement: 夜行者无窗口场景

无窗口 capture 场景表 SHALL 保留 `hostile-mob` 场景，其前一项 MUST 为 `sword-combat`，后一项 MUST 为 `water-surface-slope`；`water-underwater` MUST 仍为唯一末场景、`far-horizon` MUST 仍为倒数第二。场景 MUST 装入固定夜间确定性夹具：火把边缘固定位置呈现 8 只夜行者（其中一只处于受击状态、一只处于追逐中），场景 MUST 经与交互客户端相同的完整呈现链路无窗口抓取，MUST NOT 创建或聚焦前台游戏窗口，并 MUST 继续使用既有双阈值比对。

#### Scenario: 场景表顺序与导出

- **GIVEN** 完整 capture 场景表
- **WHEN** 检查 `ai-companion` 之后的场景
- **THEN** `hostile-mob` MUST 位于 `sword-combat` 之后、`water-surface-slope` 之前，`far-horizon` MUST 为倒数第二，`water-underwater` MUST 为唯一末场景
- **AND** 抓帧运行 MUST 产出 `hostile-mob` 图像

#### Scenario: 夹具确定性且无名标

- **GIVEN** `hostile-mob` 夹具已装入客户端镜像（8 只夜行者，1 只受击、1 只追逐）
- **WHEN** 场景完成预热、网格收敛与上传并抓帧
- **THEN** 图像 MUST 同时显示 8 只夜行者人形，其中 MUST 可辨认受击与追逐呈现，MUST NOT 出现任何相关名称标签，并 MUST 与既有双阈值保持一致
- **AND** 场景结束后临时夜行者、受击与追逐状态 MUST 一并恢复，使后续场景不继承任何夹具值

#### Scenario: 无窗口完整链路

- **GIVEN** `hostile-mob` 场景使用固定夜间世界时间与固定相机
- **WHEN** 生成或比对 `hostile-mob`
- **THEN** 抓帧 MUST 使用与交互客户端相同的完整呈现链路，MUST NOT 创建或聚焦前台游戏窗口

### Requirement: sword-combat 无窗口场景固定呈现权威命中反馈

无窗口 capture SHALL 保留 `sword-combat` 场景，位于 `ai-companion` 之后、`hostile-mob` 之前。场景 MUST 使用固定相机与世界时间，选中 `Durability=125` 的铁剑，通过合法 UUIDv4 远端玩家 spawn/state 镜像呈现一次权威确认后的受击者，并显示 0.35 水平击退后的姿态或位置关系。权威命中 marker 的像素呈现已迁 WebView HUD 组件：画面 MUST NOT 出现 marker 像素，但场景 MUST 继续在收敛后、最终帧前经 `PinVolatile` 重新武装 marker 计时状态机，钉住「6 个成功呈现帧窗口」的权威语义；场景切换的共享 reset MUST 清除 combat feedback，避免污染后续 `hostile-mob`。场景 MUST 生成并比对 `sword-combat.png`，使用既有双阈值且不得创建或聚焦前台游戏窗口。

#### Scenario: 场景状态包含非满耐久铁剑、目标和 marker

- **GIVEN** `sword-combat` 固定夹具已装入
- **WHEN** 场景完成预热与上传并准备最终帧
- **THEN** MUST 显示选中的 `Durability=125` 铁剑、合法远端玩家与可观察的 0.35 水平击退关系
- **AND** 画面 MUST NOT 出现 marker、快捷栏或状态行像素；marker 的呈现验收由 WebView HUD 组件断言承接

#### Scenario: PinVolatile 在最终帧前重新武装 marker

- **GIVEN** 场景收敛帧可能已经消耗初次 marker 窗口
- **WHEN** capture 准备最终抓帧
- **THEN** `PinVolatile` MUST 把 marker 重置为 6 个成功呈现帧，权威侧 `CombatMarkerVisible` MUST 为真
- **AND** 该计时语义 MUST 不因 marker 像素已迁 WebView 而改变

#### Scenario: 场景切换清除 combat feedback

- **GIVEN** `sword-combat` 已留下 combat tick 与 marker 帧数
- **WHEN** capture 切换到 `hostile-mob`
- **THEN** shared presentation reset MUST 清除 combat feedback，后续场景 MUST 不继承 marker 或去重状态

#### Scenario: golden 只新增 sword-combat

> 标题沿用历史名（openspec 1.7 的 MODIFIED 漂移守卫不支持 Scenario 改名）；本变更不新增任何场景，语义以下述断言为准。

- **GIVEN** 24 张正式 golden 是当前清单的全部基线
- **WHEN** 显式生成并逐图审核本清单
- **THEN** tracked golden MUST 恰好覆盖 22 个场景名，MUST NOT 包含已退役场景的 PNG
- **AND** 任何 PNG 变化 MUST 逐图归因并明确批准，否则不得接受

### Requirement: `bed-night` 无窗口夜景场景

无窗口 capture MUST 提供 `bed-night` 场景（位于 `torch-night` 之后、`ai-companion` 之前；`far-horizon` MUST 仍为倒数第二、`water-underwater` MUST 仍为唯一末场景）：固定夜间卧室内同时呈现至少两种朝向的床形态，并用像素差证明床的原创配色与半高轮廓在夜间光照下可辨认。场景 MUST 经与交互客户端相同的完整呈现链路收敛后抓取，MUST NOT 创建或聚焦任何前台窗口；其 golden 由本变更经显式基线更新写入并逐图人工复核，MUST NOT 通过放宽双阈值接受差异。

#### Scenario: 场景可构造且包含多朝向

- **GIVEN** `bed-night` 场景构造代码
- **WHEN** 无窗口运行该场景
- **THEN** 场景 MUST 至少包含两个不同朝向的完整床形态（床头与床尾同框）
- **AND** 运行不得创建或聚焦前台窗口

#### Scenario: 夜间配色可辨认且不污染后续场景

- **GIVEN** `bed-night` 场景完成预热、收敛与抓帧
- **WHEN** 与其 golden 按既有双阈值比对
- **THEN** 比对 MUST 通过，且床的配色与半高轮廓 MUST 可从图像辨认
- **AND** 场景结束后临时床与时间夹具 MUST 一并恢复，使后续场景不继承任何夹具值

### Requirement: GIF 动态基线覆盖牛行为剧本

系统 SHALL 为牛行为剧本提供 GIF 动态基线：吃草前后、持麦靠近、击杀与牛肉掉落按 tick 步进抓帧（禁用墙钟），以标准库 `image/gif` 编码并存入 `testdata/` 下 `.gif` 基线。单基线帧预算 MUST 有界（建议 ≤8fps×6s=48 帧，参照录制上限与 manifest 纪律）。比对时解码逐帧并沿用双阈值（最大通道差与差异像素占比）逐帧裁决；全部帧通过方为通过。只允许新增基线，既有 PNG 基线 MUST 逐字节不动。

#### Scenario: 剧本 GIF 可复现生成

- **GIVEN** 同一份代码与同一台机器
- **WHEN** 连续两次生成同一剧本 GIF 基线
- **THEN** 两次解码后的逐帧差异 MUST 落在既定双阈值以内

#### Scenario: 击杀剧本覆盖死亡与掉落

- **GIVEN** 击杀剧本的 GIF 基线
- **WHEN** 逐帧解码审查
- **THEN** 序列 MUST 包含红闪侧倒的死亡过渡帧与牛肉掉落小方块帧

#### Scenario: 超帧预算被拒绝

- **GIVEN** 一次请求超过帧预算上限的 GIF 录制
- **WHEN** 系统校验参数
- **THEN** 系统 MUST 在任何帧捕获之前拒绝该请求

#### Scenario: 旧 PNG 基线不受影响

- **GIVEN** 新增的 GIF 基线已入库
- **WHEN** 运行既有 PNG 视觉比对
- **THEN** 全部既有 PNG 基线的字节 MUST 与入库前一致，且比对 MUST 继续使用既有双阈值

### Requirement: GIF 编码使用自适应调色板

GIF 基线编码 MUST NOT 使用固定 256 色调色板（如 Plan9，绿/棕损伤肉眼可见）：编码器 SHALL 按基线逐个构建自适应调色板（直方图取色 + 确定性并列决胜 + 抖动），标准库内实现，不引入新依赖。同一输入 MUST 逐字节确定输出；比对仍解码逐帧沿用双阈值。

#### Scenario: 草地牛肉颜色保真

- **GIVEN** 同一帧 raw 像素
- **WHEN** 分别经固定调色板与自适应调色板编码再解码
- **THEN** 自适应版本的草绿/牛肉红棕与 raw 的通道差 MUST 显著小于固定版本，且输出 MUST 确定可复现

### Requirement: GIF 剧本呈现完整过程语义

lure 剧本 MUST 呈现牛跟随：牛逐帧向持麦玩家移动并止步、朝向玩家（跟随逻辑由 sim 单测兜底，GIF 只验呈现）。graze 剧本 MUST 呈现草→泥土切换：前段牛低头、结算帧草方块变为泥土（Step 内经既有夹具写块路径切换，只允许切换触发格一格）。kill 剧本 MUST 呈现先倒后肉：掉落 upsert 时机保持权威诚实（死亡当 tick），呈现滞后由客户端关联逻辑承载（见死亡呈现）。

#### Scenario: lure 牛位移跟随

- **GIVEN** lure 剧本的 GIF 基线
- **WHEN** 逐帧解码审查首末帧牛位
- **THEN** 牛 MUST 向玩家方向位移且末帧距玩家约止步距离，玩家 MUST 在帧内可见

#### Scenario: graze 草变泥土同镜呈现

- **GIVEN** graze 剧本的 GIF 基线
- **WHEN** 逐帧解码审查触发格
- **THEN** 前段 MUST 为草地 + 低头牛，结算帧起 MUST 为泥土格

