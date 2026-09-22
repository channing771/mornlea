/// Failures that reject a domain record before it is published.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum DomainError {
    IncompleteIdentity,
    InvalidDimension,
    InvalidHotbarSlot,
    InvalidIdentity,
    InvalidText,
    NonFiniteRotation,
    UnknownId,
    /// The item number is outside the registered range, so no stack limit,
    /// durability budget or smelting rule exists for it.
    InvalidItem,
    /// The count is zero or above the item's per-slot limit.
    InvalidCount,
    /// The durability is zero on a durable item, above its budget, or nonzero
    /// on a nondurable one.
    InvalidDurability,
    /// The drop slot is at or above the fixed per-chunk drop slot count.
    InvalidDropSlot,
    /// The drop generation is zero, which names a slot that was never used.
    InvalidDropGeneration,
    /// The container slot is outside the addressed kind's fixed per-chunk
    /// array.
    InvalidContainerSlot,
    /// The container generation is zero, which names a slot that was never
    /// used.
    InvalidContainerGeneration,
    /// The container reference names the wrong kind for the record that
    /// carries it: a furnace publication needs a furnace reference and a chest
    /// publication a chest one, which is the Go `validFurnaceRef` /
    /// `validChestRef` kind rule. The container-neutral closure notification
    /// accepts either kind, so it never publishes this rejection.
    InvalidContainerKind,
    /// A personal crafting grid carries a non-empty slot beyond its own cells,
    /// which the Go validator rejects so a client never has to guess whether a
    /// residue slot still applies.
    InvalidCraftingResidue,
    /// A furnace timer is outside its fixed range: the smelt progress has to
    /// stay strictly below the requirement and the burn time at or below its
    /// maximum, which are the Go `core.FurnaceSmeltTicks` and
    /// `core.FurnaceBurnTicks` bounds.
    InvalidFurnaceTimers,
    /// A furnace slot holds an item that slot cannot contain: the input
    /// accepts the empty stack or a registered smelting input, the fuel the
    /// empty stack or coal, and the output the empty stack or a registered
    /// smelting product.
    InvalidFurnaceSlot,
    /// The unified slot index is outside the fixed range of the view the
    /// command addresses.
    InvalidSlot,
    /// The source and the target name the same slot, so the move has no
    /// effect.
    SourceEqualsTarget,
    /// Both ends of a crafting move are inside the inventory region, which
    /// `InventoryMove` already covers.
    CraftingMoveInsideInventory,
    /// The furnace output slot is the target of a container move, while the
    /// authority reserves that slot for taking the smelting product.
    FurnaceOutputAsTarget,
    /// A survival scalar is above the authoritative maximum the Go core
    /// publishes for it: health, oxygen, hunger or armor points.
    InvalidSurvivalValue,
    /// The day phase offset is at or above the length of one display day, so
    /// it no longer names a phase inside the cycle it shifts.
    InvalidDayPhaseOffset,
    /// The weather ID is outside the three published kinds.
    InvalidWeather,
    /// The season ID is outside the four published seasons.
    InvalidSeason,
    /// The wire mining block names no legal union member: an inactive block
    /// still carries a target, a progress, a requirement or the harvestable
    /// flag, or an active block reports a progress that is zero or not below
    /// its requirement.
    InvalidMiningState,
    /// The combat hit carries a zero server tick or a damage value outside
    /// `1..=MAX_HEALTH`, so it confirms no landed hit.
    InvalidCombatHit,
    /// The combat target ID is outside the three published kinds.
    InvalidCombatTarget,
    /// The sequence is zero on a command that has to take part in command
    /// acknowledgement, which is the one sequenced intent whose wire sequence
    /// may not be zero.
    InvalidSequence,
    /// The ordering scratch cannot hold one key slot per command, either
    /// because the requested capacity could not be reserved or because the
    /// batch is larger than the scratch the caller supplied.
    InsufficientScratch,
    /// Two envelopes name the same tick, session and arrival index, so the
    /// intake metadata cannot order them: the arrival index is the producer's
    /// admission position and has to be unique inside one tick and session.
    DuplicateArrival,
    /// The block number is outside the registered range, so no section
    /// storage, palette entry or block-change value can name it.
    InvalidBlock,
    /// The indexed slot width is neither 4 nor 8, so no packed word layout
    /// exists for the section.
    InvalidSectionBits,
    /// The palette is empty, above the `2^bits` capacity, or names one block
    /// twice, so a packed slot cannot resolve to exactly one block.
    InvalidSectionPalette,
    /// The packed word count is not the exact count the slot width requires
    /// for 4096 cells.
    InvalidSectionWords,
    /// A direct word carries bits above its 15-bit slot, which no legal
    /// section sets.
    InvalidSectionHighBits,
    /// A packed slot names a palette index the palette does not hold.
    InvalidSectionSlot,
    /// The revision transition is not exactly `base + 1` from a base inside
    /// `1..=u64::MAX - 1`, so the batch does not continue one chunk history.
    InvalidRevision,
    /// The block Y coordinate is outside the world's vertical span.
    InvalidBlockY,
    /// The position's chunk column disagrees with the chunk the batch
    /// announces.
    InvalidBlockChunk,
    /// The changes are not strictly increasing by chunk-ordered block index,
    /// so the batch is not the canonical order the authority publishes.
    InvalidChangeOrder,
    /// The forget batch is empty or names one chunk twice.
    InvalidForgetChunks,
    /// A state batch carries no record at all, which no authority publishes:
    /// the batch is the delta a subscriber applies, so an empty one names
    /// nothing to apply and the Go validator rejects a count below one.
    EmptyStateBatch,
    /// The batch's identities are not strictly increasing, so the batch is not
    /// the canonical order the authority publishes and a duplicate identity
    /// would apply one record on top of itself.
    InvalidStateOrder,
    /// A companion record names a dimension other than the overworld, which
    /// the Go companion validators reject because both the companion body and
    /// its spawn live in the overworld alone.
    InvalidCompanionDimension,
    /// A companion record's pitch is outside the inclusive vertical look range
    /// the Go `validCompanionPose` predicate publishes, so it describes a look
    /// no companion can have.
    InvalidCompanionPitch,
    /// A position or velocity vector carries a NaN or infinite component, so
    /// no replay can reproduce the body state it would describe. Rotation has
    /// its own narrower error in `NonFiniteRotation`.
    NonFiniteValue,
    /// A semantic batch carries more than `MAX_SEMANTIC_BATCH_RECORDS`
    /// records, so construction rejects it before any per-record scan, copy,
    /// or sort spends work proportional to the oversized input.
    BatchTooLarge,
    /// A construction scratch could not reserve the capacity the batch needs,
    /// so the constructor publishes nothing rather than a partially built
    /// value.
    Allocation,
    /// The chunk-local block index is at or above the chunk's cell count, so
    /// it names no cell inside the chunk the record announces and the value
    /// is rejected rather than clamped to the last cell.
    InvalidBlockIndex,
}

