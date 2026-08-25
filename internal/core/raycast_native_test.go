package core

// raycast_native_test.go：native raycast 出口的 ABI 布局锁与 batch 驱动行为锁。
//
// 旧 Go DDA oracle 已随 change drop-go-test-oracles 删除，本文件不再做
// 「生产==冻结副本」差分对照；记录序列完整性改由布局锁（字节级编码断言）、
// 行为锁（回调传播、batch 边界、并发一致性）与 `FuzzRaycastBlocks` 的性质网
// 共同把守（design D2）。

import (
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"sync"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/nativeabi"
)

func TestRaycastInputCursorAndRecordLayoutV1(t *testing.T) {
	var input [40]byte
	encodeRaycastInput(
		input[:],
		mgl32.Vec3{1.25, -2.5, 3.75},
		mgl32.Vec3{-0.25, 0.5, -0.75},
		6.5,
	)
	if string(input[0:4]) != "MGR1" || binary.LittleEndian.Uint32(input[4:8]) != 1 {
		t.Fatalf("input identity=%q/%d", input[0:4], binary.LittleEndian.Uint32(input[4:8]))
	}
	for index, want := range [...]float32{1.25, -2.5, 3.75, -0.25, 0.5, -0.75, 6.5} {
		offset := 8 + index*4
		if got := binary.LittleEndian.Uint32(input[offset : offset+4]); got != math.Float32bits(want) {
			t.Fatalf("input float[%d] bits=%08x，想要 %08x", index, got, math.Float32bits(want))
		}
	}
	if input[36] != 0 || input[37] != 0 || input[38] != 0 || input[39] != 0 {
		t.Fatalf("input reserved=%v，想要全零", input[36:40])
	}

	var cursor [64]byte
	initializeRaycastCursor(cursor[:])
	if string(cursor[0:4]) != "MRC1" || binary.LittleEndian.Uint32(cursor[4:8]) != 1 {
		t.Fatalf("cursor identity=%q/%d", cursor[0:4], binary.LittleEndian.Uint32(cursor[4:8]))
	}
	for offset, value := range cursor[8:] {
		if value != 0 {
			t.Fatalf("fresh cursor[%d]=%d，想要 0", offset+8, value)
		}
	}

	recordBytes := [20]byte{
		0xf9, 0xff, 0xff, 0xff,
		0x08, 0x00, 0x00, 0x00,
		0xf7, 0xff, 0xff, 0xff,
		byte(BlockFacePosY), 0, 0, 0,
		0x00, 0x00, 0xa0, 0x3f,
	}
	record := decodeRaycastRecord(recordBytes[:], input[:])
	if record.block != (BlockPos{X: -7, Y: 8, Z: -9}) ||
		record.face != BlockFacePosY || math.Float32bits(record.distance) != math.Float32bits(1.25) {
		t.Fatalf("record=%+v", record)
	}
}

func TestNativeRaycastConcurrentCalls(t *testing.T) {
	const workers = 32
	type testCase struct {
		origin, direction mgl32.Vec3
		want              []raycastRecord
	}
	// 期望基线由同一条 native 路径在单线程下预先采集：本测试锁的是
	// 「并发 batch 调用与顺序调用逐位一致」（cursor 各自独立、互不串写），
	// 数据竞争本身由 -race 兜底；oracle 差分已由 drop-go-test-oracles 整体移除。
	corpus := make([]testCase, workers)
	for worker := range workers {
		origin := mgl32.Vec3{float32(worker%7) - 3.25, float32(worker%5) + 0.5, -2.75}
		direction := mgl32.Vec3{1, float32(worker%3) - 1, -0.25}
		corpus[worker] = testCase{
			origin:    origin,
			direction: direction,
			want:      nativeRaycastRecords(origin, direction, 96),
		}
	}
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(workers)
	errors := make(chan string, workers)
	for worker := range workers {
		go func() {
			defer group.Done()
			<-start
			test := corpus[worker]
			actual := nativeRaycastRecords(test.origin, test.direction, 96)
			if mismatch := raycastRecordMismatch(actual, test.want); mismatch != "" {
				errors <- mismatch
			}
		}()
	}
	close(start)
	group.Wait()
	close(errors)
	for mismatch := range errors {
		t.Error(mismatch)
	}
}

