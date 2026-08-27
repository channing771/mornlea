//go:build darwin

package client

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

func TestEncodeUISettingsCrossLanguageGolden(t *testing.T) {
	got := EncodeUISettings(UISettings{
		Visible:         true,
		AudioVolume:     0.25,
		Window:          UISettingsWindow960x540,
		TexturePackPath: "packs/local",
		Dirty:           true,
		Status:          "材质包将在下次启动时生效",
		Error:           "保存失败",
	})

	want := make([]byte, 0, len(got))
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, math.Float32bits(0.25))
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = appendTestUIString(want, "packs/local")
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = appendTestUIString(want, "材质包将在下次启动时生效")
	want = appendTestUIString(want, "保存失败")
	if !bytes.Equal(got, want) {
		t.Fatalf("settings layout v2 字节不一致:\n got=%x\nwant=%x", got, want)
	}
}

func TestEncodeUISettingsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		frame UISettings
	}{
		{name: "nan audio", frame: UISettings{AudioVolume: float32(math.NaN()), Window: UISettingsWindow1280x720}},
		{name: "infinite audio", frame: UISettings{AudioVolume: float32(math.Inf(1)), Window: UISettingsWindow1280x720}},
		{name: "low audio", frame: UISettings{AudioVolume: -0.01, Window: UISettingsWindow1280x720}},
		{name: "high audio", frame: UISettings{AudioVolume: 1.01, Window: UISettingsWindow1280x720}},
		{name: "unknown window", frame: UISettings{AudioVolume: 0.5, Window: UISettingsWindow(99)}},
		{name: "path too long", frame: UISettings{AudioVolume: 0.5, Window: UISettingsWindow1280x720, TexturePackPath: strings.Repeat("a", 1025)}},
		{name: "status too long", frame: UISettings{AudioVolume: 0.5, Window: UISettingsWindow1280x720, Status: strings.Repeat("a", 257)}},
		{name: "error too long", frame: UISettings{AudioVolume: 0.5, Window: UISettingsWindow1280x720, Error: strings.Repeat("a", 257)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("非法 settings frame 应 panic")
				}
			}()
			EncodeUISettings(test.frame)
		})
	}
}

func TestDecodeUIEventBatchCrossLanguageGoldenAndOrder(t *testing.T) {
	batch := testUIEventBatch(
		testUIEventRecord(2, testSettingsChangedPayload(0.25, 2, "packs/local")),
		testUIEventRecord(1, binary.LittleEndian.AppendUint32(nil, 7)),
	)
	got, err := DecodeUIEventBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	want := []UIEvent{
		{
			Kind: UIEventSettingsChanged,
			Settings: UISettingsValues{
				AudioVolume:     0.25,
				Window:          UISettingsWindow960x540,
				TexturePackPath: "packs/local",
			},
		},
		{Kind: UIEventAction, ActionID: 7},
	}
	if len(got) != len(want) {
		t.Fatalf("事件数=%d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("event[%d]=%+v, want %+v", index, got[index], want[index])
		}
	}
}

func TestDecodeUIEventBatchEmpty(t *testing.T) {
	got, err := DecodeUIEventBatch(testUIEventBatch())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("空 batch 解出 %d 条事件", len(got))
	}
}

func TestMaxUIEventBatchBytesFits64MaxSettingsEvents(t *testing.T) {
	records := make([][]byte, 64)
	for index := range records {
		records[index] = testUIEventRecord(2, testSettingsChangedPayload(0.5, 3, strings.Repeat("a", 1024)))
	}
	batch := testUIEventBatch(records...)
	if len(batch) != maxUIEventBatchBytes {
		t.Fatalf("最大 batch 长度=%d, scratch=%d", len(batch), maxUIEventBatchBytes)
	}
	events, err := DecodeUIEventBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 64 {
		t.Fatalf("最大 batch 事件数=%d", len(events))
	}
}

func TestDecodeUIEventBatchRejectsInvalidMatrix(t *testing.T) {
	validAction := testUIEventBatch(testUIEventRecord(1, binary.LittleEndian.AppendUint32(nil, 9)))
	validSettings := testUIEventBatch(testUIEventRecord(2, testSettingsChangedPayload(0.5, 3, "pack")))
	tooMany := make([][]byte, 65)
	for index := range tooMany {
		tooMany[index] = testUIEventRecord(1, binary.LittleEndian.AppendUint32(nil, uint32(index)))
	}
	nonUTF8 := testSettingsChangedPayload(0.5, 1, "x")
	nonUTF8[len(nonUTF8)-1] = 0xff

	tests := []struct {
		name  string
		batch []byte
	}{
		{name: "short header", batch: []byte{1, 0, 0}},
		{name: "unknown batch layout", batch: mutateTestU32(validAction, 0, 2)},
		{name: "too many events", batch: testUIEventBatch(tooMany...)},
		{name: "unknown kind", batch: testUIEventBatch(testUIEventRecord(99, nil))},
		{name: "truncated record", batch: validAction[:len(validAction)-1]},
		{name: "action wrong payload", batch: testUIEventBatch(testUIEventRecord(1, []byte{1, 2, 3}))},
		{name: "settings truncated", batch: validSettings[:len(validSettings)-1]},
		{name: "settings non utf8", batch: testUIEventBatch(testUIEventRecord(2, nonUTF8))},
		{name: "settings path over limit", batch: testUIEventBatch(testUIEventRecord(2, testSettingsChangedPayload(0.5, 1, strings.Repeat("a", 1025))))},
		{name: "settings nan audio", batch: testUIEventBatch(testUIEventRecord(2, testSettingsChangedPayload(float32(math.NaN()), 1, "")))},
		{name: "settings infinite audio", batch: testUIEventBatch(testUIEventRecord(2, testSettingsChangedPayload(float32(math.Inf(1)), 1, "")))},
		{name: "settings low audio", batch: testUIEventBatch(testUIEventRecord(2, testSettingsChangedPayload(-0.1, 1, "")))},
		{name: "settings high audio", batch: testUIEventBatch(testUIEventRecord(2, testSettingsChangedPayload(1.1, 1, "")))},
		{name: "settings unknown window", batch: testUIEventBatch(testUIEventRecord(2, testSettingsChangedPayload(0.5, 99, "")))},
		{name: "settings payload tail", batch: testUIEventBatch(testUIEventRecord(2, append(testSettingsChangedPayload(0.5, 1, ""), 0)))},
		{name: "batch tail", batch: append(append([]byte(nil), validAction...), 0)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeUIEventBatch(test.batch); err == nil {
				t.Fatal("非法 batch 未被拒绝")
			}
		})
	}
}

func appendTestUIString(out []byte, value string) []byte {
	out = binary.LittleEndian.AppendUint32(out, uint32(len(value)))
	return append(out, value...)
}

func testSettingsChangedPayload(audio float32, window uint32, path string) []byte {
	out := binary.LittleEndian.AppendUint32(nil, math.Float32bits(audio))
	out = binary.LittleEndian.AppendUint32(out, window)
	return appendTestUIString(out, path)
}

func testUIEventRecord(kind uint32, payload []byte) []byte {
	out := binary.LittleEndian.AppendUint32(nil, kind)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(payload)))
	return append(out, payload...)
}

func testUIEventBatch(records ...[]byte) []byte {
	out := binary.LittleEndian.AppendUint32(nil, 1)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(records)))
	for _, record := range records {
		out = append(out, record...)
	}
	return out
}

func mutateTestU32(src []byte, offset int, value uint32) []byte {
	out := append([]byte(nil), src...)
	binary.LittleEndian.PutUint32(out[offset:offset+4], value)
	return out
}
