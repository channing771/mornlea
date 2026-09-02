//go:build darwin

package devcapture

import (
	"errors"
	"testing"
	"time"

	"github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/internal/client"
)

// TestPendingCaptureWithoutRequestIsImmediate 钉住「无待办时非阻塞立即返回」：
// 空通道上的调用必须直接落回 false，绝不允许等待。
func TestPendingCaptureWithoutRequestIsImmediate(t *testing.T) {
	s := newTestService()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, ok := s.PendingCapture(); ok {
			t.Error("空请求通道不应报告待办")
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("PendingCapture 在空通道上阻塞了")
	}
}

// TestRequestDeliveredThroughCoordinator 走一遍完整交接：HTTP 侧入队、泵侧
// 非阻塞取出、`CompleteCapture` 交付，两端看到同一个请求语义。
func TestRequestDeliveredThroughCoordinator(t *testing.T) {
	s := newTestService()
	done := make(chan app.CaptureOutcome, 1)
	select {
	case s.requests <- app.CaptureRequest{Done: done}:
	default:
		t.Fatal("空通道入队不应失败")
	}
	req, ok := s.PendingCapture()
	if !ok {
		t.Fatal("有待办时 PendingCapture 应返回 true")
	}
	// 单 outstanding：请求被取走后通道回到空态。
	if _, ok := s.PendingCapture(); ok {
		t.Error("请求被取走后不应再有待办")
	}
	s.CompleteCapture(req, testFramePixels(), 2, 1, nil)
	select {
	case outcome := <-done:
		if string(outcome.Pixels) != string(testFramePixels()) || outcome.Width != 2 || outcome.Height != 1 {
			t.Errorf("交付的 outcome 不匹配：%+v", outcome)
		}
		if outcome.Err != nil {
			t.Errorf("成功交付不应带错误：%v", outcome.Err)
		}
	default:
		t.Fatal("CompleteCapture 未把结果投给请求自带的 Done 通道")
	}
}

// TestCompleteCaptureWithFullDoneDoesNotBlock 是交付路径的核心钉子：交付缓冲
// 已满（消费者已离开）时 `CompleteCapture` 必须立即返回并计数丢弃，绝不反压
// 帧循环。
func TestCompleteCaptureWithFullDoneDoesNotBlock(t *testing.T) {
	s := newTestService()
	done := make(chan app.CaptureOutcome, 1)
	// 预填使容量 1 的交付缓冲保持满：模拟消费者超时离开后无人接收。
	done <- app.CaptureOutcome{}
	s.requests <- app.CaptureRequest{Done: done}
	req, ok := s.PendingCapture()
	if !ok {
		t.Fatal("有待办时 PendingCapture 应返回 true")
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		s.CompleteCapture(req, testFramePixels(), 2, 1, nil)
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("CompleteCapture 在交付缓冲已满时阻塞了帧循环")
	}
	if got := s.droppedOutcomes(); got != 1 {
		t.Errorf("满缓冲投递应计数丢弃，dropped = %d", got)
	}
	// 丢弃的帧同样要进入 /status 的最近捕获摘要：观察者靠它发现异常。
	if summary := s.lastCaptureSnapshot(); summary.errText != "" {
		t.Errorf("成功帧的摘要不应带错误：%q", summary.errText)
	}
}

// TestCompleteCaptureWithNilDoneCountsDrop 钉住零值请求（`Done` 为 nil）的
// 防御路径：select 对 nil 通道的发送永远不就绪，必须落入丢弃分支而不是阻塞。
func TestCompleteCaptureWithNilDoneCountsDrop(t *testing.T) {
	s := newTestService()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		s.CompleteCapture(app.CaptureRequest{}, nil, 0, 0, nil)
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("CompleteCapture 对 nil Done 阻塞了帧循环")
	}
	if got := s.droppedOutcomes(); got != 1 {
		t.Errorf("nil Done 应按丢弃计数，dropped = %d", got)
	}
}

// TestLastCaptureSummaryRecordsFailure 钉住最近捕获摘要的错误留痕：捕获桥
// 报告不可用时，/status 依赖的摘要必须带稳定中文文案与发生时间。
func TestLastCaptureSummaryRecordsFailure(t *testing.T) {
	s := newTestService()
	if summary := s.lastCaptureSnapshot(); !summary.at.IsZero() {
		t.Fatal("未发生任何捕获时摘要应缺省")
	}
	done := make(chan app.CaptureOutcome, 1)
	s.requests <- app.CaptureRequest{Done: done}
	req, _ := s.PendingCapture()
	s.CompleteCapture(req, nil, 0, 0, client.ErrCaptureUnavailable)
	summary := s.lastCaptureSnapshot()
	if summary.errText == "" {
		t.Fatal("失败捕获的摘要应留错误文案")
	}
	if summary.errText != captureErrorText(client.ErrCaptureUnavailable) {
		t.Errorf("摘要错误文案与 handler 口径不一致：%q", summary.errText)
	}
	if summary.at.IsZero() {
		t.Error("摘要应记录发生时间")
	}
}

// TestRequestFrameBusyOnOutstanding 钉住单 outstanding 的入队拒绝：通道已有
// 请求时第二次入队必须立即以 `ErrCaptureBusy` 失败，不得排队等待。
func TestRequestFrameBusyOnOutstanding(t *testing.T) {
	s := newTestService()
	done := make(chan app.CaptureOutcome, 1)
	s.requests <- app.CaptureRequest{Done: done}
	if _, err := s.requestFrame(t.Context(), time.Second); !errors.Is(err, ErrCaptureBusy) {
		t.Fatalf("单 outstanding 下的第二次请求应返回 ErrCaptureBusy，得到 %v", err)
	}
}
