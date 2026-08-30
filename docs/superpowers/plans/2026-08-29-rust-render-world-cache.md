# Rust Render World Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Each checkbox task needs a fresh implementer plus independent specification and quality reviews recorded in the change ledger.

**Goal:** 交付第一个独立 OpenSpec change：建立 Rust RenderWorld 缓存和 MRW1 更新 ABI，同时不改变当前生产 mesh、上传或绘制路径，也不触碰流体相关源码。

**Architecture:** mornlea_client v12 新增一个只接收 Go 已验证压缩 section/height 更新的 RenderWorld；它完成原子解析、紧凑规范化、epoch/revision/tombstone 状态管理，但本 change 不创建 mesh worker、不写 GPU pool，也不改变现有 frame ABI 的 Visible 输入。当前 engine 的 mesh/light/quad/greedy 源码直接承载流体语义；其共享 kernel 抽取与 Rust connectivity 迁移被明确留给流体负责人交付稳定边界后的 rust-render-mesh-pipeline change。

**Tech Stack:** Go 1.26、Rust 1.97.1（locked Cargo workspace）、cgo、手写 C ABI、wgpu 29（本 change 不修改 GPU pass）。

**Spec:** docs/superpowers/specs/2026-08-29-rust-client-render-world-design.md

## Global Constraints

- 只实现路线的第一个 change：rust-render-world-cache；共享 voxel kernel、mesh worker、GPU 上传编排和 Rust visibility 属于后续独立 change。
- client ABI MUST 从 v11 升到 v12；engine ABI MUST 保持 v8；协议 v32、玩家 schema v8、区块 schema v9、世界 metadata v3、companions.ai schema v4、hostile_mobs schema v1 和 benchmark scenario v20 不变。
- MRW1 是 render update 的唯一 magic；MRC1 已被 raycast cursor 使用，MUST NOT 复用。
- Go Mirror 是客户端逻辑真相来源；Rust RenderWorld 是派生、可重建的渲染缓存，MUST NOT 被 Go 用作协议、预测或游戏规则查询来源。
- Rust 只接收 Go 已校验的语义化 palette/bitpack 数据，MUST NOT 解析 network wire packet。
- 每个 ABI 入口 MUST 验证版本、handle、pointer、length、范围、布局与容量；失败 MUST NOT 留下部分状态或跨 FFI unwind。
- Go/Rust 间的 slice/pointer 只在同步调用期间有效；Rust MUST NOT 保存 Go 内存地址。
- 不改变流体状态、传播、tick、协议或专属 mesh 策略；也不得移动或重构 engine 中带流体语义的 input、light、quad、greedy 与其测试文件。
- 生产路径 MUST NOT 新增 Go fallback；本 change 保持现有生产 mesh/draw 路径不变，只新增未接管绘制的缓存入口。
- 本 change 的新入口只由 Rust/Go ABI 测试驱动，不从 cmd/mornlea/app 的实时消息路径调用；在 cache 尚未取代一次 Go 侧 mesh 工作前，避免为每次网络更新增加一份无收益的生产复制。
- 代码注释、GoDoc 与 Rust doc comment 使用中文；标识符和 ABI 名称保留英文。
- 在独立 worktree 执行；worktree 从当前 HEAD 建立，不复制当前工作区的未提交改动。合并时保留用户改动并逐处处理文档冲突。每项实现由 fresh implementer 完成，并经过规格合规与代码质量两次独立评审，结果写入 change ledger。

---

## File Structure

