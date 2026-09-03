//go:build darwin

package devcapture

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
)

// 录制参数的契约边界：短录屏面向 UI 流程调试采样，上限同时约束响应体积与
// 采样对帧循环的持续占用；总帧数上限让最坏情况（20s×12fps）成为可预算的
// 常量而非随窗口尺寸无界膨胀。
const (
	defaultRecordSeconds = 5
	defaultRecordFPS     = 8
	maxRecordSeconds     = 20
	maxRecordFPS         = 12
	maxRecordTotalFrames = 240
)

// recordParams 是一次录制的解析后请求参数。
type recordParams struct {
	Seconds int
	FPS     int
	Format  string // "png" 或 "gif"
}

// parseRecordParams 解析并校验 `/record` 查询参数。缺省值（5s、8fps、png）
// 面向最常见的「几秒钟 UI 流程」用例；越界与不可解析一律返回稳定中文错误，
// 由 handler 映射为 400——校验发生在任何帧捕获之前，越界请求不产生采样。
func parseRecordParams(query url.Values) (recordParams, error) {
	params := recordParams{Seconds: defaultRecordSeconds, FPS: defaultRecordFPS, Format: "png"}
	if raw := query.Get("seconds"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return params, errors.New("参数无法解析：seconds 必须为整数")
		}
		params.Seconds = value
	}
	if raw := query.Get("fps"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return params, errors.New("参数无法解析：fps 必须为整数")
		}
		params.FPS = value
	}
	if raw := query.Get("format"); raw != "" {
		params.Format = raw
	}
	if params.Format != "png" && params.Format != "gif" {
		return params, errors.New("参数越界：format 仅支持 png 或 gif")
	}
	if params.Seconds <= 0 || params.Seconds > maxRecordSeconds {
		return params, fmt.Errorf("参数越界：seconds 必须满足 0 < seconds ≤ %d", maxRecordSeconds)
	}
	if params.FPS <= 0 || params.FPS > maxRecordFPS {
		return params, fmt.Errorf("参数越界：fps 必须满足 0 < fps ≤ %d", maxRecordFPS)
	}
	if params.Seconds*params.FPS > maxRecordTotalFrames {
		return params, fmt.Errorf("参数越界：seconds×fps 必须满足 ≤ %d（本次为 %d）",
			maxRecordTotalFrames, params.Seconds*params.FPS)
	}
	return params, nil
}

