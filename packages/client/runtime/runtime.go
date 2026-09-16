// Package `runtime` owns the platform-independent client session lifecycle.
package runtime

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/channing771/mornlea/packages/client/presentation"
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

// `Options` defines the platform-independent inputs accepted by `New`.
// The initial `runtime` deliberately accepts only generic lifecycle dependencies; connection details arrive later.
type Options struct {
	Dependencies Dependencies
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
	return nil
}

// `Runtime` owns an ordered set of platform-independent resources for one client session.
type Runtime struct {
	phaseMu sync.RWMutex
	phase   ConnectionPhase

	resources []Resource
	closeOnce sync.Once
	closeErr  error
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

	return &Runtime{
		phase:     ConnectionPhaseNotReady,
		resources: resources,
	}, nil
}

// `Phase` returns the current presentation-visible connection phase.
func (runtime *Runtime) Phase() ConnectionPhase {
	runtime.phaseMu.RLock()
	defer runtime.phaseMu.RUnlock()
	return runtime.phase
}

// `Close` releases resources in strict reverse construction order exactly once.
// Concurrent callers wait for the same shutdown and receive the same aggregated error.
func (runtime *Runtime) Close() error {
	runtime.closeOnce.Do(func() {
		runtime.phaseMu.Lock()
		resources := runtime.resources
		runtime.resources = nil
		runtime.phase = ConnectionPhaseDisconnected
		runtime.phaseMu.Unlock()

		runtime.closeErr = closeResourcesReverse(resources)
	})
	return runtime.closeErr
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
