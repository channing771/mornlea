//! Bridge-owned terrain pipeline between the pulled world/frame records and
//! the terrain budget stage.
//!
//! The bridge node (see `crate::bridge`) owns exactly one
//! [`BridgeTerrain`]. This module is pure data plumbing with no Godot types:
//! it decodes the producer's frozen `MCW1` world-batch record and `MCF1`
//! frame record, feeds section upserts and drops into the stage, derives the
//! camera section for the per-frame drive, and renders the structural
//! summary the headless terrain check asserts against. The resource-loading
//! half of the attach (materials, scenario, atlas texture) lives with the
//! bridge node because it needs the live engine; this module receives the
//! assembled stage instead, which keeps every decision here unit-testable
//! with the scripted backends the terrain suites already use.
//!
//! Ingest ordering and retention: the world pull is a consuming protocol, so
//! an upsert the worker queue refused (`Submit::Full`) must not be lost. The
//! bridge retains the refused remainder as a replay queue with one bound per
//! entry kind: upserts are byte-bounded at [`DEFERRED_QUAD_BYTES`] (one
//! frozen maximal world batch of packed-quad bytes, the same bound the
//! worker's own pending queue enforces; a refused upsert beyond the byte
//! bound fails closed with the capacity word — by then the producer has
//! published more than one undrained maximal batch, a streaming contract
//! violation rather than a state the bridge may silently drop), while
//! payload-less drops carry no bytes and stay bounded by producer batch
//! discipline plus supersession (one producer batch contributes at most its
//! operation count, a newer operation for a section drops its queued
//! predecessor, and every ingest call replays at most
//! [`INGEST_DROP_LIMIT`] deferred drops, so a bounded backlog converges).
//! The replay phase feeds the queue before any newer batch's operations.
//!
//! Producer-drop bounding: the stage's per-frame drop budget covers
//! reclamation only, so producer-driven drops applied through ingestion are
//! bounded here at [`INGEST_DROP_LIMIT`] per ingest call (equal to the
//! stage's drop budget in section count); the remainder defers into the same
//! replay queue and the next call continues in order.
//!
//! Camera derivation: the frame family's camera record carries the camera
//! position but no dimension, so the camera dimension follows the most
//! recently ingested world batch (the producer streams the camera's
//! dimension; Overworld before any batch). The section X/Z uses the server's
//! own rule — floor of the block position, arithmetic shift by four — which
//! `packages/server/server/player_publication.go` states as
//! `int32(math.Floor(position / SectionSize))` and `core.BlockPos.Section`
//! implements as an arithmetic `>> 4`. The identical derivation here keeps
//! the keep-band boundary aligned with the authoritative streaming
//! comparison: an off-by-one section in either direction would drift the
//! inclusive radius boundary by a whole chunk column.

use std::collections::VecDeque;

use crate::abi;
use crate::mesh_worker::{SectionId, Submit};
use crate::quad_decode::QUAD_BYTES;
use crate::terrain_budget::{CameraSection, FrameReport, TerrainBudgetStage};
use crate::terrain_resources::{SectionCoord, SectionSummary};

/// Maximum producer-driven drops one ingest call applies through the stage.
/// Reclamation owns the stage's drop budget; batch-driven drops are outside
/// it by the budget contract, so the bridge applies its own equal-count
/// bound and defers the remainder.
pub(crate) const INGEST_DROP_LIMIT: usize = crate::terrain_budget::DROP_BUDGET_SECTIONS;

/// Retention bound of the deferred replay queue in packed-quad bytes: one
/// frozen maximal world batch, mirroring the worker's pending-queue bound.
pub(crate) const DEFERRED_QUAD_BYTES: usize = abi::MAX_WORLD_BATCH_QUADS as usize * QUAD_BYTES;

/// Wire size of one world operation record and of the world header, pinned
/// against the Go producer's `WorldOperationBytes` and the frozen
/// `MornleaClientWorldHeader` (the pull-buffer module pins the same sizes).
const WORLD_OPERATION_BYTES: usize = 32;

/// Operation kind codes of the world family record body.
const WORLD_OP_UPSERT: u32 = 1;
const WORLD_OP_DROP: u32 = 2;

/// Wire offsets of the fixed frame-family records the camera derivation
/// reads: the 40-byte header, then the camera record's ready word and
/// position triple.
const FRAME_CAMERA_READY_OFFSET: usize = abi::FRAME_HEADER_BYTES;
const FRAME_CAMERA_POS_OFFSET: usize = abi::FRAME_HEADER_BYTES + 4;

/// One decoded world operation, routed by its wire kind: the section
/// identity plus the upsert's packed-quad payload slice inside the record
/// (empty for drops, whose kind is the only legal routing signal).
struct DecodedOp<'record> {
    upsert: bool,
    id: SectionId,
    quads: &'record [u8],
}

