//! 降水粒子叠加层：Go `render` 包编码的天气降水实例流的 GPU 侧呈现。
//!
//! Go 侧把权威 tick 派生的降水位置烘焙进实例变换里（雨丝与雪点复用同一
//! 通道：形态只由粒子高度相对雪线选形，见 `render::BuildWeatherParts`），
//! 经帧 TLV 段过境；本模块只做绘制选择、合法性门与 sky uniform 灰度写入，
//! 不做任何天气推测（晴天两段恒为空）。
//!
//! 复用纪律（与 `viewmodel` 同源）：
//!
//! - 实例布局逐字节同 avatar（[`WEATHER_INSTANCE_BYTES`] 即
//!   `ENTITY_INSTANCE_BYTES`：mat4 + RGBA + 材质 u32 + 保留零填充），纯色
//!   分支走哨兵材质的原纯色路径；
//! - GPU 资源直接复用 `EntityPass`（容量取 [`WEATHER_MAX_INSTANCES`] 的
//!   不透明变体），不新增管线、几何与通道创建——录制经既有实体通道的同一
//!   录制入口；
//! - 超限整帧拒绝（`validate_frame` 经 [`instances_valid`] 判否，整帧以
//!   `Invalid` 退回且不触碰 target），不断言、不 panic、不截断绘制；
//! - 天空灰化复用既有 sky uniform 与 fullscreen sky draw：灰度只写入预留位，
//!   uniform 总量仍为 112 字节、既有字段偏移不动。
//!
//! 帧序：世界 pass（地形、水面、avatar、掉落物、轮廓、裂纹、双手）之后，
//! 名牌与全屏叠加、HUD、调试面板之前；空段跳过录制，晴天帧的 draw 选择与
//! 变更前一致。

/// 单帧降水实例固定上限：与 Go 编码侧的同名上限同值，超限整帧拒绝
/// （整帧 `Invalid`）。
pub const WEATHER_MAX_INSTANCES: usize = 256;
/// 每实例字节数：与 avatar 实例逐字节同布局（mat4 + RGBA + 材质 u32 +
/// 保留零填充），绘制复用 `EntityPass` 的同一套材质分支。
pub const WEATHER_INSTANCE_BYTES: usize = super::entity::ENTITY_INSTANCE_BYTES;
/// 天气状态段字节数：灰度因子单个 f32 小端。
pub const WEATHER_STATE_BYTES: usize = 4;
/// 满段字节数：256 实例 × 96 字节，与 Go 编码侧的上限同值。
pub const WEATHER_FULL_BYTES: usize = WEATHER_MAX_INSTANCES * WEATHER_INSTANCE_BYTES;
/// 叠加 pass 的录制标签：抓帧定位降水层用，改名即红。
pub const WEATHER_PASS_LABEL: &str = "weather precip pass";
/// sky uniform 总字节数：灰度写入不得改变该总量。
pub const SKY_UNIFORM_BYTES: usize = 112;
/// 灰度在 sky uniform 内的字节偏移（原 `padding: vec2u` 的首字）：
/// shader 侧为 `weather: vec2f` 的 x 分量，y 分量保留零。
pub const SKY_WEATHER_OFFSET: usize = 88;

/// 校验降水实例段字节：96 的倍数且不超过 256 实例；空流合法（本帧无降水）。
/// 容量是编译期常量，校验无需 GPU 资源，可供无适配器环境的单元测试与
/// `validate_frame` 直接复用。
pub fn instances_valid(instances: &[u8]) -> bool {
    instances.len().is_multiple_of(WEATHER_INSTANCE_BYTES) && instances.len() <= WEATHER_FULL_BYTES
}

/// 校验天气状态段字节：恰 4 字节且灰度有限落在 0..=1；空段非法——晴天由
/// 调用方省略段表达，不以零负载段表达（晴天帧与旧版本逐字节一致的前提）。
pub fn state_valid(segment: &[u8]) -> bool {
    if segment.len() != WEATHER_STATE_BYTES {
        return false;
    }
    let gray = f32::from_le_bytes(segment.try_into().expect("状态段恰 4 字节"));
    gray.is_finite() && (0.0..=1.0).contains(&gray)
}

/// 绘制选择：非空段才录制；空段跳过，晴天帧 draw 选择不变。
/// 内容合法性由 [`instances_valid`] 把关，本函数只看有无。
pub fn wants_draw(instances: &[u8]) -> bool {
    !instances.is_empty()
}

