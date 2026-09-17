use godot::classes::{INode, Node};
use godot::prelude::*;

use crate::abi;
use crate::client_core::{
    self, ClientHandle, CoreCalls, ProducerLoadFailure, PullFamily, producer_identity,
    production_core_calls,
};
use crate::feature_negotiation::PILOT_FAMILIES;
use crate::lifecycle;
use crate::pull_buffers::{PullBufferSet, pull_via_buffer};

const GODOT_API_MAJOR: i64 = 4;
const GODOT_API_MINOR: i64 = 7;
const GODOT_RUST_VERSION: &str = "0.5.5";
#[cfg(test)]
pub(crate) const BRIDGE_CLASS_NAME: &str = "MornleaClientBridge";
#[cfg(test)]
pub(crate) const IDENTITY_METHODS: [&str; 7] = [
    "client_core_abi_major",
    "client_core_abi_minor",
    "godot_api_major",
    "godot_api_minor",
    "godot_rust_version",
    "lifecycle_stage",
    "supports_godot_api",
];

/// The Godot-independent lifecycle state of one bridge-held client session.
///
/// The bridge holds at most one producer handle plus one set of reusable
/// FFI-side pull buffers (see `crate::pull_buffers`): every pull still hands
/// Python an owned `Vec<u8>` copy, so nothing here needs destruction beyond
/// the idempotent handle release. Every method returns the producer status
/// vocabulary (or the local invalid-state word for calls made without a
/// session), mapping every status word without panicking.
pub(crate) struct BridgeSession {
    calls: Box<dyn CoreCalls>,
    handle: Option<ClientHandle>,
    buffers: PullBufferSet,
}

impl BridgeSession {
    pub(crate) fn new(calls: Box<dyn CoreCalls>) -> Self {
        Self {
            calls,
            handle: None,
            buffers: PullBufferSet::default(),
        }
    }

    /// Create one producer session requesting the pinned pilot families at
    /// their pinned contract versions. A bridge holds at most one session, so
    /// a second create without a close reports invalid state locally. The
    /// buffers' served identities reset on success because wire epochs are
    /// per producer session and a fresh session's epoch space restarts.
    pub(crate) fn create(&mut self) -> u32 {
        if self.handle.is_some() {
            return abi::STATUS_INVALID_STATE;
        }
        let mut requested = Vec::with_capacity(PILOT_FAMILIES.len());
        for descriptor in &PILOT_FAMILIES {
            requested.push((u64::from(descriptor.version) << 32) | u64::from(descriptor.family));
        }
        match self
            .calls
            .create_session(abi::ABI_MAJOR, abi::ABI_MINOR, &requested)
        {
            Ok(handle) => {
                self.handle = Some(handle);
                self.buffers.reset_served_identities();
                abi::STATUS_OK
            }
            Err(word) => word,
        }
    }

    /// Begin one asynchronous connection; the producer owns every address
    /// domain check and never blocks the calling thread.
    pub(crate) fn connect(&mut self, address: &str) -> u32 {
        let Some(handle) = self.handle else {
            return abi::STATUS_INVALID_STATE;
        };
        self.calls.connect_begin(handle, address.as_bytes())
    }

    /// Poll the connection phase: the status word plus the phase word, which
    /// is meaningful only on success.
    pub(crate) fn poll(&mut self) -> (u32, u32) {
        let Some(handle) = self.handle else {
            return (abi::STATUS_INVALID_STATE, 0);
        };
        match self.calls.connect_poll(handle) {
            Ok(phase) => (abi::STATUS_OK, phase),
            Err(word) => (word, 0),
        }
    }

    /// Submit one complete input-family batch; the batch bytes are passed
    /// through to the producer unchanged.
    pub(crate) fn submit(&mut self, batch: &[u8]) -> u32 {
        let Some(handle) = self.handle else {
            return abi::STATUS_INVALID_STATE;
        };
        self.calls.submit_input(handle, batch)
    }

    /// Drive exactly one bounded step; the frozen request record is encoded
    /// by the client-core seam.
    pub(crate) fn step(&mut self, elapsed_ns: u64, message_budget: u32, mesh_budget: u32) -> u32 {
        let Some(handle) = self.handle else {
            return abi::STATUS_INVALID_STATE;
        };
        let request = client_core::encode_step_request(elapsed_ns, message_budget, mesh_budget);
        self.calls.step(handle, &request)
    }

    /// Pull one family record through the two-phase protocol into the
    /// family's reusable FFI-side buffer, returning an owned byte vector.
    pub(crate) fn pull(&mut self, family: PullFamily) -> Result<Vec<u8>, u32> {
        let Some(handle) = self.handle else {
            return Err(abi::STATUS_INVALID_STATE);
        };
        let calls = self.calls.as_ref();
        let buffer = self.buffers.for_family(family);
        pull_via_buffer(calls, family, handle, buffer)
    }

