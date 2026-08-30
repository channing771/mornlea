//! mornlea_client 的 C ABI 出口。
//!
//! 契约:
//! - 所有入口第一个参数是调用方期望的 ABI 版本,不匹配立即返回
//!   `MORNLEA_CLIENT_STATUS_ABI_VERSION`;当前版本见 [`CLIENT_ABI_VERSION`]
//!   (v6 起远环 tile 出口加入,v7 起雾 setter 出口加入,v9 起结构化 UI 事件,
//!   v11 起离屏 benchmark batch,v12 起 render world update)。
//! - 窗口句柄存放在 thread-local 表中:句柄只在创建线程有效,跨线程调用
//!   查不到句柄而返回 `MORNLEA_CLIENT_STATUS_WINDOW`——这同时兜住了 winit
//!   macOS 的主线程约束(Go 侧已 `LockOSThread`)。
//! - 任何校验失败都不写调用方缓冲;Rust panic 被 catch_unwind 拦截并转为
//!   `MORNLEA_CLIENT_STATUS_PANIC`,不跨 FFI 边界展开。

use std::cell::RefCell;
use std::collections::HashMap;

use crate::input::SNAPSHOT_BYTES;
use crate::window::ClientWindow;

/// 当前 client ABI 版本。
///
/// v9:设置页 layout v2 与结构化事件 batch 取代裸按钮 id，排空改为整批
/// 容量门禁；v8:新增 egui 主菜单两条出口 `render_upload_ui_font`/`render_drain_ui_events`
/// 与帧 TLV tag 9(egui 菜单段)——新增导出面即 bump,ABI 版本是
/// "同版本 = 同表面"的不可混装契约(与 engine v3→v4、client v4→5 同一先例)。
/// v7:终审修复波(Ruling 14/16)新增雾参数化 `render_set_lod_fog` 出口;
/// 既有入口签名不变。
/// v6:新增远环 `render_upload_lod_tile`/`render_drop_lod_tile` 出口。
/// 变基重编说明:远环两项出口在旧基线上原编号 v5/v6,main 合并 fluid 系列
/// 后 v5 已被 water pass(`mornlea_client_render_upload_section` 按 material
/// 分成不透明与水面两条流,新增半透明 water pass)占用,故整体顺延一格。
/// 必须与 `engine/include/mornlea_client.h` 的 `MORNLEA_CLIENT_ABI_VERSION`
/// 逐版本一致。
/// v12:新增 render world update 入口。
/// v11:新增离屏 benchmark batch prepare/submit 入口。
/// v10:avatar 通道容量扩至 75 具身体(450 实例)并新增敌怪身份域。
pub const CLIENT_ABI_VERSION: u32 = 12;

/// 调用成功。
pub const MORNLEA_CLIENT_STATUS_OK: u32 = 0;
/// ABI 版本不匹配。
pub const MORNLEA_CLIENT_STATUS_ABI_VERSION: u32 = 1;
/// 指针/长度/参数非法。
pub const MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT: u32 = 2;
/// 窗口句柄无效、已销毁、跨线程访问或窗口系统操作失败。
pub const MORNLEA_CLIENT_STATUS_WINDOW: u32 = 3;
/// Rust 侧 panic 被拦截。
pub const MORNLEA_CLIENT_STATUS_PANIC: u32 = 4;

thread_local! {
    /// 本线程的活动窗口表;key 是对外句柄。thread-local 使句柄天然绑定
    /// 创建线程,跨线程访问表现为"句柄不存在"。
    static WINDOWS: RefCell<HashMap<u64, ClientWindow>> = RefCell::new(HashMap::new());
    /// 句柄分配计数;0 保留为无效句柄。
    static NEXT_HANDLE: RefCell<u64> = const { RefCell::new(1) };
}

/// 把闭包包进 catch_unwind,panic 一律折叠为 PANIC 状态码。
fn catch(operation: impl FnOnce() -> u32) -> u32 {
    match std::panic::catch_unwind(std::panic::AssertUnwindSafe(operation)) {
        Ok(status) => status,
        Err(_) => MORNLEA_CLIENT_STATUS_PANIC,
    }
}

/// 对句柄指向的窗口执行操作;句柄缺失返回 WINDOW 状态。
fn with_window(handle: u64, operation: impl FnOnce(&mut ClientWindow) -> u32) -> u32 {
    WINDOWS.with(|windows| match windows.borrow_mut().get_mut(&handle) {
        Some(window) => operation(window),
        None => MORNLEA_CLIENT_STATUS_WINDOW,
    })
}

/// 返回当前 client ABI 版本。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_abi_version() -> u32 {
    CLIENT_ABI_VERSION
}

/// 创建窗口并写出句柄。
///
/// `title`/`title_len` 必须是合法 UTF-8;失败时 `out_handle` 不被修改。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_window_create(
    abi_version: u32,
    width: u32,
    height: u32,
    title: *const u8,
    title_len: usize,
    out_handle: *mut u64,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if title.is_null() || out_handle.is_null() || width == 0 || height == 0 {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        // SAFETY: title 非空,调用方保证 title_len 字节可读。
        let bytes = unsafe { std::slice::from_raw_parts(title, title_len) };
        let Ok(title) = std::str::from_utf8(bytes) else {
            return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
        };
        let Ok(window) = ClientWindow::create(width, height, title.to_owned()) else {
            return MORNLEA_CLIENT_STATUS_WINDOW;
        };
        let handle = NEXT_HANDLE.with(|next| {
            let mut next = next.borrow_mut();
            let handle = *next;
            *next += 1;
            handle
        });
        WINDOWS.with(|windows| windows.borrow_mut().insert(handle, window));
        // SAFETY: out_handle 已判非空,只在完整成功后写一次。
        unsafe { out_handle.write(handle) };
        MORNLEA_CLIENT_STATUS_OK
    })
}

/// 销毁窗口;重复销毁返回 WINDOW 状态。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_window_destroy(abi_version: u32, handle: u64) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    catch(|| {
        WINDOWS.with(|windows| match windows.borrow_mut().remove(&handle) {
            Some(window) => {
                drop(window);
                MORNLEA_CLIENT_STATUS_OK
            }
            None => MORNLEA_CLIENT_STATUS_WINDOW,
        })
    })
}

/// 每帧一次:泵事件并写出输入快照;`out_len` 必须恰为快照字节数,
/// 校验失败不触碰输出缓冲。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_window_poll(
    abi_version: u32,
    handle: u64,
    out: *mut u8,
    out_len: usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if out.is_null() || out_len != SNAPSHOT_BYTES {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_window(handle, |window| {
            // SAFETY: out 非空且长度已校验为 SNAPSHOT_BYTES,调用方保证可写。
            let out = unsafe { std::slice::from_raw_parts_mut(out, out_len) };
            window.poll(out);
            MORNLEA_CLIENT_STATUS_OK
        })
    })
}

/// 切换光标捕获;`captured` 只接受 0/1。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_window_set_cursor_captured(
    abi_version: u32,
    handle: u64,
    captured: u8,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if captured > 1 {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_window(handle, |window| {
            window.set_cursor_captured(captured == 1);
            MORNLEA_CLIENT_STATUS_OK
        })
    })
}

/// 请求修改 content 尺寸(逻辑点)。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_window_set_content_size(
    abi_version: u32,
    handle: u64,
    width: u32,
    height: u32,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if width == 0 || height == 0 {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_window(handle, |window| {
            window.set_content_size(width, height);
            MORNLEA_CLIENT_STATUS_OK
        })
    })
}

/// 设置窗口置顶;`floating` 只接受 0/1。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_window_set_floating(
    abi_version: u32,
    handle: u64,
    floating: u8,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if floating > 1 {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_window(handle, |window| {
            window.set_floating(floating == 1);
            MORNLEA_CLIENT_STATUS_OK
        })
    })
}

/// 请求聚焦窗口。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_window_focus(abi_version: u32, handle: u64) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    catch(|| {
        with_window(handle, |window| {
            window.focus();
            MORNLEA_CLIENT_STATUS_OK
        })
    })
}

/// 撤销关闭请求。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_window_cancel_close(abi_version: u32, handle: u64) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    catch(|| {
        with_window(handle, |window| {
            window.cancel_close();
            MORNLEA_CLIENT_STATUS_OK
        })
    })
}

