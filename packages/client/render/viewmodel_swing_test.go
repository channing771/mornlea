package render

import (
	"bytes"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定挥动相位的确定性：纯函数只读权威 tick 与触发沿，不读墙钟；
// 同输入序列逐帧相同，tick 回退重新锚定，陈旧与重复命中不触发挥动。

func viewmodelTestInput(player core.PlayerID, stack core.ItemStack, tick uint64) *ViewmodelInput {
	return &ViewmodelInput{Player: player, Selected: stack, Tick: tick}
}

func TestViewmodelMiningPhaseIsPure(t *testing.T) {
	first := ViewmodelMiningAngle(100, 90, ViewmodelTierSword)
	second := ViewmodelMiningAngle(100, 90, ViewmodelTierSword)
	if first != second {
		t.Fatalf("同输入两次相位 = %v/%v，想要逐帧相同", first, second)
	}
	if advanced := ViewmodelMiningAngle(101, 90, ViewmodelTierSword); advanced == first {
		t.Fatalf("tick 推进后相位仍为 %v，想要随权威 tick 推进", first)
	}
}

func TestViewmodelMiningStopsAtNeutral(t *testing.T) {
	player := core.PlayerID{7}
	stack := core.ItemStack{Item: core.ItemStonePickaxe, Count: 1}
	encoder := &ViewmodelEncoder{}
	mining := viewmodelTestInput(player, stack, 50)
	mining.Mining = true
	encoder.EncodeViewmodelInstances(nil, mining)
	// 锚定帧相位为零，推进一 tick 后进入挥动。
	second := viewmodelTestInput(player, stack, 51)
	second.Mining = true
	swinging := append([]byte(nil), encoder.EncodeViewmodelInstances(nil, second)...)
	stopped := viewmodelTestInput(player, stack, 52)
	rest := encoder.EncodeViewmodelInstances(nil, stopped)
	idle := &ViewmodelEncoder{}
	neutral := idle.EncodeViewmodelInstances(nil, stopped)
	if bytes.Equal(swinging, rest) {
		t.Fatalf("挖掘启停两帧字节一致，想要挥动与中立可辨")
	}
	if !bytes.Equal(rest, neutral) {
		t.Fatalf("overlay 清除后未回中立持握")
	}
}

func TestViewmodelAttackWindowSixFrames(t *testing.T) {
	player := core.PlayerID{9}
	stack := core.ItemStack{Item: core.ItemIronSword, Count: 1}
	encoder := &ViewmodelEncoder{}
	neutral := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, viewmodelTestInput(player, stack, 200))

	first := viewmodelTestInput(player, stack, 200)
	first.AttackTick = 200
	frames := make([][]byte, 0, ViewmodelAttackFrames+1)
	for frame := range ViewmodelAttackFrames + 1 {
		input := viewmodelTestInput(player, stack, 200+uint64(frame))
		input.AttackTick = 200
		frames = append(frames, append([]byte(nil), encoder.EncodeViewmodelInstances(nil, input)...))
	}
	if bytes.Equal(frames[0], neutral) {
		t.Fatalf("命中确认后第 1 帧仍为中立，想要起挥")
	}
	for frame := range ViewmodelAttackFrames {
		if bytes.Equal(frames[frame], neutral) {
			t.Fatalf("窗内第 %d 帧回到中立，想要窗满 6 帧才收", frame+1)
		}
	}
	if !bytes.Equal(frames[ViewmodelAttackFrames], neutral) {
		t.Fatalf("第 6 帧后未回中立持握")
	}
}

