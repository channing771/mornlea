//! Typed decoding for the v1 frame presentation record.
//!
//! The Go client core owns frame assembly and prediction. This module owns the
//! record-to-value conversion so Python only receives validated Godot values.
//! A malformed or incomplete record is rejected as one unit; no field is
//! returned as a guessed default.

use crate::abi;

const CAMERA_BYTES: usize = 40;
const HUD_BYTES: usize = 16;
const ENVIRONMENT_BYTES: usize = 48;
const ENTITY_TICK_BYTES: usize = 8;
const ENTITY_BYTES: usize = 48;
const TARGET_BYTES: usize = 16;
const PHASE_ERROR_BYTES: usize = 8;
const PHASE_DISCONNECTED: u32 = 5;
const ERROR_MAX: u32 = 5;
const WORLD_MIN_Y: i32 = -64;
const WORLD_MAX_Y: i32 = 320;
const PITCH_LIMIT: f32 = core::f32::consts::FRAC_PI_2 - 0.01;

#[derive(Clone, Debug, PartialEq)]
pub(crate) struct TypedFrame {
    pub(crate) revision: u64,
    pub(crate) epoch: u64,
    pub(crate) camera_ready: bool,
    pub(crate) position: [f32; 3],
    pub(crate) yaw: f32,
    pub(crate) pitch: f32,
    pub(crate) fov_y: f32,
    pub(crate) aspect: f32,
    pub(crate) near: f32,
    pub(crate) far: f32,
    pub(crate) hud_ready: bool,
    pub(crate) health: u32,
    pub(crate) hunger: u32,
    pub(crate) oxygen: u32,
    pub(crate) environment_ready: bool,
    pub(crate) environment_server_tick: u64,
    pub(crate) world_time_ticks: u64,
    pub(crate) day_phase_offset: u32,
    pub(crate) weather: u32,
    pub(crate) season: u32,
    pub(crate) season_progress: u32,
    pub(crate) temperature: i32,
    pub(crate) daylight: f32,
    pub(crate) sky_color: [f32; 3],
    pub(crate) entity_server_tick: u64,
    pub(crate) entities: Vec<TypedEntity>,
    pub(crate) target_visible: bool,
    pub(crate) target_position: [i32; 3],
    pub(crate) target_name: String,
    pub(crate) phase: u32,
    pub(crate) error: u32,
}

/// One renderer-independent entity value decoded from the frame family.
/// The bridge publishes these fields to Godot only after the whole frame has
/// passed identity, capacity, duplicate, and numeric-domain validation.
#[derive(Clone, Debug, PartialEq)]
pub(crate) struct TypedEntity {
    pub(crate) kind: u32,
    pub(crate) player_id: [u8; 16],
    pub(crate) dimension: u32,
    pub(crate) position: [f32; 3],
    pub(crate) yaw: f32,
    pub(crate) pitch: f32,
}

impl TypedEntity {
    /// Return the canonical lowercase UUID text consumed as the Godot pool key.
    pub(crate) fn player_id_text(&self) -> String {
        let id = self.player_id;
        format!(
            "{:02x}{:02x}{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}{:02x}{:02x}{:02x}{:02x}",
            id[0],
            id[1],
            id[2],
            id[3],
            id[4],
            id[5],
            id[6],
            id[7],
            id[8],
            id[9],
            id[10],
            id[11],
            id[12],
            id[13],
            id[14],
            id[15],
        )
    }
}

