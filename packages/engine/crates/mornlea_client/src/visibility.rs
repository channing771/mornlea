//! 可见区段 BFS 内核：Go `mesh.VisibleSectionsInto` 的逐语句转写。
//!
//! 位布局与遍历顺序是跨语言渲染契约：`conn` 切片携带与 Go
//! `mesh.Connectivity` 逐位一致的 15 位掩码（`a<b` 时位号为
//! `a*5-a*(a-1)/2+b-a-1`，`a==b` 恒连通）；BFS 发射顺序（起点先发、
//! 队列按面序展开、`emitted` 去重而 `seen` 按进入面位多次调度、未加载
//! 节点保留发射但跳过展开）与 Go 逐语句一致，调用方依赖完全相同的
//! 候选顺序做增量网格调度。
//!
//! 本模块是纯计算，不依赖窗口与 GPU，各目标均可编译。FFI 两段式出口
//! 住在 [`crate::ffi`]（与既有 `render_*` 同域）；线程局部复用缓冲使
//! 预热后稳态零分配。

use std::cell::RefCell;

/// 每区块区段数，与 Go `core.SectionsPerChunk` 同值。
pub const SECTIONS_PER_CHUNK: i32 = 24;
/// 世界最低方块 `Y`，与 Go `core.MinY` 同值（区段包围盒最小角还原用）。
pub const MIN_Y: i32 = -64;
/// 区段边长（方块），与 Go `core.SectionSize` 同值。
pub const SECTION_SIZE: i32 = 16;
/// 可见性半径上限（FFI 契约）；内核本身只拒绝负半径（与 Go 一致）。
pub const MAX_RADIUS: i32 = 32;
/// 单次调用允许的最大连通表条目，为半径上限下的全填充格数。
pub const MAX_CONN_ENTRIES: usize = 65 * 65 * SECTIONS_PER_CHUNK as usize;

/// 起点标记位，与 Go `VisibleSectionsInto` 的 `originBit` 同值。
const ORIGIN_BIT: u8 = 1 << 6;
/// 已入队标记位，与 Go 的 `scheduledBit` 同值。
const SCHEDULED_BIT: u8 = 1 << 7;
/// 起点允许的全部 6 个出口，与 Go 的 `allowedExits = 0x3f` 同值。
const ALL_EXITS: u8 = 0x3f;

/// 面对 `(a, b)`（要求 `a<b`）在 15 位掩码中的位号，与 Go `mesh.pairBit` 逐位一致。
fn pair_bit(a: u8, b: u8) -> u32 {
    debug_assert!(a < b && b < 6);
    let (a, b) = (a as u32, b as u32);
    // `a=0` 时 `a-1` 在 Go 无符号整数中回绕、随后被乘零吞掉；Rust 用
    // `wrapping_sub` 复现同一数值而不触发调试期溢出断言。
    a * 5 - a * a.wrapping_sub(1) / 2 + b - a - 1
}

/// 两面是否连通；`a==b` 恒为真，与 Go `Connectivity.Connected` 一致。
fn connected(mask: u16, a: u8, b: u8) -> bool {
    if a == b {
        return true;
    }
    let (lo, hi) = if a < b { (a, b) } else { (b, a) };
    mask & (1u16 << pair_bit(lo, hi)) != 0
}

/// 面的反方向（`f^1`），与 Go `opposite` 一致。
fn opposite(face: u8) -> u8 {
    face ^ 1
}

/// 面步进，与 Go `stepOf` 一致：编号规则是轴号 `face>>1`（0=X,1=Y,2=Z），
/// 奇偶定正负（奇为正向 `+1`）。
fn step_of(face: u8) -> [i32; 3] {
    let d = if face & 1 == 1 { 1 } else { -1 };
    match face >> 1 {
        0 => [d, 0, 0],
        1 => [0, d, 0],
        _ => [0, 0, d],
    }
}

/// 区段包围盒，与 Go `sectionAABB` 一致：最小角为
/// `(x*16, y*16-64, z*16)`、边长 16（`Y` 是区段索引而非方块 `Y`，
/// 故纵向回加 `MIN_Y`；最大值同样先整数加再转浮点，与 Go 同舍入）。
fn section_aabb(pos: [i32; 3]) -> ([f32; 3], [f32; 3]) {
    let min = [
        (pos[0] << 4) as f32,
        ((pos[1] << 4) + MIN_Y) as f32,
        (pos[2] << 4) as f32,
    ];
    let max = [
        ((pos[0] << 4) + SECTION_SIZE) as f32,
        (((pos[1] << 4) + MIN_Y) + SECTION_SIZE) as f32,
        ((pos[2] << 4) + SECTION_SIZE) as f32,
    ];
    (min, max)
}