// manifestFrame 是单帧在 manifest.json 中的条目：文件名、相对录制开始的
// 毫秒时间戳与尺寸。时间戳必须单调不减——采样推进由同一时钟驱动。
type manifestFrame struct {
	Index       int    `json:"index"`
	File        string `json:"file"`
	TimestampMS int64  `json:"timestamp_ms"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}

// recordManifest 是 manifest.json 的结构：请求参数、实际帧数、逐帧时间戳
// 与尺寸、丢帧数、GIF 有无、终止错误。`frames` 恒为非 nil 数组，观察者
// 不必区分 null 与空。
type recordManifest struct {
	Seconds       int             `json:"seconds"`
	FPS           int             `json:"fps"`
	Format        string          `json:"format"`
	RequestFrames int             `json:"requested_frames"`
	FrameCount    int             `json:"frame_count"`
	Frames        []manifestFrame `json:"frames"`
	DroppedFrames int             `json:"dropped_frames"`
	GIF           bool            `json:"gif"`
	Error         string          `json:"error,omitempty"`
}

// recording 聚合一次录制的产出与过程状态。PNG 字节全程驻留内存（最终要整包
// 成 zip），参数上限（≤240 帧）与 localhost 调试定位使这份常量驻留可接受；
// GIF 请求下每帧就地量化成调色板图（每像素 1 字节），避免同时持有全套
// NRGBA 帧。
type recording struct {
	params   recordParams
	interval time.Duration
	deadline time.Time
	start    time.Time

	pngFrames      [][]byte
	manifestFrames []manifestFrame
	gifFrames      []*image.Paletted
	gifDelays      []int
	skipped        int
	terminalErr    string
}

// newRecording 按参数与当前时钟建立录制会话。总截止随帧数扩展：名义时长 +
// 固定余量（`recordDeadlineMargin`）+ 逐帧预算（`recordPerFrameBudget`×
// 总帧数）——捕获与编码成本随帧数线性增长，固定余量在最坏参数下数学上
// 不可能容纳它们。
func (s *Service) newRecording(params recordParams) *recording {
	start := s.now()
	totalFrames := params.Seconds * params.FPS
	return &recording{
		params:   params,
		interval: time.Duration(int64(time.Second) / int64(params.FPS)),
		deadline: start.Add(time.Duration(params.Seconds)*time.Second +
			recordDeadlineMargin + time.Duration(totalFrames)*recordPerFrameBudget),
		start:          start,
		manifestFrames: []manifestFrame{},
	}
}

// addFrame 就地编码一帧：转 NRGBA、编 PNG、（gif 格式时）Floyd-Steinberg
// 量化进调色板帧。编码失败按终端错误处理——坏帧留在产物里只会制造假证据。
func (rec *recording) addFrame(at time.Time, outcome app.CaptureOutcome) error {
	img, err := outcomeNRGBA(outcome)
	if err != nil {
		return err
	}
	framePNG, err := encodePNG(img)
	if err != nil {
		return fmt.Errorf("编码 PNG 失败: %w", err)
	}
	index := len(rec.pngFrames) + 1
	rec.pngFrames = append(rec.pngFrames, framePNG)
	rec.manifestFrames = append(rec.manifestFrames, manifestFrame{
		Index:       index,
		File:        fmt.Sprintf("frames/frame-%04d.png", index),
		TimestampMS: at.Sub(rec.start).Milliseconds(),
		Width:       outcome.Width,
		Height:      outcome.Height,
	})
	if rec.params.Format == "gif" {
		paletted := image.NewPaletted(img.Bounds(), palette.Plan9)
		draw.FloydSteinberg.Draw(paletted, img.Bounds(), img, image.Point{})
		rec.gifFrames = append(rec.gifFrames, paletted)
		// GIF 帧延迟以百分之一秒为单位：按 fps 四舍五入，8fps ≈ 13。
		rec.gifDelays = append(rec.gifDelays, (100+rec.params.FPS/2)/rec.params.FPS)
	}
	return nil
}

// serveRecord 采样窗口合成帧序列并以 zip 交付。采样编排恪守单 outstanding
// 契约：循环「（必要时）等到本帧目标时刻 → 发一帧请求 → 等交付 → 下一帧」，
// 帧间隔等待全部发生在本 goroutine，帧循环只承受每帧至多一次的捕获开销。
//
// 失败语义分三层：参数越界 400（见 `parseRecordParams`）；单帧捕获错误或
// 单帧交付超时终止采样，已收帧与终止错误写进 manifest 后仍交付 zip（局部
// 证据好于全盘丢弃）；总截止超限以 503 放弃整次录制——此时连名义时长都未
// 达成，交付残片只会误导观察者。
func (s *Service) serveRecord(w http.ResponseWriter, r *http.Request) {
	params, err := parseRecordParams(r.URL.Query())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.tryBeginRecording() {
		writeJSONError(w, http.StatusServiceUnavailable, "已有录制在进行中，请等待其完成")
		return
	}
	defer s.endRecording()

	rec := s.newRecording(params)
	totalFrames := params.Seconds * params.FPS
	for i := 0; i < totalFrames; i++ {
		// 第一帧立即开始；其后各帧等到自己的目标时刻，令帧间隔不快于 1/fps。
		if wait := time.Duration(i)*rec.interval - s.now().Sub(rec.start); wait > 0 {
			s.sleep(wait)
		}
		// 截止检查紧贴采样动作：节奏等待本身也消耗预算，等待越界（帧循环
		// 停滞或不可用）时不发起下一帧，整次录制以 503 放弃。文案回显公式
		// 的三段数值，观察者可直接对账哪一段不匹配。
		if s.now().After(rec.deadline) {
			writeJSONError(w, http.StatusServiceUnavailable,
				fmt.Sprintf("录制超出总时长上限（名义 %ds + 余量 %ds + %d 帧×%ds 预算），放弃本次录制",
					params.Seconds, int(recordDeadlineMargin.Seconds()), totalFrames, int(recordPerFrameBudget.Seconds())))
			return
		}
		outcome, err := s.requestFrame(r.Context(), s.frameWait)
		if errors.Is(err, ErrCaptureBusy) {
			// 通道被并发请求占用：跳过本拍计丢帧，采样节奏照常推进。
			rec.skipped++
			continue
		}
		if err != nil {
			rec.terminalErr = frameWaitErrorText(err)
			break
		}
		if outcome.Err != nil {
			rec.terminalErr = captureErrorText(outcome.Err)
			break
		}
		if err := rec.addFrame(s.now(), outcome); err != nil {
			rec.terminalErr = fmt.Sprintf("第 %d 帧编码失败：%v", i+1, err)
			break
		}
	}
	s.writeRecordZip(w, rec)
}

// writeRecordZip 把帧序列、可选 GIF 与 manifest 组包为 zip 交付。整包先进
// 内存缓冲：zip 组包失败时仍能改发结构化错误，而不是把半包 zip 发给客户端。
func (s *Service) writeRecordZip(w http.ResponseWriter, rec *recording) {
	manifest := recordManifest{
		Seconds:       rec.params.Seconds,
		FPS:           rec.params.FPS,
		Format:        rec.params.Format,
		RequestFrames: rec.params.Seconds * rec.params.FPS,
		FrameCount:    len(rec.pngFrames),
		Frames:        rec.manifestFrames,
		DroppedFrames: rec.skipped,
		GIF:           len(rec.gifFrames) > 0,
		Error:         rec.terminalErr,
	}
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	for i, framePNG := range rec.pngFrames {
		if err := writeZipEntry(zipWriter, rec.manifestFrames[i].File, framePNG); err != nil {
			writeJSONError(w, http.StatusServiceUnavailable, fmt.Sprintf("打包录制结果失败：%v", err))
			return
		}
	}
	if manifest.GIF {
		var gifBuf bytes.Buffer
		first := rec.gifFrames[0]
		err := gif.EncodeAll(&gifBuf, &gif.GIF{
			Image: rec.gifFrames,
			Delay: rec.gifDelays,
			Config: image.Config{
				Width:      first.Bounds().Dx(),
				Height:     first.Bounds().Dy(),
				ColorModel: first.Palette,
			},
		})
		if err != nil {
			writeJSONError(w, http.StatusServiceUnavailable, fmt.Sprintf("编码 preview.gif 失败：%v", err))
			return
		}
		if err := writeZipEntry(zipWriter, "preview.gif", gifBuf.Bytes()); err != nil {
			writeJSONError(w, http.StatusServiceUnavailable, fmt.Sprintf("打包录制结果失败：%v", err))
			return
		}
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, fmt.Sprintf("编码 manifest 失败：%v", err))
		return
	}
	if err := writeZipEntry(zipWriter, "manifest.json", manifestJSON); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, fmt.Sprintf("打包录制结果失败：%v", err))
		return
	}
	if err := zipWriter.Close(); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, fmt.Sprintf("打包录制结果失败：%v", err))
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	_, _ = w.Write(buf.Bytes())
}

// writeZipEntry 写入一个 zip 条目。打包是交付前的最后一步，任何 I/O 错误
// 都必须显式上抛，绝不允许静默截断产物。
func writeZipEntry(zipWriter *zip.Writer, name string, data []byte) error {
	entry, err := zipWriter.Create(name)
	if err != nil {
		return fmt.Errorf("创建条目 %s: %w", name, err)
	}
	if _, err := entry.Write(data); err != nil {
		return fmt.Errorf("写入条目 %s: %w", name, err)
	}
	return nil
}
