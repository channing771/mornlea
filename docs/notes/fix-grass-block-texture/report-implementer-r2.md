# 实现者报告（第二轮）：修复方块侧脸纹理朝向错误

## Status
**DONE**

## 改动清单（一句话）
把 `terrain.wgsl` 的 `face_uv`（±X/±Z 两轴）与植物交叉斜面分支、以及 `lod.wgsl`/`water.wgsl` 的 `face_uv` 统一改为纵向分量取 `-world.y`，新增 `side_tests.rs` 离屏语义守护测试（render/mod.rs 挂载），追加 `progress.md` 中文条目，并重生成全部 17 张 golden。

## 具体改动
- `engine/crates/mornlea_client/shaders/terrain.wgsl`：`face_uv` axis 0 → `vec2f(world.z, -world.y)`、axis 2 → `vec2f(world.x, -world.y)`、axis 1 保持 `vec2f(world.z, world.x)`；`face_uv` 上方新增中文约定注释；植物分支 `uv = vec2f(world.x, -world.y)`（注释同步更新为「交叉斜面 v 轴是纵向，必须与侧面同用 -world.y」）。
- `engine/crates/mornlea_client/shaders/lod.wgsl` / `water.wgsl`：`face_uv` 同步为同一公式（保持「逐字同源」约定与注释）。
- `engine/crates/mornlea_client/src/render/side_tests.rs`（新建）：单层「上半红/下半绿」16×16 atlas，用例覆盖 +Z、+X、植物交叉斜面（`Face::PlantDiagA`）与 −Z，以「上半红主导/下半绿主导」判据直接读出「纹理顶行在方块顶部」，另加空场景守卫计数口径。
- `engine/crates/mornlea_client/src/render/mod.rs`：`#[cfg(test)] mod side_tests;`（一行）。
- `docs/notes/progress.md`：末尾追加第二条 `fix-grass-block-texture` 中文条目。
- `cmd/mornlea/testdata/golden/*.png`：全部 17 张 golden 重生成。
- 备注：上一轮（alpha 合成不透明化 `grass_side.png`）的资产/provenance/`internal/assets` 改动与 `docs/texture-packs.md`、`scripts/composite_grass_side.go` 全部保留未动。

## 测试证据（命令 + 结果）
1. `cd engine && rustup run 1.97.1 cargo test --workspace --locked` → **exit 0**（两次均通过；mornlea_engine 160 passed、mornlea_client 全绿，含新 side_tests）。
   - 聚焦：`rustup run 1.97.1 cargo test -p mornlea_client side_tests -- --nocapture --test-threads=1` → **5 passed; 0 failed; 4.18s**（确认真实走 GPU 渲染路径，非跳过）。
2. `cd engine && rustup run 1.97.1 cargo fmt --check` → **clean**；`cargo clippy --workspace --all-targets -- -D warnings` → **exit 0**。
3. `GOCACHE=$(pwd)/.gocache go test ./cmd/mornlea -run 'Capture|Scene|NearBand|Compare' -count=1` → **ok（1.709s）**。
4. `make rust` → **exit 0**；`make visual-update` → **exit 0**（17 场景全部写入基线，LOD on/off 近环 control 已执行并通过）；`make visual-check` → **17/17 场景，每场最大通道差 0、差异像素 0/230400（0.0000%），exit 0**（最终一次于空白行清理后再跑）。
5. `GOCACHE=$(pwd)/.gocache go test ./internal/assets -count=1` → **ok（0.584s）**。
6. `GOCACHE=$(pwd)/.gocache go test ./internal/archcheck -count=1` → **ok（6.034s）**（含注释标识符门禁，未受影响——本轮新增注释在 WGSL/Rust）。

### 变异检查（证明测试有牙齿）
临时把 `terrain.wgsl` 退回到旧公式后 `cargo test -p mornlea_client side_tests`：
- `+Z` 面：`r_up=0 g_up=128 r_low=128 g_low=0` → 上半绿、下半红（**上下颠倒**，FAIL）。
- `−Z` 面：同上（FAIL）。
- 植物交叉斜面：`r_up=0 g_up=128 r_low=128 g_low=0` → 上下颠倒（FAIL）。
- `+X` 面：`r_up=64 g_up=64 r_low=64 g_low=64` → 红/绿各半的四象限分界（**90° 旋转**，FAIL）。
随后已恢复修正公式并复验（`make rust + make visual-check` 17/17 零差异）。

## Golden 变化概况
- 相比第二轮开工前快照，**全部 17 张 golden 的 SHA-256 均改变**（无一保持不变）。
- 变化原因：侧脸纹理朝向修正影响所有渲染了带方向侧面纹理的场景；且 `far-horizon` 经 `lod.wgsl`、`water-surface-slope` 经 `water.wgsl` 同步变化。
- 特别说明：第二轮开工前工作树已有 12 张 golden 因上一轮 alpha 合成而改动；本轮顺序重生成后，连上一轮未触及的 5 张（`ai-companion`、`block-light-room`、`skylight-tunnel`、`target-block-feedback`、`water-underwater`）也随朝向修正而变化。
- `make visual-check` 证实重生成后的 golden 与最新渲染器输出逐像素一致（0/230400 差异），基线自洽。

## 疑虑 / 备注
- **无 DONE 之外的风险项**。下列为过程性说明：
  1. `GOCACHE=$(pwd)/.gocache` 是 brief 指定的缓存路径，产生了一个 `.gocache` 临时目录；它属构建缓存、非交付物，已删除以保持工作树干净（不影响任何 Go 测试结果）。
  2. 在首次 `visual-update` 之后我对 `terrain.wgsl` 做了一次**纯空白行清理**（删一个多余空行）。该改动行为中性（WGSL 空白不改变语义），其后重新跑了 `make rust + make visual-check` 确认仍 17/17 零差异，golden 与清理后的源码仍一致。
  3. `side_tests.rs` 采用**自带夹具**而非新建共享 helper 中心：与同模块 `plant_tests.rs`/`water_tests.rs` 的既有先例一致（两者在 `atlas_bytes`/`pack_quad` 等上虽有重叠但刻意各自独立），且按 `docs/test-organization.md`「被多于一个测试模块引用的 helper 才迁入中心，单模块私有留在消费文件」判断，新抽共享中心属于不必要的重构（会触碰已通过的 plant/water 测试），故未做。
  4. 无 GPU 适配器时 `side_tests` 依既有约定跳过（`OffscreenRenderer::new` 返回 None 即 `return`），不 fail；在本机上确认真实 GPU 渲染（4.18s）。

## 报告路径
- `docs/notes/fix-grass-block-texture/report-implementer-r2.md`

