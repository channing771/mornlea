//! WGSL 内嵌:R2c 起 shader 归属本 crate(`shaders/` 目录),Rust 渲染器
//! 是唯一消费方。路径失效或文件清空由本模块单测(非空且含入口点)兜底。

/// 耕地材质层的闭区间:`material ∈ [FIRST, LAST]` 的 quad 携带角高度
/// (registry `block_top_raw` 非零的短方块,当前即干/湿耕地),terrain.wgsl
/// 据此把 bit 12..19/55..62 解码成顶点抬升而不是 `w/h` 尺寸。
///
/// 判别为什么走 material 区间:quad 布局只剩 bit 63 一个空闲位且被
/// `voxel-visual-presentation` 钉死必须留空,「是不是短方块」占不到任何位;
/// 植物(区间 31..38)已借走 face 6/7 当交叉斜面判别。耕地是轴向面(face
/// 0..5),与植物按 face 天然互斥,material 区间是唯一不冲突的判别通道。
///
/// 数值的真值源是 Go 侧 `internal/assets` 的层枚举 `LayerFarmlandDry`/
/// `LayerFarmlandWet`(iota,当前 29/30)。三处没有共享定义也没有生成步骤,
/// 只能人手同步——Go 枚举、本常量、terrain.wgsl 里 `farmland_material` 的
/// 硬编码字面量。Go 侧钉子是 internal/assets 的
/// `TestFarmlandLayerNumbersMatchClientShaderContract`,本 crate 由
/// render/farmland_tests.rs 把常量与 shader 字面量一起钉住。在 Go 层枚举的
/// 耕地**之前**插层会整体平移这段区间,那两处守卫就是仅有的报警点。
pub const FARMLAND_MATERIAL_FIRST: u16 = 29;
/// 耕地材质层闭区间的上界(湿耕地),见 [`FARMLAND_MATERIAL_FIRST`]。
pub const FARMLAND_MATERIAL_LAST: u16 = 30;

/// 地形 pass(实例化紧凑 quad)。
pub const TERRAIN: &str = include_str!("../../shaders/terrain.wgsl");
/// 水面 pass(半透明,与 terrain 共享 atlas 与世界坐标 UV)。
pub const WATER: &str = include_str!("../../shaders/water.wgsl");
/// 远环 LOD 壳 pass(世界坐标大 quad + 距离雾)。
pub const LOD: &str = include_str!("../../shaders/lod.wgsl");
/// 天空与程序化方块云 pass。
pub const SKY: &str = include_str!("../../shaders/sky.wgsl");
/// GPU culling compute。
pub const CULL: &str = include_str!("../../shaders/cull.wgsl");
/// HiZ mip 链构建 compute。
pub const HIZ_BUILD: &str = include_str!("../../shaders/hiz_build.wgsl");
/// HiZ 深度拷贝 compute。
pub const HIZ_COPY: &str = include_str!("../../shaders/hiz_copy.wgsl");
/// 实体 pass(avatar 与掉落物共用)。
pub const AVATAR: &str = include_str!("../../shaders/avatar.wgsl");
/// 全屏叠加 pass：伤害红边与水下水色共用（uniform 的 edge 位区分两者）。
pub const DAMAGE_OVERLAY: &str = include_str!("../../shaders/damage_overlay.wgsl");
/// 名牌 billboard pass。
pub const NAME_TAG: &str = include_str!("../../shaders/name_tag.wgsl");
/// 调试面板 pass。
pub const DEBUG_PANEL: &str = include_str!("../../shaders/debug_panel.wgsl");
/// HUD(hotbar 家族)pass。
pub const HUD_HOTBAR: &str = include_str!("../../shaders/hotbar.wgsl");

#[cfg(test)]
mod tests {
    use super::*;

    /// 单源存在性:路径失效或文件清空都必须在编译/测试期暴露。
    #[test]
    fn shaders_are_nonempty_and_have_entry_points() {
        for (name, source) in [
            ("terrain", TERRAIN),
            ("water", WATER),
            ("lod", LOD),
            ("sky", SKY),
            ("cull", CULL),
            ("hiz_build", HIZ_BUILD),
            ("hiz_copy", HIZ_COPY),
            ("avatar", AVATAR),
            ("damage_overlay", DAMAGE_OVERLAY),
            ("name_tag", NAME_TAG),
            ("debug_panel", DEBUG_PANEL),
            ("hud_hotbar", HUD_HOTBAR),
        ] {
            assert!(!source.trim().is_empty(), "{name} 为空");
            assert!(source.contains("fn "), "{name} 缺少入口函数");
        }
    }
}
