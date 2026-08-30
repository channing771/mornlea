//! 窗口合成捕获(darwin 专属,client ABI v13 出口的原语层)。
//!
//! 链路:`NSWindow windowNumber`(见 [`crate::window::ClientWindow::window_number`])
//! → `CGWindowListCreateImage`(best resolution,一次取回窗口完整合成画面:
//! 世界 + wgpu HUD + WKWebView 菜单层)→ `CGBitmapContextCreate`(BGRA8:
//! premultiplied first + 32-bit little endian,内存字节序与离屏 readback 的
//! `Bgra8Unorm` 逐字一致)→ `CGContextDrawImage` → 逐行拷出紧凑字节。
//!
//! 两个实现要点:
//! - CG 位图上下文原点在左下(内存第 0 行是画面底行),行尾可能带对齐
//!   padding;拷贝必须逐行、反序进行,输出与既有 readback 的自上而下行序
//!   一致,Go 消费侧直接做 BGRA8→NRGBA 字节序交换即可编码,无需再翻转;
//! - `CGWindowListCreateImage` 已被 Apple 标记弃用(后继方案
//!   ScreenCaptureKit 记录在 dev-capture change 的 design 中),当前 macOS
//!   仍全量可用。弃用面与 CoreGraphics C 绑定集中封装在本模块,未来替换
//!   实现时 FFI 契约与 Go 侧不动。
//!
//! 线程约束:必须在窗口 poll 线程调用(与 `ClientWindow` 的线程约束一致),
//! 由 FFI 层的 thread-local 句柄表兜底。
//!
//! 依赖取舍:CoreGraphics 是 C 框架而非 ObjC 类,objc2 生态在既有依赖树里
//! 没有现成绑定;本模块只手写用到的少数函数声明(`unsafe extern "C"`),
//! 签名与系统头逐字对齐,不引入新的 crate 依赖。

// 本模块的 extern 绑定为手写声明,编译器不会因弃用 API 再生警告;这里仍
// 显式局部化 `allow(deprecated)`,声明弃用面(见模块文档)不允许扩散出
// 本模块。
#![allow(deprecated)]

use std::os::raw::c_void;

/// CG 不透明句柄:`CGImageRef`/`CGColorSpaceRef`/`CGContextRef` 的统一形态。
/// 句柄全部由本模块创建,经 [`AutoRelease`] 成对释放,不跨模块流转。
type CGHandle = *mut c_void;

/// CoreGraphics 二维点(逻辑单位;本模块仅用于绘制矩形与空矩形哨兵)。
#[derive(Clone, Copy)]
#[repr(C)]
struct CGPoint {
    x: f64,
    y: f64,
}

/// CoreGraphics 二维尺寸。
#[derive(Clone, Copy)]
#[repr(C)]
struct CGSize {
    width: f64,
    height: f64,
}

/// CoreGraphics 矩形(按值传给 C,`repr(C)` 保证与系统 `CGRect` 同布局)。
#[derive(Clone, Copy)]
#[repr(C)]
struct CGRect {
    origin: CGPoint,
    size: CGSize,
}

/// `CGRectNull` 哨兵(系统头定义为无穷远原点、零尺寸):传给
/// `CGWindowListCreateImage` 表示抓取整个窗口,而非屏幕上的某个矩形。
const CG_RECT_NULL: CGRect = CGRect {
    origin: CGPoint {
        x: f64::INFINITY,
        y: f64::INFINITY,
    },
    size: CGSize {
        width: 0.0,
        height: 0.0,
    },
};

