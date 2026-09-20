//! `save.region`: region-file format primitives.
//!
//! Ported from the Go `region` storage codec. A region file is a fixed
//! superblock followed by two mutually-backing banks; a chunk payload lives in
//! the data area after the fixed headers. Selection prefers the newest valid
//! bank and ties break to bank A, so a crash always leaves a committed index.

use crate::bytes::is_zero;
use crate::crc32c::{crc32c, crc32c_join};
use crate::error::{StorageResult, corrupt, future_version};

/// Sector granularity of a region file; every extent offset and length is a
/// multiple of this.
pub const SECTOR_SIZE: u32 = 4096;

/// First sector after the fixed superblock and the two banks.
pub const DATA_START_SECTOR: u32 = 15;

/// Sector index where bank A starts.
pub const BANK_A_START_SECTOR: u32 = 1;

/// Sector index where bank B starts.
pub const BANK_B_START_SECTOR: u32 = 8;

/// Number of sectors one bank occupies.
const BANK_SECTORS: u32 = 7;

/// Byte size of one bank.
pub const BANK_SIZE: usize = BANK_SECTORS as usize * SECTOR_SIZE as usize;

/// Number of chunk slots in one bank (`32 * 32`).
pub const REGION_SLOTS: usize = 32 * 32;

/// Byte size of one bank entry.
const REGION_ENTRY_SIZE: usize = 24;

/// Largest compressed chunk payload a single region entry may hold.
pub const MAX_COMPRESSED_CHUNK: u32 = 1 << 20;

/// Current region format version.
pub const CURRENT_VERSION: u32 = 1;

const SUPERBLOCK_MAGIC: [u8; 4] = *b"MCGR";
const BANK_MAGIC: [u8; 4] = *b"MCGB";
const SUPERBLOCK_LENGTH: usize = SECTOR_SIZE as usize;
const SUPERBLOCK_CRC_OFFSET: usize = SECTOR_SIZE as usize - 4;
const BANK_HEADER_SIZE: usize = 64;

const SUPER_MAGIC: usize = 0;
const SUPER_VERSION: usize = 4;
const SUPER_SECTOR_SIZE: usize = 8;
const SUPER_DIMENSION: usize = 12;
const SUPER_REGION_X: usize = 16;
const SUPER_REGION_Z: usize = 20;
const SUPER_BANK_A_START: usize = 24;
const SUPER_BANK_B_START: usize = 28;
const SUPER_BANK_SECTORS: usize = 32;
const SUPER_DATA_START: usize = 36;
const SUPER_RESERVED: usize = 40;

const BANK_MAGIC_OFFSET: usize = 0;
const BANK_VERSION: usize = 4;
const BANK_SECTOR_SIZE: usize = 8;
const BANK_DIMENSION: usize = 12;
const BANK_REGION_X: usize = 16;
const BANK_REGION_Z: usize = 20;
const BANK_GENERATION: usize = 24;
const BANK_ENTRY_COUNT: usize = 32;
const BANK_ENTRY_SIZE: usize = 36;
const BANK_SECTORS_OFFSET: usize = 40;
const BANK_DATA_START: usize = 44;
const BANK_RESERVED: usize = 48;
const BANK_CRC_OFFSET: usize = BANK_HEADER_SIZE - 4;
const BANK_ENTRIES: usize = BANK_HEADER_SIZE;
const BANK_PADDING: usize = BANK_ENTRIES + REGION_SLOTS * REGION_ENTRY_SIZE;

const ENTRY_OFFSET_SECTOR: usize = 0;
const ENTRY_SECTOR_COUNT: usize = 4;
const ENTRY_PAYLOAD_LENGTH: usize = 8;
const ENTRY_REVISION: usize = 12;
const ENTRY_PAYLOAD_CRC32C: usize = 20;

/// Identifies one region file: a dimension plus region coordinates (one
/// region spans 32×32 chunks).
///
/// The dimension is a raw `i32` because the on-disk field is one; the domain
/// dimension type cannot represent the negative IDs the format tests freeze.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct RegionKey {
    pub dimension: i32,
    pub x: i32,
    pub z: i32,
}

