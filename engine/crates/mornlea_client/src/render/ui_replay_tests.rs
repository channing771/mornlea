//! renderer 外层 UI 输出容量失败的输入重放测试。

use super::{FrameResult, tests_support::*};
use crate::ui::{UI_OUTPUT_QUEUE_CAPACITY, UiEvent, pending_ui_event_count, push_ui_event};
use std::sync::{
    Arc,
    atomic::{AtomicUsize, Ordering},
};

const ENTER_ACTION: u32 = 11;

pub(super) struct TestCallback {
    prepared: Arc<AtomicUsize>,
    frame_view: wgpu::TextureView,
}

impl TestCallback {
    pub(super) fn new(prepared: Arc<AtomicUsize>, frame_view: wgpu::TextureView) -> Self {
        Self {
            prepared,
            frame_view,
        }
    }
}

impl egui_wgpu::CallbackTrait for TestCallback {
    fn prepare(
        &self,
        device: &wgpu::Device,
        _queue: &wgpu::Queue,
        _screen: &egui_wgpu::ScreenDescriptor,
        _egui_encoder: &mut wgpu::CommandEncoder,
        _resources: &mut egui_wgpu::CallbackResources,
    ) -> Vec<wgpu::CommandBuffer> {
        self.prepared.fetch_add(1, Ordering::Relaxed);
        let mut encoder = device.create_command_encoder(&wgpu::CommandEncoderDescriptor {
            label: Some("test egui callback"),
        });
        {
            let _pass = encoder.begin_render_pass(&wgpu::RenderPassDescriptor {
                label: Some("test egui callback clear"),
                color_attachments: &[Some(wgpu::RenderPassColorAttachment {
                    view: &self.frame_view,
                    depth_slice: None,
                    resolve_target: None,
                    ops: wgpu::Operations {
                        load: wgpu::LoadOp::Clear(wgpu::Color::RED),
                        store: wgpu::StoreOp::Store,
                    },
                })],
                depth_stencil_attachment: None,
                occlusion_query_set: None,
                timestamp_writes: None,
                multiview_mask: None,
            });
        }
        vec![encoder.finish()]
    }

    fn paint(
        &self,
        _info: egui::PaintCallbackInfo,
        _pass: &mut wgpu::RenderPass<'static>,
        _resources: &egui_wgpu::CallbackResources,
    ) {
    }
}

/// 输出队列预检必须发生在全局 window-input 队列排空之前；容量恢复后，同一批
/// click/text/scroll 输入重试得到的动作必须与无失败路径相同。
#[test]
fn output_capacity_preserves_window_input_for_retry() {
    let Some(mut retry_renderer) = renderer_or_skip_pub(320, 180) else {
        return;
    };
    let Some(mut clean_renderer) = renderer_or_skip_pub(320, 180) else {
        return;
    };
    let font = include_bytes!("../ui/testdata/demo.ttf");
    retry_renderer.upload_ui_font(font).unwrap();
    clean_renderer.upload_ui_font(font).unwrap();
    let frame = menu_frame();

    // 两条路径先各跑一次指针移动，建立同一 hover 状态。
    push_ui_event(UiEvent::CursorMoved(160.0, 78.0));
    assert_eq!(retry_renderer.render_frame(&frame), FrameResult::Rendered);
    push_ui_event(UiEvent::CursorMoved(160.0, 78.0));
    assert_eq!(clean_renderer.render_frame(&frame), FrameResult::Rendered);

    // 主菜单单帧最坏输出为 8 条；预填到只剩 7 条后，外层必须在 take 前拒绝。
    retry_renderer.test_fill_ui_actions(UI_OUTPUT_QUEUE_CAPACITY - 7);
    let retry_input = [
        UiEvent::CursorMoved(160.0, 78.0),
        UiEvent::MouseButton(true, true),
        UiEvent::Text('界'),
        UiEvent::Scroll(0.0, -24.0),
    ];
    for event in retry_input.iter().cloned() {
        push_ui_event(event);
    }
    assert_eq!(retry_renderer.render_frame(&frame), FrameResult::Capacity);
    assert_eq!(pending_ui_event_count(), retry_input.len());

    let mut scratch = vec![0u8; 4096];
    retry_renderer.drain_ui_events(&mut scratch).unwrap();
    assert_eq!(retry_renderer.render_frame(&frame), FrameResult::Rendered);
    push_ui_event(UiEvent::CursorMoved(160.0, 78.0));
    push_ui_event(UiEvent::MouseButton(true, false));
    assert_eq!(retry_renderer.render_frame(&frame), FrameResult::Rendered);
    let retry_batch = drain_batch(&mut retry_renderer);

    for event in retry_input {
        push_ui_event(event);
    }
    assert_eq!(clean_renderer.render_frame(&frame), FrameResult::Rendered);
    push_ui_event(UiEvent::CursorMoved(160.0, 78.0));
    push_ui_event(UiEvent::MouseButton(true, false));
    assert_eq!(clean_renderer.render_frame(&frame), FrameResult::Rendered);
    let clean_batch = drain_batch(&mut clean_renderer);

    assert_eq!(retry_batch, clean_batch);
    assert!(
        retry_batch
            .windows(4)
            .any(|word| word == ENTER_ACTION.to_le_bytes())
    );
}

#[test]
fn benchmark_batch_submits_egui_callbacks_before_main() {
    let Some(mut renderer) = renderer_or_skip_pub(320, 180) else {
        return;
    };
    renderer
        .upload_ui_font(include_bytes!("../ui/testdata/demo.ttf"))
        .unwrap();
    let callback_buffers = renderer.test_emit_egui_callback_buffer();

    assert_eq!(renderer.render_frame(&menu_frame()), FrameResult::Rendered);
    let mut immediate = vec![0u8; renderer.output_bytes()];
    assert!(renderer.readback(&mut immediate));

    assert_eq!(
        renderer.prepare_benchmark_batch(&menu_frame(), 1),
        FrameResult::Rendered
    );
    assert_eq!(callback_buffers.load(Ordering::Relaxed), 2);
    assert_eq!(renderer.submit_benchmark_batch(), FrameResult::Rendered);
    let mut batched = vec![0u8; renderer.output_bytes()];
    assert!(renderer.readback(&mut batched));
    assert_eq!(batched, immediate);
}

fn drain_batch(renderer: &mut super::OffscreenRenderer) -> Vec<u8> {
    let mut out = vec![0u8; 4096];
    let written = renderer.drain_ui_events(&mut out).unwrap();
    out.truncate(written);
    out
}

fn menu_frame() -> super::FrameInput {
    let mut segment = Vec::new();
    segment.extend_from_slice(&1u32.to_le_bytes());
    segment.extend_from_slice(&1u32.to_le_bytes());
    segment.extend_from_slice(&1u32.to_le_bytes());
    segment.extend_from_slice(&ENTER_ACTION.to_le_bytes());
    segment.extend_from_slice(&12u32.to_le_bytes());
    segment.extend_from_slice("进入游戏".as_bytes());
    segment.extend_from_slice(&1u32.to_le_bytes());
    for text in ["Mornlea", "test", ""] {
        segment.extend_from_slice(&(text.len() as u32).to_le_bytes());
        segment.extend_from_slice(text.as_bytes());
    }
    let mut frame = empty_frame_pub();
    frame.ui_segment = segment;
    frame
}