/// The typed result of one world ingestion, in the same section units the
/// stage uses. Plain data: the bridge forwards it as a typed dictionary.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub(crate) struct TerrainIngestSummary {
    /// Operations the record carried (upserts plus drops).
    pub(crate) operations: usize,
    /// Packed-quad bytes the record carried.
    pub(crate) record_quad_bytes: usize,
    /// Upserts accepted by the worker queue this call.
    pub(crate) upserts_submitted: usize,
    /// Upserts retained in the deferred replay queue this call (queue full).
    pub(crate) upserts_deferred: usize,
    /// Upserts replayed from a previous call's queue this call.
    pub(crate) upserts_replayed: usize,
    /// Producer drops applied through the stage this call.
    pub(crate) drops_applied: usize,
    /// Producer drops the RID table refused (stale or zero revision); the
    /// producer's own revision arbitration makes refusal legal and safe.
    pub(crate) drops_refused: usize,
    /// Drops retained in the deferred replay queue this call (limit reached).
    pub(crate) drops_deferred: usize,
    /// Drops replayed from a previous call's queue this call.
    pub(crate) drops_replayed: usize,
    /// Operations waiting in the deferred replay queue after this call.
    pub(crate) deferred_pending: usize,
}

/// The typed result of one terrain frame drive: the derived camera plus the
/// stage's frame report.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) struct TerrainFrameSummary {
    pub(crate) camera: CameraSection,
    pub(crate) report: FrameReport,
}

/// One deferred replay operation. Upserts retain their packed payload by
/// value (the worker contract returns refused payloads untouched); drops
/// carry none.
enum DeferredOp {
    Upsert { id: SectionId, packed: Vec<u64> },
    Drop { id: SectionId },
}

impl DeferredOp {
    fn coord(&self) -> SectionCoord {
        SectionCoord::of(match self {
            DeferredOp::Upsert { id, .. } | DeferredOp::Drop { id } => *id,
        })
    }

    fn quad_bytes(&self) -> usize {
        match self {
            DeferredOp::Upsert { packed, .. } => packed.len() * QUAD_BYTES,
            DeferredOp::Drop { .. } => 0,
        }
    }
}

/// The attach identity: the three material resource paths plus the scenario
/// node path, kept so a repeated attach with the same inputs is an
/// idempotent success while a different set fails closed.
#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct TerrainInputs {
    pub(crate) opaque: String,
    pub(crate) cutout: String,
    pub(crate) water: String,
    pub(crate) scenario: String,
}

/// The bridge-owned terrain pipeline over one assembled stage.
pub(crate) struct BridgeTerrain {
    stage: TerrainBudgetStage,
    /// The attach identity for idempotent re-attachment.
    pub(crate) inputs: TerrainInputs,
    /// The camera dimension of the most recently ingested world batch; the
    /// frame family carries no dimension of its own.
    camera_dimension: u32,
    deferred: VecDeque<DeferredOp>,
    deferred_quad_bytes: usize,
}

impl BridgeTerrain {
    /// Assemble over one already-built stage (worker spawned, renderer
    /// wired). The caller owns the Godot-resource half of the attach.
    pub(crate) fn assemble(stage: TerrainBudgetStage, inputs: TerrainInputs) -> Self {
        Self {
            stage,
            inputs,
            camera_dimension: 0,
            deferred: VecDeque::new(),
            deferred_quad_bytes: 0,
        }
    }

    /// Ingest one pulled `MCW1` world-batch record: decode it fail-closed,
    /// replay any deferred remainder from earlier calls first, then feed the
    /// record's operations in order under the ingest bounds. An empty record
    /// is the family's no-batch outcome (most frames publish none) and still
    /// replays deferred work; a malformed record is a producer contract
    /// violation and fails closed without touching the stage.
    pub(crate) fn ingest(&mut self, record: &[u8]) -> Result<TerrainIngestSummary, u32> {
        let ops = if record.is_empty() {
            Vec::new()
        } else {
            decode_world_record(record)?
        };
        let mut summary = TerrainIngestSummary {
            operations: ops.len(),
            record_quad_bytes: record
                .len()
                .saturating_sub(abi::WORLD_HEADER_BYTES + ops.len() * WORLD_OPERATION_BYTES),
            ..TerrainIngestSummary::default()
        };
        // The producer streams the camera's dimension; the frame family
        // carries none, so the newest batch's first operation decides it.
        if let Some(first) = ops.first() {
            self.camera_dimension = first.id.dimension;
        }
        // A newer operation supersedes a queued predecessor for the same
        // section: replaying the stale one afterwards could only be refused
        // by the RID table's revision discipline, so dropping it early keeps
        // the queue at the live set.
        let fresh: std::collections::HashSet<SectionCoord> =
            ops.iter().map(|op| SectionCoord::of(op.id)).collect();
        self.retain_deferred(|coord| !fresh.contains(&coord));

        self.replay_deferred(&mut summary);
        for op in &ops {
            if op.upsert {
                self.submit_or_defer_upsert(op.id, op.quads, &mut summary)?;
            } else {
                self.apply_or_defer_drop(op.id, &mut summary);
            }
        }
        summary.deferred_pending = self.deferred.len();
        Ok(summary)
    }

