package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// `MaxStepMessageBudget` bounds receiver polls inside one `Step`. It matches the legacy
// per-frame drain bound so a step-driven host observes the same inbound backpressure.
const MaxStepMessageBudget = 4096

// `FrameSnapshotVersion` re-exports the presentation frame identity so hosts compare step
// frames against one contract value without importing a second version domain.
const FrameSnapshotVersion = presentation.FrameSnapshotVersion

// `stepOutboundQueueCapacity` absorbs one maximally budgeted step (resyncs plus the
// predictor's fixed-step cap) so enqueueing never blocks or drops inside a single step.
const stepOutboundQueueCapacity = MaxStepMessageBudget + 8

// `StepResult` is the immutable outcome of one bounded `Step`. `Frame` is always a complete
// published snapshot; `World` is meaningful only when `HasWorld` reports a publication.
type StepResult struct {
	Frame             presentation.FrameSnapshot
	World             presentation.WorldBatch
	HasWorld          bool
	MessagesProcessed int
}

// `Step` advances one bounded frame of the client session from explicit inputs only.
// `elapsed` drives fixed-step prediction; `messageBudget` bounds non-blocking receiver
// polls; `meshBudget` bounds ready world-batch sections. A successful step publishes at
// most one `FrameSnapshot` and at most one `WorldBatch`, never reads a wall clock, and
// never waits on the network: outbound input and resync messages are handed to an
// asynchronous worker and a full outbound queue surfaces as an error instead of blocking.
func (runtime *Runtime) Step(elapsed time.Duration, messageBudget, meshBudget int) (StepResult, error) {
	if runtime == nil {
		return StepResult{}, errors.New("runtime: nil runtime")
	}
	// Validation precedes every state mutation: a rejected step must not consume receiver
	// messages, advance prediction, or consume the camera reset-target latch.
	if elapsed < 0 {
		return StepResult{}, errors.New("runtime: negative step elapsed time")
	}
	if messageBudget < 0 || messageBudget > MaxStepMessageBudget {
		return StepResult{}, fmt.Errorf(
			"runtime: step message budget %d is outside 0..%d", messageBudget, MaxStepMessageBudget,
		)
	}
	if meshBudget < 0 || meshBudget > presentation.MaxWorldBatchOperations {
		return StepResult{}, fmt.Errorf(
			"runtime: step mesh budget %d is outside 0..%d", meshBudget, presentation.MaxWorldBatchOperations,
		)
	}
	if runtime.sessionIsClosed() {
		return StepResult{}, errors.New("runtime: client session is closed")
	}
	if receiverErr := runtime.receiverStepError(); receiverErr != nil {
		return runtime.publishTerminalFrame()
	}

	messages, err := runtime.drainStepMessages(messageBudget)
	if err != nil {
		return StepResult{}, err
	}
	if err := runtime.advanceStepPrediction(elapsed); err != nil {
		return StepResult{}, err
	}
	world, hasWorld, err := runtime.drainStepWorld(meshBudget)
	if err != nil {
		return StepResult{}, err
	}
	result, err := runtime.assembleStepFrame(elapsed)
	if err != nil {
		return StepResult{}, err
	}
	result.World = world
	result.HasWorld = hasWorld
	result.MessagesProcessed = messages
	return result, nil
}

func (runtime *Runtime) sessionIsClosed() bool {
	runtime.sessionMu.Lock()
	closed := runtime.sessionClosed
	runtime.sessionMu.Unlock()
	return closed
}

func (runtime *Runtime) receiverStepError() error {
	if runtime.receiver == nil {
		return nil
	}
	return runtime.receiver.Err()
}

// `drainStepMessages` applies at most `budget` receiver messages in one `sessionMu` section.
// A revealed chunk resync is enqueued for the outbound worker here, so numbered resyncs keep
// their earlier sequence position ahead of this step's prediction inputs.
func (runtime *Runtime) drainStepMessages(budget int) (int, error) {
	if budget == 0 || runtime.receiver == nil {
		return 0, nil
	}
	runtime.sessionMu.Lock()
	if runtime.sessionClosed {
		runtime.sessionMu.Unlock()
		return 0, errors.New("runtime: client session is closed")
	}
	step := runtime.stepLocked()
	processed := 0
	var drainErr error
	for processed < budget {
		message, ok := runtime.receiver.TryRecv()
		if !ok {
			break
		}
		if message == nil {
			drainErr = errors.New("runtime: nil server message")
			break
		}
		outcome, err := runtime.applyMessageLocked(message)
		processed++
		if err != nil {
			drainErr = err
			break
		}
		if err := step.noteOutcomeLocked(runtime, outcome); err != nil {
			drainErr = err
			break
		}
	}
	runtime.sessionMu.Unlock()
	if drainErr != nil {
		if errors.Is(drainErr, errMeshPipeline) {
			// Mirror overflow terminates the session outside the held lock, matching `ApplyMessage`.
			runtime.stopForMeshFailure(drainErr)
		}
		return processed, fmt.Errorf("runtime: apply server message: %w", drainErr)
	}
	return processed, nil
}