/// 视锥与包围盒相交测试（正顶点），与 Go `Frustum.IntersectsAABB` 一致：
/// 对每个平面取盒子在法线方向最远的角，若它在平面外侧则整个盒子在外侧。
fn intersects_aabb(frustum: &[[f32; 4]; 6], min: [f32; 3], max: [f32; 3]) -> bool {
    for plane in frustum {
        let px = if plane[0] >= 0.0 { max[0] } else { min[0] };
        let py = if plane[1] >= 0.0 { max[1] } else { min[1] };
        let pz = if plane[2] >= 0.0 { max[2] } else { min[2] };
        if plane[0] * px + plane[1] * py + plane[2] * pz + plane[3] < 0.0 {
            return false;
        }
    }
    true
}

/// BFS 密集状态（`loaded`/`mask` 稠密连通表＋`emitted`/`seen`/`pending`/
/// `expanded`/`queue`），与 Go `VisibilityScratch` 同构；线程局部复用，
/// 预热后稳态零分配。
#[derive(Default)]
struct Scratch {
    loaded: Vec<bool>,
    mask: Vec<u16>,
    emitted: Vec<u8>,
    seen: Vec<u8>,
    pending: Vec<u8>,
    expanded: Vec<u8>,
    queue: Vec<usize>,
}

impl Scratch {
    /// 扩容并清零到 `total` 格（只增长不收缩，与 Go 的容量复用语义一致）。
    fn reset(&mut self, total: usize) {
        self.loaded.resize(total, false);
        self.loaded.fill(false);
        self.mask.resize(total, 0);
        self.mask.fill(0);
        self.emitted.resize(total, 0);
        self.emitted.fill(0);
        self.seen.resize(total, 0);
        self.seen.fill(0);
        self.pending.resize(total, 0);
        self.pending.fill(0);
        self.expanded.resize(total, 0);
        self.expanded.fill(0);
        self.queue.clear();
    }
}

thread_local! {
    static SCRATCH: RefCell<Scratch> = RefCell::new(Scratch::default());
    static CONN_BUF: RefCell<Vec<(i32, i32, i32, u16)>> = const { RefCell::new(Vec::new()) };
    static LAST: RefCell<Vec<[i32; 3]>> = const { RefCell::new(Vec::new()) };
}

