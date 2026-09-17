//! Reusable FFI-side pull buffers for the client-core two-phase capacity
//! protocol.
//!
//! The bridge holds one [`PullBuffer`] per pull family per session
//! ([`PullBufferSet`]); every pull queries the required size, grows the
//! family's aligned backing to exactly the declared requirement (word
//! granularity, never beyond the family's contract limit), and lets the
//! producer perform its one exact write into the reused backing. The record
//! handed to Godot is still an owned copy; reuse removes the per-pull buffer
//! allocation on the FFI side, which the contract's frozen maximums make
//! meaningful (a maximal world batch alone is 4,325,408 bytes). The
//! never-shrink bound applies per bridge session: each live bridge node owns
//! one buffer set, so the pilot's worst-case retention is one maximal world
//! backing per session.
//!
//! Growth policy: grow to the declared required size exactly, never beyond
//! the family's contract limit — a required size above the limit is a
//! producer contract violation that fails closed with the internal status
//! before any allocation — and never shrink, because the largest record ever
//! served defines the steady-state footprint and shrinking would reintroduce
//! allocation churn at exactly the boundary the contract bounds.
//!
//! Stale-epoch ruling: the world and frame frozen headers carry a publication
//! identity (a monotonic epoch plus a monotonic revision). Each buffer
//! remembers the newest identity it served and refuses a strictly older
//! record with the internal status instead of re-serving old bytes as fresh,
//! mirroring the producer's ruling that pulls always serve the latest
//! completed step. An equal identity remains legal because frame and status
//! pulls are non-consuming and repeat identical bytes between steps, and the
//! consuming world family also legitimately repeats an equal identity with
//! different content: every batch inside one mesh epoch shares the same
//! (epoch, atlas_revision) pair. Status
//! and identity records carry no publication epoch, so those families track
//! nothing; the bridge resets the tracked identities whenever a new producer
//! session is created, because a fresh session's epoch space legitimately
//! restarts.
//!
//! Boundary note: this module holds no unsafe code. The aligned byte views
//! handed to the producer are constructed by the client-core FFI module, the
//! crate's single sanctioned home of unsafe code.

use core::mem::offset_of;

use crate::abi;
use crate::client_core::{AlignedBytes, ClientHandle, CoreCalls, PullFamily, PullOutcome};

/// Fixed wire size of one world operation record. The frozen header owns the
/// `MCW1` header, the counts, and the limits; the body record layout is
/// producer-side wire vocabulary pinned against the Go producer's
/// `WorldOperationBytes` by the source test below.
const WORLD_OPERATION_BYTES: usize = 32;

/// Wire size of one packed quad: the engine's 8-byte packed face.
const WORLD_PACKED_QUAD_BYTES: usize = 8;

/// Fixed wire sizes of the frame family's body records (camera, HUD,
/// environment, entity-batch tick, per-entity, target, phase/error), pinned
/// against the Go producer's `Frame*RecordBytes` declarations by the source
/// test below.
const FRAME_CAMERA_RECORD_BYTES: usize = 40;
const FRAME_HUD_RECORD_BYTES: usize = 16;
const FRAME_ENVIRONMENT_RECORD_BYTES: usize = 48;
const FRAME_ENTITY_TICK_RECORD_BYTES: usize = 8;
const FRAME_ENTITY_RECORD_BYTES: usize = 48;
const FRAME_TARGET_RECORD_BYTES: usize = 16;
const FRAME_PHASE_ERROR_RECORD_BYTES: usize = 8;

/// Fixed wire size of every status record, pinned against the producer's
/// `StatusRecordBytes`.
const STATUS_RECORD_BYTES: usize = 16;

/// Records in the pilot status record set (phase, terminal cause, steps
/// completed, messages processed), pinned against the producer's
/// `metricsWireBytes(4)` call.
const STATUS_PILOT_RECORDS: usize = 4;

/// The contract limit of one pull family: the largest record the producer
/// may ever report as required. Composed from the header-pinned limits and
/// the producer wire vocabulary above, so a size a producer reports beyond
/// its own declared maximum is a contract violation, not a growth request.
fn family_limit_bytes(family: PullFamily) -> usize {
    match family {
        PullFamily::World => {
            abi::WORLD_HEADER_BYTES
                + abi::MAX_WORLD_BATCH_OPERATIONS as usize * WORLD_OPERATION_BYTES
                + abi::MAX_WORLD_BATCH_QUADS as usize * WORLD_PACKED_QUAD_BYTES
        }
        PullFamily::Frame => {
            abi::FRAME_HEADER_BYTES
                + FRAME_CAMERA_RECORD_BYTES
                + FRAME_HUD_RECORD_BYTES
                + FRAME_ENVIRONMENT_RECORD_BYTES
                + FRAME_ENTITY_TICK_RECORD_BYTES
                + abi::MAX_ENTITY_RECORDS as usize * FRAME_ENTITY_RECORD_BYTES
                + FRAME_TARGET_RECORD_BYTES
                + FRAME_PHASE_ERROR_RECORD_BYTES
                + abi::MAX_TARGET_NAME_BYTES as usize
        }
        PullFamily::Status => abi::STATUS_HEADER_BYTES + STATUS_PILOT_RECORDS * STATUS_RECORD_BYTES,
        PullFamily::Identity => {
            abi::IDENTITY_HEADER_BYTES + abi::FAMILY_COUNT as usize * abi::FAMILY_DESCRIPTOR_BYTES
        }
    }
}

