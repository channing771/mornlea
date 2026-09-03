//! greedy mesh 的 3×3×3 区段边界测试，属 greedy mesh 测试集（`greedy/mod.rs`）。
//!
//! mesher 的方块输入是中心区段加 3×3×3 邻域：这里验证贴边的方块能读到中心
//! 区段之外的邻域格子，跨界邻接照样参与出面判定。共享夹具见
//! `greedy/test_support.rs`（`set_block` 本身就按 3×3×3 邻域布局寻址）。

use super::mesh_section;
use super::test_support::{STONE_ID, base_input, dark_light, parse, set_block};

#[test]
fn missing_neighbor_blocks_the_boundary_face() {
    let mut bytes = base_input();
    set_block(&mut bytes, 0, 8, 8, STONE_ID);
    set_block(&mut bytes, -1, 8, 8, STONE_ID);
    let input = parse(bytes);
    let light = dark_light();
    let mut output = [0; 6];
    let count = mesh_section(&input, &light, &mut output).unwrap();

    assert_eq!(count, 5);
    assert!(
        !output[..count]
            .iter()
            .any(|packed| packed & 0xf == 0 && (packed >> 20) & 7 == 0)
    );
}