    /// Drive exactly one stage frame from one pulled `MCF1` frame record:
    /// derive the camera section (floor of the camera block position over
    /// sixteen, the authoritative streaming rule) and call `frame` once.
    /// An empty record is the family's before-first-step outcome; the
    /// origin column is the honest fallback camera for that drive, exactly
    /// like a not-ready camera's validated zero position.
    pub(crate) fn frame(&mut self, frame_record: &[u8]) -> Result<TerrainFrameSummary, u32> {
        let camera = if frame_record.is_empty() {
            CameraSection {
                dimension: self.camera_dimension,
                x: 0,
                z: 0,
            }
        } else {
            let position = decode_camera_position(frame_record)?;
            CameraSection {
                dimension: self.camera_dimension,
                x: camera_section_axis(position[0]),
                z: camera_section_axis(position[2]),
            }
        };
        let report = self.stage.frame(camera);
        Ok(TerrainFrameSummary { camera, report })
    }

    /// Session reset: the stage discards its queues and sections under its
    /// strictly monotonic epoch rule, and the bridge forgets its deferred
    /// remainder (those operations belonged to the retired session).
    pub(crate) fn reset(&mut self) {
        self.stage.reset();
        self.deferred.clear();
        self.deferred_quad_bytes = 0;
        self.camera_dimension = 0;
    }

    /// The stage's accounting snapshot for the summary surface.
    pub(crate) fn facts(&self) -> crate::terrain_budget::TerrainBudgetFacts {
        self.stage.facts()
    }

    /// The live section inventory for the no-stale-section proof.
    pub(crate) fn section_summaries(&self) -> Vec<SectionSummary> {
        self.stage.section_summaries()
    }

    /// The structural summary the headless terrain check asserts against:
    /// one object with the live section inventory (dimension, section
    /// coordinates, revision, surface count) and the stage facts. Section
    /// order is the RID table's coordinate order, so the summary is
    /// deterministic for one state.
    pub(crate) fn summary_json(&self) -> String {
        let facts = self.facts();
        let mut json = String::from("{\"status\":0,\"sections\":[");
        for (index, summary) in self.section_summaries().iter().enumerate() {
            if index > 0 {
                json.push(',');
            }
            json.push_str(&format!(
                "{{\"dimension\":{},\"x\":{},\"y\":{},\"z\":{},\"revision\":{},\"surfaces\":{}}}",
                summary.coord.dimension,
                summary.coord.x,
                summary.coord.y,
                summary.coord.z,
                summary.revision,
                summary.surfaces
            ));
        }
        json.push_str(&format!(
            "],\"facts\":{{\"epoch\":{},\"frames\":{},\"resets\":{},\"packed_input_bytes\":{},\
\"expanded_output_bytes\":{},\"prepare_duration_ns\":{},\"upload_duration_ns\":{},\
\"uploads_applied\":{},\"uploads_failed\":{},\"uploads_discarded\":{},\"reclaimed_sections\":{},\
\"producer_drops_applied\":{},\"live_sections\":{},\"live_surfaces\":{},\"live_rids\":{},\
\"peak_sections\":{},\"peak_surfaces\":{},\"peak_rids\":{}}}}}",
            facts.epoch,
            facts.frames,
            facts.resets,
            facts.packed_input_bytes,
            facts.expanded_output_bytes,
            facts.prepare_duration.as_nanos(),
            facts.upload_duration.as_nanos(),
            facts.uploads_applied,
            facts.uploads_failed,
            facts.uploads_discarded,
            facts.reclaimed_sections,
            facts.producer_drops_applied,
            facts.live_sections,
            facts.live_surfaces,
            facts.live_rids,
            facts.peak_sections,
            facts.peak_surfaces,
            facts.peak_rids
        ));
        json
    }

    /// Replay the deferred queue under the ingest bounds, preserving order:
    /// the first refused upsert stops the replay at the queue head.
    fn replay_deferred(&mut self, summary: &mut TerrainIngestSummary) {
        let mut drops = INGEST_DROP_LIMIT;
        while let Some(op) = self.deferred.pop_front() {
            let cost = op.quad_bytes();
            match op {
                DeferredOp::Upsert { id, packed } => {
                    match self.stage.submit(id, packed) {
                        Submit::Queued => {
                            self.deferred_quad_bytes -= cost;
                            summary.upserts_replayed += 1;
                        }
                        refused => {
                            // The queue is still full: hand the payload back
                            // to the queue head and stop replaying in order.
                            let packed = match refused {
                                Submit::Full { packed } | Submit::Closed { packed } => packed,
                                Submit::Queued => unreachable!("matched above"),
                            };
                            self.deferred.push_front(DeferredOp::Upsert { id, packed });
                            break;
                        }
                    }
                }
                DeferredOp::Drop { id } => {
                    if drops == 0 {
                        self.deferred.push_front(DeferredOp::Drop { id });
                        break;
                    }
                    drops -= 1;
                    match self.stage.drop_section(id) {
                        Ok(()) => summary.drops_replayed += 1,
                        Err(_) => summary.drops_refused += 1,
                    }
                }
            }
        }
    }