| 路径 | 职责 |
|---|---|
| openspec/changes/rust-render-world-cache/ | 本 change 的 proposal、delta spec、设计、任务清单与验证 ledger |
| engine/crates/mornlea_client/src/render/world.rs | MRW1 解析、规范化 section/column 缓存、epoch/revision/tombstone 状态机 |
| engine/crates/mornlea_client/src/render/world_tests.rs | RenderWorld 的纯 Rust 状态机与原子性测试 |
| engine/crates/mornlea_client/src/render/mod.rs | 持有 RenderWorld，并暴露只更新缓存的内部方法 |
| engine/crates/mornlea_client/src/ffi.rs | client ABI v12 常量与 mornlea_client_render_apply_world_updates 入口及 ABI 测试 |
| engine/crates/mornlea_client/src/lib.rs | client ABI 版本说明与 render world 模块挂载 |
| engine/include/mornlea_client.h | v12、MRW1 更新入口和 C 调用契约 |
| internal/client/render_world_update.go | Go 的 MRW1 值对象、压缩 snapshot 编码器和完整 chunk 批次构造 |
| internal/client/render_world_update_test.go | Go 编码布局、三种 palette 存储、reset/tombstone 与 chunk 构造测试 |
| internal/client/render.go | 新入口的 cgo 指令与 Renderer.ApplyRenderWorldUpdates bridge |
| internal/client/render_test.go | 有 GPU 时的 Go→client ABI 成功路径测试 |
| AGENTS.md、README.md、README.en.md、docs/architecture.md、docs/notes/lan-server.md、docs/notes/progress.md、openspec/specs/ | v12 当前事实、RenderWorld 阶段性所有权、主规格同步和完成记录 |

## Wire Contract Fixed by This Plan

MRW1 batch header 为 24 字节小端布局：

~~~text
0..4   magic = "MRW1"
4..6   layout_version = 1 (u16)
6..8   reserved = 0 (u16)
8..16  epoch (u64, non-zero)
16..20 record_count (u32, 1..=4096)
20..24 reserved = 0 (u32)
~~~

每条 record 是 32 字节头加 payload：

~~~text
0      tag (u8)
1      storage_kind (u8)
2      bits (u8)
3      reserved = 0 (u8)
4..8   dimension (i32)
8..12  section_x (i32)
12..16 section_y (i32)
16..20 section_z (i32)
20..28 revision (u64)
28..32 payload_len (u32)
32..   payload
~~~

tag 的固定含义为：1=section upsert、2=column upsert、3=section tombstone、4=column tombstone、5=world reset。batch 总长度最多 4 MiB，记录总数最多 4096；超过任一上限返回 client INVALID_ARGUMENT。

section upsert payload 是 8 字节 meta 后接 palette 与 packed words：

~~~text
0..2   single block id (u16)
2..4   palette_count (u16)
4..6   packed_word_count (u16)
6..8   reserved = 0 (u16)
8..    palette_count × u16 LE
...    packed_word_count × u64 LE
~~~

storage_kind 与 world.ContainerSnapshot 一一对应：0=single、1=indexed、2=direct。single 要求 bits/palette/packed 均为零；indexed 只允许 bits 4 或 8，并校验所有 packed slot 小于 palette_count；direct 要求 bits 15、palette_count 为零、packed_word_count 为 1024，且每个 word 的高四位为零。column upsert 的 payload 恰为 256 个 i16 LE height；其 section_y、storage_kind、bits 均为零。tombstone 的 payload 长度为零。world reset 的 record 仅允许作为 batch 第一条，所有坐标、revision、storage 元数据和 payload 均为零。

首次写入和重连恢复必须先发送 world reset，epoch 从 1 开始单调递增。非 reset record 的 epoch 必须等于当前 RenderWorld epoch；同一 key 只有 revision 更大才替换，等于或更小均为幂等忽略。tombstone 保留 revision，因此只有 revision 更大的 upsert 可以恢复该 key。

### Task 1: 建立可独立评审的 OpenSpec change 与基线 ledger

**Files:**

- Create: openspec/changes/rust-render-world-cache/proposal.md
- Create: openspec/changes/rust-render-world-cache/design.md
- Create: openspec/changes/rust-render-world-cache/tasks.md
- Create: openspec/changes/rust-render-world-cache/ledger.md
- Create: openspec/changes/rust-render-world-cache/.openspec.yaml
- Create: openspec/changes/rust-render-world-cache/specs/rust-client-render-cutover/spec.md

**Interfaces:**

- Consumes: docs/superpowers/specs/2026-08-29-rust-client-render-world-design.md 和本计划的 MRW1 固定布局。
- Produces: 唯一的 change 名 rust-render-world-cache；后续所有实现任务只以其 proposal、delta spec、design、tasks 与 ledger 为需求来源。

- [ ] **Step 1: 写入 proposal 和 delta spec 的失败约束**

在 proposal 中写明 Go CPU mesh/visibility 仍保持现状，且本 change 只新增 RenderWorld 缓存和 client ABI v12。delta spec 必须向 rust-client-render-cutover 添加下列可判定 Requirement 与 Given/When/Then Scenario：

