use std::collections::BTreeMap;
use std::sync::Arc;

const BATCH_HEADER_BYTES: usize = 24;
const RECORD_HEADER_BYTES: usize = 32;
const SECTION_META_BYTES: usize = 8;
const COLUMN_HEIGHTS: usize = 256;
const COLUMN_PAYLOAD_BYTES: usize = COLUMN_HEIGHTS * size_of::<i16>();
const MAX_BATCH_BYTES: usize = 4 * 1024 * 1024;
const MAX_RECORDS: u32 = 4096;
const SECTIONS_PER_CHUNK: i32 = 24;

const TAG_SECTION_UPSERT: u8 = 1;
const TAG_COLUMN_UPSERT: u8 = 2;
const TAG_SECTION_TOMBSTONE: u8 = 3;
const TAG_COLUMN_TOMBSTONE: u8 = 4;
const TAG_WORLD_RESET: u8 = 5;

const STORAGE_SINGLE: u8 = 0;
const STORAGE_INDEXED: u8 = 1;
const STORAGE_DIRECT: u8 = 2;

#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
pub(super) struct SectionKey {
    dimension: i32,
    x: i32,
    y: i32,
    z: i32,
}

impl SectionKey {
    #[cfg(test)]
    pub(super) const fn new(dimension: i32, x: i32, y: i32, z: i32) -> Self {
        Self { dimension, x, y, z }
    }

    #[cfg(test)]
    pub(super) const fn dimension(self) -> i32 {
        self.dimension
    }

    #[cfg(test)]
    pub(super) const fn x(self) -> i32 {
        self.x
    }

    #[cfg(test)]
    pub(super) const fn y(self) -> i32 {
        self.y
    }