/// One chunk slot inside a region bank. `offset_sector` of zero means empty.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct Entry {
    pub offset_sector: u32,
    pub sector_count: u32,
    pub payload_length: u32,
    pub revision: u64,
    pub payload_crc32c: u32,
}

/// One of the two backing bank indexes inside a region file.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Bank {
    pub generation: u64,
    pub entries: Vec<Entry>,
}

/// A chunk coordinate used to derive the owning region and slot.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ChunkKey {
    pub dimension: i32,
    pub x: i32,
    pub z: i32,
}

impl Bank {
    /// An empty bank with every slot absent.
    pub fn empty() -> Self {
        Self {
            generation: 0,
            entries: vec![Entry::default(); REGION_SLOTS],
        }
    }
}

/// Encodes the region superblock for `key`.
pub fn encode_superblock(key: RegionKey) -> [u8; SUPERBLOCK_LENGTH] {
    let mut encoded = [0u8; SUPERBLOCK_LENGTH];
    encoded[SUPER_MAGIC..SUPER_MAGIC + 4].copy_from_slice(&SUPERBLOCK_MAGIC);
    put_u32(&mut encoded, SUPER_VERSION, CURRENT_VERSION);
    put_u32(&mut encoded, SUPER_SECTOR_SIZE, SECTOR_SIZE);
    put_u32(&mut encoded, SUPER_DIMENSION, key.dimension as u32);
    put_u32(&mut encoded, SUPER_REGION_X, key.x as u32);
    put_u32(&mut encoded, SUPER_REGION_Z, key.z as u32);
    put_u32(&mut encoded, SUPER_BANK_A_START, BANK_A_START_SECTOR);
    put_u32(&mut encoded, SUPER_BANK_B_START, BANK_B_START_SECTOR);
    put_u32(&mut encoded, SUPER_BANK_SECTORS, BANK_SECTORS);
    put_u32(&mut encoded, SUPER_DATA_START, DATA_START_SECTOR);
    let checksum = crc32c(&encoded[..SUPERBLOCK_CRC_OFFSET]);
    put_u32(&mut encoded, SUPERBLOCK_CRC_OFFSET, checksum);
    encoded
}

/// Decodes and validates the region superblock against `key`.
pub fn decode_superblock(key: RegionKey, encoded: &[u8]) -> StorageResult<()> {
    if encoded.len() != SUPERBLOCK_LENGTH {
        return Err(corrupt(
            "superblock length",
            format!("{}, want {SUPERBLOCK_LENGTH}", encoded.len()),
        ));
    }
    if encoded[SUPER_MAGIC..SUPER_MAGIC + 4] != SUPERBLOCK_MAGIC {
        return Err(corrupt("superblock magic", "unexpected magic"));
    }
    let version = u32_at(encoded, SUPER_VERSION);
    if version > CURRENT_VERSION {
        return Err(future_version("region version", version));
    }
    if version != CURRENT_VERSION {
        return Err(corrupt(
            "region version",
            format!("unsupported version {version}"),
        ));
    }
    if crc32c(&encoded[..SUPERBLOCK_CRC_OFFSET]) != u32_at(encoded, SUPERBLOCK_CRC_OFFSET) {
        return Err(corrupt("superblock CRC32C", "checksum mismatch"));
    }
    if u32_at(encoded, SUPER_SECTOR_SIZE) != SECTOR_SIZE
        || u32_at(encoded, SUPER_BANK_A_START) != BANK_A_START_SECTOR
        || u32_at(encoded, SUPER_BANK_B_START) != BANK_B_START_SECTOR
        || u32_at(encoded, SUPER_BANK_SECTORS) != BANK_SECTORS
        || u32_at(encoded, SUPER_DATA_START) != DATA_START_SECTOR
    {
        return Err(corrupt("superblock fixed geometry", "unexpected layout"));
    }
    if u32_at(encoded, SUPER_DIMENSION) as i32 != key.dimension
        || u32_at(encoded, SUPER_REGION_X) as i32 != key.x
        || u32_at(encoded, SUPER_REGION_Z) as i32 != key.z
    {
        return Err(corrupt("superblock region key", "key mismatch"));
    }
    if !is_zero(&encoded[SUPER_RESERVED..SUPERBLOCK_CRC_OFFSET]) {
        return Err(corrupt("superblock reserved bytes", "nonzero"));
    }
    Ok(())
}

