package render

import (
	"bytes"
	"encoding/binary"
	"math"
	"slices"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// maxAvatars 是共享渲染通道的每帧身体上限（75 具 × 6 部件 = 450 个
	// 96-byte instance），与 Rust 侧 `AVATAR_MAX_INSTANCES` 同源同步；夜行
	// 者上线后按「玩家 + 伙伴 + 敌怪」的合计呈现预留，第 76 具在帧边界被
	// App 层稳定拒绝。
	maxAvatars          = 75
	avatarPartsPerBody  = 6
	maxAvatarParts      = maxAvatars * avatarPartsPerBody
	avatarInstanceBytes = 96

	// avatarMaterialSolid 是纯色分支的哨兵材质：与全部有效材质层号不相交，
	// Rust 侧据此走原纯色路径，像素与变更前逐字节一致。
	avatarMaterialSolid = ^uint32(0)

	avatarCameraOffset   = 0
	avatarCameraBytes    = 80
	avatarInstanceOffset = 256
	avatarInstanceSize   = maxAvatarParts * avatarInstanceBytes
	avatarIndirectOffset = avatarInstanceOffset + avatarInstanceSize
	avatarIndirectBytes  = 20
	avatarUploadBytes    = avatarIndirectOffset + avatarIndirectBytes
)

// EntityKind 区分共享渲染通道中的实体身份域。
type EntityKind uint8

const (
	// EntityPlayer 表示玩家身份域。
	EntityPlayer EntityKind = 1
	// EntityCompanion 表示伙伴身份域。
	EntityCompanion EntityKind = 2
	// EntityTarget 表示当前方块目标名牌域。
	EntityTarget EntityKind = 3
	// EntityHostile 表示夜行者等敌怪身份域；编号 3 已被 EntityTarget 实占，
	// 敌怪取下一个空闲值以保持键域两两不相交。
	EntityHostile EntityKind = 4
	// EntityPassive 表示被动牛身份域；追加在末位，不扰动既有编号。
	EntityPassive EntityKind = 5
)

// EntityKey 由身份域和独立的 16-byte ID 组成。
type EntityKey struct {
	Kind EntityKind
	ID   [16]byte
}

// HostileEntityKey 把夜行者的 u64 稳定 ID 写进 16-byte 键槽位（little-endian
// 占低 8 字节、高 8 字节保持零）：身份域由 Kind 隔离，ID 槽位只需在域内
// 一一对应。
func HostileEntityKey(id uint64) EntityKey {
	var key EntityKey
	key.Kind = EntityHostile
	for index := range 8 {
		key.ID[index] = byte(id >> (8 * index))
	}
	return key
}

// PassiveEntityKey 把被动牛的 u64 稳定 ID 写进 16-byte 键槽位（little-endian
// 占低 8 字节、高 8 字节保持零）：身份域由 Kind 隔离，ID 槽位只需在域内
// 一一对应。
func PassiveEntityKey(id uint64) EntityKey {
	var key EntityKey
	key.Kind = EntityPassive
	for index := range 8 {
		key.ID[index] = byte(id >> (8 * index))
	}
	return key
}

func compareEntityKeys(left, right EntityKey) int {
	if left.Kind < right.Kind {
		return -1
	}
	if left.Kind > right.Kind {
		return 1
	}
	return bytes.Compare(left.ID[:], right.ID[:])
}

// Avatar 是远端玩家或伙伴渲染所需的插值后姿态。`Roll` 与 `Flash` 只服务
// 被动牛的死亡保留呈现（侧倒滚转角与向红插值系数，由死亡相位函数赋值）；
// 其余身份域保持零值，零值时牛实例变换与颜色与变更前逐字节一致。
type Avatar struct {
	Key      EntityKey
	Position mgl32.Vec3
	Yaw      float32
	Pitch    float32
	Roll     float32
	Flash    float32
}

type avatarPart struct {
	transform mgl32.Mat4
	color     [4]float32
	// material 是实例的材质层号：纯色分支传哨兵，牛身传牛皮/牛头层。
	material uint32
}

func encodeAvatarPartsInto(dst []byte, parts []avatarPart) {
	for partIndex, part := range parts {
		offset := partIndex * avatarInstanceBytes
		for index, value := range part.transform {
			binary.LittleEndian.PutUint32(dst[offset+index*4:], math.Float32bits(value))
		}
		for index, value := range part.color {
			binary.LittleEndian.PutUint32(dst[offset+64+index*4:], math.Float32bits(value))
		}
		binary.LittleEndian.PutUint32(dst[offset+80:], part.material)
		clear(dst[offset+84 : offset+avatarInstanceBytes])
	}
}

