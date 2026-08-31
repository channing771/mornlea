# add-mining-crack-overlay 设计

基线 SHA：`d3e9fcbb`（main）。需求来源：需求方在任务书中给出的完整设计约束
（真实进度驱动、10 阶段、单一可复用 overlay、atlas/UV 切换、透明像素风），
本设计只做落地路径决策。

## 数据所有权与依赖方向

- 采集进度、目标、active 全部归服务端权威（`internal/sim/runtime/mining.go`
  的 `miningState`）。客户端只在 `cmd/mornlea/app` 镜像最后确认的
  `network.PlayerState.Mining*`（现状字段已齐全），本 change 把镜像补全
  目标方块位置并派生呈现，不新增任何预测或计时器。
- 依赖方向不变：`cmd/mornlea/app` → `internal/render` → `internal/client`
  → client ABI；Rust `mornlea_client` 只消费帧字节。`internal/render` 已
  依赖 `internal/assets`（`section_scheduler.go` 先例），裂纹层号从
  `assets.LayerCrack0` 取值，不复制常量。
- 跨 goroutine 边界无变化：裂纹流在 `Application.RenderFrame`（渲染线程）
  内从 `a.miningOverlay` 单线程派生，与 `hud.MiningOverlay` 的消费方式同源
  同约束。

## 关键决策

### 1. 进度→阶段映射放渲染层，镜像结构只补字段

- `hud.MiningOverlay` 追加 `Target core.BlockPos` 与 `HasTarget bool`
  两个字段（`app_messages.go` 在既有赋值点一并填充，`HasTarget` 恒随
  `MiningActive`；capture 既有 fixture 不设置 `HasTarget`，天然不触发裂纹，
  既有 golden 逐位不变）。HUD 进度条布局不消费这两个字段。
- `internal/render` 新增 `BlockCrackStage(progressTicks, requiredTicks uint16) int`：
  `requiredTicks == 0` 返回无效哨兵；否则 `min(9, floor(clamp(p,0,1)*10))`。
  阶段是呈现概念，不进 HUD 结构、不进网络。

### 2. 帧传输：新 TLV tag 10，零 ABI bump

- `internal/client/render.go`：`RenderFrame.CrackInstances []byte` +
  `frameTagCrack = 10`（tag 9 已被 client ABI v12 退役占用语义，跳过）；
  `EncodeRenderFrame` 仅在流非空时 `appendTLV`（镜像 tag 4/tag 8 条件段先例），
  无裂纹帧字节与现状逐位一致。Rust `FRAME_TAG_MAX` 8→10、`parse_frame` 新增
  分支，`seen` 数组扩容。
- 不新增任何 FFI 导出面 → client ABI 保持 v14；不用"把目标塞进 192B 帧头"
  的方案（帧头 layout 是跨版本契约，不该为单一呈现段扩张）。

### 3. Rust 渲染：独立 crack pass，复用 atlas 与 outline 的防冲突先例

- 新模块 `render/crack.rs` + 新 shader `shaders/crack.wgsl`（`shaders.rs`
  登记，存在性单测自动覆盖）。绑定组：camera uniform（80B：viewproj +
  daylight，镜像 `EntityPass`）+ 只读实例 storage + `texture_2d_array` atlas
  视图 + sampler；atlas 上传时与 `lod_pass.rebuild_bind` 同一批重建，atlas
  未上传前不绘制（与 `terrain_bind == None` 跳过同语义）。
- 实例布局 80 字节：mat4（64B）+ f32 atlas 层号（64..68）+ 12B 零填充，
  定长 1 实例容量。几何为带 UV 的 24 顶点单位立方体（每面 UV 0..1 全覆盖），
  indirect 绘制镜像 `EntityPass`。
- 深度策略：alpha blend + 深度只读 + `CompareLessEqual`，几何各向外扩
  `0.001`（合成边长 1.002，在任务允许的 1.001~1.005 带内）——与 outline 的
  外扩 + LessEqual 同一防 z-fight 先例；不启用 `DepthBiasState`（全仓 4 处
  均为 default，wgpu bias 对无深度写入的 pass 语义别扭，被否决）。
- 着色：采样后 `alpha < 0.5` discard（与 terrain cutout 同阈值，裂纹层
  alpha 二值化），rgb 乘 `daylight.x`（与 avatar 同相位，夜间裂纹随之变暗），
  不做面向法线的漫反射（裂纹是叠加层，若做面着色会与方块自身明暗打架）。
- pass 记录顺序：outline 之后、名牌之前（世界实体之后、HUD 之前，与 outline
  同带）。`water_tests` 的 `begin_render_pass` 调用点门禁 5→6，注释同步记录
  本阶段与规格 delta 的对应关系。
- 被否决的替代：把裂纹画进 outline 的既有 pass（两管线共用一个 render pass
  可不触碰调用点计数，但 outline pass 只在轮廓非空时记录，与裂纹可见性耦合
  出现条件组合漏洞，且 `EntityPass` 几何无 UV，改造面更大）；每目标常驻
  mesh/贴花（违背单一 overlay 目标）；独立纹理上传 FFI（ABI v15 bump，无必要）。

