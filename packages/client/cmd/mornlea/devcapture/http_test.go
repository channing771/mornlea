//go:build darwin

package devcapture

import (
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
)

// TestScreenshotSuccessReturnsPNG 钉住成功路径：200、`image/png`、解码尺寸
// 与交付帧一致。
func TestScreenshotSuccessReturnsPNG(t *testing.T) {
	s := newTestService()
	go pumpOnce(s, app.CaptureOutcome{Pixels: testFramePixels(), Width: 2, Height: 1})
	rr := serveToRecorder(s, "/screenshot")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200（body：%s）", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q，想要 image/png", ct)
	}
	img, err := png.Decode(rr.Body)
	if err != nil {
		t.Fatalf("响应不是合法 PNG：%v", err)
	}
	if img.Bounds() != (image.Rect(0, 0, 2, 1)) {
		t.Errorf("PNG 尺寸 = %v，想要 2×1", img.Bounds())
	}
}

// TestScreenshotUnavailableReturnsStable503 钉住捕获不可用映射：503 + 稳定
// 中文错误 JSON，绝不返回无关内容冒充画面。
func TestScreenshotUnavailableReturnsStable503(t *testing.T) {
	s := newTestService()
	go pumpOnce(s, app.CaptureOutcome{Err: client.ErrCaptureUnavailable})
	rr := serveToRecorder(s, "/screenshot")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d，想要 503", rr.Code)
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("错误响应应为 JSON：%v（body：%s）", err, rr.Body.String())
	}
	if !strings.Contains(payload.Error, "屏幕录制") || !strings.Contains(payload.Error, "不可用") {
		t.Errorf("错误文案缺少稳定要素：%q", payload.Error)
	}
}

// TestScreenshotTimesOutWhenPumpSilent 钉住帧循环停止后的有界失败：无人应答
// 时截图请求必须在等待上限内以 503 收敛，不永久挂起。
func TestScreenshotTimesOutWhenPumpSilent(t *testing.T) {
	s := newTestService()
	s.frameWait = 30 * time.Millisecond
	rr := serveToRecorder(s, "/screenshot")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d，想要 503", rr.Code)
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("错误响应应为 JSON：%v", err)
	}
	if !strings.Contains(payload.Error, "超时") {
		t.Errorf("超时文案不符：%q", payload.Error)
	}
	// 超时后残留的请求仍在通道里：迟到的泵仍可取出交付（自愈路径），服务
	// 不会吞掉这个请求导致帧循环侧悬挂。
	if req, ok := s.PendingCapture(); !ok || req.Done == nil {
		t.Error("超时后残留的待办请求应仍可被泵取出")
	}
}

// TestScreenshotRejectsNonGet 钉住方法约束：mux 的方法模式把非 GET 拒为 405。
func TestScreenshotRejectsNonGet(t *testing.T) {
	s := newTestService()
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/screenshot", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /screenshot 状态码 = %d，想要 405", rr.Code)
	}
}

// fakeStatus 是注入 `/status` 的假状态源：返回固定 phase 与窗口尺寸。
type fakeStatus struct {
	phase string
	width int
	high  int
}

func (f fakeStatus) Phase() string     { return f.phase }
func (f fakeStatus) WindowWidth() int  { return f.width }
func (f fakeStatus) WindowHeight() int { return f.high }

// decodeStatusJSON 把 `/status` 响应反解回断言用的结构。
func decodeStatusJSON(t *testing.T, body []byte) statusReport {
	t.Helper()
	var report statusReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("反解 /status JSON 失败：%v（body：%s）", err, body)
	}
	return report
}

// TestStatusReportsIdentityAndState 钉住 `/status` 的字段面：pid、注入源的
// phase 与窗口尺寸、录制状态、端口与最近捕获摘要。
func TestStatusReportsIdentityAndState(t *testing.T) {
	s := New(Options{Status: fakeStatus{phase: "menu", width: 1280, high: 720}})
	rr := serveToRecorder(s, "/status")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	report := decodeStatusJSON(t, rr.Body.Bytes())
	if report.PID != os.Getpid() {
		t.Errorf("pid = %d，想要 %d", report.PID, os.Getpid())
	}
	if report.Phase != "menu" {
		t.Errorf("phase = %q，想要 menu", report.Phase)
	}
	if report.Window == nil || report.Window.Width != 1280 || report.Window.Height != 720 {
		t.Errorf("window = %+v，想要 1280×720", report.Window)
	}
	if report.Recording {
		t.Error("空闲时 recording 应为 false")
	}
	if report.LastCapture != nil {
		t.Errorf("未发生捕获时 last_capture 应为 null，得到 %+v", report.LastCapture)
	}
}

