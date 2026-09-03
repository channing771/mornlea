package companion

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/channing771/mornlea/internal/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是 companion 域 codec 的字节原语：与根包（历史）、chunk 与 player 包
// 的同名助手同源。按域拆分后各域持有一份同构副本，域内 codec 是副本的唯一
// 消费方，域间不共享原语包。float 与 itemstack 原语的线格式与 player 域
// 完全一致。

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