~~~markdown
### Requirement: RenderWorld 只接受已验证的原子更新批

客户端 SHALL 只接受 MRW1 v1 的 render update batch。任一 record 的长度、
保留字节、palette、bitpack、epoch 或 revision 违反契约时，客户端 MUST 拒绝整个
batch 且 MUST 保持 RenderWorld 调用前状态。

#### Scenario: 非法 indexed section 不产生部分缓存

- GIVEN 已含 revision 7 section 的 RenderWorld
- WHEN 收到同一 batch 中先有合法 revision 8、后有 palette slot 越界的 section record
- THEN 调用返回 INVALID_ARGUMENT
- AND 原 revision 7 数据仍是唯一可见缓存状态
~~~

同时增加 “v12 混装早期拒绝” 和 “本 change 不改变 draw/frame 可观察结果” 两条 Requirement，各自包含 ABI 版本错误和离屏 frame 字节不变的 Scenario。

- [ ] **Step 2: 写入 design、tasks 与 ledger**

design 必须逐字列出 MRW1 的 24/32 字节布局、4 MiB/4096 限制、world reset 首记录规则、ContainerSnapshot 三态映射、RenderWorld 的派生所有权、无 worker/GPU 的非目标、流体源码零触碰和 engine ABI v8 不变理由。tasks 必须按本计划 Task 2–5 拆分。ledger 首项记录执行基线 SHA、Rust/Go toolchain 版本、下列命令与输出摘要：

~~~bash
make rust
cd engine && cargo test -p mornlea_client --locked
go test ./internal/client -race -count=1
go test ./internal/archcheck -count=1
~~~

- [ ] **Step 3: 验证 change 产物**

Run:

~~~bash
openspec validate --all --strict --no-interactive
git diff --check -- openspec/changes/rust-render-world-cache
~~~

Expected: OpenSpec strict 通过，且 diff check 无输出。

- [ ] **Step 4: 提交 change 产物**

~~~bash
git add openspec/changes/rust-render-world-cache
git commit -m "docs(openspec): propose rust render world cache"
~~~

### Task 2: 实现纯 Rust RenderWorld 与 MRW1 原子状态机

**Files:**

- Create: engine/crates/mornlea_client/src/render/world.rs
- Create: engine/crates/mornlea_client/src/render/world_tests.rs
- Modify: engine/crates/mornlea_client/src/render/mod.rs
- Test: engine/crates/mornlea_client/src/render/world_tests.rs

**Interfaces:**

- Consumes: 本计划固定的 MRW1 v1 layout；不依赖 GPU、window、network 或 Go。
- Produces: OffscreenRenderer 可调用的 RenderWorld::apply_update_batch(&[u8]) -> Result<(), RenderWorldError>；后续 change 可读取其规范化 section/column 值。

- [ ] **Step 1: 写入会失败的状态机测试**

在 world_tests.rs 先覆盖单值、4-bit indexed、8-bit indexed、15-bit direct 的 section 正常解码，并覆盖以下失败矩阵：

~~~rust
#[test]
fn invalid_second_record_keeps_the_first_record_unapplied() {
    let mut world = RenderWorld::default();
    world.apply_update_batch(&reset_then_section(1, 7, single(3))).unwrap();
    let before = world.snapshot_for_test();

    let invalid = batch(1, [
        section(8, single(4)),
        section_with_palette_slot_out_of_range(9),
    ]);
    assert_eq!(world.apply_update_batch(&invalid), Err(RenderWorldError::Invalid));
    assert_eq!(world.snapshot_for_test(), before);
}

#[test]
fn older_tombstoned_update_cannot_revive_a_section() {
    let mut world = seeded_world();
    world.apply_update_batch(&section_tombstone(1, 11)).unwrap();
    world.apply_update_batch(&section(1, 10, single(2))).unwrap();
    assert!(world.section_for_test(KEY).is_none());
}
~~~

再添加首次 reset 非 epoch 1、非递增 reset、reset 非首条、epoch 不匹配、equal revision 幂等、较大 revision 替换、column payload 非 512 字节、direct 高四位非零、reserved 非零、record_count 超限和 4 MiB 超限测试。

- [ ] **Step 2: 运行测试确认缺少类型和入口**

Run:

