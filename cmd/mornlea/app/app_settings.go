//go:build darwin

package app

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/audio"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
)

const maxSettingsMessageBytes = 256

// SettingsValues 只保存设置页公开的三项值。材质路径始终是配置原文，绝不
// 被解析后的绝对路径反向污染。
type SettingsValues struct {
	AudioVolume     float32
	TexturePackPath string
	WindowSize      config.WindowSize
}

// SettingsState 由 Go 拥有设置事务的已保存值、草稿与展示信息。dirty 不单独
// 存储，避免 Rust 回显或失败路径让布尔值与实际内容分叉。
type SettingsState struct {
	Committed SettingsValues
	Draft     SettingsValues
	status    string
	error     string
}

func (state SettingsState) dirty() bool {
	return state.Draft != state.Committed
}

func (state SettingsState) UISettings() client.UISettings {
	return client.UISettings{
		Visible:         true,
		AudioVolume:     state.Draft.AudioVolume,
		Window:          uiWindowFromConfig(state.Draft.WindowSize),
		TexturePackPath: state.Draft.TexturePackPath,
		Dirty:           state.dirty(),
		Status:          boundedSettingsMessage(state.status),
		Error:           boundedSettingsMessage(state.error),
	}
}

func settingsValuesFromUI(values client.UISettingsValues) (SettingsValues, error) {
	window, ok := configWindowFromUI(values.Window)
	if !ok {
		return SettingsValues{}, fmt.Errorf("WindowSize: 未知 UI 预设 %d", values.Window)
	}
	result := SettingsValues{
		AudioVolume:     values.AudioVolume,
		TexturePackPath: values.TexturePackPath,
		WindowSize:      window,
	}
	if err := result.validate(); err != nil {
		return SettingsValues{}, err
	}
	return result, nil

}

func (values SettingsValues) validate() error {
	if math.IsNaN(float64(values.AudioVolume)) || math.IsInf(float64(values.AudioVolume), 0) ||
		values.AudioVolume < 0 || values.AudioVolume > 1 {
		return fmt.Errorf("AudioVolume: 必须是 0..1 的有限数值，实际 %v", values.AudioVolume)
	}
	width, height := values.WindowSize.Dimensions()
	if width == 0 || height == 0 {
		return fmt.Errorf("WindowSize: 不支持的预设 %q", values.WindowSize)
	}
	if !utf8.ValidString(values.TexturePackPath) {
		return errors.New("TexturePackPath: 必须是合法 UTF-8")
	}
	if strings.ContainsAny(values.TexturePackPath, "\r\n") {
		return errors.New("TexturePackPath: 必须是单行文本")
	}
	if len(values.TexturePackPath) > config.MaxTexturePackPathBytes {
		return fmt.Errorf("TexturePackPath: %d 个 UTF-8 字节超过上限 %d",
			len(values.TexturePackPath), config.MaxTexturePackPathBytes)
	}
	return nil
}

func uiWindowFromConfig(size config.WindowSize) client.UISettingsWindow {
	switch size {
	case config.WindowSize640x360:
		return client.UISettingsWindow640x360
	case config.WindowSize960x540:
		return client.UISettingsWindow960x540
	case config.WindowSize1280x720:
		return client.UISettingsWindow1280x720
	default:
		panic("mornlea: 设置页收到非法窗口预设")
	}
}

func configWindowFromUI(window client.UISettingsWindow) (config.WindowSize, bool) {
	switch window {
	case client.UISettingsWindow640x360:
		return config.WindowSize640x360, true
	case client.UISettingsWindow960x540:
		return config.WindowSize960x540, true
	case client.UISettingsWindow1280x720:
		return config.WindowSize1280x720, true
	default:
		return "", false
	}
}