    /// Release the session idempotently: the first close destroys the handle
    /// (which also cancels and joins an in-flight connection) and every later
    /// close is a local no-op that still reports success. The buffers' served
    /// identities reset so a later session's fresh epoch space cannot be
    /// mistaken for a stale one.
    pub(crate) fn close(&mut self) -> u32 {
        let Some(handle) = self.handle.take() else {
            return abi::STATUS_OK;
        };
        self.buffers.reset_served_identities();
        self.calls.destroy_session(handle)
    }
}

impl Drop for BridgeSession {
    fn drop(&mut self) {
        // Godot frees the node during scene teardown; the handle release lands
        // here, before the boxed core-call table drops, so the producer
        // session is always destroyed before any library state goes away.
        self.close();
    }
}

/// The project-owned native bridge: identity statics for qualification plus
/// one scene-held session surface whose every value is a Godot-owned typed
/// value (integers, strings, dictionaries, packed byte arrays). Python never
/// receives a pointer, a raw buffer it must free, or a producer status it
/// cannot classify.
///
/// Main-thread discipline: Godot dispatches node calls on the main thread and
/// the bridge guards every instance method with `godot::init::is_main_thread`
/// anyway, answering off-thread calls with the internal status word instead
/// of panicking, so the single session is never raced.
#[derive(GodotClass)]
#[class(base=Node)]
struct MornleaClientBridge {
    session: BridgeSession,
    #[base]
    base: Base<Node>,
}

#[godot_api]
impl INode for MornleaClientBridge {
    fn init(base: Base<Node>) -> Self {
        Self {
            session: BridgeSession::new(production_core_calls()),
            base,
        }
    }
}

#[godot_api]
impl MornleaClientBridge {
    // Identity calls are deliberately allocation-free except for the Godot string
    // conversion and do not create a second path to engine or gameplay state.
    // The client-core identity comes from the header-pinned abi module, so a
    // header bump moves the Godot-visible identity with it.
    #[func]
    fn client_core_abi_major() -> i64 {
        i64::from(abi::ABI_MAJOR)
    }

    #[func]
    fn client_core_abi_minor() -> i64 {
        i64::from(abi::ABI_MINOR)
    }

    #[func]
    fn godot_api_major() -> i64 {
        GODOT_API_MAJOR
    }

    #[func]
    fn godot_api_minor() -> i64 {
        GODOT_API_MINOR
    }

    #[func]
    fn godot_rust_version() -> GString {
        GODOT_RUST_VERSION.into()
    }

    #[func]
    fn lifecycle_stage() -> GString {
        lifecycle::current_stage().into()
    }

    #[func]
    fn supports_godot_api(major: i64, minor: i64) -> bool {
        lifecycle::supports_godot_api(major, minor)
    }

    /// The pinned pilot feature-family table as JSON, one object per family
    /// with `family`, `version`, `record_limit`, and `record_bytes`, in
    /// ascending family order. This is the Rust mirror the bridge negotiates
    /// with; the live producer table is cross-checked through
    /// `pull_identity`.
    #[func]
    fn feature_families_json() -> GString {
        let mut json = String::from("[");
        for (index, descriptor) in PILOT_FAMILIES.iter().enumerate() {
            if index > 0 {
                json.push(',');
            }
            json.push_str(&format!(
                "{{\"family\":{},\"record_bytes\":{},\"record_limit\":{},\"version\":{}}}",
                descriptor.family,
                descriptor.record_bytes,
                descriptor.record_limit,
                descriptor.version
            ));
        }
        json.push(']');
        json.as_str().into()
    }

    /// The live producer identity as JSON: `available`, and on success the
    /// producer-reported `major` and `minor` (or `reason` when the library
    /// beside the extension could not be loaded). This is the only method that
    /// observes the real producer before any session exists.
    #[func]
    fn producer_identity_json() -> GString {
        let identity = producer_identity();
        match identity.packed_version {
            Some(packed) => {
                let json = format!(
                    "{{\"available\":true,\"major\":{},\"minor\":{}}}",
                    packed >> 32,
                    packed & 0xFFFF_FFFF
                );
                json.as_str().into()
            }
            None => {
                let reason = identity
                    .failure
                    .map(ProducerLoadFailure::reason)
                    .unwrap_or("producer identity is unavailable");
                format!("{{\"available\":false,\"reason\":\"{reason}\"}}")
                    .as_str()
                    .into()
            }
        }
    }

