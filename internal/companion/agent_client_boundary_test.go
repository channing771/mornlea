package companion

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentClientResponseBodyExactByteBoundary(t *testing.T) {
	valid := []byte(`{"status":"live"}`)
	for _, size := range []int{AgentMaxResponseBodyBytes, AgentMaxResponseBodyBytes + 1} {
		size := size
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			body := append(append([]byte(nil), valid...), []byte(strings.Repeat(" ", size-len(valid)))...)
			client := newManifestAgentClient(t, fixedAgentResponseHandler(http.StatusOK, body))
			got, err := client.Live(context.Background())
			if size == AgentMaxResponseBodyBytes {
				if err != nil || got.Status != "live" {
					t.Fatalf("exact response = %+v, %v", got, err)
				}
				return
			}
			if got != (LiveResponse{}) || !errors.Is(err, ErrAgentUnavailable) {
				t.Fatalf("oversized response = %+v, %v", got, err)
			}
		})
	}
}

func TestAgentClientBodyLimitChecksAreInclusiveAtExactByte(t *testing.T) {
	for name, maximum := range map[string]int{
		"request":  AgentMaxRequestBodyBytes,
		"response": AgentMaxResponseBodyBytes,
	} {
		t.Run(name, func(t *testing.T) {
			if !agentBodyWithinLimit(make([]byte, maximum), maximum) {
				t.Fatal("exact body boundary rejected")
			}
			if agentBodyWithinLimit(make([]byte, maximum+1), maximum) {
				t.Fatal("body above boundary accepted")
			}
		})
	}
}

func TestAgentClientRejectsChunkedResponsePastByteBoundary(t *testing.T) {
	body := append([]byte(`{"status":"live"}`), []byte(strings.Repeat(" ", AgentMaxResponseBodyBytes))...)
	client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write(body)
	}))
	got, err := client.Live(context.Background())
	if got != (LiveResponse{}) || !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("chunked oversized response = %+v, %v", got, err)
	}
}

func TestAgentClientRejectsDeclaredResponseLengthBeforeDecode(t *testing.T) {
	client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(AgentMaxResponseBodyBytes+1))
		w.WriteHeader(http.StatusOK)
	}))
	got, err := client.Live(context.Background())
	if got != (LiveResponse{}) || !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("declared oversized response = %+v, %v", got, err)
	}
}

func TestAgentClientRequestHeaderExactByteBoundary(t *testing.T) {
	request := validAgentAcquireRequest()
	response := validAgentAcquireResponseJSON()
	for _, size := range []int{AgentMaxHeaderBytes, AgentMaxHeaderBytes + 1} {
		size := size
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			headers := make(http.Header)
			headers.Set("Authorization", "Bearer ")
			headers.Set("Content-Type", "application/json")
			headers.Set("Accept-Encoding", "identity")
			credentialBytes := size - requestHeaderBytes(headers)
			if credentialBytes < 1 {
				t.Fatal("header fixture cannot fit credential")
			}
			credential := strings.Repeat("s", credentialBytes)
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(response)
			}))
			defer server.Close()
			client, err := NewAgentClient(AgentServiceSettings{Endpoint: server.URL, APIKeyEnv: "BOUNDARY_KEY"}, credential, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			got, err := client.Acquire(context.Background(), request)
			if size == AgentMaxHeaderBytes {
				if err != nil || !called || got.LeaseID != agentLeaseID {
					t.Fatalf("exact header = %+v, %v, called=%t", got, err, called)
				}
				return
			}
			if got != (AcquireResponse{}) || !errors.Is(err, ErrAgentUnavailable) || called {
				t.Fatalf("oversized header = %+v, %v, called=%t", got, err, called)
			}
		})
	}
}

func TestAgentClientResponseHeaderExactByteBoundary(t *testing.T) {
	for _, size := range []int{AgentMaxHeaderBytes, AgentMaxHeaderBytes + 1} {
		size := size
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			body := []byte(`{"status":"live"}`)
			headers := make(http.Header)
			headers.Set("Content-Type", "application/json")
			headers.Set("Content-Length", strconv.Itoa(len(body)))
			headers.Set("Date", "Mon, 02 Jan 2006 15:04:05 GMT")
			headers.Set("X-Pad", "")
			padding := size - requestHeaderBytes(headers) - len("HTTP/1.1 200 OK\r\n") - len("Connection: close\r\n")
			if padding < 0 {
				t.Fatal("header fixture exceeds target before padding")
			}
			headers.Set("X-Pad", strings.Repeat("p", padding))
			client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for name, values := range headers {
					for _, value := range values {
						w.Header().Add(name, value)
					}
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
			}))
			got, err := client.Live(context.Background())
			if size == AgentMaxHeaderBytes {
				if err != nil || got.Status != "live" {
					t.Fatalf("exact response header = %+v, %v", got, err)
				}
				return
			}
			if got != (LiveResponse{}) || !errors.Is(err, ErrAgentUnavailable) {
				t.Fatalf("oversized response header = %+v, %v", got, err)
			}
		})
	}
}

