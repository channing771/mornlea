package core

// IsTorch 报告 id 是否是火把方块（落地形态或四向墙面形态之一）。
// 火把只包含 TorchStandingID 与 TorchWallPosXID..TorchWallNegZID 这 5 个稳定
// 编号，其余任何方块（包括未注册编号）均返回 false。零碰撞、非不透明与非
// 流体等属性判定都以本谓词为唯一成员依据，不得在别处复制编号区间。
func IsTorch(id BlockID) bool {
	return id >= TorchStandingID && id <= TorchWallNegZID
}

// BlockEmission 返回方块固定发出的方块光等级：发光方块 15、五种火把形态 14、
// 其余（含未知与越界编号）0。
//
// 本函数是全仓唯一的「方块发光」判定表：客户端注册表（internal/assets 的
// Emission 只做转调）现在就消费这一张表并经 mesh registry 快照送过 ABI 边界；
// 服务端生成判定（夜行者的黑暗判定）的消费者尚未存在，将来落地时同样只消费
// 这里、不建第二套；新增发光方块只允许改这里。
func BlockEmission(id BlockID) uint8 {
	switch {
	case id == LightBlockID:
		return 15
	case IsTorch(id):
		return 14
	default:
		return 0
	}
}

// BlockLightAttenuation 返回天空光穿过该方块时的额外衰减：八个流体编号 1、
// 其余（含五种火把与未知/越界编号）0。
//
// 本函数是全仓唯一的「天空光额外衰减」判定表，与 BlockEmission 同为光照语义
// 的单一事实源；方块光模型不消费本值（水与玻璃一样直接阻断方块光传播）。
func BlockLightAttenuation(id BlockID) uint8 {
	if IsFluid(id) {
		return 1
	}
	return 0
}

// PlaceableBlockAtFace 是「物品 × 命中面 → 写入方块形态」的唯一映射窗口：
// 服务端玩家放置与未来任何放置方都必须经本函数选取方块形态，不得各自维护
// item → shape 的 switch。
//
//   - 火把是唯一面向相关的物品：命中顶面（BlockFacePosY）得到落地形态，命中
//     四个水平侧面得到形态名与命中面同名的墙面形态；墙面形态的支撑格位于
//     命中面的反方向（face.Opposite()），该支撑契约由放置执行方校验。
//   - 底面（BlockFaceNegY）与非法面值（含 BlockFaceNone）拒绝：火把没有贴
//     天花板的形态。
//   - 其余可放置物品的形状与面无关，对任意合法面恒返回 ItemPlacement 的同一
//     方块；不可放置与未知物品在任何面上都拒绝。
//
// 本函数只回答「写成哪种方块」，不回答「能不能写」：目标格是否加载、可替换、
// 被玩家占位与支撑格是否合法，仍由放置执行方逐条校验。
func PlaceableBlockAtFace(item ItemID, face BlockFace) (BlockID, bool) {
	if item == ItemTorch {
		switch face {
		case BlockFacePosY:
			return TorchStandingID, true
		case BlockFacePosX:
			return TorchWallPosXID, true
		case BlockFaceNegX:
			return TorchWallNegXID, true
		case BlockFacePosZ:
			return TorchWallPosZID, true
		case BlockFaceNegZ:
			return TorchWallNegZID, true
		default:
			return AirID, false
		}
	}
	return ItemPlacement(item)
}