/// 写出 NSWindow 指针供 gfx 创建 Metal surface;失败不触碰输出。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_window_ns_window(
    abi_version: u32,
    handle: u64,
    out_ns_window: *mut usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if out_ns_window.is_null() {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_window(handle, |window| match window.ns_window() {
            Some(pointer) => {
                // SAFETY: out_ns_window 已判非空,只在完整成功后写一次。
                unsafe { out_ns_window.write(pointer) };
                MORNLEA_CLIENT_STATUS_OK
            }
            None => MORNLEA_CLIENT_STATUS_WINDOW,
        })
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    // 真实窗口不进自动测试(仓库纪律);这里只验证不依赖窗口系统的
    // 校验拒绝路径:ABI 版本、参数校验与无效句柄。

    #[test]
    fn abi_version_is_twelve() {
        // v12 新增 render world update 入口；v11 新增离屏 benchmark batch
        // prepare/submit 入口；v10 扩大 avatar
        // 通道容量（75 具身体 / 450 实例）并新增敌怪 EntityHostile 身份域。
        assert_eq!(mornlea_client_abi_version(), 12);
    }

    #[test]
    fn wrong_abi_version_is_rejected_everywhere() {
        let mut out_handle = 0u64;
        // SAFETY: 指针来自有效局部变量。
        let create = unsafe {
            mornlea_client_window_create(
                CLIENT_ABI_VERSION + 1,
                100,
                100,
                b"t".as_ptr(),
                1,
                &mut out_handle,
            )
        };
        assert_eq!(create, MORNLEA_CLIENT_STATUS_ABI_VERSION);
        assert_eq!(out_handle, 0);
        assert_eq!(
            mornlea_client_window_destroy(CLIENT_ABI_VERSION + 1, 1),
            MORNLEA_CLIENT_STATUS_ABI_VERSION
        );
        let mut snapshot = [0u8; SNAPSHOT_BYTES];
        // SAFETY: 同上。
        let poll = unsafe {
            mornlea_client_window_poll(
                CLIENT_ABI_VERSION + 1,
                1,
                snapshot.as_mut_ptr(),
                snapshot.len(),
            )
        };
        assert_eq!(poll, MORNLEA_CLIENT_STATUS_ABI_VERSION);
    }

    #[test]
    fn invalid_arguments_are_rejected_without_writes() {
        let mut out_handle = 42u64;
        // SAFETY: 除被测的空 title 外其余指针有效。
        let create = unsafe {
            mornlea_client_window_create(
                CLIENT_ABI_VERSION,
                100,
                100,
                std::ptr::null(),
                0,
                &mut out_handle,
            )
        };
        assert_eq!(create, MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT);
        assert_eq!(out_handle, 42, "失败调用不得写 out_handle");

        let mut snapshot = [0xAAu8; SNAPSHOT_BYTES];
        // SAFETY: 长度刻意错一字节,入口必须在写入前拒绝。
        let poll = unsafe {
            mornlea_client_window_poll(
                CLIENT_ABI_VERSION,
                1,
                snapshot.as_mut_ptr(),
                snapshot.len() - 1,
            )
        };
        assert_eq!(poll, MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT);
        assert!(
            snapshot.iter().all(|&b| b == 0xAA),
            "失败调用不得写快照缓冲"
        );

        assert_eq!(
            mornlea_client_window_set_cursor_captured(CLIENT_ABI_VERSION, 1, 2),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            mornlea_client_window_set_content_size(CLIENT_ABI_VERSION, 1, 0, 100),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
    }

    #[test]
    fn unknown_handle_is_rejected_as_window_error() {
        let mut snapshot = [0u8; SNAPSHOT_BYTES];
        // SAFETY: 指针有效,句柄在本线程表中不存在。
        let poll = unsafe {
            mornlea_client_window_poll(
                CLIENT_ABI_VERSION,
                0xDEAD,
                snapshot.as_mut_ptr(),
                snapshot.len(),
            )
        };
        assert_eq!(poll, MORNLEA_CLIENT_STATUS_WINDOW);
        assert_eq!(
            mornlea_client_window_destroy(CLIENT_ABI_VERSION, 0xDEAD),
            MORNLEA_CLIENT_STATUS_WINDOW
        );
        assert_eq!(
            mornlea_client_window_focus(CLIENT_ABI_VERSION, 0xDEAD),
            MORNLEA_CLIENT_STATUS_WINDOW
        );
        let mut ns_window = 7usize;
        // SAFETY: 同上。
        let status =
            unsafe { mornlea_client_window_ns_window(CLIENT_ABI_VERSION, 0xDEAD, &mut ns_window) };
        assert_eq!(status, MORNLEA_CLIENT_STATUS_WINDOW);
        assert_eq!(ns_window, 7, "失败调用不得写 ns_window 输出");
    }
}

// ---- render 入口族(client ABI v2)----

use crate::render::{
    BENCHMARK_BATCH_MAX_REPETITIONS, FrameInput, FrameResult, OffscreenRenderer, RenderCreateError,
};

/// 本机无可用 GPU 适配器;调用方(测试)应据此跳过而非失败。
pub const MORNLEA_CLIENT_STATUS_ADAPTER: u32 = 5;
/// 渲染资源容量不足(face 池或 origin 槽位耗尽)。
pub const MORNLEA_CLIENT_STATUS_CAPACITY: u32 = 6;
/// 窗口 surface 本帧不可用(遮挡/过期),调用方跳帧后重试。
pub const MORNLEA_CLIENT_STATUS_SKIPPED: u32 = 7;

/// render_frame 输入的固定头部字节数;其后是 visible_count×12 的 section 列表。
const FRAME_HEADER_BYTES: usize = 192;
/// MRW1 单批输入的最大字节数。
const RENDER_WORLD_MAX_BATCH_BYTES: usize = 4 * 1024 * 1024;

/// 全局渲染器表:与窗口不同,wgpu 对象 Send+Sync,渲染器不受 winit 的
/// 主线程约束;Go 调用方 goroutine 会在 OS 线程间迁移,thread-local 会把
/// 合法句柄误判为失效,因此这里用进程级 Mutex 表。
static RENDERERS: std::sync::Mutex<Option<HashMap<u64, OffscreenRenderer>>> =
    std::sync::Mutex::new(None);
/// 渲染器句柄计数;独立于窗口句柄空间,0 保留为无效。
static NEXT_RENDER_HANDLE: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(1);

fn with_renderer(handle: u64, operation: impl FnOnce(&mut OffscreenRenderer) -> u32) -> u32 {
    let mut guard = RENDERERS.lock().expect("渲染器表锁中毒");
    match guard.get_or_insert_with(HashMap::new).get_mut(&handle) {
        Some(renderer) => operation(renderer),
        None => MORNLEA_CLIENT_STATUS_WINDOW,
    }
}

fn frame_result_status(result: FrameResult) -> u32 {
    match result {
        FrameResult::Rendered => MORNLEA_CLIENT_STATUS_OK,
        FrameResult::Skipped => MORNLEA_CLIENT_STATUS_SKIPPED,
        FrameResult::Invalid => MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT,
        FrameResult::Capacity => MORNLEA_CLIENT_STATUS_CAPACITY,
    }
}

/// 创建离屏渲染器并写出句柄;无 GPU 适配器返回 ADAPTER 状态。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_create(
    abi_version: u32,
    width: u32,
    height: u32,
    out_handle: *mut u64,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if out_handle.is_null() || width == 0 || height == 0 {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        let renderer = match OffscreenRenderer::new(width, height) {
            Ok(renderer) => renderer,
            Err(RenderCreateError::Adapter) => return MORNLEA_CLIENT_STATUS_ADAPTER,
            Err(RenderCreateError::Device) => return MORNLEA_CLIENT_STATUS_WINDOW,
        };
        let handle = NEXT_RENDER_HANDLE.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        RENDERERS
            .lock()
            .expect("渲染器表锁中毒")
            .get_or_insert_with(HashMap::new)
            .insert(handle, renderer);
        // SAFETY: out_handle 已判非空,只在完整成功后写一次。
        unsafe { out_handle.write(handle) };
        MORNLEA_CLIENT_STATUS_OK
    })
}

/// 销毁渲染器;重复销毁返回 WINDOW 状态。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_render_destroy(abi_version: u32, handle: u64) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    catch(|| {
        let removed = RENDERERS
            .lock()
            .expect("渲染器表锁中毒")
            .get_or_insert_with(HashMap::new)
            .remove(&handle);
        match removed {
            Some(renderer) => {
                drop(renderer);
                MORNLEA_CLIENT_STATUS_OK
            }
            None => MORNLEA_CLIENT_STATUS_WINDOW,
        }
    })
}

/// 上传材质 atlas(逐 layer、逐 mip 的 RGBA 字节,长度必须精确匹配)。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_upload_atlas(
    abi_version: u32,
    handle: u64,
    layers: u32,
    pixels: *const u8,
    pixels_len: usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if pixels.is_null() || layers == 0 {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            // SAFETY: pixels 非空,调用方保证 pixels_len 字节可读。
            let data = unsafe { std::slice::from_raw_parts(pixels, pixels_len) };
            if renderer.upload_atlas(layers, data) {
                MORNLEA_CLIENT_STATUS_OK
            } else {
                MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
            }
        })
    })
}

/// 上传/替换一个 section 的两条 packed face 字节流(各自长度均为 8 的倍数;
/// 两条都空等价 drop)。
///
/// client ABI v5 起按 material 分流:`quads` 是不透明与 cutout 面,`water_quads`
/// 是水面。两条流的元素格式完全相同(8 字节 packed quad),分流只决定它们进
/// 哪条绘制路径——前者接 GPU culling 的单次 indirect draw,后者接半透明
/// water pass。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_upload_section(
    abi_version: u32,
    handle: u64,
    section_x: i32,
    section_y: i32,
    section_z: i32,
    quads: *const u8,
    quads_len: usize,
    water_quads: *const u8,
    water_quads_len: usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if !quads_len.is_multiple_of(8)
        || !water_quads_len.is_multiple_of(8)
        || (quads.is_null() && quads_len != 0)
        || (water_quads.is_null() && water_quads_len != 0)
    {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            if renderer.has_prepared_benchmark_batch() {
                return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
            }
            let data = if quads_len == 0 {
                &[][..]
            } else {
                // SAFETY: quads 非空,调用方保证 quads_len 字节可读。
                unsafe { std::slice::from_raw_parts(quads, quads_len) }
            };
            let water = if water_quads_len == 0 {
                &[][..]
            } else {
                // SAFETY: water_quads 非空,调用方保证 water_quads_len 字节可读。
                unsafe { std::slice::from_raw_parts(water_quads, water_quads_len) }
            };
            if renderer.upload_section((section_x, section_y, section_z), data, water) {
                MORNLEA_CLIENT_STATUS_OK
            } else {
                MORNLEA_CLIENT_STATUS_CAPACITY
            }
        })
    })
}

/// 丢弃一个 section(幂等)。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_render_drop_section(
    abi_version: u32,
    handle: u64,
    section_x: i32,
    section_y: i32,
    section_z: i32,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            if renderer.drop_section((section_x, section_y, section_z)) {
                MORNLEA_CLIENT_STATUS_OK
            } else {
                MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
            }
        })
    })
}

/// 更新尚未接管绘制的 MRW1 派生缓存。
///
/// 校验顺序固定为 ABI、非零且不超过 4 MiB 的长度、非空 pointer、无溢出
/// 地址范围、已有 renderer handle，最后才解析 MRW1。通过表示层检查后，
/// 调用方必须保证 `updates_len` 字节可读；Rust 只在同步调用期间借用该内存，
/// 不保存 pointer。失败不改变缓存，panic 映射为稳定状态码。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_apply_world_updates(
    abi_version: u32,
    handle: u64,
    updates: *const u8,
    updates_len: usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if updates_len == 0 || updates_len > RENDER_WORLD_MAX_BATCH_BYTES {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    if updates.is_null() {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    if updates.addr().checked_add(updates_len).is_none() {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            // SAFETY:表示层范围已校验，调用方保证这段输入可读；slice 不逸出闭包。
            let bytes = unsafe { std::slice::from_raw_parts(updates, updates_len) };
            if renderer.apply_render_world_updates(bytes) {
                MORNLEA_CLIENT_STATUS_OK
            } else {
                MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
            }
        })
    })
}

