package main

// 本文件主题：页面服务完整性——从 dist 目录读盘提供前端产物，dist 缺失时返回「前端未构建」指引页。
//
// 重构后看板不再内嵌 HTML，而是由本包 web/ 前端的构建产物（dist/）提供。
// 这里注入 t.TempDir() 的假 dist，分别验证「dist 缺失→指引页」与「dist 就绪→逐文件读盘」两类契约。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestServeIndexDistMissing 验证 dist 缺失（空目录，无 index.html）时 GET / 返回 200，
// 且是「前端未构建」指引页，提示使用者先运行 make agent-dashboard。
func TestServeIndexDistMissing(t *testing.T) {
	dist := t.TempDir() // 空目录：没有 index.html
	h := newStatusHandlerWithDist(&fakeCollector{}, dist)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dist 缺失也应返回 200，得到 %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "make agent-dashboard") {
		t.Errorf("指引页应包含文案 «make agent-dashboard»")
	}
}

// TestServeIndexFromDist 验证 dist 就绪时 GET / 返回 dist/index.html 内容，
// 且 /assets/<file> 能读盘命中真实产物文件。
func TestServeIndexFromDist(t *testing.T) {
	dist := t.TempDir()
	index := "<!DOCTYPE html><html><head><title>Mornlea Agent 执行看板</title></head><body><div id=\"root\">前端构建产物</div></body></html>"
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte(index), 0o644); err != nil {
		t.Fatalf("写 index.html 失败：%v", err)
	}
	assets := filepath.Join(dist, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatalf("mkdir assets 失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(assets, "app.js"), []byte("console.log('app')"), 0o644); err != nil {
		t.Fatalf("写 app.js 失败：%v", err)
	}

	h := newStatusHandlerWithDist(&fakeCollector{}, dist)

	// GET / 返回 dist/index.html 内容。
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET / = %d，想要 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "<div id=\"root\">") {
		t.Errorf("GET / 应返回 dist/index.html 内容，实际：%s", rr.Body.String())
	}

	// GET /assets/app.js 返回该文件内容。
	req2 := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("GET /assets/app.js = %d，想要 200", rr2.Code)
	}
	if !strings.Contains(rr2.Body.String(), "console.log('app')") {
		t.Errorf("GET /assets/app.js 应返回文件内容，实际：%s", rr2.Body.String())
	}
}

// TestServeIndexMethodNotAllowed 验证非 GET/HEAD 请求 / 被拒绝（沿用既有端点语义）。
func TestServeIndexMethodNotAllowed(t *testing.T) {
	h := newStatusHandlerWithDist(&fakeCollector{}, t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST / 应返回 405，得到 %d", rr.Code)
	}
}

// TestServeAssetsDirNotFound 验证 /assets/ 的目录请求与穿越请求返回 404（绝不暴露目录列表），
// 且真实文件仍返回 200。
func TestServeAssetsDirNotFound(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html><body>index</body></html>"), 0o644); err != nil {
		t.Fatalf("写 index.html 失败：%v", err)
	}
	assets := filepath.Join(dist, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatalf("mkdir assets 失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(assets, "app.js"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写 app.js 失败：%v", err)
	}
	h := newStatusHandlerWithDist(&fakeCollector{}, dist)

	// 目录请求：/assets/（整除目录）与 /assets（不匹配前缀）都应 404。
	for _, p := range []string{"/assets/", "/assets"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d，想要 404", p, rr.Code)
		}
	}
	// 穿越请求应 404。
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/assets/../index.html", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /assets/../index.html = %d，想要 404", rr.Code)
	}
	// 真实文件仍 200。
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rr2.Code != http.StatusOK {
		t.Errorf("GET /assets/app.js = %d，想要 200", rr2.Code)
	}
}