// saveSettings 严格按「草稿校验 → 变化材质候选校验 → 校验并 patch 最新磁盘配置 →
// rename 提交 → 更新 Committed → 替换运行时音频和窗口」执行。rename 前失败时
// Committed 与所有运行时资源均保持原状；rename 后父目录同步失败只降低掉电
// 持久性置信度，新字节已经可见，故仍提交内存和运行时并显示有界警告。
func (a *Application) saveSettings() error {
	draft := a.settings.Draft
	if err := draft.validate(); err != nil {
		return fmt.Errorf("校验设置: %w", err)
	}

	textureChanged := draft.TexturePackPath != a.settings.Committed.TexturePackPath
	if textureChanged && draft.TexturePackPath != "" {
		candidatePath, err := resolveSettingsTexturePackPath(a.startupOptions.ConfigPath, draft.TexturePackPath)
		if err != nil {
			return fmt.Errorf("解析材质包候选 %q: %w", draft.TexturePackPath, err)
		}
		if a.startupDeps.NewRegistry == nil {
			return errors.New("校验材质包候选: registry 工厂不可用")
		}
		if _, err := a.startupDeps.NewRegistry(candidatePath); err != nil {
			return fmt.Errorf("校验材质包候选 %q: %w", draft.TexturePackPath, err)
		}
	}

	patchSettings := a.startupDeps.PatchSettings
	if patchSettings == nil {
		patchSettings = config.PatchSettings
	}
	result, persistenceErr := patchSettings(a.startupOptions.ConfigPath, config.SettingsPatch{
		AudioVolume: draft.AudioVolume, TexturePackPath: draft.TexturePackPath, WindowSize: draft.WindowSize,
	})
	if persistenceErr != nil && !result.Committed {
		return fmt.Errorf("原子保存配置: %w", persistenceErr)
	}
	if persistenceErr == nil && !result.Committed {
		return errors.New("原子保存配置: 写入器成功返回但未越过 rename 提交点")
	}
	if persistenceErr != nil {
		// 完整错误只进日志；UI 使用下面的固定有界提示，避免路径或底层错误把
		// client ABI 的设置文本撑破。此路径绝不能再走普通失败处理，否则会
		// 在磁盘已是新值时错误保留旧 runtime。
		slog.Warn("设置已保存但父目录持久性同步异常", "error", persistenceErr)
	}

	previous := a.settings.Committed
	a.settings.Committed = draft
	a.settings.error = ""
	a.settings.status = boundedSettingsMessage("设置已保存")
	if persistenceErr != nil {
		a.settings.status = boundedSettingsMessage("设置已保存但持久性同步异常")
	}
	a.startupOptions.AudioVolume = draft.AudioVolume
	a.startupOptions.TexturePackPath = draft.TexturePackPath
	a.startupOptions.WindowSize = draft.WindowSize
	if previous.AudioVolume != draft.AudioVolume {
		a.replaceAudioPlayer(draft.AudioVolume)
	}
	if previous.WindowSize != draft.WindowSize {
		a.applyWindowSize(draft.WindowSize)
	}
	if textureChanged {
		status := "设置已保存；材质包将在下次启动时生效"
		if persistenceErr != nil {
			status = "设置已保存但持久性同步异常；材质包将在下次启动时生效"
		}
		a.settings.status = boundedSettingsMessage(status)
	}
	return nil
}

func resolveSettingsTexturePackPath(configPath, raw string) (string, error) {
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	return filepath.Abs(filepath.Join(filepath.Dir(configPath), raw))
}

func (a *Application) replaceAudioPlayer(volume float32) {
	var play func(cue audio.Cue)
	var closePlayer func()
	if a.startupDeps.NewAudioPlayer != nil {
		play, closePlayer = a.startupDeps.NewAudioPlayer(volume)
	}
	oldClose := a.closeAudio
	a.playCue = play
	a.closeAudio = closePlayer
	if oldClose != nil {
		oldClose()
	}
}

func (a *Application) applyWindowSize(size config.WindowSize) {
	if a.window == nil {
		return
	}
	width, height := size.Dimensions()
	a.window.SetContentSize(width, height)
	a.window.Poll()
	fitFramebuffer(a.window, interactiveFramebufferWidth, interactiveFramebufferHeight)
}

func (a *Application) reportSettingsError(err error) {
	slog.Error("保存设置失败", "error", err)
	a.settings.error = boundedSettingsMessage(err.Error())
	a.settings.status = ""
}

func boundedSettingsMessage(value string) string {
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, "�")
	}
	if len(value) <= maxSettingsMessageBytes {
		return value
	}
	value = value[:maxSettingsMessageBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