// encodeAvatarCameraInto 写入视图投影矩阵，并在其后追加本帧固定 daylight。
func encodeAvatarFloat32sInto(dst []byte, values []float32) {
	for index, value := range values {
		binary.LittleEndian.PutUint32(dst[index*4:], math.Float32bits(value))
	}
}

func buildAvatarParts(dst []avatarPart, avatars []Avatar) []avatarPart {
	return buildOrderedAvatarParts(dst, orderedAvatarsInto(nil, avatars))
}

func orderedAvatarsInto(dst []Avatar, avatars []Avatar) []Avatar {
	ordered := append(dst, avatars...)
	slices.SortFunc(ordered, func(left, right Avatar) int {
		return compareEntityKeys(left.Key, right.Key)
	})
	return ordered
}

func buildOrderedAvatarParts(dst []avatarPart, ordered []Avatar) []avatarPart {
	for _, avatar := range ordered {
		if avatar.Key.Kind == EntityHostile {
			dst = appendHostileAvatarParts(dst, avatar)
			continue
		}
		if avatar.Key.Kind == EntityPassive {
			dst = appendPassiveAvatarParts(dst, avatar)
			continue
		}
		root := mgl32.Translate3D(avatar.Position[0], avatar.Position[1], avatar.Position[2]).Mul4(
			mgl32.HomogRotate3DY(avatar.Yaw),
		)
		base := avatarColor(avatar.Key)
		head := root.Mul4(mgl32.Translate3D(0, 1.4, 0)).
			Mul4(mgl32.HomogRotate3DX(avatar.Pitch)).
			Mul4(mgl32.Translate3D(0, 0.2, 0)).
			Mul4(mgl32.Scale3D(0.6, 0.4, 0.6))
		dst = append(dst,
			avatarPart{transform: head, color: avatarShade(base, 1.12), material: avatarMaterialSolid},
			avatarCuboid(root, mgl32.Vec3{0, 1.05, 0}, mgl32.Vec3{0.4, 0.7, 0.25}, base),
			avatarCuboid(root, mgl32.Vec3{-0.25, 1.05, 0}, mgl32.Vec3{0.1, 0.7, 0.25}, avatarShade(base, 0.82)),
			avatarCuboid(root, mgl32.Vec3{0.25, 1.05, 0}, mgl32.Vec3{0.1, 0.7, 0.25}, avatarShade(base, 0.82)),
			avatarCuboid(root, mgl32.Vec3{-0.1, 0.35, 0}, mgl32.Vec3{0.18, 0.7, 0.25}, avatarShade(base, 0.82)),
			avatarCuboid(root, mgl32.Vec3{0.1, 0.35, 0}, mgl32.Vec3{0.18, 0.7, 0.25}, avatarShade(base, 0.82)),
		)
	}
	return dst
}

// hostilePalette 是夜行者的原创固定调色：暗青基底（蓝绿主导）与灰紫头部
// （红蓝主导且互相接近）。它不进入玩家/伙伴共用的 per-ID 调色板，与全部
// 共用槽位不重合；数值为原创设计，不取自任何第三方资源。
var (
	hostileBaseColor = [4]float32{0.22, 0.48, 0.52, 0.9}
	hostileHeadColor = [4]float32{0.45, 0.4, 0.5, 0.9}
)

const (
	hostileLimbShade = float32(0.78)
	hostileHeadShade = float32(1.1)
)