/// `kCGWindowListOptionIncludingWindow`(SDK `CGWindow.h`,`CF_OPTIONS(uint32_t,
/// CGWindowListOption)`):把 `relativeToWindow` 指定的窗口纳入合成列表,配合
/// `CGRectNull` 即只抓目标窗口自身,不把遮挡其上的其他窗口叠进合成图。
/// 位值 1 << 3 由本机 SDK 头钉死,勿凭记忆改写(1 << 0 是
/// `kCGWindowListOptionOnScreenOnly`,语义要求 `kCGNullWindowID`,与传入真实
/// 窗口号的用法冲突)。
const CG_WINDOW_LIST_OPTION_INCLUDING_WINDOW: u32 = 1 << 3;
/// `kCGWindowImageBestResolution`(SDK `CGWindow.h`,`CF_OPTIONS(uint32_t,
/// CGWindowImageOption)`):按 backing scale(Retina 为 2x)输出物理像素尺寸,
/// 与 wgpu framebuffer 的尺寸来源一致。位值 1 << 3 同样以 SDK 头为准
/// (1 << 4 是 `kCGWindowImageNominalResolution`)。
const CG_WINDOW_IMAGE_BEST_RESOLUTION: u32 = 1 << 3;
/// `kCGImageAlphaPremultipliedFirst`:alpha 位于「首」分量,配合 32 位小端
/// 字节序即内存 B,G,R,A —— 与 `Bgra8Unorm` readback 逐字一致。
const CG_IMAGE_ALPHA_PREMULTIPLIED_FIRST: u32 = 1 << 1;
/// `kCGBitmapByteOrder32Little`:32 位像素按小端存储。
const CG_BITMAP_BYTE_ORDER_32_LITTLE: u32 = 2 << 12;

/// `CGBitmapContextCreate` 的位图布局参数:premultiplied first + 小端,
/// 即 BGRA8。
const BITMAP_INFO: u32 = CG_IMAGE_ALPHA_PREMULTIPLIED_FIRST | CG_BITMAP_BYTE_ORDER_32_LITTLE;
/// 每像素字节数(BGRA8)。
const BYTES_PER_PIXEL: usize = 4;
/// 位图行距按 64 字节对齐:CG 官方建议对齐行距以获得更快的绘制拷贝路径,
/// 同时让「按行紧凑拷贝」成为必须而非可省略(行尾 padding 不进输出)。
const STRIDE_ALIGNMENT: usize = 64;
/// 合成图单边硬上界(物理像素)。正常窗口合成图受物理屏幕约束,此上界只
/// 防御异常系统值把分配打爆,不约束正常路径;需覆盖当前主流显示器(6K 级)
/// 的物理宽度。
const MAX_CAPTURE_DIMENSION: usize = 16384;

// CoreGraphics 位图与窗口列表函数;darwin 平台框架恒在。
//
// SAFETY(整体):以下签名与系统 `CoreGraphics` 头逐字对齐——
// `CGWindowListOption`/`CGWindowImageOption` 都是系统头里的
// `CF_OPTIONS(uint32_t, ...)` 枚举,按 `u32` 传递;`CGBitmapInfo` 是
// `uint32_t`,按 `u32` 传递。宽度错一位即是未定义行为。
#[link(name = "CoreGraphics", kind = "framework")]
unsafe extern "C" {
    /// 抓取窗口合成图;Apple 已弃用(后继方案见模块文档),弃用面集中在本
    /// 模块。失败返回 NULL。
    fn CGWindowListCreateImage(
        screen_bounds: CGRect,
        list_option: u32,
        window_id: u32,
        image_option: u32,
    ) -> CGHandle;
    /// 创建指定位图布局的绘制上下文;`data` 为调用方提供的行缓冲,失败
    /// 返回 NULL。
    fn CGBitmapContextCreate(
        data: *mut c_void,
        width: usize,
        height: usize,
        bits_per_component: usize,
        bytes_per_row: usize,
        space: CGHandle,
        bitmap_info: u32,
    ) -> CGHandle;
    /// 把图像绘制进上下文的给定矩形(上下文坐标,原点左下)。
    fn CGContextDrawImage(context: CGHandle, rect: CGRect, image: CGHandle);
    /// 合成图物理宽度(像素)。
    fn CGImageGetWidth(image: CGHandle) -> usize;
    /// 合成图物理高度(像素)。
    fn CGImageGetHeight(image: CGHandle) -> usize;
    /// 创建设备 RGB 色彩空间;失败返回 NULL。
    fn CGColorSpaceCreateDeviceRGB() -> CGHandle;
}

