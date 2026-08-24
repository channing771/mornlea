//! egui 即时模式菜单的 GPU 呈现半部(Task 3)。
//!
//! 本模块把 crate::ui 的纯 CPU 布局/输入翻译接到 wgpu 渲染管线上:
//! 持有 egui_wgpu::Renderer,写入一帧 egui::FullOutput 的纹理增量与
//! 三角网格,并在主命令编码器上录制一个 screen-space 的 egui pass。菜单的
//! **语义**(相位、按钮 id/禁用、文本)完全留在 Go,本模块只负责呈现与输入。
//!
//! 关键设计(与 design.md 的 0.35.0 实测一致):
//! - 纹理表**不归本模块管**:egui-wgpu 0.35 的 egui_wgpu::Renderer 内部
//!   持有纹理表,经 Renderer::update_texture 与 Renderer::free_texture
//!   维护,本模块不自行创建 wgpu 纹理;
//! - 字体经 EguiPass::upload_font 一次性上传(Go embed 的 Noto CJK OTF,
//!   上限 32 MiB),Rust 侧不内嵌字体二进制;
//! - 无字体或无 UI 段时 EguiPass::run_and_record 返回 Ok(false)(零工作),
//!   不产生任何 GPU 提交;
//! - dithering 关闭以固定 capture golden 像素。GPU 路径只编译 + capture 验收,
//!   不进自动测试(真实窗口/GPU 属 Task 6 的 capture 场景)。
//!
//! 约束:不引入 egui-winit;egui/egui-wgpu 钉死 0.35(声明 wgpu ^29,与仓库
//! 直接 wgpu 29 同线);不升级 wgpu。

use crate::ui::{UiEvent, UiFrame, UiState};
use egui::{Rect, pos2, vec2};

/// 字体一次上传的字节上界(32 MiB;实际 Noto CJK OTF 约 16 MiB,留两倍余量)。
pub const MAX_UI_FONT_BYTES: usize = 32 * 1024 * 1024;

/// 本模块的错误;FFI 层据此映射稳定状态码。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EguiError {
    /// 字体负载非法:空字节或超过 MAX_UI_FONT_BYTES。
    FontInvalid,
}

/// egui 的 GPU 呈现 pass:渲染器、屏幕描述、egui 状态与已上传字体。
pub struct EguiPass {
    /// egui-wgpu 渲染器;纹理表由它内部管理(update/free 维护)。
    renderer: egui_wgpu::Renderer,
    /// 屏幕描述:物理像素尺寸 + 每逻辑点像素(缩放)。size_in_pixels 是
    /// 物理像素,pixels_per_point 是窗口 scale factor(离屏固定 1.0)。
    screen: egui_wgpu::ScreenDescriptor,
    /// egui 上下文与点击事件队列(纯状态,无 GPU)。
    ui: UiState,
    /// 已上传的字体字节;Some 表示字体已安装(见 EguiPass::has_font)。
    ///
    /// 与 UiState 的 font_loaded 一致,本字段单独保留一份字节副本,
    /// 便于可选的重复安装;字体安装本身由 UiState::install_font 完成。
    font: Option<Vec<u8>>,
}

impl EguiPass {
    /// 创建一个 egui 渲染 pass。
    ///
    /// * color_format 是输出附件的颜色格式(与 crate::render::COLOR_FORMAT
    ///   一致,均为 sRGB);RendererOptions 用结构体构造:msaa 关闭(egui 自
    ///   带 feathering)、无 depth/stencil、dithering 关闭(capture golden 确定)。
    /// * pixels_per_point 初始缩放;窗口模式取窗口 scale factor,离屏固定 1.0。
    pub fn new(
        device: &wgpu::Device,
        color_format: wgpu::TextureFormat,
        width: u32,
        height: u32,
        pixels_per_point: f32,
    ) -> Self {
        let renderer = egui_wgpu::Renderer::new(
            device,
            color_format,
            egui_wgpu::RendererOptions {
                msaa_samples: 1,
                depth_stencil_format: None,
                dithering: false,
                // 预测性纹理过滤在 capture 里也可用,但保持默认(false)
                // 与原路径一致;如未来 golden 需要再按 kittest 先例开启。
                ..Default::default()
            },
        );
        let screen = egui_wgpu::ScreenDescriptor {
            size_in_pixels: [width, height],
            pixels_per_point,
        };
        Self {
            renderer,
            screen,
            ui: UiState::new(),
            font: None,
        }
    }

    /// 调整输出尺寸与像素密度(resize 路径调用)。
    ///
    /// pixels_per_point 在窗口模式取窗口 scale factor,离屏固定 1.0。
    pub fn set_size(&mut self, width: u32, height: u32, pixels_per_point: f32) {
        self.screen.size_in_pixels = [width, height];
        self.screen.pixels_per_point = pixels_per_point;
    }

