//go:build darwin

package devcapture

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
)

// fakeClock 是可注入的时间源：sleep 会推进假时钟，让录制编排的节奏等待与
// 截止检查完全确定，测试不做任何真实等待。
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	sleeps []time.Duration
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(0, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
}

// recordingPump 是带状态的录制泵：逐次交付 deliveries 里的结果，耗尽后停手
// （后续请求交由逐帧等待超时收敛）。served 记录实际交付次数。
type recordingPump struct {
	mu         sync.Mutex
	deliveries []app.CaptureOutcome
	served     int
}

func (p *recordingPump) run(s *Service, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		p.mu.Lock()
		has := len(p.deliveries) > 0
		var next app.CaptureOutcome
		if has {
			next = p.deliveries[0]
			p.deliveries = p.deliveries[1:]
			p.served++
		}
		p.mu.Unlock()
		if !has {
			time.Sleep(200 * time.Microsecond)
			continue
		}
		// 只认自己经 PendingCapture 取到的请求；没抢到就搁回额度稍后再试。
		if req, ok := s.PendingCapture(); ok {
			s.CompleteCapture(req, next.Pixels, next.Width, next.Height, next.Err)
		} else {
			p.mu.Lock()
			p.deliveries = append([]app.CaptureOutcome{next}, p.deliveries...)
			p.served--
			p.mu.Unlock()
			time.Sleep(200 * time.Microsecond)
		}
	}
}

func (p *recordingPump) servedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.served
}

// newRecordService 构造带假时钟的录制用 `Service`，逐帧等待上限放宽到 2s
// 以吸收调度抖动；成功路径里 pump 即时交付，不会真正等待。
func newRecordService(clock *fakeClock) *Service {
	s := New(Options{Now: clock.Now, Sleep: clock.Sleep})
	s.frameWait = 2 * time.Second
	return s
}

// frameFileName 是录制 zip 内第 i 帧的约定文件名（从 1 起、四位零填充）。
func frameFileName(i int) string {
	return fmt.Sprintf("frames/frame-%04d.png", i)
}

// readZipEntries 把 zip 响应体解成「文件名 → 内容」。
func readZipEntries(t *testing.T, body []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("响应不是合法 zip：%v", err)
	}
	entries := make(map[string][]byte, len(reader.File))
	for _, f := range reader.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("打开 zip 条目 %s：%v", f.Name, err)
		}
		data, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			t.Fatalf("读取 zip 条目 %s：%v", f.Name, readErr)
		}
		entries[f.Name] = data
	}
	return entries
}

// decodeManifest 反解 zip 内的 manifest.json。
func decodeManifest(t *testing.T, entries map[string][]byte) recordManifest {
	t.Helper()
	raw, ok := entries["manifest.json"]
	if !ok {
		t.Fatalf("zip 内缺少 manifest.json（现有条目：%v）", entryNames(entries))
	}
	var manifest recordManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("反解 manifest.json 失败：%v", err)
	}
	return manifest
}

