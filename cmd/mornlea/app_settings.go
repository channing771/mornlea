//go:build darwin

package main

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

// settingsValues 只保存设置页公开的三项值。材质路径始终是配置原文，绝不
// 被解析后的绝对路径反向污染。
type settingsValues struct {
	audioVolume     float32
	texturePackPath string
	windowSize      config.WindowSize
}

// settingsState 由 Go 拥有设置事务的已保存值、草稿与展示信息。dirty 不单独
// 存储，避免 Rust 回显或失败路径让布尔值与实际内容分叉。
type settingsState struct {
	committed settingsValues
	draft     settingsValues
	status    string
	error     string
}

func (state settingsState) dirty() bool {
	return state.draft != state.committed
}

func (state settingsState) uiSettings() client.UISettings {
	return client.UISettings{
		Visible:         true,
		AudioVolume:     state.draft.audioVolume,
		Window:          uiWindowFromConfig(state.draft.windowSize),
		TexturePackPath: state.draft.texturePackPath,
		Dirty:           state.dirty(),
		Status:          boundedSettingsMessage(state.status),
		Error:           boundedSettingsMessage(state.error),
	}
}

func settingsValuesFromUI(values client.UISettingsValues) (settingsValues, error) {
	window, ok := configWindowFromUI(values.Window)
	if !ok {
		return settingsValues{}, fmt.Errorf("windowSize: 未知 UI 预设 %d", values.Window)
	}
	result := settingsValues{
		audioVolume:     values.AudioVolume,
		texturePackPath: values.TexturePackPath,
		windowSize:      window,
	}
	if err := result.validate(); err != nil {
		return settingsValues{}, err
	}
	return result, nil

}

func (values settingsValues) validate() error {
	if math.IsNaN(float64(values.audioVolume)) || math.IsInf(float64(values.audioVolume), 0) ||
		values.audioVolume < 0 || values.audioVolume > 1 {
		return fmt.Errorf("audioVolume: 必须是 0..1 的有限数值，实际 %v", values.audioVolume)
	}
	width, height := values.windowSize.Dimensions()
	if width == 0 || height == 0 {
		return fmt.Errorf("windowSize: 不支持的预设 %q", values.windowSize)
	}
	if !utf8.ValidString(values.texturePackPath) {
		return errors.New("texturePackPath: 必须是合法 UTF-8")
	}
	if strings.ContainsAny(values.texturePackPath, "\r\n") {
		return errors.New("texturePackPath: 必须是单行文本")
	}
	if len(values.texturePackPath) > config.MaxTexturePackPathBytes {
		return fmt.Errorf("texturePackPath: %d 个 UTF-8 字节超过上限 %d",
			len(values.texturePackPath), config.MaxTexturePackPathBytes)
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
// 原子落盘 → 更新 committed → 替换运行时音频和窗口」执行。前四步失败时
// committed 与所有运行时资源均保持原状。
func (a *application) saveSettings() error {
	draft := a.settings.draft
	if err := draft.validate(); err != nil {
		return fmt.Errorf("校验设置: %w", err)
	}

	textureChanged := draft.texturePackPath != a.settings.committed.texturePackPath
	if textureChanged && draft.texturePackPath != "" {
		candidatePath, err := resolveSettingsTexturePackPath(a.startupOptions.ConfigPath, draft.texturePackPath)
		if err != nil {
			return fmt.Errorf("解析材质包候选 %q: %w", draft.texturePackPath, err)
		}
		if a.startupDeps.newRegistry == nil {
			return errors.New("校验材质包候选: registry 工厂不可用")
		}
		if _, err := a.startupDeps.newRegistry(candidatePath); err != nil {
			return fmt.Errorf("校验材质包候选 %q: %w", draft.texturePackPath, err)
		}
	}

	patchSettings := a.startupDeps.patchSettings
	if patchSettings == nil {
		patchSettings = config.PatchSettings
	}
	if err := patchSettings(a.startupOptions.ConfigPath, config.SettingsPatch{
		AudioVolume: draft.audioVolume, TexturePackPath: draft.texturePackPath, WindowSize: draft.windowSize,
	}); err != nil {
		return fmt.Errorf("原子保存配置: %w", err)
	}

	previous := a.settings.committed
	a.settings.committed = draft
	a.settings.error = ""
	a.settings.status = boundedSettingsMessage("设置已保存")
	a.startupOptions.AudioVolume = draft.audioVolume
	a.startupOptions.TexturePackPath = draft.texturePackPath
	a.startupOptions.WindowSize = draft.windowSize
	if previous.audioVolume != draft.audioVolume {
		a.replaceAudioPlayer(draft.audioVolume)
	}
	if previous.windowSize != draft.windowSize {
		a.applyWindowSize(draft.windowSize)
	}
	if textureChanged {
		a.settings.status = boundedSettingsMessage("设置已保存；材质包将在下次启动时生效")
	}
	return nil
}

func resolveSettingsTexturePackPath(configPath, raw string) (string, error) {
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	return filepath.Abs(filepath.Join(filepath.Dir(configPath), raw))
}

func (a *application) replaceAudioPlayer(volume float32) {
	var play func(cue audio.Cue)
	var closePlayer func()
	if a.startupDeps.newAudioPlayer != nil {
		play, closePlayer = a.startupDeps.newAudioPlayer(volume)
	}
	oldClose := a.closeAudio
	a.playCue = play
	a.closeAudio = closePlayer
	if oldClose != nil {
		oldClose()
	}
}

func (a *application) applyWindowSize(size config.WindowSize) {
	if a.window == nil {
		return
	}
	width, height := size.Dimensions()
	a.window.SetContentSize(width, height)
	a.window.Poll()
	fitFramebuffer(a.window, interactiveFramebufferWidth, interactiveFramebufferHeight)
}

func (a *application) reportSettingsError(err error) {
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
