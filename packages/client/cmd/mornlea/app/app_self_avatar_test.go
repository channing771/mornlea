//go:build darwin

package app

// app_self_avatar_test.go：第三人称自身身体与双手 viewmodel 的视角互斥：
// 第一人称只显示双手、不渲染自身身体，第三人称（背面与正面）只渲染自身身体、
// 不显示双手像素；两者共用 HUD 联动门（背包/菜单/全景/断线时同隐同现）。

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// TestDeriveViewmodelInputNilInThirdPerson 锁定视角互斥的一半：第三人称下
// viewmodel 派生恒为 nil（无双手像素），第一人称保持既有呈现。
func TestDeriveViewmodelInputNilInThirdPerson(t *testing.T) {
	for _, mode := range []client.CameraMode{client.CameraThirdPersonBack, client.CameraThirdPersonFront} {
		app := &Application{}
		applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
		app.serverTick = 10
		app.cameraMode = mode
		if input := app.deriveViewmodelInput(false, render.BlockCrack{}); input != nil {
			t.Fatalf("视角 %d 派生=%+v，想要 nil（第三人称无双手）", mode, input)
		}
	}
	app := &Application{}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.serverTick = 10
	app.cameraMode = client.CameraFirstPerson
	if input := app.deriveViewmodelInput(false, render.BlockCrack{}); input == nil {
		t.Fatal("第一人称派生为 nil，想要非 nil（双手保持）")
	}
}

// TestAppendSelfAvatarAddsBodyOnlyInThirdPerson 锁定互斥的另一半：第一人称
// 不追加自身身体；第三人称背面与正面各追加恰一具，且位姿与呈现相机一致。
func TestAppendSelfAvatarAddsBodyOnlyInThirdPerson(t *testing.T) {
	login := core.PlayerID{0: 0x12, 6: 0x40, 8: 0x80, 15: 1}
	newThirdPersonApp := func(mode client.CameraMode) *Application {
		app := &Application{}
		app.startupOptions.Identity = &network.Identity{PlayerID: login, DisplayName: "Tester"}
		app.cameraMode = mode
		app.camera.Pos = mgl32.Vec3{10, 20, 30}
		app.camera.Yaw = 1.2
		app.camera.Pitch = -0.3
		return app
	}
	// 第一人称：批次原样返回，不渲染自己。
	first := &Application{}
	if got := first.appendSelfAvatar(nil, false); len(got) != 0 {
		t.Fatalf("第一人称追加了 %d 具身体，想要 0", len(got))
	}
	for _, mode := range []client.CameraMode{client.CameraThirdPersonBack, client.CameraThirdPersonFront} {
		app := newThirdPersonApp(mode)
		got := app.appendSelfAvatar(nil, false)
		if len(got) != 1 {
			t.Fatalf("视角 %d 追加了 %d 具身体，想要 1", mode, len(got))
		}
		self := got[0]
		if self.Key != (render.EntityKey{Kind: render.EntityPlayer, ID: [16]byte(login)}) {
			t.Fatalf("视角 %d 自身键=%+v，想要玩家域 + 登录身份", mode, self.Key)
		}
		eye := physics.ActiveTunables().EyeHeight
		wantFeet := mgl32.Vec3{10, 20 - eye, 30}
		if self.Position != wantFeet {
			t.Fatalf("视角 %d 自身脚底=%v，想要相机位减眼高 %v", mode, self.Position, wantFeet)
		}
		if self.Yaw != 1.2 || self.Pitch != -0.3 {
			t.Fatalf("视角 %d 自身朝向 yaw/pitch=%v/%v，想要相机的 1.2/-0.3", mode, self.Yaw, self.Pitch)
		}
	}
}

