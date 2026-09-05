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

/// 火把材质层:墙面火把的倾斜薄板携带角高度(支撑侧 9/16、远离侧 14/16),
/// terrain.wgsl 据此把 bit 12..19/55..62 解码成顶点抬升而不是 `w/h` 尺寸——
/// 与耕地共用同一条角高度解码路径,判别同样走 material(火把薄板是轴向面
/// face 0..5,与植物的 face 6/7 按 face 天然互斥)。
///
/// 数值的真值源是 Go 侧 `internal/assets` 层枚举末位追加的 `LayerTorch`
/// (iota,当前 59,门层 55 与工作台三层 56..58 之后)。三处没有共享定义也没有
/// 生成步骤,只能人手同步——Go 枚举、本常量、terrain.wgsl 里 `torch_material`
/// 的硬编码字面量,由 render/farmland_tests.rs 的源码扫描钉在一起。在 Go 层
/// 枚举的火把之前插层会平移这个编号,那处守卫就是仅有的报警点之一。
pub const TORCH_MATERIAL: u16 = 59;

/// 床材质层的闭区间：床尾/床头 × 南西北东八层（60..67，火把层之后的冻结
/// 区间，后接短草层 68）。床是 registry `block_top_raw`=8 的短方块（9/16
/// 半高板），五条 quad 全部携带角高度原值（侧板顶缘与平顶为 8、底缘为 0），terrain.wgsl
/// 据此把 bit 12..19/55..62 解码成顶点抬升而不是 `w/h` 尺寸——与耕地/火把
/// 共用同一条角高度解码路径，判别同样走 material（床是轴向面 face 0..5，与
/// 植物的 face 6/7 按 face 天然互斥）。
///
/// 数值的真值源是 Go 侧 `internal/assets` 层枚举中的冻结区间
/// `LayerBedFootSouth`..`LayerBedHeadEast`（iota，当前 60..67，后接 68）。三处没有
/// 共享定义也没有生成步骤，只能人手同步——Go 枚举、本常量、terrain.wgsl 里
/// `bed_material` 的硬编码字面量，由 render/farmland_tests.rs 的源码扫描与
/// 床渲染回归钉在一起。在 Go 层枚举的床之前插层会整体平移这段区间，那两处
/// 守卫就是仅有的报警点。
pub const BED_MATERIAL_FIRST: u16 = 60;
/// 床材质层闭区间的上界（东向床头），见 [`BED_MATERIAL_FIRST`]。
pub const BED_MATERIAL_LAST: u16 = 67;

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
/// 采掘裂纹 overlay pass(目标方块六面的透明 cutout 裂纹层)。
pub const CRACK: &str = include_str!("../../shaders/crack.wgsl");
/// 调试面板 pass。
pub const DEBUG_PANEL: &str = include_str!("../../shaders/debug_panel.wgsl");
/// HUD(hotbar 家族)pass。
pub const HUD_HOTBAR: &str = include_str!("../../shaders/hotbar.wgsl");

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn human_material_faces_are_isolated_from_existing_entities() {
        assert!(AVATAR.contains("material >= 112u && material < 160u"));
        assert!(AVATAR.contains("return material + face;"));
        assert!(AVATAR.contains("out.material = avatar_face_material(instance.material, face);"));
        assert!(AVATAR.contains("default: { return vec2f(local.x + 0.5, 0.5 - local.y); }"));
    }

    /// 物品薄片只保留本地 ±Z 大面；材质边界严格夹在牛头层之后、人物层之前，
    /// 避免把方块、牛、人物或纯色粒子的窄面一并丢弃。
    #[test]
    fn item_icon_materials_discard_only_narrow_faces() {
        assert!(AVATAR.contains("fn item_icon_material(material: u32) -> bool"));
        assert!(AVATAR.contains("material >= 81u && material < 112u"));
        assert!(AVATAR.contains("@location(5)       face:     u32"));
        assert!(AVATAR.contains("if (item_icon_material(in.material) && in.face < 4u)"));
    }

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
            ("crack", CRACK),
            ("debug_panel", DEBUG_PANEL),
            ("hud_hotbar", HUD_HOTBAR),
        ] {
            assert!(!source.trim().is_empty(), "{name} 为空");
            assert!(source.contains("fn "), "{name} 缺少入口函数");
        }
    }
}
