//! 火把有限模型（placeable-torches）的几何发射。
//!
//! 五种形态共用一张竖直火柄纹理（材质 alpha 收窄视觉），mesher 只负责发射
//! 「形态几何」：
//!
//! - **落地（model 1）**：与植物完全同构的两条交叉斜面（face 6/7 × 正背各
//!   一）——这是 8 字节 quad 格式里唯一「格内居中竖直」的表达，两条对角面
//!   都过格心的竖直线，任何水平视角下恰好各留一条。
//! - **墙面（model 2..5）**：三片 quad——承载倾斜的两片轴向薄板 + 贴住支撑
//!   面的一片平帽。薄板顶缘用角高度表达「向远离支撑方向倾斜」（支撑侧
//!   9/16、远离侧 14/16），帽面落在支撑平面上表达「贴近支撑面」。角高度走
//!   与流体/短方块相同的位通道（bit 12..19/55..62，恒 1×1 不贪心合并），
//!   terrain 着色器按材质门控解码；今日渲染里火柄因世界坐标锁定 UV 保持
//!   竖直、倾斜体现在薄板的斜顶边。
//!
//! 与植物相同的口径：光照取正上方相邻格、AO 记满（格内几何没有共面邻居可
//! 采，硬算只会得到与视角无关的脏阴影）；不参与贪心合并；枚举次序
//! `y → z → x` 保证输出确定。

use crate::input::MeshInput;
use crate::light::LightScratch;
use crate::quad::{Face, Quad};

use super::{MeshError, PLANT_QUADS};

/// 落地火把每格面实例数的**固定上界**：两条交叉斜面 × 正背各一。
///
/// 4 等于植物的上界、小于普通方块的 6，整段输出上界不变，Go 侧
/// `maxNativeQuads = 6 * BlocksPerSection` 依旧覆盖最坏情况。数值由
/// `torch_tests` 的固定数量断言钉住（发射函数的循环结构本身只产出这个数），
/// 因此常量只在测试构建里存在。
#[cfg(test)]
pub(crate) const TORCH_QUADS_PER_STANDING_CELL: usize = 4;
/// 墙面火把每格面实例数的**固定上界**：两片倾斜薄板 + 一片贴面帽。
///
/// 同上：3 小于普通方块的 6，由 `torch_tests` 钉住，仅测试构建存在。
#[cfg(test)]
pub(crate) const TORCH_QUADS_PER_WALL_CELL: usize = 3;
/// 墙面火把**支撑侧**顶缘的 4-bit 角高度原值：呈现高度 (8+1)/16 = 9/16。
///
/// 支撑侧低、远离侧高，两角高度差即「向远离支撑方向倾斜」的倾斜度。
pub(crate) const TORCH_WALL_TOP_NEAR_RAW: u8 = 8;
/// 墙面火把**远离支撑侧**顶缘的 4-bit 角高度原值：呈现高度 (13+1)/16 = 14/16。
///
/// 取 13 而不是 14：角高度 14 是耕地顶面的生产取值，避开同一数值在不同语义
/// 间误传；也留出与满格 15（流体水柱内部专用）的余量。
pub(crate) const TORCH_WALL_TOP_FAR_RAW: u8 = 13;

/// mesh_torches 为区段里每个火把格发射有限模型几何，返回新的 quad 总数。
///
/// 这是 model dispatcher 的发射半边：tag 0（默认）不进本函数、继续走既有
/// 几何；tag 6（床，保留）与未知值在 `RegistryView::validate` 的 parse 期就
/// 被拒绝，走不到这里——闭区间 `match` 保持穷尽，未来新增 tag 会在编译期
/// 强制显式处理而不是静默回退。
pub(crate) fn mesh_torches(
    input: &MeshInput<'_>,
    light: &LightScratch<'_>,
    output: &mut [u64],
    mut count: usize,
) -> Result<usize, MeshError> {
    for y in 0..16 {
        for z in 0..16 {
            for x in 0..16 {
                let id = input.block(x, y, z);
                // 空气早退：绝大多数格是空气，先挡掉能省下一次 registry 二分。
                if id == input.air_id {
                    continue;
                }
                let tag = input.registry.model(id);
                if tag == 0 {
                    continue;
                }
                let Some(material) = input.registry.material(id, 0) else {
                    continue;
                };
                let light_above = light.at(x, y + 1, z);
                count = match tag {
                    1 => emit_standing(x, y, z, material, light_above, output, count)?,
                    2..=5 => emit_wall(tag, x, y, z, material, light_above, output, count)?,
                    // validate 已把 > 5（含床 tag 6 与未知值）整体拒绝成
                    // InputError::Registry，这里的 `_` 分支只为穷尽性而存在。
                    _ => unreachable!("model tag 已被 RegistryView::validate 拒绝"),
                };
            }
        }
    }
    Ok(count)
}

