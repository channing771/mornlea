//go:build darwin

package app

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// hudLinkedHidden 是双手 viewmodel 与自身身体共用的 HUD 联动门：全景接管、
// 会话关闭、背包/容器打开或切出游戏相位时，两者都与 HUD 常显层同隐同现。
// 调用方在各自的视角门之外复用它，不另起一套相位口径。
func (a *Application) hudLinkedHidden(panorama bool) bool {
	return panorama || a.clientSessionClosed || a.inventoryOpen || a.menu.phase != MenuPhaseGame
}

// appendSelfAvatar 在第三人称（背面与正面）把自身身体追加进本帧 avatar 批次：
// 复用远端身体的同一编码管线（`InstanceEncoder` 按呈现位置差分累计行走摆动，
// 稳定键即得步态），不新增帧字段、TLV 段与 client ABI 版本。第一人称不追加
// （相机即眼睛，身体只会糊住近裁剪面）；自名牌不追加（会糊脸）。
//
// 位姿取本帧呈现相机：脚底为相机位减眼高（与交互循环同步相机同式），朝向
// 直通相机 yaw/pitch；键取玩家域 + 登录身份（与双手 viewmodel 同键，远端所
// 见自身颜色一致），无头与测试装配未保留身份时回落零值，仍确定可重放。
// 远端批次已达帧身体上限时自身让路，保证第三人称不把原本合法的满员帧变成
// 拒绝帧。纯呈现派生，不触碰权威位置与朝向。
func (a *Application) appendSelfAvatar(avatars []render.Avatar, panorama bool) []render.Avatar {
	if a.cameraMode == client.CameraFirstPerson || a.hudLinkedHidden(panorama) {
		return avatars
	}
	if len(avatars) >= maxFrameAvatars {
		return avatars
	}
	var player core.PlayerID
	if identity := a.startupOptions.Identity; identity != nil {
		player = identity.PlayerID
	}
	eye := physics.ActiveTunables().EyeHeight
	return append(avatars, render.Avatar{
		Key:      render.EntityKey{Kind: render.EntityPlayer, ID: [16]byte(player)},
		Position: a.camera.Pos.Sub(mgl32.Vec3{0, eye, 0}),
		Yaw:      a.camera.Yaw,
		Pitch:    a.camera.Pitch,
	})
}
