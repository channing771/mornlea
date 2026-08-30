//go:build darwin

package devcapture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"strconv"
	"time"
)

// Handler 返回捕获服务的 HTTP 面。三个端点都以方法模式挂载：非 GET 请求由
// mux 统一拒绝为 405，handler 内不再重复判方法。
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.serveStatus)
	mux.HandleFunc("GET /screenshot", s.serveScreenshot)
	mux.HandleFunc("GET /record", s.serveRecord)
	return mux
}

// writeJSONError 以统一形状下发错误：`{"error": <稳定中文文案>}`。文案的
// 稳定性是契约的一部分——观察者（agent）按文案分流处理，随机措辞等于逼人
// 解析整段响应。
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// frameWaitErrorText 把请求编排层的失败（忙碌、超时、取消）映射为稳定中文
// 文案；全部对应 503——服务在但帧循环侧暂不可用。
func frameWaitErrorText(err error) string {
	switch {
	case errors.Is(err, ErrCaptureBusy):
		return "已有捕获请求在执行，请稍后重试"
	case errors.Is(err, ErrFrameWaitTimeout):
		return "等待捕获交付超时（帧循环可能已停止）"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "捕获请求已取消"
	default:
		return "捕获请求失败：" + err.Error()
	}
}

// windowDoc 是注入状态源报告的当前窗口内容尺寸。
type windowDoc struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// lastCaptureDoc 是最近一次捕获交付的摘要：时间为交付时刻（RFC3339），尺寸
// 为该帧合成图尺寸，错误为稳定中文文案（成功时缺省）。
type lastCaptureDoc struct {
	At     string `json:"at"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Error  string `json:"error,omitempty"`
}

// statusReport 是 `/status` 的响应结构。last_capture 从未捕获时为 null，
// JSON 键恒存在，观察者不必猜字段。
type statusReport struct {
	PID         int             `json:"pid"`
	Phase       string          `json:"phase"`
	Window      *windowDoc      `json:"window,omitempty"`
	Port        int             `json:"port"`
	Recording   bool            `json:"recording"`
	LastCapture *lastCaptureDoc `json:"last_capture"`
}

// serveStatus 返回服务自身与客户端状态的 JSON 快照。字段面刻意最小：pid 与
// 端口用于发现与确认目标进程，phase 与窗口尺寸给观察者预期画面形态，最近
// 捕获摘要暴露帧循环侧的健康状况（授权缺失等失败在这里可见，不静默）。
func (s *Service) serveStatus(w http.ResponseWriter, r *http.Request) {
	report := statusReport{
		PID:   os.Getpid(),
		Port:  s.Port(),
		Phase: "unknown",
	}
	if s.status != nil {
		if phase := s.status.Phase(); phase != "" {
			report.Phase = phase
		}
		width, height := s.status.WindowWidth(), s.status.WindowHeight()
		if width > 0 && height > 0 {
			report.Window = &windowDoc{Width: width, Height: height}
		}
	}
	if report.Phase == "" {
		report.Phase = "unknown"
	}
	if summary := s.lastCaptureSnapshot(); !summary.at.IsZero() {
		report.LastCapture = &lastCaptureDoc{
			At:     summary.at.Format(time.RFC3339),
			Width:  summary.width,
			Height: summary.height,
			Error:  summary.errText,
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(report)
}

// serveScreenshot 交付单张窗口合成截图：经协调器通道请求一帧 → 等交付 →
// 转 NRGBA → PNG。等待上限 `frameWait`（默认 10s）：帧循环正常运行时一帧
// 捕获远快于此；超限意味着帧循环已停止，按契约以 503 有界收敛。编码先进
// 内存缓冲再落响应：PNG 编码失败时仍能下发结构化错误而不是半张图。
func (s *Service) serveScreenshot(w http.ResponseWriter, r *http.Request) {
	outcome, err := s.requestFrame(r.Context(), s.frameWait)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, frameWaitErrorText(err))
		return
	}
	if outcome.Err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, captureErrorText(outcome.Err))
		return
	}
	img, err := outcomeNRGBA(outcome)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, fmt.Sprintf("编码 PNG 失败：%v", err))
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	_, _ = w.Write(buf.Bytes())
}
