## MODIFIED Requirements

### Requirement: 方块材质具有稳定且可辨识的像素图案

图形客户端产品路径 SHALL 在完整、确定性的 16×16 程序化材质注册表上，以经许可适配并内嵌的 Pastelcraft（MIT）子集替换具有直接对应素材的 layer；没有内嵌映射的 layer MUST 保留程序化像素。程序化注册表 MUST 保持可独立构造，作为完整最终回退与测试基线。程序化 registry 与内嵌默认包合成的产品默认材质 MUST 让草方块、石砖、矿石、熔炉、铁块、箱子及新增的圆石、平滑石、沙子、砾石、原木、木板、树叶、玻璃、砖块、白色羊毛、红色瓦块、黏土、雪块和苔藓圆石具有不同于单纯随机噪声的稳定结构图案。仓库提供的程序化与内嵌材质 MUST NOT 使用 Mojang 或其他未经授权的二进制美术资源；内嵌第三方材质 MUST 携带其许可证、署名、来源与修改说明。本地用户 override 的像素内容、结构与许可证不属于仓库分发物契约。

terrain 材质采样相位 SHALL 由当前面的世界坐标轴决定；相同材质被 AO、天空光、贪心合并上限、区段或区块边界拆分时 MUST 保持连续，负世界坐标 MUST 继续周期采样。程序化草顶明暗簇 MUST 跨 16×16 边界包裹，程序化草侧缘 MUST 使用闭合周期序列，最右列与最左列的草缘高度差 MUST 不超过一个像素。

程序化 registry 与内嵌默认包中的树叶和玻璃 SHALL 使用 alpha 仅为 `0` 或 `255` 的基础层；无论像素来自产品默认还是用户 override，树叶和玻璃 MUST 继续走现有 atlas 与 terrain cutout 分类，透明像素 MUST 在 fragment 阶段按既有规则丢弃，其余像素 MUST 按该 pass 的既有规则写入深度目标。cutout mip MUST 使用保持覆盖率的降采样，其他不透明层 MUST 保持颜色平均语义。有效用户 override MAY 在任意 layer 使用 PNG 可表达的中间 alpha，loader MUST NOT 因此拒绝；该像素选择不得改变 render classification、几何、layer ID 或映射。

#### Scenario: 同一方块材质重复生成保持一致

- **GIVEN** 相同代码版本与方块注册表
- **WHEN** 重复创建两份程序化材质集合
- **THEN** 每个材质层的 RGBA 字节 MUST 完全一致

#### Scenario: 已映射 layer 使用内嵌默认像素

- **GIVEN** 一个逻辑 layer 在内嵌默认包中存在经许可且直接对应的素材
- **WHEN** 图形客户端构造产品默认材质集合
- **THEN** 该 layer MUST 使用内嵌默认像素而非程序化像素

#### Scenario: 未映射 layer 保持程序化结果

- **GIVEN** 一个逻辑 layer 在内嵌默认包中没有直接对应素材
- **WHEN** 图形客户端构造产品默认材质集合
- **THEN** 该 layer MUST 与独立程序化注册表中的对应 layer 逐字节一致