/// Reports whether the 16 bytes are a non-zero UUIDv4 in wire order.
///
/// The rule is the Go `core.PlayerID.Valid` gate: a zero value, a version
/// nibble other than `4`, or a variant outside RFC-4122 is rejected. Every
/// identity newtype in this crate funnels through it so the player and
/// companion rules cannot drift apart.
fn is_uuid_v4(bytes: [u8; 16]) -> bool {
    bytes != [0; 16] && bytes[6] >> 4 == 4 && bytes[8] & 0xc0 == 0x80
}

/// Stable UUIDv4 player identity.
///
/// The wire layout is the standard big-endian UUID byte order, and the
/// identity is a value type: cloning copies the bytes and the type stays
/// distinct from a companion identity so a record cannot name the wrong
/// subject.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct PlayerId([u8; 16]);

impl PlayerId {
    /// Wraps wire bytes, rejecting a zero, non-v4, or non-RFC-4122 variant.
    pub fn try_from_bytes(bytes: [u8; 16]) -> Result<Self, DomainError> {
        if !is_uuid_v4(bytes) {
            return Err(DomainError::InvalidIdentity);
        }
        Ok(Self(bytes))
    }

    pub fn bytes(self) -> [u8; 16] {
        self.0
    }
}

/// Stable UUIDv4 companion identity.
///
/// The byte rule is identical to `PlayerId`, but the type is separate because
/// a companion names a different subject with its own ordering space. This
/// crate publishes no absent identity: a wire-level "no companion" form is a
/// protocol concern and stays out of the domain value set.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct CompanionId([u8; 16]);

