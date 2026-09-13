package hostile

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestHostileCodecV1FixtureMigratesToKindZero 覆盖「v1 旧档读入→再保存→新版本
// 落盘且原字节保留」：输入是**冻结的 v1 字节**（testdata/hostile-mobs-v1.bin，
// 本变更一字不改），不是当前编码器现场生成的负载——当前编码器已经写 v2，用
// 它"生成 v1"只会得到一份带 kind 尾字节的 v2 记录，迁移分支根本不会被执行，
// 用例会全绿而什么都没测。
//
// v1 记录没有 kind 字节，迁移语义是「恒 0」：全部读入为夜行者，其余字段逐
// 字段不变。
func TestHostileCodecV1FixtureMigratesToKindZero(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", "hostile-mobs-v1.bin"))
	if err != nil {
		t.Fatal(err)
	}
	// 冻结输入的自证：v1 golden 永远停在 schema 1，若被误用新编码器重写，
	// 这里立刻红。
	if schema := binary.LittleEndian.Uint32(golden[8:12]); schema != hostileSchemaV1 {
		t.Fatalf("v1 golden schema=%d，想要 %d", schema, hostileSchemaV1)
	}
	before := bytes.Clone(golden)
	decoded, err := Decode(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(golden, before) {
		t.Fatal("解码改写了输入字节：v1 只读迁移必须在下次保存前保留原文件")
	}
	want := fixtureHostileRecordsV1Migrated()
	if !reflect.DeepEqual(decoded.Records, want) {
		t.Fatalf("v1 迁移记录=%+v，想要 kind 恒 0 的 %+v", decoded.Records, want)
	}
	if decoded.Revision != 19 {
		t.Fatalf("v1 迁移 revision=%d，想要 19", decoded.Revision)
	}

	// 再保存：编码端只写当前 schema；信封身份段（magic、envelope 版本、
	// revision、count）逐位不变，payload 长度恰好每条记录加 1。
	// 「原字节保留」按记录断言：v2 记录是尾部追加，每条 v1 记录的 72 字节
	// 必须逐位成为对应 v2 记录的前缀、随后只跟 1 字节零 kind——多记录聚合
	// 文件的 kind 字节交错在记录之间，整段 payload 不是单调前缀。
	rewritten, err := Encode(HostileMobsSave{Revision: decoded.Revision, Records: decoded.Records})
	if err != nil {
		t.Fatal(err)
	}
	if schema := binary.LittleEndian.Uint32(rewritten[8:12]); schema != CurrentSchema {
		t.Fatalf("重写 schema=%d，想要 %d", schema, CurrentSchema)
	}
	if !bytes.Equal(rewritten[:8], golden[:8]) || !bytes.Equal(rewritten[12:24], golden[12:24]) {
		t.Fatal("v1 存档重写改动了信封身份段")
	}
	v1PayloadLength := len(golden) - hostileHeaderLength
	if got := binary.LittleEndian.Uint32(rewritten[24:28]); got != uint32(v1PayloadLength+len(want)) {
		t.Fatalf("重写 payload 长度=%d，想要 %d", got, v1PayloadLength+len(want))
	}
	for index := range want {
		v1Record := golden[hostileHeaderLength+index*hostileRecordLengthV1 : hostileHeaderLength+(index+1)*hostileRecordLengthV1]
		v2Record := rewritten[hostileHeaderLength+index*hostileRecordLength : hostileHeaderLength+(index+1)*hostileRecordLength]
		if !bytes.Equal(v2Record[:hostileRecordLengthV1], v1Record) {
			t.Fatalf("v1 记录 %d 的字节未逐位保留为 v2 记录前缀", index)
		}
		if kind := v2Record[hostileRecordLengthV1]; kind != 0 {
			t.Fatalf("v2 记录 %d 的迁移 kind=%d，想要 0", index, kind)
		}
	}
}