~~~bash
cd engine && cargo test -p mornlea_client --locked render::world_tests
~~~

Expected: FAIL，错误指出 RenderWorld、RenderWorldError 或 MRW1 helper 尚未定义。

- [ ] **Step 3: 实现全量验证后一次提交的 RenderWorld**

在 render/world.rs 定义以下内部接口：

~~~rust
pub(super) struct RenderWorld {
    epoch: Option<u64>,
    sections: std::collections::BTreeMap<SectionKey, SectionEntry>,
    columns: std::collections::BTreeMap<ColumnKey, ColumnEntry>,
}

pub(super) enum RenderWorldError {
    Invalid,
}

impl RenderWorld {
    pub(super) fn apply_update_batch(
        &mut self,
        bytes: &[u8],
    ) -> Result<(), RenderWorldError>;
}
~~~

解析器先把完整 batch 解为拥有所有 bytes 的 ParsedRecord 值并完成所有结构、容量、payload 和 snapshot 校验；只有完整 parse 成功后才按 record 顺序修改 map。SectionEntry 必须保留 revision 和 `Live(SectionData)`/`Tombstone` 状态；`SectionData` 保持紧凑的 single block、indexed palette+packed words 或 direct 1,024 packed words，不能展开为 4,096 个 block ID。ColumnEntry 存 [i16; 256] 与 revision/tombstone 状态；tombstone 不从 map 删除。首次 reset 只接受 epoch 1，后续 reset 必须严格大于当前 epoch；world reset 才能清空两个 map 并替换 epoch，非 reset record 不得改变 epoch。render/mod.rs 以 `mod world;` 和仅测试时的 `mod world_tests;` 挂载状态机，为 OffscreenRenderer 增加 render_world: RenderWorld 字段，在 build_renderer 中初始化，并添加只调用状态机的内部方法。

- [ ] **Step 4: 运行状态机测试确认通过**

Run:

~~~bash
cd engine && cargo test -p mornlea_client --locked render::world_tests
cd engine && cargo fmt --check
~~~

Expected: PASS，格式检查无输出。

- [ ] **Step 5: 提交 RenderWorld 状态机**

~~~bash
git add engine/crates/mornlea_client/src/render
git commit -m "feat(client): add render world cache"
~~~

### Task 3: 实现 Go MRW1 编码器和完整 chunk 更新构造

**Files:**

- Create: internal/client/render_world_update.go
- Create: internal/client/render_world_update_test.go
- Test: internal/client/render_world_update_test.go

**Interfaces:**

- Consumes: core.SectionKey、world.ContainerSnapshot、world.Chunk.Section、world.Chunk.Heights 和 Task 2 的 MRW1 v1 layout。
- Produces: 不依赖 cgo 的 RenderWorldBatch、RenderWorldUpdate、EncodeRenderWorldBatch 和 BuildRenderWorldChunkBatch；Task 4 直接把编码字节交给 Renderer。

- [ ] **Step 1: 写入 Go 编码失败测试**

定义测试先使用 world.NewChunk、Section(i).Blocks.Set 和 Compact 构造 single/indexed/direct 三态，并断言 batch header、每个 record header、u64 小端 packed words、24 条 section 加 1 条 column 的完整 chunk 计数：

~~~go
func TestBuildRenderWorldChunkBatchEncodesAllSectionsAndHeightMap(t *testing.T) {
    chunk := world.NewChunk(core.ChunkPos{X: -2, Z: 3})
    chunk.SetBlock(1, core.MinY, 2, core.StoneID)
    batch, err := BuildRenderWorldChunkBatch(1, core.Overworld, 17, chunk)
    if err != nil {
        t.Fatal(err)
    }
    if got, want := len(batch.Updates), core.SectionsPerChunk+1; got != want {
        t.Fatalf("更新数=%d, want %d", got, want)
    }
    encoded, err := EncodeRenderWorldBatch(batch)
    if err != nil {
        t.Fatal(err)
    }
    if got := string(encoded[:4]); got != "MRW1" {
        t.Fatalf("magic=%q", got)
    }
}
~~~

还要写 zero epoch、reset 非首条、column Y 非零、direct packed 高位、indexed slot 越界与超过 4096 record 的编码拒绝测试。

- [ ] **Step 2: 运行测试确认 API 尚未定义**

