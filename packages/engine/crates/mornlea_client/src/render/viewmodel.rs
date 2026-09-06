//! 第一人称双手 viewmodel 叠加层：Go `render` 包编码的相机空间实例流的
//! GPU 侧呈现。
//!
//! Go 侧把权威 tick 派生的摆动相位烘焙进世界空间无关的实例变换里（左手、
//! 右手、持物各一，另保留一位副手占位，恒不超过 [`VIEWMODEL_MAX_INSTANCES`]），
//! 经帧 TLV 段过境；本模块只做绘制选择与合法性门，不做任何摆动推测。
//!
//! 复用纪律（与 `entity.rs` 同源）：
//!
//! - 实例布局逐字节同 avatar（[`VIEWMODEL_INSTANCE_BYTES`] 即
//!   `ENTITY_INSTANCE_BYTES`：mat4 + RGBA + 材质 u32 + 保留零填充），纯色
//!   分支走哨兵材质的原纯色路径，贴图分支经材质层号采样同一方块 atlas；
//! - GPU 资源直接复用 `EntityPass`（容量取 [`VIEWMODEL_MAX_INSTANCES`] 的
//!   不透明变体），不新增管线、几何与通道创建——录制经既有实体通道的同一
//!   录制入口，半透明阶段预算与 pass 名单门禁保持不变；
//! - 超限整帧拒绝（`validate_frame` 经 [`instances_valid`] 判否，整帧以
//!   `Invalid` 退回且不触碰 target——世界与其它 pass 本帧一并丢弃，与
//!   avatar/drop/轮廓/裂纹的门禁纪律同形），不断言、不 panic、不截断绘制；
//!   计数门由 Go 装配侧承担，本侧只做防御性丢弃。
//!
//! 帧序：世界 pass（地形、水面、avatar、掉落物、轮廓、裂纹）之后，名牌与
//! 全屏叠加、HUD、调试面板之前；空段跳过录制，无段帧的 draw 选择与变更前
//! 一致。

/// 单帧 viewmodel 实例恒定上限：左手、右手、持物各一，另保留一位副手扩展
/// 占位；与 Go 编码侧的同名上限同值，超限整帧拒绝（整帧 `Invalid`）。
pub const VIEWMODEL_MAX_INSTANCES: usize = 4;
/// 每实例字节数：与 avatar 实例逐字节同布局（mat4 + RGBA + 材质 u32 +
/// 保留零填充），绘制复用 `EntityPass` 的同一套材质分支。
pub const VIEWMODEL_INSTANCE_BYTES: usize = super::entity::ENTITY_INSTANCE_BYTES;
/// 叠加 pass 的录制标签：抓帧定位相机空间叠加层用，改名即红。
pub const VIEWMODEL_PASS_LABEL: &str = "viewmodel pass";

/// 校验 viewmodel 实例段字节：96 的倍数且不超过 4 实例；空流合法（本帧
/// 无双手）。容量是编译期常量，校验无需 GPU 资源，可供无适配器环境的
/// 单元测试与 `validate_frame` 直接复用。
pub fn instances_valid(instances: &[u8]) -> bool {
    instances.len().is_multiple_of(VIEWMODEL_INSTANCE_BYTES)
        && instances.len() <= VIEWMODEL_MAX_INSTANCES * VIEWMODEL_INSTANCE_BYTES
}

