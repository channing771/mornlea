//! Bounded CPU mesh-prepare worker for the pilot Godot terrain path.
//!
//! Design decision 5 of the Godot client migration: after a valid
//! section-mesh batch arrives, the GDExtension performs bounded expansion
//! of packed quads, and that expansion must not consume unbounded
//! Godot-main-thread time. This module owns the prepare half of that
//! ruling: a single background `std::thread` turns one copied per-section
//! packed payload into owned per-surface vertex/index arrays through
//! `quad_decode`, and the Godot main thread later submits prepared arrays
//! under the terrain upload budget owned by the later budget work. Pure
//! data in, pure data out: no unsafe code, no Godot types, no bridge, no
//! RIDs, no clock, and no I/O — Godot objects are main-thread-only by
//! crate rule, so nothing here may touch them.
//!
//! Threading model: exactly one worker thread. Expansion of one section
//! is bounded by the frozen per-section quad limit, and a single worker
//! keeps both queues FIFO, which is what makes submit order,
//! cancellation, and epoch discard decidable; a worker pool would add
//! ordering and lifetime complexity the pilot's correctness-first
//! terrain goal does not need.
//!
//! Ownership boundary: `try_submit` takes the packed payload by value.
//! `Queued` transfers the bytes to the worker, mirroring the project-wide
//! rule that a message and its slices are immutable after a successful
//! cross-thread send — the move enforces in the type system what the Go
//! rule documents, because the caller is left with nothing to mutate.
//! `Full` and `Closed` hand the payload back untouched, so a rejected
//! submit never drops work silently. Every surfaced result owns its
//! vertex and index arrays outright; nothing aliases worker-internal
//! state.
//!
//! Queue bounds: both queues are accounted in packed-quad bytes and
//! pinned to the frozen maximal world batch — `abi::MAX_WORLD_BATCH_QUADS`
//! eight-byte quads, exactly 4 MiB. The pending queue therefore holds at
//! most one maximal batch of packed input (the largest all-or-nothing
//! unit the frozen presentation contract can publish), and the completed
//! queue at most one maximal batch of expanded output, so retention is
//! tight against the contract instead of an arbitrary number. Every job
//! and result costs at least one quad's bytes, so degenerate streams —
//! for example repeated empty payloads, which decode rejects — cannot
//! occupy unbounded queue slots at zero cost.
//!
//! Backpressure: submission and consumption are both try-style and never
//! block (`Submit::Full` and `Take::Empty` are decidable answers; the
//! vocabulary mirrors the Go client's bounded mesh queues). When the
//! completed queue is at capacity the worker parks on the shared
//! condvar — backpressure lands on the worker thread, never on the
//! Godot main thread — and draining a result or closing the worker
//! unblocks it.
//!
//! Epoch rule: each job carries the world-family session epoch of the
//! `MCW1` batch it came from. A newer epoch on submit becomes current and
//! sweeps still-queued older-epoch jobs (a new session epoch invalidates
//! prior state). A result is surfaced only while its epoch is still
//! current and above every epoch retired by `cancel_pending`; a job
//! already in flight always runs its decode to completion, but its result
//! is discarded under the same rule. Submitting for an older epoch is
//! accepted and discarded the same way: the worker is a pipeline, not an
//! epoch validator — the producer's epochs are monotonic and the bridge
//! owns that contract.
//!
//! Panic isolation: the decode pipeline runs under `catch_unwind`, the
//! same boundary conversion the lifecycle module uses. A panicking job
//! surfaces as that job's stable `abi::STATUS_PANIC` error result, the
//! worker keeps serving, and no shared mutex can be poisoned because no
//! caller code ever runs under a lock (decodes happen lock-free on the
//! worker thread).
//!
//! Close contract: `close` is idempotent. The first call marks the state
//! closed, discards all pending jobs and completed results (close is
//! session teardown — the upload path stops caring about prepared
//! geometry), unblocks every worker wait, and joins the thread, so it may
//! wait for at most the in-flight section's bounded decode. After close,
//! every API answers its stable closed status (`Submit::Closed`,
//! `Take::Closed`, and `abi::STATUS_INVALID_STATE` from `cancel_pending`).
//! `Drop` performs the same close, so forgetting explicit teardown cannot
//! leak the thread or deadlock: the state lock is always released before
//! the join, and the closed flag plus notification unblock every wait.

// This module is deliberately ahead of its non-test consumers: the bridge
// session that submits pulled world batches and the frame upload path land
// with the later terrain tasks, and until then only the tests below
// reference the service surface. Allow `dead_code` module-wide so the
// worker does not fail `cargo clippy --all-targets -- -D warnings` before
// those consumers exist; remove this allowance once production code owns
// a worker instance.
#![allow(dead_code)]

use std::collections::VecDeque;
use std::panic::{AssertUnwindSafe, catch_unwind};
use std::sync::{Arc, Condvar, Mutex, MutexGuard};
use std::thread::{Builder, JoinHandle};

use crate::abi;
use crate::quad_decode::{self, SectionGeometry};

/// Pending-queue retention bound in packed-quad bytes: exactly the frozen
/// maximal world batch ([`abi::MAX_WORLD_BATCH_QUADS`] eight-byte quads,
/// 4 MiB). The producer publishes world state as all-or-nothing batches,
/// so one maximal batch is the largest pending set the frozen contract
/// can ever legitimately hand the worker; holding less would reject
/// submits of a legal batch, and holding more would be retention without
/// a producer-side counterpart.
pub(crate) const PENDING_BOUND_BYTES: usize =
    abi::MAX_WORLD_BATCH_QUADS as usize * quad_decode::QUAD_BYTES;

