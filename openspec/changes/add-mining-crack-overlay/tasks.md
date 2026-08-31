# add-mining-crack-overlay 任务

约束：每任务 fresh implementer + TDD（red→green→refactor）+ SPEC/QUALITY 双评审；
代码注释一律中文且不得出现任务编号；提交信息单行英文
`<type>(<scope>): <subject>`。基线 SHA `d3e9fcbb`。

## 1. 程序化裂纹材质层

- [ ] 1.1 失败测试先行：`internal/assets` 新增裂纹层测试——层号冻结
  `LayerCrack0..LayerCrack9` = 68..77 且 `layerCount` = 78；裂纹纹理逐阶段
  确定性一致；阶段 i 的非透明像素集合 ⊇ 阶段 i−1（增量生长）；全部像素
  alpha ∈ {0, 255} 且非透明像素落在深棕/深灰域；`isCutoutLayer` 对 10 层
  全真；`textureBindings` 含 `crack_0..crack_9` 且 AtlasPixels 输出长度随
  layerCount 走。
- [ ] 1.2 实现：`blocks.go` 枚举末位追加 10 层 + cutout 分类 + 绑定槽位；
  新文件 `internal/assets/crack.go` 固定种子程序化生成（16×16、透明背景、
  中心起始、分枝加密的像素裂纹）。
- [ ] 1.3 验证：`go test ./internal/assets -race -count=1`；
  `go test ./internal/mesh -run TestNativeOracleParity -count=1`（层枚举
  追加不得扰动植物/耕地/火把/床区间的跨语言守卫）。

## 2. Go 侧镜像补全与实例编码

- [ ] 2.1 失败测试先行：`internal/render/hud` 测试 `MiningOverlay` 新字段
  零值安全（HUD 布局不消费）；`internal/render` 新增
  `BlockCrackStage` 边界表（0 ticks、饱和 clamp、required=0 哨兵、各区间
  断点）与 `EncodeBlockCrackInstances` 字节布局测试（80B/实例、mat4 平移
  与 1.002 缩放、64..68 为 `LayerCrack0+stage` 的 f32、尾部零、不可见返回
  空流）；`internal/client` 测试 `EncodeRenderFrame` 仅在裂纹流非空时追加
  tag 10 段且无裂纹帧字节与追加前逐位一致。
- [ ] 2.2 实现：`hud/layout.go` 补 `Target`/`HasTarget`；新文件
  `internal/render/block_crack.go`（类型 + 阶段映射 + 几何构建，扩边
  0.001）；`frame_streams.go` 增 `EncodeBlockCrackInstances`；
  `internal/client/render.go` 增 `CrackInstances`/`frameTagCrack`/条件
  appendTLV；`app_messages.go` 填充镜像新字段；`app_frame.go` 派生裂纹流
  （门控：Active+HasTarget、`!blockTargetReset && !clientSessionClosed`、
  `vista == nil`）并填入 `client.RenderFrame`。
- [ ] 2.3 验证：`go test ./internal/render/... ./internal/client
  ./cmd/mornlea/app -race -count=1`。

## 3. Rust crack pass 与帧解析

- [ ] 3.1 失败测试先行：`ffi.rs` 测试覆盖 tag 10 解析（合法段、重复段拒绝、
  越界/未知 tag 拒绝、长度非 4 对齐拒绝）与 `FRAME_TAG_MAX = 10`；
  `render` 侧测试 crack pass 实例校验（80B 倍数、容量 1）、atlas 未上传时
  不绘制、`water_tests` 计数门禁更新为 6 并点名 crack 阶段；`shaders.rs`
  登记由既有存在性单测覆盖。
- [ ] 3.2 实现：`ffi.rs`（tag 常量、白名单、`FrameInput.crack_instances`、
  `empty_passes` 纳入）；新文件 `render/crack.rs`（绑定组、带 UV 立方体、
  alpha blend + 只读深度 LessEqual 管线、rebuild_bind、upload/record）；
  `shaders/crack.wgsl`（采样 atlas 层 + `alpha < 0.5` discard + daylight
  乘色）；`shaders.rs` 登记；`mod.rs` 装配 pass、`upload_atlas` 重建
  bind、合法性校验、outline 之后名牌之前记录；`water_tests.rs` 计数 5→6
  并更新注释与规格 delta 对应关系。
- [ ] 3.3 验证：`make rust`；`(cd engine && cargo test -p mornlea_client
  --locked)`；`go build ./...`（cgo 侧头文件未动，编译即回归）。

## 4. 视觉场景与收尾门禁

- [ ] 4.1 失败测试先行：`cmd/mornlea/capture` 新增裂纹场景测试——固定世界中
  以 `fixture.Mining`（含 Target/HasTarget）钉住两个阶段（浅裂纹/重裂纹）
  的离屏帧断言裂纹像素出现在目标方块投影区域内、非 active 场景断言无裂纹
  像素。
- [ ] 4.2 实现：`capture.go` 场景表新增裂纹场景（含 PinVolatile 位姿钉死）；
  `--update-golden` 生成新基线；确认既有基线逐位不变。
- [ ] 4.3 验证：`go test ./cmd/mornlea/capture -race -count=1`；
  `make visual-check`；`gofmt -l .` 无输出；`go vet ./...`；
  `go test ./... -race`；`make rust-check`；
  `openspec validate --all --strict --no-interactive`。
