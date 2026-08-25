package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeCollector 是注入 handler 的假采集器：返回固定 Status，便于做确定性断言。
type fakeCollector struct {
	status Status
}

// Collect 返回构造时给定的固定状态。
func (f *fakeCollector) Collect(ctx context.Context) Status { return f.status }

// TestStatusEndpoint 验证 GET /api/status 返回 200、JSON 关键字段存在且切片非 null。
func TestStatusEndpoint(t *testing.T) {
	fixed := Status{
		GeneratedAt: "2026-08-25T00:00:00Z",
		Root:        "/repo",
		Agents:      []AgentStatus{},
		Chains:      []ChainStatus{},
		Tasks:       []BacklogTask{},
		Worktrees:   []WorktreeStatus{},
		Confirm:     []ConfirmCard{},
		PRs:         []PRStatus{},
		Logs:        map[string][]string{},
		Errors:      map[string]string{},
	}
	h := newStatusHandler(&fakeCollector{status: fixed})
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	// 关键 JSON 键存在。
	body := rr.Body.String()
	for _, key := range []string{"generatedAt", "root", "agents", "chains", "tasks", "worktrees", "confirm", "prs", "logs", "errors"} {
		if !strings.Contains(body, `"`+key+`"`) {
			t.Errorf("响应缺少键 %q", key)
		}
	}
	// 能反解回 Status，且各切片保持空数组而非 null。
	var st Status
	if err := json.Unmarshal(rr.Body.Bytes(), &st); err != nil {
		t.Fatalf("反解 JSON 失败：%v", err)
	}
	if st.GeneratedAt != fixed.GeneratedAt || st.Root != "/repo" {
		t.Errorf("generatedAt=%q root=%q", st.GeneratedAt, st.Root)
	}
	if st.Agents == nil || st.Chains == nil || st.Tasks == nil || st.Worktrees == nil || st.Confirm == nil {
		t.Errorf("各切片不应为 null（应保持空数组）")
	}
	if st.PRs == nil {
		t.Errorf("prs 在可用时应为非 null 数组")
	}
	if st.Logs == nil || st.Errors == nil {
		t.Errorf("logs/errors 应为非 nil map")
	}
}

// TestStatusEndpointPRsDown 验证 gh 不可用时 prs 为 null、errors.prs 有说明，但请求仍 200。
func TestStatusEndpointPRsDown(t *testing.T) {
	fixed := Status{
		GeneratedAt: "2026-08-25T00:00:00Z",
		Root:        "/repo",
		Agents:      []AgentStatus{},
		Tasks:       []BacklogTask{},
		PRs:         nil, // gh 不可用
		Errors:      map[string]string{"prs": "gh 未登录"},
	}
	h := newStatusHandler(&fakeCollector{status: fixed})
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("gh 降级也应返回 200，得到 %d", rr.Code)
	}
	var st Status
	if err := json.Unmarshal(rr.Body.Bytes(), &st); err != nil {
		t.Fatalf("反解失败：%v", err)
	}
	if st.PRs != nil {
		t.Errorf("prs 应为 null，得到 %#v", st.PRs)
	}
	if st.Errors["prs"] != "gh 未登录" {
		t.Errorf("errors.prs = %q", st.Errors["prs"])
	}
}

// TestIndexEndpoint 验证 GET / 返回 200、text/html 且包含看板标题。
func TestIndexEndpoint(t *testing.T) {
	h := newStatusHandler(&fakeCollector{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "Mornlea Agent 执行看板") {
		t.Errorf("页面应包含标题「Mornlea Agent 执行看板」")
	}
}

// TestIndexMethodNotAllowed 验证非 GET/HEAD 请求 / 被拒绝。
func TestIndexMethodNotAllowed(t *testing.T) {
	h := newStatusHandler(&fakeCollector{})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST / 应返回 405，得到 %d", rr.Code)
	}
}
