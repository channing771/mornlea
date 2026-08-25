package physics_test

import (
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/nativeabi"
	"github.com/channing771/mornlea/internal/physics"
)

const (
	testCollisionHeaderBytes  = 64
	testCollisionCellBytes    = 196
	testCollisionOutputBytes  = 16
	testCollisionMaxCells     = 4096
	testCollisionMaxBytes     = 802880
	testCollisionRegularCells = 135
	testCollisionRegularBytes = 26524
)

func TestCollisionInputLayoutV1(t *testing.T) {
	box := [...]float32{-0.25, 0.125, 0.5, 1.25, 0.875, 1.5}
	prism := testCollisionCheckedPrism(core.BlockPos{X: -7, Y: 8, Z: -9}, [3]uint32{1, 1, 1})
	input := make([]byte, prism.bytes)
	testEncodeCollisionInputInto(
		input,
		prism,
		physics.State{Position: mgl32.Vec3{1.25, -2.5, 3.75}},
		mgl32.Vec3{-4.5, 5.25, -6.75},
		testCollisionWorld{{X: -7, Y: 8, Z: -9}: {
			Loaded: true,
			Count:  1,
			Boxes: [8]core.AABB{{
				Min: mgl32.Vec3{box[0], box[1], box[2]},
				Max: mgl32.Vec3{box[3], box[4], box[5]},
			}},
		}},
		true,
		0.6,
	)

	if len(input) != 260 || testCollisionOutputBytes != 16 ||
		testCollisionHeaderBytes+testCollisionMaxCells*testCollisionCellBytes != testCollisionMaxBytes {
		t.Fatalf("collision layout size=%d/%d，want 260/%d", len(input), testCollisionMaxBytes, 64+4096*196)
	}
	if got := string(input[:4]); got != "MGC1" {
		t.Fatalf("magic=%q，want MGC1", got)
	}
	if got := binary.LittleEndian.Uint32(input[4:8]); got != 1 {
		t.Fatalf("layout version=%d，want 1", got)
	}
	for index, value := range [...]float32{1.25, -2.5, 3.75, -4.5, 5.25, -6.75} {
		offset := 8 + index*4
		if index >= 3 {
			offset = 20 + (index-3)*4
		}
		if got := binary.LittleEndian.Uint32(input[offset : offset+4]); got != math.Float32bits(value) {
			t.Fatalf("header float %d bits=%08x，want %08x", index, got, math.Float32bits(value))
		}
	}
	if input[32] != 1 || input[33] != 0 || input[34] != 0 || input[35] != 0 {
		t.Fatalf("began_grounded/reserved=%v，want 1,0,0,0", input[32:36])
	}
	if got := binary.LittleEndian.Uint32(input[36:40]); got != math.Float32bits(0.6) {
		t.Fatalf("step_height bits=%08x，want %08x", got, math.Float32bits(0.6))
	}
	if input[64] != 1 || input[65] != 1 || input[66] != 0 || input[67] != 0 {
		t.Fatalf("cell header=%v，want 1,1,0,0", input[64:68])
	}
	for index, value := range [...]int32{-7, 8, -9} {
		if got := int32(binary.LittleEndian.Uint32(input[40+index*4 : 44+index*4])); got != value {
			t.Fatalf("origin[%d]=%d，want %d", index, got, value)
		}
		if got := binary.LittleEndian.Uint32(input[52+index*4 : 56+index*4]); got != 1 {
			t.Fatalf("dimension[%d]=%d，want 1", index, got)
		}
	}
	for index, value := range box {
		if got := math.Float32frombits(binary.LittleEndian.Uint32(input[68+index*4 : 72+index*4])); math.Float32bits(got) != math.Float32bits(value) {
			t.Fatalf("box[%d]=%v，want %v", index, got, value)
		}
	}
	for _, offset := range []int{33, 34, 35, 66, 67} {
		if input[offset] != 0 {
			t.Fatalf("reserved byte %d=%d，want 0", offset, input[offset])
		}
	}
}

func putTestCollisionVec3(output []byte, value mgl32.Vec3) {
	for index := range 3 {
		putTestCollisionFloat(output[index*4:index*4+4], value[index])
	}
}

func putTestCollisionFloat(output []byte, value float32) {
	binary.LittleEndian.PutUint32(output, math.Float32bits(value))
}

type testCollisionPrism struct {
	origin     core.BlockPos
	dimensions [3]uint32
	cells      int
	bytes      int
}