// appendHostileAvatarParts 追加一只夜行者的 6 个 cuboid：与玩家同构的双臂
// 双腿直立骨架，但头身比例刻意不同——头部更大（0.72 宽）、躯干更短
// （0.55 高）、手臂更长（0.85），叠加灰紫头部与暗青躯干构成与玩家/伙伴
// 一眼可辨的原创轮廓。身体仍锚定在站位 Y（脚底），俯仰只作用于头部。
func appendHostileAvatarParts(dst []avatarPart, avatar Avatar) []avatarPart {
	root := mgl32.Translate3D(avatar.Position[0], avatar.Position[1], avatar.Position[2]).Mul4(
		mgl32.HomogRotate3DY(avatar.Yaw),
	)
	head := root.Mul4(mgl32.Translate3D(0, 1.5, 0)).
		Mul4(mgl32.HomogRotate3DX(avatar.Pitch)).
		Mul4(mgl32.Translate3D(0, 0.25, 0)).
		Mul4(mgl32.Scale3D(0.72, 0.5, 0.72))
	dst = append(dst,
		avatarPart{transform: head, color: avatarShade(hostileHeadColor, hostileHeadShade), material: avatarMaterialSolid},
		avatarCuboid(root, mgl32.Vec3{0, 1.0, 0}, mgl32.Vec3{0.34, 0.55, 0.22}, hostileBaseColor),
		avatarCuboid(root, mgl32.Vec3{-0.23, 0.98, 0}, mgl32.Vec3{0.1, 0.85, 0.2}, avatarShade(hostileBaseColor, hostileLimbShade)),
		avatarCuboid(root, mgl32.Vec3{0.23, 0.98, 0}, mgl32.Vec3{0.1, 0.85, 0.2}, avatarShade(hostileBaseColor, hostileLimbShade)),
		avatarCuboid(root, mgl32.Vec3{-0.09, 0.275, 0}, mgl32.Vec3{0.15, 0.55, 0.2}, avatarShade(hostileBaseColor, hostileLimbShade)),
		avatarCuboid(root, mgl32.Vec3{0.09, 0.275, 0}, mgl32.Vec3{0.15, 0.55, 0.2}, avatarShade(hostileBaseColor, hostileLimbShade)),
	)
	return dst
}

// passiveGrazeHeadPitch 是放牧低头位姿的牛头固定下压角（弧度）：牛面朝
// +X（头部相对身体的偏移方向），侧轴即 Z 轴，负角把面朝从 +X 压向前下方
// （约 −52°），吻部指向身前地面；取值是呈现侧的固定 pose 选择，不进任何
// 线上契约，数值由放牧位姿测试按弧度锁定。
const passiveGrazeHeadPitch = float32(-0.9)

// PassiveGrazeHeadPitch 把放牧标志映射为牛头俯仰角：置位返回固定下压角，
// 清位归零。位姿完全由权威镜像驱动，客户端不做任何推测或随机摆动；调用方
// （呈现装配）只做直通，不在别处复制该角度。
func PassiveGrazeHeadPitch(grazing bool) float32 {
	if grazing {
		return passiveGrazeHeadPitch
	}
	return 0
}

// passiveIdleNodPeriodTicks 是闲时点头的正弦周期（权威 tick）：64 tick 约
// 3 秒，40 tick 窗内必含极值，起伏肉眼可辨。
const passiveIdleNodPeriodTicks = 64

// passiveIdleNodAmplitude 是闲时点头的振幅（弧度）：0.09 约 5°，小幅不抢戏。
const passiveIdleNodAmplitude = float32(0.09)

// PassiveIdleNodPitch 由权威 tick 与牛 ID 派生闲时点头俯仰：慢速小幅正弦纯
// 函数（掉落旋转同先例，禁用墙钟）；同 `(tick, ID)` 重放逐帧相同，ID 参与错
// 相。只服务非放牧、非死亡的牛，调用方负责门控，本函数不读任何权威字段。
func PassiveIdleNodPitch(tick, id uint64) float32 {
	phase := 2 * math.Pi * float64((tick+id*7)%passiveIdleNodPeriodTicks) / passiveIdleNodPeriodTicks
	return passiveIdleNodAmplitude * float32(math.Sin(phase))
}

// PassiveDeathTicks 是被动牛死亡保留的权威 tick 数：保留体在 T+19 仍在、
// T+20 移除。渲染侧拥有该常量的呈现解释权；客户端镜像侧的同值常量与本值
// 由应用装配包的边界测试钉住一致（见 `app` 的死亡保留边界测试）。
const PassiveDeathTicks = 20