// CoreFoundation 基础对象释放;CG 句柄经此成对释放,防止本模块泄漏。
#[link(name = "CoreFoundation", kind = "framework")]
unsafe extern "C" {
    fn CFRelease(cf: *const c_void);
}

/// RAII 守卫:Drop 时恰好 `CFRelease` 一次。字段顺序决定释放顺序(逆序
/// Drop),调用方保证 `raw` 行缓冲声明在 context 守卫之前。
struct AutoRelease(CGHandle);

impl Drop for AutoRelease {
    fn drop(&mut self) {
        // SAFETY: 句柄由本模块的 CG 创建函数取得,Drop 保证恰好释放一次,
        // 不存在二次释放或悬垂借用。
        unsafe { CFRelease(self.0) };
    }
}

/// 窗口合成捕获失败:只涵盖运行期预期条件(窗口号无效、系统返回空图、
/// 位图上下文创建失败等)。参数违约在 FFI 层拦截,不经本类型。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CaptureUnavailable;

/// 一次窗口合成捕获的紧凑输出:`pixels` 为自上而下、无行 padding 的
/// BGRA8 字节,长度恰为 `width * height * 4`。
pub struct CapturedFrame {
    pub pixels: Vec<u8>,
    pub width: u32,
    pub height: u32,
}

/// 抓取指定窗口号的完整合成画面,输出自上而下、紧凑无 padding 的 BGRA8
/// 字节。`window_number` 取不到或任一系统步骤失败时返回
/// [`CaptureUnavailable`],不产生部分结果。
pub fn capture_window(window_number: i64) -> Result<CapturedFrame, CaptureUnavailable> {
    // CGWindowID 是 u32 域;越界或零(窗口不在任何 screen 上)按不可用处理。
    if window_number <= 0 || window_number > u32::MAX as i64 {
        return Err(CaptureUnavailable);
    }
    // SAFETY: 纯查询调用,窗口服务器对无效窗口号返回 NULL 而非未定义行为;
    // 返回的 CGImage 由 AutoRelease 成对释放。
    let image = AutoRelease(unsafe {
        CGWindowListCreateImage(
            CG_RECT_NULL,
            CG_WINDOW_LIST_OPTION_INCLUDING_WINDOW,
            window_number as u32,
            CG_WINDOW_IMAGE_BEST_RESOLUTION,
        )
    });
    if image.0.is_null() {
        return Err(CaptureUnavailable);
    }
    // SAFETY: image 是上方刚创建并验非空的有效 CGImage。
    let (width, height) = unsafe { (CGImageGetWidth(image.0), CGImageGetHeight(image.0)) };
    if width == 0 || height == 0 || width > MAX_CAPTURE_DIMENSION || height > MAX_CAPTURE_DIMENSION
    {
        return Err(CaptureUnavailable);
    }
    let Some(pixels_len) = width
        .checked_mul(height)
        .and_then(|pixels| pixels.checked_mul(BYTES_PER_PIXEL))
    else {
        return Err(CaptureUnavailable);
    };
    // SAFETY: 设备 RGB 色彩空间为常驻对象,失败返回 NULL;句柄由
    // AutoRelease 成对释放。
    let color_space = AutoRelease(unsafe { CGColorSpaceCreateDeviceRGB() });
    if color_space.0.is_null() {
        return Err(CaptureUnavailable);
    }
    let width_bytes = width * BYTES_PER_PIXEL;
    let Some(stride) = align_up(width_bytes, STRIDE_ALIGNMENT) else {
        return Err(CaptureUnavailable);
    };
    let Some(raw_len) = stride.checked_mul(height) else {
        return Err(CaptureUnavailable);
    };
    // 行缓冲声明在 context 守卫之前:Drop 逆序执行,context 先于行缓冲
    // 释放,绘制上下文不会悬垂引用已回收内存。
    let mut raw = vec![0u8; raw_len];
    // SAFETY: raw 覆盖 stride×height 字节且后于 context 释放;8 bpp +
    // premultiplied first + 32-bit little endian 是文档保证支持的组合,
    // 行距不小于 width_bytes。
    let context = AutoRelease(unsafe {
        CGBitmapContextCreate(
            raw.as_mut_ptr().cast::<c_void>(),
            width,
            height,
            8,
            stride,
            color_space.0,
            BITMAP_INFO,
        )
    });
    if context.0.is_null() {
        return Err(CaptureUnavailable);
    }
    // SAFETY: image 与 context 均为本函数创建的有效句柄,绘制矩形铺满
    // 整个上下文。
    unsafe { CGContextDrawImage(context.0, full_rect(width, height), image.0) };
    // CG 上下文原点在左下,内存第 0 行是画面底行;逐行反序拷出同时剥离
    // 行距 padding,输出与 readback 的自上而下行序一致。
    let mut pixels = vec![0u8; pixels_len];
    if !copy_rows_top_down(&raw, stride, width_bytes, height, &mut pixels) {
        return Err(CaptureUnavailable);
    }
    Ok(CapturedFrame {
        pixels,
        width: width as u32,
        height: height as u32,
    })
}