type testCollisionResult struct {
	position   mgl32.Vec3
	clipped    [3]bool
	onGround   bool
	usedStep   bool
	hitUnknown bool
}

type testCollisionWorld map[core.BlockPos]physics.CollisionBoxSet

func (world testCollisionWorld) CollisionBoxes(position core.BlockPos) physics.CollisionBoxSet {
	if set, ok := world[position]; ok {
		return set
	}
	return physics.CollisionBoxSet{Loaded: true}
}

func testCollisionPrismFor(position, displacement mgl32.Vec3, stepHeight float32) testCollisionPrism {
	halfWidth := physics.PlayerWidth / 2
	minimum := mgl32.Vec3{
		min(position.X(), position.X()+displacement.X()) - halfWidth - physics.CollisionEpsilon,
		position.Y() + min(float32(0), displacement.Y(), stepHeight) - physics.GroundProbe - physics.CollisionEpsilon,
		min(position.Z(), position.Z()+displacement.Z()) - halfWidth - physics.CollisionEpsilon,
	}
	maximum := mgl32.Vec3{
		max(position.X(), position.X()+displacement.X()) + halfWidth + physics.CollisionEpsilon,
		position.Y() + max(float32(0), displacement.Y(), stepHeight) + physics.PlayerHeight + physics.CollisionEpsilon,
		max(position.Z(), position.Z()+displacement.Z()) + halfWidth + physics.CollisionEpsilon,
	}
	origin := core.BlockPos{
		X: testCollisionCheckedFloor(minimum.X()),
		Y: testCollisionCheckedFloor(minimum.Y()),
		Z: testCollisionCheckedFloor(minimum.Z()),
	}
	end := core.BlockPos{
		X: testCollisionCheckedFloor(maximum.X()),
		Y: testCollisionCheckedFloor(maximum.Y()),
		Z: testCollisionCheckedFloor(maximum.Z()),
	}
	return testCollisionCheckedPrism(origin, [3]uint32{
		testCollisionCheckedDimension(origin.X, end.X),
		testCollisionCheckedDimension(origin.Y, end.Y),
		testCollisionCheckedDimension(origin.Z, end.Z),
	})
}

func testCollisionCheckedDimension(minimum, maximum int32) uint32 {
	dimension := int64(maximum) - int64(minimum) + 1
	if dimension <= 0 || dimension > 1<<32-1 {
		panic("physics: collision prism 尺寸不可表示")
	}
	return uint32(dimension)
}

func testCollisionCheckedFloor(value float32) int32 {
	floored := math.Floor(float64(value))
	if math.IsNaN(floored) || math.IsInf(floored, 0) || floored < -1<<31 || floored > 1<<31-1 {
		panic("physics: collision prism 坐标不可表示")
	}
	return int32(floored)
}

func testCollisionCheckedPrism(origin core.BlockPos, dimensions [3]uint32) testCollisionPrism {
	coordinates := [...]int32{origin.X, origin.Y, origin.Z}
	cells := uint64(1)
	for axis, dimension := range dimensions {
		if dimension == 0 || int64(coordinates[axis])+int64(dimension)-1 > 1<<31-1 {
			panic("physics: collision prism 尺寸非法")
		}
		cells *= uint64(dimension)
		if cells > testCollisionMaxCells {
			panic("physics: collision prism 超过 4096 cells")
		}
	}
	encodedBytes := uint64(testCollisionHeaderBytes) + cells*testCollisionCellBytes
	if encodedBytes > testCollisionMaxBytes {
		panic("physics: collision prism 编码长度溢出")
	}
	return testCollisionPrism{origin: origin, dimensions: dimensions, cells: int(cells), bytes: int(encodedBytes)}
}

func testEncodeCollisionInput(
	state physics.State,
	displacement mgl32.Vec3,
	source physics.CollisionSource,
	beganGrounded bool,
	stepHeight float32,
) []byte {
	prism := testCollisionPrismFor(state.Position, displacement, stepHeight)
	input := make([]byte, prism.bytes)
	testEncodeCollisionInputInto(input, prism, state, displacement, source, beganGrounded, stepHeight)
	return input
}

