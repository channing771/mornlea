# visual-verification 变更

## MODIFIED Requirements

### Requirement: 抓帧模式产出确定性的视觉场景图像

系统 SHALL 提供一个无头抓帧模式，按固定的视觉场景清单产出图像文件。每个视觉场景 MUST 由确定性的世界状态、固定的相机位姿与固定的抓帧时机共同定义，三者 MUST 全部是常量而非运行时输入。抓帧 MUST NOT 创建或聚焦任何前台游戏窗口。抓帧渲染 MUST 复用与交互式客户端相同的渲染调用链，不得使用专供抓帧的旁路。

系统 SHALL 支持显式场景子集：调用方可以场景名清单请求只执行完整清单的一个子集。子集 MUST 按完整场景清单的固有顺序保序执行，不得重排；子集路径产出的每个场景图像 MUST 与全量路径使用同一 golden 基线、同一双阈值与同一差异图规则比对。未请求子集时系统 MUST 执行完整场景清单，行为与既有语义逐项一致。场景子集 MUST NOT 改变场景清单本身、任何场景的定义或全量路径的任何行为；全量运行仍是权威视觉门禁。

#### Scenario: 抓帧产出全部场景图像

- **WHEN** 以抓帧模式指定一个输出目录运行且未请求场景子集
- **THEN** 该目录中 MUST 为场景清单里的每个场景产出一份图像文件，文件名 MUST 与场景名一致

#### Scenario: 显式场景子集按表序产出

- **GIVEN** 调用方以场景名清单请求一个非空子集
- **WHEN** 以抓帧模式运行
- **THEN** 输出目录 MUST 仅为子集所列场景产出图像文件
- **AND** 子集场景的执行顺序 MUST 与完整场景清单的相对顺序一致
- **AND** 每个子集场景 MUST 与对应 golden 按既有双阈值比对，差异图规则不变

#### Scenario: 未知或重复场景名被拒绝

- **GIVEN** 场景子集清单包含未知场景名、重复场景名或空项
- **WHEN** 解析启动参数
- **THEN** 系统 MUST 拒绝启动并给出包含问题名称的错误，MUST NOT 静默忽略该项或降级为全量运行

#### Scenario: 子集标志缺少抓帧模式被拒绝

- **WHEN** 指定场景子集或显式请求 GIF 而未指定抓帧模式
- **THEN** 系统 MUST 拒绝启动并说明原因

#### Scenario: 同一提交上重复抓帧结果稳定

- **GIVEN** 同一份代码与同一台机器
- **WHEN** 连续两次以抓帧模式运行
- **THEN** 两次产出的图像之间的差异 MUST 落在既定的比对阈值以内

#### Scenario: 图像的颜色通道顺序正确

- **GIVEN** 渲染目标采用与图像文件不同的颜色通道顺序
- **WHEN** 抓帧写出图像文件
- **THEN** 图像文件中每个像素的红、绿、蓝分量 MUST 与渲染目标中该像素的对应分量一致，不得整体偏色

### Requirement: GIF 动态基线覆盖牛行为剧本

系统 SHALL 为牛行为剧本提供 GIF 动态基线：吃草前后、持麦靠近、击杀与牛肉掉落按 tick 步进抓帧（禁用墙钟），以标准库 `image/gif` 编码并存入 `testdata/` 下 `.gif` 基线。GIF 不进入自动比对：生成产物 SHALL 仅供人工审查，逐帧自动裁决保持退役。GIF 生成时机 MUST 受运行模式门控：显式更新基线时 MUST 生成；纯比对运行默认 MUST NOT 生成，只有调用方显式请求时 SHALL 生成到抓帧输出目录。单基线帧预算 MUST 有界（建议 ≤8fps×6s=48 帧，参照录制上限与 manifest 纪律）。只允许新增基线，既有 PNG 基线 MUST 逐字节不动。

#### Scenario: 比对运行默认不生成 GIF

- **GIVEN** 未显式请求 GIF 的纯比对抓帧运行
- **WHEN** 场景表执行完毕
- **THEN** 系统 MUST NOT 生成任何 GIF 剧本
- **AND** 本次运行的比对结论 MUST 只由 PNG 场景比对决定

#### Scenario: 显式请求或更新时生成 GIF

- **GIVEN** 调用方显式请求 GIF，或本次运行为基线更新
- **WHEN** 场景表执行完毕
- **THEN** 系统 MUST 生成全部 GIF 剧本供人工审查（更新运行写入 `.gif` 基线，显式请求的比对运行写入抓帧输出目录）

#### Scenario: 剧本 GIF 可复现生成

- **GIVEN** 同一份代码与同一台机器
- **WHEN** 连续两次生成同一剧本 GIF
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

### Requirement: 远环与水下场景顺序及近环保护保持不变

抓帧场景清单 MUST 保留 `far-horizon` 为倒数第二个场景，并 MUST 保留 `water-underwater` 为唯一末场景。重建材质视觉基线时，系统 MUST 在写入任何 golden 前，以两个 disposable application 和相同生效 registry、世界种子、场景状态、相机及渲染配置分别抓取启用与禁用 LOD 的 `far-horizon`；两次 control 除 `lodEnabled` 外 MUST 等价。系统 MUST 复用既有几何推导的顶部与底部受保护行，对两张当前帧执行逐像素近环比较；任一受保护行不同 MUST 拒绝整次更新且不得覆盖任何 golden。每个已经成功构造的 control application MUST 在成功、后续构造失败或 guard 失败路径关闭；guard 通过并关闭两者后，系统 MUST 再构造一个 fresh LOD-on application，且只有该 application MAY 按正常完整场景顺序执行正式 capture 与写盘。调用方显式请求场景子集时，LOD on/off control 与受保护行比较 MUST NOT 因子集而跳过或弱化，正式 capture MUST 按该子集与完整清单一致的相对顺序执行。该 control MUST NOT 依赖旧 golden 是否存在，既有视觉比较阈值 MUST 保持不变。

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

#### Scenario: 子集更新仍先执行近环 control

- **GIVEN** 调用方以场景子集显式请求更新基线
- **WHEN** 系统准备覆盖第一张 golden
- **THEN** LOD on/off `far-horizon` 成对抓帧与受保护行比较 MUST 照常先行执行，任何近环差异仍 MUST 拒绝整次更新
- **AND** 正式 capture MUST 用 fresh LOD-on application 按子集的保序场景执行

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
