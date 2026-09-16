// Package `runtime` owns the platform-independent client session lifecycle.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
	networktcp "github.com/channing771/mornlea/packages/shared/network/tcp"
)

// `ConnectionPhase` is the session lifecycle that `runtime` exposes to a presentation host.
// It aliases the shared presentation contract so hosts cannot observe divergent phase values.
type ConnectionPhase = presentation.SessionPhase

const (
	// `ConnectionPhaseNotReady` means `runtime` has not published a session lifecycle.
	ConnectionPhaseNotReady = presentation.SessionPhaseNotReady
	// `ConnectionPhaseConnecting` covers asynchronous transport setup.
	ConnectionPhaseConnecting = presentation.SessionPhaseConnecting
	// `ConnectionPhaseLogin` means transport setup completed and login is in progress.
	ConnectionPhaseLogin = presentation.SessionPhaseLogin
	// `ConnectionPhaseLoading` means login succeeded but the initial world is incomplete.
	ConnectionPhaseLoading = presentation.SessionPhaseLoading
	// `ConnectionPhasePlay` means `runtime` may publish playable presentation state.
	ConnectionPhasePlay = presentation.SessionPhasePlay
	// `ConnectionPhaseDisconnected` is the terminal session phase.
	ConnectionPhaseDisconnected = presentation.SessionPhaseDisconnected
)

// `Resource` is an owned `runtime` dependency that must release all of its work before `Close` returns.
// `Runtime` only coordinates this lifecycle; later tasks supply transport, receiver, mirror, and worker resources.
type Resource interface {
	Close() error
}

// `ResourceFactory` creates one `runtime`-owned resource during `New`.
// A resource returned with an error is still owned by `runtime` and is closed before `New` returns.
type ResourceFactory interface {
	Open() (Resource, error)
}

// `Dependencies` supplies the ordered `runtime` resource factories.
// `Resources` open in slice order and close in the strict reverse order, so later resources may depend on earlier ones.
type Dependencies struct {
	Resources []ResourceFactory
}

// `Receiver` owns a logged-in endpoint and exposes its bounded inbound queue to later runtime work.
// The interface keeps remote assembly platform-independent while preserving `client.Receiver` as the sole receiver implementation.
type Receiver interface {
	Resource
	TryRecv() (network.ServerMessage, bool)
	Err() error
}

// `inputSender` is a non-owning outbound view of the logged-in endpoint. The receiver remains the
// sole close owner; retaining this interface only lets prediction send protocol input with the
// caller's explicit context until a later bounded outbound queue takes over frame stepping.
type inputSender interface {
	Send(context.Context, network.ClientMessage) error
}

// `RemoteDependencies` permits deterministic remote-construction tests without replacing the production protocol implementation.
// A custom `Login` MUST close its input stream on error and transfer stream ownership to its non-nil endpoint on success; a nil function selects the existing constructor.
type RemoteDependencies struct {
	Dial        func(context.Context, string) (network.ClientPacketStream, error)
	Login       func(context.Context, network.ClientPacketStream, network.Identity, uint8) (network.ClientEndpoint, uint64, error)
	NewReceiver func(network.ClientEndpoint, int) (Receiver, error)
}

// `Options` defines the platform-independent inputs accepted by `New` and `NewRemote`.
// Remote fields identify an explicit dedicated-server session; the future local assembly can reuse the private logged-in endpoint seam.
type Options struct {
	Dependencies       Dependencies
	RemoteAddress      string
	Identity           network.Identity
	ViewDistance       uint8
	ReceiverCapacity   int
	RemoteDependencies RemoteDependencies
	// `Mesh` is optional so lifecycle and transcript tests can assemble a session without native
	// mesh workers. Production presentation hosts configure it before draining server messages.
	Mesh *MeshOptions
}