/// Encodes one region bank for `key`, rejecting structures that could never be
/// read back.
pub fn encode_region_bank(key: RegionKey, bank: &Bank) -> StorageResult<[u8; BANK_SIZE]> {
    validate_region_bank(bank, 0, false)?;
    let mut encoded = [0u8; BANK_SIZE];
    encoded[BANK_MAGIC_OFFSET..BANK_MAGIC_OFFSET + 4].copy_from_slice(&BANK_MAGIC);
    put_u32(&mut encoded, BANK_VERSION, CURRENT_VERSION);
    put_u32(&mut encoded, BANK_SECTOR_SIZE, SECTOR_SIZE);
    put_u32(&mut encoded, BANK_DIMENSION, key.dimension as u32);
    put_u32(&mut encoded, BANK_REGION_X, key.x as u32);
    put_u32(&mut encoded, BANK_REGION_Z, key.z as u32);
    encoded[BANK_GENERATION..BANK_GENERATION + 8].copy_from_slice(&bank.generation.to_le_bytes());
    put_u32(&mut encoded, BANK_ENTRY_COUNT, REGION_SLOTS as u32);
    put_u32(&mut encoded, BANK_ENTRY_SIZE, REGION_ENTRY_SIZE as u32);
    put_u32(&mut encoded, BANK_SECTORS_OFFSET, BANK_SECTORS);
    put_u32(&mut encoded, BANK_DATA_START, DATA_START_SECTOR);
    for (slot, entry) in bank.entries.iter().enumerate() {
        let offset = BANK_ENTRIES + slot * REGION_ENTRY_SIZE;
        put_u32(
            &mut encoded,
            offset + ENTRY_OFFSET_SECTOR,
            entry.offset_sector,
        );
        put_u32(
            &mut encoded,
            offset + ENTRY_SECTOR_COUNT,
            entry.sector_count,
        );
        put_u32(
            &mut encoded,
            offset + ENTRY_PAYLOAD_LENGTH,
            entry.payload_length,
        );
        encoded[offset + ENTRY_REVISION..offset + ENTRY_REVISION + 8]
            .copy_from_slice(&entry.revision.to_le_bytes());
        put_u32(
            &mut encoded,
            offset + ENTRY_PAYLOAD_CRC32C,
            entry.payload_crc32c,
        );
    }
    let checksum = region_bank_checksum(&encoded);
    put_u32(&mut encoded, BANK_CRC_OFFSET, checksum);
    Ok(encoded)
}