Run:

~~~bash
go test ./internal/client -run 'Test(BuildRenderWorld|EncodeRenderWorld)' -count=1
~~~

Expected: FAIL，错误指出 RenderWorldBatch、BuildRenderWorldChunkBatch 或 EncodeRenderWorldBatch 未定义。

- [ ] **Step 3: 实现值对象、验证和编码**

在 render_world_update.go 定义：

~~~go
type RenderWorldUpdateKind uint8

const (
    RenderWorldSectionUpsert RenderWorldUpdateKind = 1
    RenderWorldColumnUpsert  RenderWorldUpdateKind = 2
    RenderWorldSectionTombstone RenderWorldUpdateKind = 3
    RenderWorldColumnTombstone  RenderWorldUpdateKind = 4
    RenderWorldReset            RenderWorldUpdateKind = 5
)

type RenderWorldUpdate struct {
    Kind     RenderWorldUpdateKind
    Key      core.SectionKey
    Revision uint64
    Snapshot world.ContainerSnapshot
    Heights  world.HeightMap
}

type RenderWorldBatch struct {
    Epoch   uint64
    Updates []RenderWorldUpdate
}

func EncodeRenderWorldBatch(batch RenderWorldBatch) ([]byte, error)
func BuildRenderWorldChunkBatch(
    epoch uint64,
    dimension core.DimensionID,
    revision uint64,
    chunk *world.Chunk,
) (RenderWorldBatch, error)
~~~

BuildRenderWorldChunkBatch 必须拒绝 nil chunk，为 section index 0..core.SectionsPerChunk-1 依次调用 Blocks.Snapshot，并追加一个 column upsert；它不发送 reset。EncodeRenderWorldBatch 复用本计划 Wire Contract 的 MRW1 constants，先验证整个 Go 值对象再 append bytes，避免返回部分结果。任何调用方提供的非法 snapshot 返回 error；不 panic、不展开 4096 格、也不引入 network 包依赖。

- [ ] **Step 4: 运行 Go 编码测试**

Run:

~~~bash
gofmt -w internal/client/render_world_update.go internal/client/render_world_update_test.go
go test ./internal/client -run 'Test(BuildRenderWorld|EncodeRenderWorld)' -count=1
go test ./internal/world -run 'Test.*Snapshot' -count=1
~~~

Expected: PASS。

- [ ] **Step 5: 提交 Go 编码器**

~~~bash
git add internal/client/render_world_update.go internal/client/render_world_update_test.go
git commit -m "feat(client): encode render world updates"
~~~

### Task 4: 升级 client ABI 到 v12 并接通 Go→Rust 缓存入口

**Files:**

- Modify: engine/include/mornlea_client.h
- Modify: engine/crates/mornlea_client/src/lib.rs
- Modify: engine/crates/mornlea_client/src/ffi.rs
- Modify: engine/crates/mornlea_client/src/render/mod.rs
- Modify: internal/client/render.go
- Modify: internal/client/render_test.go
- Test: engine/crates/mornlea_client/src/ffi.rs
- Test: internal/client/render_test.go

**Interfaces:**

- Consumes: Task 2 的 OffscreenRenderer::apply_render_world_updates 与 Task 3 的 EncodeRenderWorldBatch 输出。
- Produces: client ABI v12 的 mornlea_client_render_apply_world_updates 及 Go Renderer.ApplyRenderWorldUpdates；生产 app 尚不调用它，直到后续 mesh-pipeline change 接管对应 Go 工作。

- [ ] **Step 1: 写入 ABI 与 bridge 的失败测试**

在 Rust ffi 测试中先新增：

~~~rust
#[test]
fn apply_world_updates_rejects_wrong_abi_and_invalid_bytes() {
    assert_eq!(
        unsafe {
            mornlea_client_render_apply_world_updates(
                CLIENT_ABI_VERSION + 1,
                1,
                std::ptr::null(),
                0,
            )
        },
        MORNLEA_CLIENT_STATUS_ABI_VERSION,
    );
}
~~~

在 Go render_test.go 新增离屏测试：创建 renderer（无适配器时 Skip），先通过 ApplyRenderWorldUpdates 发送 reset + single section batch，再渲染现有 identity frame，断言不 panic、FrameCalls 仍只由 RenderFrame 增加。测试不修改 Visible、UploadSection、frame bytes 或截图预期。