/// Completed-queue retention bound in packed-quad bytes, pinned to the
/// same frozen maximal batch. The consumer drains under the terrain
/// upload budget, so one maximal batch of prepared-but-unconsumed output
/// is the retention ceiling; beyond it the worker parks instead of
/// expanding more (backpressure), which bounds steady-state memory by the
/// main thread's drain rate.
pub(crate) const COMPLETED_BOUND_BYTES: usize =
    abi::MAX_WORLD_BATCH_QUADS as usize * quad_decode::QUAD_BYTES;

/// Retention cost of one job or result, in packed-quad bytes. Every entry
/// costs at least one quad's bytes so degenerate streams (empty payloads,
/// which decode fails closed on) cannot occupy unbounded queue slots at
/// zero cost.
fn cost_bytes(packed_quads: usize) -> usize {
    packed_quads.max(1) * quad_decode::QUAD_BYTES
}

/// Identity of one prepared section: the world-family record vocabulary
/// of `core.SectionKey` plus the presentation revision — dimension,
/// signed section X/Y/Z, revision — matching the `MCW1` operation record
/// and `presentation.SectionMeshUpsert`. The worker treats the identity
/// as opaque producer-validated data; section-domain rules stay owned by
/// the producer.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) struct SectionId {
    pub(crate) dimension: u32,
    pub(crate) x: i32,
    pub(crate) y: i32,
    pub(crate) z: i32,
    pub(crate) revision: u64,
}

/// One queued prepare job: the session epoch of the world batch the
/// upsert arrived in, the target section identity, and the owned packed
/// payload of that one section upsert.
struct MeshJob {
    epoch: u64,
    id: SectionId,
    packed: Vec<u64>,
}

/// One completed prepare result. `outcome` owns every vertex and index
/// array outright on success and carries the stable `abi` failure word
/// otherwise; nothing aliases worker-internal state. `input_quads` is the
/// packed-quad count the result was prepared from: the pilot report's
/// packed-input accounting reads it, and the completed queue charges the
/// same retention cost the job paid on submit.
#[derive(Debug)]
pub(crate) struct PreparedSection {
    pub(crate) epoch: u64,
    pub(crate) id: SectionId,
    pub(crate) input_quads: usize,
    pub(crate) outcome: Result<SectionGeometry, u32>,
}

/// The try-style answer of [`MeshWorker::try_submit`]. Submission never
/// blocks and never drops work silently: the payload's ownership moves to
/// the worker only on `Queued`, and every rejecting variant hands the
/// bytes back untouched.
#[derive(Debug)]
pub(crate) enum Submit {
    /// The job was queued; the worker now owns the payload bytes.
    Queued,
    /// The pending queue is at its packed-byte bound; the payload is
    /// returned so the caller can retry without re-copying.
    Full { packed: Vec<u64> },
    /// The worker is closed; the payload is returned untouched.
    Closed { packed: Vec<u64> },
}

/// The try-style answer of [`MeshWorker::try_take`]. The consumer side of
/// the completed queue never blocks: the Godot main thread polls once per
/// frame under the terrain upload budget instead of waiting on the
/// worker.
#[derive(Debug)]
pub(crate) enum Take {
    /// One completed result, in submit order.
    Output(PreparedSection),
    /// No completed result is ready right now.
    Empty,
    /// The worker is closed; no result will ever follow.
    Closed,
}

/// The decode pipeline the worker runs per job. A boxed closure (not a
/// fn pointer) so tests can close over deterministic coordination state;
/// production always installs [`production_decode`].
type DecodeFn = Box<dyn Fn(&[u64]) -> Result<SectionGeometry, u32> + Send>;

/// Production decode pipeline: the frozen batch guard first, then the
/// combined-payload decode. One section upsert is exactly one world-batch
/// operation, so the guard's operation count is the literal `1` and the
/// payload length is that operation's packed-quad total; this rejects a
/// whole-batch-sized payload before any per-section decode spends work.
fn production_decode(packed: &[u64]) -> Result<SectionGeometry, u32> {
    quad_decode::check_world_batch(1, packed.len())?;
    quad_decode::decode_pilot_section(packed)
}

/// All mutable worker state, under one mutex so every cross-thread
/// transition is a single consistent step. `current_epoch` is the newest
/// session epoch ever submitted; `retired_through` is the highest epoch
/// explicitly cancelled; a result surfaces only while its epoch equals
/// `current_epoch` and sits above `retired_through`.
struct WorkerState {
    closed: bool,
    current_epoch: Option<u64>,
    retired_through: Option<u64>,
    pending: VecDeque<MeshJob>,
    pending_bytes: usize,
    completed: VecDeque<PreparedSection>,
    completed_bytes: usize,
    /// How many times the worker entered the results-capacity wait; the
    /// deterministic observable the backpressure tests poll for.
    result_waits: usize,
}

/// The shared mutex plus the single condvar every worker wait uses (an
/// empty pending queue, a full completed queue). Each mutator notifies
/// after changing any wait predicate, and each wait re-checks its own
/// loop condition, so one signal domain cannot miss a wakeup.
struct Shared {
    state: Mutex<WorkerState>,
    signal: Condvar,
}

/// Bounded single-worker CPU mesh-prepare service; see the module
/// documentation for the threading, bound, epoch, panic, and close
/// contracts.
pub(crate) struct MeshWorker {
    shared: Arc<Shared>,
    /// The worker join handle, taken exactly once by `close` (or `Drop`)
    /// so both are idempotent, and never held while the state lock is
    /// taken.
    thread: Mutex<Option<JoinHandle<()>>>,
}

impl MeshWorker {
    /// Start the prepare worker with the production decode pipeline.
    pub(crate) fn new() -> Result<Self, u32> {
        Self::start(Box::new(production_decode))
    }

    /// Test seam: start a worker whose decode pipeline is injected, in the
    /// Go producer's `worldBatchEncode` style — production always runs
    /// [`production_decode`]; tests substitute gated or panicking
    /// pipelines to control in-flight timing deterministically.
    #[cfg(test)]
    pub(crate) fn with_decode(decode: DecodeFn) -> Result<Self, u32> {
        Self::start(decode)
    }