/// Decodes and validates one region bank. `file_size` is the physical region
/// file size in bytes and is only consulted when `check_file_size` is set.
pub fn decode_region_bank(key: RegionKey, encoded: &[u8], file_size: i64) -> StorageResult<Bank> {
    if encoded.len() != BANK_SIZE {
        return Err(corrupt(
            "region bank length",
            format!("{}, want {BANK_SIZE}", encoded.len()),
        ));
    }
    if encoded[BANK_MAGIC_OFFSET..BANK_MAGIC_OFFSET + 4] != BANK_MAGIC {
        return Err(corrupt("region bank magic", "unexpected magic"));
    }
    let version = u32_at(encoded, BANK_VERSION);
    if version > CURRENT_VERSION {
        return Err(future_version("region bank version", version));
    }
    if version != CURRENT_VERSION {
        return Err(corrupt(
            "region bank version",
            format!("unsupported version {version}"),
        ));
    }
    if region_bank_checksum(encoded) != u32_at(encoded, BANK_CRC_OFFSET) {
        return Err(corrupt("region bank CRC32C", "checksum mismatch"));
    }
    if u32_at(encoded, BANK_SECTOR_SIZE) != SECTOR_SIZE
        || u32_at(encoded, BANK_ENTRY_COUNT) != REGION_SLOTS as u32
        || u32_at(encoded, BANK_ENTRY_SIZE) != REGION_ENTRY_SIZE as u32
        || u32_at(encoded, BANK_SECTORS_OFFSET) != BANK_SECTORS
        || u32_at(encoded, BANK_DATA_START) != DATA_START_SECTOR
    {
        return Err(corrupt("region bank fixed geometry", "unexpected layout"));
    }
    if u32_at(encoded, BANK_DIMENSION) as i32 != key.dimension
        || u32_at(encoded, BANK_REGION_X) as i32 != key.x
        || u32_at(encoded, BANK_REGION_Z) as i32 != key.z
    {
        return Err(corrupt("region bank key", "key mismatch"));
    }
    if !is_zero(&encoded[BANK_RESERVED..BANK_CRC_OFFSET]) {
        return Err(corrupt("region bank reserved bytes", "nonzero"));
    }
    if !is_zero(&encoded[BANK_PADDING..]) {
        return Err(corrupt("region bank padding", "nonzero"));
    }
    let generation = u64_at(encoded, BANK_GENERATION);
    let mut entries = vec![Entry::default(); REGION_SLOTS];
    for (slot, entry) in entries.iter_mut().enumerate() {
        let offset = BANK_ENTRIES + slot * REGION_ENTRY_SIZE;
        *entry = Entry {
            offset_sector: u32_at(encoded, offset + ENTRY_OFFSET_SECTOR),
            sector_count: u32_at(encoded, offset + ENTRY_SECTOR_COUNT),
            payload_length: u32_at(encoded, offset + ENTRY_PAYLOAD_LENGTH),
            revision: u64_at(encoded, offset + ENTRY_REVISION),
            payload_crc32c: u32_at(encoded, offset + ENTRY_PAYLOAD_CRC32C),
        };
    }
    let bank = Bank {
        generation,
        entries,
    };
    validate_region_bank(&bank, file_size, true)?;
    Ok(bank)
}

/// Picks the newest committed bank between the two decoded copies.
///
/// Each side carries its own decode result: a decode failure is a fact about
/// that copy, not about the file. A structurally valid bank with generation
/// zero is an uncommitted standby and is treated as invalid, exactly as the
/// Go selector does.
pub fn select_region_bank(
    bank_a: StorageResult<Bank>,
    bank_b: StorageResult<Bank>,
) -> StorageResult<(Bank, usize)> {
    let bank_a = standby_is_invalid(bank_a);
    let bank_b = standby_is_invalid(bank_b);
    match (bank_a, bank_b) {
        (Ok(bank_a), Ok(bank_b)) => {
            if bank_a.generation > bank_b.generation {
                return Ok((bank_a, 0));
            }
            if bank_b.generation > bank_a.generation {
                return Ok((bank_b, 1));
            }
            // Decoding accepts only canonical zero-filled headers and padding,
            // so equal decoded values are byte-identical banks for one key.
            if bank_a == bank_b {
                return Ok((bank_a, 0));
            }
            Err(corrupt(
                "region banks",
                format!("divergent at generation {}", bank_a.generation),
            ))
        }
        (Err(_), Ok(bank_b)) => Ok((bank_b, 1)),
        (Ok(bank_a), Err(_)) => Ok((bank_a, 0)),
        (Err(err_a), Err(err_b)) => Err(corrupt(
            "region banks",
            format!("both region banks invalid: bank A: {err_a}; bank B: {err_b}"),
        )),
        (Err(err_a), Ok(_)) => Err(corrupt(
            "region banks",
            format!("bank A is invalid: {err_a}"),
        )),
    }
}

/// Derives the owning region and the in-bank slot index for a chunk key.
pub fn region_for(key: ChunkKey) -> (RegionKey, usize) {
    let (rx, lx) = floor_div32(key.x);
    let (rz, lz) = floor_div32(key.z);
    (
        RegionKey {
            dimension: key.dimension,
            x: rx,
            z: rz,
        },
        (lz * 32 + lx) as usize,
    )
}