    /// Create one session over the pinned pilot families; returns the
    /// producer status word (0 means the bridge now holds a live session).
    #[func]
    fn session_create(&mut self) -> i64 {
        if !Self::on_main_thread() {
            return i64::from(abi::STATUS_INTERNAL);
        }
        i64::from(self.session.create())
    }

    /// Begin one asynchronous connection to a "host:port" address; returns
    /// the producer status word. The call never blocks the main thread.
    #[func]
    fn session_connect(&mut self, address: GString) -> i64 {
        if !Self::on_main_thread() {
            return i64::from(abi::STATUS_INTERNAL);
        }
        i64::from(self.session.connect(&address.to_string()))
    }

    /// Poll the connection once: a dictionary with `status` and, when the
    /// status is 0, the producer `phase` word (otherwise 0).
    #[func]
    fn session_poll(&mut self) -> VarDictionary {
        let polled = if Self::on_main_thread() {
            self.session.poll()
        } else {
            (abi::STATUS_INTERNAL, 0)
        };
        let mut result = VarDictionary::new();
        result.set("status", i64::from(polled.0));
        result.set("phase", i64::from(polled.1));
        result
    }

    /// Submit one complete input-family batch (the packed wire record);
    /// returns the producer status word.
    #[func]
    fn session_submit(&mut self, batch: PackedByteArray) -> i64 {
        if !Self::on_main_thread() {
            return i64::from(abi::STATUS_INTERNAL);
        }
        i64::from(self.session.submit(&batch.to_vec()))
    }

    /// Drive exactly one bounded step. Negative or oversized arguments wrap
    /// into the producer's rejection domains instead of panicking on the
    /// integer conversion.
    #[func]
    fn session_step(&mut self, elapsed_ns: i64, message_budget: i64, mesh_budget: i64) -> i64 {
        if !Self::on_main_thread() {
            return i64::from(abi::STATUS_INTERNAL);
        }
        let elapsed = elapsed_ns as u64;
        let messages = u32::try_from(message_budget).unwrap_or(u32::MAX);
        let meshes = u32::try_from(mesh_budget).unwrap_or(u32::MAX);
        i64::from(self.session.step(elapsed, messages, meshes))
    }

    /// Drain the retained world batch: a dictionary with `status` and
    /// `record` (a Godot-owned packed byte array; empty when no batch is
    /// pending).
    #[func]
    fn pull_world(&mut self) -> VarDictionary {
        self.pull_dictionary(PullFamily::World)
    }

    /// Read the per-step frame snapshot: the same dictionary shape; the
    /// record is empty until a step published a frame.
    #[func]
    fn pull_frame(&mut self) -> VarDictionary {
        self.pull_dictionary(PullFamily::Frame)
    }

    /// Read the status and metrics record set: the same dictionary shape.
    #[func]
    fn pull_status(&mut self) -> VarDictionary {
        self.pull_dictionary(PullFamily::Status)
    }

    /// Read the producer identity record: the same dictionary shape.
    #[func]
    fn pull_identity(&mut self) -> VarDictionary {
        self.pull_dictionary(PullFamily::Identity)
    }

    /// Release the session idempotently; repeated closes and the node's
    /// scene-teardown drop are all safe and report success.
    #[func]
    fn session_close(&mut self) -> i64 {
        if !Self::on_main_thread() {
            return i64::from(abi::STATUS_INTERNAL);
        }
        i64::from(self.session.close())
    }

    /// One pull as the typed dictionary every pull shares: the producer
    /// status word and a Godot-owned copy of the record bytes. The record is
    /// never a retained native buffer.
    fn pull_dictionary(&mut self, family: PullFamily) -> VarDictionary {
        let outcome = if Self::on_main_thread() {
            self.session.pull(family)
        } else {
            Err(abi::STATUS_INTERNAL)
        };
        let mut result = VarDictionary::new();
        match outcome {
            Ok(record) => {
                let record_array = PackedByteArray::from(record);
                result.set("status", i64::from(abi::STATUS_OK));
                result.set("record", &record_array);
            }
            Err(word) => {
                let empty = PackedByteArray::new();
                result.set("status", i64::from(word));
                result.set("record", &empty);
            }
        }
        result
    }

    /// The crate's main-thread check; instance methods answer off-thread
    /// calls with the internal status word instead of panicking.
    fn on_main_thread() -> bool {
        godot::init::is_main_thread()
    }
}

#[cfg(test)]
mod tests {
    use super::{BridgeSession, GODOT_API_MAJOR, GODOT_API_MINOR, GODOT_RUST_VERSION};
    use crate::abi::{
        self, ABI_MAJOR, ABI_MINOR, STATUS_ABI_MISMATCH, STATUS_INPUT_REJECTED,
        STATUS_INSUFFICIENT_CAPACITY, STATUS_INTERNAL, STATUS_INVALID_HANDLE, STATUS_INVALID_STATE,
        STATUS_OK,
    };
    use crate::client_core::{
        ClientHandle, CoreCalls, PullFamily, PullOutcome, encode_step_request,
        production_core_calls,
    };
    use crate::feature_negotiation::PILOT_FAMILIES;
    use std::cell::RefCell;
    use std::rc::Rc;

