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

// BlockOpaque 返回方块是否完全不透明（遮挡邻面、阻断光照传播）：已注册的普通
// 实心立方体 true；空气、玻璃、树叶、八个流体、全部作物、九个门形态与五种
// 火把形态 false；未注册与越界编号一律 false。
//
// 判据逐值承接自 internal/assets 的 Registry.Opaque 迁移前的实际实现（迁移前
// 的穷举矩阵由 block_properties_test.go 锁定），两处差异都有几何成因，不得
// 「简化」掉：门是厚度 3/16 的薄板、火把是零碰撞的发光形态，二者都不填满
// 格子，与玻璃/树叶/作物同属透明类；屏障虽不可见，但作为普通实心立方体保持
// true。作物非不透明还承托着「作物下方耕地仍被照亮」——客户端天空光 BFS 的
// 阻断判据就是本表经注册表的转调。
//
// 本函数是全仓唯一的「方块不透明」判定表：客户端注册表（internal/assets 的
// Opaque 只做转调，值再经 mesh registry 快照送过 ABI 边界）与服务端夜行者的
// 局部暗度判定都消费这里；新增透明或实心方块只允许改这里，不得在消费方复制
// 判定分支。
func BlockOpaque(id BlockID) bool {
	return RegisteredBlock(id) && id != AirID && id != GlassID &&
		id != LeavesID && !IsFluid(id) && !IsCrop(id) && !IsDoor(id) && !IsTorch(id)
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