    /// Submit one upsert, deferring it with its payload when the worker
    /// queue refuses. Fails closed when the retention bound is exceeded.
    fn submit_or_defer_upsert(
        &mut self,
        id: SectionId,
        quads: &[u8],
        summary: &mut TerrainIngestSummary,
    ) -> Result<(), u32> {
        let packed = decode_packed_quads(quads)?;
        let cost = packed.len() * QUAD_BYTES;
        match self.stage.submit(id, packed) {
            Submit::Queued => {
                summary.upserts_submitted += 1;
                return Ok(());
            }
            refused => {
                let packed = match refused {
                    Submit::Full { packed } | Submit::Closed { packed } => packed,
                    Submit::Queued => unreachable!("matched above"),
                };
                if self.deferred_quad_bytes + cost > DEFERRED_QUAD_BYTES {
                    // More than one undrained maximal batch: a streaming
                    // contract violation, never a silent drop.
                    return Err(abi::STATUS_INSUFFICIENT_CAPACITY);
                }
                self.deferred_quad_bytes += cost;
                self.deferred.push_back(DeferredOp::Upsert { id, packed });
                summary.upserts_deferred += 1;
            }
        }
        Ok(())
    }

    /// Apply one producer drop under the per-call bound, deferring the
    /// remainder. A refused drop (stale or zero revision) is a legal
    /// producer-side arbitration outcome, counted and consumed.
    fn apply_or_defer_drop(&mut self, id: SectionId, summary: &mut TerrainIngestSummary) {
        let applied = summary.drops_applied + summary.drops_refused;
        if applied >= INGEST_DROP_LIMIT {
            self.deferred.push_back(DeferredOp::Drop { id });
            summary.drops_deferred += 1;
            return;
        }
        match self.stage.drop_section(id) {
            Ok(()) => summary.drops_applied += 1,
            Err(_) => summary.drops_refused += 1,
        }
    }

    /// Drop deferred entries selected by `keep` (retained when true),
    /// keeping the byte accounting exact.
    fn retain_deferred(&mut self, keep: impl Fn(SectionCoord) -> bool) {
        let mut index = 0;
        while index < self.deferred.len() {
            let coord = self.deferred[index].coord();
            if keep(coord) {
                index += 1;
            } else {
                let removed = self.deferred.remove(index);
                self.deferred_quad_bytes -= removed.map(|op| op.quad_bytes()).unwrap_or(0);
            }
        }
    }
}

/// Decode the world record's frozen header and operation table. Validation
/// is fail-closed: a short record or a size/shape contradiction reports
/// `STATUS_INPUT_REJECTED`, and a wrong magic or layout word reports
/// `STATUS_ABI_MISMATCH` (record identity precedes every content check, the
/// ABI convention). The header's epoch and atlas revision are consumed by
/// the pull-buffer staleness ruling before this decode runs, so the
/// operation table is the only content this decode surfaces.
fn decode_world_record(record: &[u8]) -> Result<Vec<DecodedOp<'_>>, u32> {
    if record.len() < abi::WORLD_HEADER_BYTES {
        return Err(abi::STATUS_INPUT_REJECTED);
    }
    let le32 = |offset: usize| u32::from_le_bytes(record[offset..offset + 4].try_into().unwrap());
    let le64 = |offset: usize| u64::from_le_bytes(record[offset..offset + 8].try_into().unwrap());
    if le32(0) != abi::MAGIC_WORLD {
        return Err(abi::STATUS_ABI_MISMATCH);
    }
    if le32(4) != abi::WORLD_VERSION {
        return Err(abi::STATUS_ABI_MISMATCH);
    }
    let operation_count = le32(8) as usize;
    let quad_count = le32(12) as usize;
    if operation_count == 0 || operation_count > abi::MAX_WORLD_BATCH_OPERATIONS as usize {
        return Err(abi::STATUS_INPUT_REJECTED);
    }
    if quad_count > abi::MAX_WORLD_BATCH_QUADS as usize {
        return Err(abi::STATUS_INPUT_REJECTED);
    }
    let operations_end = abi::WORLD_HEADER_BYTES + operation_count * WORLD_OPERATION_BYTES;
    let record_end = operations_end + quad_count * QUAD_BYTES;
    if record.len() != record_end {
        return Err(abi::STATUS_INPUT_REJECTED);
    }
    let mut ops = Vec::with_capacity(operation_count);
    let mut quad_offset = operations_end;
    for index in 0..operation_count {
        let base = abi::WORLD_HEADER_BYTES + index * WORLD_OPERATION_BYTES;
        let kind = le32(base);
        let dimension = le32(base + 4);
        let x = i32::from_le_bytes(record[base + 8..base + 12].try_into().unwrap());
        let y = i32::from_le_bytes(record[base + 12..base + 16].try_into().unwrap());
        let z = i32::from_le_bytes(record[base + 16..base + 20].try_into().unwrap());
        let op_quads = le32(base + 20) as usize;
        let revision = le64(base + 24);
        if dimension > 1 {
            // The world family's dimension vocabulary is Overworld and
            // Depths; anything else is a producer drift.
            return Err(abi::STATUS_INPUT_REJECTED);
        }
        let id = SectionId {
            dimension,
            x,
            y,
            z,
            revision,
        };
        match kind {
            WORLD_OP_UPSERT => {
                // A zero-quad upsert is not a wire-legal section operation:
                // the presentation contract requires at least one quad per
                // upsert (removing a section is a drop), so the decode fails
                // closed on the grammar instead of silently re-routing the
                // operation as a drop.
                if op_quads == 0 {
                    return Err(abi::STATUS_INPUT_REJECTED);
                }
                let end = quad_offset + op_quads * QUAD_BYTES;
                if end > record.len() {
                    return Err(abi::STATUS_INPUT_REJECTED);
                }
                ops.push(DecodedOp {
                    upsert: true,
                    id,
                    quads: &record[quad_offset..end],
                });
                quad_offset = end;
            }
            WORLD_OP_DROP => {
                if op_quads != 0 {
                    return Err(abi::STATUS_INPUT_REJECTED);
                }
                ops.push(DecodedOp {
                    upsert: false,
                    id,
                    quads: &[],
                });
            }
            _ => return Err(abi::STATUS_INPUT_REJECTED),
        }
    }
    if quad_offset != record.len() {
        // The quad area must be exactly consumed by the upsert records.
        return Err(abi::STATUS_INPUT_REJECTED);
    }
    Ok(ops)
}