pub(crate) fn decode_frame_record(record: &[u8]) -> Result<TypedFrame, u32> {
    let fixed = abi::FRAME_HEADER_BYTES
        + CAMERA_BYTES
        + HUD_BYTES
        + ENVIRONMENT_BYTES
        + ENTITY_TICK_BYTES
        + TARGET_BYTES
        + PHASE_ERROR_BYTES;
    if record.len() < fixed
        || le32(record, 0) != abi::MAGIC_FRAME
        || le32(record, 4) != abi::FRAME_VERSION
        || le32(record, 8) != abi::FRAME_SNAPSHOT_VERSION
        || le32(record, 20) != 0
    {
        return Err(abi::STATUS_INTERNAL);
    }
    let entity_count = le32(record, 12) as usize;
    let name_len = le32(record, 16) as usize;
    if entity_count > abi::MAX_ENTITY_RECORDS as usize
        || name_len > abi::MAX_TARGET_NAME_BYTES as usize
    {
        return Err(abi::STATUS_INTERNAL);
    }
    let expected = fixed + entity_count * ENTITY_BYTES + name_len;
    if expected != record.len() {
        return Err(abi::STATUS_INTERNAL);
    }

    let camera = abi::FRAME_HEADER_BYTES;
    let camera_ready = ready_word(record, camera)?;
    let position = [
        float(record, camera + 4)?,
        float(record, camera + 8)?,
        float(record, camera + 12)?,
    ];
    let yaw = float(record, camera + 16)?;
    let pitch = float(record, camera + 20)?;
    let fov_y = float(record, camera + 24)?;
    let aspect = float(record, camera + 28)?;
    let near = float(record, camera + 32)?;
    let far = float(record, camera + 36)?;
    if !camera_ready {
        if position != [0.0; 3]
            || yaw != 0.0
            || pitch != 0.0
            || fov_y != 0.0
            || aspect != 0.0
            || near != 0.0
            || far != 0.0
        {
            return Err(abi::STATUS_INTERNAL);
        }
    } else if !position.iter().all(|value| value.is_finite())
        || !yaw.is_finite()
        || !pitch.is_finite()
        || !fov_y.is_finite()
        || !aspect.is_finite()
        || !near.is_finite()
        || !far.is_finite()
        || pitch.abs() > PITCH_LIMIT
        || fov_y <= 0.0
        || fov_y >= core::f32::consts::PI
        || aspect <= 0.0
        || near <= 0.0
        || far <= near
    {
        return Err(abi::STATUS_INTERNAL);
    }

    let hud = camera + CAMERA_BYTES;
    let hud_ready = ready_word(record, hud)?;
    let health = le32(record, hud + 4);
    let hunger = le32(record, hud + 8);
    let oxygen = le32(record, hud + 12);
    if health > 20
        || hunger > 20
        || oxygen > 300
        || (!hud_ready && (health != 0 || hunger != 0 || oxygen != 0))
    {
        return Err(abi::STATUS_INTERNAL);
    }
    let environment = hud + HUD_BYTES;
    let environment_ready = ready_word(record, environment)?;
    let environment_server_tick = le64(record, environment + 8);
    let world_time_ticks = le64(record, environment + 16);
    let day_phase_offset = le32(record, environment + 24);
    let weather = le32(record, environment + 28);
    let season = le32(record, environment + 32);
    let season_progress = le32(record, environment + 36);
    let temperature = le32(record, environment + 40) as i32;
    // Reject the complete observation before any resource mutation, including
    // data hidden behind readiness and both reserved words.
    if le32(record, environment + 4) != 0
        || le32(record, environment + 44) != 0
        || (!environment_ready
            && record[environment + 4..environment + ENVIRONMENT_BYTES]
                .iter()
                .any(|byte| *byte != 0))
        || day_phase_offset >= 24000
        || weather > 2
        || season > 3
        || season_progress > 255
        || !(-40..=45).contains(&temperature)
    {
        return Err(abi::STATUS_INTERNAL);
    }

    let entity_tick = environment + ENVIRONMENT_BYTES;
    let entity_server_tick = le64(record, entity_tick);
    let entity_start = entity_tick + ENTITY_TICK_BYTES;
    let mut entities = Vec::with_capacity(entity_count);
    for index in 0..entity_count {
        let offset = entity_start + index * ENTITY_BYTES;
        let kind = le32(record, offset);
        if kind != 1 || le32(record, offset + 4) != 0 {
            return Err(abi::STATUS_INTERNAL);
        }
        let mut player_id = [0u8; 16];
        player_id.copy_from_slice(&record[offset + 8..offset + 24]);
        if player_id == [0; 16] || player_id[6] >> 4 != 4 || player_id[8] & 0xc0 != 0x80 {
            return Err(abi::STATUS_INTERNAL);
        }
        if entities
            .iter()
            .any(|entity: &TypedEntity| entity.player_id == player_id)
        {
            return Err(abi::STATUS_INTERNAL);
        }
        let dimension = le32(record, offset + 24);
        if dimension > 1 {
            return Err(abi::STATUS_INTERNAL);
        }
        let position = [
            float(record, offset + 28)?,
            float(record, offset + 32)?,
            float(record, offset + 36)?,
        ];
        let yaw = float(record, offset + 40)?;
        let pitch = float(record, offset + 44)?;
        if !position.iter().all(|value| value.is_finite()) || !yaw.is_finite() || !pitch.is_finite()
        {
            return Err(abi::STATUS_INTERNAL);
        }
        entities.push(TypedEntity {
            kind,
            player_id,
            dimension,
            position,
            yaw,
            pitch,
        });
    }

    let target = entity_start + entity_count * ENTITY_BYTES;
    let target_visible = ready_word(record, target)?;
    let target_position = [
        le32(record, target + 4) as i32,
        le32(record, target + 8) as i32,
        le32(record, target + 12) as i32,
    ];
    let name_start = target + TARGET_BYTES + PHASE_ERROR_BYTES;
    let target_name = if target_visible {
        if name_len == 0 || target_position[1] < WORLD_MIN_Y || target_position[1] >= WORLD_MAX_Y {
            return Err(abi::STATUS_INTERNAL);
        }
        core::str::from_utf8(&record[name_start..])
            .map_err(|_| abi::STATUS_INTERNAL)?
            .to_owned()
    } else {
        if target_position != [0; 3] || name_len != 0 {
            return Err(abi::STATUS_INTERNAL);
        }
        String::new()
    };

    let phase = le32(record, target + TARGET_BYTES);
    let error = le32(record, target + TARGET_BYTES + 4);
    if phase > PHASE_DISCONNECTED
        || error > ERROR_MAX
        || ((phase == PHASE_DISCONNECTED) != (error != 0))
    {
        return Err(abi::STATUS_INTERNAL);
    }

    Ok(TypedFrame {
        revision: le64(record, 24),
        epoch: le64(record, 32),
        camera_ready,
        position,
        yaw,
        pitch,
        fov_y,
        aspect,
        near,
        far,
        hud_ready,
        health,
        hunger,
        oxygen,
        environment_ready,
        environment_server_tick,
        world_time_ticks,
        day_phase_offset,
        weather,
        season,
        season_progress,
        temperature,
        daylight: 0.0,
        sky_color: [0.0; 3],
        entity_server_tick,
        entities,
        target_visible,
        target_position,
        target_name,
        phase,
        error,
    })
}