    fn start(decode: DecodeFn) -> Result<Self, u32> {
        let shared = Arc::new(Shared {
            state: Mutex::new(WorkerState {
                closed: false,
                current_epoch: None,
                retired_through: None,
                pending: VecDeque::new(),
                pending_bytes: 0,
                completed: VecDeque::new(),
                completed_bytes: 0,
                result_waits: 0,
            }),
            signal: Condvar::new(),
        });
        let worker_shared = Arc::clone(&shared);
        let spawned = Builder::new()
            .name("mornlea-mesh-prepare".into())
            .spawn(move || serve(worker_shared, decode));
        // Thread spawn failure is an OS resource failure with no narrower
        // stable classification; the internal word is the honest answer.
        let thread = spawned.map_err(|_| abi::STATUS_INTERNAL)?;
        Ok(Self {
            shared,
            thread: Mutex::new(Some(thread)),
        })
    }

    /// Submit one section upsert for preparation. Try-style by contract:
    /// the answer is decidable in bounded time (a new-epoch submit sweeps
    /// the pending queue once) and the call never blocks, so the Godot
    /// main thread pays no queue wait.
    ///
    /// Ownership boundary: `packed` is the copied packed-quad payload of
    /// the one section upsert, passed by value. After `Queued` the bytes
    /// belong to the worker — mirroring the project-wide rule that a
    /// message and its slices are immutable after a successful
    /// cross-thread send; the move makes that rule unbreakable in the
    /// type system because the caller is left with nothing to mutate.
    /// `Full` and `Closed` return the payload untouched, so a rejected
    /// submit never drops work silently.
    ///
    /// Epoch handling: a session epoch newer than any seen before becomes
    /// current and sweeps still-queued older-epoch jobs (a new session
    /// epoch invalidates prior state). A submit for an older epoch is
    /// accepted — the worker is a pipeline, not an epoch validator — and
    /// its result is discarded at completion like any superseded work.
    pub(crate) fn try_submit(&self, epoch: u64, id: SectionId, packed: Vec<u64>) -> Submit {
        let cost = cost_bytes(packed.len());
        let mut state = lock_shared(&self.shared);
        if state.closed {
            return Submit::Closed { packed };
        }
        if state.current_epoch.is_none_or(|current| epoch > current) {
            state.current_epoch = Some(epoch);
            let WorkerState {
                pending,
                pending_bytes,
                ..
            } = &mut *state;
            pending.retain(|job| {
                if job.epoch < epoch {
                    *pending_bytes -= cost_bytes(job.packed.len());
                    false
                } else {
                    true
                }
            });
        }
        if state.pending_bytes + cost > PENDING_BOUND_BYTES {
            return Submit::Full { packed };
        }
        state.pending_bytes += cost;
        state.pending.push_back(MeshJob { epoch, id, packed });
        drop(state);
        self.shared.signal.notify_all();
        Submit::Queued
    }

    /// Discard every queued job of `epoch` and retire that epoch; the
    /// answer is the count of discarded queued jobs. A job of the epoch
    /// already in flight is not preempted — its decode always runs to
    /// completion — but the result is discarded at publication because a
    /// retired epoch's results are never surfaced. Retiring stays in
    /// force: later submits for the same epoch complete and are discarded
    /// the same way until a newer session epoch becomes current.
    pub(crate) fn cancel_pending(&self, epoch: u64) -> Result<usize, u32> {
        let mut state = lock_shared(&self.shared);
        if state.closed {
            return Err(abi::STATUS_INVALID_STATE);
        }
        state.retired_through = state.retired_through.max(Some(epoch));
        let mut discarded = 0;
        let WorkerState {
            pending,
            pending_bytes,
            ..
        } = &mut *state;
        pending.retain(|job| {
            if job.epoch == epoch {
                discarded += 1;
                *pending_bytes -= cost_bytes(job.packed.len());
                false
            } else {
                true
            }
        });
        drop(state);
        self.shared.signal.notify_all();
        Ok(discarded)
    }

    /// Take one completed result. Never blocks: `Empty` means no result is
    /// ready right now (the frame loop polls again next frame under the
    /// terrain upload budget) and `Closed` is the stable answer after
    /// `close`. Taking a result frees completed-queue capacity and wakes
    /// the worker if it was parked on that capacity.
    pub(crate) fn try_take(&self) -> Take {
        let mut state = lock_shared(&self.shared);
        if state.closed {
            return Take::Closed;
        }
        match state.completed.pop_front() {
            Some(prepared) => {
                state.completed_bytes -= cost_bytes(prepared.input_quads);
                drop(state);
                self.shared.signal.notify_all();
                Take::Output(prepared)
            }
            None => Take::Empty,
        }
    }

    /// Close the worker. Idempotent: the first call marks the state
    /// closed, discards all pending jobs and completed results (close is
    /// session teardown — the upload path stops caring about prepared
    /// geometry), unblocks every worker wait, and joins the thread, so it
    /// may block for at most the in-flight section's bounded decode.
    /// Later calls find the join handle already taken and return
    /// immediately. After `close`, every API answers its stable closed
    /// status.
    pub(crate) fn close(&self) {
        {
            let mut state = lock_shared(&self.shared);
            state.closed = true;
            state.pending.clear();
            state.pending_bytes = 0;
            state.completed.clear();
            state.completed_bytes = 0;
            // Wake every parked worker wait: both wait loops re-check the
            // closed flag first and exit without touching the cleared
            // queues.
            self.shared.signal.notify_all();
        }
        // Never join while holding the state lock: the worker must take
        // that lock to observe `closed` and exit.
        let handle = self
            .thread
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .take();
        if let Some(handle) = handle {
            // The worker converts its own decode panics into per-job
            // results, so an errored join has no recoverable meaning: the
            // thread is gone either way and the closed state above is
            // already final.
            let _ = handle.join();
        }
    }