/// Convert one upsert's packed-quad bytes into the worker's `u64` words.
fn decode_packed_quads(quads: &[u8]) -> Result<Vec<u64>, u32> {
    if !quads.len().is_multiple_of(QUAD_BYTES) {
        return Err(abi::STATUS_INPUT_REJECTED);
    }
    Ok(quads
        .chunks_exact(QUAD_BYTES)
        .map(|word| u64::from_le_bytes(word.try_into().unwrap()))
        .collect())
}

/// Wire size of the frame family's fixed camera record, pinned against the
/// Go producer's `FrameCameraRecordBytes` (the pull-buffer module pins the
/// same size against the source declaration).
const FRAME_CAMERA_RECORD_BYTES: usize = 40;

/// Decode the camera position from one pulled frame record. A not-ready
/// camera is the producer's validated zero value, so it decodes like any
/// other record; the derivation then lands on section origin (0, 0).
fn decode_camera_position(record: &[u8]) -> Result<[f32; 3], u32> {
    if record.len() < abi::FRAME_HEADER_BYTES + FRAME_CAMERA_RECORD_BYTES {
        return Err(abi::STATUS_INPUT_REJECTED);
    }
    let le32 = |offset: usize| u32::from_le_bytes(record[offset..offset + 4].try_into().unwrap());
    if le32(0) != abi::MAGIC_FRAME {
        return Err(abi::STATUS_ABI_MISMATCH);
    }
    if le32(4) != abi::FRAME_VERSION {
        return Err(abi::STATUS_ABI_MISMATCH);
    }
    // The ready word itself does not gate the derivation: a not-ready camera
    // reports the validated zero position and the stage treats the origin
    // column as the view center, which is the same honest fallback the old
    // client's scheduling uses before the first confirmed state.
    let _ready = le32(FRAME_CAMERA_READY_OFFSET);
    let position = [
        f32::from_le_bytes(
            record[FRAME_CAMERA_POS_OFFSET..FRAME_CAMERA_POS_OFFSET + 4]
                .try_into()
                .unwrap(),
        ),
        f32::from_le_bytes(
            record[FRAME_CAMERA_POS_OFFSET + 4..FRAME_CAMERA_POS_OFFSET + 8]
                .try_into()
                .unwrap(),
        ),
        f32::from_le_bytes(
            record[FRAME_CAMERA_POS_OFFSET + 8..FRAME_CAMERA_POS_OFFSET + 12]
                .try_into()
                .unwrap(),
        ),
    ];
    Ok(position)
}

/// One axis of the camera section derivation: floor of the block position,
/// then the arithmetic four-bit shift that `core.BlockPos.Section` uses, so
/// negative positions floor toward negative infinity exactly like the
/// server's streaming comparison. Out-of-range positions saturate; the
/// presentation camera domain keeps them unreachable.
fn camera_section_axis(position: f32) -> i32 {
    let block = position.floor() as i64;
    let section = block >> 4;
    i32::try_from(section).unwrap_or(if section < 0 { i32::MIN } else { i32::MAX })
}

/// Test-shared fixtures in the pull-buffer style: one protocol-legal world
/// record and one frame record carrying a chosen camera position, so the
/// bridge-level suites can drive the terrain pipeline with wire bytes
/// instead of internal types.
#[cfg(test)]
pub(crate) mod test_records {
    use super::{WORLD_OP_DROP, WORLD_OP_UPSERT, WORLD_OPERATION_BYTES};
    use crate::abi;
    use crate::mesh_worker::SectionId;