// entryNames 返回 zip 条目名的稳定列表，用于失败信息展示。
func entryNames(entries map[string][]byte) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// TestRecordSuccessProducesParsableZip 钉住成功录制：zip 内 PNG 帧数与
// manifest 声明一致、文件名从 frame-0001 起、帧时间戳单调不减、节奏等待
// 发生在服务 goroutine 而非帧循环。
func TestRecordSuccessProducesParsableZip(t *testing.T) {
	clock := newFakeClock()
	s := newRecordService(clock)
	pump := &recordingPump{deliveries: []app.CaptureOutcome{
		{Pixels: testFramePixels(), Width: 2, Height: 1},
		{Pixels: testFramePixels(), Width: 2, Height: 1},
		{Pixels: testFramePixels(), Width: 2, Height: 1},
		{Pixels: testFramePixels(), Width: 2, Height: 1},
	}}
	stop := make(chan struct{})
	defer close(stop)
	go pump.run(s, stop)

	rr := serveToRecorder(s, "/record?seconds=1&fps=4")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200（body：%s）", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q，想要 application/zip", ct)
	}
	entries := readZipEntries(t, rr.Body.Bytes())
	manifest := decodeManifest(t, entries)
	if manifest.FrameCount != 4 || manifest.RequestFrames != 4 {
		t.Errorf("manifest 帧数 = %d/%d，想要 4/4", manifest.FrameCount, manifest.RequestFrames)
	}
	if manifest.Seconds != 1 || manifest.FPS != 4 || manifest.Format != "png" {
		t.Errorf("manifest 参数 = %+v，想要 1s/4fps/png", manifest)
	}
	if manifest.DroppedFrames != 0 || manifest.GIF || manifest.Error != "" {
		t.Errorf("成功录制的 manifest 异常：%+v", manifest)
	}
	pngCount := 0
	lastTS := int64(-1)
	for i := 1; i <= 4; i++ {
		name := frameFileName(i)
		data, ok := entries[name]
		if !ok {
			t.Fatalf("zip 内缺少 %s（现有：%v）", name, entryNames(entries))
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s 不是合法 PNG：%v", name, err)
		}
		if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 1 {
			t.Errorf("%s 尺寸 = %v，想要 2×1", name, img.Bounds())
		}
		pngCount++
		ts := manifest.Frames[i-1].TimestampMS
		if ts < lastTS {
			t.Errorf("帧时间戳应单调不减：%d < %d", ts, lastTS)
		}
		lastTS = ts
		entry := manifest.Frames[i-1]
		if entry.File != name || entry.Width != 2 || entry.Height != 1 || entry.Index != i {
			t.Errorf("manifest 帧条目不符：%+v", entry)
		}
	}
	if pngCount != manifest.FrameCount {
		t.Errorf("zip 内 PNG 数 %d 与 manifest 帧数 %d 不一致", pngCount, manifest.FrameCount)
	}
	if _, ok := entries["preview.gif"]; ok {
		t.Error("png 格式不应产出 preview.gif")
	}
	if got := pump.servedCount(); got != 4 {
		t.Errorf("泵实际交付 %d 次，想要 4", got)
	}
	// 节奏等待必须发生在服务侧：假时钟应收到按帧间隔推进的 sleep 请求。
	if len(clock.sleeps) == 0 {
		t.Error("录制循环未做任何帧间隔等待")
	}
	if s.recordingActive() {
		t.Error("录制结束后 recording 状态应复位")
	}
}

// TestRecordGIFAddsDecodablePreview 钉住 GIF 附加产物：format=gif 时 zip
// 内含可解码的 preview.gif 条目，manifest 标记 GIF 存在，PNG 帧序列照常保留。
func TestRecordGIFAddsDecodablePreview(t *testing.T) {
	clock := newFakeClock()
	s := newRecordService(clock)
	pump := &recordingPump{deliveries: []app.CaptureOutcome{
		{Pixels: testFramePixels(), Width: 2, Height: 1},
		{Pixels: testFramePixels(), Width: 2, Height: 1},
		{Pixels: testFramePixels(), Width: 2, Height: 1},
		{Pixels: testFramePixels(), Width: 2, Height: 1},
	}}
	stop := make(chan struct{})
	defer close(stop)
	go pump.run(s, stop)

	rr := serveToRecorder(s, "/record?seconds=1&fps=4&format=gif")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200（body：%s）", rr.Code, rr.Body.String())
	}
	entries := readZipEntries(t, rr.Body.Bytes())
	manifest := decodeManifest(t, entries)
	if !manifest.GIF {
		t.Error("manifest 应标记 GIF 存在")
	}
	raw, ok := entries["preview.gif"]
	if !ok {
		t.Fatalf("zip 内缺少 preview.gif（现有：%v）", entryNames(entries))
	}
	img, err := gif.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("preview.gif 无法解码：%v", err)
	}
	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 1 {
		t.Errorf("preview.gif 尺寸 = %v，想要 2×1", img.Bounds())
	}
	for i := 1; i <= 4; i++ {
		if _, ok := entries[frameFileName(i)]; !ok {
			t.Errorf("gif 格式下缺少 %s", frameFileName(i))
		}
	}
}