    /// Test-only snapshot of the queue state, so tests can poll for
    /// structural facts (a full completed queue, an empty pending queue)
    /// instead of timing.
    #[cfg(test)]
    pub(crate) fn facts(&self) -> WorkerFacts {
        let state = lock_shared(&self.shared);
        WorkerFacts {
            pending_jobs: state.pending.len(),
            pending_bytes: state.pending_bytes,
            completed_jobs: state.completed.len(),
            completed_bytes: state.completed_bytes,
            result_waits: state.result_waits,
        }
    }
}

/// The test-only observable of [`MeshWorker::facts`].
#[cfg(test)]
#[derive(Debug)]
pub(crate) struct WorkerFacts {
    pub(crate) pending_jobs: usize,
    pub(crate) pending_bytes: usize,
    pub(crate) completed_jobs: usize,
    pub(crate) completed_bytes: usize,
    pub(crate) result_waits: usize,
}

impl Drop for MeshWorker {
    /// Drop is `close`: an owner that forgets explicit teardown still gets
    /// a deterministic thread join, bounded by the in-flight decode, and
    /// cannot deadlock — the state lock is released before the join, and
    /// the closed flag plus notification unblock every worker wait.
    fn drop(&mut self) {
        self.close();
    }
}

/// The worker thread body: take one job, decode it under panic isolation,
/// publish or discard the result, repeat until closed. The single thread
/// keeps both queues FIFO, which is what makes submit order,
/// cancellation, and epoch-discard semantics decidable.
fn serve(shared: Arc<Shared>, decode: DecodeFn) {
    loop {
        // Acquire the next job, parking while the pending queue is empty.
        let job = {
            let mut state = lock_shared(&shared);
            loop {
                if state.closed {
                    return;
                }
                if let Some(job) = state.pending.pop_front() {
                    state.pending_bytes -= cost_bytes(job.packed.len());
                    break job;
                }
                state = wait_while(&shared, state);
            }
        };
        // Decode outside every lock: callers never wait on worker math,
        // and no caller code runs under a mutex, so a panic here cannot
        // poison shared state. The panic becomes this job's stable
        // `STATUS_PANIC` result and the worker keeps serving, mirroring
        // the lifecycle module's boundary conversion.
        let outcome = match catch_unwind(AssertUnwindSafe(|| decode(&job.packed))) {
            Ok(decoded) => decoded,
            Err(_) => Err(abi::STATUS_PANIC),
        };
        let mut state = lock_shared(&shared);
        if state.closed {
            return;
        }
        // The epoch filter — the single place old work is dropped: a
        // superseded epoch (a newer session epoch became current) and a
        // retired epoch (cancel_pending) both complete their decode but
        // never surface.
        let surfaced = state
            .current_epoch
            .is_some_and(|current| current == job.epoch)
            && state
                .retired_through
                .is_none_or(|retired| job.epoch > retired);
        if !surfaced {
            continue;
        }
        let cost = cost_bytes(job.packed.len());
        loop {
            if state.closed {
                return;
            }
            if state.completed_bytes + cost <= COMPLETED_BOUND_BYTES {
                state.completed_bytes += cost;
                state.completed.push_back(PreparedSection {
                    epoch: job.epoch,
                    id: job.id,
                    input_quads: job.packed.len(),
                    outcome,
                });
                break;
            }
            // Backpressure: the completed queue is at its bound, so the
            // worker parks until the consumer drains a result (or close
            // unblocks it); the main thread never pays this wait.
            state.result_waits += 1;
            state = wait_while(&shared, state);
        }
    }
}

/// Lock the shared state, recovering from poisoning. No caller code runs
/// under this mutex — decodes happen lock-free on the worker thread — so
/// poisoning is impossible by construction and recovery is a documented
/// invariant, not a reachable path.
fn lock_shared(shared: &Shared) -> MutexGuard<'_, WorkerState> {
    shared
        .state
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

/// Re-park on the shared condvar, recovering the guard from poisoning the
/// same way [`lock_shared`] does.
fn wait_while<'a>(
    shared: &'a Shared,
    guard: MutexGuard<'a, WorkerState>,
) -> MutexGuard<'a, WorkerState> {
    shared
        .signal
        .wait(guard)
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

#[cfg(test)]
mod tests {
    use super::{
        COMPLETED_BOUND_BYTES, MeshWorker, PENDING_BOUND_BYTES, PreparedSection, SectionId, Submit,
        Take, cost_bytes,
    };
    use crate::abi;
    use crate::quad_decode::{
        self, CUTOUT_MATERIAL_LEAVES, PLANT_MATERIAL_SHORT_GRASS, SectionGeometry, TestQuad,
        WATER_MATERIAL,
    };
    use std::sync::atomic::{AtomicBool, Ordering};
    use std::sync::{Arc, Condvar, Mutex};
    use std::thread;
    use std::time::{Duration, Instant};

    /// Generous but finite bound for every poll in this suite; a correct
    /// worker settles in milliseconds, so hitting this deadline is a bug
    /// report, not a race.
    const TEST_TIMEOUT: Duration = Duration::from_secs(20);
    const POLL_STEP: Duration = Duration::from_millis(1);

    /// Sentinel word the panicking decode seam aborts on; no fixture word
    /// equals it because the seam panics before decode runs.
    const POISON_WORD: u64 = 0x0DEA_D000_0000_0001;

    /// Bound-filling tests retain one maximal batch of expanded geometry
    /// (roughly a hundred mebibytes); serialize them so parallel test
    /// threads do not multiply that peak. Poisoning recovery keeps one
    /// failing heavy test from masking the real failure in the next.
    static HEAVY: Mutex<()> = Mutex::new(());

