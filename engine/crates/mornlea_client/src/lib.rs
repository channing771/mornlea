//! mornlea_client:darwin 客户端的窗口与事件循环库(R1)。
//!
//! 本 crate 以 winit 独占客户端窗口与输入采集的生产实现,通过独立 C ABI
//! (版本见 `mornlea_client_abi_version`,当前 v14,与 `mornlea_engine` ABI
//! 互不耦合)供 Go `internal/client` 调用。client ABI v14 在 v13 窗口合成
//! 捕获出口 `mornlea_client_window_capture`(见 [`capture`] 模块)上新增
//! render world update 入口；v12
//! 退役菜单字体上传出口与帧 tag 9 UI 段,新增 `mornlea_client_ui_push_state`
//! 菜单状态下行出口;事件排空出口格式改为版本化 JSON 信封。
//! 控制权保持在 Go 主线程:每帧一次
//! `mornlea_client_window_poll` 以零超时 `pump_app_events` 驱动事件循环,
//! 并返回固定布局的输入快照(见 [`input`] 模块的布局说明)。
//!
//! 平台约束:窗口栈只在 `target_os = "macos"` 生产;其他平台本 crate 编译
//! 为空库,保证 Linux 专服 workspace 构建不引入任何窗口依赖。

#[cfg(target_os = "macos")]
pub mod bridge;
#[cfg(target_os = "macos")]
mod capture;
#[cfg(target_os = "macos")]
pub mod input;
#[cfg(target_os = "macos")]
pub mod render;
#[cfg(target_os = "macos")]
pub mod webview;
#[cfg(target_os = "macos")]
mod window;

#[cfg(target_os = "macos")]
mod ffi;