/// emit_standing 发射落地形态：两条交叉斜面 × 正背各一，复用植物的
/// `PLANT_QUADS` 编组（face 6/7 + 正背位）。
fn emit_standing(
    x: i32,
    y: i32,
    z: i32,
    material: u16,
    light_above: u8,
    output: &mut [u64],
    mut count: usize,
) -> Result<usize, MeshError> {
    for (face, back) in PLANT_QUADS {
        let Some(slot) = output.get_mut(count) else {
            return Err(MeshError::OutputOverflow);
        };
        *slot = Quad {
            x: x as u8,
            y: y as u8,
            z: z as u8,
            w: 1,
            h: 1,
            face,
            material,
            ao: 0xff,
            light: light_above,
            corners: [0; 4],
            back,
        }
        .pack();
        count += 1;
    }
    Ok(count)
}

/// emit_wall 发射墙面形态：两片倾斜薄板 + 一片贴面帽，次序固定、Go 侧逐条
/// 对齐。
///
/// 角顺序与 `compute_ao` 一致：局部 (u,v) 的 (0,0)(1,0)(1,1)(0,1)。薄板的
/// u 轴是倾斜方向（±X 墙 → Z 法线面上 u=x；±Z 墙 → X 法线面上 v=z），顶缘
/// 两个角分别取 near/far、底缘两角恒 0（贴地）。
fn emit_wall(
    tag: u8,
    x: i32,
    y: i32,
    z: i32,
    material: u16,
    light_above: u8,
    output: &mut [u64],
    mut count: usize,
) -> Result<usize, MeshError> {
    let near = TORCH_WALL_TOP_NEAR_RAW;
    let far = TORCH_WALL_TOP_FAR_RAW;
    // (倾斜薄板的 face 对, 贴面帽 face, 薄板四角)。墙面形态名 = 命中面名，
    // 支撑格在 face.Opposite() 方向：wall +X 的支撑在 −X 侧（近侧 x+0 是
    // 角 3、远侧 x+1 是角 2），其余同理镜像。
    let (plates, cap, corners) = match tag {
        2 => ([Face::NegZ, Face::PosZ], Face::NegX, [0, 0, far, near]),
        3 => ([Face::NegZ, Face::PosZ], Face::PosX, [0, 0, near, far]),
        4 => ([Face::NegX, Face::PosX], Face::NegZ, [0, near, far, 0]),
        _ => ([Face::NegX, Face::PosX], Face::PosZ, [0, far, near, 0]),
    };
    for face in plates {
        let Some(slot) = output.get_mut(count) else {
            return Err(MeshError::OutputOverflow);
        };
        *slot = Quad {
            x: x as u8,
            y: y as u8,
            z: z as u8,
            w: 1,
            h: 1,
            face,
            material,
            ao: 0xff,
            light: light_above,
            corners,
            back: false,
        }
        .pack();
        count += 1;
    }
    let Some(slot) = output.get_mut(count) else {
        return Err(MeshError::OutputOverflow);
    };
    *slot = Quad {
        x: x as u8,
        y: y as u8,
        z: z as u8,
        w: 1,
        h: 1,
        face: cap,
        material,
        ao: 0xff,
        light: light_above,
        corners: [0; 4],
        back: false,
    }
    .pack();
    Ok(count + 1)
}
