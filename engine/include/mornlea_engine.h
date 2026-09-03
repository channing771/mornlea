#ifndef MORNLEA_ENGINE_H
#define MORNLEA_ENGINE_H

#include <stddef.h>
#include <stdint.h>

/* ABI v10:worldgen `MGW1` 请求材料表由 14 项扩为 15 项(末项 short_grass,
 * 位于偏移 52,perm 后移到偏移 54):带内 layout 2 → 3、公共 header 564 →
 * 566 字节、chunk 输入 572 → 574 字节、probe 输入 570 + 16×N(输出仍为每条
 * 8 字节)、LOD 壳输入 580 → 582 字节;自然短草在树与海水之后按确定性整数
 * 判定写入草地表面——natural-grass-seeds 变更。三个输出格式与长度契约均不
 * 因新材料改变。既有入口签名与语义不变。engine 与 Go 侧是同一不可跨版本
 * 混装的 release unit。
 * ABI v9:新增流体双内核 mornlea_fluid_eval_batch(批量单格流体规则求值,
 * 输入布局 v1 = 8 字节头 + 每项 14 字节 7×u16,输出每项定长 12 字节 4 条
 * 候选写入;输出尺寸是输入的确定函数,容量不足按参数违约拒绝,无两段式
 * 探测)与 mornlea_fluid_rescan(确定性流体重扫扫描:输入 MFL1 布局 v1,
 * 输出世界坐标流 + summary,两段式输出容量探测)——rust-engine-fluid
 * 变更。既有入口签名与语义不变。engine 与 Go 侧是同一不可跨版本混装的
 * release unit。
 * ABI v8:mesh `MGM1` 输入的单条 registry 条目由 19 字节扩到 20 字节,末尾追加
 * `model`(有限模型 tag 的封闭集合:0=默认、1..=5=火把五形态[1=落地、2..=5=墙面
 * +X/−X/+Z/−Z,与火把方块编号 71..75 同序]、6=床[床尾/床头 × 四向八形态共用
 * 半高板几何,朝向差异由逐形态床面材质层表达]、7 起未知拒绝),由
 * greedy 的 model dispatcher 消费:0 走既有几何,1..5 走火把、6 走 emit_bed 发射。条目上限
 * 64→80(v7 期内)与 80→96(床批次)均为同批同步的容量调整,不随升版记账
 * (条目上限不在版本契约内的同一先例)。既有入口签名与语义不变。
 * engine 与 Go 侧是同一不可跨版本混装的 release unit。
 * ABI v7:mesh `MGM1` 输入的单条 registry 条目由 18 字节扩到 19 字节,末尾追加
 * `block_top_raw`(该方块的 4-bit 顶面高度原值:0 为「满格」哨兵,1..=14 表示
 * 全部可见面的上缘按 (h_raw+1)/16 下沉,15 非法;与 fluid_height 互斥)。承载
 * 耕地(干/湿填 14,呈现高度 15/16 与碰撞体一致)等非满格方块的常量角高度
 * 几何。既有入口签名与语义不变。engine 与 Go 侧是同一不可跨版本混装的
 * release unit。
 * ABI v6:新增 `mornlea_lod_shell` 远环壳出口(rust-engine-lod-shell 变更),
 * 既有入口签名与语义不变。变基重编说明:该出口在旧基线上原编号 v4,main
 * 合并 fluid 系列已占用 v4(worldgen 注水)与 v5(mesh registry 扩容),故
 * 顺延重编为 v6;engine 与 Go 侧是同一不可跨版本混装的 release unit。
 * ABI v5:mesh `MGM1` 输入的 registry 条目上限由 27 扩到 35,流体的 8 个方块
 * 编号随之进入 registry 快照(见 input.rs 的 MAX_REGISTRY_ENTRIES);同一版本内
 * 单条 registry 条目由 16 字节扩到 18 字节,末尾追加 `fluid_height`(该格孤立时
 * 的 4-bit 流体高度原值,0 为「非流体」哨兵)与 `light_attenuation`(天空光穿过
 * 该方块的额外衰减)两个每方块字节。engine 与 Go 侧是同一不可跨版本混装的
 * release unit。
 * ABI v4:worldgen `MGW1` header 的材料表由 13 项扩到 14 项(末项 water,
 * 占用 v3 的 reserved 槽,header 总长仍为 564 字节)。 */