// `Validate` rejects configurations that cannot safely begin construction without opening a resource.
func (options Options) Validate() error {
	if len(options.Dependencies.Resources) == 0 {
		return errors.New("runtime: no resource factories configured")
	}
	for index, factory := range options.Dependencies.Resources {
		if isNilInterface(factory) {
			return fmt.Errorf("runtime: resource factory %d is nil", index)
		}
	}
	if options.Mesh != nil {
		if err := options.Mesh.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (options Options) validateRemote() error {
	if strings.TrimSpace(options.RemoteAddress) == "" {
		return errors.New("runtime: remote address is required")
	}
	if err := protocol.ValidateClientPacket(protocol.StateLogin, protocol.LoginStart{
		PlayerID:     options.Identity.PlayerID,
		DisplayName:  options.Identity.DisplayName,
		ViewDistance: options.ViewDistance,
	}); err != nil {
		return fmt.Errorf("runtime: invalid remote login options: %w", err)
	}
	if options.ReceiverCapacity < 1 {
		return errors.New("runtime: receiver capacity must be positive")
	}
	if options.Mesh != nil {
		if err := options.Mesh.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// `Runtime` owns an ordered set of platform-independent resources for one client session.
type Runtime struct {
	phaseMu       sync.RWMutex
	phase         ConnectionPhase
	sessionMu     sync.Mutex
	mirrors       *sessionMirrors
	predictor     *client.Predictor
	semanticInput SemanticInput
	sequence      uint64
	playerTick    uint64
	sessionClosed bool
	// Camera preference is local presentation state. Its eye pose is derived from the predicted
	// player under `sessionMu`, so neither a device nor a host render transform can become state.
	cameraMode       client.CameraMode
	cameraF5WasDown  bool
	cameraYaw        float32
	cameraPitch      float32
	cameraProjection CameraProjection
	// `cameraTargetReset` suppresses exactly the next published target after an authoritative
	// reset, matching the legacy frame boundary even when the corrected pose is unchanged.
	cameraTargetReset bool
	// Meshing remains entirely CPU-side: `client.Mesher` reaches engine ABI v11 only through the
	// existing `mesh` and `nativeabi` ownership chain, while runtime owns scheduling and publication.
	meshOptions      MeshOptions
	mesher           *client.Mesher
	meshReady        meshReadyQueue
	meshEpoch        uint64
	sectionRevisions map[core.SectionKey]uint64
	meshChunks       map[core.ChunkKey]struct{}

	resources  []Resource
	closeOnce  sync.Once
	closeErrMu sync.RWMutex
	closeErr   error

	receiver  Receiver
	sender    inputSender
	worldSeed uint64

	terminalMu  sync.Mutex
	terminalErr error
}

// `NewRemote` synchronously assembles a remote session through the established TCP and v44 login implementations.
// On login failure, the existing login state machine retains stream ownership and closes it; after success, only the receiver owns the endpoint.
func NewRemote(ctx context.Context, options Options) (*Runtime, error) {
	if ctx == nil {
		return nil, errors.New("runtime: nil remote context")
	}
	if err := options.validateRemote(); err != nil {
		return nil, err
	}

	dependencies := options.RemoteDependencies
	if dependencies.Dial == nil {
		dependencies.Dial = networktcp.DialTCP
	}
	if dependencies.Login == nil {
		dependencies.Login = network.LoginClientWithSeed
	}
	if dependencies.NewReceiver == nil {
		dependencies.NewReceiver = func(endpoint network.ClientEndpoint, capacity int) (Receiver, error) {
			return client.NewReceiver(endpoint, capacity), nil
		}
	}

	stream, err := dependencies.Dial(ctx, options.RemoteAddress)
	if err != nil {
		return nil, fmt.Errorf("runtime: dial remote %q: %w", options.RemoteAddress, err)
	}
	if isNilInterface(stream) {
		return nil, errors.New("runtime: remote dial returned nil stream")
	}

	endpoint, worldSeed, err := dependencies.Login(ctx, stream, options.Identity, options.ViewDistance)
	if err != nil {
		return nil, fmt.Errorf("runtime: remote login: %w", err)
	}
	if isNilInterface(endpoint) {
		return nil, errors.Join(errors.New("runtime: remote login returned nil endpoint"), stream.Close())
	}

	receiver, err := dependencies.NewReceiver(endpoint, options.ReceiverCapacity)
	if err != nil || isNilInterface(receiver) {
		if !isNilInterface(receiver) {
			err = errors.Join(err, receiver.Close())
		} else {
			err = errors.Join(err, endpoint.Close())
		}
		if err == nil {
			err = errors.New("runtime: receiver factory returned nil receiver")
		}
		return nil, fmt.Errorf("runtime: create receiver: %w", err)
	}

	runtime := newLoggedInRuntime(receiver, endpoint, worldSeed)
	if options.Mesh != nil {
		if err := runtime.ConfigureMeshing(*options.Mesh); err != nil {
			return nil, errors.Join(err, runtime.Close())
		}
	}
	return runtime, nil
}

// `newLoggedInRuntime` centralizes ownership transfer after a successful login without exposing a premature local-mode API.
// `sender` aliases the receiver-owned endpoint and MUST NOT be added to `resources` or closed separately.
func newLoggedInRuntime(receiver Receiver, sender inputSender, worldSeed uint64) *Runtime {
	return &Runtime{
		phase:            ConnectionPhaseLoading,
		resources:        []Resource{receiver},
		receiver:         receiver,
		sender:           sender,
		worldSeed:        worldSeed,
		mirrors:          newSessionMirrors(),
		predictor:        client.NewPredictor(),
		cameraProjection: DefaultCameraProjection(),
	}
}

// `New` validates every option before opening resources, then transfers successful resources to `runtime`.
// A failure closes all acquired resources in strict reverse order and joins release errors with the cause.
func New(options Options) (*Runtime, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}

	resources := make([]Resource, 0, len(options.Dependencies.Resources))
	for index, factory := range options.Dependencies.Resources {
		resource, err := factory.Open()
		if isNilInterface(resource) {
			err = errors.Join(err, fmt.Errorf("runtime: resource factory %d returned nil resource", index))
		}
		if err != nil {
			if !isNilInterface(resource) {
				resources = append(resources, resource)
			}
			return nil, errors.Join(
				fmt.Errorf("runtime: open resource %d: %w", index, err),
				closeResourcesReverse(resources),
			)
		}
		resources = append(resources, resource)
	}

	runtime := &Runtime{
		phase:            ConnectionPhaseNotReady,
		resources:        resources,
		mirrors:          newSessionMirrors(),
		predictor:        client.NewPredictor(),
		cameraProjection: DefaultCameraProjection(),
	}
	if options.Mesh != nil {
		if err := runtime.ConfigureMeshing(*options.Mesh); err != nil {
			return nil, errors.Join(err, runtime.Close())
		}
	}
	return runtime, nil
}

// `Phase` returns the current presentation-visible connection phase.
func (runtime *Runtime) Phase() ConnectionPhase {
	runtime.observeReceiverTerminalError()
	runtime.phaseMu.RLock()
	defer runtime.phaseMu.RUnlock()
	return runtime.phase
}

// `WorldSeed` returns the immutable authoritative seed received during the current successful login.
func (runtime *Runtime) WorldSeed() uint64 {
	return runtime.worldSeed
}

// `Err` reports a terminal receiver failure after releasing the receiver-owned endpoint.
func (runtime *Runtime) Err() error {
	runtime.observeReceiverTerminalError()
	runtime.terminalMu.Lock()
	terminalErr := runtime.terminalErr
	runtime.terminalMu.Unlock()
	runtime.closeErrMu.RLock()
	defer runtime.closeErrMu.RUnlock()
	return errors.Join(terminalErr, runtime.closeErr)
}

// `Close` releases resources in strict reverse construction order exactly once.
// Concurrent callers wait for the same shutdown and receive the same aggregated error.
func (runtime *Runtime) Close() error {
	runtime.closeOnce.Do(func() {
		runtime.sessionMu.Lock()
		runtime.sessionClosed = true
		mesher := runtime.resetSessionLocked(true)
		runtime.sessionMu.Unlock()
		if mesher != nil {
			mesher.Close()
		}

		runtime.phaseMu.Lock()
		resources := runtime.resources
		runtime.resources = nil
		runtime.phase = ConnectionPhaseDisconnected
		runtime.phaseMu.Unlock()

		runtime.closeErrMu.Lock()
		runtime.closeErr = closeResourcesReverse(resources)
		runtime.closeErrMu.Unlock()
	})
	runtime.closeErrMu.RLock()
	defer runtime.closeErrMu.RUnlock()
	return runtime.closeErr
}

func (runtime *Runtime) observeReceiverTerminalError() {
	if runtime.receiver == nil {
		return
	}
	if err := runtime.receiver.Err(); err != nil {
		runtime.terminalMu.Lock()
		if runtime.terminalErr == nil {
			runtime.terminalErr = err
		}
		runtime.terminalMu.Unlock()
		_ = runtime.Close()
	}
}

func closeResourcesReverse(resources []Resource) error {
	var closeErr error
	for index := len(resources) - 1; index >= 0; index-- {
		closeErr = errors.Join(closeErr, resources[index].Close())
	}
	return closeErr
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
