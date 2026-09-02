#ifndef MORNLEA_CLIENT_H
#define MORNLEA_CLIENT_H

#include <stddef.h>
#include <stdint.h>

/* v14:在 v13 窗口合成捕获表面上新增 render world update 入口。
 * v13:新增窗口合成捕获出口 mornlea_client_window_capture(NSWindow
 * windowNumber → CGWindowListCreateImage → CGBitmapContext,输出紧凑
 * BGRA8 原始字节,两段式容量协议;新增溢出与捕获不可用两个状态码)。
 * v12:菜单层迁进程内 WKWebView——退役 render_upload_ui_font 出口与帧
 * TLV tag 9 UI 段(layout v1–v4 编解码随之作废,下发即 INVALID_ARGUMENT);
 * 新增 ui_push_state 状态下行出口(窗口句柄域,JSON 字符串);
 * render_drain_ui_events 签名不变、字节格式改为版本化 JSON 事件信封
 * (空队列写 0 字节)。v11:新增离屏 benchmark batch prepare/submit 入口；
 * v10:avatar 通道容量扩至 75 具身体(450 个 80-byte instance)并新增敌怪
 * EntityHostile 身份域;v9:新增设置页 layout v2，并把 render_drain_ui_events 升级为结构化事件
 * batch 与整批容量门禁；v8:新增 egui 主菜单两出口
 * render_upload_ui_font / render_drain_ui_events
 * 与帧 TLV tag 9(egui 菜单段);v7:终审修复波新增雾参数化
 * render_set_lod_fog 出口(新增导出面即 bump,同版本 = 同表面的不可混装
 * 契约);v6:新增远环 LOD tile 出口(render_upload_lod_tile/drop_lod_tile)。
 * 变基重编:远环两项出口在旧基线上原编号 v5/v6,main 的 water pass
 * (按 material 分流 + 半透明 water pass)占用 v5 后整体顺延一格。 */
#define MORNLEA_CLIENT_ABI_VERSION 14u

#define MORNLEA_CLIENT_STATUS_OK 0u
#define MORNLEA_CLIENT_STATUS_ABI_VERSION 1u
#define MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT 2u
#define MORNLEA_CLIENT_STATUS_WINDOW 3u
#define MORNLEA_CLIENT_STATUS_PANIC 4u
#define MORNLEA_CLIENT_STATUS_ADAPTER 5u
#define MORNLEA_CLIENT_STATUS_CAPACITY 6u
#define MORNLEA_CLIENT_STATUS_SKIPPED 7u
/* v13:窗口合成捕获状态。CAPTURE_OVERFLOW = 输出缓冲不足(所需字节数已
 * 回填 *out_required,两段式协议按其重试);CAPTURE_UNAVAILABLE = 捕获
 * 不可用(窗口号缺失、屏幕录制授权未授予、系统返回空图等运行期预期
 * 条件),调用方映射为可观察失败而非契约违约。 */
#define MORNLEA_CLIENT_STATUS_CAPTURE_OVERFLOW 8u
#define MORNLEA_CLIENT_STATUS_CAPTURE_UNAVAILABLE 9u

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

/* 更新尚未接管绘制的 MRW1 派生缓存(client ABI v14)。updates_len 必须在
 * 1..=4 MiB，updates 非空且地址范围不得溢出；通过这些表示层检查后，
 * 调用方保证 updates_len 字节可读。Rust 只在本次同步调用内借用输入，
 * 不保存 updates pointer。 */
uint32_t mornlea_client_render_apply_world_updates(
    uint32_t abi_version,
    uint64_t handle,
    const uint8_t *updates,
    size_t updates_len);

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

/* 菜单状态下行(client ABI v12 引入、v14 保留):把 Go 组装的 UI 状态 JSON 推给挂在该
 * 窗口上的 WebView(经 evaluateJavaScript 调 window.mornlea.onState)。
 * json 非空且 json_len>0,内容须为可解析 JSON 对象且含 phase 字符串字段;
 * 违约返回 INVALID_ARGUMENT。首次调用惰性挂载 WebView;同状态重复推送
 * 幂等(不产生新的 JS 求值)。 */
uint32_t mornlea_client_ui_push_state(
    uint32_t abi_version,
    uint64_t handle,
    const uint8_t *json,
    size_t json_len);

/* 窗口合成捕获(client ABI v13):抓取窗口完整合成画面(世界 + wgpu HUD +
 * WebView 菜单层),输出自上而下、无行 padding 的 BGRA8 原始字节,长度恰为
 * width×height×4。两段式容量协议:out_capacity 不足返回 CAPTURE_OVERFLOW,
 * 所需字节数回填 out_required、尺寸回填 out_width 与 out_height,输出缓冲
 * 保持调用前内容;成功时三个出参同样回填。捕获不可用(窗口号缺失、屏幕
 * 录制授权未授予、系统返回空图等运行期预期条件)返回 CAPTURE_UNAVAILABLE,
 * 出参与输出缓冲均原样。out_pixels 仅在 out_capacity 非 0 时须非空(零容量
 * 查询传 NULL,此时捕获仍真实执行)。必须在窗口 poll 线程调用(窗口句柄
 * 表是 thread-local 的)。 */
uint32_t mornlea_client_window_capture(
    uint32_t abi_version,
    uint64_t handle,
    uint8_t *out_pixels,
    uint64_t out_capacity,
    uint64_t *out_required,
    uint32_t *out_width,
    uint32_t *out_height);

/* 排空版本化 JSON UI 事件信封(client ABI v12 引入、v14 保留):只有完整信封可装入 out 时
 * 才写入、排空并把字节数写进 *out_written；容量不足返回 CAPACITY，三个对象
 * 均不变。空队列写 0 字节;信封形如 {"v":1,"events":[...]},事件按页面产生
 * 顺序出现,深层校验由调用方(Go)执行。 */
uint32_t mornlea_client_render_drain_ui_events(
    uint32_t abi_version,
    uint64_t handle,
    uint8_t *out,
    size_t out_len,
    size_t *out_written);

uint32_t mornlea_client_render_frame(
    uint32_t abi_version,
    uint64_t handle,
    const uint8_t *frame,
    size_t frame_len);

/* 离屏 benchmark 批次(client ABI v11):prepare 校验 frame 与 `1..=256` 次数后
 * 录制一个未提交的 command buffer；同一 renderer 已有 batch 时返回
 * INVALID_ARGUMENT 且保留原 batch。submit 消费该 buffer，一次提交并等待 GPU
 * 完成；没有 prepared batch 时返回 INVALID_ARGUMENT。 */
uint32_t mornlea_client_render_prepare_benchmark_batch(
    uint32_t abi_version,
    uint64_t handle,
    const uint8_t *frame,
    size_t frame_len,
    uint32_t repeat);

uint32_t mornlea_client_render_submit_benchmark_batch(
    uint32_t abi_version,
    uint64_t handle);

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