#define MORNLEA_ENGINE_ABI_VERSION 10u

#define MORNLEA_STATUS_OK 0u
#define MORNLEA_STATUS_ABI_VERSION 1u
#define MORNLEA_STATUS_INVALID_ARGUMENT 2u
#define MORNLEA_STATUS_INPUT 3u
#define MORNLEA_STATUS_SCRATCH 4u
#define MORNLEA_STATUS_REGISTRY 5u
#define MORNLEA_STATUS_EMISSION 6u
#define MORNLEA_STATUS_OUTPUT_OVERFLOW 7u
#define MORNLEA_STATUS_QUEUE_OVERFLOW 8u
#define MORNLEA_STATUS_PANIC 9u

uint32_t mornlea_engine_abi_version(void);

uint32_t mornlea_mesh_section(
    uint32_t abi_version,
    const uint8_t *input,
    size_t input_len,
    uint8_t *scratch,
    size_t scratch_len,
    uint64_t *output,
    size_t output_capacity,
    size_t *output_len);

uint32_t mornlea_collision_resolve(
    uint32_t abi_version,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_len);

uint32_t mornlea_raycast_batch(
    uint32_t abi_version,
    const uint8_t *input,
    size_t input_len,
    uint8_t *cursor,
    size_t cursor_len,
    uint8_t *output,
    size_t output_len,
    size_t *output_count,
    uint8_t *done);

uint32_t mornlea_physics_step(
    uint32_t abi_version,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_len);

uint32_t mornlea_worldgen_chunk(
    uint32_t abi_version,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_len);

uint32_t mornlea_worldgen_probe(
    uint32_t abi_version,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_len);

/*
 * mornlea_lod_shell:确定性远环 LOD 壳生成(无状态纯函数)。
 *
 * 输入 582 字节 = 与 mornlea_worldgen_chunk 完全一致的 MGW1 header(566)
 * + tile_x i32(566)+ tile_z i32(570)+ columns u32(574,必须等于 64)
 * + lod_step u32(578,合法值 2/4/8);tile 覆盖 [tile_x*64, tile_x*64+64)
 * × [tile_z*64, tile_z*64+64) 列。
 *
 * 输出为壳 quad 字节流(LE)。单 quad 20 字节:x i32(0)| z i32(4)|
 * y i32(8)| w u16(12)| d u16(14)| face u8(16,取值见 [`LodFace`])|
 * material u16(17)| shade u8(19)。face 共 5 值:顶面 top + 四向侧裙
 * side(0=top、1=neg_x、2=pos_x、3=neg_z、4=pos_z);着色权重:顶面 255、
 * ±Z 204、±X 153。位布局与 engine crate lod.rs 的 encode_shell 定稿一致。
 *
 * 容量语义(两段式探测):output_capacity 不足时返回
 * MORNLEA_STATUS_OUTPUT_OVERFLOW 并把所需字节数写入 *output_len(输出
 * 缓冲不写入任何字节);调用方扩容(≥所需)后重试即成功,成功时
 * *output_len 为实际写入字节数。其余状态语义与既有导出一致:abi_version
 * 不匹配返回 MORNLEA_STATUS_ABI_VERSION;指针、范围或重叠违约返回
 * MORNLEA_STATUS_INVALID_ARGUMENT;输入内容违约(长度、header、列数、
 * 步长、越界 tile)返回 MORNLEA_STATUS_INPUT;Rust panic 收敛为
 * MORNLEA_STATUS_PANIC。除两段式 overflow 报告所需容量外,失败路径
 * *output_len 恒为 0 且输出缓冲原样。
 */
uint32_t mornlea_lod_shell(
    uint32_t abi_version,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len);

