//! 权威投射物的世界空间呈现层：Go `render` 包编码的实例流的 GPU 侧绘制
//! 选择与合法性门。
//!
//! Go 侧把镜像呈现（latest-wins + 插值、绝不预测弹道）烘焙进实例变换——
//! 长轴沿权威速度估计取向、配色按弹种固定（骨刺骨白、箭深棕），本模块只
//! 做绘制选择与合法性门，不做任何弹道推测。
//!
//! 复用纪律（与 `entity.rs`/`viewmodel.rs` 同源）：
//!
//! - 实例布局逐字节同 avatar（[`PROJECTILE_INSTANCE_BYTES`] 即
//!   `ENTITY_INSTANCE_BYTES`：mat4 + RGBA + 材质 u32 + 保留零填充），纯色
//!   分支走哨兵材质的原纯色路径；
//! - GPU 资源直接复用 `EntityPass`（容量取 [`PROJECTILE_MAX_INSTANCES`]
//!   的不透明变体），不新增管线、几何与通道创建——录制经既有实体通道的
//!   同一录制入口（`entity.rs` 内的通道起始调用点），半透明阶段预算
//!   与 pass 名单门禁保持不变；
//! - 超限整帧拒绝（`validate_frame` 经 [`instances_valid`] 判否，整帧以
//!   `Invalid` 退回且不触碰 target），不断言、不 panic、不截断绘制；计数
//!   门由 Go 装配侧承担，本侧只做防御性丢弃。
//!
//! 帧序：avatar → 掉落物 → 轮廓 → 裂纹 → 双手 → 降水之后、名牌之前——
//! 投射物是高速小目标，画在世界叠加链的末端保证不被降水粒子整片遮盖，
//! 又先于名牌/HUD 等屏幕空间层；该顺序是固定契约，注释与
//! `translucent_render_passes_stay_within_water_and_crack` 门禁共同钉住
//! （本 pass 复用实体通道，不引入新的 render pass 起始调用点，门禁计数
//! 不变）。空段跳过录制，无段帧的 draw 选择与变更前一致。

/// 单帧投射物实例恒定上限：与客户端镜像容量、Go 编码侧段预算及 wire 侧
/// record 上限同值（128），超限整帧拒绝（整帧 `Invalid`）。
pub const PROJECTILE_MAX_INSTANCES: usize = 128;
/// 每实例字节数：与 avatar 实例逐字节同布局（mat4 + RGBA + 材质 u32 +
/// 保留零填充），绘制复用 `EntityPass` 的同一套材质分支。
pub const PROJECTILE_INSTANCE_BYTES: usize = super::entity::ENTITY_INSTANCE_BYTES;
/// 投射物 pass 的录制标签：抓帧定位投射物层用，改名即红。
pub const PROJECTILE_PASS_LABEL: &str = "projectile pass";

/// 校验投射物实例段字节：96 的倍数且不超过 128 实例；空流合法（本帧无
/// 投射物）。容量是编译期常量，校验无需 GPU 资源，可供无适配器环境的
/// 单元测试与 `validate_frame` 直接复用。
pub fn instances_valid(instances: &[u8]) -> bool {
    instances.len().is_multiple_of(PROJECTILE_INSTANCE_BYTES)
        && instances.len() <= PROJECTILE_MAX_INSTANCES * PROJECTILE_INSTANCE_BYTES
}

/// 绘制选择：非空段才录制；空段跳过，无段帧的 draw 选择与变更前一致。
/// 内容合法性由 [`instances_valid`] 把关，本函数只看有无。
pub fn wants_draw(instances: &[u8]) -> bool {
    !instances.is_empty()
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 跨语言布局锁：96 字节/实例与 avatar 逐字节同布局，容量恒为 128
    /// （与 Go 编码侧 `MaxProjectileInstances` 同值），材质槽偏移与哨兵沿
    /// avatar 材质分支纪律。
    #[test]
    fn layout_matches_avatar_and_go_encoder() {
        assert_eq!(PROJECTILE_INSTANCE_BYTES, 96);
        assert_eq!(
            PROJECTILE_INSTANCE_BYTES,
            super::super::entity::ENTITY_INSTANCE_BYTES
        );
        assert_eq!(PROJECTILE_MAX_INSTANCES, 128);
        assert_eq!(
            PROJECTILE_MAX_INSTANCES * PROJECTILE_INSTANCE_BYTES,
            128 * 96,
            "满段 128 实例的字节数"
        );
        assert_eq!(super::super::entity::AVATAR_MATERIAL_OFFSET, 80);
        assert_eq!(
            super::super::entity::AVATAR_MATERIAL_SOLID,
            u32::MAX,
            "哨兵须与全部有效材质层号不相交"
        );
    }

    /// 实例段校验锁：空流合法（本帧无投射物），1..=128 实例合法；错位长度
    /// 与第 129 个实例整帧拒绝，拒绝语义与 avatar/drop/轮廓/裂纹门一致。
    #[test]
    fn instances_valid_locks_count_and_alignment() {
        assert!(instances_valid(&[]), "空流合法（本帧无投射物）");
        for count in [1usize, 2, 64, PROJECTILE_MAX_INSTANCES] {
            let stream = vec![0u8; count * PROJECTILE_INSTANCE_BYTES];
            assert!(instances_valid(&stream), "{count} 实例合法");
        }
        let oversized = vec![0u8; (PROJECTILE_MAX_INSTANCES + 1) * PROJECTILE_INSTANCE_BYTES];
        assert!(!instances_valid(&oversized), "第 129 个实例必须整帧拒绝");
        let misaligned = vec![0u8; PROJECTILE_INSTANCE_BYTES + 1];
        assert!(!instances_valid(&misaligned), "非 96 倍数必须拒绝");
        assert!(!instances_valid(&[0u8; 79]), "错位短流必须拒绝");
    }

    /// 绘制选择锁：空段永不录制（无段帧 draw 选择不变），非空合法段才
    /// 绘制；内容合法性由 [`instances_valid`] 把关，本函数只看有无。
    #[test]
    fn empty_stream_never_draws() {
        assert!(!wants_draw(&[]), "空段不得绘制");
        assert!(
            wants_draw(&[0u8; PROJECTILE_INSTANCE_BYTES]),
            "非空段必须绘制"
        );
    }

    /// 无段帧仍是纯地形帧：空投射物流不构成 pass 段，非空流构成；该边界
    /// 由解码侧的 `empty_passes` 承担，此处从消费侧锁定。
    #[test]
    fn empty_projectile_stream_keeps_pure_terrain_frame() {
        use super::super::FrameInput;
        let frame = FrameInput::default();
        assert!(frame.empty_passes(), "纯地形帧必须是 empty_passes");
        let with_projectile = FrameInput {
            projectile_instances: vec![0u8; PROJECTILE_INSTANCE_BYTES],
            ..FrameInput::default()
        };
        assert!(!with_projectile.empty_passes(), "非空投射物流构成 pass 段");
    }

    /// pass 标签稳定：录制调用点以该标签命名，改名即红，便于抓帧定位
    /// 投射物层。
    #[test]
    fn pass_label_is_stable() {
        assert_eq!(PROJECTILE_PASS_LABEL, "projectile pass");
    }
}