// TestRecordTerminatesOnCaptureError 钉住单帧失败语义：捕获错误终止采样，
// 已收帧与终止错误写进 manifest 后仍以 zip 交付（不静默吞帧、不整单失败）。
func TestRecordTerminatesOnCaptureError(t *testing.T) {
	clock := newFakeClock()
	s := newRecordService(clock)
	pump := &recordingPump{deliveries: []app.CaptureOutcome{
		{Pixels: testFramePixels(), Width: 2, Height: 1},
		{Err: client.ErrCaptureUnavailable},
		{Pixels: testFramePixels(), Width: 2, Height: 1},
	}}
	stop := make(chan struct{})
	defer close(stop)
	go pump.run(s, stop)

	rr := serveToRecorder(s, "/record?seconds=1&fps=4")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200（终止录制仍交付已收帧）", rr.Code)
	}
	manifest := decodeManifest(t, readZipEntries(t, rr.Body.Bytes()))
	if manifest.FrameCount != 1 {
		t.Errorf("终止后 manifest 帧数 = %d，想要 1", manifest.FrameCount)
	}
	if !strings.Contains(manifest.Error, "不可用") {
		t.Errorf("manifest 应带终止错误文案，得到 %q", manifest.Error)
	}
	if _, ok := readZipEntries(t, rr.Body.Bytes())[frameFileName(2)]; ok {
		t.Error("终止后不应再有后续帧")
	}
	if got := pump.servedCount(); got != 2 {
		t.Errorf("终止后泵不应继续消耗交付额度，served = %d，想要 2", got)
	}
}

// TestRecordCountsBusyTicksAsDropped 钉住丢帧计数：请求通道被占用时逐帧
// 忙碌跳过，manifest 记录丢帧数且不产出任何帧。
func TestRecordCountsBusyTicksAsDropped(t *testing.T) {
	clock := newFakeClock()
	s := newRecordService(clock)
	// 预占请求通道模拟另一处 outstanding：整个录制期间每帧都忙碌。
	done := make(chan app.CaptureOutcome, 1)
	s.requests <- app.CaptureRequest{Done: done}

	rr := serveToRecorder(s, "/record?seconds=1&fps=2")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200（body：%s）", rr.Code, rr.Body.String())
	}
	manifest := decodeManifest(t, readZipEntries(t, rr.Body.Bytes()))
	if manifest.FrameCount != 0 {
		t.Errorf("全部忙碌时不应有帧，得到 %d", manifest.FrameCount)
	}
	if manifest.DroppedFrames != 2 {
		t.Errorf("manifest 丢帧数 = %d，想要 2", manifest.DroppedFrames)
	}
}

// TestRecordDeadlineScalesWithFrames 钉住总截止公式：名义时长 + 固定余量 +
// 逐帧预算×帧数。逐帧预算随帧数线性扩展且取最慢可选路径（GIF 量化约
// 2.2s/帧实测）放大，是 2s×8fps 到 20s×12fps 全参数域、全格式可完成的前提。
func TestRecordDeadlineScalesWithFrames(t *testing.T) {
	s := newRecordService(newFakeClock())
	rec := s.newRecording(recordParams{Seconds: 2, FPS: 8})
	want := time.Unix(0, 0).Add(2*time.Second + recordDeadlineMargin + 16*recordPerFrameBudget)
	if !rec.deadline.Equal(want) {
		t.Errorf("2s×8fps 的截止 = %v，想要 %v（2s 名义 + 30s 余量 + 16 帧×2.5s 预算 = 72s）",
			rec.deadline, want)
	}
	worst := s.newRecording(recordParams{Seconds: 20, FPS: 12})
	wantWorst := time.Unix(0, 0).Add(20*time.Second + recordDeadlineMargin + 240*recordPerFrameBudget)
	if !worst.deadline.Equal(wantWorst) {
		t.Errorf("20s×12fps 的截止 = %v，想要 %v（20s 名义 + 30s 余量 + 240 帧×2.5s 预算 = 650s）",
			worst.deadline, wantWorst)
	}
}

