package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// guidancePageHTML 是前端产物缺失时返回的指引页（纯内联样式、全中文、零外部资源）。
//
// 该页仅在 dist 目录为空或缺少 index.html 时下发，帮助使用者在开发环境定位
// 「尚未构建前端」这一最常见启动误区；返回 200 而非 5xx，保证看板入口永远可响应。
const guidancePageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mornlea Agent 执行看板</title>
<style>
  body{margin:0;background:#0f1115;color:#d8dee8;font-family:-apple-system,'PingFang SC','Microsoft YaHei',sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh}
  .box{max-width:560px;padding:32px 36px;background:#171a21;border:1px solid #2a303c;border-radius:12px}
  h1{margin:0 0 12px;font-size:20px;color:#4f8cff;font-weight:600}
  p{margin:8px 0;line-height:1.6;color:#aab2c0}
  code{background:#1d222b;border:1px solid #2a303c;border-radius:6px;padding:2px 8px;color:#7fb0ff;font-size:13px}
</style>
</head>
<body>
<div class="box">
  <h1>Mornlea Agent 执行看板</h1>
  <p>前端未构建：请运行 <code>make agent-dashboard</code>（会先 <code>corepack pnpm install --frozen-lockfile</code> 与 <code>corepack pnpm run build</code>）。</p>
  <p>也可以只启动前端开发服务器：<code>make agent-ui-dev</code>（Vite 会把 <code>/api</code> 代理到本服务）。</p>
</div>
</body>
</html>
`

// Collector 抽象看板的数据采集器。真实实现 liveCollector 在 collect.go；
// 测试中可注入固定 Status 的假实现，从而对 /api/status 做确定性断言。
type Collector interface {
	// Collect 采集当前状态；实现必须 best-effort，失败只写入 Errors。
	Collect(ctx context.Context) Status
}

// statusHandler 处理 /、/assets/* 与 /api/status 三个路由，携带注入的 Collector
// 与前端产物目录 distDir。distDir 为空或缺 index.html 时，/ 回退到指引页。
type statusHandler struct {
	// collector 为注入的数据采集器，不可为 nil（由 newStatusHandler 系列保证）。
	collector Collector
	// distDir 为前端构建产物目录（packages/tools/agent-board/web/dist）；为空表示未构建。
	distDir string
}

// newStatusHandler 构造看板处理器，distDir 为空（/ 返回「前端未构建」指引页）。
// 该便利签名仅用于 web_test.go 的既有断言（distDir 为空时 / 返回指引页）；
// 生产路径由 newStatusHandlerWithDist 传入真实的 dist 目录。
func newStatusHandler(collector Collector) http.Handler {
	return &statusHandler{collector: collector}
}

// newStatusHandlerWithDist 构造看板处理器，distDir 为前端构建产物目录；
// main.go 用 <root>/packages/tools/agent-board/web/dist 传入。
func newStatusHandlerWithDist(collector Collector, distDir string) http.Handler {
	return &statusHandler{collector: collector, distDir: distDir}
}

// ServeHTTP 按路径分发：/ 返回 index.html（或指引页）、/assets/* 读盘静态产物、
// /api/status 返回 JSON 状态。
func (h *statusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/status":
		h.serveStatus(w, r)
	case r.URL.Path == "/":
		h.serveIndex(w, r)
	case strings.HasPrefix(r.URL.Path, "/assets/"):
		h.serveAssets(w, r)
	default:
		http.NotFound(w, r)
	}
}

// serveIndex 返回 dist/index.html；dist 缺失或无 index.html 时返回指引页。
// 仅允许 GET/HEAD。
func (h *statusHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET")
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	if h.distDir != "" {
		if data, err := os.ReadFile(filepath.Join(h.distDir, "index.html")); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(guidancePageHTML))
}

// serveAssets 把 /assets/<file> 映射到 distDir/assets/<file>，由 http.FileServer
// 直接读盘；仅允许 GET/HEAD。对空路径与目录请求一律 404，绝不暴露目录列表。
func (h *statusHandler) serveAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET")
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	if h.distDir == "" {
		http.NotFound(w, r)
		return
	}
	// 相对路径去掉 /assets/ 前缀；空路径（/assets/）与携带 .. 的穿越请求直接 404。
	rel := strings.TrimPrefix(r.URL.Path, "/assets/")
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "..") {
		http.NotFound(w, r)
		return
	}
	// 只允许普通文件：目标为目录（或不存在的路径）时返回 404，避免目录列表泄漏。
	full := filepath.Join(h.distDir, "assets", filepath.FromSlash(rel))
	if info, err := os.Stat(full); err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	// StripPrefix 去掉 /assets/ 前缀，交给以 distDir/assets 为根的 FileServer，
	// 使 /assets/<file> 精确命中 distDir/assets/<file>。
	http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(h.distDir, "assets")))).ServeHTTP(w, r)
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