// TestAppendSelfAvatarHiddenWithHUD 锁定自身身体的 HUD 联动隐藏：背包打开、
// 切出游戏相位、全景接管与断线时，第三人称也不追加自身（与双手同隐同现）。
func TestAppendSelfAvatarHiddenWithHUD(t *testing.T) {
	newThirdPersonApp := func() *Application {
		app := &Application{}
		app.cameraMode = client.CameraThirdPersonBack
		app.camera.Pos = mgl32.Vec3{10, 20, 30}
		return app
	}
	t.Run("背包", func(t *testing.T) {
		app := newThirdPersonApp()
		app.inventoryOpen = true
		if got := app.appendSelfAvatar(nil, false); len(got) != 0 {
			t.Fatalf("开包追加了 %d 具身体，想要 0（随 HUD 隐藏）", len(got))
		}
	})
	t.Run("暂停", func(t *testing.T) {
		app := newThirdPersonApp()
		app.menu.phase = menuPhasePaused
		if got := app.appendSelfAvatar(nil, false); len(got) != 0 {
			t.Fatalf("暂停追加了 %d 具身体，想要 0（随 HUD 隐藏）", len(got))
		}
	})
	t.Run("菜单", func(t *testing.T) {
		app := newThirdPersonApp()
		app.menu.phase = MenuPhaseMenu
		if got := app.appendSelfAvatar(nil, false); len(got) != 0 {
			t.Fatalf("菜单追加了 %d 具身体，想要 0（随 HUD 隐藏）", len(got))
		}
	})
	t.Run("全景", func(t *testing.T) {
		app := newThirdPersonApp()
		if got := app.appendSelfAvatar(nil, true); len(got) != 0 {
			t.Fatalf("全景追加了 %d 具身体，想要 0", len(got))
		}
	})
	t.Run("断线", func(t *testing.T) {
		app := newThirdPersonApp()
		app.clientSessionClosed = true
		if got := app.appendSelfAvatar(nil, false); len(got) != 0 {
			t.Fatalf("断线追加了 %d 具身体，想要 0", len(got))
		}
	})
}

// TestAppendSelfAvatarYieldsAtFrameCap 锁定满员让路：远端批次已达帧身体上限
// 时自身不追加，保证第三人称不把原本合法的满员帧变成拒绝帧。
func TestAppendSelfAvatarYieldsAtFrameCap(t *testing.T) {
	app := &Application{}
	app.cameraMode = client.CameraThirdPersonBack
	app.camera.Pos = mgl32.Vec3{10, 20, 30}
	full := make([]render.Avatar, maxFrameAvatars)
	for index := range full {
		full[index].Key = render.EntityKey{Kind: render.EntityPlayer, ID: [16]byte{byte(index)}}
	}
	if got := app.appendSelfAvatar(full, false); len(got) != maxFrameAvatars {
		t.Fatalf("满员追加后 %d 具，想要 %d（自身让路）", len(got), maxFrameAvatars)
	}
	room := full[:maxFrameAvatars-1]
	if got := app.appendSelfAvatar(room, false); len(got) != maxFrameAvatars {
		t.Fatalf("留一位追加后 %d 具，想要 %d", len(got), maxFrameAvatars)
	}
}

// TestRenderFrameSelfAvatarMutualExclusion 锁定帧级互斥端到端：第一人称有双
// 手、无自身身体；切第三人称后有自身身体、无双手像素，且不带自名牌。
func TestRenderFrameSelfAvatarMutualExclusion(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 5, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		Health: 12, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	}); err != nil {
		t.Fatal(err)
	}
	applyViewmodelHotbar(t, app, viewmodelStoneStack, 0)
	app.SetServerTick(10)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("第一人称帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.viewmodelStream) == 0 {
		t.Fatal("第一人称 viewmodel 流为空，想要双手像素")
	}
	if len(app.avatarStream) != 0 {
		t.Fatalf("第一人称 avatar 流 %d 字节，想要 0（不渲染自己）", len(app.avatarStream))
	}
	app.cameraMode = client.CameraThirdPersonBack
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("第三人称帧 RenderFrame=(%v,%v)", rendered, err)
	}
	if len(app.viewmodelStream) != 0 {
		t.Fatalf("第三人称 viewmodel 流 %d 字节，想要 0（无双手像素）", len(app.viewmodelStream))
	}
	if len(app.avatarStream) != 6*96 {
		t.Fatalf("第三人称 avatar 流 %d 字节，想要 576（一具身体六部件）", len(app.avatarStream))
	}
	for _, tag := range app.remoteNameTags {
		if tag.Key.Kind == render.EntityPlayer {
			t.Fatalf("第三人称带自名牌 %+v，想要无名牌（糊脸）", tag)
		}
	}
}