fn floor_div32(value: i32) -> (i32, i32) {
    let wide = i64::from(value);
    let quotient = if wide >= 0 {
        wide / 32
    } else {
        -((-wide + 31) / 32)
    };
    (quotient as i32, value - quotient as i32 * 32)
}

fn standby_is_invalid(bank: StorageResult<Bank>) -> StorageResult<Bank> {
    match bank {
        Ok(bank) if bank.generation == 0 => Err(corrupt("region bank", "uncommitted standby")),
        other => other,
    }
}

fn validate_region_bank(bank: &Bank, file_size: i64, check_file_size: bool) -> StorageResult<()> {
    if check_file_size && file_size < i64::from(DATA_START_SECTOR) * i64::from(SECTOR_SIZE) {
        return Err(corrupt("region file", "shorter than the fixed headers"));
    }
    let mut ranges: Vec<(u64, u64)> = Vec::new();
    for (slot, entry) in bank.entries.iter().enumerate() {
        if entry.offset_sector == 0 {
            if *entry != Entry::default() {
                return Err(corrupt(
                    "region entry",
                    format!("absent slot {slot} has nonzero fields"),
                ));
            }
            continue;
        }
        if bank.generation == 0 {
            return Err(corrupt("region bank", "generation zero bank is not empty"));
        }
        if entry.sector_count == 0 {
            return Err(corrupt(
                "region entry",
                format!("slot {slot} has zero sector count"),
            ));
        }
        if entry.payload_length > MAX_COMPRESSED_CHUNK {
            return Err(corrupt(
                "region entry",
                format!("slot {slot} payload exceeds limit"),
            ));
        }
        if u64::from(entry.payload_length) > u64::from(entry.sector_count) * u64::from(SECTOR_SIZE)
        {
            return Err(corrupt(
                "region entry",
                format!("slot {slot} payload exceeds extent"),
            ));
        }
        if entry.revision == 0 {
            return Err(corrupt(
                "region entry",
                format!("slot {slot} has zero revision"),
            ));
        }
        let first = u64::from(entry.offset_sector);
        let end = first + u64::from(entry.sector_count);
        if end > u64::from(u32::MAX) {
            return Err(corrupt(
                "region entry",
                format!("slot {slot} extent overflows uint32"),
            ));
        }
        ranges.push((first, end));
    }

    ranges.sort_by_key(|range| range.0);
    for (index, range) in ranges.iter().enumerate() {
        if range.0 < u64::from(DATA_START_SECTOR) || range.0 >= range.1 {
            return Err(corrupt("region entry", "invalid extent"));
        }
        if check_file_size && range.1 > file_size as u64 / u64::from(SECTOR_SIZE) {
            return Err(corrupt("region entry", "invalid extent"));
        }
        if index > 0 && ranges[index - 1].1 > range.0 {
            return Err(corrupt("region entry", "overlapping extents"));
        }
    }
    Ok(())
}

fn region_bank_checksum(encoded: &[u8]) -> u32 {
    // The checksum covers the whole bank with the checksum field itself read
    // as zero, matching the Go `crc32.New(CRCTable)` write sequence.
    crc32c_join(&[
        &encoded[..BANK_CRC_OFFSET],
        &[0, 0, 0, 0],
        &encoded[BANK_CRC_OFFSET + 4..],
    ])
}

fn put_u32(encoded: &mut [u8], offset: usize, value: u32) {
    encoded[offset..offset + 4].copy_from_slice(&value.to_le_bytes());
}

fn u32_at(encoded: &[u8], offset: usize) -> u32 {
    u32::from_le_bytes(
        encoded[offset..offset + 4]
            .try_into()
            .expect("four bytes at a fixed offset"),
    )
}

fn u64_at(encoded: &[u8], offset: usize) -> u64 {
    u64::from_le_bytes(
        encoded[offset..offset + 8]
            .try_into()
            .expect("eight bytes at a fixed offset"),
    )
}