// PassiveDeathPhase 由 despawn 权威 tick、牛 ID 与当前 tick 派生死亡相位：
// 返回侧倒滚转角（0→90°）与向红插值系数（0→1）。与掉落物动画相位同形——
// 纯函数，不读墙钟、帧间隔或本地随机数；同 `(T, ID)` 序列重放逐帧相同；ID
// 参与红闪错相，避免同 tick 死亡的个体整齐划一。死亡当 tick 返回零值，调用
// 方零值分支因此与变更前逐字节一致。
func PassiveDeathPhase(deathTick, id, nowTick uint64) (roll, flash float32) {
	var elapsed uint64
	if nowTick > deathTick {
		elapsed = nowTick - deathTick
	}
	if elapsed > PassiveDeathTicks {
		elapsed = PassiveDeathTicks
	}
	progress := float32(elapsed) / PassiveDeathTicks
	roll = progress * math.Pi / 2
	offset := float32(id%8) * (math.Pi / 4)
	shimmer := 0.5 + 0.5*float32(math.Sin(float64(progress*4*math.Pi+offset)))
	flash = progress * (0.45 + 0.55*shimmer)
	return roll, flash
}

// appendPassiveAvatarParts 追加一头牛的 6 个 cuboid：横向躯干 + 4 短腿 +
// 头部的四足体型，与夜行者直立骨架一眼可辨。身体锚定在站位 Y（脚底），
// 俯仰只作用于牛头（`Avatar.Pitch` 由放牧位映射而来，复用既有的头部俯仰
// 通道）：牛面朝 +X，俯仰绕侧轴 Z 把面朝压向前下方；`Pitch` 为零时旋转即
// 单位阵，头部链与引入放牧位之前逐字节一致。各面采样材质贴图而非纯色：头部
// 采牛头层，其余五部件采牛皮层；颜色通道被着色器忽略，填中性白。
// 死亡保留体另由 `Avatar.Roll`（绕面朝轴 X 的整体滚转，0→90° 侧倒）与
// `Avatar.Flash`（中性白向红插值）修饰：两者零值时分支跳过，变换与颜色与
// 变更前逐字节一致。
func appendPassiveAvatarParts(dst []avatarPart, avatar Avatar) []avatarPart {
	root := mgl32.Translate3D(avatar.Position[0], avatar.Position[1], avatar.Position[2]).Mul4(
		mgl32.HomogRotate3DY(avatar.Yaw),
	)
	if avatar.Roll != 0 {
		root = root.Mul4(mgl32.HomogRotate3DX(avatar.Roll))
	}
	white := [4]float32{1, 1, 1, 1}
	if avatar.Flash != 0 {
		blend := min(max(avatar.Flash, 0), 1)
		white = [4]float32{1, 1 - 0.85*blend, 1 - 0.9*blend, 1}
	}
	hide := uint32(assets.LayerCowHide)
	headLayer := uint32(assets.LayerCowHead)
	// 牛头绕颈轴俯转：零俯仰时与旧链逐字节一致（直通分支），非零时头心绕颈
	// 点摆动下压，吻部（头包围盒最低点）落在站立平面上 0.5 格以内；旧链绕头
	// 心自转，任何角度都够不到 0.5（头心高 1.0、半对角仅 0.32）。
	head := root.Mul4(mgl32.Translate3D(0.7, 1.0, 0)).
		Mul4(mgl32.HomogRotate3DZ(avatar.Pitch)).
		Mul4(mgl32.Scale3D(0.45, 0.45, 0.45))
	if avatar.Pitch != 0 {
		head = root.Mul4(mgl32.Translate3D(0.35, 0.85, 0)).
			Mul4(mgl32.HomogRotate3DZ(avatar.Pitch)).
			Mul4(mgl32.Translate3D(0.35, 0.15, 0)).
			Mul4(mgl32.Scale3D(0.45, 0.45, 0.45))
	}
	dst = append(dst,
		avatarPart{transform: head, color: white, material: headLayer},
		avatarPart{
			transform: root.Mul4(mgl32.Translate3D(0, 0.85, 0)).
				Mul4(mgl32.Scale3D(1.1, 0.6, 0.55)),
			color:    white,
			material: hide,
		},
		avatarPart{
			transform: root.Mul4(mgl32.Translate3D(-0.4, 0.25, -0.18)).
				Mul4(mgl32.Scale3D(0.18, 0.5, 0.18)),
			color:    white,
			material: hide,
		},
		avatarPart{
			transform: root.Mul4(mgl32.Translate3D(-0.4, 0.25, 0.18)).
				Mul4(mgl32.Scale3D(0.18, 0.5, 0.18)),
			color:    white,
			material: hide,
		},
		avatarPart{
			transform: root.Mul4(mgl32.Translate3D(0.4, 0.25, -0.18)).
				Mul4(mgl32.Scale3D(0.18, 0.5, 0.18)),
			color:    white,
			material: hide,
		},
		avatarPart{
			transform: root.Mul4(mgl32.Translate3D(0.4, 0.25, 0.18)).
				Mul4(mgl32.Scale3D(0.18, 0.5, 0.18)),
			color:    white,
			material: hide,
		},
	)
	return dst
}

