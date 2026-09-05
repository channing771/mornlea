//go:build darwin

package render

import (
	"encoding/binary"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
)

// 本文件为 rust-client-render-entities 提供最小导出面:把各 pass 已有的
// CPU 编码结果(96B avatar 实例、64B 名牌实例等)以只读字节流暴露给
// 平行 Rust 渲染器的帧装配。所有函数只是既有内部逻辑的复用出口,
// 不改变任何渲染行为。

// InstanceEncoder 持有实例编码的复用缓冲:热路径(每帧编码)零分配。
type InstanceEncoder struct {
	ordered []Avatar
	parts   []avatarPart
	// tracks 是摆动速度估计的呈现位置差分历史：键为实体键，值为上次编码
	// 的位置/tick/速度；每帧只保留本帧出现的键，有界于单帧身体数。
	tracks map[EntityKey]swingTrack
}

// swingTrack 记录单个实体上次编码时的呈现位置与权威 tick，`speed` 是据此
// 差分出的上次估计速度（格/tick）。
type swingTrack struct {
	pos   [3]float32
	tick  uint64
	speed float32
}

// EncodeAvatarInstances 把插值后的 avatars 编码为 96 字节/实例的字节流,
// 与 AvatarRenderer.Render 的内部编码逐字节一致。dst 会被重置复用。
//
// `tick` 是最后确认的权威 server tick：编码器据此与实体稳定 ID 派生四肢摆
// 动相位（见 `AvatarSwingAngle`），速度由呈现位置差分估计——同 tick 重复编
// 码沿用上次速度，tick 回退（场景切换重钉）时重新锚定。静止实体恒为中性位
// 姿，因此静态抓帧基线与机器速度无关。
func (e *InstanceEncoder) EncodeAvatarInstances(dst []byte, tick uint64, avatars []Avatar) []byte {
	e.ordered = orderedAvatarsInto(e.ordered[:0], avatars)
	e.applyLocomotionSwing(tick)
	e.parts = buildOrderedAvatarParts(e.parts[:0], e.ordered)
	dst = growEncodeBuffer(dst, len(e.parts)*avatarInstanceBytes)
	encodeAvatarPartsInto(dst, e.parts)
	return dst
}

// applyLocomotionSwing 为已排序的本帧身体逐个估计呈现速度并填写 `Swing`：
// 首见/回退锚定为 0，同 tick 沿用上次速度，前进时按位移差分重估；离场实体
// 的历史同步清理。死亡/低头让路由装配侧（`appendPassiveAvatarParts` 等）按
// 位姿门控，本函数只填角。
func (e *InstanceEncoder) applyLocomotionSwing(tick uint64) {
	if e.tracks == nil {
		e.tracks = make(map[EntityKey]swingTrack, len(e.ordered))
	}
	for index := range e.ordered {
		avatar := &e.ordered[index]
		var speed float32
		if last, ok := e.tracks[avatar.Key]; ok {
			switch {
			case tick > last.tick:
				speed = mgl32.Vec3(avatar.Position).Sub(mgl32.Vec3(last.pos)).Len() / float32(tick-last.tick)
			case tick == last.tick:
				speed = last.speed
			}
		}
		e.tracks[avatar.Key] = swingTrack{pos: [3]float32(avatar.Position), tick: tick, speed: speed}
		avatar.Swing = AvatarSwingAngle(tick, swingPhaseID(avatar.Key), speed)
	}
	if len(e.tracks) > len(e.ordered) {
		seen := make(map[EntityKey]struct{}, len(e.ordered))
		for index := range e.ordered {
			seen[e.ordered[index].Key] = struct{}{}
		}
		for key := range e.tracks {
			if _, ok := seen[key]; !ok {
				delete(e.tracks, key)
			}
		}
	}
}

