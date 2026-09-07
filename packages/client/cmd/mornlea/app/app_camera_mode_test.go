//go:build darwin

package app

import (
	"errors"
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/config"
)

func TestCycleCameraModeFollowsFirstBackFrontFirst(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	if app.cameraMode != client.CameraFirstPerson {
		t.Fatalf("初始视角 = %d，想要第一人称", app.cameraMode)
	}
	for step, want := range []client.CameraMode{
		client.CameraThirdPersonBack, client.CameraThirdPersonFront, client.CameraFirstPerson,
	} {
		app.cycleCameraMode()
		if app.cameraMode != want {
			t.Fatalf("第 %d 次切换后 = %d，想要 %d", step+1, app.cameraMode, want)
		}
	}
}

func TestCameraModeKeyCyclesOnRisingEdgeOnly(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	// 按住不放只切换一次。
	app.handleCameraModeKey(true, false)
	app.handleCameraModeKey(true, false)
	if app.cameraMode != client.CameraThirdPersonBack {
		t.Fatalf("按住 F5 后 = %d，想要背面且只切换一次", app.cameraMode)
	}
	// 阻塞（聊天/暂停/面板）时不切换，但边沿状态仍更新。
	app.handleCameraModeKey(false, false)
	app.handleCameraModeKey(true, true)
	if app.cameraMode != client.CameraThirdPersonBack {
		t.Fatalf("阻塞中 F5 切换了视角 = %d", app.cameraMode)
	}
	// 松开后再次按下继续循环。
	app.handleCameraModeKey(false, false)
	app.handleCameraModeKey(true, false)
	if app.cameraMode != client.CameraThirdPersonFront {
		t.Fatalf("再次按下后 = %d，想要正面", app.cameraMode)
	}
}

func TestCameraModeSurvivesSessionReset(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	app.cameraMode = client.CameraThirdPersonBack
	app.resetSessionOwnedState()
	if app.cameraMode != client.CameraThirdPersonBack {
		t.Fatalf("会话重置后视角 = %d，想要跨世界保留的背面", app.cameraMode)
	}
}

func TestPersistCameraModeWritesPathAndMode(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	var gotPath string
	var gotMode int
	calls := 0
	app.startupOptions.ConfigPath = "/tmp/camera-mode-test-config.json"
	app.startupDeps.PatchCameraMode = func(path string, mode int) (config.PersistenceResult, error) {
		calls++
		gotPath, gotMode = path, mode
		return config.PersistenceResult{Committed: true}, nil
	}
	app.cameraMode = client.CameraThirdPersonFront
	app.persistCameraMode()
	if calls != 1 || gotPath != "/tmp/camera-mode-test-config.json" || gotMode != 2 {
		t.Fatalf("持久化调用 = %d 次 %q/%d，想要 1 次且模式 2", calls, gotPath, gotMode)
	}
}

func TestPersistCameraModeSkipsEmptyPathAndToleratesFailure(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	app.startupDeps.PatchCameraMode = func(string, int) (config.PersistenceResult, error) {
		t.Fatal("空路径不应触碰存储")
		return config.PersistenceResult{}, nil
	}
	app.persistCameraMode()

	app.startupOptions.ConfigPath = "/tmp/camera-mode-test-config.json"
	app.startupDeps.PatchCameraMode = func(string, int) (config.PersistenceResult, error) {
		return config.PersistenceResult{}, errors.New("磁盘不可写")
	}
	// 落盘失败只告警，不得破坏世界退出流程。
	app.persistCameraMode()
}

func TestCloseClientSessionPersistsCameraMode(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	var gotMode int
	calls := 0
	app.startupOptions.ConfigPath = "/tmp/camera-mode-test-config.json"
	app.startupDeps.PatchCameraMode = func(_ string, mode int) (config.PersistenceResult, error) {
		calls++
		gotMode = mode
		return config.PersistenceResult{Committed: true}, nil
	}
	app.cameraMode = client.CameraThirdPersonBack
	app.CloseClientSession(nil)
	if calls != 1 || gotMode != 1 {
		t.Fatalf("会话关闭持久化 = %d 次模式 %d，想要 1 次模式 1", calls, gotMode)
	}
}

func TestQuitToMenuPersistsCameraMode(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	var gotMode int
	calls := 0
	app.startupOptions.ConfigPath = "/tmp/camera-mode-test-config.json"
	app.startupDeps.PatchCameraMode = func(_ string, mode int) (config.PersistenceResult, error) {
		calls++
		gotMode = mode
		return config.PersistenceResult{Committed: true}, nil
	}
	app.menu.phase = menuPhasePaused
	app.cameraMode = client.CameraThirdPersonFront
	app.quitToMenuFromPause()
	if calls != 1 || gotMode != 2 {
		t.Fatalf("退回主菜单持久化 = %d 次模式 %d，想要 1 次模式 2", calls, gotMode)
	}
	if app.cameraMode != client.CameraThirdPersonFront {
		t.Fatalf("退回主菜单后内存视角 = %d，想要保留正面", app.cameraMode)
	}
}