    #[cfg(test)]
    pub(super) const fn z(self) -> i32 {
        self.z
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
struct ColumnKey {
    dimension: i32,
    x: i32,
    z: i32,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub(super) enum SectionData {
    Single(u16),
    Indexed {
        bits: u8,
        palette: Box<[u16]>,
        packed_words: Box<[u64]>,
    },
    Direct {
        packed_words: Box<[u64]>,
    },
}

#[derive(Clone, Debug, PartialEq, Eq)]
enum SectionState {
    Live(SectionData),
    Tombstone,
}

#[derive(Clone, Debug, PartialEq, Eq)]
struct SectionEntry {
    revision: u64,
    state: SectionState,
}

#[derive(Clone, Debug, PartialEq, Eq)]
enum ColumnState {
    Live(Arc<[i16; COLUMN_HEIGHTS]>),
    Tombstone,
}

#[derive(Clone, Debug, PartialEq, Eq)]
struct ColumnEntry {
    revision: u64,
    state: ColumnState,
}

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub(super) struct RenderWorld {
    epoch: Option<u64>,
    sections: BTreeMap<SectionKey, SectionEntry>,
    columns: BTreeMap<ColumnKey, ColumnEntry>,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(super) enum RenderWorldError {
    Invalid,
}

struct ParsedBatch {
    epoch: u64,
    records: Vec<ParsedRecord>,
}

enum ParsedRecord {
    SectionUpsert {
        key: SectionKey,
        revision: u64,
        data: SectionData,
    },
    ColumnUpsert {
        key: ColumnKey,
        revision: u64,
        heights: Arc<[i16; COLUMN_HEIGHTS]>,
    },
    SectionTombstone {
        key: SectionKey,
        revision: u64,
    },
    ColumnTombstone {
        key: ColumnKey,
        revision: u64,
    },
    WorldReset,
}

impl RenderWorld {
    pub(super) fn apply_update_batch(&mut self, bytes: &[u8]) -> Result<(), RenderWorldError> {
        let parsed = ParsedBatch::parse(bytes)?;
        let mut staged = self.clone();
        staged.apply_parsed(parsed)?;
        *self = staged;
        Ok(())
    }

    fn apply_parsed(&mut self, batch: ParsedBatch) -> Result<(), RenderWorldError> {
        let starts_with_reset = matches!(batch.records.first(), Some(ParsedRecord::WorldReset));
        match (self.epoch, starts_with_reset) {
            (None, true) if batch.epoch == 1 => {}
            (Some(current), true) if batch.epoch > current => {}
            (Some(current), false) if batch.epoch == current => {}
            _ => return Err(RenderWorldError::Invalid),
        }

        let mut records = batch.records.into_iter();
        if starts_with_reset {
            self.sections.clear();
            self.columns.clear();
            self.epoch = Some(batch.epoch);
            let _ = records.next();
        }

        for record in records {
            match record {
                ParsedRecord::SectionUpsert {
                    key,
                    revision,
                    data,
                } => Self::replace_section_if_newer(
                    &mut self.sections,
                    key,
                    SectionEntry {
                        revision,
                        state: SectionState::Live(data),
                    },
                ),
                ParsedRecord::ColumnUpsert {
                    key,
                    revision,
                    heights,
                } => Self::replace_column_if_newer(
                    &mut self.columns,
                    key,
                    ColumnEntry {
                        revision,
                        state: ColumnState::Live(heights),
                    },
                ),
                ParsedRecord::SectionTombstone { key, revision } => {
                    Self::replace_section_if_newer(
                        &mut self.sections,
                        key,
                        SectionEntry {
                            revision,
                            state: SectionState::Tombstone,
                        },
                    );
                }
                ParsedRecord::ColumnTombstone { key, revision } => {
                    Self::replace_column_if_newer(
                        &mut self.columns,
                        key,
                        ColumnEntry {
                            revision,
                            state: ColumnState::Tombstone,
                        },
                    );
                }
                ParsedRecord::WorldReset => return Err(RenderWorldError::Invalid),
            }
        }
        Ok(())
    }

    fn replace_section_if_newer(
        sections: &mut BTreeMap<SectionKey, SectionEntry>,
        key: SectionKey,
        replacement: SectionEntry,
    ) {
        if sections
            .get(&key)
            .is_none_or(|entry| replacement.revision > entry.revision)
        {
            sections.insert(key, replacement);
        }
    }

    fn replace_column_if_newer(
        columns: &mut BTreeMap<ColumnKey, ColumnEntry>,
        key: ColumnKey,
        replacement: ColumnEntry,
    ) {
        if columns
            .get(&key)
            .is_none_or(|entry| replacement.revision > entry.revision)
        {
            columns.insert(key, replacement);
        }
    }

    #[cfg(test)]
    pub(super) fn snapshot_for_test(&self) -> Self {
        self.clone()
    }

    #[cfg(test)]
    pub(super) fn epoch_for_test(&self) -> Option<u64> {
        self.epoch
    }

    #[cfg(test)]
    pub(super) fn section_for_test(&self, key: SectionKey) -> Option<&SectionData> {
        match self.sections.get(&key) {
            Some(SectionEntry {
                state: SectionState::Live(data),
                ..
            }) => Some(data),
            _ => None,
        }
    }

    #[cfg(test)]
    pub(super) fn column_for_test(
        &self,
        dimension: i32,
        x: i32,
        z: i32,
    ) -> Option<&[i16; COLUMN_HEIGHTS]> {
        self.column_payload_arc_for_test(dimension, x, z)
            .map(AsRef::as_ref)
    }

    #[cfg(test)]
    pub(super) fn column_payload_arc_for_test(
        &self,
        dimension: i32,
        x: i32,
        z: i32,
    ) -> Option<&Arc<[i16; COLUMN_HEIGHTS]>> {
        match self.columns.get(&ColumnKey { dimension, x, z }) {
            Some(ColumnEntry {
                state: ColumnState::Live(heights),
                ..
            }) => Some(heights),
            _ => None,
        }
    }
}

impl ParsedBatch {
    fn parse(bytes: &[u8]) -> Result<Self, RenderWorldError> {
        if bytes.len() < BATCH_HEADER_BYTES || bytes.len() > MAX_BATCH_BYTES {
            return Err(RenderWorldError::Invalid);
        }

        let mut reader = Reader::new(bytes);
        if reader.take(4)? != b"MRW1" || reader.read_u16()? != 1 || reader.read_u16()? != 0 {
            return Err(RenderWorldError::Invalid);
        }
        let epoch = reader.read_u64()?;
        let record_count = reader.read_u32()?;
        if epoch == 0 || !(1..=MAX_RECORDS).contains(&record_count) || reader.read_u32()? != 0 {
            return Err(RenderWorldError::Invalid);
        }

        let record_count = usize::try_from(record_count).map_err(|_| RenderWorldError::Invalid)?;
        let mut records = Vec::with_capacity(record_count);
        for index in 0..record_count {
            records.push(ParsedRecord::parse(&mut reader, index)?);
        }
        if !reader.is_finished() {
            return Err(RenderWorldError::Invalid);
        }
        Ok(Self { epoch, records })
    }
}

impl ParsedRecord {
    fn parse(reader: &mut Reader<'_>, index: usize) -> Result<Self, RenderWorldError> {
        let header = reader.take(RECORD_HEADER_BYTES)?;
        let mut header = Reader::new(header);
        let tag = header.read_u8()?;
        let storage_kind = header.read_u8()?;
        let bits = header.read_u8()?;
        if header.read_u8()? != 0 {
            return Err(RenderWorldError::Invalid);
        }
        let dimension = header.read_i32()?;
        let x = header.read_i32()?;
        let y = header.read_i32()?;
        let z = header.read_i32()?;
        let revision = header.read_u64()?;
        let payload_len =
            usize::try_from(header.read_u32()?).map_err(|_| RenderWorldError::Invalid)?;
        if !header.is_finished() {
            return Err(RenderWorldError::Invalid);
        }
        let payload = reader.take(payload_len)?;

        match tag {
            TAG_SECTION_UPSERT => {
                let key = section_key(dimension, x, y, z)?;
                let data = parse_section(storage_kind, bits, payload)?;
                Ok(Self::SectionUpsert {
                    key,
                    revision,
                    data,
                })
            }
            TAG_COLUMN_UPSERT => {
                let key = column_key(storage_kind, bits, dimension, x, y, z)?;
                let heights = parse_column(payload)?;
                Ok(Self::ColumnUpsert {
                    key,
                    revision,
                    heights,
                })
            }
            TAG_SECTION_TOMBSTONE => {
                if !payload.is_empty() {
                    return Err(RenderWorldError::Invalid);
                }
                Ok(Self::SectionTombstone {
                    key: section_key(dimension, x, y, z)?,
                    revision,
                })
            }
            TAG_COLUMN_TOMBSTONE => {
                if !payload.is_empty() {
                    return Err(RenderWorldError::Invalid);
                }
                Ok(Self::ColumnTombstone {
                    key: column_key(storage_kind, bits, dimension, x, y, z)?,
                    revision,
                })
            }
            TAG_WORLD_RESET => {
                if index != 0
                    || storage_kind != 0
                    || bits != 0
                    || dimension != 0
                    || x != 0
                    || y != 0
                    || z != 0
                    || revision != 0
                    || !payload.is_empty()
                {
                    return Err(RenderWorldError::Invalid);
                }
                Ok(Self::WorldReset)
            }
            _ => Err(RenderWorldError::Invalid),
        }
    }
}

fn section_key(dimension: i32, x: i32, y: i32, z: i32) -> Result<SectionKey, RenderWorldError> {
    if !(0..SECTIONS_PER_CHUNK).contains(&y) {
        return Err(RenderWorldError::Invalid);
    }
    Ok(SectionKey { dimension, x, y, z })
}

fn column_key(
    storage_kind: u8,
    bits: u8,
    dimension: i32,
    x: i32,
    y: i32,
    z: i32,
) -> Result<ColumnKey, RenderWorldError> {
    if storage_kind != 0 || bits != 0 || y != 0 {
        return Err(RenderWorldError::Invalid);
    }
    Ok(ColumnKey { dimension, x, z })
}

fn parse_section(
    storage_kind: u8,
    bits: u8,
    payload: &[u8],
) -> Result<SectionData, RenderWorldError> {
    let mut reader = Reader::new(payload);
    let meta = reader.take(SECTION_META_BYTES)?;
    let mut meta = Reader::new(meta);
    let single = meta.read_u16()?;
    let palette_count = usize::from(meta.read_u16()?);
    let packed_word_count = usize::from(meta.read_u16()?);
    if meta.read_u16()? != 0 || !meta.is_finished() {
        return Err(RenderWorldError::Invalid);
    }

    match storage_kind {
        STORAGE_SINGLE => {
            if bits != 0 || palette_count != 0 || packed_word_count != 0 || !reader.is_finished() {
                return Err(RenderWorldError::Invalid);
            }
            Ok(SectionData::Single(single))
        }
        STORAGE_INDEXED => {
            let expected_words = match bits {
                4 => 256,
                8 => 512,
                _ => return Err(RenderWorldError::Invalid),
            };
            let palette_limit = 1usize
                .checked_shl(u32::from(bits))
                .ok_or(RenderWorldError::Invalid)?;
            if palette_count == 0
                || palette_count > palette_limit
                || packed_word_count != expected_words
            {
                return Err(RenderWorldError::Invalid);
            }
            let palette = read_u16_values(&mut reader, palette_count)?;
            let packed_words = read_u64_values(&mut reader, packed_word_count)?;
            if !reader.is_finished() || !indexed_slots_are_valid(&packed_words, bits, palette_count)
            {
                return Err(RenderWorldError::Invalid);
            }
            Ok(SectionData::Indexed {
                bits,
                palette: palette.into_boxed_slice(),
                packed_words: packed_words.into_boxed_slice(),
            })
        }
        STORAGE_DIRECT => {
            if bits != 15 || single != 0 || palette_count != 0 || packed_word_count != 1024 {
                return Err(RenderWorldError::Invalid);
            }
            let packed_words = read_u64_values(&mut reader, packed_word_count)?;
            if !reader.is_finished()
                || packed_words
                    .iter()
                    .any(|word| word & 0xf000_0000_0000_0000 != 0)
            {
                return Err(RenderWorldError::Invalid);
            }
            Ok(SectionData::Direct {
                packed_words: packed_words.into_boxed_slice(),
            })
        }
        _ => Err(RenderWorldError::Invalid),
    }
}

fn parse_column(payload: &[u8]) -> Result<Arc<[i16; COLUMN_HEIGHTS]>, RenderWorldError> {
    if payload.len() != COLUMN_PAYLOAD_BYTES {
        return Err(RenderWorldError::Invalid);
    }
    let mut reader = Reader::new(payload);
    let mut heights = [0; COLUMN_HEIGHTS];
    for height in &mut heights {
        *height = reader.read_i16()?;
    }
    if !reader.is_finished() {
        return Err(RenderWorldError::Invalid);
    }
    Ok(Arc::new(heights))
}

fn read_u16_values(reader: &mut Reader<'_>, count: usize) -> Result<Vec<u16>, RenderWorldError> {
    let byte_count = count
        .checked_mul(size_of::<u16>())
        .ok_or(RenderWorldError::Invalid)?;
    let mut values = Vec::with_capacity(count);
    let mut values_reader = Reader::new(reader.take(byte_count)?);
    for _ in 0..count {
        values.push(values_reader.read_u16()?);
    }
    Ok(values)
}

fn read_u64_values(reader: &mut Reader<'_>, count: usize) -> Result<Vec<u64>, RenderWorldError> {
    let byte_count = count
        .checked_mul(size_of::<u64>())
        .ok_or(RenderWorldError::Invalid)?;
    let mut values = Vec::with_capacity(count);
    let mut values_reader = Reader::new(reader.take(byte_count)?);
    for _ in 0..count {
        values.push(values_reader.read_u64()?);
    }
    Ok(values)
}

fn indexed_slots_are_valid(words: &[u64], bits: u8, palette_count: usize) -> bool {
    let slots_per_word = 64 / usize::from(bits);
    let mask = (1u64 << bits) - 1;
    words.iter().all(|word| {
        (0..slots_per_word).all(|slot| {
            let shift = slot * usize::from(bits);
            usize::try_from((word >> shift) & mask).is_ok_and(|index| index < palette_count)
        })
    })
}

struct Reader<'a> {
    bytes: &'a [u8],
    offset: usize,
}

impl<'a> Reader<'a> {
    const fn new(bytes: &'a [u8]) -> Self {
        Self { bytes, offset: 0 }
    }

    fn take(&mut self, length: usize) -> Result<&'a [u8], RenderWorldError> {
        let end = self
            .offset
            .checked_add(length)
            .ok_or(RenderWorldError::Invalid)?;
        let value = self
            .bytes
            .get(self.offset..end)
            .ok_or(RenderWorldError::Invalid)?;
        self.offset = end;
        Ok(value)
    }

    fn read_u8(&mut self) -> Result<u8, RenderWorldError> {
        self.take(1)?
            .first()
            .copied()
            .ok_or(RenderWorldError::Invalid)
    }

    fn read_u16(&mut self) -> Result<u16, RenderWorldError> {
        let mut bytes = [0; size_of::<u16>()];
        bytes.copy_from_slice(self.take(size_of::<u16>())?);
        Ok(u16::from_le_bytes(bytes))
    }

    fn read_i16(&mut self) -> Result<i16, RenderWorldError> {
        let mut bytes = [0; size_of::<i16>()];
        bytes.copy_from_slice(self.take(size_of::<i16>())?);
        Ok(i16::from_le_bytes(bytes))
    }

    fn read_u32(&mut self) -> Result<u32, RenderWorldError> {
        let mut bytes = [0; size_of::<u32>()];
        bytes.copy_from_slice(self.take(size_of::<u32>())?);
        Ok(u32::from_le_bytes(bytes))
    }

    fn read_i32(&mut self) -> Result<i32, RenderWorldError> {
        let mut bytes = [0; size_of::<i32>()];
        bytes.copy_from_slice(self.take(size_of::<i32>())?);
        Ok(i32::from_le_bytes(bytes))
    }

    fn read_u64(&mut self) -> Result<u64, RenderWorldError> {
        let mut bytes = [0; size_of::<u64>()];
        bytes.copy_from_slice(self.take(size_of::<u64>())?);
        Ok(u64::from_le_bytes(bytes))
    }

    fn is_finished(&self) -> bool {
        self.offset == self.bytes.len()
    }
}