func TestRaycastBlocksDoesNotAllocate(t *testing.T) {
	solid := func(BlockPos) (bool, error) { return false, nil }
	origin := mgl32.Vec3{0.5, 0.5, 0.5}
	direction := mgl32.Vec3{1, 0.73, 0.41}
	allocations := testing.AllocsPerRun(1000, func() {
		if _, _, err := RaycastBlocks(origin, direction, 32, solid); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("RaycastBlocks allocations=%v，想要 0", allocations)
	}
}

func TestRaycastBlocksExtremeFiniteInputPreservesSecondCallbackError(t *testing.T) {
	origin := mgl32.Vec3{float32(math.MaxInt32), 0.5, 0.5}
	direction := mgl32.Vec3{1e-30, 1, 0}
	want := [...]BlockPos{{X: math.MinInt32}, {X: math.MinInt32 + 1}}
	sentinel := errors.New("extreme ray sentinel")
	for _, test := range []struct {
		name    string
		raycast func(mgl32.Vec3, mgl32.Vec3, float32, func(BlockPos) (bool, error)) (RayHit, bool, error)
	}{
		{name: "native", raycast: RaycastBlocks},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			var visited [2]BlockPos
			_, found, err := test.raycast(origin, direction, 1, func(position BlockPos) (bool, error) {
				if calls < len(visited) {
					visited[calls] = position
				}
				calls++
				if calls == len(want) {
					return false, sentinel
				}
				return false, nil
			})
			if err != sentinel || found || calls != len(want) {
				t.Fatalf("found/err/calls=%v/%v/%d，想要 false/sentinel/%d", found, err, calls, len(want))
			}
			if visited != want {
				t.Fatalf("callback 顺序=%+v，想要 %+v", visited, want)
			}
		})
	}
}

func TestRaycastBlocksExtremeFiniteInputPreservesSecondCellHit(t *testing.T) {
	origin := mgl32.Vec3{float32(math.MaxInt32), 0.5, 0.5}
	direction := mgl32.Vec3{1e-30, 1, 0}
	target := BlockPos{X: math.MinInt32 + 1}
	for _, test := range []struct {
		name    string
		raycast func(mgl32.Vec3, mgl32.Vec3, float32, func(BlockPos) (bool, error)) (RayHit, bool, error)
	}{
		{name: "native", raycast: RaycastBlocks},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			hit, found, err := test.raycast(origin, direction, 1, func(position BlockPos) (bool, error) {
				calls++
				return calls == 2, nil
			})
			// int32 回绕场景下真实 tMax 溢出，权威契约允许派生的 −Inf 距离
			// （见 `raycastRecordDistanceIsValid`），此处连同命中格与进入面一起钉住。
			if err != nil || !found || calls != 2 || hit.Block != target || hit.Face != BlockFaceNegX ||
				!math.IsInf(float64(hit.Distance), -1) {
				t.Fatalf("hit/found/err/calls=%+v/%v/%v/%d", hit, found, err, calls)
			}
		})
	}
}

func TestRaycastBlocksExtremeFiniteInputContinuesAcrossBatch(t *testing.T) {
	extreme := float32(math.MaxInt32)
	for _, ray := range []struct {
		name              string
		origin, direction mgl32.Vec3
		target            BlockPos
	}{
		{name: "single axis finite delta", origin: mgl32.Vec3{extreme, 0.5, 0.5}, direction: mgl32.Vec3{1e-30, 1, 0}, target: BlockPos{X: math.MinInt32 + 64}},
		{name: "single axis infinite delta", origin: mgl32.Vec3{extreme, 0.5, 0.5}, direction: mgl32.Vec3{1e-40, 1, 0}, target: BlockPos{X: math.MinInt32 + 64}},
		{name: "untouched infinite delta after negative infinity", origin: mgl32.Vec3{extreme, extreme, 0.5}, direction: mgl32.Vec3{1e-30, 1e-40, 1}, target: BlockPos{X: math.MinInt32 + 64, Y: math.MinInt32}},
		{name: "untouched infinite delta after NaN", origin: mgl32.Vec3{extreme, extreme, 0.5}, direction: mgl32.Vec3{1e-40, 1e-40, 1}, target: BlockPos{X: math.MinInt32 + 64, Y: math.MinInt32}},
	} {
		t.Run(ray.name, func(t *testing.T) {
			sentinel := errors.New("cross-batch sentinel")
			calls := 0
			var visited [65]BlockPos
			_, found, err := RaycastBlocks(ray.origin, ray.direction, 1, func(position BlockPos) (bool, error) {
				if calls < len(visited) {
					visited[calls] = position
				}
				calls++
				if calls == 65 {
					return false, sentinel
				}
				return false, nil
			})
			if err != sentinel || found || calls != 65 {
				t.Fatalf("found/err/calls=%v/%v/%d，想要 false/sentinel/65", found, err, calls)
			}
			if visited[64] != ray.target {
				t.Fatalf("callback[64]=%+v，想要 %+v", visited[64], ray.target)
			}
		})
	}
}