// EncodeItemDropInstances 把掉落物编码为 96 字节/实例的字节流,
// 与 ItemDropRenderer.Render 的内部编码逐字节一致。
func (e *InstanceEncoder) EncodeItemDropInstances(dst []byte, serverTick uint64, drops []ItemDrop) []byte {
	e.parts = buildItemDropParts(e.parts[:0], serverTick, drops)
	dst = growEncodeBuffer(dst, len(e.parts)*avatarInstanceBytes)
	encodeAvatarPartsInto(dst, e.parts)
	return dst
}

// EncodeBlockOutlineInstances 把目标方块轮廓编码为 12×96 字节实例流;
// 不可见时返回空。
func (e *InstanceEncoder) EncodeBlockOutlineInstances(dst []byte, outline BlockOutline) []byte {
	if !outline.Visible {
		return dst[:0]
	}
	e.parts = buildBlockOutlineParts(e.parts[:0], outline.Position)
	dst = growEncodeBuffer(dst, len(e.parts)*avatarInstanceBytes)
	encodeAvatarPartsInto(dst, e.parts)
	return dst
}

// EncodeBlockCrackInstances 把采掘裂纹 overlay 编码为恰 1 个 80 字节实例的
// 字节流；不可见或阶段无效时返回空。字节布局是与 Rust 侧 crack pass 的
// 跨语言契约：0..63 mat4、64..68 atlas 层号 f32（LayerCrack0+stage）、
// 68..80 零填充。dst 会被重置复用；注意 `growEncodeBuffer` 不清零旧内容，
// 零填充区必须显式写零，否则复用缓冲会把上一帧的陈旧字节漏进实例流。
func (e *InstanceEncoder) EncodeBlockCrackInstances(dst []byte, crack BlockCrack) []byte {
	if !crack.valid() {
		return dst[:0]
	}
	dst = growEncodeBuffer(dst, blockCrackInstanceBytes)
	part := buildBlockCrackPart(crack.Position)
	for index, value := range part.transform {
		binary.LittleEndian.PutUint32(dst[index*4:], math.Float32bits(value))
	}
	binary.LittleEndian.PutUint32(
		dst[64:], math.Float32bits(float32(int(assets.LayerCrack0)+crack.Stage)),
	)
	clear(dst[68:blockCrackInstanceBytes])
	return dst
}

// EncodeBillboardCameraBytes 导出名牌 billboard 相机的 96 字节编码。
func EncodeBillboardCameraBytes(dst []byte, camera BillboardCamera) []byte {
	dst = growEncodeBuffer(dst, nameTagCameraBytes)
	encodeBillboardCamera(dst, camera)
	return dst
}

// FrameStreams 返回 Prepare 之后已编码的名牌背景与字形实例字节
// (只读视图,下一次 Prepare 前有效)。
func (renderer *NameTagRenderer) FrameStreams() (backgrounds, glyphs []byte) {
	backgrounds = renderer.upload[nameTagBackgroundOffset : nameTagBackgroundOffset+
		len(renderer.layout.backgrounds)*nameTagInstanceBytes]
	glyphs = renderer.upload[nameTagGlyphOffset : nameTagGlyphOffset+
		len(renderer.layout.glyphs)*nameTagInstanceBytes]
	return backgrounds, glyphs
}

// GlyphAtlasSize 导出字形图集边长,供装配方校验。
const GlyphAtlasSize = glyphAtlasSize

func growEncodeBuffer(dst []byte, size int) []byte {
	if cap(dst) < size {
		return make([]byte, size)
	}
	return dst[:size]
}

// NewNameTagLayouter 创建 layout-only 的名牌 renderer:只支持 Prepare 与
// FrameStreams,不创建任何 GPU 资源(生产切换后由 Rust 渲染器绘制)。
func NewNameTagLayouter(atlas GlyphSource) *NameTagRenderer {
	return &NameTagRenderer{
		atlas:   atlas,
		ordered: make([]NameTag, 0, maxNameTags),
		upload:  make([]byte, nameTagUploadBytes),
		layout: nameTagLayout{
			glyphs:      make([]nameTagGlyph, 0, maxNameTagGlyphs),
			backgrounds: make([]nameTagBackground, 0, maxNameTags),
		},
	}
}
