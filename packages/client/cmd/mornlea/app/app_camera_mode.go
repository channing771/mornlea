//go:build darwin

package app

import (
	"log/slog"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/config"
)

// cycleCameraMode 按第一人称→背面→正面→第一人称推进本地视角。纯本地呈现
// 状态：不发送任何服务端消息，不触碰权威与预测状态。
func (a *Application) cycleCameraMode() {
	a.cameraMode = a.cameraMode.Next()
}

// handleCameraModeKey 消费一帧的 F5 电平：仅在未阻塞的上升沿切换视角。
// `f5WasDown` 逐帧更新（含阻塞帧），避免阻塞期按住的 F5 在恢复后被当成新的
// 上升沿。调用方按 `runGamePhase` 的既有键位栈口径传入阻塞条件。
func (a *Application) handleCameraModeKey(f5Down, blocked bool) {
	if f5Down && !a.f5WasDown && !blocked {
		a.cycleCameraMode()
	}
	a.f5WasDown = f5Down
}

// persistCameraMode 把当前本地视角模式写回配置文件，供下次进入世界恢复。
// 世界退出路径（退回主菜单、会话关闭、进程关闭）经它收摊：路径为空
// （benchmark/capture 不设保存目标）时跳过，落盘失败只告警——持久化是
// 本地偏好，绝不能破坏退出流程。越界值在装配点已落回，见 `NewWithDependencies`。
func (a *Application) persistCameraMode() {
	path := a.startupOptions.ConfigPath
	if path == "" {
		return
	}
	patchCameraMode := a.startupDeps.PatchCameraMode
	if patchCameraMode == nil {
		patchCameraMode = config.PatchCameraMode
	}
	if _, err := patchCameraMode(path, int(a.cameraMode)); err != nil {
		slog.Warn("保存视角模式失败", "error", err)
	}
}

// clampCameraMode 把装配输入的视角收敛到三态合法域：越界（手改配置或直接
// 构造）一律落回第一人称，与配置文件加载侧的钳制同口径。
func clampCameraMode(mode client.CameraMode) client.CameraMode {
	if !mode.Valid() {
		return client.CameraFirstPerson
	}
	return mode
}