/*
 * mornlea_fluid_eval_batch:流体单格规则批量求值(无状态纯函数)。
 *
 * 输入 = u32 layout_version(当前 1,LE)+ u32 item_count + 每项 14 字节
 * (7 个 u16 LE 方块编号,槽位序:0=自格、1=上、2=下、3=+x、4=−x、5=+z、
 * 6=−z,与 Go internal/fluid 的 sixNeighbors 同序)。方块编号是协议稳定值,
 * 与 Go internal/core/block.go 的 iota 逐一对应。
 *
 * 输出 = 每项 12 字节:4 条候选写入 ×(目标槽位 u8(0..6;0xFF=无写入)+
 * BlockID u16 LE)。同一项内至多 4 条(垂直优先 1 条或水平传播 4 条或
 * 自格消亡 1 条),多余槽位为无写入哨兵。
 *
 * input_len 必须等于 8 + item_count*14,output 容量不足返回
 * MORNLEA_STATUS_INVALID_ARGUMENT(输出尺寸是输入的确定函数,无需两段式
 * 探测);layout_version 或 item_count 违约返回 MORNLEA_STATUS_INPUT;
 * 其余状态语义与既有导出一致。
 */
uint32_t mornlea_fluid_eval_batch(
    uint32_t abi_version,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len);

/*
 * mornlea_fluid_rescan:确定性流体重扫扫描(无状态纯函数,两段式容量探测)。
 *
 * 输入为 MFL1 布局 v1(LE),四段:
 * 1. header 26 字节:u32 layout_version(当前 1)| i32 center_chunk_x |
 *    i32 center_chunk_z | u16 x0 | u16 x1 | u16 z0 | u16 z1(扫描区域的
 *    盒内局部列闭区间,域 0..17 含裙边;被扫描区块是盒中心区块)| u8
 *    start_section(0..23)| u8 reserved(必须 0)| u32 budget;
 * 2. 中心区块 24 区段记录(按 y 区段 0..23):u8 kind(0=均匀、1=密集)+
 *    u8 pad(必须 0);kind=0 追加 u16 uniform_id(记录共 4 字节),
 *    kind=1 追加 4096×u16(区段内序 x + z*16 + y16*256,与 Go
 *    internal/world 的 blockIndex 一致);
 * 3. 裙边 68 列 × 384 u16,列序固定:(x=-1,z=0..15)、(x=16,z=0..15)、
 *    (z=-1,x=0..15)、(z=16,x=0..15)、四角 (-1,-1)/(16,-1)/(-1,16)/(16,16);
 *    列内 y 0..383(盒内局部列:中心区块局部 (lx,lz) ∈ 0..15 映射盒
 *    (lx+1, lz+1);y 指标 0..383 对应世界 y_base + 0..383,y_base 由
 *    Go 编码方取 core.MinY = -64);
 * 4. 元数据 9 区块 × 24 区段 × 3B(u8 uniform_flag + u16 id;flag=0 时
 *    id 必须 0):区块序 中心、(-1,-1)、(0,-1)、(1,-1)、(-1,0)、(1,0)、
 *    (-1,1)、(0,1)、(1,1)。
 *
 * 扫描语义镜像 Go internal/sim/realm 的 enqueueChunkFluids:区段循环前查
 * 额度(单次调用至多超支一个区段);均匀非流体区段计 1;均匀水源区段且
 * 区段级不动点成立(下方区段 + 四个水平邻区段均匀且不可替换)计 1;
 * 其余区段逐格计 1,流体格产出坐标,水源格过五邻不动点(下方 + 四个
 * 水平邻格不可替换)不产出,越界 y 读 Barrier。
 *
 * 输出 = 流体格世界坐标流(每条 12 字节:u32 x、u32 y、u32 z;世界坐标
 * 可为负,按二进制补码编码,Go 侧以 int32 重读)+ 尾部 summary 8 字节
 * (u32 spent | u8 done | u8[3] pad)。done 表示扫描范围在预算内完成;
 * spent 是本次记账总数,续扫起点由调用方按确定性记账重放推出。
 *
 * 容量语义(两段式探测)同 mornlea_lod_shell:output_capacity 不足时返回
 * MORNLEA_STATUS_OUTPUT_OVERFLOW 并把所需字节数写入 *output_len(输出
 * 缓冲不写入任何字节);调用方扩容(≥所需)后重试即成功,成功时
 * *output_len 为实际写入字节数。layout_version、区段记录或元数据违约
 * 返回 MORNLEA_STATUS_INPUT;其余状态语义与既有导出一致;Rust panic
 * 收敛为 MORNLEA_STATUS_PANIC。除两段式 overflow 报告所需容量外,失败
 * 路径 *output_len 恒为 0 且输出缓冲原样。
 */
uint32_t mornlea_fluid_rescan(
    uint32_t abi_version,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len);

#endif
