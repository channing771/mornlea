//go:build darwin

package render

import (
	"encoding/binary"
	"math"

	"github.com/channing771/mornlea/packages/client/assets"
)

// 本文件为 rust-client-render-entities 提供最小导出面:把各 pass 已有的
// CPU 编码结果(80B avatar 实例、64B 名牌实例等)以只读字节流暴露给
// 平行 Rust 渲染器的帧装配。所有函数只是既有内部逻辑的复用出口,
// 不改变任何渲染行为。

// InstanceEncoder 持有实例编码的复用缓冲:热路径(每帧编码)零分配。
type InstanceEncoder struct {
	ordered []Avatar
	parts   []avatarPart
}

// EncodeAvatarInstances 把插值后的 avatars 编码为 80 字节/实例的字节流,
// 与 AvatarRenderer.Render 的内部编码逐字节一致。dst 会被重置复用。
func (e *InstanceEncoder) EncodeAvatarInstances(dst []byte, avatars []Avatar) []byte {
	e.ordered = orderedAvatarsInto(e.ordered[:0], avatars)
	e.parts = buildOrderedAvatarParts(e.parts[:0], e.ordered)
	dst = growEncodeBuffer(dst, len(e.parts)*avatarInstanceBytes)
	encodeAvatarPartsInto(dst, e.parts)
	return dst
}

// EncodeItemDropInstances 把掉落物编码为 80 字节/实例的字节流,
// 与 ItemDropRenderer.Render 的内部编码逐字节一致。
func (e *InstanceEncoder) EncodeItemDropInstances(dst []byte, serverTick uint64, drops []ItemDrop) []byte {
	e.parts = buildItemDropParts(e.parts[:0], serverTick, drops)
	dst = growEncodeBuffer(dst, len(e.parts)*avatarInstanceBytes)
	encodeAvatarPartsInto(dst, e.parts)
	return dst
}

// EncodeBlockOutlineInstances 把目标方块轮廓编码为 12×80 字节实例流;
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