/// The publication identity one served record carried: a monotonic epoch
/// plus a monotonic revision inside it. Derived order is lexicographic
/// (epoch first), which is exactly the producer's monotonicity contract for
/// the world (`epoch`, `atlas_revision`) and frame (`epoch`, `revision`)
/// header pairs.
#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
struct PublicationIdentity {
    epoch: u64,
    revision: u64,
}

/// Read the publication identity out of one completed record's frozen
/// header. The world header carries `epoch` then `atlas_revision`; the frame
/// header carries `revision` then `epoch`; both offsets come from the
/// header-pinned `#[repr(C)]` mirrors, so a layout bump moves them with it.
/// Status and identity records carry no publication epoch and track
/// nothing. A record shorter than its own frozen header cannot carry the
/// identity and fails closed with the internal status.
///
/// Trust boundary: the identity fields are read without re-validating the
/// header magic. In production the only writer of the backing is the
/// producer, whose success status commits a complete frozen header by
/// construction, so a garbage identity can only follow a producer contract
/// violation — and its consequence (later pulls refused with the internal
/// status) still fails closed.
fn publication_identity(
    family: PullFamily,
    backing: &AlignedBytes,
    written: usize,
) -> Result<Option<PublicationIdentity>, u32> {
    match family {
        PullFamily::World => {
            if written < abi::WORLD_HEADER_BYTES {
                return Err(abi::STATUS_INTERNAL);
            }
            Ok(Some(PublicationIdentity {
                epoch: backing.le_u64_at(offset_of!(abi::WorldHeader, epoch)),
                revision: backing.le_u64_at(offset_of!(abi::WorldHeader, atlas_revision)),
            }))
        }
        PullFamily::Frame => {
            if written < abi::FRAME_HEADER_BYTES {
                return Err(abi::STATUS_INTERNAL);
            }
            Ok(Some(PublicationIdentity {
                epoch: backing.le_u64_at(offset_of!(abi::FrameHeader, epoch)),
                revision: backing.le_u64_at(offset_of!(abi::FrameHeader, revision)),
            }))
        }
        PullFamily::Status | PullFamily::Identity => Ok(None),
    }
}

/// One pull family's reusable FFI-side buffer: the aligned backing the
/// two-phase driver writes each record into, plus the served-identity state
/// of the stale-epoch ruling (see the module documentation). The backing
/// starts empty, so a session that never pulls a family never allocates for
/// it; it grows to the declared requirement exactly and never shrinks.
#[derive(Default)]
pub(crate) struct PullBuffer {
    backing: AlignedBytes,
    /// How many times the backing grew; the reuse-accounting observable.
    grow_events: usize,
    /// The newest publication identity served through this buffer.
    served: Option<PublicationIdentity>,
}

impl PullBuffer {
    /// Set the exact-write span to `required` bytes, growing the backing to
    /// the requirement (word granularity) when capacity is insufficient and
    /// counting the growth for reuse accounting.
    fn reserve_span(&mut self, required: usize) {
        if self.backing.reserve_span(required) {
            self.grow_events += 1;
        }
    }
}

/// Test-only reuse accounting: the growth count and the byte capacity of
/// the aligned backing.
#[cfg(test)]
impl PullBuffer {
    fn grow_events(&self) -> usize {
        self.grow_events
    }

    fn capacity_bytes(&self) -> usize {
        self.backing.capacity_bytes()
    }
}

/// The per-session set of reusable pull buffers, one per family. Families
/// differ by four orders of magnitude in their contract limits and carry
/// independent served identities, so each family keeps its own backing; a
/// single world-sized shared buffer would pin the maximal footprint even for
/// sessions that never pull world data.
#[derive(Default)]
pub(crate) struct PullBufferSet {
    world: PullBuffer,
    frame: PullBuffer,
    status: PullBuffer,
    identity: PullBuffer,
}

impl PullBufferSet {
    /// The reusable buffer of one pull family.
    pub(crate) fn for_family(&mut self, family: PullFamily) -> &mut PullBuffer {
        match family {
            PullFamily::World => &mut self.world,
            PullFamily::Frame => &mut self.frame,
            PullFamily::Status => &mut self.status,
            PullFamily::Identity => &mut self.identity,
        }
    }

    /// Forget every served publication identity while keeping the backings
    /// for reuse. The bridge calls this whenever a new producer session is
    /// created (and on close): wire epochs are per producer session, so a
    /// fresh session's epoch space legitimately restarts below the retired
    /// session's newest identity.
    pub(crate) fn reset_served_identities(&mut self) {
        self.world.served = None;
        self.frame.served = None;
        self.status.served = None;
        self.identity.served = None;
    }
}

/// One exact-write phase against the buffer's current span.
fn exact_write(
    calls: &dyn CoreCalls,
    family: PullFamily,
    handle: ClientHandle,
    buffer: &mut PullBuffer,
) -> PullOutcome {
    buffer
        .backing
        .with_span_view(|view| family.pull(calls, handle, view))
}