/// 天气灰度写入 sky uniform 的预留位（88..92 小端 f32，92..96 恒零）：
/// 布局总量仍为 112 字节，既有字段偏移不动，沿用同一份 fullscreen sky draw。
/// 调用方保证 `gray` 已在解析层校验（有限且 0..=1），本函数只做字节搬运。
pub fn write_sky_weather(sky_data: &mut [u8; SKY_UNIFORM_BYTES], gray: f32) {
    sky_data[SKY_WEATHER_OFFSET..SKY_WEATHER_OFFSET + 4].copy_from_slice(&gray.to_le_bytes());
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 跨语言布局锁：96 字节/实例与 avatar 逐字节同布局，容量恒为 256，
    /// 状态段恒为 4 字节；与 Go 编码侧的同值常量一致，漂移即红。
    #[test]
    fn layout_matches_avatar_and_go_encoder() {
        assert_eq!(WEATHER_INSTANCE_BYTES, 96);
        assert_eq!(
            WEATHER_INSTANCE_BYTES,
            super::super::entity::ENTITY_INSTANCE_BYTES
        );
        assert_eq!(WEATHER_MAX_INSTANCES, 256);
        assert_eq!(WEATHER_FULL_BYTES, 24576, "满段 256 实例的字节数");
        assert_eq!(WEATHER_STATE_BYTES, 4);
        assert_eq!(super::super::entity::AVATAR_MATERIAL_OFFSET, 80);
        assert_eq!(
            super::super::entity::AVATAR_MATERIAL_SOLID,
            u32::MAX,
            "哨兵须与全部有效材质层号不相交"
        );
    }

    /// 实例段校验锁：空流合法（晴天），1..=256 实例合法；错位长度与第 257
    /// 个实例整帧拒绝，拒绝语义与 avatar/drop/轮廓/裂纹/双手门一致。
    #[test]
    fn instances_valid_locks_count_and_alignment() {
        assert!(instances_valid(&[]), "空流合法（晴天）");
        for count in [1, 2, 255, WEATHER_MAX_INSTANCES] {
            let stream = vec![0u8; count * WEATHER_INSTANCE_BYTES];
            assert!(instances_valid(&stream), "{count} 实例合法");
        }
        let oversized = vec![0u8; (WEATHER_MAX_INSTANCES + 1) * WEATHER_INSTANCE_BYTES];
        assert!(!instances_valid(&oversized), "第 257 个实例必须整帧拒绝");
        let misaligned = vec![0u8; WEATHER_INSTANCE_BYTES + 1];
        assert!(!instances_valid(&misaligned), "非 96 倍数必须拒绝");
    }

    /// 状态段校验锁：恰 4 字节有限 0..=1 合法；空段、错位长度、NaN 与越界拒绝。
    #[test]
    fn state_valid_locks_shape_and_range() {
        assert!(state_valid(&0.5f32.to_le_bytes()), "雨天灰度合法");
        assert!(state_valid(&0.0f32.to_le_bytes()), "零灰度合法");
        assert!(!state_valid(&[]), "空段非法（晴天省略段表达）");
        assert!(!state_valid(&[0u8; 8]), "错位长度拒绝");
        assert!(!state_valid(&f32::NAN.to_le_bytes()), "NaN 拒绝");
        assert!(!state_valid(&1.5f32.to_le_bytes()), "越界拒绝");
        assert!(!state_valid(&(-0.5f32).to_le_bytes()), "负值拒绝");
    }

    /// 绘制选择锁：空段永不录制（晴天帧 draw 选择不变），非空合法段才绘制。
    #[test]
    fn empty_stream_never_draws() {
        assert!(!wants_draw(&[]), "空段不得绘制");
        assert!(wants_draw(&[0u8; WEATHER_INSTANCE_BYTES]), "非空段必须绘制");
    }

    /// 无段帧仍是纯地形帧：空降水流且零灰度不构成 pass 段；该边界由解码侧的
    /// `empty_passes` 承担，此处从消费侧锁定。
    #[test]
    fn empty_weather_stream_keeps_pure_terrain_frame() {
        use super::super::FrameInput;
        let frame = FrameInput::default();
        assert!(frame.empty_passes(), "纯地形帧必须是 empty_passes");
        let with_precip = FrameInput {
            precip_instances: vec![0u8; WEATHER_INSTANCE_BYTES],
            ..FrameInput::default()
        };
        assert!(!with_precip.empty_passes(), "非空降水流构成 pass 段");
        let with_gray = FrameInput {
            weather_gray: 0.5,
            ..FrameInput::default()
        };
        assert!(!with_gray.empty_passes(), "非零灰度构成 pass 段");
    }

    /// sky uniform 灰度写入锁：总量仍为 112 字节，灰度落在 88..92，92..96
    /// 恒零，既有字段（macro@84、camera@96）不动。
    #[test]
    fn sky_weather_write_keeps_uniform_layout() {
        let mut sky_data = [0xAAu8; SKY_UNIFORM_BYTES];
        write_sky_weather(&mut sky_data, 0.5);
        assert_eq!(
            f32::from_le_bytes(
                sky_data[SKY_WEATHER_OFFSET..SKY_WEATHER_OFFSET + 4]
                    .try_into()
                    .unwrap()
            ),
            0.5
        );
        assert!(
            sky_data[SKY_WEATHER_OFFSET + 4..SKY_WEATHER_OFFSET + 8]
                .iter()
                .all(|&b| b == 0xAA),
            "保留字恒不动（调用方零初始化时即为零）"
        );
        assert_eq!(sky_data.len(), SKY_UNIFORM_BYTES);
    }

    /// shader 契约锁：sky 着色器必须以 `weather: vec2f` 承载灰度（取代旧
    /// `padding: vec2u`，总量与偏移不变），并在合成末尾按灰度向亮度灰靠拢。
    #[test]
    fn sky_shader_carries_weather_gray() {
        let source = super::super::shaders::SKY;
        assert!(
            source.contains("weather:         vec2f"),
            "sky uniform 须有 weather vec2f 字段"
        );
        assert!(
            !source.contains("padding:         vec2u"),
            "旧 padding 须被 weather 取代"
        );
        assert!(source.contains("sky.weather.x"), "合成须消费天气灰度");
    }

    /// pass 标签稳定：录制调用点以该标签命名，改名即红，便于抓帧定位降水层。
    #[test]
    fn pass_label_is_stable() {
        assert_eq!(WEATHER_PASS_LABEL, "weather precip pass");
    }
}