func testEncodeCollisionInputInto(
	input []byte,
	prism testCollisionPrism,
	state physics.State,
	displacement mgl32.Vec3,
	source physics.CollisionSource,
	beganGrounded bool,
	stepHeight float32,
) {
	if len(input) != prism.bytes {
		panic("physics: collision input 缓冲区长度非法")
	}
	clear(input)
	copy(input[:4], "MGC1")
	binary.LittleEndian.PutUint32(input[4:8], 1)
	putTestCollisionVec3(input[8:20], state.Position)
	putTestCollisionVec3(input[20:32], displacement)
	if beganGrounded {
		input[32] = 1
	}
	putTestCollisionFloat(input[36:40], stepHeight)
	for index, value := range [...]int32{prism.origin.X, prism.origin.Y, prism.origin.Z} {
		binary.LittleEndian.PutUint32(input[40+index*4:44+index*4], uint32(value))
	}
	for index, value := range prism.dimensions {
		binary.LittleEndian.PutUint32(input[52+index*4:56+index*4], value)
	}

	offset := testCollisionHeaderBytes
	for y := uint32(0); y < prism.dimensions[1]; y++ {
		for x := uint32(0); x < prism.dimensions[0]; x++ {
			for z := uint32(0); z < prism.dimensions[2]; z++ {
				position := core.BlockPos{
					X: prism.origin.X + int32(x),
					Y: prism.origin.Y + int32(y),
					Z: prism.origin.Z + int32(z),
				}
				set := source.CollisionBoxes(position)
				if set.Loaded {
					input[offset] = 1
				}
				count := min(int(set.Count), len(set.Boxes))
				input[offset+1] = byte(count)
				for boxIndex := range count {
					box := set.Boxes[boxIndex]
					components := [...]float32{
						box.Min.X(), box.Min.Y(), box.Min.Z(),
						box.Max.X(), box.Max.Y(), box.Max.Z(),
					}
					for componentIndex, value := range components {
						putTestCollisionFloat(input[offset+4+boxIndex*24+componentIndex*4:], value)
					}
				}
				offset += testCollisionCellBytes
			}
		}
	}
	if offset != len(input) {
		panic("physics: collision prism 编码不完整")
	}
}

func testNativeCollision(
	state physics.State,
	displacement mgl32.Vec3,
	source physics.CollisionSource,
	beganGrounded bool,
	stepHeight float32,
) testCollisionResult {
	prism := testCollisionPrismFor(state.Position, displacement, stepHeight)
	var regular [testCollisionRegularBytes]byte
	var input []byte
	if prism.cells <= testCollisionRegularCells {
		input = regular[:prism.bytes]
	} else {
		input = make([]byte, prism.bytes)
	}
	testEncodeCollisionInputInto(input, prism, state, displacement, source, beganGrounded, stepHeight)
	var output [testCollisionOutputBytes]byte
	nativeabi.CollisionResolve(input, output[:])
	return testDecodeCollisionOutput(output[:])
}

func testDecodeCollisionOutput(output []byte) testCollisionResult {
	if len(output) != testCollisionOutputBytes || output[12]&^byte(7) != 0 || output[13] > 1 || output[14] > 1 || output[15] > 1 {
		panic("physics: native collision output 非法")
	}
	result := testCollisionResult{
		position: mgl32.Vec3{
			math.Float32frombits(binary.LittleEndian.Uint32(output[0:4])),
			math.Float32frombits(binary.LittleEndian.Uint32(output[4:8])),
			math.Float32frombits(binary.LittleEndian.Uint32(output[8:12])),
		},
		onGround:   output[13] == 1,
		usedStep:   output[14] == 1,
		hitUnknown: output[15] == 1,
	}
	for axis, mask := range [...]byte{1, 2, 4} {
		result.clipped[axis] = output[12]&mask != 0
	}
	return result
}

type testRecordingCollisionSource struct {
	positions []core.BlockPos
	set       physics.CollisionBoxSet
}

func (source *testRecordingCollisionSource) CollisionBoxes(position core.BlockPos) physics.CollisionBoxSet {
	source.positions = append(source.positions, position)
	return source.set
}

func TestCollisionSnapshotUsesYXZOrderAndQueriesEachCellOnce(t *testing.T) {
	source := &testRecordingCollisionSource{set: physics.CollisionBoxSet{Loaded: true}}
	physics.Step(physics.State{
		Position: mgl32.Vec3{0.5, 1, 0.5},
		Velocity: mgl32.Vec3{0, 1.6, 0},
		OnGround: true,
	}, physics.Input{}, source)
	want := []core.BlockPos{
		{X: 0, Y: 0, Z: 0},
		{X: 0, Y: 1, Z: 0},
		{X: 0, Y: 2, Z: 0},
		{X: 0, Y: 3, Z: 0},
	}
	if !slices.Equal(source.positions, want) {
		t.Fatalf("query order=%v，want Y/X/Z %v", source.positions, want)
	}
}