/// Join only matching immutable observations; a concurrent step must never
/// combine new lighting with an older camera or HUD publication.
pub(crate) fn apply_environment_projection(
    frame: &mut TypedFrame,
    record: &[u8],
) -> Result<(), u32> {
    if record.len() != abi::ENVIRONMENT_BYTES
        || le32(record, 0) != abi::MAGIC_ENVIRONMENT
        || le32(record, 4) != abi::ENVIRONMENT_VERSION
        || le32(record, 12) != 0
        || le64(record, 16) != frame.revision
        || le64(record, 24) != frame.epoch
        || ready_word(record, 8)? != frame.environment_ready
    {
        return Err(abi::STATUS_INTERNAL);
    }
    let values = [
        float(record, 32)?,
        float(record, 36)?,
        float(record, 40)?,
        float(record, 44)?,
    ];
    if values
        .iter()
        .any(|value| !value.is_finite() || !(0.0..=1.0).contains(value))
        || (!frame.environment_ready && values != [0.0; 4])
    {
        return Err(abi::STATUS_INTERNAL);
    }
    frame.daylight = values[0];
    frame.sky_color.copy_from_slice(&values[1..]);
    Ok(())
}

fn ready_word(record: &[u8], offset: usize) -> Result<bool, u32> {
    match le32(record, offset) {
        0 => Ok(false),
        1 => Ok(true),
        _ => Err(abi::STATUS_INTERNAL),
    }
}

fn float(record: &[u8], offset: usize) -> Result<f32, u32> {
    Ok(f32::from_bits(le32(record, offset)))
}

fn le32(record: &[u8], offset: usize) -> u32 {
    u32::from_le_bytes(
        record[offset..offset + 4]
            .try_into()
            .expect("validated word"),
    )
}

fn le64(record: &[u8], offset: usize) -> u64 {
    u64::from_le_bytes(
        record[offset..offset + 8]
            .try_into()
            .expect("validated word"),
    )
}

#[cfg(test)]
mod tests {
    use super::decode_frame_record;
    use crate::abi;
    use crate::pull_buffers::test_frame_record;