// TestRecordAbortsBeyondDeadline 钉住总时长上限：把每次节奏等待放大到越过
// 截止，下一帧前的截止检查必须以 503 放弃整次录制并停止采样，文案回显
// 公式数值供观察者对账。
func TestRecordAbortsBeyondDeadline(t *testing.T) {
	clock := newFakeClock()
	s := newRecordService(clock)
	s.sleep = func(d time.Duration) { clock.Sleep(d + 60*time.Second) }
	pump := &recordingPump{deliveries: []app.CaptureOutcome{
		{Pixels: testFramePixels(), Width: 2, Height: 1},
		{Pixels: testFramePixels(), Width: 2, Height: 1},
	}}
	stop := make(chan struct{})
	defer close(stop)
	go pump.run(s, stop)

	rr := serveToRecorder(s, "/record?seconds=1&fps=4")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d，想要 503（body：%s）", rr.Code, rr.Body.String())
	}
	for _, fragment := range []string{"总时长上限", "预算"} {
		if !strings.Contains(rr.Body.String(), fragment) {
			t.Errorf("503 文案缺少数值口径要素 %q：%s", fragment, rr.Body.String())
		}
	}
	if got := pump.servedCount(); got != 1 {
		t.Errorf("截止放弃应停止继续采样，served = %d，想要 1", got)
	}
}

// TestRecordFrameWaitTimeoutTerminates 钉住帧循环中途停止的有界收敛：交付
// 额度耗尽后，逐帧等待超时终止录制，manifest 带超时错误与已收帧。
func TestRecordFrameWaitTimeoutTerminates(t *testing.T) {
	clock := newFakeClock()
	s := newRecordService(clock)
	s.frameWait = 30 * time.Millisecond
	pump := &recordingPump{deliveries: []app.CaptureOutcome{
		{Pixels: testFramePixels(), Width: 2, Height: 1},
	}}
	stop := make(chan struct{})
	defer close(stop)
	go pump.run(s, stop)

	rr := serveToRecorder(s, "/record?seconds=1&fps=4")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200（body：%s）", rr.Code, rr.Body.String())
	}
	manifest := decodeManifest(t, readZipEntries(t, rr.Body.Bytes()))
	if manifest.FrameCount != 1 || !strings.Contains(manifest.Error, "超时") {
		t.Errorf("manifest = %+v，想要 1 帧 + 超时终止错误", manifest)
	}
}

// TestRecordRejectsConcurrentRecording 钉住并发录制互斥的失败路径：录制
// 标志被占用时，第二个 `/record` 以 503 与稳定中文文案失败，不交叠采样。
func TestRecordRejectsConcurrentRecording(t *testing.T) {
	s := newRecordService(newFakeClock())
	// 直接持有录制标志模拟另一场录制正在进行；结束时由测试自行释放。
	if !s.tryBeginRecording() {
		t.Fatal("占位录制标志失败")
	}
	defer s.endRecording()

	rr := serveToRecorder(s, "/record?seconds=1&fps=2")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d，想要 503（body：%s）", rr.Code, rr.Body.String())
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("错误响应应为 JSON：%v（body：%s）", err, rr.Body.String())
	}
	if !strings.Contains(payload.Error, "已有录制在进行中") {
		t.Errorf("错误文案缺少稳定要素：%q", payload.Error)
	}
	// 被拒绝的请求不得碰通道：帧循环侧不应看到任何采样请求。
	if _, ok := s.PendingCapture(); ok {
		t.Error("并发录制被拒时不应产生帧捕获")
	}
}

// TestRecordingFlagObservedByPump 钉住录制中状态对观察者可见：泵在交付
// 第一帧时读到的 recording 标志必须为 true。
func TestRecordingFlagObservedByPump(t *testing.T) {
	clock := newFakeClock()
	s := newRecordService(clock)
	sawRecording := make(chan bool, 1)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			req, ok := s.PendingCapture()
			if !ok {
				time.Sleep(200 * time.Microsecond)
				continue
			}
			sawRecording <- s.recordingActive()
			s.CompleteCapture(req, testFramePixels(), 2, 1, nil)
			return
		}
	}()

	rr := serveToRecorder(s, "/record?seconds=1&fps=1")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200（body：%s）", rr.Code, rr.Body.String())
	}
	select {
	case saw := <-sawRecording:
		if !saw {
			t.Error("录制采样期间 recording 标志应为 true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("泵未观察到录制中的请求")
	}
}