### 4. 材质：10 张程序化裂纹层追加在枚举末位

- `internal/assets/blocks.go`：`LayerCrack0..LayerCrack9`（68..77）追加在
  `LayerBedHeadEast` 之后、`layerCount` 之前——不扰动植物/耕地/火把/床区间，
  既有三处人手同步守卫（farmland/shaders.rs/terrain.wgsl 字面量）不受影响。
  `isCutoutLayer` 纳入裂纹层（alpha 二值 + 保覆盖率 mip 降采样，裂纹像素
  稀疏，平均降采样会在远处整层消失）。`textureBindings` 追加
  `crack_0..crack_9` 覆盖槽位（镜像 torch/bed 的"仅覆盖槽位"语义）。
- 新文件 `internal/assets/crack.go`：确定性（固定种子）程序化裂纹生成——
  16×16 RGBA、背景 alpha 0、裂纹像素 alpha 255、深棕/深灰系；逐阶段**增量
  生长**（第 i 阶段像素集合 ⊇ 第 i−1 阶段），阶段 0 为中心初始裂点，高阶段
  分枝加密加粗。不引入第三方依赖，不用 `math/rand` 全局源。
- 验收锚点：与 cozy/retro 风格协调由 capture golden 人工确认（judge）。

### 5. App 接线与清理门控

- `app_frame.go`：在既有 `blockOutline` 派生点旁派生 `BlockCrack`：
  门控 `HasTarget && a.miningOverlay.Active`，且与 outline 同受
  `!blockTargetReset && !a.clientSessionClosed` 约束；菜单全景相位
  （`vista != nil`）强制隐藏（镜像 `eyeInFluid` 在全景相位强制 false 的先例，
  防止世界空间内容渗入全景底图）；背包/容器打开与菜单相位由权威侧输入抑制
  自然归零（服务端 `miningHeld` 置 false → 下一批 PlayerState 非 active），
  呈现层再以菜单相位门控兜底同帧隐藏。
- 破坏同帧清理不依赖顺序巧合：裂纹流每帧从当前镜像整体重算，权威
  `MiningActive=false` 与 `BlockChanges` 在同一批消息落地后，下一帧流自然
  为空；不存在"残留最后一帧"的路径。

## 受影响文件清单

| 层 | 文件 | 动作 |
|---|---|---|
| assets | `internal/assets/blocks.go` | 层枚举 + cutout 分类 + 绑定槽位 |
| assets | `internal/assets/crack.go`（新） | 程序化裂纹生成 |
| render | `internal/render/block_crack.go`（新） | `BlockCrack`/`BlockCrackStage`/编码 |
| render | `internal/render/frame_streams.go` | `EncodeBlockCrackInstances` |
| hud | `internal/render/hud/layout.go` | `MiningOverlay` 补 Target/HasTarget |
| client | `internal/client/render.go` | `CrackInstances` + tag 10 + 条件 TLV |
| app | `cmd/mornlea/app/app_messages.go` | 镜像填充 Target/HasTarget |
| app | `cmd/mornlea/app/app_frame.go` | 派生裂纹流 + 门控 + 帧字段 |
| rust | `engine/crates/mornlea_client/src/ffi.rs` | tag 10 解析 |
| rust | `engine/crates/mornlea_client/src/render/crack.rs`（新） | crack pass |
| rust | `engine/crates/mornlea_client/shaders/crack.wgsl`（新） | shader |
| rust | `engine/crates/mornlea_client/src/render/shaders.rs` | 登记 |
| rust | `engine/crates/mornlea_client/src/render/mod.rs` | pass 装配/记录/atlas 重建 |
| rust | `engine/crates/mornlea_client/src/render/water_tests.rs` | 计数门禁 5→6 |
| capture | `cmd/mornlea/capture/capture.go` | 裂纹场景 |
| spec | `openspec/specs/*`（归档期） | delta 沉淀 |

## 风险与回退

- 风险：新 shader/pass 在无 GPU 环境（CI 容器）无法实例化——既有 pass 同约束，
  Rust 测试沿用水_tests 的既有 skip 机制，不新增环境分支。
- 风险：golden 基线漂移——裂纹流为空时帧字节逐位一致、渲染路径无变化，
  既有场景基线 MUST 逐位不变；唯一新增基线是新场景。
- 回退：revert 单一 change 分支即可；无 schema/协议/ABI 迁移包袱。

## 验证方法

- 定点：`go test ./internal/assets ./internal/render ./internal/render/hud
  ./internal/client ./cmd/mornlea/app -race -count=1`；
  `make rust && (cd engine && cargo test -p mornlea_client --locked)`。
- 视觉：`make visual-check`（既有基线逐位不变）+ 新场景 `--update-golden`
  生成基线后复跑确认稳定；golden 图由视觉验收（judge）确认裂纹阶段可判读。
- 收尾门禁：`gofmt`、`go vet ./...`、`go test ./... -race`、
  `openspec validate --all --strict --no-interactive`、`make rust-check`。
