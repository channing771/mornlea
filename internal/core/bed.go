package core

// 床是双格八形态方块：床尾与床头各占 4 个水平朝向。本文件是「朝向 ↔ 编号」
// 映射的唯一窗口——形态编号、方向解析与床头邻格偏移都从这里出，放置与采掘
// 执行方不得各自维护 ID 算式（与火把的 PlaceableBlockAtFace、门的 DoorDir
// 同一口径）。
//
// 方向编码沿用门先例：南 0、西 1、北 2、东 3；坐标约定同样是南 +Z、西 −X、
// 北 −Z、东 +X。形态名中的方向表示床头相对床尾所指的方向：南向床的床尾格
// 在南、床头格在其 +Z 邻格。床头段与床尾段同序平移 4，因此同方向的床头
// 编号恒等于床尾编号 + 4，双格配对可以走纯算术。

// IsBed 报告 id 是否是床方块（床尾或床头四向形态之一）。
// 床只包含 BedFootSouthID..BedHeadEastID 这 8 个稳定编号，其余任何方块（包括
// 未注册编号）均返回 false。半高碰撞、非不透明与流体占据等属性判定都以本谓词
// 为唯一成员依据，不得在别处复制编号区间。
func IsBed(id BlockID) bool {
	return id >= BedFootSouthID && id <= BedHeadEastID
}

// IsBedFoot 报告 id 是否是床尾形态（床双格中承载「入睡重生点」的基准格）。
func IsBedFoot(id BlockID) bool {
	return id >= BedFootSouthID && id <= BedFootEastID
}

// IsBedHead 报告 id 是否是床头形态。
func IsBedHead(id BlockID) bool {
	return id >= BedHeadSouthID && id <= BedHeadEastID
}

// BedDir 返回床形态的方向编码：南 0、西 1、北 2、东 3（与 DoorDir 同序）。
// 非床方块（含未注册与越界编号）返回 -1。
func BedDir(id BlockID) int {
	switch id {
	case BedFootSouthID, BedHeadSouthID:
		return 0
	case BedFootWestID, BedHeadWestID:
		return 1
	case BedFootNorthID, BedHeadNorthID:
		return 2
	case BedFootEastID, BedHeadEastID:
		return 3
	default:
		return -1
	}
}

// bedFootIDs 按方向编码（南/西/北/东）列出床尾形态，与方块编号冻结顺序一致。
var bedFootIDs = [4]BlockID{BedFootSouthID, BedFootWestID, BedFootNorthID, BedFootEastID}

// bedHeadOffsets 按方向编码列出床头格相对床尾格的偏移。
// 南 +Z、西 −X、北 −Z、东 +X，与门方向的坐标约定一致。
var bedHeadOffsets = [4]BlockPos{
	{X: 0, Y: 0, Z: 1},
	{X: -1, Y: 0, Z: 0},
	{X: 0, Y: 0, Z: -1},
	{X: 1, Y: 0, Z: 0},
}

// BedFootID 返回对应方向的床尾形态编号；方向越界（不在 0..3）返回 AirID。
// 放置执行方以本函数为唯一「方向 → 编号」入口，不得自建 switch。
func BedFootID(dir int) BlockID {
	if dir < 0 || dir > 3 {
		return AirID
	}
	return bedFootIDs[dir]
}

// BedHeadID 返回对应方向的床头形态编号；方向越界（不在 0..3）返回 AirID。
// 同方向的床头编号恒为床尾编号 + 4，这里仍走查表而不是算式，让「编号冻结
// 顺序」只声明在 const 一处。
func BedHeadID(dir int) BlockID {
	if dir < 0 || dir > 3 {
		return AirID
	}
	return bedFootIDs[dir] + 4
}

// BedHeadNeighbor 返回床尾格在指定朝向下的床头邻格；方向越界时返回原格。
// 只做纯坐标平移，不读世界：床头格是否加载、是否空气、下方是否有实心支撑，
// 仍由放置执行方逐条校验（门先例）。
func BedHeadNeighbor(foot BlockPos, dir int) BlockPos {
	if dir < 0 || dir > 3 {
		return foot
	}
	offset := bedHeadOffsets[dir]
	return BlockPos{X: foot.X + offset.X, Y: foot.Y + offset.Y, Z: foot.Z + offset.Z}
}