/// 绘制选择：非空段才录制；空段跳过，无段帧的 draw 选择与变更前一致。
/// 内容合法性由 [`instances_valid`] 把关，本函数只看有无。
pub fn wants_draw(instances: &[u8]) -> bool {
    !instances.is_empty()
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 跨语言布局锁：96 字节/实例与 avatar 逐字节同布局，容量恒为 4
    /// （左手、右手、持物、副手保留位），与 Go 编码侧的同值常量一致；
    /// 材质槽偏移与哨兵沿 avatar 材质分支纪律，不相交性即纯色与贴图可辨。
    #[test]
    fn layout_matches_avatar_and_go_encoder() {
        assert_eq!(VIEWMODEL_INSTANCE_BYTES, 96);
        assert_eq!(
            VIEWMODEL_INSTANCE_BYTES,
            super::super::entity::ENTITY_INSTANCE_BYTES
        );
        assert_eq!(VIEWMODEL_MAX_INSTANCES, 4);
        assert_eq!(
            VIEWMODEL_MAX_INSTANCES * VIEWMODEL_INSTANCE_BYTES,
            384,
            "满段 4 实例的字节数"
        );
        assert_eq!(super::super::entity::AVATAR_MATERIAL_OFFSET, 80);
        assert_eq!(
            super::super::entity::AVATAR_MATERIAL_SOLID,
            u32::MAX,
            "哨兵须与全部有效材质层号不相交"
        );
    }

    /// 实例段校验锁：空流合法（本帧无双手），1..=4 实例合法；错位长度与
    /// 第 5 个实例整帧拒绝，拒绝语义与 avatar/drop/轮廓/裂纹门一致。
    #[test]
    fn instances_valid_locks_count_and_alignment() {
        assert!(instances_valid(&[]), "空流合法（本帧无双手）");
        for count in 1..=VIEWMODEL_MAX_INSTANCES {
            let stream = vec![0u8; count * VIEWMODEL_INSTANCE_BYTES];
            assert!(instances_valid(&stream), "{count} 实例合法");
        }
        let oversized = vec![0u8; (VIEWMODEL_MAX_INSTANCES + 1) * VIEWMODEL_INSTANCE_BYTES];
        assert!(!instances_valid(&oversized), "第 5 个实例必须整帧拒绝");
        let misaligned = vec![0u8; VIEWMODEL_INSTANCE_BYTES + 1];
        assert!(!instances_valid(&misaligned), "非 96 倍数必须拒绝");
        assert!(!instances_valid(&[0u8; 79]), "错位短流必须拒绝");
    }

    /// 绘制选择锁：空段永不录制（无段帧 draw 选择不变），非空合法段才
    /// 绘制；内容合法性由 [`instances_valid`] 把关，本函数只看有无。
    #[test]
    fn empty_stream_never_draws() {
        assert!(!wants_draw(&[]), "空段不得绘制");
        assert!(
            wants_draw(&[0u8; VIEWMODEL_INSTANCE_BYTES]),
            "非空段必须绘制"
        );
    }

    /// 无段帧仍是纯地形帧：空 viewmodel 流不构成 pass 段，非空流构成；
    /// 该边界由解码侧的 `empty_passes` 承担，此处从消费侧锁定。
    #[test]
    fn empty_viewmodel_stream_keeps_pure_terrain_frame() {
        use super::super::FrameInput;
        let frame = FrameInput::default();
        assert!(frame.empty_passes(), "纯地形帧必须是 empty_passes");
        let with_viewmodel = FrameInput {
            viewmodel_instances: vec![0u8; VIEWMODEL_INSTANCE_BYTES],
            ..FrameInput::default()
        };
        assert!(
            !with_viewmodel.empty_passes(),
            "非空 viewmodel 流构成 pass 段"
        );
    }

    /// pass 标签稳定：录制调用点以该标签命名，改名即红，便于抓帧定位
    /// 相机空间叠加层。
    #[test]
    fn pass_label_is_stable() {
        assert_eq!(VIEWMODEL_PASS_LABEL, "viewmodel pass");
    }
}

#[cfg(test)]
mod render_tests {
    use super::super::tests_support::*;
    use super::*;

    /// 构造一个 96 字节 viewmodel 实例：恒等 mat4（列主序）+ 红色 +
    /// 哨兵材质 + 保留零填充，布局与 avatar 实例逐字节一致。
    fn viewmodel_instance() -> Vec<u8> {
        let mut instance = vec![0u8; VIEWMODEL_INSTANCE_BYTES];
        for i in 0..4 {
            instance[(i * 4 + i) * 4..(i * 4 + i) * 4 + 4].copy_from_slice(&1.0f32.to_le_bytes());
        }
        instance[64..68].copy_from_slice(&1.0f32.to_le_bytes());
        instance[76..80].copy_from_slice(&1.0f32.to_le_bytes());
        instance[80..84]
            .copy_from_slice(&super::super::entity::AVATAR_MATERIAL_SOLID.to_le_bytes());
        instance
    }