/// 从相机所在区段做广度优先遍历，返回可见候选区段（发射顺序即 BFS 顺序）。
///
/// 与 Go `mesh.VisibleSectionsInto` 逐语句对应：`origin` 先发射；`conn`
/// 未覆盖的坐标视为未加载（保留发射、跳过展开）；`frustum` 为 6 平面
/// `[x,y,z,w]`（内法线＋有符号距离）；`out` 先清空再追加（对应 Go 的
/// `dst[:0]` 复用语义）。负半径或 `origin` 纵坐标越界返回空（与 Go
/// 一致）；半径上界由 FFI 层按 [`MAX_RADIUS`] 强制，内核不设上限。
pub fn select_visible(
    origin: [i32; 3],
    radius: i32,
    frustum: [[f32; 4]; 6],
    conn: &[(i32, i32, i32, u16)],
    out: &mut Vec<[i32; 3]>,
) {
    out.clear();
    if radius < 0 || origin[1] < 0 || origin[1] >= SECTIONS_PER_CHUNK {
        return;
    }
    SCRATCH.with(|cell| {
        let mut s = cell.borrow_mut();
        let width = 2 * radius + 1;
        let total = (width as usize) * (width as usize) * (SECTIONS_PER_CHUNK as usize);
        s.reset(total);
        let base_x = origin[0] - radius;
        let base_z = origin[2] - radius;
        // 稠密连通表：`conn` 条目落到界内格才生效（界外忽略），重复条目后写为准。
        for &(x, y, z, mask) in conn {
            if !(0..SECTIONS_PER_CHUNK).contains(&y) {
                continue;
            }
            let dx = x - base_x;
            let dz = z - base_z;
            if dx < 0 || dx >= width || dz < 0 || dz >= width {
                continue;
            }
            let index = ((dz * width + dx) * SECTIONS_PER_CHUNK + y) as usize;
            s.loaded[index] = true;
            s.mask[index] = mask;
        }
        // `indexOf(origin)`：相对坐标恒为 `(radius, radius)`。
        let origin_index = ((radius * width + radius) * SECTIONS_PER_CHUNK + origin[1]) as usize;
        s.queue.push(origin_index);
        s.pending[origin_index] = ORIGIN_BIT | SCHEDULED_BIT;
        s.emitted[origin_index] = 1;
        out.push(origin);

        let mut head = 0;
        while head < s.queue.len() {
            let cur = s.queue[head];
            head += 1;
            let entries = s.pending[cur] & !SCHEDULED_BIT;
            s.pending[cur] = 0;
            // `positionOf` 的逆运算：`y=index%24`，列号回解 `x/z`。
            let y = (cur % SECTIONS_PER_CHUNK as usize) as i32;
            let column = cur / SECTIONS_PER_CHUNK as usize;
            let cx = base_x + (column % width as usize) as i32;
            let cz = base_z + (column / width as usize) as i32;
            // 未加载：保留发射但跳过展开（与 Go 的 `!loaded → continue` 一致）。
            if !s.loaded[cur] {
                continue;
            }
            let mask = s.mask[cur];
            let mut allowed = if entries & ORIGIN_BIT != 0 {
                ALL_EXITS
            } else {
                let mut exits = 0u8;
                for entry in 0..6u8 {
                    if entries & (1 << entry) == 0 {
                        continue;
                    }
                    for exit in 0..6u8 {
                        if connected(mask, opposite(entry), exit) {
                            exits |= 1 << exit;
                        }
                    }
                }
                exits
            };
            // 已展开过的出口不再调度（对应 Go 的 `allowedExits &^= expanded`）。
            allowed &= !s.expanded[cur];
            s.expanded[cur] |= allowed;
            for exit in 0..6u8 {
                if allowed & (1 << exit) == 0 {
                    continue;
                }
                let d = step_of(exit);
                let (nx, ny, nz) = (cx + d[0], y + d[1], cz + d[2]);
                if !(0..SECTIONS_PER_CHUNK).contains(&ny) {
                    continue;
                }
                // 切比雪夫半径（与 Go 的 `abs32` 同谓词，免去绝对值的溢出边角）。
                if nx - origin[0] > radius || nx - origin[0] < -radius {
                    continue;
                }
                if nz - origin[2] > radius || nz - origin[2] < -radius {
                    continue;
                }
                let (mn, mx) = section_aabb([nx, ny, nz]);
                if !intersects_aabb(&frustum, mn, mx) {
                    continue;
                }
                let nindex =
                    (((nz - base_z) * width + (nx - base_x)) * SECTIONS_PER_CHUNK + ny) as usize;
                if s.emitted[nindex] == 0 {
                    s.emitted[nindex] = 1;
                    out.push([nx, ny, nz]);
                }
                let bit = 1u8 << exit;
                if s.seen[nindex] & bit != 0 {
                    continue;
                }
                s.seen[nindex] |= bit;
                s.pending[nindex] |= bit;
                if s.pending[nindex] & SCHEDULED_BIT == 0 {
                    s.pending[nindex] |= SCHEDULED_BIT;
                    s.queue.push(nindex);
                }
            }
        }
    });
}

/// FFI 两段式查询的计算半段（仅供 [`crate::ffi`]）：把拍平的 `xyz`
///（`3×mask.len()` 个 `i32`）与掩码装配后求解，结果缓存在调用线程本地
/// 并返回可见数；调用方保证 `xyz.len() == 3 * mask.len()`。
pub(crate) fn compute_cached(
    origin: [i32; 3],
    radius: i32,
    frustum: [[f32; 4]; 6],
    xyz: &[i32],
    mask: &[u16],
) -> usize {
    CONN_BUF.with(|conn| {
        let mut conn = conn.borrow_mut();
        conn.clear();
        conn.extend(
            xyz.chunks_exact(3)
                .zip(mask.iter())
                .map(|(t, &m)| (t[0], t[1], t[2], m)),
        );
        LAST.with(|last| {
            let mut last = last.borrow_mut();
            select_visible(origin, radius, frustum, &conn[..], &mut last);
            last.len()
        })
    })
}

/// 最近一次缓存的可见数（`i32` 元素需求为其 3 倍，仅供 [`crate::ffi`]）。
pub(crate) fn last_visible_len() -> usize {
    LAST.with(|last| last.borrow().len())
}