    #[test]
    fn frame_decode_rejects_unconfirmed_hud_and_environment_payloads() {
        let fixed = abi::FRAME_HEADER_BYTES + 40 + 16 + 48 + 8 + 16 + 8;
        let mut valid = vec![0u8; fixed];
        valid[0..4].copy_from_slice(&abi::MAGIC_FRAME.to_le_bytes());
        valid[4..8].copy_from_slice(&abi::FRAME_VERSION.to_le_bytes());
        valid[8..12].copy_from_slice(&abi::FRAME_SNAPSHOT_VERSION.to_le_bytes());
        assert!(decode_frame_record(&valid).is_ok());
        for offset in [
            80, 84, 88, 92, 96, 100, 104, 112, 120, 124, 128, 132, 136, 140,
        ] {
            let mut bad = valid.clone();
            bad[offset..offset + 4].copy_from_slice(&99999u32.to_le_bytes());
            assert!(decode_frame_record(&bad).is_err(), "offset {offset}");
        }
        let hud = 80;
        valid[hud..hud + 4].copy_from_slice(&1u32.to_le_bytes());
        valid[hud + 4..hud + 8].copy_from_slice(&20u32.to_le_bytes());
        valid[hud + 12..hud + 16].copy_from_slice(&300u32.to_le_bytes());
        assert!(decode_frame_record(&valid).is_ok());
        valid[hud + 12..hud + 16].copy_from_slice(&301u32.to_le_bytes());
        assert!(decode_frame_record(&valid).is_err());
    }

    #[test]
    fn frame_decode_rejects_the_existing_zero_fixture_until_ready_fields_are_valid() {
        let record = test_frame_record(2, 3);
        assert!(decode_frame_record(&record).is_err());
    }

    #[test]
    fn frame_decode_accepts_a_ready_camera_and_visible_target() {
        let fixed = abi::FRAME_HEADER_BYTES + 40 + 16 + 48 + 8 + 16 + 8;
        let name = b"Stone";
        let mut record = vec![0u8; fixed + name.len()];
        record[0..4].copy_from_slice(&abi::MAGIC_FRAME.to_le_bytes());
        record[4..8].copy_from_slice(&abi::FRAME_VERSION.to_le_bytes());
        record[8..12].copy_from_slice(&abi::FRAME_SNAPSHOT_VERSION.to_le_bytes());
        record[12..16].copy_from_slice(&0u32.to_le_bytes());
        record[16..20].copy_from_slice(&(name.len() as u32).to_le_bytes());
        record[24..32].copy_from_slice(&3u64.to_le_bytes());
        record[32..40].copy_from_slice(&2u64.to_le_bytes());
        let camera = abi::FRAME_HEADER_BYTES;
        record[camera..camera + 4].copy_from_slice(&1u32.to_le_bytes());
        for (offset, value) in [
            (4, 1.0f32),
            (8, 65.0),
            (12, -2.0),
            (16, 0.5),
            (20, -0.2),
            (24, 1.2),
            (28, 16.0 / 9.0),
            (32, 0.1),
            (36, 1000.0),
        ] {
            record[camera + offset..camera + offset + 4]
                .copy_from_slice(&value.to_bits().to_le_bytes());
        }
        let target = camera + 40 + 16 + 48 + 8;
        record[target..target + 4].copy_from_slice(&1u32.to_le_bytes());
        record[target + 4..target + 8].copy_from_slice(&4i32.to_le_bytes());
        record[target + 8..target + 12].copy_from_slice(&65i32.to_le_bytes());
        record[target + 12..target + 16].copy_from_slice(&(-2i32).to_le_bytes());
        record[target + 16 + 8..].copy_from_slice(name);
        let decoded = decode_frame_record(&record).expect("typed frame");
        assert_eq!(decoded.revision, 3);
        assert_eq!(decoded.epoch, 2);
        assert_eq!(decoded.position, [1.0, 65.0, -2.0]);
        assert_eq!(decoded.target_name, "Stone");
    }

    #[test]
    fn frame_decode_rejects_hidden_target_payload_and_invalid_camera() {
        let mut record = test_frame_record(1, 1);
        let target = abi::FRAME_HEADER_BYTES + 40 + 16 + 48 + 8;
        record[target + 4..target + 8].copy_from_slice(&1i32.to_le_bytes());
        assert!(decode_frame_record(&record).is_err());
        let camera = abi::FRAME_HEADER_BYTES;
        record[target + 4..target + 8].fill(0);
        record[camera..camera + 4].copy_from_slice(&1u32.to_le_bytes());
        record[camera + 20..camera + 24].copy_from_slice(&f32::NAN.to_bits().to_le_bytes());
        assert!(decode_frame_record(&record).is_err());
    }