    /// 双手叠加层真实参与成像：atlas 预热后同一实例必须改变图像；超限
    /// （5 实例）与错位流在渲染前整帧拒绝且不触碰 target。
    #[test]
    fn viewmodel_renders_and_invalid_rejects_without_touching_target() {
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
        frame.viewmodel_instances = viewmodel_instance();
        assert_eq!(renderer.render_frame(&frame), FrameResult::Rendered);
        let mut with_viewmodel = vec![0u8; 64 * 64 * 4];
        assert!(renderer.readback(&mut with_viewmodel));
        assert_ne!(base, with_viewmodel, "双手实例必须改变图像");

        let mut oversized = empty_frame_pub();
        oversized.viewmodel_instances =
            vec![0u8; (VIEWMODEL_MAX_INSTANCES + 1) * VIEWMODEL_INSTANCE_BYTES];
        assert_eq!(renderer.render_frame(&oversized), FrameResult::Invalid);
        let mut misaligned = empty_frame_pub();
        misaligned.viewmodel_instances = vec![0u8; 100];
        assert_eq!(renderer.render_frame(&misaligned), FrameResult::Invalid);
        let mut after_bad = vec![0u8; 64 * 64 * 4];
        assert!(renderer.readback(&mut after_bad));
        assert_eq!(with_viewmodel, after_bad, "拒绝帧不得触碰 target");
    }

    /// 无段帧 draw 选择不变：空 viewmodel 流的成像与纯地形帧逐字节一致，
    /// 叠加层不向无段帧引入任何额外绘制。
    #[test]
    fn empty_viewmodel_frame_matches_pure_terrain_pixels() {
        use super::super::FrameResult;
        let Some(mut renderer) = renderer_or_skip_pub(64, 64) else {
            return;
        };
        let empty = empty_frame_pub();
        assert_eq!(renderer.render_frame(&empty), FrameResult::Rendered);
        let mut base = vec![0u8; 64 * 64 * 4];
        assert!(renderer.readback(&mut base));

        let mut no_segment = empty_frame_pub();
        no_segment.viewmodel_instances = Vec::new();
        assert_eq!(renderer.render_frame(&no_segment), FrameResult::Rendered);
        let mut without_draw = vec![0u8; 64 * 64 * 4];
        assert!(renderer.readback(&mut without_draw));
        assert_eq!(base, without_draw, "无段帧不得引入额外绘制");
    }

    /// 叠压顺序锁：全屏叠加画在双手之后——同一双手实例上再叠加均匀水色，
    /// 手部区域像素必须改变。双手是不透明覆盖，若它画在叠加之后，手部像
    /// 素将与纯双手帧逐字节相同。用 edge = 0 的均匀水色而非伤害红边：红边
    /// 是边缘渐变，恒等相机下双手落在零覆盖的画面中央，锁不住顺序；水色与
    /// 红边共用同一条 pass 与管线，顺序结论互通。名牌/HUD/调试面板的其后
    /// 顺序由代码位置保证，交场景 golden 覆盖。
    #[test]
    fn water_tint_covers_viewmodel_hand() {
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

        let mut hand = empty_frame_pub();
        hand.viewmodel_instances = viewmodel_instance();
        assert_eq!(renderer.render_frame(&hand), FrameResult::Rendered);
        let mut hand_pixels = vec![0u8; 64 * 64 * 4];
        assert!(renderer.readback(&mut hand_pixels));
        assert_ne!(hand_pixels, base, "双手实例必须先改变图像");

        let mut tinted_hand = empty_frame_pub();
        tinted_hand.viewmodel_instances = viewmodel_instance();
        tinted_hand.water_tint = [0.1, 0.2, 0.8, 0.5];
        assert_eq!(renderer.render_frame(&tinted_hand), FrameResult::Rendered);
        let mut tinted_pixels = vec![0u8; 64 * 64 * 4];
        assert!(renderer.readback(&mut tinted_pixels));

        // 手部区域 = 纯双手帧中区别于空帧的像素；叠加之后其中至少一处须变。
        let covered = hand_pixels
            .chunks_exact(4)
            .zip(tinted_pixels.chunks_exact(4))
            .zip(base.chunks_exact(4))
            .filter(|((hand, tinted), empty)| hand != empty && tinted != hand)
            .count();
        assert!(covered > 0, "全屏叠加必须盖在双手之上");
    }
}