/// Drive one pull family through the two-phase capacity protocol into the
/// family's reusable buffer and return the record as an owned byte vector.
///
/// The query phase is a zero-capacity call: the no-record outcome
/// (`STATUS_OK` with size zero) answers with an empty record and touches
/// neither the backing span nor the served identity, so bytes retained from
/// an earlier record can never leak out as a fresh record. A required size
/// beyond the family's contract limit is a producer contract violation that
/// fails closed with the internal status before any allocation. The
/// exact-write phase feeds a span of exactly the declared requirement from
/// the reusable backing; a record that grew between the query and the write
/// allows exactly one retry with the fresh size (the same limit check
/// applies to it). A written length that disagrees with the declared span on
/// the first write fails closed with the internal word; any contradiction
/// after the sanctioned retry, including a disagreeing retry length, fails
/// closed with the capacity word. On every non-OK path the served identity
/// stays untouched and no byte is handed back.
///
/// On a completed write the stale-epoch ruling applies (see the module
/// documentation): the record's publication identity is read from its frozen
/// header, a strictly older identity is refused with the internal status
/// instead of being re-served as fresh, and an equal or newer identity
/// becomes the buffer's served state before the owned copy is returned.
pub(crate) fn pull_via_buffer(
    calls: &dyn CoreCalls,
    family: PullFamily,
    handle: ClientHandle,
    buffer: &mut PullBuffer,
) -> Result<Vec<u8>, u32> {
    let required = match family.pull(calls, handle, &mut []) {
        PullOutcome::Complete { written: 0 } => return Ok(Vec::new()),
        // A completed write into a zero-capacity query contradicts the
        // two-phase protocol; fail closed instead of guessing a length.
        PullOutcome::Complete { .. } => return Err(abi::STATUS_INTERNAL),
        PullOutcome::Capacity { required: 0 } => return Err(abi::STATUS_INTERNAL),
        PullOutcome::Capacity { required } => required as usize,
        PullOutcome::Status(word) => return Err(word),
    };
    let limit = family_limit_bytes(family);
    if required > limit {
        return Err(abi::STATUS_INTERNAL);
    }
    buffer.reserve_span(required);
    let span = match exact_write(calls, family, handle, buffer) {
        PullOutcome::Complete { written } if written as usize == required => required,
        PullOutcome::Complete { .. } => return Err(abi::STATUS_INTERNAL),
        PullOutcome::Capacity { required: fresh } if fresh as usize > required => {
            let fresh = fresh as usize;
            if fresh > limit {
                return Err(abi::STATUS_INTERNAL);
            }
            buffer.reserve_span(fresh);
            match exact_write(calls, family, handle, buffer) {
                PullOutcome::Complete { written } if written as usize == fresh => fresh,
                // A producer that signals capacity twice in a row after the
                // one sanctioned retry is a contradiction.
                _ => return Err(abi::STATUS_INSUFFICIENT_CAPACITY),
            }
        }
        PullOutcome::Capacity { .. } => return Err(abi::STATUS_INSUFFICIENT_CAPACITY),
        PullOutcome::Status(word) => return Err(word),
    };
    let identity = publication_identity(family, &buffer.backing, span)?;
    if let Some(identity) = identity {
        if buffer.served.is_some_and(|served| identity < served) {
            // Discard the stale-epoch record: old bytes must never be
            // re-served as fresh, and the served state must not move.
            return Err(abi::STATUS_INTERNAL);
        }
        buffer.served = Some(identity);
    }
    Ok(buffer.backing.to_owned_prefix(span))
}

/// Build one world wire record with the frozen `MCW1` header (magic, layout,
/// operation and quad counts, epoch, atlas revision) and a deterministic
/// payload of `operations` 32-byte operation records plus `quads` packed
/// quads. Test-shared fixture: the bridge-level buffer tests reuse it.
#[cfg(test)]
pub(crate) fn test_world_record(
    epoch: u64,
    atlas_revision: u64,
    operations: usize,
    quads: usize,
) -> Vec<u8> {
    let length = abi::WORLD_HEADER_BYTES
        + operations * WORLD_OPERATION_BYTES
        + quads * WORLD_PACKED_QUAD_BYTES;
    let mut record = vec![0u8; length];
    record[0..4].copy_from_slice(&abi::MAGIC_WORLD.to_le_bytes());
    record[4..8].copy_from_slice(&abi::WORLD_VERSION.to_le_bytes());
    record[8..12].copy_from_slice(&u32::try_from(operations).unwrap_or(u32::MAX).to_le_bytes());
    record[12..16].copy_from_slice(&u32::try_from(quads).unwrap_or(u32::MAX).to_le_bytes());
    record[16..24].copy_from_slice(&epoch.to_le_bytes());
    record[24..32].copy_from_slice(&atlas_revision.to_le_bytes());
    for (index, byte) in record[abi::WORLD_HEADER_BYTES..].iter_mut().enumerate() {
        *byte = (index % 251) as u8;
    }
    record
}