    #[test]
    fn bridge_identity_matches_pinned_dependencies() {
        assert_eq!((i64::from(ABI_MAJOR), i64::from(ABI_MINOR)), (1, 0));
        assert_eq!((GODOT_API_MAJOR, GODOT_API_MINOR), (4, 7));
        assert_eq!(GODOT_RUST_VERSION, "0.5.5");
    }

    /// The mutable half of the scripted core-call table.
    struct ScriptState {
        next_handle_word: u64,
        live: Vec<ClientHandle>,
        retired: Vec<ClientHandle>,
        create_status: u32,
        connect_status: u32,
        poll_result: Result<u32, u32>,
        submit_status: u32,
        step_status: u32,
        pull_outcomes: std::collections::VecDeque<PullOutcome>,
        pull_bytes: Vec<u8>,
        create_requests: Vec<(u32, u32, Vec<u64>)>,
        connect_addresses: Vec<Vec<u8>>,
        submitted_batches: Vec<Vec<u8>>,
        step_requests: Vec<[u8; abi::STEP_REQUEST_BYTES]>,
        destroy_calls: usize,
        pull_calls: usize,
        poll_calls: usize,
        pull_views: Vec<(usize, usize)>,
    }

    /// A scripted core-call table shared with the test through an `Rc`, so the
    /// bridge can own its boxed copy while the test keeps inspecting and
    /// scripting the same state. It mirrors the producer's handle vocabulary
    /// exactly (live handle, retired tombstone with idempotent destroy,
    /// anything else invalid) while each family call is scripted by the test.
    #[derive(Clone)]
    struct ScriptedCore {
        state: Rc<RefCell<ScriptState>>,
    }

    impl ScriptedCore {
        fn new() -> Self {
            Self {
                state: Rc::new(RefCell::new(ScriptState {
                    next_handle_word: 1,
                    live: Vec::new(),
                    retired: Vec::new(),
                    create_status: STATUS_OK,
                    connect_status: STATUS_OK,
                    poll_result: Ok(1),
                    submit_status: STATUS_INVALID_STATE,
                    step_status: STATUS_INVALID_STATE,
                    pull_outcomes: std::collections::VecDeque::new(),
                    pull_bytes: Vec::new(),
                    create_requests: Vec::new(),
                    connect_addresses: Vec::new(),
                    submitted_batches: Vec::new(),
                    step_requests: Vec::new(),
                    destroy_calls: 0,
                    pull_calls: 0,
                    poll_calls: 0,
                    pull_views: Vec::new(),
                })),
            }
        }

        fn state(&self) -> std::cell::RefMut<'_, ScriptState> {
            self.state.borrow_mut()
        }

