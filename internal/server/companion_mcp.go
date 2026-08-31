package server

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newCompanionMCPHandler(authority string, registry *companion.SnapshotRegistry) (http.Handler, error) {
	sdkHandler, err := newCompanionMCPSDKHandler()
	if err != nil {
		return nil, err
	}
	return newCompanionMCPOuterHandler(authority, registry, sdkHandler), nil
}

func newCompanionMCPSDKHandler() (http.Handler, error) {
	contract, err := loadCompanionMCPContract()
	if err != nil {
		return nil, err
	}
	server := mcp.NewServer(
		&mcp.Implementation{Name: "mornlea-companion-agent-mcp", Version: "v1"},
		&mcp.ServerOptions{
			Capabilities: &mcp.ServerCapabilities{
				Tools: &mcp.ToolCapabilities{ListChanged: false},
			},
		},
	)
	for _, contractTool := range contract.Tools {
		tool := contractTool
		server.AddTool(&mcp.Tool{
			Name:         tool.Name,
			InputSchema:  tool.InputSchema,
			OutputSchema: tool.OutputSchema,
		}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			lease, ok := ctx.Value(companionMCPLeaseContextKey{}).(companion.SnapshotLease)
			if !ok || request == nil || request.Params == nil || request.Params.Name != tool.Name {
				return companionMCPToolError(), nil
			}
			result, err := companion.ExecutePlanningTool(ctx, lease, tool.Name, request.Params.Arguments)
			if err != nil {
				return companionMCPToolError(), nil
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: string(result.Canonical)},
				},
				StructuredContent: result.Structured,
			}, nil
		})
	}
	sdkHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			Stateless:                    true,
			JSONResponse:                 true,
			MaxRequestBodyBytes:          int64(contract.RequestBodyBytes),
			PropagateRequestCancellation: true,
		},
	)
	return sdkHandler, nil
}

func companionMCPToolError() *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: `{"code":"unavailable"}`},
		},
		IsError: true,
	}
}

type companionMCPService struct {
	registry  *companion.SnapshotRegistry
	listener  net.Listener
	server    *http.Server
	endpoint  string
	done      chan error
	closing   atomic.Bool
	closeOnce sync.Once
}

type companionMCPServiceFactory func(*companion.SnapshotRegistry) (*companionMCPService, error)
type companionMCPListenFunc func(string, string) (net.Listener, error)
type companionMCPHandlerFactory func(string, *companion.SnapshotRegistry) (http.Handler, error)

func newCompanionMCPService(registry *companion.SnapshotRegistry) (*companionMCPService, error) {
	return newCompanionMCPServiceWithDependencies(registry, net.Listen, newCompanionMCPHandler)
}

func newCompanionMCPServiceWithDependencies(
	registry *companion.SnapshotRegistry,
	listen companionMCPListenFunc,
	newHandler companionMCPHandlerFactory,
) (*companionMCPService, error) {
	if registry == nil {
		return nil, errors.New("server: nil companion MCP registry")
	}
	if listen == nil || newHandler == nil {
		return nil, errors.New("server: nil companion MCP dependency")
	}
	listener, err := listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	authority := listener.Addr().String()
	handler, err := newHandler(authority, registry)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	service := &companionMCPService{
		registry: registry,
		listener: listener,
		server: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       35 * time.Second,
			WriteTimeout:      35 * time.Second,
			IdleTimeout:       5 * time.Second,
			MaxHeaderBytes:    16 << 10,
			ErrorLog:          log.New(io.Discard, "", 0),
		},
		endpoint: "http://" + authority + "/mcp",
		done:     make(chan error, 1),
	}
	go func() {
		err := service.server.Serve(listener)
		if service.closing.Load() && (errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed)) {
			err = nil
		}
		service.done <- err
		close(service.done)
	}()
	return service, nil
}

// Endpoint 返回使用实际 loopback IP literal authority 的 `/mcp` URL。
func (s *companionMCPService) Endpoint() string { return s.endpoint }

// Done 返回 Serve 的单次退出结果；意外错误只结束 MCP，不影响世界 runtime。
func (s *companionMCPService) Done() <-chan error { return s.done }

// Close 幂等关闭 listener、HTTP server 与 registry，且不等待在途 handler。
func (s *companionMCPService) Close() {
	s.closeOnce.Do(func() {
		s.closing.Store(true)
		s.registry.Close()
		_ = s.server.Close()
		_ = s.listener.Close()
	})
}