    /// Build one `MCW1` record: the frozen header (epoch and atlas
    /// revision), the upsert records with their packed quads, then the drop
    /// records, mirroring the Go producer's encoding order.
    pub(crate) fn world_record(
        epoch: u64,
        atlas_revision: u64,
        upserts: &[(SectionId, Vec<u64>)],
        drops: &[SectionId],
    ) -> Vec<u8> {
        let quad_total: usize = upserts.iter().map(|(_, packed)| packed.len()).sum();
        let operations = upserts.len() + drops.len();
        let mut record =
            vec![
                0u8;
                abi::WORLD_HEADER_BYTES + operations * WORLD_OPERATION_BYTES + quad_total * 8
            ];
        record[0..4].copy_from_slice(&abi::MAGIC_WORLD.to_le_bytes());
        record[4..8].copy_from_slice(&abi::WORLD_VERSION.to_le_bytes());
        record[8..12].copy_from_slice(&(operations as u32).to_le_bytes());
        record[12..16].copy_from_slice(&(quad_total as u32).to_le_bytes());
        record[16..24].copy_from_slice(&epoch.to_le_bytes());
        record[24..32].copy_from_slice(&atlas_revision.to_le_bytes());
        let mut offset = abi::WORLD_HEADER_BYTES;
        let mut quad_offset = abi::WORLD_HEADER_BYTES + operations * WORLD_OPERATION_BYTES;
        fn put_op(record: &mut [u8], kind: u32, id: SectionId, quads: usize, at: usize) {
            record[at..at + 4].copy_from_slice(&kind.to_le_bytes());
            record[at + 4..at + 8].copy_from_slice(&id.dimension.to_le_bytes());
            record[at + 8..at + 12].copy_from_slice(&id.x.to_le_bytes());
            record[at + 12..at + 16].copy_from_slice(&id.y.to_le_bytes());
            record[at + 16..at + 20].copy_from_slice(&id.z.to_le_bytes());
            record[at + 20..at + 24].copy_from_slice(&(quads as u32).to_le_bytes());
            record[at + 24..at + 32].copy_from_slice(&id.revision.to_le_bytes());
        }
        for (id, packed) in upserts {
            put_op(&mut record, WORLD_OP_UPSERT, *id, packed.len(), offset);
            offset += WORLD_OPERATION_BYTES;
            for word in packed {
                record[quad_offset..quad_offset + 8].copy_from_slice(&word.to_le_bytes());
                quad_offset += 8;
            }
        }
        for id in drops {
            put_op(&mut record, WORLD_OP_DROP, *id, 0, offset);
            offset += WORLD_OPERATION_BYTES;
        }
        record
    }

    /// Build one `MCF1` frame record whose camera record carries `position`
    /// and the ready word; the remaining fixed records stay zeroed because
    /// the camera derivation reads none of them.
    pub(crate) fn frame_record_with_camera(ready: bool, position: [f32; 3]) -> Vec<u8> {
        let mut record = vec![0u8; abi::FRAME_HEADER_BYTES + 40];
        record[0..4].copy_from_slice(&abi::MAGIC_FRAME.to_le_bytes());
        record[4..8].copy_from_slice(&abi::FRAME_VERSION.to_le_bytes());
        record[8..12].copy_from_slice(&abi::FRAME_SNAPSHOT_VERSION.to_le_bytes());
        let camera = abi::FRAME_HEADER_BYTES;
        record[camera..camera + 4].copy_from_slice(&u32::from(ready).to_le_bytes());
        record[camera + 4..camera + 8].copy_from_slice(&position[0].to_le_bytes());
        record[camera + 8..camera + 12].copy_from_slice(&position[1].to_le_bytes());
        record[camera + 12..camera + 16].copy_from_slice(&position[2].to_le_bytes());
        record
    }
}

#[cfg(test)]
mod tests {
    use super::test_records::{frame_record_with_camera, world_record};
    use super::{
        BridgeTerrain, CameraSection, INGEST_DROP_LIMIT, TerrainInputs, camera_section_axis,
    };
    use crate::abi;
    use crate::mesh_worker::SectionId;
    use crate::quad_decode::TestQuad;
    use crate::terrain_budget::TerrainBudgetStage;
    use crate::terrain_resources::render_script::ScriptedBackend;
    use std::thread;
    use std::time::{Duration, Instant};

    const TEST_TIMEOUT: Duration = Duration::from_secs(20);
    const POLL_STEP: Duration = Duration::from_millis(1);

    fn inputs() -> TerrainInputs {
        TerrainInputs {
            opaque: "res://test/opaque.tres".into(),
            cutout: "res://test/cutout.tres".into(),
            water: "res://test/water.tres".into(),
            scenario: "/root/TerrainNear".into(),
        }
    }

    fn section(x: i32, y: i32, z: i32, revision: u64) -> SectionId {
        SectionId {
            dimension: 0,
            x,
            y,
            z,
            revision,
        }
    }

    fn stone_quad() -> u64 {
        TestQuad::default().pack()
    }