func avatarCuboid(root mgl32.Mat4, center, size mgl32.Vec3, color [4]float32) avatarPart {
	return avatarPart{
		transform: root.Mul4(mgl32.Translate3D(center[0], center[1], center[2])).
			Mul4(mgl32.Scale3D(size[0], size[1], size[2])),
		color:    color,
		material: avatarMaterialSolid,
	}
}

func avatarShade(color [4]float32, factor float32) [4]float32 {
	for channel := 0; channel < 3; channel++ {
		color[channel] = max(0.2, min(color[channel]*factor, 0.9))
	}
	return color
}

var avatarPalette = [...][4]float32{
	{0.82, 0.34, 0.30, 0.9},
	{0.32, 0.62, 0.86, 0.9},
	{0.38, 0.72, 0.36, 0.9},
	{0.88, 0.66, 0.28, 0.9},
	{0.68, 0.42, 0.28, 0.9},
	{0.34, 0.76, 0.84, 0.9},
	{0.86, 0.46, 0.68, 0.9},
	{0.54, 0.50, 0.88, 0.9},
	{0.82, 0.54, 0.26, 0.9},
	{0.42, 0.70, 0.58, 0.9},
	{0.88, 0.40, 0.42, 0.9},
	{0.72, 0.42, 0.82, 0.9},
	{0.46, 0.64, 0.84, 0.9},
	{0.76, 0.72, 0.30, 0.9},
	{0.28, 0.78, 0.68, 0.9},
	{0.84, 0.48, 0.32, 0.9},
}

// AvatarColor 用 PlayerID 的全部 16 bytes 计算稳定的 FNV-1a 调色板索引。
func AvatarColor(playerID core.PlayerID) [4]float32 {
	const (
		offset32 = uint32(2166136261)
		prime32  = uint32(16777619)
	)
	hash := offset32
	for _, value := range playerID {
		hash ^= uint32(value)
		hash *= prime32
	}
	return avatarPalette[hash%uint32(len(avatarPalette))]
}

func avatarColor(key EntityKey) [4]float32 {
	if key.Kind == EntityPlayer {
		return AvatarColor(core.PlayerID(key.ID))
	}
	if key.Kind == EntityHostile {
		// 夜行者使用固定原创调色：不按 ID 索引共用调色板，颜色域与玩家/
		// 伙伴全部槽位天然隔离。
		return hostileBaseColor
	}
	if key.Kind == EntityPassive {
		// 被动牛走贴图路径，颜色通道被着色器忽略，取中性白保持编码确定。
		return [4]float32{1, 1, 1, 1}
	}
	if key.Kind != EntityCompanion {
		panic("render: avatar color requires player, companion, hostile or passive key")
	}
	const (
		offset32             = uint32(2166136261)
		prime32              = uint32(16777619)
		companionColorDomain = "companion:"
	)
	hash := offset32
	for index := range len(companionColorDomain) {
		hash ^= uint32(companionColorDomain[index])
		hash *= prime32
	}
	for _, value := range key.ID {
		hash ^= uint32(value)
		hash *= prime32
	}
	return avatarPalette[hash%uint32(len(avatarPalette))]
}

func avatarPartBytes(parts []avatarPart) []byte {
	out := make([]byte, len(parts)*avatarInstanceBytes)
	for partIndex, part := range parts {
		offset := partIndex * avatarInstanceBytes
		for index, value := range part.transform {
			binary.LittleEndian.PutUint32(out[offset+index*4:], math.Float32bits(value))
		}
		for index, value := range part.color {
			binary.LittleEndian.PutUint32(out[offset+64+index*4:], math.Float32bits(value))
		}
		binary.LittleEndian.PutUint32(out[offset+80:], part.material)
		clear(out[offset+84 : offset+avatarInstanceBytes])
	}
	return out
}

func avatarFloat32Bytes(values []float32) []byte {
	out := make([]byte, len(values)*4)
	for index, value := range values {
		binary.LittleEndian.PutUint32(out[index*4:], math.Float32bits(value))
	}
	return out
}