    /// 是否已安装字体(有字体的 UI 帧才可绘制)。
    pub fn has_font(&self) -> bool {
        self.font.is_some() || self.ui.has_font()
    }

    /// 上传字体字节到 UiState 并记住已安装。
    ///
    /// 空字节或超过 MAX_UI_FONT_BYTES 返回 EguiError::FontInvalid;
    /// 成功则 UiState::install_font 安装(proportional+monospace 同族)。
    /// 重复上传同一字体是幂等(egui 按内容比较)。
    pub fn upload_font(&mut self, bytes: &[u8]) -> Result<(), EguiError> {
        if bytes.is_empty() || bytes.len() > MAX_UI_FONT_BYTES {
            return Err(EguiError::FontInvalid);
        }
        self.ui.install_font(bytes);
        self.font = Some(bytes.to_vec());
        Ok(())
    }

    /// 排空点击事件队列(读前清空),返回按钮 id 序列。
    pub fn drain_events(&mut self) -> Vec<u32> {
        self.ui.drain_events()
    }

    /// 运行一帧菜单并在 encoder 上录制 egui pass。
    ///
    /// 输入事件 events 来自 crate::ui::take_ui_events(本帧已取走)。
    /// 返回 Ok(true) 表示已录制 pass;Ok(false) 表示零工作(菜单不可见或
    /// 未装字体;调用方应已先判过,此为防御性返回);Err 表示输入/字体违约。
    ///
    /// 帧序:本 pass 排在整个帧的**最上层**(HUD/debug 之后),与 HUD 一致的
    /// screen-space 语义,无深度缓冲(feathering 自灭锯齿)。
    #[allow(clippy::too_many_arguments)]
    pub fn run_and_record(
        &mut self,
        device: &wgpu::Device,
        queue: &wgpu::Queue,
        encoder: &mut wgpu::CommandEncoder,
        frame_view: &wgpu::TextureView,
        frame: &UiFrame,
        events: Vec<UiEvent>,
    ) -> Result<bool, EguiError> {
        if !frame.visible || !self.has_font() {
            return Ok(false);
        }
        let ppp = self.screen.pixels_per_point;
        let [w, h] = self.screen.size_in_pixels;
        // egui 的 screen_rect 用逻辑点:物理像素 / 像素密度。
        let screen_rect = Rect::from_min_size(pos2(0.0, 0.0), vec2(w as f32 / ppp, h as f32 / ppp));
        // 菜单无动画,time 固定 None 以保 golden 确定性。
        let raw = crate::ui::raw_input(&events, screen_rect, ppp, None);
        let full = match self.ui.run_frame(raw, frame, ppp) {
            Some(full) => full,
            // 无字体/不可见:run_frame 返回 None,本 pass 零工作。
            None => return Ok(false),
        };
        let egui::FullOutput {
            shapes,
            textures_delta,
            pixels_per_point,
            ..
        } = full;

        // 纹理增量先应用(渲染前),释放延迟到渲染后由 egui-wgpu 内部处理:
        // 菜单字形/图集纹理经 renderer.update_texture/free_texture 维护,
        // 本模块不自行建 wgpu 纹理。
        for (id, delta) in textures_delta.set {
            self.renderer.update_texture(device, queue, id, &delta);
        }
        for id in textures_delta.free {
            self.renderer.free_texture(&id);
        }

        // tessellate:shapes -> 可画三角网格;需要持有字体图集的上下文。
        let jobs = self.ui.ctx().tessellate(shapes, pixels_per_point);

        // 上传顶点/索引/uniform(回调相关会返回独立命令缓冲,常规为空)。
        let callback_buffers =
            self.renderer
                .update_buffers(device, queue, encoder, &jobs, &self.screen);

        // 录制最上层 pass:load=Load(叠加在既有画面之上)、store=Store,
        // 无 depth(egui 自处理顺序),标签 "egui pass"。render 需要
        // RenderPass<'static>,故经 forget_lifetime 擦除生命周期。
        let pass = encoder.begin_render_pass(&wgpu::RenderPassDescriptor {
            label: Some("egui pass"),
            color_attachments: &[Some(wgpu::RenderPassColorAttachment {
                view: frame_view,
                depth_slice: None,
                resolve_target: None,
                ops: wgpu::Operations {
                    load: wgpu::LoadOp::Load,
                    store: wgpu::StoreOp::Store,
                },
            })],
            depth_stencil_attachment: None,
            occlusion_query_set: None,
            timestamp_writes: None,
            multiview_mask: None,
        });
        let mut pass = pass.forget_lifetime();
        self.renderer.render(&mut pass, &jobs, &self.screen);
        drop(pass);

        // 回调命令缓冲(常规为空)随主编码器一并提交,保证回调侧资源就绪。
        queue.submit(callback_buffers);

        Ok(true)
    }
}