    fn assembled() -> (BridgeTerrain, ScriptedBackend) {
        let (renderer, script) = ScriptedBackend::scripted_renderer();
        let stage = TerrainBudgetStage::new(renderer).expect("stage starts");
        (BridgeTerrain::assemble(stage, inputs()), script)
    }

    /// Drive one camera frame per poll until the live section count reaches
    /// `expected`; the frame call is the only path that drains prepared
    /// sections into the renderer.
    fn settle_at(mut terrain: BridgeTerrain, expected: usize) -> BridgeTerrain {
        let frame = frame_record_with_camera(true, [8.0, -32.0, 8.0]);
        let deadline = Instant::now() + TEST_TIMEOUT;
        while terrain.facts().live_sections != expected {
            assert!(
                Instant::now() < deadline,
                "terrain_bridge timed out waiting for {expected} live sections"
            );
            let _ = terrain.frame(&frame);
            thread::sleep(POLL_STEP);
        }
        terrain
    }

    #[test]
    fn terrain_bridge_runs_the_scenario_pipeline_without_stale_sections() {
        let (mut terrain, _script) = assembled();

        // Initial snapshot: one visible upsert plus a drop of an absent
        // section (the all-air mesh the producer legitimately publishes).
        let initial = world_record(1, 1, &[(section(0, 0, 0, 1), vec![stone_quad()])], &[]);
        let summary = terrain.ingest(&initial).expect("initial ingest");
        assert_eq!(summary.operations, 1);
        assert_eq!(summary.upserts_submitted, 1);
        terrain = settle_at(terrain, 1);
        let facts = terrain.facts();
        assert_eq!(facts.live_sections, 1);
        let summaries = terrain.section_summaries();
        assert_eq!(summaries.len(), 1);
        assert_eq!(summaries[0].coord.x, 0);
        assert_eq!(summaries[0].coord.y, 0);
        assert_eq!(summaries[0].coord.z, 0);
        assert_eq!(summaries[0].revision, 1);

        // Delta: a newer revision of the same section plus a new section.
        let delta = world_record(
            1,
            1,
            &[
                (section(0, 0, 0, 2), vec![stone_quad(), stone_quad()]),
                (section(0, 1, 0, 2), vec![stone_quad()]),
            ],
            &[],
        );
        terrain.ingest(&delta).expect("delta ingest");
        terrain = settle_at(terrain, 2);
        let summaries = terrain.section_summaries();
        assert_eq!(summaries.len(), 2);
        assert!(summaries.iter().all(|summary| summary.revision == 2));

        // The structural summary names every live section with its
        // revision, the surface the headless check asserts against.
        let json = terrain.summary_json();
        assert!(json.contains("\"revision\":2"));
        assert!(json.contains("\"live_sections\":2"));
        assert!(json.contains("\"epoch\":1"));

        // Forget: producer drops at newer revisions remove both sections,
        // and the empty structural summary proves no stale section remains.
        let forget = world_record(1, 1, &[], &[section(0, 0, 0, 3), section(0, 1, 0, 3)]);
        let summary = terrain.ingest(&forget).expect("forget ingest");
        assert_eq!(summary.drops_applied, 2);
        assert_eq!(terrain.facts().live_sections, 0);
        assert!(terrain.section_summaries().is_empty());
        assert!(terrain.summary_json().contains("\"sections\":[]"));

        // Reset (the close path) clears the state and bumps the epoch; a
        // fresh ingest repopulates from revision one because the reset RID
        // table holds nothing.
        terrain.reset();
        let facts = terrain.facts();
        assert_eq!(facts.live_sections, 0);
        assert_eq!(facts.resets, 1);
        assert_eq!(facts.epoch, 2);
        let reentry = world_record(1, 1, &[(section(0, 0, 0, 1), vec![stone_quad()])], &[]);
        terrain.ingest(&reentry).expect("re-entry ingest");
        terrain = settle_at(terrain, 1);
        let summaries = terrain.section_summaries();
        assert_eq!(summaries[0].revision, 1);
    }

    #[test]
    fn terrain_bridge_camera_derivation_matches_the_server_floor_rule() {
        let (mut terrain, _script) = assembled();
        // The authoritative rule is floor(position / 16) per horizontal
        // axis, floor toward negative infinity; the frame family's empty
        // record before the first step derives the origin column.
        let empty = terrain.frame(&[]).expect("empty frame derives");
        assert_eq!(
            empty.camera,
            CameraSection {
                dimension: 0,
                x: 0,
                z: 0
            }
        );
        let cases: [([f32; 3], i32, i32); 5] = [
            ([0.5, -32.0, 0.5], 0, 0),
            ([15.9, 0.0, -0.1], 0, -1),
            ([16.0, 0.0, -16.0], 1, -1),
            ([-0.5, 0.0, -17.5], -1, -2),
            ([1000.5, 65.0, -4096.0], 62, -256),
        ];
        for (position, want_x, want_z) in cases {
            let record = frame_record_with_camera(true, position);
            let driven = terrain.frame(&record).expect("camera frame derives");
            assert_eq!(
                driven.camera,
                CameraSection {
                    dimension: 0,
                    x: want_x,
                    z: want_z
                },
                "position {position:?}"
            );
        }
        // The axis helper is the pinned server rule on its own.
        assert_eq!(camera_section_axis(0.0), 0);
        assert_eq!(camera_section_axis(-0.1), -1);
        assert_eq!(camera_section_axis(-16.0), -1);
        assert_eq!(camera_section_axis(-16.1), -2);
    }