- [ ] **Step 2: 运行测试确认新 C 符号与 Go 方法不存在**

Run:

~~~bash
cd engine && cargo test -p mornlea_client --locked apply_world_updates_rejects_wrong_abi_and_invalid_bytes
go test ./internal/client -run TestRendererApplyRenderWorldUpdates -count=1
~~~

Expected: FAIL，错误指出 C ABI 导出或 Renderer.ApplyRenderWorldUpdates 缺失。

- [ ] **Step 3: 实现 v12 ABI 与 bridge**

将 header 与 Rust CLIENT_ABI_VERSION 同步为 12，并在 header 添加：

~~~c
uint32_t mornlea_client_render_apply_world_updates(
    uint32_t abi_version,
    uint64_t handle,
    const uint8_t *updates,
    size_t updates_len);
~~~

Rust 入口依次检查 ABI、非零长度、非空 pointer、地址范围和已存在 renderer handle；在 catch 包裹中从 updates 创建只读 slice 并调用 renderer.apply_render_world_updates。MRW1 错误映射 MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT，handle 错误映射 WINDOW，panic 映射 PANIC。该入口没有输出缓冲，成功前不允许修改 RenderWorld。

在 Go render.go 的 cgo 序言加入 noescape/nocallback 指令，并实现：

~~~go
func (r *Renderer) ApplyRenderWorldUpdates(encoded []byte) {
    if len(encoded) == 0 {
        panic("client: render world update为空")
    }
    r.check("apply render world updates", uint32(
        C.mornlea_client_render_apply_world_updates(
            C.MORNLEA_CLIENT_ABI_VERSION,
            C.uint64_t(r.handle),
            (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(encoded))),
            C.size_t(len(encoded)),
        ),
    ))
}
~~~

保留所有既有 ABI v11 入口的参数和帧布局，只让它们共同接受 v12 常量；修改 client lib.rs 与 header 顶部版本说明，明确 v12 新增此入口。

- [ ] **Step 4: 构建并运行跨语言 ABI 回归**

Run:

~~~bash
make rust
cd engine && cargo test -p mornlea_client --locked
go test ./internal/client -run 'Test(RendererApplyRenderWorldUpdates|RendererRoundtripOrSkip|EncodeRenderFrameLayout)' -count=1
go test ./internal/client -race -count=1
~~~

Expected: PASS；无 GPU 适配器的离屏测试按既有 ErrNoGPUAdapter 约定 Skip。

- [ ] **Step 5: 提交 ABI v12**

~~~bash
git add engine/include/mornlea_client.h engine/crates/mornlea_client internal/client/render.go internal/client/render_test.go
git commit -m "feat(client): add render world update ABI"
~~~

### Task 5: 同步版本事实、完成 OpenSpec 验收并记录证据

**Files:**

- Modify: AGENTS.md
- Modify: README.md
- Modify: README.en.md
- Modify: docs/architecture.md
- Modify: docs/notes/lan-server.md
- Modify: docs/notes/progress.md
- Modify: openspec/changes/rust-render-world-cache/{proposal,design,tasks,ledger}.md
- Modify: openspec/changes/rust-render-world-cache/specs/rust-client-render-cutover/spec.md
- Modify: openspec/specs/rust-client-render-cutover/spec.md
- Modify: openspec/specs/tiered-swords-combat/spec.md
- Test: internal/archcheck/baseline_test.go

**Interfaces:**

- Consumes: v12 header、Task 1 change 产物、Task 2–4 的验证输出。
- Produces: 与代码一致的当前版本事实、完整 ledger 和可归档的 OpenSpec change。

- [ ] **Step 1: 更新当前事实与 delta spec**

将根 AGENTS.md、README.md、README.en.md、docs/architecture.md、docs/notes/lan-server.md 与 openspec/specs/tiered-swords-combat/spec.md 中的 client ABI 当前值从 v11 改为 v12，并在 architecture 文本中说明：RenderWorld 已接收派生缓存更新，但 Go 仍拥有生产 mesh 调度、可见性和上传直到后续 change。docs/notes/progress.md 追加一段只陈述已实现的 v12 cache/ABI 事实，不把后续 mesh/visibility 工作写成已完成。`openspec/config.yaml` 的旧版本矩阵是既有、跨多个无关版本的陈旧内容，本 change 不顺带重写；ledger 必须把它记录为排除项。