#[cfg(test)]
mod render_tests {
    use super::super::tests_support::*;
    use super::*;

    /// 构造一个 96 字节投射物实例：恒等 mat4（列主序）+ 红色 + 哨兵材质 +
    /// 保留零填充，布局与 avatar 实例逐字节一致。
    fn projectile_instance() -> Vec<u8> {
        let mut instance = vec![0u8; PROJECTILE_INSTANCE_BYTES];
        for i in 0..4 {
            instance[(i * 4 + i) * 4..(i * 4 + i) * 4 + 4].copy_from_slice(&1.0f32.to_le_bytes());
        }
        instance[64..68].copy_from_slice(&1.0f32.to_le_bytes());
        instance[76..80].copy_from_slice(&1.0f32.to_le_bytes());
        instance[80..84]
            .copy_from_slice(&super::super::entity::AVATAR_MATERIAL_SOLID.to_le_bytes());
        instance
    }

    /// 投射物层真实参与成像：atlas 预热后同一实例必须改变图像；超限
    /// （129 实例）与错位流在渲染前整帧拒绝且不触碰 target。
    #[test]
    fn projectile_renders_and_invalid_rejects_without_touching_target() {
        use super::super::FrameResult;
        let Some(mut renderer) = renderer_or_skip_pub(64, 64) else {
            return;
        };
        let bytes_per_layer: usize = (0..super::super::ATLAS_MIPS)
            .map(|m| {
                let s = (super::super::ATLAS_TEX_SIZE >> m).max(1) as usize;
                s * s * 4
            })
            .sum();
        assert!(renderer.upload_atlas(1, &vec![128u8; bytes_per_layer]));
        let empty = empty_frame_pub();
        assert_eq!(renderer.render_frame(&empty), FrameResult::Rendered);
        let mut base = vec![0u8; 64 * 64 * 4];
        assert!(renderer.readback(&mut base));

        let mut frame = empty_frame_pub();
        frame.projectile_instances = projectile_instance();
        assert_eq!(renderer.render_frame(&frame), FrameResult::Rendered);
        let mut with_projectile = vec![0u8; 64 * 64 * 4];
        assert!(renderer.readback(&mut with_projectile));
        assert_ne!(base, with_projectile, "投射物实例必须改变图像");

        let mut oversized = empty_frame_pub();
        oversized.projectile_instances =
            vec![0u8; (PROJECTILE_MAX_INSTANCES + 1) * PROJECTILE_INSTANCE_BYTES];
        assert_eq!(renderer.render_frame(&oversized), FrameResult::Invalid);
        let mut misaligned = empty_frame_pub();
        misaligned.projectile_instances = vec![0u8; 100];
        assert_eq!(renderer.render_frame(&misaligned), FrameResult::Invalid);
        let mut after_bad = vec![0u8; 64 * 64 * 4];
        assert!(renderer.readback(&mut after_bad));
        assert_eq!(with_projectile, after_bad, "拒绝帧不得触碰 target");
    }

    /// 无段帧 draw 选择不变：空投射物流的成像与纯地形帧逐字节一致，
    /// 投射物层不向无段帧引入任何额外绘制。
    #[test]
    fn empty_projectile_frame_matches_pure_terrain_pixels() {
        use super::super::FrameResult;
        let Some(mut renderer) = renderer_or_skip_pub(64, 64) else {
            return;
        };
        let empty = empty_frame_pub();
        assert_eq!(renderer.render_frame(&empty), FrameResult::Rendered);
        let mut base = vec![0u8; 64 * 64 * 4];
        assert!(renderer.readback(&mut base));

        let mut no_segment = empty_frame_pub();
        no_segment.projectile_instances = Vec::new();
        assert_eq!(renderer.render_frame(&no_segment), FrameResult::Rendered);
        let mut without_draw = vec![0u8; 64 * 64 * 4];
        assert!(renderer.readback(&mut without_draw));
        assert_eq!(base, without_draw, "无段帧不得引入额外绘制");
    }
}