func TestCollisionSnapshotClampsBoxCount(t *testing.T) {
	for _, rawCount := range []uint8{8, 9, 255} {
		t.Run(strconv.Itoa(int(rawCount)), func(t *testing.T) {
			set := physics.CollisionBoxSet{Loaded: true, Count: rawCount}
			for index := range set.Boxes {
				set.Boxes[index] = fullCube
			}
			state := physics.State{
				Position: mgl32.Vec3{0.5, 1, 0.5},
				Velocity: mgl32.Vec3{10, 0, 0},
				OnGround: true,
			}
			world := testCollisionWorld{{X: 1, Y: 1, Z: 0}: set}
			got := physics.Step(state, physics.Input{}, world)
			want := physics.StepResult{State: physics.State{
				Position: mgl32.Vec3{0.7, 0.92, 0.5},
				Velocity: mgl32.Vec3{0, -1.6, 0},
			}}
			if got != want {
				t.Fatalf("raw count %d result=%+v，want %+v", rawCount, got, want)
			}
		})
	}
}

func TestCollisionSnapshotAllows4096Cells(t *testing.T) {
	t.Cleanup(func() { physics.SetTunables(physics.DefaultTunables()) })
	tunables := physics.DefaultTunables()
	tunables.StepHeight = 13.2
	physics.SetTunables(tunables)
	source := &testCountingCollisionSource{}
	got := physics.Step(physics.State{
		Position: mgl32.Vec3{0.5, 1.1, 0.5},
		Velocity: mgl32.Vec3{5088.5, 1.6, 0},
		OnGround: true,
	}, physics.Input{}, source)
	if source.queries != testCollisionMaxCells {
		t.Fatalf("4096-cell production snapshot queries=%d，want %d", source.queries, testCollisionMaxCells)
	}
	if got.State.Position != (mgl32.Vec3{254.8, 1.1, 0.5}) {
		t.Fatalf("4096-cell production result=%+v，want complete movement", got)
	}
}

func TestCollisionSnapshotRejects4097BeforeQuery(t *testing.T) {
	t.Cleanup(func() { physics.SetTunables(physics.DefaultTunables()) })
	tunables := physics.DefaultTunables()
	tunables.StepHeight = 15
	physics.SetTunables(tunables)

	// x/y/z dimensions are exactly 241/17/1, for 4097 cells.
	source := &testCountingCollisionSource{}
	defer func() {
		if got := fmt.Sprint(recover()); got != "physics: collision prism 超过 4096 cells" {
			t.Errorf("4097-cell snapshot panic=%q，want stable capacity panic", got)
		}
		if source.queries != 0 {
			t.Fatalf("超限 snapshot 查询了 %d 个 cells，want 0", source.queries)
		}
	}()
	physics.Step(physics.State{
		Position: mgl32.Vec3{0.5, 1.1, 0.5},
		Velocity: mgl32.Vec3{4788.5, 1.6, 0},
		OnGround: true,
	}, physics.Input{}, source)
}

type testCountingCollisionSource struct{ queries int }

func (source *testCountingCollisionSource) CollisionBoxes(core.BlockPos) physics.CollisionBoxSet {
	source.queries++
	return physics.CollisionBoxSet{Loaded: true}
}

func TestCollisionConfiguredMaximumFitsRegularBuffer(t *testing.T) {
	prism := testStepPrismFor(mgl32.Vec3{0, 64, 0}, mgl32.Vec3{0, -10, 0}, mgl32.Vec3{1, 0, 1}, 1.5)
	if prism.cells > testStepRegularCells || prism.bytes > testStepRegularBytes {
		t.Fatalf("configured step prism=%d cells/%d bytes，want <=135/%d", prism.cells, prism.bytes, testStepRegularBytes)
	}
}