在 change delta spec 的 v12 Requirement 中锁定所有 header/Rust/Go 版本常数一致、旧动态库早期拒绝、MRW1 无效 batch 不改缓存、既有 RenderFrame 字节布局和截图行为不变。归档前按 OpenSpec sync 流程把已验证的 delta 合入 openspec/specs/rust-client-render-cutover/spec.md，而非手工复制未验证草案。tasks.md 勾选的项目必须与已执行的 Task 1–4 一一对应，ledger 对每条命令记录 commit SHA、结果、Skip 原因和性能基线。

- [ ] **Step 2: 写入版本与可观察行为回归测试**

在 delta spec 中加入下列 Scenario，并在 ledger 关联其实际命令：

~~~markdown
#### Scenario: v11 动态库不能与 v12 Go bridge 混用

- GIVEN client ABI v11 的动态库
- WHEN v12 Go bridge 创建 renderer 或提交 render update
- THEN 调用 MUST 在任何 RenderWorld 状态改变前返回 ABI_VERSION

#### Scenario: 新缓存入口不改变现有 frame 编码

- GIVEN 相同 RenderFrame 输入和空 RenderWorld
- WHEN 分别在应用一个合法但未接管绘制的 MRW1 batch 前后编码 frame
- THEN EncodeRenderFrame 输出 MUST 逐字节一致
~~~

- [ ] **Step 3: 运行阶段边界验证**

Run:

~~~bash
make rust
make rust-check
gofmt -w internal/client/render_world_update.go internal/client/render_world_update_test.go internal/client/render.go internal/client/render_test.go
go vet ./...
go test ./internal/client -race -count=1
go test ./internal/mesh -race -count=1
go test ./internal/archcheck -count=1
make test-race-changed
go test ./... -race
make visual-check
openspec validate --all --strict --no-interactive
git diff --check
~~~

Expected: 所有正确性命令 PASS；性能命令只记录数值。若 visual-check 因无 GPU 环境无法运行，ledger 必须记录实际平台、失败类别和 CI/capture 所需的复验命令，不得更新 golden。

- [ ] **Step 4: 完成 change ledger 与提交**

ledger 最终条目必须列出 MRW1 的 4 MiB/4096 上限、v12/v8 版本结论、未修改协议/schema/benchmark/fluid 的证据、每项评审裁决及其对应 commit。提交只包含该 change 的文档同步：

~~~bash
git add AGENTS.md README.md README.en.md docs/architecture.md docs/notes/lan-server.md docs/notes/progress.md openspec/changes/rust-render-world-cache openspec/specs/rust-client-render-cutover/spec.md openspec/specs/tiered-swords-combat/spec.md
git commit -m "docs: record render world cache migration"
~~~

## Plan Self-Review

- Spec coverage: Task 1 固定 OpenSpec 契约与基线；Task 2 实现 MRW1 原子缓存状态机；Task 3 生成 Go 语义化更新；Task 4 升 v12 并跨 FFI 接通；Task 5 同步版本、视觉和 OpenSpec 验收。共享 kernel 与 Rust connectivity 被明确放到流体负责人完成后的下一 change；cache ABI 在取代对应 Go 工作前不接入实时 app，避免新增生产复制。流体排除、无生产 fallback、无网络 wire 解析、容量限制、并发/所有权和暂不切换 draw 的要求均已落入对应任务。
- Completeness scan: 本计划不含占位说明或缺失接口；每个任务均列出文件、接口、失败测试、验证与提交。
- Type consistency: MRW1、RenderWorld、RenderWorldBatch、RenderWorldUpdate、EncodeRenderWorldBatch、BuildRenderWorldChunkBatch、ApplyRenderWorldUpdates 与 C ABI 名称在所有任务中一致；MRC1 不作为 render update magic 出现。

## Execution Handoff

计划已拆为独立的 OpenSpec tasks。项目规则要求 complex change 使用 subagent-driven-development：每项由 fresh implementer 执行，并进行规格合规与质量双评审；控制会话不得直接实现。执行前必须在隔离 worktree 中重新读取 change 的 proposal、delta specs、design、tasks 与 ledger。