impl CompanionId {
    /// Wraps wire bytes, rejecting a zero, non-v4, or non-RFC-4122 variant.
    pub fn try_from_bytes(bytes: [u8; 16]) -> Result<Self, DomainError> {
        if !is_uuid_v4(bytes) {
            return Err(DomainError::InvalidIdentity);
        }
        Ok(Self(bytes))
    }

    pub fn bytes(self) -> [u8; 16] {
        self.0
    }
}

/// Reports whether a scalar entity identifier is publishable: zero is the
/// absent form in every entity family, so it is rejected here instead of
/// being interpreted downstream.
fn is_nonzero_entity_id(id: u64) -> bool {
    id != 0
}

/// Nonzero authoritative night-walker identity.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct HostileId(u64);

impl HostileId {
    pub fn try_new(id: u64) -> Result<Self, DomainError> {
        if !is_nonzero_entity_id(id) {
            return Err(DomainError::InvalidIdentity);
        }
        Ok(Self(id))
    }

    pub fn get(self) -> u64 {
        self.0
    }
}

/// Nonzero authoritative passive-mob identity.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct PassiveId(u64);

impl PassiveId {
    pub fn try_new(id: u64) -> Result<Self, DomainError> {
        if !is_nonzero_entity_id(id) {
            return Err(DomainError::InvalidIdentity);
        }
        Ok(Self(id))
    }

    pub fn get(self) -> u64 {
        self.0
    }
}

/// Nonzero authoritative projectile identity.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct ProjectileId(u64);

impl ProjectileId {
    pub fn try_new(id: u64) -> Result<Self, DomainError> {
        if !is_nonzero_entity_id(id) {
            return Err(DomainError::InvalidIdentity);
        }
        Ok(Self(id))
    }

    pub fn get(self) -> u64 {
        self.0
    }
}

/// Current supported contract versions shared by replay and inventory identity.
///
/// Values must stay aligned with `testdata/runtime-migration/contracts.json`.
/// `current_identities_match_frozen_inventory` fails if either side drifts.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Identities {
    pub protocol: u32,
    pub chunk_schema: u32,
    pub player_schema: u32,
    pub world_metadata: u32,
    pub companions_ai_schema: u32,
    pub hostile_mobs_schema: u32,
    pub passive_mobs_schema: u32,
    pub engine_abi: u32,
    pub region_format: u32,
    pub agent_http: String,
    pub agent_mcp: String,
}

impl Identities {
    pub fn current() -> Self {
        Self {
            protocol: 45,
            chunk_schema: 9,
            player_schema: 9,
            world_metadata: 6,
            companions_ai_schema: 5,
            hostile_mobs_schema: 2,
            passive_mobs_schema: 1,
            engine_abi: 11,
            region_format: 1,
            agent_http: "v1".into(),
            agent_mcp: "v1".into(),
        }
    }

    /// Rejects a zero version or empty agent contract label so incomplete
    /// evidence cannot be counted as agreement.
    pub fn validate(&self) -> Result<(), DomainError> {
        if self.protocol == 0
            || self.chunk_schema == 0
            || self.player_schema == 0
            || self.world_metadata == 0
            || self.companions_ai_schema == 0
            || self.hostile_mobs_schema == 0
            || self.passive_mobs_schema == 0
            || self.engine_abi == 0
            || self.region_format == 0
            || self.agent_http.trim().is_empty()
            || self.agent_mcp.trim().is_empty()
        {
            return Err(DomainError::IncompleteIdentity);
        }
        Ok(())
    }
}

/// Replay identity consumed by later offline differential comparison.
///
/// Construction fails closed on a missing source revision, corpus digest, or
/// incomplete contract version set. The constructor does not write files.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ReplayIdentity {
    pub source_revision: String,
    pub corpus_digest: String,
    pub identities: Identities,
    pub seed: u64,
}

impl ReplayIdentity {
    pub fn new(
        source_revision: impl Into<String>,
        corpus_digest: impl Into<String>,
        identities: Identities,
        seed: u64,
    ) -> Result<Self, DomainError> {
        let source_revision = source_revision.into();
        let corpus_digest = corpus_digest.into();
        if source_revision.trim().is_empty() || corpus_digest.trim().is_empty() {
            return Err(DomainError::IncompleteIdentity);
        }
        identities.validate()?;
        Ok(Self {
            source_revision,
            corpus_digest,
            identities,
            seed,
        })
    }
}
