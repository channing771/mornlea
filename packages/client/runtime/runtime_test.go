package runtime

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestLifecyclePartialConstructionFailureReleasesInStrictReverseOrder(t *testing.T) {
	constructErr := errors.New("construct third resource")
	secondCloseErr := errors.New("close second resource")
	firstCloseErr := errors.New("close first resource")
	var events []string

	runtime, err := New(Options{Dependencies: Dependencies{Resources: []ResourceFactory{
		resourceFactoryFunc(func() (Resource, error) {
			events = append(events, "open:first")
			return &recordingResource{name: "first", events: &events, closeErr: firstCloseErr}, nil
		}),
		resourceFactoryFunc(func() (Resource, error) {
			events = append(events, "open:second")
			return &recordingResource{name: "second", events: &events, closeErr: secondCloseErr}, nil
		}),
		resourceFactoryFunc(func() (Resource, error) {
			events = append(events, "open:third")
			return nil, constructErr
		}),
	}}})
	if runtime != nil {
		t.Fatal("New returned a runtime after construction failure")
	}
	if !errors.Is(err, constructErr) || !errors.Is(err, secondCloseErr) || !errors.Is(err, firstCloseErr) {
		t.Fatalf("New error does not preserve construction and release errors: %v", err)
	}
	if want := []string{"open:first", "open:second", "open:third", "close:second", "close:first"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
}

func TestLifecycleInvalidOptionsHaveNoSideEffects(t *testing.T) {
	if runtime, err := New(Options{}); runtime != nil || err == nil {
		t.Fatalf("New(Options{}) = (%v, %v), want nil runtime and error", runtime, err)
	}

	called := false
	runtime, err := New(Options{Dependencies: Dependencies{Resources: []ResourceFactory{
		resourceFactoryFunc(func() (Resource, error) {
			called = true
			return &recordingResource{}, nil
		}),
		nil,
	}}})
	if runtime != nil || err == nil {
		t.Fatalf("New with nil factory = (%v, %v), want nil runtime and error", runtime, err)
	}
	if called {
		t.Fatal("New invoked a factory before rejecting invalid options")
	}
}

func TestLifecycleStartsNotReady(t *testing.T) {
	runtime, err := New(Options{Dependencies: Dependencies{Resources: []ResourceFactory{
		resourceFactoryFunc(func() (Resource, error) { return &recordingResource{}, nil }),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()

	if got := runtime.Phase(); got != ConnectionPhaseNotReady {
		t.Fatalf("Phase() = %v, want %v", got, ConnectionPhaseNotReady)
	}
}

func TestCloseIsIdempotentAndConcurrent(t *testing.T) {
	closeErr := errors.New("close second resource")
	var events []string
	var eventsMu sync.Mutex
	newResource := func(name string, err error) ResourceFactory {
		return resourceFactoryFunc(func() (Resource, error) {
			return &recordingResource{name: name, events: &events, eventsMu: &eventsMu, closeErr: err}, nil
		})
	}
	runtime, err := New(Options{Dependencies: Dependencies{Resources: []ResourceFactory{
		newResource("first", nil),
		newResource("second", closeErr),
	}}})
	if err != nil {
		t.Fatal(err)
	}

	const callers = 32
	errorsByCaller := make(chan error, callers)
	var callersWG sync.WaitGroup
	callersWG.Add(callers)
	for range callers {
		go func() {
			defer callersWG.Done()
			errorsByCaller <- runtime.Close()
		}()
	}
	callersWG.Wait()
	close(errorsByCaller)
	for closeResult := range errorsByCaller {
		if !errors.Is(closeResult, closeErr) {
			t.Fatalf("Close() error = %v, want joined %v", closeResult, closeErr)
		}
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()
	if want := []string{"close:second", "close:first"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("close events = %v, want %v", events, want)
	}
	if got := runtime.Phase(); got != ConnectionPhaseDisconnected {
		t.Fatalf("Phase() after Close = %v, want %v", got, ConnectionPhaseDisconnected)
	}
}

type resourceFactoryFunc func() (Resource, error)

func (factory resourceFactoryFunc) Open() (Resource, error) {
	return factory()
}

type recordingResource struct {
	name     string
	events   *[]string
	eventsMu *sync.Mutex
	closeErr error
}

func (resource *recordingResource) Close() error {
	if resource.events != nil {
		if resource.eventsMu != nil {
			resource.eventsMu.Lock()
			defer resource.eventsMu.Unlock()
		}
		*resource.events = append(*resource.events, "close:"+resource.name)
	}
	return resource.closeErr
}