func TestViewmodelStaleAndDuplicateHitsIgnored(t *testing.T) {
	player := core.PlayerID{11}
	stack := core.ItemStack{Item: core.ItemIronSword, Count: 1}
	triggerAt := func(encoder *ViewmodelEncoder, tick, attack uint64) []byte {
		input := viewmodelTestInput(player, stack, tick)
		input.AttackTick = attack
		return append([]byte(nil), encoder.EncodeViewmodelInstances(nil, input)...)
	}
	// 同一确认重复到达：窗口按原帧序继续，与清零后续输入的编码一致。
	duplicated := &ViewmodelEncoder{}
	triggerAt(duplicated, 300, 300)
	duplicatedSecond := triggerAt(duplicated, 301, 300)
	cleared := &ViewmodelEncoder{}
	triggerAt(cleared, 300, 300)
	clearedSecond := triggerAt(cleared, 301, 0)
	if !bytes.Equal(duplicatedSecond, clearedSecond) {
		t.Fatalf("重复命中改变了相位，想要同确认不二次触发")
	}
	// 新确认到达：窗口重启，相位与旧窗口可辨。
	restarted := &ViewmodelEncoder{}
	triggerAt(restarted, 300, 300)
	restartedSecond := triggerAt(restarted, 301, 301)
	if bytes.Equal(restartedSecond, duplicatedSecond) {
		t.Fatalf("新确认未重启窗口，想要触发沿推进相位")
	}
	// 陈旧确认（小于已见触发沿）：窗口已收拢时不触发挥动。
	staleEncoder := &ViewmodelEncoder{}
	triggerAt(staleEncoder, 400, 400)
	for frame := range ViewmodelAttackFrames {
		triggerAt(staleEncoder, 401+uint64(frame), 400)
	}
	triggerAt(staleEncoder, 420, 399)
	afterStale := triggerAt(staleEncoder, 421, 0)
	neutral := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, viewmodelTestInput(player, stack, 421))
	if !bytes.Equal(afterStale, neutral) {
		t.Fatalf("陈旧命中触发挥动，想要直接忽略")
	}
}

func TestViewmodelReplayIdentical(t *testing.T) {
	player := core.PlayerID{13}
	stack := core.ItemStack{Item: core.ItemStone, Count: 1}
	run := func() []byte {
		encoder := &ViewmodelEncoder{}
		var out []byte
		for tick := uint64(500); tick < 520; tick++ {
			input := viewmodelTestInput(player, stack, tick)
			input.Mining = tick%2 == 0
			if tick == 505 {
				input.AttackTick = 505
			} else if tick > 505 {
				input.AttackTick = 505
			}
			out = append(out, encoder.EncodeViewmodelInstances(nil, input)...)
		}
		return out
	}
	if first, second := run(), run(); !bytes.Equal(first, second) {
		t.Fatalf("同 tick 序列两次编码不一致，想要逐字节相同")
	}
}

func TestViewmodelTickRollbackReanchors(t *testing.T) {
	player := core.PlayerID{15}
	stack := core.ItemStack{Item: core.ItemStonePickaxe, Count: 1}
	encoder := &ViewmodelEncoder{}
	before := viewmodelTestInput(player, stack, 900)
	before.Mining = true
	encoder.EncodeViewmodelInstances(nil, before)
	after := viewmodelTestInput(player, stack, 100)
	after.Mining = true
	after.AttackTick = 0
	rolled := encoder.EncodeViewmodelInstances(nil, after)
	fresh := &ViewmodelEncoder{}
	wantInput := viewmodelTestInput(player, stack, 100)
	wantInput.Mining = true
	want := fresh.EncodeViewmodelInstances(nil, wantInput)
	if !bytes.Equal(rolled, want) {
		t.Fatalf("tick 回退后未重新锚定，想要旧会话挥动不延续")
	}
}

func TestViewmodelSwordAndBlockAnglesDiffer(t *testing.T) {
	sword := ViewmodelMiningAngle(610, 600, ViewmodelTierSword)
	block := ViewmodelMiningAngle(610, 600, ViewmodelTierBlock)
	if sword == block {
		t.Fatalf("相同相位下剑与方块旋转角相等，想要剪影可辨")
	}
	swordAmp, _ := ViewmodelSwingParams(ViewmodelTierSword)
	if sword < -swordAmp || sword > swordAmp {
		t.Fatalf("剑旋转角 %v 超出参数表标定区间 [-%v,%v]", sword, swordAmp, swordAmp)
	}
	blockAmp, _ := ViewmodelSwingParams(ViewmodelTierBlock)
	if block < -blockAmp || block > blockAmp {
		t.Fatalf("方块旋转角 %v 超出参数表标定区间 [-%v,%v]", block, blockAmp, blockAmp)
	}
}
