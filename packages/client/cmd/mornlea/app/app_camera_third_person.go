//go:build darwin

package app

import (
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
)

// resolveRenderCamera 由眼睛位姿推导本帧渲染相机位姿：`a.camera` 恒为眼睛
// （与服务端交互射线同源的眼高），瞄准、挖掘与放置射线继续读它；渲染侧
// （视图投影、可见性、天气粒子、名牌）读本函数返回的后拉位姿。
// 全景接管时调用方直接用全景位姿，不经此函数。
func (a *Application) resolveRenderCamera() client.Camera {
	return client.ResolveThirdPersonCamera(a.camera, a.cameraMode, a.thirdPersonSolid())
}

// thirdPersonSolid 把只读世界镜像适配为防穿墙射线的完整不透明判据：与
// 天空光/遮挡共用全仓唯一的 `core.BlockOpaque` 表，不复制判定分支。
// 未加载与已失同步的格视为空气（按开阔地处理，不凭未知收缩）；世界高度
// 之外的格由 `Mirror.BlockAt` 直接报空气，同为空。
func (a *Application) thirdPersonSolid() func(core.BlockPos) (bool, error) {
	return func(position core.BlockPos) (bool, error) {
		id, loaded := a.mirror.BlockAt(core.Overworld, position)
		if !loaded {
			return false, nil
		}
		if chunk, ok := a.mirror.Chunk(core.Overworld, position.Chunk()); ok && chunk != nil && chunk.Desynced {
			return false, nil
		}
		return core.BlockOpaque(id), nil
	}
}