/// Build one frame wire record with the frozen `MCF1` header (magic, layout,
/// snapshot version, zero entity count, zero name length, revision, epoch)
/// and every fixed body record with a deterministic payload: 176 bytes, the
/// minimum frame record. Test-shared fixture like `test_world_record`.
#[cfg(test)]
pub(crate) fn test_frame_record(epoch: u64, revision: u64) -> Vec<u8> {
    let length = abi::FRAME_HEADER_BYTES
        + FRAME_CAMERA_RECORD_BYTES
        + FRAME_HUD_RECORD_BYTES
        + FRAME_ENVIRONMENT_RECORD_BYTES
        + FRAME_ENTITY_TICK_RECORD_BYTES
        + FRAME_TARGET_RECORD_BYTES
        + FRAME_PHASE_ERROR_RECORD_BYTES;
    let mut record = vec![0u8; length];
    record[0..4].copy_from_slice(&abi::MAGIC_FRAME.to_le_bytes());
    record[4..8].copy_from_slice(&abi::FRAME_VERSION.to_le_bytes());
    record[8..12].copy_from_slice(&abi::FRAME_SNAPSHOT_VERSION.to_le_bytes());
    // Entity count, name length, and the reserved word stay zero.
    record[24..32].copy_from_slice(&revision.to_le_bytes());
    record[32..40].copy_from_slice(&epoch.to_le_bytes());
    for (index, byte) in record[abi::FRAME_HEADER_BYTES..].iter_mut().enumerate() {
        *byte = (index % 241) as u8;
    }
    record
}

#[cfg(test)]
mod tests {
    use super::{
        FRAME_CAMERA_RECORD_BYTES, FRAME_ENTITY_RECORD_BYTES, FRAME_ENTITY_TICK_RECORD_BYTES,
        FRAME_ENVIRONMENT_RECORD_BYTES, FRAME_HUD_RECORD_BYTES, FRAME_PHASE_ERROR_RECORD_BYTES,
        FRAME_TARGET_RECORD_BYTES, PullBuffer, PullBufferSet, STATUS_PILOT_RECORDS,
        STATUS_RECORD_BYTES, WORLD_OPERATION_BYTES, family_limit_bytes, pull_via_buffer,
        test_frame_record, test_world_record,
    };
    use crate::abi;
    use crate::client_core::{ClientHandle, CoreCalls, PullFamily, PullOutcome};
    use std::cell::{RefCell, RefMut};
    use std::collections::VecDeque;
    use std::rc::Rc;

    const WORLD_GO: &str = include_str!("../../../../client/cmd/mornlea-godot-core/world.go");
    const FRAME_GO: &str = include_str!("../../../../client/cmd/mornlea-godot-core/frame.go");
    const STATUS_GO: &str = include_str!("../../../../client/cmd/mornlea-godot-core/status.go");

    /// A fixed live handle: the driver under test passes it through and this
    /// seam does not script handle vocabulary (the bridge tests own that).
    const LIVE: ClientHandle = ClientHandle::from_word(1);

    /// What the scripted producer observed of one caller buffer: its address,
    /// its declared span, and the bytes it held before the scripted outcome
    /// applied. Pointer and content observations are the reuse-accounting
    /// evidence: a reused backing keeps its address and failed calls leave
    /// the previous record's bytes in place.
    struct ObservedView {
        address: usize,
        length: usize,
        contents_before: Vec<u8>,
    }

    /// The mutable half of the scripted pull seam.
    struct ScriptState {
        outcomes: VecDeque<PullOutcome>,
        bytes: Vec<u8>,
        views: Vec<ObservedView>,
        pulls: usize,
    }

    /// A scripted core-call table for the buffer driver: every non-pull
    /// export reports the internal status (unused here) and the four pull
    /// families share one scripted outcome queue, mirroring the producer's
    /// contract that a query never consumes and only a completed write
    /// touches the caller buffer.
    #[derive(Clone)]
    struct ScriptedCore {
        state: Rc<RefCell<ScriptState>>,
    }

    impl ScriptedCore {
        fn new() -> Self {
            Self {
                state: Rc::new(RefCell::new(ScriptState {
                    outcomes: VecDeque::new(),
                    bytes: Vec::new(),
                    views: Vec::new(),
                    pulls: 0,
                })),
            }
        }

        fn state(&self) -> RefMut<'_, ScriptState> {
            self.state.borrow_mut()
        }

        fn script(&self, outcomes: &[PullOutcome], bytes: &[u8]) {
            let mut state = self.state();
            state.outcomes = outcomes.iter().copied().collect();
            state.bytes = bytes.to_vec();
        }