/// 铺满整个上下文的绘制矩形。
fn full_rect(width: usize, height: usize) -> CGRect {
    CGRect {
        origin: CGPoint { x: 0.0, y: 0.0 },
        size: CGSize {
            width: width as f64,
            height: height as f64,
        },
    }
}

/// 把值向上对齐到 `align`(`align` 必须是 2 的幂);溢出返回 None。
fn align_up(value: usize, align: usize) -> Option<usize> {
    debug_assert!(align.is_power_of_two());
    value
        .checked_add(align - 1)
        .map(|padded| padded & !(align - 1))
}

/// 把 CG 位图缓冲(行距 `stride`、原点左下)拷成自上而下、无 padding 的
/// 紧凑输出:输出第 `row` 行取自源缓冲第 `height-1-row` 行的前
/// `width_bytes` 字节。校验先行,任一入参不一致(源缓冲覆盖不满、`dst`
/// 长度不符、行距计算溢出)即返回 false 且不写 `dst`,保证调用方拿到的
/// 失败不携带部分输出。
fn copy_rows_top_down(
    src: &[u8],
    stride: usize,
    width_bytes: usize,
    height: usize,
    dst: &mut [u8],
) -> bool {
    if stride < width_bytes {
        return false;
    }
    let Some(required) = width_bytes.checked_mul(height) else {
        return false;
    };
    if dst.len() != required {
        return false;
    }
    if height == 0 {
        return dst.is_empty();
    }
    let Some(last_row_start) = (height - 1).checked_mul(stride) else {
        return false;
    };
    if src.len() < last_row_start + width_bytes {
        return false;
    }
    for row in 0..height {
        let src_start = (height - 1 - row) * stride;
        dst[row * width_bytes..(row + 1) * width_bytes]
            .copy_from_slice(&src[src_start..src_start + width_bytes]);
    }
    true
}

#[cfg(test)]
mod tests {
    use super::{
        BITMAP_INFO, CG_BITMAP_BYTE_ORDER_32_LITTLE, CG_IMAGE_ALPHA_PREMULTIPLIED_FIRST,
        CG_WINDOW_IMAGE_BEST_RESOLUTION, CG_WINDOW_LIST_OPTION_INCLUDING_WINDOW,
        MAX_CAPTURE_DIMENSION, align_up, copy_rows_top_down,
    };