// `advanceStepPrediction` consumes only the explicit `elapsed`. Zero elapsed advances
// nothing, and a not-ready predictor skips the advance exactly like the legacy frame loop,
// so stepping during loading neither fails nor allocates protocol input.
func (runtime *Runtime) advanceStepPrediction(elapsed time.Duration) error {
	if elapsed == 0 {
		return nil
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.sessionClosed {
		return errors.New("runtime: client session is closed")
	}
	if runtime.predictor == nil {
		runtime.predictor = client.NewPredictor()
	}
	if _, ready := runtime.predictor.State(); !ready {
		return nil
	}
	_ = runtime.stepLocked()
	control := runtime.semanticInput.control()
	source := client.MirrorCollisionSource{Mirror: runtime.mirrors.world, Dimension: core.Overworld}
	return runtime.predictor.Advance(
		elapsed,
		control,
		source,
		runtime.nextSequenceLocked,
		func(message network.PlayerInput) error {
			return runtime.enqueueOutboundLocked(message)
		},
	)
}

func (runtime *Runtime) drainStepWorld(budget int) (presentation.WorldBatch, bool, error) {
	if budget == 0 {
		return presentation.WorldBatch{}, false, nil
	}
	return runtime.DrainWorldBatch(budget)
}

// `assembleStepFrame` publishes the frame identity plus confirmed aggregates. Camera and
// target come from `CameraPresentation`, which consumes the authoritative reset latch only
// when this step actually reaches publication; earlier step failures leave the latch intact.
// A receiver that dies mid-step yields the same single terminal frame as a step that began
// with the failure already visible.
func (runtime *Runtime) assembleStepFrame(elapsed time.Duration) (StepResult, error) {
	phase := runtime.Phase()
	if phase == ConnectionPhaseDisconnected {
		if receiverErr := runtime.receiverStepError(); receiverErr != nil {
			return runtime.publishTerminalFrame()
		}
		return StepResult{}, errors.New("runtime: client session is closed")
	}

	var (
		hud         presentation.HUDSnapshot
		environment presentation.EnvironmentSnapshot
		entities    presentation.EntityBatch
		revision    uint64
		epoch       uint64
	)
	runtime.sessionMu.Lock()
	step := runtime.stepLocked()
	revision = step.frameRevision + 1
	epoch = runtime.frameEpochLocked()
	snapshot := runtime.predictionSnapshotLocked(0)
	serverTick := runtime.playerTick
	if snapshot.Ready {
		hud = presentation.HUDSnapshot{
			Ready: true, Health: snapshot.Health, Hunger: snapshot.Hunger, Oxygen: snapshot.Oxygen,
		}
		if step.worldTimeConfirmed {
			environment = presentation.EnvironmentSnapshot{
				Ready:          true,
				ServerTick:     serverTick,
				WorldTimeTicks: step.worldTimeTicks,
				DayPhaseOffset: step.dayPhaseOffset,
				Weather:        snapshot.Weather,
				Season:         snapshot.Season,
				SeasonProgress: snapshot.SeasonProgress,
				Temperature:    snapshot.Temperature,
			}
		}
	}
	var entityErr error
	entities, entityErr = step.entityBatchLocked(serverTick, elapsed)
	runtime.sessionMu.Unlock()
	if entityErr != nil {
		return StepResult{}, entityErr
	}

	camera := runtime.CameraPresentation()
	frame := presentation.FrameSnapshot{
		Version:     presentation.FrameSnapshotVersion,
		Revision:    revision,
		Epoch:       epoch,
		Phase:       phase,
		Camera:      camera.Camera,
		HUD:         hud,
		Environment: environment,
		Entities:    entities,
		Target:      camera.Target,
	}
	if err := frame.Validate(); err != nil {
		return StepResult{}, fmt.Errorf("runtime: build step frame: %w", err)
	}
	// The identity commits only together with the successful publication, so failed frames
	// never consume a revision. The compare-and-set alone does not guarantee one revision
	// per published frame: two concurrent Step callers could publish duplicate revisions
	// when the second commit no-ops. Uniqueness rests on the single-driver step contract.
	runtime.sessionMu.Lock()
	if step.frameRevision == revision-1 {
		step.frameRevision = revision
	}
	runtime.sessionMu.Unlock()
	return StepResult{Frame: frame}, nil
}

// `publishTerminalFrame` reports a terminal receiver failure as exactly one published frame.
// The helper records the cause through the existing terminal-error path, which also closes
// the session, so any later `Step` fails before publishing again.
func (runtime *Runtime) publishTerminalFrame() (StepResult, error) {
	runtime.observeReceiverTerminalError()
	runtime.sessionMu.Lock()
	step := runtime.stepLocked()
	revision := step.frameRevision + 1
	epoch := runtime.frameEpochLocked()
	runtime.sessionMu.Unlock()
	frame := presentation.FrameSnapshot{
		Version:  presentation.FrameSnapshotVersion,
		Revision: revision,
		Epoch:    epoch,
		Phase:    runtime.Phase(),
		Error:    presentation.ErrorState{Code: presentation.ErrorConnection},
	}
	if err := frame.Validate(); err != nil {
		return StepResult{}, fmt.Errorf("runtime: terminal frame: %w", err)
	}
	runtime.sessionMu.Lock()
	if step.frameRevision == revision-1 {
		step.frameRevision = revision
	}
	runtime.sessionMu.Unlock()
	return StepResult{Frame: frame}, nil
}

// `frameEpochLocked` aligns the frame epoch with the world publication epoch. Without a
// configured mesher the mesh epoch is zero, yet frames still require a positive identity,
// so the unconfigured session reports epoch 1 and no world batch can exist to invalidate.
func (runtime *Runtime) frameEpochLocked() uint64 {
	if runtime.meshEpoch == 0 {
		return 1
	}
	return runtime.meshEpoch
}

// `stepState` is the cross-call state of the step pipeline, guarded by `sessionMu`.
// `mirrors` records the mirror-set generation: `ResetMirrors` installs a new mirror object,
// and comparing identities discards step-owned confirmed aggregates at that boundary.
// `frameRevision` is monotonic for the runtime lifetime; the epoch distinguishes resets.
type stepState struct {
	mirrors            *sessionMirrors
	frameRevision      uint64
	worldTimeConfirmed bool
	worldTimeTicks     uint64
	dayPhaseOffset     uint16
	remotes            map[core.PlayerID]*remoteFrameActor
	outbound           *outboundQueue
}

func newStepState(mirrors *sessionMirrors) *stepState {
	return &stepState{mirrors: mirrors, remotes: make(map[core.PlayerID]*remoteFrameActor)}
}

// `stepLocked` returns the per-runtime step state, dropping confirmed environment and
// remote-pose aggregates when the session mirrors were reset since the previous call.
// It also mirrors `ApplyMessage`'s lazy mirror-set creation so step phases never observe
// a nil mirror set on defensively constructed runtimes.
func (runtime *Runtime) stepLocked() *stepState {
	if runtime.mirrors == nil {
		runtime.mirrors = newSessionMirrors()
	}
	if runtime.step == nil {
		runtime.step = newStepState(runtime.mirrors)
		return runtime.step
	}
	if runtime.step.mirrors != runtime.mirrors {
		runtime.step.resetAfterMirrorsLocked(runtime.mirrors)
	}
	return runtime.step
}

func (step *stepState) resetAfterMirrorsLocked(mirrors *sessionMirrors) {
	step.mirrors = mirrors
	step.worldTimeConfirmed = false
	step.worldTimeTicks = 0
	step.dayPhaseOffset = 0
	clear(step.remotes)
}

// `noteOutcomeLocked` feeds step-owned aggregates from one applied message. The numbered
// chunk resync produced by a stale-mirror delta joins the outbound queue immediately so it
// precedes this step's prediction inputs in both sequence and transport order.
func (step *stepState) noteOutcomeLocked(runtime *Runtime, outcome MessageOutcome) error {
	if outcome.World.Resync != nil {
		if err := runtime.enqueueOutboundLocked(*outcome.World.Resync); err != nil {
			return fmt.Errorf("runtime: enqueue chunk resync: %w", err)
		}
	}
	if outcome.PredictionChanged {
		step.noteEnvironmentLocked(outcome.Message)
	}
	step.noteRemotesLocked(outcome.Message)
	return nil
}

func (step *stepState) noteEnvironmentLocked(message network.ServerMessage) {
	switch message := message.(type) {
	case network.PlayerState:
		step.worldTimeConfirmed = true
		step.worldTimeTicks = message.WorldTimeTicks
		step.dayPhaseOffset = message.DayPhaseOffset
	case *network.PlayerState:
		if message == nil {
			return
		}
		step.worldTimeConfirmed = true
		step.worldTimeTicks = message.WorldTimeTicks
		step.dayPhaseOffset = message.DayPhaseOffset
	}
}

func (step *stepState) noteRemotesLocked(message network.ServerMessage) {
	switch message := message.(type) {
	case network.RemotePlayerSpawn:
		step.resetRemoteLocked(message.PlayerID, remoteFrameSample{
			tick: message.ServerTick, dimension: message.Dimension,
			position: message.Position, yaw: message.Yaw, pitch: message.Pitch,
		})
	case *network.RemotePlayerSpawn:
		if message != nil {
			step.resetRemoteLocked(message.PlayerID, remoteFrameSample{
				tick: message.ServerTick, dimension: message.Dimension,
				position: message.Position, yaw: message.Yaw, pitch: message.Pitch,
			})
		}
	case network.RemotePlayerStates:
		step.noteRemoteStatesLocked(message)
	case *network.RemotePlayerStates:
		if message != nil {
			step.noteRemoteStatesLocked(*message)
		}
	case network.RemotePlayerDespawn:
		delete(step.remotes, message.PlayerID)
	case *network.RemotePlayerDespawn:
		if message != nil {
			delete(step.remotes, message.PlayerID)
		}
	}
}

func (step *stepState) noteRemoteStatesLocked(states network.RemotePlayerStates) {
	for _, state := range states.Players {
		sample := remoteFrameSample{
			tick: states.ServerTick, dimension: state.Dimension,
			position: state.Position, yaw: state.Yaw, pitch: state.Pitch,
		}
		if state.Reset {
			step.resetRemoteLocked(state.PlayerID, sample)
			continue
		}
		actor, exists := step.remotes[state.PlayerID]
		if !exists {
			step.resetRemoteLocked(state.PlayerID, sample)
			continue
		}
		actor.push(sample)
	}
}

func (step *stepState) resetRemoteLocked(id core.PlayerID, sample remoteFrameSample) {
	actor, exists := step.remotes[id]
	if !exists {
		actor = &remoteFrameActor{}
		step.remotes[id] = actor
	}
	actor.reset(sample)
}

// `entityBatchLocked` presents one bounded interpolated pose per tracked remote player.
// The pose always stays inside the two most recent authoritative samples: the interpolation
// weight is the explicit step elapsed over elapsed-plus-window, so any finite elapsed yields
// a strictly interior pose and zero elapsed presents the previous authoritative sample.
// Records keep deterministic player-ID order and are truncated to the batch capacity.
func (step *stepState) entityBatchLocked(serverTick uint64, elapsed time.Duration) (presentation.EntityBatch, error) {
	ids := make([]core.PlayerID, 0, len(step.remotes))
	for id := range step.remotes {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return presentation.EntityBatch{}, nil
	}
	slices.SortFunc(ids, func(left, right core.PlayerID) int {
		return bytes.Compare(left[:], right[:])
	})
	if len(ids) > presentation.MaxEntityBatchRecords {
		ids = ids[:presentation.MaxEntityBatchRecords]
	}
	records := make([]presentation.EntityRecord, 0, len(ids))
	for _, id := range ids {
		sample := step.remotes[id].sample(elapsed)
		records = append(records, presentation.EntityRecord{
			Kind:      presentation.EntityKindRemotePlayer,
			PlayerID:  id,
			Dimension: sample.dimension,
			Position:  [3]float32(sample.position),
			Yaw:       sample.yaw,
			Pitch:     sample.pitch,
		})
	}
	batch, err := presentation.NewEntityBatch(serverTick, records)
	if err != nil {
		return presentation.EntityBatch{}, fmt.Errorf("runtime: build entity batch: %w", err)
	}
	return batch, nil
}

// `remoteFrameActor` keeps the two most recent authoritative samples of one remote player.
type remoteFrameActor struct {
	samples [2]remoteFrameSample
	count   int
}

type remoteFrameSample struct {
	tick      uint64
	dimension core.DimensionID
	position  mgl32.Vec3
	yaw       float32
	pitch     float32
}

func (actor *remoteFrameActor) reset(sample remoteFrameSample) {
	actor.samples[0] = sample
	actor.samples[1] = sample
	actor.count = 1
}

func (actor *remoteFrameActor) push(sample remoteFrameSample) {
	if actor.count == 0 || sample.dimension != actor.samples[1].dimension {
		actor.reset(sample)
		return
	}
	actor.samples[0] = actor.samples[1]
	actor.samples[1] = sample
	actor.count = 2
}

func (actor *remoteFrameActor) sample(elapsed time.Duration) remoteFrameSample {
	if actor.count < 2 {
		return actor.samples[actor.count-1]
	}
	previous, current := actor.samples[0], actor.samples[1]
	windowTicks := current.tick - previous.tick
	if windowTicks == 0 {
		return current
	}
	window := time.Duration(windowTicks) * physics.FixedDelta
	alpha := float32(elapsed) / float32(elapsed+window)
	return remoteFrameSample{
		tick:      current.tick,
		dimension: current.dimension,
		position:  previous.position.Mul(1 - alpha).Add(current.position.Mul(alpha)),
		yaw:       lerpShortestAngle(previous.yaw, current.yaw, alpha),
		pitch:     previous.pitch + (current.pitch-previous.pitch)*alpha,
	}
}

func lerpShortestAngle(from, to, alpha float32) float32 {
	delta := normalizeStepAngle(to - from)
	return normalizeStepAngle(from + delta*alpha)
}

func normalizeStepAngle(angle float32) float32 {
	wrapped := float32(math.Mod(float64(angle)+math.Pi, 2*math.Pi))
	if wrapped < 0 {
		wrapped += 2 * math.Pi
	}
	return wrapped - math.Pi
}

// `outboundQueue` is the bounded asynchronous outbound path shared by prediction inputs and
// numbered chunk resyncs. The worker owns the only blocking transport interaction; `Step`
// only performs non-blocking enqueue operations under `sessionMu`.
type outboundQueue struct {
	ctx    context.Context
	cancel context.CancelFunc
	queue  chan network.ClientMessage
}

// `Close` cancels the in-flight send through the send context and abandons queued messages;
// it never waits for a wedged transport write and always reports success so close ordering
// keeps releasing the receiver-owned endpoint afterwards.
func (queue *outboundQueue) Close() error {
	queue.cancel()
	return nil
}

func (queue *outboundQueue) run(sender inputSender) {
	for {
		// Cancellation outranks queued work: closing discards pending outbound instead of
		// letting the worker begin further transport sends.
		select {
		case <-queue.ctx.Done():
			return
		default:
		}
		select {
		case <-queue.ctx.Done():
			return
		case message := <-queue.queue:
			// Send failures are not retryable here: inputs retry through the predictor's
			// fixed-step path and terminal receiver failures end the session, so the worker
			// stays available for later messages. The enqueue-side queue-full contract,
			// including the non-retryable desync latch for dropped resyncs, is documented at
			// `enqueueOutboundLocked`.
			_ = sender.Send(queue.ctx, message)
		}
	}
}

// `enqueueOutboundLocked` hands one already-numbered immutable message to the outbound
// worker. A runtime assembled without an outbound endpoint accepts the message locally:
// that seam exists for lifecycle tests and the future local-mode assembly, while production
// remote sessions always carry the receiver-owned endpoint as their sender.
func (runtime *Runtime) enqueueOutboundLocked(message network.ClientMessage) error {
	if runtime.sender == nil {
		return nil
	}
	step := runtime.stepLocked()
	if step.outbound == nil {
		runtime.startOutboundLocked(step)
	}
	select {
	case step.outbound.queue <- message:
		return nil
	default:
		// Queue-full failure contract: a dropped prediction input can abort `Advance` mid-loop
		// with a partially consumed accumulator and input history plus a sequence gap; later
		// steps self-heal through authoritative reconciliation. A dropped chunk resync is not
		// recoverable at runtime: the mirror already latched the chunk `Desynced` with its
		// resync marked in flight, so no later step can re-request it, and convergence returns
		// only through `ResetMirrors` or `Close`.
		return errors.New("runtime: step outbound queue is full")
	}
}

// `startOutboundLocked` registers the worker as the last runtime resource so `Close`
// cancels it before the receiver-owned endpoint is released. Registration happens under
// `sessionMu` after the closed-session check, and `Close` marks the session closed under
// the same mutex before snapshotting resources, so the worker can never escape shutdown.
func (runtime *Runtime) startOutboundLocked(step *stepState) {
	ctx, cancel := context.WithCancel(context.Background())
	queue := &outboundQueue{
		ctx:    ctx,
		cancel: cancel,
		queue:  make(chan network.ClientMessage, stepOutboundQueueCapacity),
	}
	runtime.phaseMu.Lock()
	runtime.resources = append(runtime.resources, queue)
	runtime.phaseMu.Unlock()
	step.outbound = queue
	go queue.run(runtime.sender)
}