        /// The views recorded so far as (address, length) pairs.
        fn view_facts(&self) -> Vec<(usize, usize)> {
            self.state()
                .views
                .iter()
                .map(|view| (view.address, view.length))
                .collect()
        }
    }

    impl CoreCalls for ScriptedCore {
        fn abi_version(&self) -> Option<u64> {
            Some((u64::from(abi::ABI_MAJOR) << 32) | u64::from(abi::ABI_MINOR))
        }

        fn create_session(
            &self,
            _abi_major: u32,
            _abi_minor: u32,
            _requested_families: &[u64],
        ) -> Result<ClientHandle, u32> {
            Err(abi::STATUS_INTERNAL)
        }

        fn destroy_session(&self, _handle: ClientHandle) -> u32 {
            abi::STATUS_INTERNAL
        }

        fn connect_begin(&self, _handle: ClientHandle, _address: &[u8]) -> u32 {
            abi::STATUS_INTERNAL
        }

        fn connect_poll(&self, _handle: ClientHandle) -> Result<u32, u32> {
            Err(abi::STATUS_INTERNAL)
        }

        fn disconnect_session(&self, _handle: ClientHandle) -> u32 {
            abi::STATUS_INTERNAL
        }

        fn submit_input(&self, _handle: ClientHandle, _batch: &[u8]) -> u32 {
            abi::STATUS_INTERNAL
        }

        fn step(&self, _handle: ClientHandle, _request: &[u8; abi::STEP_REQUEST_BYTES]) -> u32 {
            abi::STATUS_INTERNAL
        }

        fn world_pull(&self, _handle: ClientHandle, buffer: &mut [u8]) -> PullOutcome {
            self.scripted_pull(buffer)
        }

        fn frame_pull(&self, _handle: ClientHandle, buffer: &mut [u8]) -> PullOutcome {
            self.scripted_pull(buffer)
        }

        fn status_pull(&self, _handle: ClientHandle, buffer: &mut [u8]) -> PullOutcome {
            self.scripted_pull(buffer)
        }

        fn identity_pull(&self, _handle: ClientHandle, buffer: &mut [u8]) -> PullOutcome {
            self.scripted_pull(buffer)
        }
    }

    impl ScriptedCore {
        /// Pop the next scripted outcome, record the caller buffer as
        /// observed, and fill the buffer from the scripted bytes on a
        /// completed write (the producer's one exact write).
        fn scripted_pull(&self, buffer: &mut [u8]) -> PullOutcome {
            let mut state = self.state();
            state.pulls += 1;
            state.views.push(ObservedView {
                address: buffer.as_ptr() as usize,
                length: buffer.len(),
                contents_before: buffer.to_vec(),
            });
            let outcome = state
                .outcomes
                .pop_front()
                .unwrap_or(PullOutcome::Status(abi::STATUS_INTERNAL));
            if let PullOutcome::Complete { written } = outcome {
                for index in 0..buffer.len().min(written as usize) {
                    buffer[index] = state.bytes.get(index).copied().unwrap_or(0);
                }
            }
            outcome
        }
    }

    /// Drive one pull through the buffer driver under test.
    fn drive(
        core: &ScriptedCore,
        family: PullFamily,
        buffer: &mut PullBuffer,
    ) -> Result<Vec<u8>, u32> {
        pull_via_buffer(core, family, LIVE, buffer)
    }

    /// Script one successful two-phase pull of `record`: the query reports
    /// the record's exact size and the write completes it.
    fn script_record(core: &ScriptedCore, record: &[u8]) {
        let required = u32::try_from(record.len()).expect("record length fits u32");
        core.script(
            &[
                PullOutcome::Capacity { required },
                PullOutcome::Complete { written: required },
            ],
            record,
        );
    }

    /// Collapse every whitespace run to one space so source-text pins do not
    /// depend on wrapping.
    fn normalize_whitespace(text: &str) -> String {
        text.split_whitespace().collect::<Vec<_>>().join(" ")
    }

    /// Parse one decimal Go constant (`const Name = N` or a block entry
    /// `Name = N`) from a producer source file.
    fn go_const(source: &str, name: &str) -> usize {
        for raw in source.lines() {
            let Some((head, tail)) = raw.split_once('=') else {
                continue;
            };
            if head.trim().trim_start_matches("const").trim() != name {
                continue;
            }
            let digits: String = tail
                .trim()
                .chars()
                .take_while(|character| character.is_ascii_digit())
                .collect();
            assert!(
                !digits.is_empty(),
                "Go const {name} carries no decimal literal"
            );
            return digits.parse().expect("decimal Go const value");
        }
        panic!("Go const {name} was not found");
    }

    #[test]
    fn pull_buffers_family_limits_pin_the_producer_wire_vocabulary() {
        // The composed contract limits equal the producer's documented
        // single-allocation ceilings: a maximal world batch (header plus
        // 4096 operation records plus 524,288 packed quads), a maximal frame
        // (header plus every fixed record, seven entities, the 64-byte
        // name), the fixed pilot status record set, and the identity record
        // with the full seven-descriptor table.
        assert_eq!(family_limit_bytes(PullFamily::World), 4_325_408);
        assert_eq!(family_limit_bytes(PullFamily::Frame), 576);
        assert_eq!(family_limit_bytes(PullFamily::Status), 80);
        assert_eq!(family_limit_bytes(PullFamily::Identity), 192);

        // The body record sizes are producer-side wire vocabulary (the
        // frozen header owns only the headers and counts), so this pin
        // reads the Go producer's declarations directly: any size change
        // without this mirror failing loudly would silently move the limit.
        assert_eq!(
            go_const(WORLD_GO, "WorldOperationBytes"),
            WORLD_OPERATION_BYTES
        );
        for (name, value) in [
            ("FrameCameraRecordBytes", FRAME_CAMERA_RECORD_BYTES),
            ("FrameHUDRecordBytes", FRAME_HUD_RECORD_BYTES),
            (
                "FrameEnvironmentRecordBytes",
                FRAME_ENVIRONMENT_RECORD_BYTES,
            ),
            ("FrameEntityTickRecordBytes", FRAME_ENTITY_TICK_RECORD_BYTES),
            ("FrameEntityRecordBytes", FRAME_ENTITY_RECORD_BYTES),
            ("FrameTargetRecordBytes", FRAME_TARGET_RECORD_BYTES),
            ("FramePhaseErrorRecordBytes", FRAME_PHASE_ERROR_RECORD_BYTES),
        ] {
            assert_eq!(go_const(FRAME_GO, name), value, "frame record {name}");
        }
        assert_eq!(
            go_const(STATUS_GO, "StatusRecordBytes"),
            STATUS_RECORD_BYTES
        );
        assert_eq!(STATUS_PILOT_RECORDS, 4);
        // The packed quad is the 8-byte packed face and the pilot status
        // set holds exactly four records, per the producer's wire functions.
        let world = normalize_whitespace(WORLD_GO);
        assert!(world.contains("packedQuads*8"), "packed quad byte width");
        let status = normalize_whitespace(STATUS_GO);
        assert!(
            status.contains("metricsWireBytes(4)"),
            "pilot status record count"
        );
    }

    #[test]
    fn pull_buffers_grow_to_the_declared_required_size_exactly() {
        let core = ScriptedCore::new();
        let mut buffer = PullBuffer::default();

        // A first pull grows the empty backing to exactly the declared
        // requirement (word granularity: 192 needs no padding).
        let record = vec![5u8; 192];
        script_record(&core, &record);
        assert_eq!(
            drive(&core, PullFamily::Identity, &mut buffer),
            Ok(record.clone())
        );
        assert_eq!(buffer.grow_events(), 1);
        assert_eq!(buffer.capacity_bytes(), 192);

        // A smaller record reuses the same backing without shrinking it and
        // without a second growth event.
        let small = vec![9u8; 96];
        script_record(&core, &small);
        assert_eq!(drive(&core, PullFamily::Identity, &mut buffer), Ok(small));
        assert_eq!(buffer.grow_events(), 1);
        assert_eq!(buffer.capacity_bytes(), 192);

        // A larger record grows to the new requirement exactly. The padded
        // size (380 bytes) is deliberately not word-aligned so the capacity
        // assert below proves the backing rounds up to the 8-byte word
        // granularity.
        let larger = test_frame_record(1, 1);
        let larger_padded = larger.len() + 201;
        let mut padded = larger.clone();
        padded.resize(larger_padded, 0xAB);
        script_record(&core, &padded);
        assert_eq!(drive(&core, PullFamily::Frame, &mut buffer), Ok(padded));
        assert_eq!(buffer.grow_events(), 2);
        // 380 bytes round up to the 8-byte word granularity.
        assert_eq!(buffer.capacity_bytes(), larger_padded.div_ceil(8) * 8);
        assert_eq!(buffer.capacity_bytes(), 384);
    }

    #[test]
    fn pull_buffers_required_size_beyond_the_contract_limit_fails_closed() {
        let core = ScriptedCore::new();
        for (family, limit) in [
            (PullFamily::World, 4_325_408),
            (PullFamily::Frame, 576),
            (PullFamily::Status, 80),
            (PullFamily::Identity, 192),
        ] {
            let mut buffer = PullBuffer::default();
            core.script(
                &[PullOutcome::Capacity {
                    required: limit as u32 + 1,
                }],
                &[],
            );
            assert_eq!(
                drive(&core, family, &mut buffer),
                Err(abi::STATUS_INTERNAL),
                "over-limit query for {family:?}"
            );
            // Fail closed before any allocation: one query call, no
            // backing, no growth.
            assert_eq!(core.state().pulls, 1);
            assert_eq!(buffer.grow_events(), 0);
            assert_eq!(buffer.capacity_bytes(), 0);
            core.state().pulls = 0;
        }

        // A fresh size beyond the limit on the retry path fails closed the
        // same way, keeping the backing at the first requirement.
        let mut buffer = PullBuffer::default();
        core.script(
            &[
                PullOutcome::Capacity { required: 100 },
                PullOutcome::Capacity {
                    required: 4_325_409,
                },
            ],
            &[],
        );
        assert_eq!(
            drive(&core, PullFamily::World, &mut buffer),
            Err(abi::STATUS_INTERNAL)
        );
        assert_eq!(core.state().pulls, 2);
        assert_eq!(buffer.capacity_bytes(), 104);

        // The exact limit itself is servable.
        let mut buffer = PullBuffer::default();
        let record = vec![1u8; 80];
        script_record(&core, &record);
        assert_eq!(drive(&core, PullFamily::Status, &mut buffer), Ok(record));
        assert_eq!(buffer.capacity_bytes(), 80);
    }

    #[test]
    fn pull_buffers_repeated_empty_batches_never_serve_the_previous_record() {
        let core = ScriptedCore::new();
        let mut buffer = PullBuffer::default();

        // Seed one world record so the buffer holds real bytes and a served
        // identity.
        let seeded = test_world_record(5, 1, 1, 4);
        script_record(&core, &seeded);
        assert_eq!(
            drive(&core, PullFamily::World, &mut buffer),
            Ok(seeded.clone())
        );
        assert_eq!(core.state().pulls, 2);

        // Repeated no-batch outcomes answer with an empty record: each is a
        // single query call that touches neither the backing span nor the
        // served identity, so the retained bytes can never leak out.
        for _ in 0..3 {
            core.script(&[PullOutcome::Complete { written: 0 }], &[]);
            assert_eq!(drive(&core, PullFamily::World, &mut buffer), Ok(Vec::new()));
        }
        assert_eq!(core.state().pulls, 5);

        // The served identity survived the empty batches: a stale-epoch
        // record is still refused, and the previous record is not re-served.
        let stale = test_world_record(4, 9, 1, 4);
        script_record(&core, &stale);
        assert_eq!(
            drive(&core, PullFamily::World, &mut buffer),
            Err(abi::STATUS_INTERNAL)
        );

        // The equal identity still re-serves legally (non-consuming
        // semantics allow repeats between steps).
        script_record(&core, &seeded);
        assert_eq!(drive(&core, PullFamily::World, &mut buffer), Ok(seeded));
    }

    #[test]
    fn pull_buffers_maximum_world_batch_round_trips_without_regrowth() {
        let core = ScriptedCore::new();
        let mut buffer = PullBuffer::default();

        // The maximal world batch: 32-byte header, 4096 operation records,
        // 524,288 packed quads — exactly the family's contract limit.
        let first = test_world_record(1, 1, 4096, 524_288);
        assert_eq!(first.len(), 4_325_408);
        script_record(&core, &first);
        assert_eq!(drive(&core, PullFamily::World, &mut buffer), Ok(first));

        // One growth event, capacity at the limit, and the write landed at a
        // stable backing address (the zero-capacity query passes an empty
        // span, whose dangling address is not the backing).
        assert_eq!(buffer.grow_events(), 1);
        assert_eq!(buffer.capacity_bytes(), 4_325_408);
        let views = core.view_facts();
        assert_eq!(views[0].1, 0);
        assert_eq!(views[1].1, 4_325_408);

        // A second maximal batch with a newer epoch reuses the backing
        // byte-for-byte with no reallocation.
        let second = test_world_record(2, 1, 4096, 524_288);
        script_record(&core, &second);
        assert_eq!(drive(&core, PullFamily::World, &mut buffer), Ok(second));
        assert_eq!(buffer.grow_events(), 1);
        assert_eq!(buffer.capacity_bytes(), 4_325_408);
        let views = core.view_facts();
        assert_eq!(views[3].0, views[1].0, "the backing address is reused");
        assert_eq!(views[3].1, 4_325_408);
    }

    #[test]
    fn pull_buffers_invalid_batches_leave_buffer_and_served_state_untouched() {
        let core = ScriptedCore::new();
        let mut buffer = PullBuffer::default();

        // Seed one frame record; its bytes stay in the reused backing.
        let seeded = test_frame_record(3, 9);
        script_record(&core, &seeded);
        assert_eq!(
            drive(&core, PullFamily::Frame, &mut buffer),
            Ok(seeded.clone())
        );
        assert_eq!(buffer.grow_events(), 1);

        // An error status on the write phase leaves the backing bytes as the
        // producer contract demands (failed pulls write nothing).
        core.script(
            &[
                PullOutcome::Capacity { required: 176 },
                PullOutcome::Status(abi::STATUS_INTERNAL),
            ],
            &[],
        );
        assert_eq!(
            drive(&core, PullFamily::Frame, &mut buffer),
            Err(abi::STATUS_INTERNAL)
        );
        {
            let state = core.state();
            let observed = &state.views[3];
            assert_eq!(observed.length, 176);
            assert_eq!(observed.contents_before, seeded);
        }

        // A capacity signal whose retry also fails closes the driver without
        // touching the served identity; the growth to the fresh size is
        // capacity, not content, and the previous record's bytes survive it.
        core.script(
            &[
                PullOutcome::Capacity { required: 176 },
                PullOutcome::Capacity { required: 200 },
                PullOutcome::Capacity { required: 220 },
            ],
            &[],
        );
        assert_eq!(
            drive(&core, PullFamily::Frame, &mut buffer),
            Err(abi::STATUS_INSUFFICIENT_CAPACITY)
        );
        {
            let state = core.state();
            // The first write attempt still holds the seeded bytes; the
            // grown retry span holds them unchanged in its prefix (new
            // capacity zero-fills only the padding).
            let attempt = &state.views[5];
            assert_eq!(attempt.length, 176);
            assert_eq!(attempt.contents_before, seeded);
            let retry = &state.views[6];
            assert_eq!(retry.length, 200);
            assert_eq!(&retry.contents_before[..176], &seeded[..]);
        }

        // The failed pulls never advanced the served identity: a record
        // regressing below the seeded one is still refused, and the equal
        // identity still re-serves.
        let regression = test_frame_record(3, 8);
        script_record(&core, &regression);
        assert_eq!(
            drive(&core, PullFamily::Frame, &mut buffer),
            Err(abi::STATUS_INTERNAL)
        );
        script_record(&core, &seeded);
        assert_eq!(drive(&core, PullFamily::Frame, &mut buffer), Ok(seeded));
    }

    #[test]
    fn pull_buffers_discard_stale_epoch_records_and_accept_equal_re_serves() {
        let core = ScriptedCore::new();

        // World: epochs and atlas revisions are monotonic; every strictly
        // older pair is refused, equal pairs re-serve, newer pairs replace.
        let mut world = PullBuffer::default();
        script_record(&core, &test_world_record(5, 1, 1, 0));
        assert!(drive(&core, PullFamily::World, &mut world).is_ok());
        script_record(&core, &test_world_record(6, 1, 1, 0));
        assert!(drive(&core, PullFamily::World, &mut world).is_ok());
        script_record(&core, &test_world_record(5, 9, 1, 0));
        assert_eq!(
            drive(&core, PullFamily::World, &mut world),
            Err(abi::STATUS_INTERNAL)
        );
        script_record(&core, &test_world_record(6, 1, 1, 0));
        assert!(drive(&core, PullFamily::World, &mut world).is_ok());
        script_record(&core, &test_world_record(6, 7, 1, 0));
        assert!(drive(&core, PullFamily::World, &mut world).is_ok());
        script_record(&core, &test_world_record(6, 3, 1, 0));
        assert_eq!(
            drive(&core, PullFamily::World, &mut world),
            Err(abi::STATUS_INTERNAL)
        );

        // Frame: the same ruling over the frame revision inside one epoch,
        // and a newer epoch starts a fresh revision space.
        let mut frame = PullBuffer::default();
        script_record(&core, &test_frame_record(2, 10));
        assert!(drive(&core, PullFamily::Frame, &mut frame).is_ok());
        script_record(&core, &test_frame_record(2, 9));
        assert_eq!(
            drive(&core, PullFamily::Frame, &mut frame),
            Err(abi::STATUS_INTERNAL)
        );
        script_record(&core, &test_frame_record(2, 10));
        assert!(drive(&core, PullFamily::Frame, &mut frame).is_ok());
        script_record(&core, &test_frame_record(3, 1));
        assert!(drive(&core, PullFamily::Frame, &mut frame).is_ok());

        // The buffer recovers after a refusal: the next fresh record serves.
        script_record(&core, &test_world_record(7, 1, 1, 0));
        assert!(drive(&core, PullFamily::World, &mut world).is_ok());
    }

    #[test]
    fn pull_buffers_records_shorter_than_their_frozen_header_fail_closed() {
        let core = ScriptedCore::new();
        // A world record shorter than its own 32-byte header cannot carry
        // the publication identity the stale-epoch ruling reads; fail
        // closed instead of decoding past the record.
        let mut world = PullBuffer::default();
        core.script(
            &[
                PullOutcome::Capacity { required: 16 },
                PullOutcome::Complete { written: 16 },
            ],
            &[0x16; 16],
        );
        assert_eq!(
            drive(&core, PullFamily::World, &mut world),
            Err(abi::STATUS_INTERNAL)
        );

        let mut frame = PullBuffer::default();
        core.script(
            &[
                PullOutcome::Capacity { required: 32 },
                PullOutcome::Complete { written: 32 },
            ],
            &[0x20; 32],
        );
        assert_eq!(
            drive(&core, PullFamily::Frame, &mut frame),
            Err(abi::STATUS_INTERNAL)
        );

        // The status family carries no publication identity; a 16-byte
        // record is exactly its frozen header and serves normally.
        let mut status = PullBuffer::default();
        script_record(&core, &[0x11; 16]);
        assert_eq!(
            drive(&core, PullFamily::Status, &mut status),
            Ok(vec![0x11; 16])
        );
    }

    #[test]
    fn pull_buffers_steady_state_pulls_reuse_one_backing_allocation() {
        let core = ScriptedCore::new();
        let mut buffers = PullBufferSet::default();

        // Repeated same-size records across three families allocate each
        // backing exactly once: the growth count stays at one per family and
        // every exact write lands at the same backing address.
        let identity_record = vec![3u8; 192];
        let status_record = vec![4u8; 80];
        let frame_record = test_frame_record(1, 1);
        for _ in 0..4 {
            script_record(&core, &identity_record);
            assert_eq!(
                drive(
                    &core,
                    PullFamily::Identity,
                    buffers.for_family(PullFamily::Identity)
                ),
                Ok(identity_record.clone())
            );
            script_record(&core, &status_record);
            assert_eq!(
                drive(
                    &core,
                    PullFamily::Status,
                    buffers.for_family(PullFamily::Status)
                ),
                Ok(status_record.clone())
            );
            script_record(&core, &frame_record);
            assert_eq!(
                drive(
                    &core,
                    PullFamily::Frame,
                    buffers.for_family(PullFamily::Frame)
                ),
                Ok(frame_record.clone())
            );
        }

        let views = core.view_facts();
        let writes: Vec<(usize, usize)> = views
            .iter()
            .copied()
            .filter(|(_, length)| *length > 0)
            .collect();
        assert_eq!(writes.len(), 12);
        // The three families interleave, so group the write views by span:
        // every span of one family shares a single stable backing address.
        for family_length in [192usize, 80, 176] {
            let family_writes: Vec<(usize, usize)> = writes
                .iter()
                .copied()
                .filter(|(_, length)| *length == family_length)
                .collect();
            assert_eq!(
                family_writes.len(),
                4,
                "four writes of span {family_length}"
            );
            assert!(
                family_writes
                    .iter()
                    .all(|view| view.0 == family_writes[0].0),
                "one stable backing address per family"
            );
        }
        for family in [
            PullFamily::World,
            PullFamily::Frame,
            PullFamily::Status,
            PullFamily::Identity,
        ] {
            assert_eq!(
                buffers.for_family(family).grow_events(),
                if family == PullFamily::World { 0 } else { 1 }
            );
        }

        // Resetting the served identities (the bridge does this whenever a
        // new producer session is created) keeps the backing for reuse.
        let world_record = test_world_record(9, 1, 1, 0);
        script_record(&core, &world_record);
        assert_eq!(
            drive(
                &core,
                PullFamily::World,
                buffers.for_family(PullFamily::World)
            ),
            Ok(world_record.clone())
        );
        buffers.reset_served_identities();
        // A fresh session's epoch space legitimately restarts at one.
        let restarted = test_world_record(1, 1, 1, 0);
        script_record(&core, &restarted);
        assert_eq!(
            drive(
                &core,
                PullFamily::World,
                buffers.for_family(PullFamily::World)
            ),
            Ok(restarted)
        );
    }
}
