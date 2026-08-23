//! greedy mesh 测试的共享夹具中心。
//!
//! 这里原先内联在 `greedy.rs` 的 `mod tests` 头尾：`base_input`/`set_block`/
//! `parse`/`dark_light` 等夹具与 `REGISTRY_OFFSET` 等布局常量被贪心合并、水面
//! 角高度、植物、区段边界多个主题测试文件共用，按「被多于一个测试文件引用的
//! helper 迁入共享中心」的编排规则集中到这里。跨文件成员的可见性跟随
//! `input.rs::tests` 的 `pub(crate)` 先例；仅本中心内部使用的常量保持私有。
//! `fill_slab`/`GLASS_ID` 等单主题私有的夹具留在各自的主题测试文件里。

use crate::input::{MeshInput, tests::valid_input};
use crate::light::{LIGHT_VOLUME, LightScratch};

const BLOCKS_OFFSET: usize = 16;
const BLOCKS_BYTES: usize = 27 * 4096 * 2;
const HEIGHTS_PRESENT_OFFSET: usize = BLOCKS_OFFSET + BLOCKS_BYTES;
const HEIGHTS_BYTES: usize = 9 + 9 * 256 * 2;
pub(crate) const REGISTRY_OFFSET: usize = HEIGHTS_PRESENT_OFFSET + HEIGHTS_BYTES;
pub(crate) const ENTRY_BYTES: usize = crate::input::tests::ENTRY_BYTES;
pub(crate) const STONE_ID: u16 = 1;

pub(crate) fn base_input() -> Vec<u8> {
    let mut bytes = valid_input();
    bytes[4..8].copy_from_slice(&0_i32.to_le_bytes());
    bytes[BLOCKS_OFFSET..BLOCKS_OFFSET + BLOCKS_BYTES].fill(0);
    bytes[HEIGHTS_PRESENT_OFFSET..REGISTRY_OFFSET].fill(0);
    bytes[REGISTRY_OFFSET + 2 * ENTRY_BYTES + 3] = 0;
    bytes[REGISTRY_OFFSET + 2 * ENTRY_BYTES + 16] = 0;
    set_visibility(&mut bytes, 5, 1);
    bytes
}

pub(crate) fn set_visibility(bytes: &mut [u8], stone: u64, glass: u64) {
    let visibility = REGISTRY_OFFSET + 3 * ENTRY_BYTES;
    bytes[visibility..visibility + 8].copy_from_slice(&0_u64.to_le_bytes());
    bytes[visibility + 8..visibility + 16].copy_from_slice(&stone.to_le_bytes());
    bytes[visibility + 16..visibility + 24].copy_from_slice(&glass.to_le_bytes());
}

pub(crate) fn set_block(bytes: &mut [u8], x: i32, y: i32, z: i32, id: u16) {
    let (cx, lx) = neighbor_cell(x);
    let (cy, ly) = neighbor_cell(y);
    let (cz, lz) = neighbor_cell(z);
    let section = (cx * 3 + cy) * 3 + cz;
    let cell = (ly << 8) | (lz << 4) | lx;
    let offset = BLOCKS_OFFSET + (section * 4096 + cell) * 2;
    bytes[offset..offset + 2].copy_from_slice(&id.to_le_bytes());
}

fn neighbor_cell(value: i32) -> (usize, usize) {
    let shifted = value + 16;
    ((shifted >> 4) as usize, (shifted & 15) as usize)
}

pub(crate) fn parse(bytes: Vec<u8>) -> MeshInput<'static> {
    MeshInput::parse(Box::leak(bytes.into_boxed_slice())).unwrap()
}

pub(crate) fn dark_light() -> LightScratch<'static> {
    LightScratch::new(
        Box::leak(vec![0; LIGHT_VOLUME].into_boxed_slice()),
        Box::leak(vec![0; LIGHT_VOLUME].into_boxed_slice()),
    )
}
