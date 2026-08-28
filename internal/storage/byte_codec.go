package storage

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/storage/storagedef"
)

// 本文件是根包实体域 codec（companion/hostile）共用的字节原语：与 chunk 包
// chunk_codec_primitives.go、player 包 codec_primitives.go 同源。按域拆分后
// 各域包持有一份同构副本，后续任务把实体 codec 迁入各自子包时随域带走；
// player 域已迁出，这里保留的 float/itemstack 原语只为尚未迁走的
// companion/hostile codec 服务。

func appendU32(dst []byte, value uint32) []byte {
	return binary.LittleEndian.AppendUint32(dst, value)
}

func appendU64(dst []byte, value uint64) []byte {
	return binary.LittleEndian.AppendUint64(dst, value)
}

type byteDecoder struct {
	data   []byte
	offset int
}

func (d *byteDecoder) u8() (uint8, error) {
	bytes, err := d.take(1)
	if err != nil {
		return 0, err
	}
	return bytes[0], nil
}

func (d *byteDecoder) u16() (uint16, error) {
	bytes, err := d.take(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(bytes), nil
}

func (d *byteDecoder) u32() (uint32, error) {
	if d.remaining() < 4 {
		return 0, io.ErrUnexpectedEOF
	}
	value := binary.LittleEndian.Uint32(d.data[d.offset:])
	d.offset += 4
	return value, nil
}

func (d *byteDecoder) u64() (uint64, error) {
	if d.remaining() < 8 {
		return 0, io.ErrUnexpectedEOF
	}
	value := binary.LittleEndian.Uint64(d.data[d.offset:])
	d.offset += 8
	return value, nil
}

func (d *byteDecoder) magic(want [4]byte) error {
	got, err := d.take(len(want))
	if err != nil {
		return err
	}
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("unexpected magic")
		}
	}
	return nil
}

func (d *byteDecoder) take(length int) ([]byte, error) {
	if length < 0 || length > d.remaining() {
		return nil, io.ErrUnexpectedEOF
	}
	value := d.data[d.offset : d.offset+length]
	d.offset += length
	return value, nil
}

func (d *byteDecoder) remaining() int {
	return len(d.data) - d.offset
}

func corrupt(field string, err error) error {
	return fmt.Errorf("%w: %s: %v", storagedef.ErrCorrupt, field, err)
}

// appendF32/decodeF32/finitePlayerFloat 与 appendPlayerStack/decodePlayerStack
// 是 companion/hostile codec 仍在消费的同构副本：float 与 itemstack 的线格式
// 与 player 域完全一致，定义镜像 player 包的同名助手，companion/hostile 随域
// 迁走时一并带走。

func appendF32(dst []byte, value float32) []byte {
	return binary.LittleEndian.AppendUint32(dst, math.Float32bits(value))
}

func decodeF32(decoder *byteDecoder) (float32, error) {
	bits, err := decoder.u32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(bits), nil
}

func appendPlayerStack(dst []byte, stack core.ItemStack) []byte {
	dst = binary.LittleEndian.AppendUint16(dst, uint16(stack.Item))
	dst = append(dst, stack.Count)
	return binary.LittleEndian.AppendUint16(dst, stack.Durability)
}

func decodePlayerStack(decoder *byteDecoder) (core.ItemStack, error) {
	item, err := decoder.u16()
	if err != nil {
		return core.ItemStack{}, err
	}
	count, err := decoder.u8()
	if err != nil {
		return core.ItemStack{}, err
	}
	durability, err := decoder.u16()
	if err != nil {
		return core.ItemStack{}, err
	}
	return core.ItemStack{Item: core.ItemID(item), Count: count, Durability: durability}, nil
}

func finitePlayerFloat(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
