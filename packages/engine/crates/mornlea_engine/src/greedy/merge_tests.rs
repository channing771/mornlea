//! greedy mesh 的贪心合并与 AO 测试，属 greedy mesh 测试集（`greedy/mod.rs`）。
//!
//! 覆盖：孤立块的六面出面、16×16 顶面贪心合并、不同材质不合并、玻璃可见性
//! 边界、AO 的 Go 角序与四角分辨、quad 输出次序、光照打包采样、输出缓冲不足
//! 报 `OutputOverflow`、全空气早退。共享夹具见 `greedy/test_support.rs`。

use super::test_support::{STONE_ID, base_input, dark_light, parse, set_block, set_visibility};
use super::{MeshError, compute_ao, mesh_section};
use crate::light::{LIGHT_VOLUME, LightScratch};

const GLASS_ID: u16 = 40000;

#[test]
fn isolated_block_produces_six_unit_quads() {
    let mut bytes = base_input();
    set_block(&mut bytes, 8, 8, 8, STONE_ID);
    let input = parse(bytes);
    let light = dark_light();
    let mut output = [0; 6];

    assert_eq!(mesh_section(&input, &light, &mut output), Ok(6));
    for (face, packed) in output.into_iter().enumerate() {
        assert_eq!((packed >> 12) & 0xff, 0);
        assert_eq!((packed >> 20) & 7, face as u64);
        assert_eq!((packed >> 39) & 0xff, 0xff);
    }
    assert_eq!(light.at(8, 8, 8), 0);
}

#[test]
fn flat_sixteen_by_sixteen_top_merges_to_one_quad() {
    let mut bytes = base_input();
    fill_slab(&mut bytes, STONE_ID);
    let input = parse(bytes);
    let light = dark_light();
    let mut output = [0; 1];

    assert_eq!(mesh_section(&input, &light, &mut output), Ok(1));
    assert_eq!((output[0] >> 20) & 7, 3);
    assert_eq!((output[0] >> 12) & 0xf, 15);
    assert_eq!((output[0] >> 16) & 0xf, 15);
}

#[test]
fn two_top_materials_do_not_merge() {
    let mut bytes = base_input();
    fill_slab(&mut bytes, STONE_ID);
    set_visibility(&mut bytes, 1, 1);
    for x in 8..16 {
        for z in 0..16 {
            set_block(&mut bytes, x, 0, z, GLASS_ID);
        }
    }
    let input = parse(bytes);
    let light = dark_light();
    let mut output = [0; 2];

    assert_eq!(mesh_section(&input, &light, &mut output), Ok(2));
    assert_eq!((output[0] >> 12) & 0xff, 0x7f);
    assert_eq!((output[1] >> 12) & 0xff, 0x7f);
    assert_ne!((output[0] >> 23) & 0xffff, (output[1] >> 23) & 0xffff);
}

#[test]
fn stone_glass_boundary_keeps_only_stone_face() {
    let mut bytes = base_input();
    set_block(&mut bytes, 7, 8, 8, STONE_ID);
    set_block(&mut bytes, 8, 8, 8, GLASS_ID);
    let input = parse(bytes);
    let light = dark_light();
    let mut output = [0; 12];
    let count = mesh_section(&input, &light, &mut output).unwrap();

    let stone = output[..count]
        .iter()
        .any(|packed| packed & 0xf == 7 && (packed >> 20) & 7 == 1);
    let glass = output[..count]
        .iter()
        .any(|packed| packed & 0xf == 8 && (packed >> 20) & 7 == 0);
    assert!(stone);
    assert!(!glass);
}

#[test]
fn occluded_top_face_preserves_go_corner_order() {
    let mut bytes = base_input();
    set_block(&mut bytes, 8, 8, 8, STONE_ID);
    set_block(&mut bytes, 7, 9, 8, STONE_ID);
    set_block(&mut bytes, 8, 9, 7, STONE_ID);
    let input = parse(bytes);
    let light = dark_light();
    let mut output = [0; 18];
    let count = mesh_section(&input, &light, &mut output).unwrap();

    let top = output[..count]
        .iter()
        .copied()
        .find(|packed| {
            packed & 0xf == 8
                && (packed >> 4) & 0xf == 8
                && (packed >> 8) & 0xf == 8
                && (packed >> 20) & 7 == 3
        })
        .unwrap();
    assert_eq!((top >> 39) & 0xff, 0xb8);
}