    #[test]
    fn frame_decode_rejects_target_height_and_terminal_error_drift() {
        let mut record = test_frame_record(1, 1);
        let target = abi::FRAME_HEADER_BYTES + 40 + 16 + 48 + 8;
        record[target..target + 4].copy_from_slice(&1u32.to_le_bytes());
        record[16..20].copy_from_slice(&4u32.to_le_bytes());
        record[target + 8..target + 12].copy_from_slice(&320i32.to_le_bytes());
        record.extend_from_slice(b"test");
        assert!(decode_frame_record(&record).is_err());

        let mut terminal = test_frame_record(1, 1);
        terminal[target + 16..target + 20].copy_from_slice(&5u32.to_le_bytes());
        assert!(decode_frame_record(&terminal).is_err());
    }

    #[test]
    fn camera_mapping_domain_is_fail_closed() {
        let mut record = test_frame_record(1, 1);
        let camera = abi::FRAME_HEADER_BYTES;
        record[camera..camera + 4].copy_from_slice(&1u32.to_le_bytes());
        record[camera + 20..camera + 24]
            .copy_from_slice(&core::f32::consts::FRAC_PI_2.to_bits().to_le_bytes());
        assert!(decode_frame_record(&record).is_err());
    }

    #[test]
    fn entity_snapshot_decode_exposes_typed_remote_players() {
        let record = frame_record_with_entities(2);
        let decoded = decode_frame_record(&record).expect("typed entity frame");
        assert_eq!(decoded.entity_server_tick, 42);
        assert_eq!(decoded.entities.len(), 2);
        assert_eq!(decoded.entities[0].kind, 1);
        assert_eq!(decoded.entities[0].player_id[0], 1);
        assert_eq!(decoded.entities[0].dimension, 0);
        assert_eq!(decoded.entities[0].position, [1.0, 65.0, -2.0]);
        assert_eq!(decoded.entities[0].yaw, 0.25);
        assert_eq!(decoded.entities[0].pitch, -0.125);
    }

    #[test]
    fn entity_snapshot_decode_rejects_duplicate_identity_and_capacity_overflow() {
        let mut duplicate = frame_record_with_entities(2);
        let entities = abi::FRAME_HEADER_BYTES + 40 + 16 + 48 + 8;
        let first_id = duplicate[entities + 8..entities + 24].to_vec();
        duplicate[entities + 48 + 8..entities + 48 + 24].copy_from_slice(&first_id);
        assert!(decode_frame_record(&duplicate).is_err());
        assert!(decode_frame_record(&frame_record_with_entities(8)).is_err());
    }

    fn frame_record_with_entities(count: usize) -> Vec<u8> {
        let fixed = abi::FRAME_HEADER_BYTES + 40 + 16 + 48 + 8 + 16 + 8;
        let mut record = vec![0u8; fixed + count * 48];
        record[0..4].copy_from_slice(&abi::MAGIC_FRAME.to_le_bytes());
        record[4..8].copy_from_slice(&abi::FRAME_VERSION.to_le_bytes());
        record[8..12].copy_from_slice(&abi::FRAME_SNAPSHOT_VERSION.to_le_bytes());
        record[12..16].copy_from_slice(&(count as u32).to_le_bytes());
        record[24..32].copy_from_slice(&3u64.to_le_bytes());
        record[32..40].copy_from_slice(&2u64.to_le_bytes());
        let tick = abi::FRAME_HEADER_BYTES + 40 + 16 + 48;
        record[tick..tick + 8].copy_from_slice(&42u64.to_le_bytes());
        let entities = tick + 8;
        for index in 0..count {
            let offset = entities + index * 48;
            record[offset..offset + 4].copy_from_slice(&1u32.to_le_bytes());
            record[offset + 8] = (index + 1) as u8;
            record[offset + 14] = 0x40;
            record[offset + 16] = 0x80;
            record[offset + 28..offset + 32]
                .copy_from_slice(&(index as f32 + 1.0).to_bits().to_le_bytes());
            record[offset + 32..offset + 36].copy_from_slice(&65.0f32.to_bits().to_le_bytes());
            record[offset + 36..offset + 40].copy_from_slice(&(-2.0f32).to_bits().to_le_bytes());
            record[offset + 40..offset + 44].copy_from_slice(&0.25f32.to_bits().to_le_bytes());
            record[offset + 44..offset + 48].copy_from_slice(&(-0.125f32).to_bits().to_le_bytes());
        }
        record
    }
}