    #[test]
    fn terrain_bridge_rejects_malformed_world_and_frame_records() {
        let (mut terrain, _script) = assembled();
        // Wrong magic and layout are identity mismatches; shape violations
        // are input rejections. Nothing reaches the stage on any failure.
        let mut wrong_magic = world_record(1, 1, &[(section(0, 0, 0, 1), vec![stone_quad()])], &[]);
        wrong_magic[0..4].copy_from_slice(&abi::MAGIC_FRAME.to_le_bytes());
        assert_eq!(terrain.ingest(&wrong_magic), Err(abi::STATUS_ABI_MISMATCH));
        assert_eq!(
            terrain.ingest(&world_record(1, 1, &[], &[])),
            Err(abi::STATUS_INPUT_REJECTED)
        );
        assert_eq!(terrain.ingest(&[0u8; 16]), Err(abi::STATUS_INPUT_REJECTED));
        // A zero-quad upsert is a wire-grammar violation, not a drop in
        // disguise: the presentation contract requires at least one quad per
        // upsert, so the decode fails closed instead of re-routing it.
        assert_eq!(
            terrain.ingest(&world_record(
                1,
                1,
                &[(section(0, 0, 0, 1), Vec::new())],
                &[]
            )),
            Err(abi::STATUS_INPUT_REJECTED)
        );
        let frame = frame_record_with_camera(true, [0.0; 3]);
        assert_eq!(terrain.frame(&frame[..70]), Err(abi::STATUS_INPUT_REJECTED));
        let mut wrong_frame_magic = frame_record_with_camera(true, [0.0; 3]);
        wrong_frame_magic[0..4].copy_from_slice(&abi::MAGIC_WORLD.to_le_bytes());
        assert_eq!(
            terrain.frame(&wrong_frame_magic),
            Err(abi::STATUS_ABI_MISMATCH)
        );
        assert_eq!(terrain.facts().live_sections, 0);
        assert_eq!(terrain.facts().packed_input_bytes, 0);
    }

    #[test]
    fn terrain_bridge_bounds_producer_drops_and_replays_the_remainder() {
        let (mut terrain, _script) = assembled();
        // One batch with more drops than the per-call bound: exactly the
        // bound applies, the remainder defers, and the counts stay
        // decodable through the typed ingest summary.
        let drops: Vec<SectionId> = (0..(INGEST_DROP_LIMIT + 10) as i32)
            .map(|index| section(index, 0, 0, 1))
            .collect();
        let batch = world_record(1, 1, &[], &drops);
        let summary = terrain.ingest(&batch).expect("drop batch ingests");
        assert_eq!(summary.operations, drops.len());
        assert_eq!(summary.drops_applied, INGEST_DROP_LIMIT);
        assert_eq!(summary.drops_deferred, 10);
        assert_eq!(summary.deferred_pending, 10);

        // A newer upsert for one DEFERRED coordinate (x=65 landed past the
        // first call's bound) supersedes that queued drop, and the same
        // ingest's replay phase drains the nine surviving drops.
        let supersede = world_record(1, 1, &[(section(65, 0, 0, 5), vec![stone_quad()])], &[]);
        let replayed = terrain
            .ingest(&supersede)
            .expect("superseding batch ingests");
        assert_eq!(replayed.drops_replayed, 9);
        assert_eq!(replayed.upserts_submitted, 1);
        assert_eq!(replayed.deferred_pending, 0);
        // A later empty batch (the steady no-batch outcome) is a no-op.
        let idle = terrain.ingest(&[]).expect("empty batch ingests");
        assert_eq!(idle.operations, 0);
        assert_eq!(terrain.facts().live_sections, 0);
    }

    #[test]
    fn terrain_bridge_camera_dimension_follows_the_ingested_batch() {
        let (mut terrain, _script) = assembled();
        // The frame family carries no dimension; the newest batch's
        // dimension decides, which keeps cross-dimension sections out of
        // the camera's reclamation sweep.
        let record = frame_record_with_camera(true, [0.0, 0.0, 0.0]);
        let driven = terrain.frame(&record).expect("frame derives");
        assert_eq!(driven.camera.dimension, 0);
        let depths_id = SectionId {
            dimension: 1,
            ..section(0, 0, 0, 1)
        };
        let depths_batch = world_record(1, 1, &[(depths_id, vec![stone_quad()])], &[]);
        terrain.ingest(&depths_batch).expect("depths batch ingests");
        let driven = terrain.frame(&record).expect("frame derives");
        assert_eq!(driven.camera.dimension, 1);
    }
}
