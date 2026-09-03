//go:build darwin

package devcapture

import (
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
)

// newTestService 构造不启动监听、不写端口文件的裸 `Service`：协调器与
// handler 单测都从它出发，按需覆写非导出字段（`frameWait`、时间源等）。
func newTestService() *Service {
	return New(Options{})
}

// testFramePixels 构造 2×1 的 BGRA 像素，字节值可辨识（通道断言见 bgra_test.go）。
func testFramePixels() []byte {
	return []byte{10, 20, 30, 255, 40, 50, 60, 255}
}

// pumpOnce 在后台扮演帧循环捕获泵：轮询待办并交付给定结果，仅交付一次。
func pumpOnce(s *Service, outcome app.CaptureOutcome) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if req, ok := s.PendingCapture(); ok {
			s.CompleteCapture(req, outcome.Pixels, outcome.Width, outcome.Height, outcome.Err)
			return
		}
		time.Sleep(100 * time.Microsecond)
	}
}

// serveToRecorder 直接驱动 mux（httptest 模式：不经网络、不建窗口）。
func serveToRecorder(s *Service, target string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, target, nil))
	return rr
}