/// frame v2 的 TLV pass 段 tag,每类至多出现一次。
const FRAME_TAG_AVATAR: u32 = 1;
const FRAME_TAG_DROP: u32 = 2;
const FRAME_TAG_OUTLINE: u32 = 3;
const FRAME_TAG_OVERLAY: u32 = 4;
const FRAME_TAG_NAME_TAG: u32 = 5;
const FRAME_TAG_HUD: u32 = 6;
const FRAME_TAG_DEBUG: u32 = 7;
/// 水下水色叠加段(4 个 f32:RGBA)。client ABI v5 内的追加 tag,不升 ABI 版本。
const FRAME_TAG_WATER: u32 = 8;
/// egui UI 段(client ABI v9):layout v1/v2 字节,见 [`crate::ui::decode_ui_frame`]。
const FRAME_TAG_UI: u32 = 9;

/// 解析 render_frame 输入;违约返回 None。
///
/// header@188 是 layout version:0 为 v1(纯地形,精确长度),2 为 v2
/// (可见列表之后跟 TLV pass 段序列:tag u32 + length u32 + bytes,
/// 除 UI 段外各段 length 4 对齐,未知 tag/越界/重复段拒绝)。
///
/// UI 段(FRAME_TAG_UI)豁免 4 对齐:它是 UTF-8 字段序列(layout/flags/按钮数/
/// 各按钮 id+label+enabled/title/version/error),长度由字段自身界定,不保证
/// 4 对齐(如四按钮 + 中文 error 的 142 字节);其余 pass 段(avatar/drop/outline/
/// 字样/水色)是定长实例数组,天然 4 对齐,故各段校验不变。UI 段内容合法性由
/// decode_ui_frame 在解析层校验。
fn parse_frame(bytes: &[u8]) -> Option<FrameInput> {
    if bytes.len() < FRAME_HEADER_BYTES {
        return None;
    }
    let read_f32 =
        |offset: usize| f32::from_le_bytes(bytes[offset..offset + 4].try_into().unwrap());
    let read_u32 =
        |offset: usize| u32::from_le_bytes(bytes[offset..offset + 4].try_into().unwrap());
    let mut view_proj = [0f32; 16];
    let mut view_proj_inv = [0f32; 16];
    for i in 0..16 {
        view_proj[i] = read_f32(i * 4);
        view_proj_inv[i] = read_f32(64 + i * 4);
    }
    let layout = read_u32(188);
    let visible_count = read_u32(184) as usize;
    if !(layout == 0 || layout == 2) || visible_count > 128 * 1024 {
        return None;
    }
    let sections_end = FRAME_HEADER_BYTES + visible_count * 12;
    if layout == 0 && bytes.len() != sections_end {
        return None;
    }
    if bytes.len() < sections_end {
        return None;
    }
    let mut visible = Vec::with_capacity(visible_count);
    for index in 0..visible_count {
        let offset = FRAME_HEADER_BYTES + index * 12;
        let read_i32 = |o: usize| i32::from_le_bytes(bytes[o..o + 4].try_into().unwrap());
        visible.push((read_i32(offset), read_i32(offset + 4), read_i32(offset + 8)));
    }
    // v2:可见列表之后消费 TLV pass 段。
    let mut avatar_instances = Vec::new();
    let mut drop_instances = Vec::new();
    let mut outline = Vec::new();
    let mut overlay_strength = 0.0f32;
    let mut water_tint = [0.0f32; 4];
    let mut name_tag_vertices = Vec::new();
    let mut hud_vertices = Vec::new();
    let mut debug_vertices = Vec::new();
    let mut ui_segment = Vec::new();
    if layout == 2 {
        let mut cursor = sections_end;
        let mut seen = [false; 10];
        while cursor < bytes.len() {
            if bytes.len() - cursor < 8 {
                return None;
            }
            let tag = read_u32(cursor);
            let length = read_u32(cursor + 4) as usize;
            cursor += 8;
            // 除 UI 段(UTF-8 字段序列,长度由字段界定)外,其余 pass 段均为定长
            // 实例数组(长度天然 4 对齐),故对齐检查对非 UI 段保持一致;UI 段
            // 只做越界检查,内容合法性由 decode_ui_frame 在解析层保证。
            if (tag != FRAME_TAG_UI && !length.is_multiple_of(4)) || bytes.len() - cursor < length {
                return None;
            }
            let payload = &bytes[cursor..cursor + length];
            cursor += length;
            let index = tag as usize;
            if !(1..=9).contains(&index) || seen[index] {
                return None;
            }
            seen[index] = true;
            match tag {
                FRAME_TAG_AVATAR => avatar_instances = payload.to_vec(),
                FRAME_TAG_DROP => drop_instances = payload.to_vec(),
                FRAME_TAG_OUTLINE => outline = payload.to_vec(),
                FRAME_TAG_OVERLAY => {
                    if length != 4 {
                        return None;
                    }
                    overlay_strength = f32::from_le_bytes(payload.try_into().unwrap());
                }
                FRAME_TAG_NAME_TAG => name_tag_vertices = payload.to_vec(),
                FRAME_TAG_HUD => hud_vertices = payload.to_vec(),
                FRAME_TAG_DEBUG => debug_vertices = payload.to_vec(),
                FRAME_TAG_WATER => {
                    if length != 16 {
                        return None;
                    }
                    for (index, slot) in water_tint.iter_mut().enumerate() {
                        *slot = f32::from_le_bytes(
                            payload[index * 4..index * 4 + 4].try_into().unwrap(),
                        );
                    }
                }
                FRAME_TAG_UI => {
                    ui_segment = payload.to_vec();
                    // UI 段语义校验在解析层完成:decode_ui_frame 违约即拒绝,
                    // 先于渲染器状态、不触碰帧 target,与既有"未知 tag/越界/重复
                    // 段拒绝"同一路径(ffi 层统一转 INVALID_ARGUMENT)。
                    if crate::ui::decode_ui_frame(&ui_segment).is_err() {
                        return None;
                    }
                }
                _ => return None,
            }
        }
    }
    Some(FrameInput {
        view_proj,
        view_proj_inv,
        pos: [read_f32(128), read_f32(132), read_f32(136)],
        daylight: read_f32(140),
        sun_direction: [read_f32(144), read_f32(148), read_f32(152)],
        star_visibility: read_f32(156),
        sky_color: [read_f32(160), read_f32(164), read_f32(168), read_f32(172)],
        cloud_macro_x: read_u32(176),
        cloud_local: read_f32(180),
        visible,
        avatar_instances,
        drop_instances,
        outline,
        overlay_strength,
        water_tint,
        name_tag_vertices,
        hud_vertices,
        debug_vertices,
        ui_segment,
    })
}

/// 渲染一帧(每帧一次;帧输入为固定头 + 可见 section 列表)。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_frame(
    abi_version: u32,
    handle: u64,
    frame: *const u8,
    frame_len: usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if frame.is_null() {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        // SAFETY: frame 非空,调用方保证 frame_len 字节可读。
        let bytes = unsafe { std::slice::from_raw_parts(frame, frame_len) };
        let Some(input) = parse_frame(bytes) else {
            return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
        };
        with_renderer(handle, |renderer| {
            frame_result_status(renderer.render_frame(&input))
        })
    })
}

/// 录制离屏 benchmark 批次但不提交。
///
/// `frame` 必须指向 `frame_len` 个可读字节，`repeat` 只接受
/// `1..=BENCHMARK_BATCH_MAX_REPETITIONS`。调用不保留 `frame` 指针；输入、
/// renderer 状态或离屏约束违约时返回 `INVALID_ARGUMENT`，不得替换已有批次。
/// 此入口与 header 的 client ABI v11 声明必须同步。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_prepare_benchmark_batch(
    abi_version: u32,
    handle: u64,
    frame: *const u8,
    frame_len: usize,
    repeat: u32,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if frame.is_null() || !(1..=BENCHMARK_BATCH_MAX_REPETITIONS).contains(&repeat) {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        // SAFETY: frame 非空,调用方保证 frame_len 字节可读。
        let bytes = unsafe { std::slice::from_raw_parts(frame, frame_len) };
        let Some(input) = parse_frame(bytes) else {
            return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
        };
        with_renderer(handle, |renderer| {
            frame_result_status(renderer.prepare_benchmark_batch(&input, repeat))
        })
    })
}

/// 提交已录制的离屏 benchmark 批次并等待 GPU 完成。
///
/// 入口消费 renderer 持有的唯一 command buffer；不存在 prepared batch 时返回
/// `INVALID_ARGUMENT`。无裸指针，调用方不转移其他资源所有权；此入口与 header
/// 的 client ABI v11 声明必须同步。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_render_submit_benchmark_batch(
    abi_version: u32,
    handle: u64,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            frame_result_status(renderer.submit_benchmark_batch())
        })
    })
}

/// 阻塞回读离屏 BGRA 图像;`out_len` 必须恰为 width×height×4。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_readback(
    abi_version: u32,
    handle: u64,
    out: *mut u8,
    out_len: usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if out.is_null() {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            if out_len != renderer.output_bytes() {
                return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
            }
            // SAFETY: out 非空且长度已校验,调用方保证可写。
            let out = unsafe { std::slice::from_raw_parts_mut(out, out_len) };
            if renderer.readback(out) {
                MORNLEA_CLIENT_STATUS_OK
            } else {
                // 窗口模式不支持回读。
                MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
            }
        })
    })
}

#[cfg(test)]
mod render_ffi_tests {
    use super::*;