#[test]
fn asymmetric_ao_distinguishes_all_four_corners() {
    let mut bytes = base_input();
    set_block(&mut bytes, 8, 8, 8, STONE_ID);
    set_block(&mut bytes, 7, 9, 8, STONE_ID);
    set_block(&mut bytes, 8, 9, 9, STONE_ID);
    set_block(&mut bytes, 7, 9, 7, STONE_ID);
    let input = parse(bytes);

    let ao = compute_ao(&input, [8, 8, 8], 1, 2, 0, 1);

    assert_eq!(ao, 0xe1);
    assert_eq!(
        [ao & 3, (ao >> 2) & 3, (ao >> 4) & 3, (ao >> 6) & 3],
        [1, 0, 2, 3]
    );
}

#[test]
fn asymmetric_fixture_preserves_complete_face_slice_row_order() {
    let mut bytes = base_input();
    set_block(&mut bytes, 2, 3, 4, STONE_ID);
    set_block(&mut bytes, 5, 1, 7, STONE_ID);
    set_block(&mut bytes, 2, 8, 1, GLASS_ID);
    let input = parse(bytes);
    let light = dark_light();
    let mut output = [0; 18];

    assert_eq!(mesh_section(&input, &light, &mut output), Ok(18));
    assert_eq!(
        output,
        [
            0x007fce20000182,
            0x007f8000800432,
            0x007f8000800715,
            0x007fce20900182,
            0x007f8001100432,
            0x007f8001100715,
            0x007f8001a00715,
            0x007f8001a00432,
            0x007fce21200182,
            0x007f8002300715,
            0x007f8002300432,
            0x007fce21b00182,
            0x007fce22400182,
            0x007f8002c00432,
            0x007f8002c00715,
            0x007fce22d00182,
            0x007f8003500432,
            0x007f8003500715,
        ]
    );
}

#[test]
fn packed_light_samples_each_asymmetric_adjacent_cell() {
    let mut bytes = base_input();
    set_block(&mut bytes, 8, 8, 8, STONE_ID);
    let input = parse(bytes);
    let mut levels = vec![0; LIGHT_VOLUME];
    levels[54168] = 0x12;
    levels[58776] = 0x34;
    levels[56424] = 0x56;
    levels[56520] = 0x78;
    levels[56471] = 0x9a;
    levels[56473] = 0xbc;
    let mut queue = [];
    let light = LightScratch::new(&mut levels, &mut queue);
    let mut output = [0; 6];

    assert_eq!(mesh_section(&input, &light, &mut output), Ok(6));
    assert_eq!(
        output.map(|packed| ((packed >> 47) & 0xff) as u8),
        [0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc]
    );
}

#[test]
fn one_short_output_reports_overflow() {
    let mut bytes = base_input();
    set_block(&mut bytes, 8, 8, 8, STONE_ID);
    let input = parse(bytes);
    let light = dark_light();
    let mut output = [0; 5];

    assert_eq!(
        mesh_section(&input, &light, &mut output),
        Err(MeshError::OutputOverflow)
    );
}

#[test]
fn uniform_air_returns_before_reading_light() {
    let input = parse(base_input());
    let mut levels = [];
    let mut queue = [];
    let light = LightScratch::new(&mut levels, &mut queue);
    let mut output = [0; 1];

    assert_eq!(mesh_section(&input, &light, &mut output), Ok(0));
}

fn fill_slab(bytes: &mut [u8], id: u16) {
    for x in -16..32 {
        for y in -16..=0 {
            for z in -16..32 {
                set_block(bytes, x, y, z, id);
            }
        }
    }
}