func TestAgentClientRequiresExactJSONContentType(t *testing.T) {
	for _, contentType := range []string{"", "Application/JSON", "application/json; charset=utf-8", "text/json"} {
		contentType := contentType
		t.Run(strings.ReplaceAll(contentType, "/", "-"), func(t *testing.T) {
			client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if contentType != "" {
					w.Header().Set("Content-Type", contentType)
				}
				_, _ = w.Write([]byte(`{"status":"live"}`))
			}))
			got, err := client.Live(context.Background())
			if got != (LiveResponse{}) || !errors.Is(err, ErrAgentUnavailable) {
				t.Fatalf("Content-Type %q = %+v, %v", contentType, got, err)
			}
		})
	}
}

func TestAgentClientTransportIsOwnedAndDisablesProxyCompressionRetry(t *testing.T) {
	var injectedCalls atomic.Int32
	injected := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		injectedCalls.Add(1)
		return nil, errors.New("injected transport must not run")
	})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Accept-Encoding"); got != "identity" {
			t.Errorf("Accept-Encoding = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"live"}`))
	}))
	defer server.Close()
	client, err := NewAgentClient(AgentServiceSettings{Endpoint: server.URL, APIKeyEnv: "OWNED_KEY"}, "owned-secret", injected)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.http.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T", client.http.Transport)
	}
	if transport.Proxy != nil || !transport.DisableCompression || !transport.DisableKeepAlives || transport.MaxResponseHeaderBytes != AgentMaxHeaderBytes {
		t.Fatalf("owned transport invariants = %+v", transport)
	}
	if got, err := client.Live(context.Background()); err != nil || got.Status != "live" {
		t.Fatalf("Live = %+v, %v", got, err)
	}
	client.Close()
	if injectedCalls.Load() != 0 {
		t.Fatalf("injected transport calls = %d", injectedCalls.Load())
	}
}

func TestAgentClientDoesNotRetryBrokenGETTransport(t *testing.T) {
	var calls atomic.Int32
	client := newManifestAgentClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := calls.Add(1)
		if call == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("response writer cannot hijack")
				return
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			_ = connection.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"live"}`))
	}))
	got, err := client.Live(context.Background())
	if got != (LiveResponse{}) || !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("broken GET = %+v, %v", got, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("GET attempts = %d, want 1", calls.Load())
	}
}

func TestAgentClientCloseCancelsInflightAndIsConcurrentIdempotent(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	client := newManifestAgentClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	}))
	done := make(chan struct {
		response AcquireResponse
		err      error
	}, 1)
	go func() {
		response, err := client.Acquire(context.Background(), validAgentAcquireRequest())
		done <- struct {
			response AcquireResponse
			err      error
		}{response: response, err: err}
	}()
	<-started
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			client.Close()
		}()
	}
	group.Wait()
	result := <-done
	close(release)
	if result.response != (AcquireResponse{}) || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("closed acquire = %+v, %v", result.response, result.err)
	}
}

func TestAgentClientCallerDeadlineCancelsInflightWithZeroValue(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	client := newManifestAgentClient(t, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan struct {
		response AcquireResponse
		err      error
	}, 1)
	go func() {
		response, err := client.Acquire(ctx, validAgentAcquireRequest())
		done <- struct {
			response AcquireResponse
			err      error
		}{response: response, err: err}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("request did not become in-flight before caller deadline")
	}
	result := <-done
	close(release)
	if result.response != (AcquireResponse{}) || !errors.Is(result.err, context.DeadlineExceeded) {
		t.Fatalf("deadline acquire = %+v, %v", result.response, result.err)
	}
}

func TestAgentClientNormalCallsDoNotAccumulateLifetimeGoroutines(t *testing.T) {
	client := newManifestAgentClient(t, fixedAgentResponseHandler(http.StatusOK, []byte(`{"status":"live"}`)))
	baseline := runtime.NumGoroutine()
	for range 64 {
		got, err := client.Live(context.Background())
		if err != nil || got.Status != "live" {
			t.Fatalf("Live = %+v, %v", got, err)
		}
	}
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	if got := runtime.NumGoroutine(); got > baseline+8 {
		t.Fatalf("goroutines grew from %d to %d after normal calls", baseline, got)
	}
}

func TestAgentClientFormattingNeverExposesCredential(t *testing.T) {
	const secret = "format-secret-never-print"
	client := newManifestAgentClientWithCredential(t, secret, fixedAgentResponseHandler(http.StatusOK, []byte(`{"status":"live"}`)))
	for _, formatted := range []string{
		fmt.Sprintf("%v", client),
		fmt.Sprintf("%+v", client),
		fmt.Sprintf("%#v", client),
	} {
		if strings.Contains(formatted, secret) {
			t.Fatalf("formatted client leaked credential: %q", formatted)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func validAgentAcquireRequest() AcquireRequest {
	return AcquireRequest{
		ContractVersion:  AgentContractVersion,
		RequestID:        agentRequestID,
		ClientInstanceID: agentClientID,
		NamespaceID:      agentNamespace,
	}
}

func validAgentAcquireResponseJSON() []byte {
	return []byte(`{"contract_version":"v1","request_id":"` + agentRequestID + `","client_instance_id":"` + agentClientID + `","namespace_id":"` + agentNamespace + `","lease_id":"` + agentLeaseID + `","lease_expires_in_ms":15000}`)
}

func newManifestAgentClientWithCredential(t *testing.T, credential string, handler http.Handler) *AgentClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewAgentClient(AgentServiceSettings{Endpoint: server.URL, APIKeyEnv: "FORMAT_KEY"}, credential, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}