    #[test]
    fn all_versioned_exports_reject_wrong_abi_first() {
        macro_rules! assert_bad_abi {
            ($call:expr) => {
                assert_eq!($call, MORNLEA_CLIENT_STATUS_ABI_VERSION)
            };
        }
        let bad = CLIENT_ABI_VERSION + 1;

        assert_bad_abi!(unsafe {
            mornlea_client_window_create(bad, 0, 0, std::ptr::null(), 0, std::ptr::null_mut())
        });
        assert_bad_abi!(mornlea_client_window_destroy(bad, 0));
        assert_bad_abi!(unsafe { mornlea_client_window_poll(bad, 0, std::ptr::null_mut(), 0) });
        assert_bad_abi!(mornlea_client_window_set_cursor_captured(bad, 0, 2));
        assert_bad_abi!(mornlea_client_window_set_content_size(bad, 0, 0, 0));
        assert_bad_abi!(mornlea_client_window_set_floating(bad, 0, 2));
        assert_bad_abi!(mornlea_client_window_focus(bad, 0));
        assert_bad_abi!(mornlea_client_window_cancel_close(bad, 0));
        assert_bad_abi!(unsafe { mornlea_client_window_ns_window(bad, 0, std::ptr::null_mut()) });

        assert_bad_abi!(unsafe { mornlea_client_render_create(bad, 0, 0, std::ptr::null_mut()) });
        assert_bad_abi!(mornlea_client_render_destroy(bad, 0));
        assert_bad_abi!(unsafe {
            mornlea_client_render_upload_atlas(bad, 0, 0, std::ptr::null(), 0)
        });
        assert_bad_abi!(unsafe {
            mornlea_client_render_upload_section(
                bad,
                0,
                0,
                0,
                0,
                std::ptr::null(),
                1,
                std::ptr::null(),
                1,
            )
        });
        assert_bad_abi!(mornlea_client_render_drop_section(bad, 0, 0, 0, 0));
        assert_bad_abi!(unsafe {
            mornlea_client_render_apply_world_updates(bad, 0, std::ptr::null(), 0)
        });
        assert_bad_abi!(unsafe {
            mornlea_client_render_upload_lod_tile(bad, 0, 0, 0, std::ptr::null(), 1)
        });
        assert_bad_abi!(mornlea_client_render_drop_lod_tile(bad, 0, 0, 0));
        assert_bad_abi!(mornlea_client_render_set_lod_fog(
            bad,
            0,
            f32::NAN,
            f32::NAN
        ));
        assert_bad_abi!(unsafe {
            mornlea_client_render_upload_ui_font(bad, 0, std::ptr::null(), 0)
        });
        assert_bad_abi!(unsafe {
            mornlea_client_render_drain_ui_events(
                bad,
                0,
                std::ptr::null_mut(),
                0,
                std::ptr::null_mut(),
            )
        });
        assert_bad_abi!(unsafe { mornlea_client_render_frame(bad, 0, std::ptr::null(), 0) });
        assert_bad_abi!(unsafe {
            mornlea_client_render_prepare_benchmark_batch(bad, 0, std::ptr::null(), 0, 0)
        });
        assert_bad_abi!(mornlea_client_render_submit_benchmark_batch(bad, 0));
        assert_bad_abi!(unsafe {
            mornlea_client_render_upload_glyph_rect(bad, 0, 0, 0, 0, 0, std::ptr::null(), 0)
        });
        assert_bad_abi!(unsafe {
            mornlea_client_render_upload_hud_atlas(bad, 0, 0, 0, std::ptr::null(), 0)
        });
        assert_bad_abi!(unsafe {
            mornlea_client_render_create_windowed(bad, 0, std::ptr::null_mut())
        });
        assert_bad_abi!(mornlea_client_render_resize(bad, 0, 0, 0));
        assert_bad_abi!(unsafe { mornlea_client_render_readback(bad, 0, std::ptr::null_mut(), 0) });
    }

