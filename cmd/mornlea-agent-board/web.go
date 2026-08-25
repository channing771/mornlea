package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
)

// dashboardHTML 是嵌入的单页看板，通过 //go:embed 打进二进制。
//
//go:embed dashboard.html
var dashboardHTML []byte

// Collector 抽象看板的数据采集器。真实实现 liveCollector 在 collect.go；
// 测试中可注入固定 Status 的假实现，从而对 /api/status 做确定性断言。
type Collector interface {
	// Collect 采集当前状态；实现必须 best-effort，失败只写入 Errors。
	Collect(ctx context.Context) Status
}

// statusHandler 处理 / 与 /api/status 两个路由，携带注入的 Collector。
type statusHandler struct {
	// collector 为注入的数据采集器，不可为 nil（由 newStatusHandler 保证）。
	collector Collector
}

// newStatusHandler 构造看板处理器。
func newStatusHandler(collector Collector) http.Handler {
	return &statusHandler{collector: collector}
}

// ServeHTTP 按路径分发：/ 返回内嵌看板页，/api/status 返回 JSON 状态。
func (h *statusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		h.serveIndex(w, r)
	case "/api/status":
		h.serveStatus(w, r)
	default:
		http.NotFound(w, r)
	}
}

// serveIndex 返回内嵌的 dashboard.html（仅允许 GET/HEAD）。
func (h *statusHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET")
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(dashboardHTML)
}

// serveStatus 采集当前状态并以 JSON 返回；采集器即便失败也绝不返回 5xx。
func (h *statusHandler) serveStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if h.collector == nil {
		// 防御：不注入采集器时给出明确错误，但仍返回 200。
		st := Status{
			Errors: map[string]string{"global": "采集器未注入（internal error）"},
		}
		_ = json.NewEncoder(w).Encode(st)
		return
	}
	st := h.collector.Collect(r.Context())
	_ = json.NewEncoder(w).Encode(st)
}