    fn heavy_ticket() -> std::sync::MutexGuard<'static, ()> {
        HEAVY
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
    }

    /// Deterministic worker-thread coordination: a decode seam parks
    /// inside the gate until the test opens it, so tests control exactly
    /// when the in-flight job completes without sleep-based races.
    /// `entered` and `finished` make the park and its release observable.
    #[derive(Clone, Default)]
    struct Gate {
        inner: Arc<GateInner>,
    }

    #[derive(Default)]
    struct GateInner {
        open: Mutex<bool>,
        entered: AtomicBool,
        finished: AtomicBool,
        changed: Condvar,
    }

    impl Gate {
        /// The gated decode: record entry, park until opened (bounded, so
        /// a broken test fails instead of hanging), then decode for real.
        fn decode(&self, packed: &[u64]) -> Result<SectionGeometry, u32> {
            self.inner.entered.store(true, Ordering::SeqCst);
            let mut open = self.inner.open.lock().expect("gate lock");
            let deadline = Instant::now() + TEST_TIMEOUT;
            while !*open {
                let remaining = deadline.saturating_duration_since(Instant::now());
                let (guard, _timed_out) = self
                    .inner
                    .changed
                    .wait_timeout(open, remaining)
                    .expect("gate lock");
                open = guard;
                if !*open && Instant::now() >= deadline {
                    // A broken flow parked forever; proceed so the missing
                    // assertions fail with a message instead of a hang.
                    break;
                }
            }
            drop(open);
            self.inner.finished.store(true, Ordering::SeqCst);
            quad_decode::decode_pilot_section(packed)
        }

        fn open(&self) {
            let mut open = self.inner.open.lock().expect("gate lock");
            *open = true;
            self.inner.changed.notify_all();
        }

        fn entered(&self) -> bool {
            self.inner.entered.load(Ordering::SeqCst)
        }

        fn finished(&self) -> bool {
            self.inner.finished.load(Ordering::SeqCst)
        }
    }

    fn gated_worker() -> (MeshWorker, Gate) {
        let gate = Gate::default();
        let decode_gate = gate.clone();
        let worker = MeshWorker::with_decode(Box::new(move |packed| decode_gate.decode(packed)))
            .expect("gated worker starts");
        (worker, gate)
    }

    fn panicking_worker() -> MeshWorker {
        MeshWorker::with_decode(Box::new(|packed: &[u64]| {
            if packed.contains(&POISON_WORD) {
                panic!("mesh_worker panic-isolation probe");
            }
            quad_decode::decode_pilot_section(packed)
        }))
        .expect("panicking worker starts")
    }

    /// Fixture quads shared with the decode tests: the bit-layout
    /// knowledge lives once in `TestQuad::pack`.
    fn stone_quad() -> u64 {
        TestQuad::default().pack()
    }

    fn leaves_quad() -> u64 {
        TestQuad {
            material: CUTOUT_MATERIAL_LEAVES,
            w: 2,
            h: 1,
            ..TestQuad::default()
        }
        .pack()
    }

    fn water_quad() -> u64 {
        TestQuad {
            material: WATER_MATERIAL,
            corners: [8, 9, 10, 11],
            ..TestQuad::default()
        }
        .pack()
    }

    fn grass_quad() -> u64 {
        TestQuad {
            face: 6,
            material: PLANT_MATERIAL_SHORT_GRASS,
            ..TestQuad::default()
        }
        .pack()
    }

    fn section(x: i32, revision: u64) -> SectionId {
        SectionId {
            dimension: if x < 0 { 1 } else { 0 },
            x,
            y: -1,
            z: 2,
            revision,
        }
    }

    /// Poll `try_take` until `expected` results arrived, failing with a
    /// clear message at the deadline instead of hanging.
    fn drain(worker: &MeshWorker, expected: usize) -> Vec<PreparedSection> {
        let deadline = Instant::now() + TEST_TIMEOUT;
        let mut collected = Vec::new();
        while collected.len() < expected {
            match worker.try_take() {
                Take::Output(prepared) => collected.push(prepared),
                Take::Empty => {
                    assert!(
                        Instant::now() < deadline,
                        "mesh_worker drained {}/{} results before the deadline",
                        collected.len(),
                        expected
                    );
                    thread::sleep(POLL_STEP);
                }
                Take::Closed => panic!("mesh_worker closed with {expected} results pending"),
            }
        }
        collected
    }

    /// Poll a structural predicate until it holds, with a finite deadline
    /// and a named failure message.
    fn poll_until(probe: impl Fn() -> bool, what: &str) {
        let deadline = Instant::now() + TEST_TIMEOUT;
        while !probe() {
            assert!(
                Instant::now() < deadline,
                "mesh_worker timed out waiting for {what}"
            );
            thread::sleep(POLL_STEP);
        }
    }

    /// Fill both queues to exactly the frozen bound: 21 maximal sections
    /// plus one 8192-quad section is 524,288 packed quads — one maximal
    /// world batch. Waits until the worker parked every result in the
    /// completed queue and drained the pending queue, so the caller
    /// observes a deterministically full completed queue. Returns the
    /// submitted ids in order.
    fn fill_to_bound(worker: &MeshWorker) -> Vec<SectionId> {
        let maximal = abi::MAX_SECTION_MESH_QUADS as usize;
        let mut ids = Vec::with_capacity(22);
        for index in 0..21i32 {
            let id = section(index, u64::try_from(index).expect("index fits") + 1);
            let packed = vec![stone_quad(); maximal];
            assert!(
                matches!(worker.try_submit(9, id, packed), Submit::Queued),
                "maximal section {index} must fit the pending bound"
            );
            ids.push(id);
        }
        let tail = section(21, 22);
        let packed = vec![stone_quad(); 8192];
        assert!(matches!(worker.try_submit(9, tail, packed), Submit::Queued));
        ids.push(tail);
        poll_until(
            || {
                let facts = worker.facts();
                facts.pending_jobs == 0 && facts.completed_bytes == COMPLETED_BOUND_BYTES
            },
            "the completed queue to reach the frozen bound",
        );
        ids
    }

    #[test]
    fn mesh_worker_pins_queue_bounds_to_the_frozen_batch_limits() {
        // Both bounds derive from the frozen maximal world batch: the
        // presentation contract's 4 MiB packed payload ceiling.
        assert_eq!(abi::MAX_WORLD_BATCH_QUADS, 524_288);
        assert_eq!(PENDING_BOUND_BYTES, 4 * 1024 * 1024);
        assert_eq!(COMPLETED_BOUND_BYTES, PENDING_BOUND_BYTES);
        // Accounting is in packed-quad bytes and every entry costs at
        // least one quad, so degenerate streams cannot ride for free.
        assert_eq!(cost_bytes(0), quad_decode::QUAD_BYTES);
        assert_eq!(cost_bytes(7), 7 * quad_decode::QUAD_BYTES);
        assert_eq!(
            cost_bytes(abi::MAX_WORLD_BATCH_QUADS as usize),
            PENDING_BOUND_BYTES
        );
    }

    #[test]
    fn mesh_worker_prepares_sections_end_to_end_in_submit_order() {
        let worker = MeshWorker::new().expect("worker starts");
        let first = vec![stone_quad(), leaves_quad()];
        let second = vec![leaves_quad(), water_quad()];
        let third = vec![grass_quad()];
        for (index, packed) in [first, second, third].into_iter().enumerate() {
            assert!(
                matches!(
                    worker.try_submit(2, section(index as i32, 7), packed),
                    Submit::Queued
                ),
                "section {index} must queue"
            );
        }
        let collected = drain(&worker, 3);
        for (index, prepared) in collected.iter().enumerate() {
            assert_eq!(prepared.id, section(index as i32, 7), "FIFO order");
            assert_eq!(prepared.epoch, 2);
        }
        assert_eq!(collected[0].input_quads, 2);
        assert_eq!(collected[1].input_quads, 2);
        assert_eq!(collected[2].input_quads, 1);

        let one = collected[0].outcome.as_ref().expect("stone+leaves decode");
        assert_eq!(one.opaque.vertices.len(), 4);
        assert_eq!(one.opaque.indices.len(), 6);
        assert_eq!(one.cutout.vertices.len(), 4);
        assert!(one.water.vertices.is_empty());

        let two = collected[1].outcome.as_ref().expect("leaves+water decode");
        assert_eq!(two.cutout.vertices.len(), 4);
        assert_eq!(two.water.vertices.len(), 4);
        assert_eq!(two.water.vertices[0].layer, WATER_MATERIAL);
        assert!(two.opaque.vertices.is_empty());

        let three = collected[2].outcome.as_ref().expect("grass decode");
        assert_eq!(three.cutout.vertices.len(), 4);
        assert!(three.opaque.vertices.is_empty() && three.water.vertices.is_empty());
        worker.close();
    }

    #[test]
    fn mesh_worker_full_pending_queue_answers_full_and_never_drops() {
        let _heavy = heavy_ticket();
        let (worker, gate) = gated_worker();
        // Park the worker on the first job so the pending queue fills
        // deterministically while nothing is decoded.
        let maximal = abi::MAX_SECTION_MESH_QUADS as usize;
        assert!(matches!(
            worker.try_submit(4, section(0, 1), vec![stone_quad(); maximal]),
            Submit::Queued
        ));
        poll_until(
            || gate.entered(),
            "the gated worker to pick up the first job",
        );

        // 21 maximal sections plus one 8192-quad section fill the pending
        // queue to exactly the frozen bound.
        let mut expected = vec![section(0, 1)];
        for index in 1..=21i32 {
            let id = section(index, u64::try_from(index).expect("index fits"));
            assert!(matches!(
                worker.try_submit(4, id, vec![stone_quad(); maximal]),
                Submit::Queued
            ));
            expected.push(id);
        }
        let tail = section(22, 23);
        assert!(matches!(
            worker.try_submit(4, tail, vec![stone_quad(); 8192]),
            Submit::Queued
        ));
        expected.push(tail);

        // One more quad does not fit: Full, with the payload handed back
        // untouched so the caller can retry without re-copying.
        let rejected_id = section(23, 9);
        match worker.try_submit(4, rejected_id, vec![grass_quad()]) {
            Submit::Full { packed } => assert_eq!(packed, vec![grass_quad()]),
            other => panic!("expected Submit::Full, got {other:?}"),
        }
        let facts = worker.facts();
        assert_eq!(facts.pending_bytes, PENDING_BOUND_BYTES);
        assert_eq!(facts.pending_jobs, 22);

        // No silent drop: opening the gate surfaces every accepted job in
        // submit order, and the rejected payload retries successfully
        // once capacity frees.
        gate.open();
        let collected = drain(&worker, expected.len());
        for (prepared, want) in collected.iter().zip(&expected) {
            assert_eq!(&prepared.id, want, "accepted jobs surface in order");
            assert_eq!(prepared.epoch, 4);
        }
        let geometry = collected[0]
            .outcome
            .as_ref()
            .expect("the parked maximal section decodes");
        assert_eq!(geometry.opaque.vertices.len(), maximal * 4);
        assert!(matches!(
            worker.try_submit(4, rejected_id, vec![grass_quad()]),
            Submit::Queued
        ));
        let tail_result = drain(&worker, 1);
        assert_eq!(tail_result[0].id, rejected_id);
        worker.close();
    }

    #[test]
    fn mesh_worker_results_backpressure_parks_the_worker_until_drained() {
        let _heavy = heavy_ticket();
        let worker = MeshWorker::new().expect("worker starts");
        let ids = fill_to_bound(&worker);

        // The pending queue is empty and the completed queue exactly
        // full, so one more accepted job cannot be published until the
        // consumer frees capacity.
        let extra = section(99, 1);
        assert!(matches!(
            worker.try_submit(9, extra, vec![water_quad()]),
            Submit::Queued
        ));
        poll_until(
            || worker.facts().result_waits > 0,
            "the worker to park on results capacity",
        );
        let facts = worker.facts();
        assert_eq!(facts.pending_jobs, 0, "the extra job was decoded");
        assert_eq!(facts.completed_jobs, ids.len());
        assert_eq!(facts.completed_bytes, COMPLETED_BOUND_BYTES);

        // Draining the oldest result frees a maximal section of capacity
        // and the parked worker publishes the waiting result.
        match worker.try_take() {
            Take::Output(prepared) => assert_eq!(prepared.id, ids[0]),
            other => panic!("expected the oldest result, got {other:?}"),
        }
        let rest = drain(&worker, ids.len());
        assert_eq!(rest.len(), ids.len());
        let last = rest.last().expect("the extra result surfaces");
        assert_eq!(last.id, extra);
        let geometry = last.outcome.as_ref().expect("the water section decodes");
        assert_eq!(geometry.water.vertices.len(), 4);
        assert!(geometry.opaque.vertices.is_empty());
        worker.close();
    }

    #[test]
    fn mesh_worker_close_unblocks_a_worker_parked_on_results_capacity() {
        let _heavy = heavy_ticket();
        let worker = MeshWorker::new().expect("worker starts");
        fill_to_bound(&worker);
        assert!(matches!(
            worker.try_submit(9, section(7, 1), vec![stone_quad()]),
            Submit::Queued
        ));
        poll_until(
            || worker.facts().result_waits > 0,
            "the worker to park on results capacity",
        );
        // Close discards everything and must join the parked worker
        // instead of deadlocking against the capacity wait.
        worker.close();
        assert!(matches!(worker.try_take(), Take::Closed));
    }

    #[test]
    fn mesh_worker_cancel_pending_discards_queued_jobs_and_retires_the_epoch() {
        let (worker, gate) = gated_worker();
        assert!(matches!(
            worker.try_submit(5, section(0, 1), vec![stone_quad(); 3]),
            Submit::Queued
        ));
        poll_until(
            || gate.entered(),
            "the gated worker to pick up the in-flight job",
        );
        for index in 1..=3i32 {
            assert!(matches!(
                worker.try_submit(5, section(index, 2), vec![stone_quad()]),
                Submit::Queued
            ));
        }
        // Three queued jobs of the epoch are discarded; the in-flight job
        // keeps running by contract.
        assert_eq!(worker.cancel_pending(5), Ok(3));
        let facts = worker.facts();
        assert_eq!(facts.pending_jobs, 0);
        assert_eq!(facts.pending_bytes, 0);

        // The in-flight decode completes, but a retired epoch's result
        // never surfaces — and a same-epoch resubmit is discarded too.
        gate.open();
        poll_until(|| gate.finished(), "the in-flight decode to complete");
        assert!(matches!(
            worker.try_submit(5, section(9, 2), vec![stone_quad()]),
            Submit::Queued
        ));
        // A newer epoch resumes surfacing; FIFO ordering means its result
        // can only exist after both retired-epoch jobs were processed.
        let fresh = section(10, 1);
        assert!(matches!(
            worker.try_submit(6, fresh, vec![stone_quad()]),
            Submit::Queued
        ));
        let collected = drain(&worker, 1);
        assert_eq!(collected[0].id, fresh);
        assert_eq!(collected[0].epoch, 6);
        // Cancelling an epoch with nothing queued discards nothing.
        assert_eq!(worker.cancel_pending(6), Ok(0));
        worker.close();
    }

    #[test]
    fn mesh_worker_discards_results_of_superseded_epochs() {
        let worker = MeshWorker::new().expect("worker starts");
        let current = section(0, 1);
        assert!(matches!(
            worker.try_submit(2, current, vec![stone_quad()]),
            Submit::Queued
        ));
        let surfaced = drain(&worker, 1);
        assert_eq!(surfaced[0].id, current);
        assert_eq!(surfaced[0].epoch, 2);

        // A stale submit is accepted and never surfaces: the new-epoch
        // sweep may drop it from pending before decode, or the worker may
        // complete it and discard it at publication — both interleavings
        // satisfy this assertion, and the gated sweep test pins the
        // publication-discard path specifically.
        assert!(matches!(
            worker.try_submit(1, section(1, 1), vec![stone_quad()]),
            Submit::Queued
        ));
        let fresh = section(2, 1);
        assert!(matches!(
            worker.try_submit(3, fresh, vec![stone_quad()]),
            Submit::Queued
        ));
        let collected = drain(&worker, 1);
        assert_eq!(collected[0].id, fresh);
        assert_eq!(collected[0].epoch, 3);
        let facts = worker.facts();
        assert_eq!(facts.pending_jobs, 0);
        assert_eq!(facts.completed_jobs, 0);
        worker.close();
    }

    #[test]
    fn mesh_worker_new_epoch_sweeps_queued_prior_state() {
        let (worker, gate) = gated_worker();
        assert!(matches!(
            worker.try_submit(4, section(0, 1), vec![stone_quad(); 2]),
            Submit::Queued
        ));
        poll_until(
            || gate.entered(),
            "the gated worker to pick up the in-flight job",
        );
        assert!(matches!(
            worker.try_submit(4, section(1, 1), vec![stone_quad()]),
            Submit::Queued
        ));
        // A new session epoch invalidates prior state: the queued
        // epoch-4 job is swept on submit, leaving only the new job.
        let fresh = section(2, 1);
        assert!(matches!(
            worker.try_submit(5, fresh, vec![stone_quad()]),
            Submit::Queued
        ));
        let facts = worker.facts();
        assert_eq!(facts.pending_jobs, 1);
        assert_eq!(facts.pending_bytes, quad_decode::QUAD_BYTES);

        // The in-flight epoch-4 job completes but is discarded (its
        // epoch is superseded); only the epoch-5 job surfaces.
        gate.open();
        let collected = drain(&worker, 1);
        assert_eq!(collected[0].id, fresh);
        assert_eq!(collected[0].epoch, 5);
        worker.close();
    }

    #[test]
    fn mesh_worker_surfaces_decode_failures_as_error_results() {
        let worker = MeshWorker::new().expect("worker starts");
        let invalid_batch = vec![stone_quad(), stone_quad() | 1 << 63];
        assert!(matches!(
            worker.try_submit(1, section(0, 1), invalid_batch),
            Submit::Queued
        ));
        // An empty payload is not an upsert: decode fails closed.
        assert!(matches!(
            worker.try_submit(1, section(1, 1), Vec::new()),
            Submit::Queued
        ));
        // A payload above the frozen per-section quad limit decodes to an
        // error result (the whole-batch quad budget is stricter still:
        // a payload that large can never enter the pending queue, because
        // submit answers Full at the byte bound — the worker-side batch
        // guard stays the invariant that fails first if that ever drifts).
        let over_section = vec![stone_quad(); abi::MAX_SECTION_MESH_QUADS as usize + 1];
        assert!(matches!(
            worker.try_submit(1, section(2, 1), over_section),
            Submit::Queued
        ));
        assert!(matches!(
            worker.try_submit(1, section(3, 1), vec![stone_quad()]),
            Submit::Queued
        ));
        let collected = drain(&worker, 4);
        for (index, prepared) in collected.iter().enumerate().take(3) {
            assert_eq!(
                prepared.outcome,
                Err(abi::STATUS_INTERNAL),
                "failing job {index} surfaces its error word"
            );
        }
        assert_eq!(
            collected[0].input_quads, 2,
            "error results keep their input accounting"
        );
        assert_eq!(collected[1].input_quads, 0);
        assert_eq!(
            collected[2].input_quads,
            abi::MAX_SECTION_MESH_QUADS as usize + 1
        );
        let geometry = collected[3]
            .outcome
            .as_ref()
            .expect("the valid job still decodes after failures");
        assert_eq!(geometry.opaque.vertices.len(), 4);
        worker.close();
    }

    #[test]
    fn mesh_worker_converts_panicking_jobs_and_keeps_serving() {
        let worker = panicking_worker();
        assert!(matches!(
            worker.try_submit(1, section(0, 1), vec![POISON_WORD]),
            Submit::Queued
        ));
        let survivor = section(1, 1);
        assert!(matches!(
            worker.try_submit(1, survivor, vec![stone_quad()]),
            Submit::Queued
        ));
        let collected = drain(&worker, 2);
        assert_eq!(
            collected[0].outcome,
            Err(abi::STATUS_PANIC),
            "the panic surfaces as that job's stable error word"
        );
        assert_eq!(collected[1].id, survivor);
        assert!(collected[1].outcome.is_ok(), "the worker kept serving");
        worker.close();
    }

    #[test]
    fn mesh_worker_close_waits_for_the_in_flight_job_then_settles() {
        let (worker, gate) = gated_worker();
        assert!(matches!(
            worker.try_submit(1, section(0, 1), vec![stone_quad(); 4]),
            Submit::Queued
        ));
        poll_until(
            || gate.entered(),
            "the gated worker to pick up the in-flight job",
        );

        // Close runs on a helper thread because it must join the worker,
        // and the in-flight decode is still parked: close cannot return
        // yet by contract, which this bounded window observes.
        let worker = Arc::new(worker);
        let closer = {
            let worker = Arc::clone(&worker);
            thread::spawn(move || worker.close())
        };
        let window = Instant::now() + Duration::from_millis(150);
        while Instant::now() < window {
            assert!(
                !closer.is_finished(),
                "close returned while the in-flight decode was still parked"
            );
            thread::sleep(POLL_STEP);
        }
        // Releasing the decode lets the worker exit and close join it.
        gate.open();
        poll_until(
            || closer.is_finished(),
            "close to join the in-flight decode",
        );
        closer.join().expect("close thread");

        // Idempotent: later calls return immediately with no thread left
        // to join, and every API answers its stable closed status.
        let start = Instant::now();
        worker.close();
        assert!(
            start.elapsed() < Duration::from_secs(5),
            "a second close returns immediately"
        );
        assert!(matches!(
            worker.try_submit(1, section(1, 1), vec![stone_quad()]),
            Submit::Closed { .. }
        ));
        assert!(matches!(worker.try_take(), Take::Closed));
        assert_eq!(worker.cancel_pending(1), Err(abi::STATUS_INVALID_STATE));
        worker.close();
    }

    #[test]
    fn mesh_worker_drop_without_close_still_joins_the_worker() {
        let (worker, gate) = gated_worker();
        assert!(matches!(
            worker.try_submit(1, section(0, 1), vec![stone_quad(); 2]),
            Submit::Queued
        ));
        poll_until(
            || gate.entered(),
            "the gated worker to pick up the in-flight job",
        );
        // Drop waits for the in-flight decode (it is `close`), so it
        // cannot return while the decode is parked — and once released,
        // it must complete instead of deadlocking or leaking the thread.
        let dropper = thread::spawn(move || drop(worker));
        let window = Instant::now() + Duration::from_millis(150);
        while Instant::now() < window {
            assert!(
                !dropper.is_finished(),
                "drop returned while the in-flight decode was still parked"
            );
            thread::sleep(POLL_STEP);
        }
        gate.open();
        poll_until(
            || dropper.is_finished(),
            "drop to join the in-flight decode",
        );
        dropper.join().expect("drop thread");
    }
}