        fn script_pulls(&self, outcomes: &[PullOutcome], bytes: &[u8]) {
            let mut state = self.state();
            state.pull_outcomes = outcomes.iter().copied().collect();
            state.pull_bytes = bytes.to_vec();
        }
    }

    impl CoreCalls for ScriptedCore {
        fn abi_version(&self) -> Option<u64> {
            Some((u64::from(ABI_MAJOR) << 32) | u64::from(ABI_MINOR))
        }

        fn create_session(
            &self,
            abi_major: u32,
            abi_minor: u32,
            requested_families: &[u64],
        ) -> Result<ClientHandle, u32> {
            let mut state = self.state();
            state
                .create_requests
                .push((abi_major, abi_minor, requested_families.to_vec()));
            if state.create_status != STATUS_OK {
                return Err(state.create_status);
            }
            let handle = ClientHandle::from_word(state.next_handle_word);
            state.next_handle_word += 1;
            state.live.push(handle);
            Ok(handle)
        }

        fn destroy_session(&self, handle: ClientHandle) -> u32 {
            let mut state = self.state();
            state.destroy_calls += 1;
            if let Some(index) = state.live.iter().position(|live| *live == handle) {
                state.live.remove(index);
                state.retired.push(handle);
                return STATUS_OK;
            }
            if state.retired.contains(&handle) {
                return STATUS_OK;
            }
            STATUS_INVALID_HANDLE
        }

        fn connect_begin(&self, handle: ClientHandle, address: &[u8]) -> u32 {
            let mut state = self.state();
            if let Some(status) = require_live(&state, handle) {
                return status;
            }
            state.connect_addresses.push(address.to_vec());
            state.connect_status
        }

        fn connect_poll(&self, handle: ClientHandle) -> Result<u32, u32> {
            let mut state = self.state();
            if let Some(status) = require_live(&state, handle) {
                return Err(status);
            }
            state.poll_calls += 1;
            state.poll_result
        }

        fn disconnect_session(&self, handle: ClientHandle) -> u32 {
            let state = self.state();
            if let Some(status) = require_live(&state, handle) {
                return status;
            }
            STATUS_OK
        }

        fn submit_input(&self, handle: ClientHandle, batch: &[u8]) -> u32 {
            let mut state = self.state();
            if let Some(status) = require_live(&state, handle) {
                return status;
            }
            state.submitted_batches.push(batch.to_vec());
            state.submit_status
        }

        fn step(&self, handle: ClientHandle, request: &[u8; abi::STEP_REQUEST_BYTES]) -> u32 {
            let mut state = self.state();
            if let Some(status) = require_live(&state, handle) {
                return status;
            }
            state.step_requests.push(*request);
            state.step_status
        }

        fn world_pull(&self, handle: ClientHandle, buffer: &mut [u8]) -> PullOutcome {
            self.scripted_pull(handle, buffer)
        }

        fn frame_pull(&self, handle: ClientHandle, buffer: &mut [u8]) -> PullOutcome {
            self.scripted_pull(handle, buffer)
        }

        fn status_pull(&self, handle: ClientHandle, buffer: &mut [u8]) -> PullOutcome {
            self.scripted_pull(handle, buffer)
        }

        fn identity_pull(&self, handle: ClientHandle, buffer: &mut [u8]) -> PullOutcome {
            self.scripted_pull(handle, buffer)
        }
    }

    impl ScriptedCore {
        /// The scripted primitive pull shared by every family: pop the next
        /// scripted outcome, record the caller buffer's address and span (the
        /// reuse-accounting evidence for the buffer tests), and fill the
        /// caller buffer from the scripted bytes on a completed write.
        fn scripted_pull(&self, handle: ClientHandle, buffer: &mut [u8]) -> PullOutcome {
            let mut state = self.state();
            if let Some(status) = require_live(&state, handle) {
                return PullOutcome::Status(status);
            }
            state.pull_calls += 1;
            state
                .pull_views
                .push((buffer.as_ptr() as usize, buffer.len()));
            let outcome = state
                .pull_outcomes
                .pop_front()
                .unwrap_or(PullOutcome::Status(STATUS_INTERNAL));
            if let PullOutcome::Complete { written } = outcome {
                for index in 0..buffer.len().min(written as usize) {
                    buffer[index] = state.pull_bytes.get(index).copied().unwrap_or(0);
                }
            }
            outcome
        }
    }

    /// The producer's handle ruling as a helper: a retired tombstone reports
    /// invalid state, an unknown value invalid handle, and a live handle
    /// nothing.
    fn require_live(state: &ScriptState, handle: ClientHandle) -> Option<u32> {
        if state.retired.contains(&handle) {
            return Some(STATUS_INVALID_STATE);
        }
        if !state.live.contains(&handle) {
            return Some(STATUS_INVALID_HANDLE);
        }
        None
    }

    /// A bridge session over a scripted table, with the table handle kept for
    /// scripting and inspection.
    fn scripted_session() -> (BridgeSession, ScriptedCore) {
        let core = ScriptedCore::new();
        let session = BridgeSession::new(Box::new(core.clone()));
        (session, core)
    }

    #[test]
    fn bridge_lifecycle_unavailable_production_table_fails_closed() {
        // The cargo-test production wiring is the fail-closed unavailable
        // table (no test may load the producer library), so creation reports
        // the internal word and no handle ever exists: every later call stays
        // the local invalid state and close stays clean.
        let mut session = BridgeSession::new(production_core_calls());
        assert_eq!(session.create(), STATUS_INTERNAL);
        assert_eq!(session.poll(), (STATUS_INVALID_STATE, 0));
        assert_eq!(session.connect("127.0.0.1:9"), STATUS_INVALID_STATE);
        assert_eq!(session.submit(&[]), STATUS_INVALID_STATE);
        assert_eq!(session.step(0, 0, 0), STATUS_INVALID_STATE);
        assert_eq!(session.pull(PullFamily::World), Err(STATUS_INVALID_STATE));
        assert_eq!(session.close(), STATUS_OK);
    }

    #[test]
    fn bridge_lifecycle_offline_sequence_closes_idempotently() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        // Offline states: a connecting-or-idle session rejects submission and
        // stepping with the producer's invalid-state word.
        assert_eq!(session.poll(), (STATUS_OK, 1));
        assert_eq!(
            session.submit(&[77, 67, 78, 49, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]),
            STATUS_INVALID_STATE
        );
        assert_eq!(session.step(16_666_667, 64, 32), STATUS_INVALID_STATE);
        assert_eq!(session.close(), STATUS_OK);
        assert_eq!(session.close(), STATUS_OK);
        // The second close is a no-op: exactly one producer destroy ran.
        assert_eq!(core.state().destroy_calls, 1);
    }

    #[test]
    fn bridge_lifecycle_drop_after_close_and_drop_live_both_destroy_once() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        assert_eq!(session.close(), STATUS_OK);
        drop(session);
        assert_eq!(core.state().destroy_calls, 1);

        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        drop(session);
        assert_eq!(core.state().destroy_calls, 1);
    }

    #[test]
    fn bridge_lifecycle_failed_create_keeps_no_handle_and_closes_cleanly() {
        let (mut session, core) = scripted_session();
        core.state().create_status = STATUS_ABI_MISMATCH;
        assert_eq!(session.create(), STATUS_ABI_MISMATCH);
        core.state().create_status = STATUS_INPUT_REJECTED;
        assert_eq!(session.create(), STATUS_INPUT_REJECTED);
        // Close after a failed create must not destroy a handle that was never
        // issued.
        assert_eq!(session.close(), STATUS_OK);
        assert_eq!(core.state().destroy_calls, 0);
    }

    #[test]
    fn bridge_lifecycle_second_create_is_rejected_and_keeps_the_first_handle() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        let first = core.state().live[0];
        assert_eq!(session.create(), STATUS_INVALID_STATE);
        assert_eq!(core.state().create_requests.len(), 1);
        assert_eq!(core.state().live, vec![first]);
    }

    #[test]
    fn bridge_lifecycle_create_requests_the_pinned_pilot_families() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        let requests = core.state().create_requests.clone();
        assert_eq!(requests.len(), 1);
        let (major, minor, words) = &requests[0];
        assert_eq!((*major, *minor), (ABI_MAJOR, ABI_MINOR));
        let pinned: Vec<u64> = PILOT_FAMILIES
            .iter()
            .map(|descriptor| (u64::from(descriptor.version) << 32) | u64::from(descriptor.family))
            .collect();
        assert_eq!(words, &pinned);
    }

    #[test]
    fn bridge_lifecycle_destroyed_and_unknown_handles_follow_the_producer_vocabulary() {
        let core = ScriptedCore::new();
        assert_eq!(
            core.create_session(ABI_MAJOR, ABI_MINOR, &[1]),
            Ok(ClientHandle::from_word(1))
        );
        let issued = core.state().live[0];
        assert_eq!(core.destroy_session(issued), STATUS_OK);
        // Idempotent destroy of the same tombstone.
        assert_eq!(core.destroy_session(issued), STATUS_OK);
        // Every other export rejects the tombstone with invalid state.
        assert_eq!(core.connect_poll(issued), Err(STATUS_INVALID_STATE));
        assert_eq!(
            core.identity_pull(issued, &mut []),
            PullOutcome::Status(STATUS_INVALID_STATE)
        );
        // A never-issued value is invalid everywhere.
        let unknown = ClientHandle::from_word(999);
        assert_eq!(core.destroy_session(unknown), STATUS_INVALID_HANDLE);
        assert_eq!(core.connect_poll(unknown), Err(STATUS_INVALID_HANDLE));
    }

    #[test]
    fn bridge_lifecycle_post_close_calls_stay_local() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        assert_eq!(session.close(), STATUS_OK);
        let polls = core.state().poll_calls;
        let pulls = core.state().pull_calls;
        assert_eq!(session.poll(), (STATUS_INVALID_STATE, 0));
        assert_eq!(session.pull(PullFamily::World), Err(STATUS_INVALID_STATE));
        assert_eq!(session.connect("127.0.0.1:9"), STATUS_INVALID_STATE);
        assert_eq!(session.submit(&[]), STATUS_INVALID_STATE);
        assert_eq!(session.step(0, 0, 0), STATUS_INVALID_STATE);
        // No producer call ran after the bridge retired its handle.
        assert_eq!(core.state().poll_calls, polls);
        assert_eq!(core.state().pull_calls, pulls);
    }

    #[test]
    fn bridge_lifecycle_two_phase_pull_queries_then_exact_writes() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        let record: Vec<u8> = (0..192u32).map(|byte| (byte % 251) as u8).collect();
        core.script_pulls(
            &[
                PullOutcome::Capacity { required: 192 },
                PullOutcome::Complete { written: 192 },
            ],
            &record,
        );
        assert_eq!(session.pull(PullFamily::Identity), Ok(record));
        assert_eq!(core.state().pull_calls, 2);
    }

    #[test]
    fn bridge_lifecycle_pull_no_batch_returns_an_empty_owned_record() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        core.script_pulls(&[PullOutcome::Complete { written: 0 }], &[]);
        assert_eq!(session.pull(PullFamily::World), Ok(Vec::new()));
        core.script_pulls(&[PullOutcome::Complete { written: 0 }], &[]);
        assert_eq!(session.pull(PullFamily::Frame), Ok(Vec::new()));
    }

    #[test]
    fn bridge_lifecycle_pull_retries_one_capacity_signal_then_fails_closed() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        let fresh: Vec<u8> = (0..120u32).map(|byte| byte as u8).collect();
        // A batch that grew between the query and the write allows exactly one
        // retry with the fresh required size.
        core.script_pulls(
            &[
                PullOutcome::Capacity { required: 100 },
                PullOutcome::Capacity { required: 120 },
                PullOutcome::Complete { written: 120 },
            ],
            &fresh,
        );
        assert_eq!(session.pull(PullFamily::World), Ok(fresh.clone()));
        assert_eq!(core.state().pull_calls, 3);

        // A second capacity signal after the one retry is a producer
        // contradiction and fails closed with the capacity status word.
        core.script_pulls(
            &[
                PullOutcome::Capacity { required: 100 },
                PullOutcome::Capacity { required: 120 },
                PullOutcome::Capacity { required: 200 },
            ],
            &[],
        );
        assert_eq!(
            session.pull(PullFamily::World),
            Err(STATUS_INSUFFICIENT_CAPACITY)
        );

        // An error status on the write phase surfaces the raw word.
        core.script_pulls(
            &[
                PullOutcome::Capacity { required: 100 },
                PullOutcome::Status(STATUS_INTERNAL),
            ],
            &[],
        );
        assert_eq!(session.pull(PullFamily::World), Err(STATUS_INTERNAL));
    }

    #[test]
    fn bridge_lifecycle_pull_contradictory_query_outcomes_fail_closed() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        // A completed write into a zero-capacity query and a zero required
        // size both contradict the two-phase protocol; the driver fails closed
        // instead of guessing.
        core.script_pulls(&[PullOutcome::Complete { written: 40 }], &[0; 40]);
        assert_eq!(session.pull(PullFamily::World), Err(STATUS_INTERNAL));
        core.script_pulls(&[PullOutcome::Capacity { required: 0 }], &[]);
        assert_eq!(session.pull(PullFamily::World), Err(STATUS_INTERNAL));
    }

    #[test]
    fn bridge_lifecycle_pulled_records_are_owned_copies() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        let record = vec![7u8; 64];
        core.script_pulls(
            &[
                PullOutcome::Capacity { required: 64 },
                PullOutcome::Complete { written: 64 },
            ],
            &record,
        );
        let mut pulled = session.pull(PullFamily::Frame).expect("record");
        assert_eq!(pulled, record);
        // Mutating the returned buffer must not corrupt the next pull.
        pulled.iter_mut().for_each(|byte| *byte = 0);
        core.script_pulls(
            &[
                PullOutcome::Capacity { required: 64 },
                PullOutcome::Complete { written: 64 },
            ],
            &record,
        );
        assert_eq!(session.pull(PullFamily::Frame), Ok(record));
    }

    #[test]
    fn bridge_lifecycle_pull_without_a_session_reports_invalid_state() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.pull(PullFamily::Status), Err(STATUS_INVALID_STATE));
        assert_eq!(core.state().pull_calls, 0);
    }

    #[test]
    fn bridge_lifecycle_submit_and_step_pass_exact_wire_bytes() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        core.state().submit_status = STATUS_OK;
        core.state().step_status = STATUS_OK;
        let batch: Vec<u8> = (0..16u32).map(|byte| byte as u8).collect();
        assert_eq!(session.submit(&batch), STATUS_OK);
        assert_eq!(core.state().submitted_batches, vec![batch]);

        assert_eq!(session.step(16_666_667, 64, 32), STATUS_OK);
        let request = encode_step_request(16_666_667, 64, 32);
        assert_eq!(core.state().step_requests, vec![request]);
        // The frozen record: magic "MCS1", layout 1, little-endian elapsed and
        // budgets.
        assert_eq!(request[0..4], abi::MAGIC_STEP.to_le_bytes());
        assert_eq!(request[4..8], abi::STEP_VERSION.to_le_bytes());
        assert_eq!(request[8..16], 16_666_667u64.to_le_bytes());
        assert_eq!(request[16..20], 64u32.to_le_bytes());
        assert_eq!(request[20..24], 32u32.to_le_bytes());
    }

    #[test]
    fn bridge_lifecycle_connect_passes_address_bytes_and_requires_a_session() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.connect("127.0.0.1:9"), STATUS_INVALID_STATE);
        assert!(core.state().connect_addresses.is_empty());
        assert_eq!(session.create(), STATUS_OK);
        assert_eq!(session.connect("127.0.0.1:9"), STATUS_OK);
        assert_eq!(
            core.state().connect_addresses,
            vec![b"127.0.0.1:9".to_vec()]
        );
        // A producer refusal (for example a second begin on a begun session)
        // surfaces unchanged.
        core.state().connect_status = STATUS_INVALID_STATE;
        assert_eq!(session.connect("127.0.0.1:10"), STATUS_INVALID_STATE);
    }

    #[test]
    fn bridge_lifecycle_every_status_word_maps_without_panicking() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        let words = [
            STATUS_OK,
            1,
            STATUS_ABI_MISMATCH,
            STATUS_INPUT_REJECTED,
            STATUS_INSUFFICIENT_CAPACITY,
            5,
            STATUS_INVALID_STATE,
            7,
            STATUS_INTERNAL,
            9,
            10,
            11,
            100,
            u32::MAX,
        ];
        for word in words {
            core.state().submit_status = word;
            core.state().step_status = word;
            core.state().connect_status = word;
            core.state().poll_result = Err(word);
            core.script_pulls(
                &[
                    PullOutcome::Capacity { required: 8 },
                    PullOutcome::Status(word),
                ],
                &[],
            );
            assert_eq!(session.submit(&[]), word, "submit word {word}");
            assert_eq!(session.step(1, 1, 1), word, "step word {word}");
            assert_eq!(session.connect("127.0.0.1:9"), word, "connect word {word}");
            assert_eq!(session.poll(), (word, 0), "poll word {word}");
            assert_eq!(
                session.pull(PullFamily::Status),
                Err(word),
                "pull word {word}"
            );
        }
    }

    #[test]
    fn bridge_lifecycle_step_request_encoder_pins_the_wire_record() {
        let request = encode_step_request(u64::MAX, 4096, 4096);
        assert_eq!(
            request,
            [
                0x4D, 0x43, 0x53, 0x31, 0x01, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
                0xFF, 0xFF, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00,
            ]
        );
        assert_eq!(encode_step_request(0, 0, 0)[0..4], [0x4D, 0x43, 0x53, 0x31]);
    }

    #[test]
    fn bridge_lifecycle_pull_buffers_reuse_one_backing_across_session_pulls() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        let record: Vec<u8> = (0..192u32).map(|byte| (byte % 251) as u8).collect();
        for _ in 0..3 {
            core.script_pulls(
                &[
                    PullOutcome::Capacity { required: 192 },
                    PullOutcome::Complete { written: 192 },
                ],
                &record,
            );
            // The Python-facing value stays an owned copy: mutating it cannot
            // corrupt the next pull even though the FFI-side backing is
            // shared across pulls.
            let mut pulled = session.pull(PullFamily::Identity).expect("record");
            assert_eq!(pulled, record);
            pulled.iter_mut().for_each(|byte| *byte = 0);
        }
        // Three two-phase pulls made six producer calls, and the three exact
        // writes landed at one stable FFI-side backing address: no per-pull
        // buffer allocation in the steady state.
        let views = core.state().pull_views.clone();
        assert_eq!(views.len(), 6);
        let writes: Vec<(usize, usize)> = views
            .iter()
            .copied()
            .filter(|(_, length)| *length > 0)
            .collect();
        assert_eq!(writes.len(), 3);
        assert!(
            writes.iter().all(|view| view.0 == writes[0].0),
            "the session reuses one FFI backing address"
        );
    }

    #[test]
    fn bridge_lifecycle_pull_buffers_reset_identities_on_session_recreate() {
        let (mut session, core) = scripted_session();
        assert_eq!(session.create(), STATUS_OK);
        let first = crate::pull_buffers::test_world_record(9, 1, 1, 0);
        core.script_pulls(
            &[
                PullOutcome::Capacity {
                    required: first.len() as u32,
                },
                PullOutcome::Complete {
                    written: first.len() as u32,
                },
            ],
            &first,
        );
        assert_eq!(session.pull(PullFamily::World), Ok(first));

        // Close and create again: wire epochs are per producer session, so a
        // fresh session's epoch space legitimately restarts below the old
        // one and must not be refused as stale.
        assert_eq!(session.close(), STATUS_OK);
        assert_eq!(session.create(), STATUS_OK);
        let second = crate::pull_buffers::test_world_record(1, 1, 1, 0);
        core.script_pulls(
            &[
                PullOutcome::Capacity {
                    required: second.len() as u32,
                },
                PullOutcome::Complete {
                    written: second.len() as u32,
                },
            ],
            &second,
        );
        assert_eq!(session.pull(PullFamily::World), Ok(second));
    }
}
