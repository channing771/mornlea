#ifndef MORNLEA_CLIENT_H
#define MORNLEA_CLIENT_H

#include <stddef.h>
#include <stdint.h>

/* v8:新增 egui 主菜单两出口 render_upload_ui_font / render_drain_ui_events
 * 与帧 TLV tag 9(egui 菜单段);v7:终审修复波新增雾参数化
 * render_set_lod_fog 出口(新增导出面即 bump,同版本 = 同表面的不可混装
 * 契约);v6:新增远环 LOD tile 出口(render_upload_lod_tile/drop_lod_tile)。
 * 变基重编:远环两项出口在旧基线上原编号 v5/v6,main 的 water pass
 * (按 material 分流 + 半透明 water pass)占用 v5 后整体顺延一格。 */
#define MORNLEA_CLIENT_ABI_VERSION 8u

#define MORNLEA_CLIENT_STATUS_OK 0u
#define MORNLEA_CLIENT_STATUS_ABI_VERSION 1u
#define MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT 2u
#define MORNLEA_CLIENT_STATUS_WINDOW 3u
#define MORNLEA_CLIENT_STATUS_PANIC 4u
#define MORNLEA_CLIENT_STATUS_ADAPTER 5u
#define MORNLEA_CLIENT_STATUS_CAPACITY 6u
#define MORNLEA_CLIENT_STATUS_SKIPPED 7u

/* 输入快照:64 字节头 + 1024 x u32 文本段,布局见 crate input 模块文档。 */
#define MORNLEA_CLIENT_SNAPSHOT_BYTES 4160u

uint32_t mornlea_client_abi_version(void);

uint32_t mornlea_client_window_create(
    uint32_t abi_version,
    uint32_t width,
    uint32_t height,
    const uint8_t *title,
    size_t title_len,
    uint64_t *out_handle);

uint32_t mornlea_client_window_destroy(uint32_t abi_version, uint64_t handle);

uint32_t mornlea_client_window_poll(
    uint32_t abi_version,
    uint64_t handle,
    uint8_t *out,
    size_t out_len);

uint32_t mornlea_client_window_set_cursor_captured(
    uint32_t abi_version,
    uint64_t handle,
    uint8_t captured);

uint32_t mornlea_client_window_set_content_size(
    uint32_t abi_version,
    uint64_t handle,
    uint32_t width,
    uint32_t height);

uint32_t mornlea_client_window_set_floating(
    uint32_t abi_version,
    uint64_t handle,
    uint8_t floating);

uint32_t mornlea_client_window_focus(uint32_t abi_version, uint64_t handle);

uint32_t mornlea_client_window_cancel_close(uint32_t abi_version, uint64_t handle);

uint32_t mornlea_client_window_ns_window(
    uint32_t abi_version,
    uint64_t handle,
    uintptr_t *out_ns_window);

uint32_t mornlea_client_render_create(
    uint32_t abi_version,
    uint32_t width,
    uint32_t height,
    uint64_t *out_handle);

uint32_t mornlea_client_render_destroy(uint32_t abi_version, uint64_t handle);

uint32_t mornlea_client_render_upload_atlas(
    uint32_t abi_version,
    uint64_t handle,
    uint32_t layers,
    const uint8_t *pixels,
    size_t pixels_len);

uint32_t mornlea_client_render_upload_section(
    uint32_t abi_version,
    uint64_t handle,
    int32_t section_x,
    int32_t section_y,
    int32_t section_z,
    const uint8_t *quads,
    size_t quads_len,
    const uint8_t *water_quads,
    size_t water_quads_len);

uint32_t mornlea_client_render_drop_section(
    uint32_t abi_version,
    uint64_t handle,
    int32_t section_x,
    int32_t section_y,
    int32_t section_z);

/* 远环 LOD tile(client ABI v6):上传/替换一个 tile 的壳 quad 字节流。
 * 每 quad 20 字节 LE:x/z/y i32、w/d u16、face u8(顶面 + 四向侧裙共
 * 5 值)、material u16、shade u8,布局与 engine mornlea_lod_shell 输出
 * 逐字一致。整 tile 替换语义:重复上传同 tile 即整体替换;quads_len
 * 为 0 等价 drop。流非法返回 INVALID_ARGUMENT,tile 表容量耗尽返回
 * CAPACITY。tile 坐标为 chunk 坐标,每 tile 覆盖 4x4 chunk。 */
uint32_t mornlea_client_render_upload_lod_tile(
    uint32_t abi_version,
    uint64_t handle,
    int32_t tile_x,
    int32_t tile_z,
    const uint8_t *quads,
    size_t quads_len);

/* 丢弃一个远环 tile;不存在时为幂等空操作。 */
uint32_t mornlea_client_render_drop_lod_tile(
    uint32_t abi_version,
    uint64_t handle,
    int32_t tile_x,
    int32_t tile_z);

/* 设置远环距离雾参数(client ABI v7,Ruling 14 参数化):start 起雾
 * 距离、full 全雾距离(block)。入口校验 start > 0 且 full > start
 * (NaN 拒绝),违约返回 INVALID_ARGUMENT 且先于句柄查找;渲染器默认
 * 768/1152 锚定 lodFarMultiplier=3 的默认几何,非默认倍率由上层按
 * 配置推导后调用。 */
uint32_t mornlea_client_render_set_lod_fog(
    uint32_t abi_version,
    uint64_t handle,
    float start,
    float full);

/* 上传 egui 菜单字体(client ABI v8):bytes 非空、len>0 且 <= 32 MiB;
 * 超上限返回 CAPACITY,其余违约返回 INVALID_ARGUMENT(先于句柄查找)。 */
uint32_t mornlea_client_render_upload_ui_font(
    uint32_t abi_version,
    uint64_t handle,
    const uint8_t *bytes,
    size_t len);

/* 排空 egui 菜单点击事件(client ABI v8):把按钮 id(u32,小端)序列写进 out,
 * 并把写入个数写进 *out_count,返回状态码;out 为空、out_len 非 4 倍数或
 * out_count 为空返回 INVALID_ARGUMENT(均先于句柄查找),事件多于容量时写满
 * 截断。返回值是状态码,事件数一律经 out_count 回读,避免与状态码空间冲突。 */
uint32_t mornlea_client_render_drain_ui_events(
    uint32_t abi_version,
    uint64_t handle,
    uint8_t *out,
    size_t out_len,
    uint32_t *out_count);

uint32_t mornlea_client_render_frame(
    uint32_t abi_version,
    uint64_t handle,
    const uint8_t *frame,
    size_t frame_len);

uint32_t mornlea_client_render_upload_glyph_rect(
    uint32_t abi_version,
    uint64_t handle,
    uint32_t x,
    uint32_t y,
    uint32_t width,
    uint32_t height,
    const uint8_t *pixels,
    size_t pixels_len);

uint32_t mornlea_client_render_upload_hud_atlas(
    uint32_t abi_version,
    uint64_t handle,
    uint32_t width,
    uint32_t height,
    const uint8_t *pixels,
    size_t pixels_len);

uint32_t mornlea_client_render_create_windowed(
    uint32_t abi_version,
    uint64_t window_handle,
    uint64_t *out_handle);

uint32_t mornlea_client_render_resize(
    uint32_t abi_version,
    uint64_t handle,
    uint32_t width,
    uint32_t height);

uint32_t mornlea_client_render_readback(
    uint32_t abi_version,
    uint64_t handle,
    uint8_t *out,
    size_t out_len);

#endif
