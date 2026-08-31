package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/companion"
)

const (
	companionMCPRequestBytes  = 256 << 10
	companionMCPResponseBytes = 160 << 10
)

type companionMCPLeaseContextKey struct{}

type companionMCPEnvelope struct {
	ID     json.RawMessage
	Method string
}

type companionMCPSnapshotAccess interface {
	Authorize(string) (companion.SnapshotAuthorization, error)
	Materialize(companion.SnapshotAuthorization) (companion.SnapshotLease, error)
}

type companionMCPOuterHandler struct {
	authority string
	origin    string
	snapshots companionMCPSnapshotAccess
	next      http.Handler
}

func newCompanionMCPOuterHandler(authority string, snapshots companionMCPSnapshotAccess, next http.Handler) http.Handler {
	return &companionMCPOuterHandler{
		authority: authority,
		origin:    "http://" + authority,
		snapshots: snapshots,
		next:      next,
	}
}

func (h *companionMCPOuterHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL == nil || request.URL.EscapedPath() != "/mcp" || request.URL.RawQuery != "" {
		writeCompanionMCPError(writer, http.StatusNotFound, "not_found")
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeCompanionMCPError(writer, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if request.Host != h.authority {
		writeCompanionMCPError(writer, http.StatusForbidden, "forbidden")
		return
	}
	origins := request.Header.Values("Origin")
	if len(origins) > 1 || (len(origins) == 1 && origins[0] != h.origin) {
		writeCompanionMCPError(writer, http.StatusForbidden, "forbidden")
		return
	}
	authorizations := request.Header.Values("Authorization")
	if len(authorizations) != 1 || !strings.HasPrefix(authorizations[0], "Bearer ") {
		writeCompanionMCPError(writer, http.StatusUnauthorized, "unauthorized")
		return
	}
	capability := strings.TrimPrefix(authorizations[0], "Bearer ")
	if capability == "" || len(capability) > 512 || strings.ContainsAny(capability, " \t\r\n") {
		writeCompanionMCPError(writer, http.StatusUnauthorized, "unauthorized")
		return
	}
	authorization, err := h.snapshots.Authorize(capability)
	if err != nil {
		writeCompanionMCPError(writer, http.StatusUnauthorized, "unauthorized")
		return
	}
	contentTypes := request.Header.Values("Content-Type")
	if len(contentTypes) != 1 {
		writeCompanionMCPError(writer, http.StatusUnsupportedMediaType, "invalid_content_type")
		return
	}
	mediaType, _, err := mime.ParseMediaType(contentTypes[0])
	if err != nil || mediaType != "application/json" {
		writeCompanionMCPError(writer, http.StatusUnsupportedMediaType, "invalid_content_type")
		return
	}
	if request.ContentLength > companionMCPRequestBytes {
		writeCompanionMCPError(writer, http.StatusRequestEntityTooLarge, "request_too_large")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, companionMCPRequestBytes+1))
	if err != nil || len(body) > companionMCPRequestBytes {
		writeCompanionMCPError(writer, http.StatusRequestEntityTooLarge, "request_too_large")
		return
	}
	if !utf8.Valid(body) {
		writeCompanionMCPError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	envelope, err := parseCompanionMCPEnvelope(body, request.Header.Values("Mcp-Protocol-Version"))
	if err != nil {
		writeCompanionMCPError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	if request.Context().Err() != nil {
		writeCompanionMCPError(writer, http.StatusServiceUnavailable, "unavailable")
		return
	}
	lease, err := h.snapshots.Materialize(authorization)
	if err != nil || !companionMCPRequestAlive(request.Context(), lease) {
		writeCompanionMCPError(writer, http.StatusServiceUnavailable, "unavailable")
		return
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request = request.WithContext(context.WithValue(request.Context(), companionMCPLeaseContextKey{}, lease))
	recorder := newBoundedMCPResponseRecorder(companionMCPResponseBytes)
	h.next.ServeHTTP(recorder, request)
	if recorder.overflow || !companionMCPRequestAlive(request.Context(), lease) {
		writeCompanionMCPError(writer, http.StatusBadGateway, "unavailable")
		return
	}
	normalized, err := validateAndNormalizeCompanionMCPResponse(envelope.Method, recorder, companionMCPResponseBytes)
	if err != nil || !companionMCPRequestAlive(request.Context(), lease) {
		writeCompanionMCPError(writer, http.StatusBadGateway, "unavailable")
		return
	}
	if envelope.Method == "notifications/initialized" {
		writer.WriteHeader(recorder.statusCode())
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(recorder.statusCode())
	_, _ = writer.Write(normalized)
}

func companionMCPRequestAlive(ctx context.Context, lease companion.SnapshotLease) bool {
	return ctx.Err() == nil && lease.Checkpoint() == nil
}

func parseCompanionMCPEnvelope(body []byte, protocolHeaders []string) (companionMCPEnvelope, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return companionMCPEnvelope{}, fmt.Errorf("MCP envelope 非 object")
	}
	if err := validateCompanionMCPJSONShape(trimmed); err != nil {
		return companionMCPEnvelope{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return companionMCPEnvelope{}, err
	}
	for field := range fields {
		switch field {
		case "jsonrpc", "id", "method", "params":
		default:
			return companionMCPEnvelope{}, fmt.Errorf("MCP envelope 未知字段")
		}
	}
	var jsonRPC, method string
	if err := json.Unmarshal(fields["jsonrpc"], &jsonRPC); err != nil || jsonRPC != "2.0" {
		return companionMCPEnvelope{}, fmt.Errorf("MCP jsonrpc 非法")
	}
	if err := json.Unmarshal(fields["method"], &method); err != nil || method == "" {
		return companionMCPEnvelope{}, fmt.Errorf("MCP method 非法")
	}
	allowed := method == "initialize" || method == "notifications/initialized" || method == "tools/list" || method == "tools/call"
	if !allowed {
		return companionMCPEnvelope{}, fmt.Errorf("MCP method 未允许")
	}
	id, hasID := fields["id"]
	if method == "notifications/initialized" {
		if hasID {
			return companionMCPEnvelope{}, fmt.Errorf("MCP notification 带 id")
		}
	} else if !hasID || !validCompanionMCPRequestID(id) {
		return companionMCPEnvelope{}, fmt.Errorf("MCP request id 非法")
	}
	params, hasParams := fields["params"]
	if method != "tools/list" && !hasParams {
		return companionMCPEnvelope{}, fmt.Errorf("MCP params 缺失")
	}
	if hasParams {
		trimmedParams := bytes.TrimSpace(params)
		if len(trimmedParams) == 0 || trimmedParams[0] != '{' {
			return companionMCPEnvelope{}, fmt.Errorf("MCP params 非 object")
		}
	}
	if method == "initialize" {
		if len(protocolHeaders) > 1 || (len(protocolHeaders) == 1 && protocolHeaders[0] != "2025-11-25") {
			return companionMCPEnvelope{}, fmt.Errorf("MCP initialize header version 非法")
		}
		var initialize struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(params, &initialize); err != nil || initialize.ProtocolVersion != "2025-11-25" {
			return companionMCPEnvelope{}, fmt.Errorf("MCP initialize params version 非法")
		}
	} else if len(protocolHeaders) != 1 || protocolHeaders[0] != "2025-11-25" {
		return companionMCPEnvelope{}, fmt.Errorf("MCP protocol header 缺失或非法")
	}
	return companionMCPEnvelope{ID: append(json.RawMessage(nil), id...), Method: method}, nil
}

func validCompanionMCPRequestID(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	switch id := value.(type) {
	case string:
		return id != ""
	case json.Number:
		_, err := id.Int64()
		return err == nil
	default:
		return false
	}
}

type boundedMCPResponseRecorder struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	limit    int
	overflow bool
}

func newBoundedMCPResponseRecorder(limit int) *boundedMCPResponseRecorder {
	return &boundedMCPResponseRecorder{header: make(http.Header), limit: limit}
}

func (r *boundedMCPResponseRecorder) Header() http.Header { return r.header }

func (r *boundedMCPResponseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *boundedMCPResponseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	remaining := r.limit + 1 - r.body.Len()
	if remaining > 0 {
		_, _ = r.body.Write(data[:min(len(data), remaining)])
	}
	if r.body.Len() > r.limit || len(data) > remaining {
		r.overflow = true
	}
	return len(data), nil
}

func (r *boundedMCPResponseRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func validateAndNormalizeCompanionMCPResponse(method string, recorder *boundedMCPResponseRecorder, limit int) ([]byte, error) {
	if len(recorder.header.Values("Mcp-Session-Id")) != 0 {
		return nil, fmt.Errorf("MCP response 带 session")
	}
	if method == "notifications/initialized" {
		if recorder.statusCode() != http.StatusAccepted || recorder.body.Len() != 0 || len(recorder.header.Values("Content-Type")) != 0 {
			return nil, fmt.Errorf("MCP notification response 非空")
		}
		return nil, nil
	}
	contentTypes := recorder.header.Values("Content-Type")
	if len(contentTypes) != 1 {
		return nil, fmt.Errorf("MCP response Content-Type 非唯一")
	}
	mediaType, _, err := mime.ParseMediaType(contentTypes[0])
	if err != nil || mediaType != "application/json" || strings.Contains(strings.ToLower(contentTypes[0]), "text/event-stream") {
		return nil, fmt.Errorf("MCP response Content-Type 非 JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(recorder.body.Bytes()))
	decoder.UseNumber()
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil || envelope == nil {
		return nil, fmt.Errorf("MCP response 非 JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("MCP response 存在尾随 JSON")
	}
	if _, hasError := envelope["error"]; hasError {
		if _, hasResult := envelope["result"]; hasResult {
			return nil, fmt.Errorf("MCP response 同时包含 result/error")
		}
		envelope["error"] = map[string]any{"code": -32603, "message": "unavailable"}
	}
	result, hasResult := envelope["result"].(map[string]any)
	if hasResult && method == "initialize" {
		result["capabilities"] = map[string]any{"tools": map[string]any{"listChanged": false}}
	}
	if hasResult && method == "tools/list" {
		rawTools, ok := result["tools"].([]any)
		if !ok {
			return nil, fmt.Errorf("MCP tools/list result 非法")
		}
		order := companion.PlanningToolNames()
		byName := make(map[string]any, len(rawTools))
		for _, rawTool := range rawTools {
			tool, ok := rawTool.(map[string]any)
			name, nameOK := tool["name"].(string)
			if !ok || !nameOK {
				return nil, fmt.Errorf("MCP tool entry 非法")
			}
			byName[name] = rawTool
		}
		ordered := make([]any, 0, len(order))
		for _, name := range order {
			tool, ok := byName[name]
			if !ok {
				return nil, fmt.Errorf("MCP tool %q 缺失", name)
			}
			ordered = append(ordered, tool)
		}
		if len(ordered) != len(rawTools) {
			return nil, fmt.Errorf("MCP tool 集合漂移")
		}
		result["tools"] = ordered
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(envelope); err != nil {
		return nil, err
	}
	normalized := bytes.TrimSuffix(output.Bytes(), []byte{'\n'})
	if len(normalized) > limit {
		return nil, fmt.Errorf("MCP response 超限")
	}
	return normalized, nil
}

func writeCompanionMCPError(writer http.ResponseWriter, status int, code string) {
	body := []byte(`{"error":{"code":"` + code + `"}}`)
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(body)
}

func validateCompanionMCPJSONShape(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumeCompanionMCPJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("JSON 存在尾随 token")
	}
	return nil
}

func consumeCompanionMCPJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key 非字符串")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("JSON object key 重复")
			}
			seen[key] = struct{}{}
			if err := consumeCompanionMCPJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := consumeCompanionMCPJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("JSON delimiter 非法")
	}
}