    fn reset_and_single_section_batch() -> Vec<u8> {
        let mut bytes = Vec::with_capacity(96);
        bytes.extend_from_slice(b"MRW1");
        bytes.extend_from_slice(&1u16.to_le_bytes());
        bytes.extend_from_slice(&0u16.to_le_bytes());
        bytes.extend_from_slice(&1u64.to_le_bytes());
        bytes.extend_from_slice(&2u32.to_le_bytes());
        bytes.extend_from_slice(&0u32.to_le_bytes());

        bytes.push(5);
        bytes.extend_from_slice(&[0; 3]);
        bytes.extend_from_slice(&[0; 28]);

        bytes.push(1);
        bytes.extend_from_slice(&[0; 3]);
        bytes.extend_from_slice(&0i32.to_le_bytes());
        bytes.extend_from_slice(&0i32.to_le_bytes());
        bytes.extend_from_slice(&0i32.to_le_bytes());
        bytes.extend_from_slice(&0i32.to_le_bytes());
        bytes.extend_from_slice(&1u64.to_le_bytes());
        bytes.extend_from_slice(&8u32.to_le_bytes());
        bytes.extend_from_slice(&2u16.to_le_bytes());
        bytes.extend_from_slice(&0u16.to_le_bytes());
        bytes.extend_from_slice(&0u16.to_le_bytes());
        bytes.extend_from_slice(&0u16.to_le_bytes());
        bytes
    }

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
        assert_eq!(
            unsafe {
                mornlea_client_render_apply_world_updates(
                    CLIENT_ABI_VERSION,
                    1,
                    std::ptr::null(),
                    0,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT,
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_apply_world_updates(
                    CLIENT_ABI_VERSION,
                    1,
                    std::ptr::null(),
                    4 * 1024 * 1024 + 1,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT,
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_apply_world_updates(
                    CLIENT_ABI_VERSION,
                    1,
                    std::ptr::null(),
                    1,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT,
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_apply_world_updates(
                    CLIENT_ABI_VERSION,
                    1,
                    std::ptr::without_provenance(usize::MAX),
                    1,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT,
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_apply_world_updates(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    std::ptr::without_provenance(1),
                    1,
                )
            },
            MORNLEA_CLIENT_STATUS_WINDOW,
            "未知句柄路径不得解引用 input",
        );

        let mut handle = 0u64;
        let create =
            unsafe { mornlea_client_render_create(CLIENT_ABI_VERSION, 16, 16, &mut handle) };
        if create == MORNLEA_CLIENT_STATUS_ADAPTER {
            return;
        }
        assert_eq!(create, MORNLEA_CLIENT_STATUS_OK);

        let invalid = b"not-mrw1";
        assert_eq!(
            unsafe {
                mornlea_client_render_apply_world_updates(
                    CLIENT_ABI_VERSION,
                    handle,
                    invalid.as_ptr(),
                    invalid.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT,
        );
        let valid = reset_and_single_section_batch();
        assert_eq!(
            unsafe {
                mornlea_client_render_apply_world_updates(
                    CLIENT_ABI_VERSION,
                    handle,
                    valid.as_ptr(),
                    valid.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_OK,
        );
        assert_eq!(
            mornlea_client_render_destroy(CLIENT_ABI_VERSION, handle),
            MORNLEA_CLIENT_STATUS_OK,
        );
    }

    #[test]
    fn catch_maps_panic_to_status() {
        assert_eq!(
            catch(|| panic!("验证 client FFI panic catcher")),
            MORNLEA_CLIENT_STATUS_PANIC,
        );
    }

    #[test]
    fn render_entries_reject_bad_abi_and_arguments() {
        let mut handle = 7u64;
        // SAFETY: 指针来自有效局部变量。
        let create =
            unsafe { mornlea_client_render_create(CLIENT_ABI_VERSION + 1, 64, 64, &mut handle) };
        assert_eq!(create, MORNLEA_CLIENT_STATUS_ABI_VERSION);
        assert_eq!(handle, 7, "失败调用不得写句柄");
        // SAFETY: 同上;宽为零必须拒绝。
        let zero = unsafe { mornlea_client_render_create(CLIENT_ABI_VERSION, 0, 64, &mut handle) };
        assert_eq!(zero, MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT);

        // 任一条流的长度非 8 的倍数都必须拒绝(两条流各校验一次:
        // 只校验其中一条的实现会让另一条静默错位)。
        let quads = [0u8; 9];
        let aligned = [0u8; 8];
        // SAFETY: 同上。
        let misaligned = unsafe {
            mornlea_client_render_upload_section(
                CLIENT_ABI_VERSION,
                0xBEEF,
                0,
                0,
                0,
                quads.as_ptr(),
                quads.len(),
                std::ptr::null(),
                0,
            )
        };
        assert_eq!(misaligned, MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT);
        // SAFETY: 同上。
        let misaligned_water = unsafe {
            mornlea_client_render_upload_section(
                CLIENT_ABI_VERSION,
                0xBEEF,
                0,
                0,
                0,
                aligned.as_ptr(),
                aligned.len(),
                quads.as_ptr(),
                quads.len(),
            )
        };
        assert_eq!(misaligned_water, MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT);

        // 未知句柄一律 WINDOW。
        assert_eq!(
            mornlea_client_render_destroy(CLIENT_ABI_VERSION, 0xBEEF),
            MORNLEA_CLIENT_STATUS_WINDOW
        );
        let frame = [0u8; FRAME_HEADER_BYTES];
        // SAFETY: 同上。
        let status = unsafe {
            mornlea_client_render_frame(CLIENT_ABI_VERSION, 0xBEEF, frame.as_ptr(), frame.len())
        };
        assert_eq!(status, MORNLEA_CLIENT_STATUS_WINDOW);
    }

    #[test]
    fn prepare_benchmark_batch_validates_input_and_count_range() {
        let frame = [0u8; FRAME_HEADER_BYTES];
        let mut malformed = frame;
        malformed[188..192].copy_from_slice(&1u32.to_le_bytes());
        // SAFETY: 空指针、畸形帧和范围外次数均为入口必须在句柄查找前拒绝的非法输入。
        assert_eq!(
            unsafe {
                mornlea_client_render_prepare_benchmark_batch(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    std::ptr::null(),
                    0,
                    1,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // SAFETY: 指针来自有效局部数组；layout 1 不是合法帧布局。
        assert_eq!(
            unsafe {
                mornlea_client_render_prepare_benchmark_batch(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    malformed.as_ptr(),
                    malformed.len(),
                    1,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // SAFETY: 指针来自有效局部数组；`0` 与 `257` 均在 ABI `1..=256` 范围外。
        assert_eq!(
            unsafe {
                mornlea_client_render_prepare_benchmark_batch(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    frame.as_ptr(),
                    frame.len(),
                    0,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_prepare_benchmark_batch(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    frame.as_ptr(),
                    frame.len(),
                    257,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // SAFETY: 最大合法次数通过参数校验后才因未知句柄被拒绝。
        assert_eq!(
            unsafe {
                mornlea_client_render_prepare_benchmark_batch(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    frame.as_ptr(),
                    frame.len(),
                    256,
                )
            },
            MORNLEA_CLIENT_STATUS_WINDOW
        );
    }

    #[test]
    fn submit_benchmark_batch_rejects_unprepared_renderer() {
        let mut handle = 0u64;
        // SAFETY: 指针来自有效局部变量。
        let create =
            unsafe { mornlea_client_render_create(CLIENT_ABI_VERSION, 16, 16, &mut handle) };
        if create == MORNLEA_CLIENT_STATUS_ADAPTER {
            return;
        }
        assert_eq!(create, MORNLEA_CLIENT_STATUS_OK);

        assert_eq!(
            mornlea_client_render_submit_benchmark_batch(CLIENT_ABI_VERSION, handle),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            mornlea_client_render_destroy(CLIENT_ABI_VERSION, handle),
            MORNLEA_CLIENT_STATUS_OK
        );
    }

    #[test]
    fn prepared_benchmark_batch_rejects_mutations_and_preserves_first_frame() {
        let mut handle = 0u64;
        // SAFETY: 指针来自有效局部变量。
        let create =
            unsafe { mornlea_client_render_create(CLIENT_ABI_VERSION, 32, 16, &mut handle) };
        if create == MORNLEA_CLIENT_STATUS_ADAPTER {
            return;
        }
        assert_eq!(create, MORNLEA_CLIENT_STATUS_OK);

        let mut first = [0u8; FRAME_HEADER_BYTES];
        let mut second = [0u8; FRAME_HEADER_BYTES];
        for i in 0..4 {
            let one = 1.0f32.to_le_bytes();
            first[i * 16 + i * 4..i * 16 + i * 4 + 4].copy_from_slice(&one);
            first[64 + i * 16 + i * 4..64 + i * 16 + i * 4 + 4].copy_from_slice(&one);
            second[i * 16 + i * 4..i * 16 + i * 4 + 4].copy_from_slice(&one);
            second[64 + i * 16 + i * 4..64 + i * 16 + i * 4 + 4].copy_from_slice(&one);
        }
        first[132..136].copy_from_slice(&192.0f32.to_le_bytes());
        first[140..144].copy_from_slice(&1.0f32.to_le_bytes());
        first[148..152].copy_from_slice(&1.0f32.to_le_bytes());
        first[160..176].copy_from_slice(&[1.0f32, 0.0, 0.0, 1.0].map(f32::to_le_bytes).concat());
        second.copy_from_slice(&first);
        second[140..144].copy_from_slice(&0.0f32.to_le_bytes());
        second[160..176].copy_from_slice(&[0.0f32, 0.0, 1.0, 1.0].map(f32::to_le_bytes).concat());

        let mut first_output = vec![0u8; 32 * 16 * 4];
        let mut second_output = vec![0u8; 32 * 16 * 4];
        // 两帧天空色不同；daylight 同时变化以确保全屏天空 pass 的回读可区分。
        // SAFETY: 帧和回读指针均来自有效局部数组。
        assert_eq!(
            unsafe {
                mornlea_client_render_frame(CLIENT_ABI_VERSION, handle, first.as_ptr(), first.len())
            },
            MORNLEA_CLIENT_STATUS_OK
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_readback(
                    CLIENT_ABI_VERSION,
                    handle,
                    first_output.as_mut_ptr(),
                    first_output.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_OK
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_frame(
                    CLIENT_ABI_VERSION,
                    handle,
                    second.as_ptr(),
                    second.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_OK
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_readback(
                    CLIENT_ABI_VERSION,
                    handle,
                    second_output.as_mut_ptr(),
                    second_output.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_OK
        );
        assert_ne!(first_output, second_output, "两份合法帧的回读必须可区分");

        // SAFETY: 帧指针来自有效局部数组。
        assert_eq!(
            unsafe {
                mornlea_client_render_prepare_benchmark_batch(
                    CLIENT_ABI_VERSION,
                    handle,
                    first.as_ptr(),
                    first.len(),
                    1,
                )
            },
            MORNLEA_CLIENT_STATUS_OK
        );
        let atlas = vec![
            0u8;
            (0..crate::render::ATLAS_MIPS)
                .map(|mip| {
                    let size = (crate::render::ATLAS_TEX_SIZE >> mip).max(1) as usize;
                    size * size * 4
                })
                .sum()
        ];
        let section = [0u8; 8];
        let lod_tile = [0u8; crate::render::lod::LOD_QUAD_BYTES];
        let glyph = [0u8; 1];
        let hud = [0u8; 4];
        let font = include_bytes!("ui/testdata/demo.ttf");
        // prepared batch 持有所有这些 GPU 资源的引用；它提交前每项变更都必须拒绝。
        // SAFETY: 所有指针均来自本测试的有效局部数组或静态字体字节。
        assert_eq!(
            unsafe {
                mornlea_client_render_frame(
                    CLIENT_ABI_VERSION,
                    handle,
                    second.as_ptr(),
                    second.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            mornlea_client_render_resize(CLIENT_ABI_VERSION, handle, 16, 32),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_upload_atlas(
                    CLIENT_ABI_VERSION,
                    handle,
                    1,
                    atlas.as_ptr(),
                    atlas.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_upload_section(
                    CLIENT_ABI_VERSION,
                    handle,
                    0,
                    0,
                    0,
                    section.as_ptr(),
                    section.len(),
                    std::ptr::null(),
                    0,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            mornlea_client_render_drop_section(CLIENT_ABI_VERSION, handle, 0, 0, 0),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_upload_lod_tile(
                    CLIENT_ABI_VERSION,
                    handle,
                    0,
                    0,
                    lod_tile.as_ptr(),
                    lod_tile.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            mornlea_client_render_drop_lod_tile(CLIENT_ABI_VERSION, handle, 0, 0),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            mornlea_client_render_set_lod_fog(CLIENT_ABI_VERSION, handle, 768.0, 1152.0),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_upload_glyph_rect(
                    CLIENT_ABI_VERSION,
                    handle,
                    0,
                    0,
                    1,
                    1,
                    glyph.as_ptr(),
                    glyph.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_upload_hud_atlas(
                    CLIENT_ABI_VERSION,
                    handle,
                    1,
                    1,
                    hud.as_ptr(),
                    hud.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            unsafe {
                mornlea_client_render_upload_ui_font(
                    CLIENT_ABI_VERSION,
                    handle,
                    font.as_ptr(),
                    font.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // SAFETY: 同一 prepared batch 未 submit 前不得被第二帧覆盖。
        assert_eq!(
            unsafe {
                mornlea_client_render_prepare_benchmark_batch(
                    CLIENT_ABI_VERSION,
                    handle,
                    second.as_ptr(),
                    second.len(),
                    1,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            mornlea_client_render_submit_benchmark_batch(CLIENT_ABI_VERSION, handle),
            MORNLEA_CLIENT_STATUS_OK
        );
        let mut submitted = vec![0u8; 32 * 16 * 4];
        // SAFETY: 回读指针来自有效局部数组。
        assert_eq!(
            unsafe {
                mornlea_client_render_readback(
                    CLIENT_ABI_VERSION,
                    handle,
                    submitted.as_mut_ptr(),
                    submitted.len(),
                )
            },
            MORNLEA_CLIENT_STATUS_OK
        );
        assert_eq!(submitted, first_output, "拒绝重复 prepare 后必须提交首批帧");
        assert_eq!(
            mornlea_client_render_destroy(CLIENT_ABI_VERSION, handle),
            MORNLEA_CLIENT_STATUS_OK
        );
    }

    #[test]
    fn submit_benchmark_batch_consumes_prepared_batch_once() {
        let mut handle = 0u64;
        // SAFETY: 指针来自有效局部变量。
        let create =
            unsafe { mornlea_client_render_create(CLIENT_ABI_VERSION, 16, 16, &mut handle) };
        if create == MORNLEA_CLIENT_STATUS_ADAPTER {
            return;
        }
        assert_eq!(create, MORNLEA_CLIENT_STATUS_OK);

        let frame = [0u8; FRAME_HEADER_BYTES];
        // SAFETY: 帧指针来自有效局部数组。
        assert_eq!(
            unsafe {
                mornlea_client_render_prepare_benchmark_batch(
                    CLIENT_ABI_VERSION,
                    handle,
                    frame.as_ptr(),
                    frame.len(),
                    1,
                )
            },
            MORNLEA_CLIENT_STATUS_OK
        );
        assert_eq!(
            mornlea_client_render_submit_benchmark_batch(CLIENT_ABI_VERSION, handle),
            MORNLEA_CLIENT_STATUS_OK
        );
        assert_eq!(
            mornlea_client_render_submit_benchmark_batch(CLIENT_ABI_VERSION, handle),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(
            mornlea_client_render_destroy(CLIENT_ABI_VERSION, handle),
            MORNLEA_CLIENT_STATUS_OK
        );
    }

    #[test]
    fn render_roundtrip_or_skip_without_adapter() {
        let mut handle = 0u64;
        // SAFETY: 指针来自有效局部变量。
        let create =
            unsafe { mornlea_client_render_create(CLIENT_ABI_VERSION, 32, 16, &mut handle) };
        if create == MORNLEA_CLIENT_STATUS_ADAPTER {
            return; // 无 GPU 环境跳过。
        }
        assert_eq!(create, MORNLEA_CLIENT_STATUS_OK);

        let mut frame = [0u8; FRAME_HEADER_BYTES];
        // 恒等矩阵 + daylight=1。
        for i in 0..4 {
            let one = 1.0f32.to_le_bytes();
            frame[i * 16 + i * 4..i * 16 + i * 4 + 4].copy_from_slice(&one);
            frame[64 + i * 16 + i * 4..64 + i * 16 + i * 4 + 4].copy_from_slice(&one);
        }
        // SAFETY: 同上。
        let status = unsafe {
            mornlea_client_render_frame(CLIENT_ABI_VERSION, handle, frame.as_ptr(), frame.len())
        };
        assert_eq!(status, MORNLEA_CLIENT_STATUS_OK);

        let mut out = vec![0u8; 32 * 16 * 4];
        // SAFETY: 同上。
        let readback = unsafe {
            mornlea_client_render_readback(CLIENT_ABI_VERSION, handle, out.as_mut_ptr(), out.len())
        };
        assert_eq!(readback, MORNLEA_CLIENT_STATUS_OK);
        assert!(out.iter().any(|&b| b != 0));

        // 回读缓冲长度不符必须拒绝且不触碰缓冲。
        let mut short = vec![0xAAu8; 32 * 16 * 4 - 1];
        // SAFETY: 同上。
        let bad = unsafe {
            mornlea_client_render_readback(
                CLIENT_ABI_VERSION,
                handle,
                short.as_mut_ptr(),
                short.len(),
            )
        };
        assert_eq!(bad, MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT);
        assert!(short.iter().all(|&b| b == 0xAA));

        assert_eq!(
            mornlea_client_render_destroy(CLIENT_ABI_VERSION, handle),
            MORNLEA_CLIENT_STATUS_OK
        );
        assert_eq!(
            mornlea_client_render_destroy(CLIENT_ABI_VERSION, handle),
            MORNLEA_CLIENT_STATUS_WINDOW
        );
    }
}

#[cfg(test)]
mod frame_v2_tests {
    use super::*;

    /// 构造 layout v2 帧:头 + 0 个可见 section + 给定 TLV 段字节。
    fn v2_frame(passes: &[u8]) -> Vec<u8> {
        let mut frame = vec![0u8; FRAME_HEADER_BYTES];
        frame[188..192].copy_from_slice(&2u32.to_le_bytes());
        frame.extend_from_slice(passes);
        frame
    }

    fn tlv(tag: u32, payload: &[u8]) -> Vec<u8> {
        let mut out = Vec::new();
        out.extend_from_slice(&tag.to_le_bytes());
        out.extend_from_slice(&(payload.len() as u32).to_le_bytes());
        out.extend_from_slice(payload);
        out
    }

    /// 经 render_frame 入口驱动解析:解析失败返回 INVALID_ARGUMENT,
    /// 解析成功但句柄未知返回 WINDOW——以此区分接受与拒绝,无需 GPU。
    fn parse_status(frame: &[u8]) -> u32 {
        // SAFETY: 指针来自有效切片。
        unsafe {
            mornlea_client_render_frame(CLIENT_ABI_VERSION, 0xF00D, frame.as_ptr(), frame.len())
        }
    }

    #[test]
    fn v2_pass_segments_parse_matrix() {
        // 合法:每类 tag 一次 + overlay 4 字节。
        let mut passes = Vec::new();
        passes.extend(tlv(FRAME_TAG_AVATAR, &[0u8; 8]));
        passes.extend(tlv(FRAME_TAG_DROP, &[0u8; 4]));
        passes.extend(tlv(FRAME_TAG_OUTLINE, &[0u8; 16]));
        passes.extend(tlv(FRAME_TAG_OVERLAY, &0.5f32.to_le_bytes()));
        passes.extend(tlv(FRAME_TAG_NAME_TAG, &[0u8; 32]));
        passes.extend(tlv(FRAME_TAG_HUD, &[0u8; 32]));
        passes.extend(tlv(FRAME_TAG_DEBUG, &[]));
        assert_eq!(
            parse_status(&v2_frame(&passes)),
            MORNLEA_CLIENT_STATUS_WINDOW,
            "合法 v2 帧应通过解析并因句柄未知被拒"
        );

        // 空 pass 段序列同样合法(v2 允许零段)。
        assert_eq!(parse_status(&v2_frame(&[])), MORNLEA_CLIENT_STATUS_WINDOW);

        // 未知 tag(10 超出白名单 1..=9)。
        assert_eq!(
            parse_status(&v2_frame(&tlv(10, &[0u8; 4]))),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // 重复段。
        let mut dup = tlv(FRAME_TAG_HUD, &[0u8; 4]);
        dup.extend(tlv(FRAME_TAG_HUD, &[0u8; 4]));
        assert_eq!(
            parse_status(&v2_frame(&dup)),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // 长度越界(声明超过剩余字节)。
        let mut overflow = Vec::new();
        overflow.extend_from_slice(&FRAME_TAG_AVATAR.to_le_bytes());
        overflow.extend_from_slice(&64u32.to_le_bytes());
        overflow.extend_from_slice(&[0u8; 8]);
        assert_eq!(
            parse_status(&v2_frame(&overflow)),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // 长度非 4 对齐。
        assert_eq!(
            parse_status(&v2_frame(&tlv(FRAME_TAG_NAME_TAG, &[0u8; 6]))),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // overlay 段长度必须为 4。
        assert_eq!(
            parse_status(&v2_frame(&tlv(FRAME_TAG_OVERLAY, &[0u8; 8]))),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // 截断的 TLV 头。
        assert_eq!(
            parse_status(&v2_frame(&[1, 0, 0])),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // 非法 layout version。
        let mut bad_layout = vec![0u8; FRAME_HEADER_BYTES];
        bad_layout[188..192].copy_from_slice(&1u32.to_le_bytes());
        assert_eq!(
            parse_status(&bad_layout),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // v1(layout 0)带尾随字节拒绝。
        let mut v1_trailing = vec![0u8; FRAME_HEADER_BYTES + 4];
        v1_trailing[188] = 0;
        assert_eq!(
            parse_status(&v1_trailing),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
    }
    /// 最小合法 UI 段:layout v1 + visible + 0 按钮 + 三个空串(24 字节,4 对齐)。
    fn ui_segment_minimal() -> Vec<u8> {
        let mut out = Vec::new();
        out.extend_from_slice(&crate::ui::UI_LAYOUT_VERSION.to_le_bytes());
        out.extend_from_slice(&crate::ui::UI_FLAG_VISIBLE.to_le_bytes());
        out.extend_from_slice(&0u32.to_le_bytes()); // 按钮数
        out.extend_from_slice(&0u32.to_le_bytes()); // title len
        out.extend_from_slice(&0u32.to_le_bytes()); // version len
        out.extend_from_slice(&0u32.to_le_bytes()); // error len
        out
    }

    #[test]
    fn v2_ui_segment_parses_and_carries_bytes() {
        // 合法 UI 段:解析通过(句柄未知停在 WINDOW 证明接受);无 tag 帧也合法,
        // ui_segment 为空。
        let seg = ui_segment_minimal();
        assert_eq!(
            parse_status(&v2_frame(&tlv(FRAME_TAG_UI, &seg))),
            MORNLEA_CLIENT_STATUS_WINDOW
        );
        assert_eq!(parse_status(&v2_frame(&[])), MORNLEA_CLIENT_STATUS_WINDOW);
    }

    #[test]
    fn v2_invalid_ui_segment_rejected_same_path() {
        // 非法 UI 段(layout 版本错误)经 render_frame 回调 → INVALID_ARGUMENT,
        // 与既有非法 pass 段同一拒绝种类。
        let mut bad = Vec::new();
        bad.extend_from_slice(&0u32.to_le_bytes()); // layout=0 非法
        bad.extend_from_slice(&0u32.to_le_bytes()); // flags
        bad.extend_from_slice(&0u32.to_le_bytes()); // 按钮数
        bad.extend_from_slice(&0u32.to_le_bytes());
        bad.extend_from_slice(&0u32.to_le_bytes());
        bad.extend_from_slice(&0u32.to_le_bytes());
        assert_eq!(
            parse_status(&v2_frame(&tlv(FRAME_TAG_UI, &bad))),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
    }

    fn ui_settings_segment() -> Vec<u8> {
        let mut out = Vec::new();
        out.extend_from_slice(&crate::ui::UI_SETTINGS_LAYOUT_VERSION.to_le_bytes());
        out.extend_from_slice(&1u32.to_le_bytes());
        out.extend_from_slice(&0.25f32.to_bits().to_le_bytes());
        out.extend_from_slice(&2u32.to_le_bytes());
        out.extend_from_slice(&11u32.to_le_bytes());
        out.extend_from_slice(b"packs/local");
        out.extend_from_slice(&1u32.to_le_bytes());
        out.extend_from_slice(&0u32.to_le_bytes());
        out.extend_from_slice(&0u32.to_le_bytes());
        out
    }

    #[test]
    fn v2_settings_ui_invalid_values_fail_before_handle_lookup() {
        let valid = ui_settings_segment();
        assert_eq!(
            parse_status(&v2_frame(&tlv(FRAME_TAG_UI, &valid))),
            MORNLEA_CLIENT_STATUS_WINDOW
        );

        for (name, offset, value) in [
            ("flags", 4usize, 2u32),
            ("nan audio", 8, f32::NAN.to_bits()),
            ("window", 12, 99),
            ("dirty", 31, 2),
        ] {
            let mut bad = valid.clone();
            bad[offset..offset + 4].copy_from_slice(&value.to_le_bytes());
            assert_eq!(
                parse_status(&v2_frame(&tlv(FRAME_TAG_UI, &bad))),
                MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT,
                "case={name}"
            );
        }
        let mut tail = valid;
        tail.push(0);
        assert_eq!(
            parse_status(&v2_frame(&tlv(FRAME_TAG_UI, &tail))),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
    }

    /// 构造真实菜单 UI 段(四按钮 + 中文 error,与 Go EncodeUIMenu 同一语义):
    /// 142 字节,非 4 对齐(142 % 4 == 2)。用于证明 parse 层接受非对齐 UI 段。
    fn ui_segment_four_button_error() -> Vec<u8> {
        let mut out = Vec::new();
        out.extend_from_slice(&crate::ui::UI_LAYOUT_VERSION.to_le_bytes()); // layout
        out.extend_from_slice(&crate::ui::UI_FLAG_VISIBLE.to_le_bytes()); // flags visible
        out.extend_from_slice(&4u32.to_le_bytes()); // button count
        let buttons = [
            (1u32, "进入游戏", 1u32),
            (2, "多人游戏", 0),
            (3, "设置", 0),
            (4, "退出游戏", 1),
        ];
        for (id, label, enabled) in buttons {
            out.extend_from_slice(&id.to_le_bytes());
            out.extend_from_slice(&(label.len() as u32).to_le_bytes());
            out.extend_from_slice(label.as_bytes());
            out.extend_from_slice(&enabled.to_le_bytes());
        }
        let title = "Mornlea";
        out.extend_from_slice(&(title.len() as u32).to_le_bytes());
        out.extend_from_slice(title.as_bytes());
        let version = "dev";
        out.extend_from_slice(&(version.len() as u32).to_le_bytes());
        out.extend_from_slice(version.as_bytes());
        let error = "存档无法打开";
        out.extend_from_slice(&(error.len() as u32).to_le_bytes());
        out.extend_from_slice(error.as_bytes());
        out
    }

    #[test]
    fn v2_ui_segment_non_aligned_length_accepted() {
        // 真实菜单(四按钮 + 中文 error)编码为 142 字节,非 4 对齐。
        let seg = ui_segment_four_button_error();
        assert_eq!(seg.len(), 142);
        assert_eq!(seg.len() % 4, 2);
        // parse 层必须接受非对齐 UI 段:停在句柄层 WINDOW(而非 INVALID_ARGUMENT)。
        assert_eq!(
            parse_status(&v2_frame(&tlv(FRAME_TAG_UI, &seg))),
            MORNLEA_CLIENT_STATUS_WINDOW
        );
        // 非 UI 段仍拒绝非对齐长度(既有 4 对齐守卫不因豁免而放松)。
        assert_eq!(
            parse_status(&v2_frame(&tlv(FRAME_TAG_NAME_TAG, &[0u8; 6]))),
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
    }
}

/// 上传字形图集矩形(R8);越界/长度不符拒绝且不写纹理。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_upload_glyph_rect(
    abi_version: u32,
    handle: u64,
    x: u32,
    y: u32,
    width: u32,
    height: u32,
    pixels: *const u8,
    pixels_len: usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if pixels.is_null() {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            // SAFETY: pixels 非空,调用方保证 pixels_len 字节可读。
            let data = unsafe { std::slice::from_raw_parts(pixels, pixels_len) };
            if renderer.upload_glyph_rect(x, y, width, height, data) {
                MORNLEA_CLIENT_STATUS_OK
            } else {
                MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
            }
        })
    })
}

/// 上传 HUD 图集(RGBA,一次性;重复上传替换);长度不符拒绝。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_upload_hud_atlas(
    abi_version: u32,
    handle: u64,
    width: u32,
    height: u32,
    pixels: *const u8,
    pixels_len: usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if pixels.is_null() {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            // SAFETY: pixels 非空,调用方保证 pixels_len 字节可读。
            let data = unsafe { std::slice::from_raw_parts(pixels, pixels_len) };
            if renderer.upload_hud_atlas(width, height, data) {
                MORNLEA_CLIENT_STATUS_OK
            } else {
                MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
            }
        })
    })
}

#[cfg(test)]
mod atlas_ffi_tests {
    use super::*;

    #[test]
    fn atlas_entries_reject_bad_abi_arguments_and_handles() {
        let pixels = [0u8; 16];
        // SAFETY: 指针来自有效局部变量。
        let wrong_abi = unsafe {
            mornlea_client_render_upload_glyph_rect(
                CLIENT_ABI_VERSION + 1,
                1,
                0,
                0,
                4,
                4,
                pixels.as_ptr(),
                pixels.len(),
            )
        };
        assert_eq!(wrong_abi, MORNLEA_CLIENT_STATUS_ABI_VERSION);
        // SAFETY: 同上;句柄未知(校验先于纹理写入,经 with_renderer)。
        let unknown = unsafe {
            mornlea_client_render_upload_glyph_rect(
                CLIENT_ABI_VERSION,
                0xF00D,
                0,
                0,
                4,
                4,
                pixels.as_ptr(),
                pixels.len(),
            )
        };
        assert_eq!(unknown, MORNLEA_CLIENT_STATUS_WINDOW);
        // SAFETY: 同上。
        let hud_unknown = unsafe {
            mornlea_client_render_upload_hud_atlas(
                CLIENT_ABI_VERSION,
                0xF00D,
                2,
                2,
                pixels.as_ptr(),
                pixels.len(),
            )
        };
        assert_eq!(hud_unknown, MORNLEA_CLIENT_STATUS_WINDOW);
    }

    #[test]
    fn atlas_bounds_and_length_are_validated_on_live_renderer() {
        let mut handle = 0u64;
        // SAFETY: 指针来自有效局部变量。
        let create =
            unsafe { mornlea_client_render_create(CLIENT_ABI_VERSION, 16, 16, &mut handle) };
        if create == MORNLEA_CLIENT_STATUS_ADAPTER {
            return; // 无 GPU 环境跳过。
        }
        assert_eq!(create, MORNLEA_CLIENT_STATUS_OK);
        let pixels = [0u8; 16];
        // 越界矩形(x+w 超过 1024)。
        // SAFETY: 同上。
        let oob = unsafe {
            mornlea_client_render_upload_glyph_rect(
                CLIENT_ABI_VERSION,
                handle,
                1022,
                0,
                4,
                4,
                pixels.as_ptr(),
                pixels.len(),
            )
        };
        assert_eq!(oob, MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT);
        // 长度不符。
        // SAFETY: 同上。
        let short = unsafe {
            mornlea_client_render_upload_glyph_rect(
                CLIENT_ABI_VERSION,
                handle,
                0,
                0,
                4,
                4,
                pixels.as_ptr(),
                8,
            )
        };
        assert_eq!(short, MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT);
        // 合法矩形与 HUD 图集。
        // SAFETY: 同上。
        let ok = unsafe {
            mornlea_client_render_upload_glyph_rect(
                CLIENT_ABI_VERSION,
                handle,
                0,
                0,
                4,
                4,
                pixels.as_ptr(),
                pixels.len(),
            )
        };
        assert_eq!(ok, MORNLEA_CLIENT_STATUS_OK);
        // SAFETY: 同上。
        let hud = unsafe {
            mornlea_client_render_upload_hud_atlas(
                CLIENT_ABI_VERSION,
                handle,
                2,
                2,
                pixels.as_ptr(),
                pixels.len(),
            )
        };
        assert_eq!(hud, MORNLEA_CLIENT_STATUS_OK);
        assert_eq!(
            mornlea_client_render_destroy(CLIENT_ABI_VERSION, handle),
            MORNLEA_CLIENT_STATUS_OK
        );
    }
}

/// 上传/替换一个远环 tile 的壳 quad 字节流(20 字节/quad 的 LE 编码,
/// 布局与 engine `mornlea_lod_shell` 输出逐字一致;空等价 drop)。
/// 整 tile 替换语义:重复上传同 tile 即整体替换。流非法返回
/// INVALID_ARGUMENT,tile 表容量耗尽返回 CAPACITY。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_upload_lod_tile(
    abi_version: u32,
    handle: u64,
    tile_x: i32,
    tile_z: i32,
    quads: *const u8,
    quads_len: usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if !quads_len.is_multiple_of(crate::render::lod::LOD_QUAD_BYTES)
        || (quads.is_null() && quads_len != 0)
    {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            let data = if quads_len == 0 {
                &[][..]
            } else {
                // SAFETY: quads 非空,调用方保证 quads_len 字节可读。
                unsafe { std::slice::from_raw_parts(quads, quads_len) }
            };
            match renderer.upload_lod_tile((tile_x, tile_z), data) {
                Ok(()) => MORNLEA_CLIENT_STATUS_OK,
                Err(crate::render::lod::LodUploadError::Invalid) => {
                    MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
                }
                Err(crate::render::lod::LodUploadError::Capacity) => MORNLEA_CLIENT_STATUS_CAPACITY,
            }
        })
    })
}

/// 丢弃一个远环 tile;不存在时为幂等空操作。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_render_drop_lod_tile(
    abi_version: u32,
    handle: u64,
    tile_x: i32,
    tile_z: i32,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            if renderer.drop_lod_tile((tile_x, tile_z)) {
                MORNLEA_CLIENT_STATUS_OK
            } else {
                MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
            }
        })
    })
}

/// 设置远环距离雾参数(Ruling 14 参数化):`start` 起雾距离、`full`
/// 全雾距离(block)。入口校验 start > 0 且 full > start(NaN 天然被拒),
/// 违约返回 INVALID_ARGUMENT 且校验先于句柄查找(不触碰任何渲染器
/// 状态);渲染器内的默认值 768/1152 锚定 lodFarMultiplier=3 的默认
/// 几何,非默认倍率的推导接线由上层(5.2)按配置计算后调用本出口。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_render_set_lod_fog(
    abi_version: u32,
    handle: u64,
    start: f32,
    full: f32,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if !(start > 0.0 && full > start) {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            // 入口校验已通过;渲染器层同契约再校验一次(防御直连调用方),
            // 除 prepared batch 冻结外恒为 true。
            if renderer.set_lod_fog(start, full) {
                MORNLEA_CLIENT_STATUS_OK
            } else {
                MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
            }
        })
    })
}

#[cfg(test)]
mod lod_ffi_tests {
    use super::*;

    // 远环入口的无头校验:错误 ABI、非法 quad 流长度、空指针与未知句柄。
    // tile 替换语义与图像路径在 render 模块的 GPU-or-skip 测试中覆盖。

    #[test]
    fn lod_entries_reject_bad_abi_and_arguments() {
        let quads = [0u8; 20];
        // SAFETY: 指针来自有效局部变量。
        let wrong_abi = unsafe {
            mornlea_client_render_upload_lod_tile(
                CLIENT_ABI_VERSION + 1,
                0xBEEF,
                0,
                0,
                quads.as_ptr(),
                quads.len(),
            )
        };
        assert_eq!(wrong_abi, MORNLEA_CLIENT_STATUS_ABI_VERSION);
        assert_eq!(
            mornlea_client_render_drop_lod_tile(CLIENT_ABI_VERSION + 1, 0xBEEF, 0, 0),
            MORNLEA_CLIENT_STATUS_ABI_VERSION
        );

        // 长度非 20 的倍数必须在校验层拒绝。
        let odd = [0u8; 21];
        // SAFETY: 同上。
        let misaligned = unsafe {
            mornlea_client_render_upload_lod_tile(
                CLIENT_ABI_VERSION,
                0xBEEF,
                0,
                0,
                odd.as_ptr(),
                odd.len(),
            )
        };
        assert_eq!(misaligned, MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT);

        // 空指针 + 非零长度拒绝;空指针 + 零长度是合法的 drop 语义,
        // 只会因句柄未知停在 WINDOW。
        // SAFETY: 刻意的空指针,长度非零必须先于解引用被拒绝。
        let null = unsafe {
            mornlea_client_render_upload_lod_tile(
                CLIENT_ABI_VERSION,
                0xBEEF,
                0,
                0,
                std::ptr::null(),
                4,
            )
        };
        assert_eq!(null, MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT);

        // 参数本身合法时,未知句柄一律 WINDOW。
        // SAFETY: 同上。
        let unknown = unsafe {
            mornlea_client_render_upload_lod_tile(
                CLIENT_ABI_VERSION,
                0xF00D,
                0,
                0,
                quads.as_ptr(),
                quads.len(),
            )
        };
        assert_eq!(unknown, MORNLEA_CLIENT_STATUS_WINDOW);
        assert_eq!(
            mornlea_client_render_drop_lod_tile(CLIENT_ABI_VERSION, 0xF00D, 0, 0),
            MORNLEA_CLIENT_STATUS_WINDOW
        );
    }

    /// 雾参数化 setter(Ruling 14)的无头校验矩阵:入口校验(start > 0、
    /// full > start,NaN 与任一违约都拒绝)必须先于句柄查找——非法参数
    /// 配未知句柄仍报 INVALID_ARGUMENT;参数合法才因句柄未知停在 WINDOW。
    #[test]
    fn set_lod_fog_validates_arguments_before_handle_lookup() {
        // 错误 ABI 优先于一切参数校验。
        assert_eq!(
            mornlea_client_render_set_lod_fog(CLIENT_ABI_VERSION + 1, 0xF00D, 768.0, 1152.0),
            MORNLEA_CLIENT_STATUS_ABI_VERSION
        );
        for (name, start, full) in [
            ("start 为零", 0.0, 100.0),
            ("start 为负", -1.0, 100.0),
            ("start 为 NaN", f32::NAN, 100.0),
            ("full 等于 start", 100.0, 100.0),
            ("full 小于 start", 200.0, 100.0),
            ("full 为 NaN", 100.0, f32::NAN),
        ] {
            assert_eq!(
                mornlea_client_render_set_lod_fog(CLIENT_ABI_VERSION, 0xF00D, start, full),
                MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT,
                "{name}"
            );
        }
        // 参数合法 + 未知句柄 → WINDOW,证明校验通过后到达句柄查找。
        assert_eq!(
            mornlea_client_render_set_lod_fog(CLIENT_ABI_VERSION, 0xF00D, 768.0, 1152.0),
            MORNLEA_CLIENT_STATUS_WINDOW
        );
    }
}

/// 创建窗口模式渲染器:`window_handle` 必须是本线程窗口表中的有效句柄
/// (winit 窗口与其表同线程);渲染器句柄写入 `out_handle` 并存入全局表。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_create_windowed(
    abi_version: u32,
    window_handle: u64,
    out_handle: *mut u64,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if out_handle.is_null() {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        let (window, width, height) = match WINDOWS.with(|windows| {
            windows.borrow().get(&window_handle).and_then(|entry| {
                entry.shared_window().map(|window| {
                    let size = window.inner_size();
                    (window, size.width.max(1), size.height.max(1))
                })
            })
        }) {
            Some(parts) => parts,
            None => return MORNLEA_CLIENT_STATUS_WINDOW,
        };
        let renderer = match OffscreenRenderer::new_windowed(window, width, height) {
            Ok(renderer) => renderer,
            Err(RenderCreateError::Adapter) => return MORNLEA_CLIENT_STATUS_ADAPTER,
            Err(RenderCreateError::Device) => return MORNLEA_CLIENT_STATUS_WINDOW,
        };
        let handle = NEXT_RENDER_HANDLE.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        RENDERERS
            .lock()
            .expect("渲染器表锁中毒")
            .get_or_insert_with(HashMap::new)
            .insert(handle, renderer);
        // SAFETY: out_handle 已判非空,只在完整成功后写一次。
        unsafe { out_handle.write(handle) };
        MORNLEA_CLIENT_STATUS_OK
    })
}

/// 调整渲染器输出尺寸:重建 color/depth/HiZ,窗口模式重配 surface。
#[unsafe(no_mangle)]
pub extern "C" fn mornlea_client_render_resize(
    abi_version: u32,
    handle: u64,
    width: u32,
    height: u32,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if width == 0 || height == 0 {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            if renderer.resize(width, height) {
                MORNLEA_CLIENT_STATUS_OK
            } else {
                MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
            }
        })
    })
}
/// 上传 egui 菜单字体(client ABI v9 保留出口):字节负载须非空且 <= 32 MiB。
///
/// 入口校验(空指针、零长度 -> INVALID_ARGUMENT;超 32 MiB -> CAPACITY)
/// 先于句柄查找(与 set_lod_fog 同一约定,无头可测);成功则安装到
/// EguiPass 的 UiState(proportional + monospace 同族)。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_upload_ui_font(
    abi_version: u32,
    handle: u64,
    bytes: *const u8,
    len: usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if bytes.is_null() || len == 0 {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    if len > crate::render::egui::MAX_UI_FONT_BYTES {
        return MORNLEA_CLIENT_STATUS_CAPACITY;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            // SAFETY: bytes 非空,调用方保证 len 字节可读。
            let data = unsafe { std::slice::from_raw_parts(bytes, len) };
            match renderer.upload_ui_font(data) {
                Ok(()) => MORNLEA_CLIENT_STATUS_OK,
                Err(_) => MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT,
            }
        })
    })
}

/// 排空 client ABI v9 结构化 UI 事件：只有完整 batch 能装入 `out` 时才
/// 写入并清空队列，把实际字节数写入 `*out_written`。容量不足返回
/// `MORNLEA_CLIENT_STATUS_CAPACITY`，三个对象均保持不变。
///
/// 入口校验(ABI 版本、`out` 与 `out_written` 非空)先于句柄查找；校验失败
/// 不触碰调用方对象。空队列仍输出 8 字节合法空 batch。
#[unsafe(no_mangle)]
pub unsafe extern "C" fn mornlea_client_render_drain_ui_events(
    abi_version: u32,
    handle: u64,
    out: *mut u8,
    out_len: usize,
    out_written: *mut usize,
) -> u32 {
    if abi_version != CLIENT_ABI_VERSION {
        return MORNLEA_CLIENT_STATUS_ABI_VERSION;
    }
    if out.is_null() || out_written.is_null() {
        return MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT;
    }
    catch(|| {
        with_renderer(handle, |renderer| {
            // SAFETY: out 非空，调用方保证 `out_len` 字节可写。
            let out = unsafe { std::slice::from_raw_parts_mut(out, out_len) };
            finish_ui_event_drain(renderer.drain_ui_events(out), out_written)
        })
    })
}

/// 把排空结果映射为 FFI 状态；只有成功才写 `out_written`，失败路径保持调用
/// 方 marker 不变。调用方必须先保证指针非空且可写。
fn finish_ui_event_drain(
    result: Result<usize, crate::ui::UiOutputError>,
    out_written: *mut usize,
) -> u32 {
    match result {
        Ok(written) => {
            // SAFETY:出口已校验指针；纯函数测试同样传入有效局部变量。
            unsafe { out_written.write(written) };
            MORNLEA_CLIENT_STATUS_OK
        }
        Err(crate::ui::UiOutputError::Capacity) => MORNLEA_CLIENT_STATUS_CAPACITY,
        Err(crate::ui::UiOutputError::Invalid) => MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT,
    }
}

#[cfg(test)]
mod ui_ffi_tests {
    use super::*;

    // 菜单两条出口(client ABI v9)的无头校验:错误 ABI、非法参数先于句柄查找。

    #[test]
    fn upload_ui_font_rejects_bad_arguments_before_handle_lookup() {
        // 错误 ABI 优先。
        assert_eq!(
            unsafe {
                mornlea_client_render_upload_ui_font(
                    CLIENT_ABI_VERSION + 1,
                    0xF00D,
                    std::ptr::null(),
                    0,
                )
            },
            MORNLEA_CLIENT_STATUS_ABI_VERSION
        );
        // 空指针 -> INVALID_ARGUMENT(先于句柄查找)。
        assert_eq!(
            unsafe {
                mornlea_client_render_upload_ui_font(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    std::ptr::null(),
                    4,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // 零长度 -> INVALID_ARGUMENT。
        let dummy = [0u8; 4];
        assert_eq!(
            unsafe {
                mornlea_client_render_upload_ui_font(CLIENT_ABI_VERSION, 0xF00D, dummy.as_ptr(), 0)
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // 超 32 MiB -> CAPACITY。
        assert_eq!(
            unsafe {
                mornlea_client_render_upload_ui_font(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    dummy.as_ptr(),
                    crate::render::egui::MAX_UI_FONT_BYTES + 1,
                )
            },
            MORNLEA_CLIENT_STATUS_CAPACITY
        );
    }

    #[test]
    fn drain_ui_events_rejects_bad_arguments_before_handle_lookup() {
        // 错误 ABI 优先。
        assert_eq!(
            unsafe {
                mornlea_client_render_drain_ui_events(
                    CLIENT_ABI_VERSION + 1,
                    0xF00D,
                    std::ptr::null_mut(),
                    0,
                    std::ptr::null_mut(),
                )
            },
            MORNLEA_CLIENT_STATUS_ABI_VERSION
        );
        // out 为 null -> INVALID_ARGUMENT(先于句柄查找),且不触碰 out_written。
        let mut marker = 0xDEADBEEFusize;
        assert_eq!(
            unsafe {
                mornlea_client_render_drain_ui_events(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    std::ptr::null_mut(),
                    8,
                    &mut marker,
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        assert_eq!(marker, 0xDEADBEEF);
        // 任意字节长度都合法；参数合法但句柄未知 -> WINDOW，输出对象不变。
        let mut out = [0u8; 8];
        marker = 0xDEADBEEFusize;
        assert_eq!(
            unsafe {
                mornlea_client_render_drain_ui_events(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    out.as_mut_ptr(),
                    7,
                    &mut marker,
                )
            },
            MORNLEA_CLIENT_STATUS_WINDOW
        );
        assert_eq!(marker, 0xDEADBEEF);
        // out_written 为 null -> INVALID_ARGUMENT。
        assert_eq!(
            unsafe {
                mornlea_client_render_drain_ui_events(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    out.as_mut_ptr(),
                    8,
                    std::ptr::null_mut(),
                )
            },
            MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT
        );
        // 参数全部合法但句柄未知 -> WINDOW,不触碰 out 与 out_written。
        let mut out = [0xAAu8; 8];
        let mut written = 0xBBBBBBBBusize;
        assert_eq!(
            unsafe {
                mornlea_client_render_drain_ui_events(
                    CLIENT_ABI_VERSION,
                    0xF00D,
                    out.as_mut_ptr(),
                    8,
                    &mut written,
                )
            },
            MORNLEA_CLIENT_STATUS_WINDOW
        );
        assert!(out.iter().all(|&b| b == 0xAA));
        assert_eq!(written, 0xBBBBBBBB);
    }

    #[test]
    fn drain_capacity_does_not_write_out_written() {
        let mut marker = 0xA5A5A5A5usize;
        assert_eq!(
            finish_ui_event_drain(Err(crate::ui::UiOutputError::Capacity), &mut marker),
            MORNLEA_CLIENT_STATUS_CAPACITY
        );
        assert_eq!(marker, 0xA5A5A5A5);
        assert_eq!(
            finish_ui_event_drain(Ok(37), &mut marker),
            MORNLEA_CLIENT_STATUS_OK
        );
        assert_eq!(marker, 37);
    }
}
