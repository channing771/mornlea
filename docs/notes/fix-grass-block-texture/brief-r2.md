# 任务 brief（第二轮）：修复方块侧脸纹理朝向错误

## 根因（已由控制会话确认，开工前请复核关键证据）

`terrain.wgsl` 的 `face_uv` 对侧面采样的轴约定错误，导致侧脸纹理被翻转/旋转：

```wgsl
fn face_uv(world: vec3f, axis: u32) -> vec2f {
    if (axis == 0u) { return vec2f(world.y, world.z); }   // ±X 面:u=纵向!纹理旋转 90°
    if (axis == 1u) { return vec2f(world.z, world.x); }   // ±Y 面(顶/底):两个水平轴,正确
    return vec2f(world.x, world.y);                        // ±Z 面:v=world.y,纹理上下颠倒
}
```

- wgpu 纹理坐标 (0,0) = 图像左上角,即 v=0 采样图像**顶行**;`v = world.y` 使图像顶行(草绿缘)
  落在方块**底部**:±Z 面上下颠倒;±X 面 `u = world.y` 把纵向当横向,整张纹理旋转 90°。
- 控制会话已按该公式做过精确模拟(采样 `texel[v*16][u*16]`):+Z 面输出为"上 8 行泥土、下 8 行草",
  +X 面输出为"绿色竖条纹在左、泥土在右"——与需求(草带在侧面顶部、横向)完全相反。
- 同一 bug 影响所有带方向性的侧面材质:草侧(最醒目)、熔炉正面、箱子锁扣、原木树皮(±X 面树皮会横过来)、
  雪侧;植物交叉斜面 `uv = vec2f(world.x, world.y)` 同样把小麦纹理上下颠倒。
- 意图证据:`voxel-visual-presentation` 主规格要求"侧面草缘 MUST 具有可辨识的深度变化与**下垂像素**"、
  "原木侧面 MUST 显示**纵向树皮**";`procedural_test.go` 的 `TestGrassTexturesWrapAcrossPeriodicBoundaries`
  与 `TestNaturalBlocksHaveClusteredSurfaceDetail` 断言草侧纹理的草带在图像 0..8 行(顶部)。纹理是按
  "图像顶行=方块顶部" 创作的,是着色器采样方向与它不一致。
- 同源公式三处:`terrain.wgsl`(含植物分支 `uv = vec2f(world.x, world.y)`)、`lod.wgsl`(远环侧裙,注释声明与近环"逐字同源")、
  `water.wgsl`(水面侧边)。采样器地址模式为 Repeat(`render/mod.rs` block-sampler),负 v 会正确环绕,
  因此改用 `-world.y` 安全。

## 修复方案

1. `engine/crates/mornlea_client/shaders/terrain.wgsl`:
   - `face_uv`:axis 0 → `vec2f(world.z, -world.y)`;axis 2 → `vec2f(world.x, -world.y)`;axis 1 保持 `vec2f(world.z, world.x)`。
   - 植物分支(face>=6):`uv = vec2f(world.x, -world.y);`,并更新其上方注释(写明:交叉斜面的 v 轴是纵向,
     必须与侧面同用 `-world.y` 才能让纹理顶行在格子顶部)。
   - 在 `face_uv` 附近补一条中文注释,说明约定与原因:纹理坐标 v=0 是图像顶行,世界 y 向上,
     侧面/植物的纵向分量必须取 `-world.y`(采样器 Repeat 使负坐标环绕,连续性与负坐标语义不变)。
2. `lod.wgsl`、`water.wgsl` 的 `face_uv` 同步为同一公式(保持"逐字同源"的既有约定与注释)。
3. **语义守护测试(重点)**:新建 `engine/crates/mornlea_client/src/render/side_tests.rs`(render/mod.rs 加
   `#[cfg(test)] mod side_tests;`),按 `plant_tests.rs` 的离屏渲染范式(自带夹具;若与 plant/water 的
   helper 大量重复,先读 `docs/test-organization.md` 的「Rust 映射」一节判断该不该建共享 helper 中心):
   - 夹具:atlas 单层 16×16,上 8 行(R0..R7)不透明**红**,下 8 行(R8..R15)不透明**绿**(纯色即可,逐层逐 mip)。
   - 用例 A:+Z 面(`Face::PosZ = 5`),单元格 (8,8,8) 与 plant_tests 同约定(区段 (0,4,0) 原点条件),水平正交相机从 +Z;
     断言画面中央 16×16 方块区域:上半像素红色主导、下半绿色主导(用 plant_tests 的通道主导判据思路,红:r>g+40 且 r>b+40;
     绿:g>r+40 且 g>b+40),即"纹理顶行在方块顶部"。
   - 用例 B:+X 面(`Face::PosX = 1`),从 +X 水平正交观察:同样断言红上绿下(守卫 90° 旋转回归)。
   - 用例 C:植物交叉斜面(`Face::PlantDiagA = 6`),从 +Z 水平正交观察:红上绿下(守卫植物 v 轴)。
   - 用例 D(可选):−Z 面(`Face::NegZ = 4`)从 −Z 观察,同样红上绿下(守卫负方向)。
   - 无 GPU 适配器时按既有约定跳过(`OffscreenRenderer::new` 返回 None 即 skip),不 fail。
4. **golden 重生成**:`make rust` 后 `make visual-update`(内置 LOD on/off 近环 control,全部 17 张都会因侧面
   方向修正而变化),再 `make visual-check` 验证 17/17、每场 0/230400 差异。**修改前先拍一张当前 worktree 的
   `materials-showcase.png` 与 `terrain-noon.png` 做对照(可选)**,否则直接按 update 流程走。
5. **文档**:`docs/notes/progress.md` 末尾追加中文条目(根因:侧面/植物的 v 轴误用 `world.y` 导致纹理上下颠倒/
   旋转 90°;修复:三处 `face_uv` 与植物分支统一 `-world.y`;新增 side_tests.rs 语义守护;golden 重生成)。
   上一轮已修的"草侧 alpha 半透明"保留不动。

## 硬性约束

- 不改:mesh 编码、`internal/mesh`、Go 侧、layer 编号、`isCutoutLayer`、`applyPack`、协议/存档/engine ABI/client ABI/benchmark scenario。
- 不触碰:`AGENTS.md`、`CLAUDE.md`、`docs/superpowers/`、`openspec/`;不要删除旧 golden、不要手动改 golden、不要绕过 near-band control。
- 注释/文档用中文;Go 标识符与既有术语保留英文;新注释提及 WGSL 语法时无需反引号,提及 Go/Rust 标识符可用 `` 包裹。
- 工作树直接修改,不要 git commit;控制会话负责版本控制。

## 验证(按序,报告附命令与结果)

1. `cd engine && rustup run 1.97.1 cargo test --workspace --locked`(需包含新 side_tests + 既有 plant/water 测试)
2. `cd engine && rustup run 1.97.1 cargo fmt --check && cargo clippy --workspace --all-targets -- -D warnings`
3. `GOCACHE=$(pwd)/.gocache go test ./cmd/mornlea -run 'Capture|Scene|NearBand|Compare' -count=1`
4. `make visual-update` → `make visual-check`(17/17、0/230400、exit 0)
5. `GOCACHE=$(pwd)/.gocache go test ./internal/assets -count=1`
6. 任何与 brief 的偏差必须说明理由。

## 报告

写到 `docs/notes/fix-grass-block-texture/report-implementer-r2.md`(在既有目录里),回复:
Status(DONE/DONE_WITH_CONCERNS/BLOCKED)、改动清单、测试证据摘要、golden 变化概况(哪些场景变了/值)、疑虑、报告路径。