/// FFI 两段式查询的取数半段（仅供 [`crate::ffi`]）：把缓存按 `x,y,z`
/// 三元组展开为 `i32` 流写进 `out`，返回写入的 `i32` 元素数；调用方保证
/// `out.len()` 是 3 的倍数且装得下（调试断言钉住该契约）。
pub(crate) fn fetch_cached(out: &mut [i32]) -> usize {
    LAST.with(|last| {
        let last = last.borrow();
        debug_assert!(out.len() >= last.len() * 3);
        for (i, pos) in last.iter().enumerate() {
            out[3 * i..3 * i + 3].copy_from_slice(pos);
        }
        last.len() * 3
    })
}

/// 清空最近一次缓存（失败的 `len` 调用用它丢弃过期结果，仅供 [`crate::ffi`]）。
pub(crate) fn clear_last() {
    LAST.with(|last| last.borrow_mut().clear());
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 全通过视锥（6 平面恒成立），与 Go `EverythingVisible` 同值。
    const EVERYTHING: [[f32; 4]; 6] = [[0.0, 0.0, 0.0, 1.0]; 6];

    /// Go 侧固定输出逐项抄入（origin=(0,5,0)、radius=1、`x==1` 实墙、
    /// `y∈[4,6]` 全连通、其余实心；一切变体见 `GO_TIGHT`）。
    const GO_EVERYTHING: [[i32; 3]; 39] = [
        [0, 5, 0],
        [-1, 5, 0],
        [1, 5, 0],
        [0, 4, 0],
        [0, 6, 0],
        [0, 5, -1],
        [0, 5, 1],
        [-1, 4, 0],
        [-1, 6, 0],
        [-1, 5, -1],
        [-1, 5, 1],
        [1, 4, 0],
        [0, 3, 0],
        [0, 4, -1],
        [0, 4, 1],
        [1, 6, 0],
        [0, 7, 0],
        [0, 6, -1],
        [0, 6, 1],
        [1, 5, -1],
        [1, 5, 1],
        [-1, 3, 0],
        [-1, 4, -1],
        [-1, 4, 1],
        [-1, 7, 0],
        [-1, 6, -1],
        [-1, 6, 1],
        [1, 4, -1],
        [0, 3, -1],
        [1, 4, 1],
        [0, 3, 1],
        [1, 6, -1],
        [0, 7, -1],
        [1, 6, 1],
        [0, 7, 1],
        [-1, 3, -1],
        [-1, 3, 1],
        [-1, 7, -1],
        [-1, 7, 1],
    ];

    /// 同一装置、视锥首平面换为 `[-1,0,0,0]`（`Min.x>0` 即 `x>=1` 的区段
    /// 整体剔除，恰好拿掉 9 个墙面发射）的 Go 侧输出。
    const GO_TIGHT: [[i32; 3]; 30] = [
        [0, 5, 0],
        [-1, 5, 0],
        [0, 4, 0],
        [0, 6, 0],
        [0, 5, -1],
        [0, 5, 1],
        [-1, 4, 0],
        [-1, 6, 0],
        [-1, 5, -1],
        [-1, 5, 1],
        [0, 3, 0],
        [0, 4, -1],
        [0, 4, 1],
        [0, 7, 0],
        [0, 6, -1],
        [0, 6, 1],
        [-1, 3, 0],
        [-1, 4, -1],
        [-1, 4, 1],
        [-1, 7, 0],
        [-1, 6, -1],
        [-1, 6, 1],
        [0, 3, -1],
        [0, 3, 1],
        [0, 7, -1],
        [0, 7, 1],
        [-1, 3, -1],
        [-1, 3, 1],
        [-1, 7, -1],
        [-1, 7, 1],
    ];

    /// 与 Go 装置一致的连通表：半径盒内 `x==1` 为实墙（掩码 0 但已加载）、
    /// `y∈[4,6]` 全连通（15 位全 1）、其余实心（掩码 0）。
    fn fixture_conn() -> Vec<(i32, i32, i32, u16)> {
        let mut conn = Vec::new();
        for x in -1..=1 {
            for y in 0..SECTIONS_PER_CHUNK {
                for z in -1..=1 {
                    let mask = if x == 1 {
                        0
                    } else if (4..=6).contains(&y) {
                        0x7FFF
                    } else {
                        0
                    };
                    conn.push((x, y, z, mask));
                }
            }
        }
        conn
    }

    #[test]
    fn visibility_parity_with_go_everything() {
        let conn = fixture_conn();
        let mut out = Vec::new();
        select_visible([0, 5, 0], 1, EVERYTHING, &conn, &mut out);
        assert_eq!(out.len(), GO_EVERYTHING.len(), "发射数量与 Go 不一致");
        assert_eq!(out, GO_EVERYTHING, "发射顺序与 Go 不一致");
    }

    #[test]
    fn visibility_parity_with_go_tight_frustum() {
        let mut tight = EVERYTHING;
        tight[0] = [-1.0, 0.0, 0.0, 0.0];
        let conn = fixture_conn();
        let mut out = Vec::new();
        select_visible([0, 5, 0], 1, tight, &conn, &mut out);
        assert_eq!(out.len(), GO_TIGHT.len(), "剔除后数量与 Go 不一致");
        assert_eq!(out, GO_TIGHT, "剔除后顺序与 Go 不一致");
    }

    #[test]
    fn visibility_pair_bit_covers_all_fifteen_pairs() {
        let mut bits = std::collections::BTreeSet::new();
        for a in 0..6u8 {
            for b in a + 1..6u8 {
                bits.insert(pair_bit(a, b));
            }
        }
        assert_eq!(bits.len(), 15, "15 个面对必须占满 0..14");
        assert_eq!(*bits.iter().next().unwrap(), 0);
        assert_eq!(*bits.iter().next_back().unwrap(), 14);
        assert_eq!(pair_bit(0, 1), 0);
        assert_eq!(pair_bit(4, 5), 14);
    }

    #[test]
    fn visibility_connected_matches_go_semantics() {
        assert!(connected(0, 2, 2), "同面恒连通");
        assert!(!connected(0, 0, 1), "零掩码下异面不连通");
        assert!(connected(0x7FFF, 0, 5), "全 1 掩码下异面连通");
        assert_eq!(connected(0x7FFF, 1, 4), connected(0x7FFF, 4, 1));
        assert_eq!(opposite(0), 1);
        assert_eq!(opposite(5), 4);
        assert_eq!(step_of(0), [-1, 0, 0]);
        assert_eq!(step_of(1), [1, 0, 0]);
        assert_eq!(step_of(2), [0, -1, 0]);
        assert_eq!(step_of(5), [0, 0, 1]);
    }

    #[test]
    fn visibility_missing_lookup_emits_without_expansion() {
        // 空连通表：起点未加载也发射，但无从展开，结果只有起点自身。
        let mut out = vec![[9, 9, 9]];
        select_visible([0, 5, 0], 2, EVERYTHING, &[], &mut out);
        assert_eq!(out, [[0, 5, 0]], "未加载起点应只发射自身并复用 out");
    }

    #[test]
    fn visibility_aabb_positive_vertex_matches_go_branches() {
        // 直测 `intersects_aabb` 的正顶点选取：法线分量为负取最小角，否则取最大角。
        let down = [
            [0.0, 0.0, 0.0, 1.0],
            [0.0, 0.0, 0.0, 1.0],
            [0.0, 0.0, 0.0, 1.0],
            [0.0, 0.0, 0.0, 1.0],
            [0.0, 0.0, 0.0, 1.0],
            [0.0, -1.0, 0.0, 100.0],
        ];
        let (lo_min, lo_max) = section_aabb([0, 5, 0]);
        assert_eq!((lo_min[1], lo_max[1]), (16.0, 32.0));
        assert!(intersects_aabb(&down, lo_min, lo_max));
        let (hi_min, hi_max) = section_aabb([0, 23, 0]);
        assert!(!intersects_aabb(&down, hi_min, hi_max));
        let mut up = EVERYTHING;
        up[1] = [0.0, 1.0, 0.0, 47.0];
        let (bot_min, bot_max) = section_aabb([0, 0, 0]);
        assert!(!intersects_aabb(&up, bot_min, bot_max));
        assert!(intersects_aabb(&up, lo_min, lo_max));
    }

    #[test]
    fn visibility_invalid_origin_or_radius_yields_empty() {
        let conn = fixture_conn();
        let mut out = Vec::new();
        select_visible([0, 5, 0], -1, EVERYTHING, &conn, &mut out);
        assert!(out.is_empty(), "负半径应返回空（与 Go 一致）");
        select_visible([0, 24, 0], 1, EVERYTHING, &conn, &mut out);
        assert!(out.is_empty(), "纵坐标越界应返回空（与 Go 一致）");
        select_visible([0, -1, 0], 1, EVERYTHING, &conn, &mut out);
        assert!(out.is_empty(), "纵坐标为负应返回空（与 Go 一致）");
    }
}