// TestStatusWithoutSourceFallsBackToUnknown 钉住未注入状态源的兜底：phase
// 显示 unknown 而不是空串或报错。
func TestStatusWithoutSourceFallsBackToUnknown(t *testing.T) {
	report := decodeStatusJSON(t, serveToRecorder(newTestService(), "/status").Body.Bytes())
	if report.Phase != "unknown" {
		t.Errorf("未注入状态源时 phase = %q，想要 unknown", report.Phase)
	}
}

// TestStatusReflectsLastCapture 钉住摘要联动：一次失败捕获之后 `/status` 必须能看到。
func TestStatusReflectsLastCapture(t *testing.T) {
	s := newTestService()
	done := make(chan app.CaptureOutcome, 1)
	s.requests <- app.CaptureRequest{Done: done}
	req, _ := s.PendingCapture()
	s.CompleteCapture(req, nil, 0, 0, client.ErrCaptureUnavailable)
	report := decodeStatusJSON(t, serveToRecorder(s, "/status").Body.Bytes())
	if report.LastCapture == nil {
		t.Fatal("失败捕获后 last_capture 不应为 null")
	}
	if report.LastCapture.Error == "" || report.LastCapture.Width != 0 || report.LastCapture.Height != 0 {
		t.Errorf("last_capture = %+v，想要零尺寸加错误文案", report.LastCapture)
	}
	if report.LastCapture.At == "" {
		t.Error("last_capture.at 应有值")
	}
}

// TestRecordRejectsOutOfRangeParams 钉住参数越界矩阵：三种越界与非法格式
// 全部 400 + 中文错误，且不产生任何帧捕获（通道保持空态）。
func TestRecordRejectsOutOfRangeParams(t *testing.T) {
	for _, target := range []string{
		"/record?seconds=100",
		"/record?fps=0",
		"/record?seconds=21&fps=12", // 总帧 252 超上限
		"/record?format=jpeg",
		"/record?seconds=abc",
	} {
		s := newTestService()
		rr := serveToRecorder(s, target)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s 状态码 = %d，想要 400（body：%s）", target, rr.Code, rr.Body.String())
			continue
		}
		var payload struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil || payload.Error == "" {
			t.Errorf("%s 应返回非空中文错误 JSON：%s", target, rr.Body.String())
		}
		if _, ok := s.PendingCapture(); ok {
			t.Errorf("%s 越界请求不应产生帧捕获", target)
		}
	}
}

// TestRecordParamParsingDefaultsAndBounds 在解析层钉住默认值与边界：默认
// seconds=5、fps=8、png；恰好压线的 20×12=240 必须接受，越线一档即拒绝。
func TestRecordParamParsingDefaultsAndBounds(t *testing.T) {
	defaults, err := parseRecordParams(nil)
	if err != nil || defaults.Seconds != 5 || defaults.FPS != 8 || defaults.Format != "png" {
		t.Fatalf("默认参数 = %+v, err = %v，想要 5/8/png", defaults, err)
	}
	edge, err := parseRecordParams(url.Values{"seconds": {"20"}, "fps": {"12"}})
	if err != nil {
		t.Fatalf("压线参数 20×12 应被接受，得到 %v", err)
	}
	if edge.Seconds != 20 || edge.FPS != 12 {
		t.Errorf("压线参数解析 = %+v", edge)
	}
	for _, query := range []url.Values{
		{"seconds": {"0"}},
		{"seconds": {"-1"}},
		{"seconds": {"21"}},
		{"fps": {"13"}},
		{"fps": {"-2"}},
		{"seconds": {"20"}, "fps": {"13"}},
	} {
		if _, err := parseRecordParams(query); err == nil {
			t.Errorf("越界参数 %v 应被拒绝", query)
		}
	}
}