    /// 手抄的 CG 枚举位值必须与本机 SDK 头(`CGWindow.h`/`CGImage.h`)逐位
    /// 一致:窗口列表与图像分辨率两个枚举都是 `CF_OPTIONS(uint32_t, ...)`,
    /// 位值错一位会把「抓目标窗口 best resolution」静默变成「截全屏 nominal
    /// 分辨率」这类语义错误,且无窗测试无法暴露,只能靠钉值防回退。
    #[test]
    fn window_list_option_values_match_sdk_header() {
        // kCGWindowListOptionIncludingWindow = (1 << 3)。
        assert_eq!(CG_WINDOW_LIST_OPTION_INCLUDING_WINDOW, 1 << 3);
        // kCGWindowImageBestResolution = (1 << 3);1 << 4 是 nominal 分辨率。
        assert_eq!(CG_WINDOW_IMAGE_BEST_RESOLUTION, 1 << 3);
        // 位图布局同样手抄自系统头:kCGImageAlphaPremultipliedFirst = (1 << 1),
        // kCGBitmapByteOrder32Little = (2 << 12),组合即 BGRA8。
        assert_eq!(CG_IMAGE_ALPHA_PREMULTIPLIED_FIRST, 1 << 1);
        assert_eq!(CG_BITMAP_BYTE_ORDER_32_LITTLE, 2 << 12);
        assert_eq!(BITMAP_INFO, (1 << 1) | (2 << 12));
    }

    /// 逐行反序拷贝:剥离行尾 padding,同时把左下原点的 CG 行序翻转为
    /// 自上而下。
    #[test]
    fn copy_rows_top_down_strips_padding_and_flips() {
        // 3 行 × 每行 4 有效字节,行距 8(每行行尾 4 字节 padding)。
        let mut src = vec![0u8; 24];
        for row in 0..3usize {
            for col in 0..4usize {
                src[row * 8 + col] = (row * 16 + col + 1) as u8;
            }
        }
        let mut dst = vec![0u8; 12];
        assert!(copy_rows_top_down(&src, 8, 4, 3, &mut dst));
        // 输出第 0 行是画面顶行,对应源缓冲最后一行;padding 不进输出。
        assert_eq!(&dst[0..4], &src[16..20]);
        assert_eq!(&dst[4..8], &src[8..12]);
        assert_eq!(&dst[8..12], &src[0..4]);
    }

    /// 源缓冲覆盖不满(末行被截断)必须整体拒绝,不产生部分输出。
    #[test]
    fn copy_rows_top_down_rejects_short_source_without_partial_writes() {
        // 行距 8 × 2 行至少需要 12 字节,刻意短 1 字节。
        let src = vec![0x11u8; 11];
        let mut dst = vec![0xAAu8; 8];
        assert!(!copy_rows_top_down(&src, 8, 4, 2, &mut dst));
        assert!(dst.iter().all(|&byte| byte == 0xAA), "失败不得写 dst");
    }

    /// 输出缓冲长度与 width_bytes×height 不符必须拒绝。
    #[test]
    fn copy_rows_top_down_rejects_mismatched_dst() {
        let src = vec![0u8; 16];
        let mut short = [0u8; 7];
        let mut long = [0u8; 9];
        assert!(!copy_rows_top_down(&src, 8, 4, 2, &mut short));
        assert!(!copy_rows_top_down(&src, 8, 4, 2, &mut long));
    }

    /// 行距小于有效宽度是调用方违约,必须拒绝。
    #[test]
    fn copy_rows_top_down_rejects_stride_below_width_bytes() {
        let src = vec![0u8; 8];
        let mut dst = [0u8; 8];
        assert!(!copy_rows_top_down(&src, 3, 4, 2, &mut dst));
    }

    /// 对齐助手:不足则补齐,已对齐原样返回,溢出显式失败。
    #[test]
    fn align_up_rounds_to_next_multiple() {
        assert_eq!(align_up(1, 64), Some(64));
        assert_eq!(align_up(64, 64), Some(64));
        assert_eq!(align_up(65, 64), Some(128));
        assert_eq!(align_up(usize::MAX, 64), None);
    }

    /// 维度上限只防御异常系统值,必须容得下当前主流显示器的物理宽度,
    /// 避免把正常全屏捕获误判为不可用。
    #[test]
    fn max_capture_dimension_covers_common_displays() {
        // 常见 4K/5K/6K 显示器的物理宽度;经运行期变量断言,上限收缩到
        // 任何一档之下都应红灯。
        for display_width in [3840usize, 5120, 6016] {
            assert!(MAX_CAPTURE_DIMENSION >= display_width);
        }
    }
}