func TestDecodeRaycastRecordRejectsInvalidConsumedRecord(t *testing.T) {
	var rayInput [raycastInputBytes]byte
	encodeRaycastInput(rayInput[:], mgl32.Vec3{0.5, 0.5, 0.5}, mgl32.Vec3{1, 0, 0}, 6)
	valid := make([]byte, raycastRecordBytes)
	valid[12] = byte(BlockFaceNegX)
	for _, mutate := range []func([]byte){
		func(record []byte) { record[12] = 6 },
		func(record []byte) { record[13] = 1 },
		func(record []byte) {
			binary.LittleEndian.PutUint32(record[16:20], math.Float32bits(float32(math.NaN())))
		},
	} {
		record := append([]byte(nil), valid...)
		mutate(record)
		func() {
			defer func() {
				if recover() == nil {
					t.Error("非法 consumed record 未 panic")
				}
			}()
			decodeRaycastRecord(record, rayInput[:])
		}()
	}
}

func TestDecodeRaycastRecordAllowsDerivedOverflowAfterCellWrap(t *testing.T) {
	for _, test := range []struct {
		directionX, distance float32
	}{
		{directionX: 1e-30, distance: float32(math.Inf(-1))},
		{directionX: 1e-40, distance: float32(math.NaN())},
	} {
		var rayInput [raycastInputBytes]byte
		encodeRaycastInput(
			rayInput[:],
			mgl32.Vec3{float32(math.MaxInt32), 0.5, 0.5},
			mgl32.Vec3{test.directionX, 1, 0},
			1,
		)
		var record [raycastRecordBytes]byte
		binary.LittleEndian.PutUint32(record[0:4], uint32(math.MaxInt32))
		record[12] = byte(BlockFaceNegX)
		binary.LittleEndian.PutUint32(record[16:20], math.Float32bits(test.distance))
		decodeRaycastRecord(record[:], rayInput[:])
	}
}

func nativeRaycastRecords(origin, direction mgl32.Vec3, maximum float32) []raycastRecord {
	length := math.Hypot(math.Hypot(float64(direction[0]), float64(direction[1])), float64(direction[2]))
	direction = direction.Mul(float32(1 / length))
	var input [raycastInputBytes]byte
	var cursor [raycastCursorBytes]byte
	var output [raycastOutputBytes]byte
	encodeRaycastInput(input[:], origin, direction, maximum)
	initializeRaycastCursor(cursor[:])
	records := make([]raycastRecord, 0, 64)
	for {
		count, done := nativeabi.RaycastBatch(input[:], cursor[:], output[:])
		for index := range count {
			records = append(records, decodeRaycastRecord(output[index*raycastRecordBytes:(index+1)*raycastRecordBytes], input[:]))
		}
		if done {
			return records
		}
	}
}

func raycastRecordMismatch(actual, want []raycastRecord) string {
	if len(actual) != len(want) {
		return "native raycast record count=" + strconv.Itoa(len(actual)) + "，想要 " + strconv.Itoa(len(want))
	}
	for index := range want {
		if actual[index].block != want[index].block || actual[index].face != want[index].face ||
			math.Float32bits(actual[index].distance) != math.Float32bits(want[index].distance) {
			return "native raycast record mismatch at " + strconv.Itoa(index)
		}
	}
	return ""
}