func TestCollisionSnapshotBeyondRegularBufferUsesExactAllocation(t *testing.T) {
	t.Cleanup(func() { physics.SetTunables(physics.DefaultTunables()) })
	tunables := physics.DefaultTunables()
	tunables.StepHeight = 15
	physics.SetTunables(tunables)
	source := &testCountingCollisionSource{}
	got := physics.Step(physics.State{
		Position: mgl32.Vec3{0.5, 1.1, 0.5},
		Velocity: mgl32.Vec3{168.5, 1.6, 0},
		OnGround: true,
	}, physics.Input{}, source)
	if source.queries != 170 {
		t.Fatalf("large production snapshot queries=%d，want 170", source.queries)
	}
	if got.State.Position != (mgl32.Vec3{8.8, 1.1, 0.5}) {
		t.Fatalf("large production snapshot result=%+v，want complete movement", got)
	}
}

// TestNativeCollisionRejectedUnknownStepKeepsOrdinaryHitUnknownFalse 锁定
// rejected-step 的隔离语义：step-up 尝试在上升段撞见 unknown 格（{X:0,Y:3,Z:0}）
// 被整体否决后，回落到的普通移动只查询了普通路径覆盖的格子——它既没有跨上半砖
// （`usedStep`=false），也不得把 step 尝试的 `hitUnknown` 泄漏进结果。期望值是
// 生产路径一次性采集固化的字面量：x=0.7 恰好贴墙、`clipped` 只在 x 轴。
func TestNativeCollisionRejectedUnknownStepKeepsOrdinaryHitUnknownFalse(t *testing.T) {
	world := testCollisionWorld{
		{X: 0, Y: 0, Z: 0}: {Loaded: true, Count: 1, Boxes: [8]core.AABB{fullCube}},
		{X: 1, Y: 0, Z: 0}: {Loaded: true, Count: 1, Boxes: [8]core.AABB{fullCube}},
		{X: 1, Y: 1, Z: 0}: {Loaded: true, Count: 1, Boxes: [8]core.AABB{{Max: mgl32.Vec3{1, 0.5, 1}}}},
		{X: 0, Y: 3, Z: 0}: {},
	}
	state := physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true}
	got := testNativeCollision(state, mgl32.Vec3{0.5, 0, 0}, world, true, 0.6)
	want := testCollisionResult{
		position: mgl32.Vec3{
			math.Float32frombits(0x3f333333),
			math.Float32frombits(0x3f800000),
			math.Float32frombits(0x3f000000),
		},
		clipped:    [3]bool{true, false, false},
		onGround:   true,
		usedStep:   false,
		hitUnknown: false,
	}
	if got != want {
		t.Fatalf("rejected step unknown=%+v，want %+v", got, want)
	}
}

// TestNativeCollisionConcurrentCalls 锁定 native collision 桥的并发确定性：
// 16 个 goroutine 各跑 100 次同输入调用，输出必须与串行基准逐位一致——
// 并发引入的任何非确定性（共享缓冲、数据竞争）都会在这里显形。
func TestNativeCollisionConcurrentCalls(t *testing.T) {
	world := testCollisionWorld{{X: 1, Y: 1, Z: 0}: {Loaded: true, Count: 1, Boxes: [8]core.AABB{fullCube}}}
	state := physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true}
	input := testEncodeCollisionInput(state, mgl32.Vec3{0.5, 0, 0}, world, true, 0.6)
	var reference [testCollisionOutputBytes]byte
	nativeabi.CollisionResolve(input, reference[:])
	want := testDecodeCollisionOutput(reference[:])
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 100 {
				var output [testCollisionOutputBytes]byte
				nativeabi.CollisionResolve(input, output[:])
				if got := testDecodeCollisionOutput(output[:]); got != want {
					t.Errorf("concurrent collision=%+v，want %+v", got, want)
					return
				}
			}
		}()
	}
	group.Wait()
}

func TestNativeCollisionBridgeDoesNotAllocate(t *testing.T) {
	state := physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true}
	displacement := mgl32.Vec3{0, -0.1, 0}
	prism := testCollisionPrismFor(state.Position, displacement, 0.6)
	var input [testCollisionRegularBytes]byte
	testEncodeCollisionInputInto(input[:prism.bytes], prism, state, displacement, testCollisionWorld{}, true, 0.6)
	var output [testCollisionOutputBytes]byte
	allocations := testing.AllocsPerRun(1000, func() {
		nativeabi.CollisionResolve(input[:prism.bytes], output[:])
	})
	if allocations != 0 {
		t.Fatalf("native collision bridge allocations=%v，want 0", allocations)
	}
}
