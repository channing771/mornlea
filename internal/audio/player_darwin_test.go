//go:build darwin

package audio

import "testing"

// TestPlayerUnavailableState 证明没有系统队列的播放器保持无声，测试不触碰设备。
func TestPlayerUnavailableState(t *testing.T) {
	player := &Player{}
	if player.available() {
		t.Fatal("未初始化的 player 不应可播放")
	}
}

// TestPlayerInvalidCueIsNoOp 证明损坏或未来版本的 cue 不会进入播放路径。
func TestPlayerInvalidCueIsNoOp(t *testing.T) {
	player := &Player{}
	player.Play(Cue(cueCount))
	if player.available() {
		t.Fatal("非法 cue 不得改变无声 player 的状态")
	}
}

// TestPlayerStatePathWithoutDevice 在不创建 AudioQueue 的情况下覆盖与生产播放
// 相同的 PCM copy、八槽认领、BUSY、enqueue 失败复位及失败后静音状态。
func TestPlayerStatePathWithoutDevice(t *testing.T) {
	state := newPlayerStateTest()
	if state == nil {
		t.Fatal("创建无设备 player 状态失败")
	}
	t.Cleanup(state.close)
	pcm := synthesize(cueSpecs[CueEatingComplete])

	for slot := 0; slot < 8; slot++ {
		if status := state.play(pcm); status != audioPlayReady {
			t.Fatalf("第 %d 个 buffer status = %d, want READY", slot, status)
		}
		if got := state.lastSlot(); got != slot {
			t.Fatalf("第 %d 次播放选中 slot = %d, want %d", slot, got, slot)
		}
	}
	if status := state.play(pcm); status != audioPlayBusy {
		t.Fatalf("八槽占满后 status = %d, want BUSY", status)
	}
	for slot := 0; slot < 8; slot++ {
		state.finish(slot)
	}

	state.failNext()
	if status := state.play(pcm); status != audioPlayFailure {
		t.Fatalf("enqueue 失败 status = %d, want FAILURE", status)
	}
	if got := state.busyCount(); got != 0 {
		t.Fatalf("enqueue 失败后 busy buffer = %d, want 0", got)
	}
	if !state.failed() {
		t.Fatal("enqueue 失败后必须进入 failure 状态")
	}
	if status := state.play(pcm); status != audioPlayFailure {
		t.Fatalf("failure 状态 status = %d, want FAILURE", status)
	}
}

// TestPlayerStatePathIsAllocationFree 锁定实际 PCM 指针、C copy 和槽状态路径不向
// Go 堆分配；它刻意不初始化任何系统音频设备。
func TestPlayerStatePathIsAllocationFree(t *testing.T) {
	state := newPlayerStateTest()
	if state == nil {
		t.Fatal("创建无设备 player 状态失败")
	}
	t.Cleanup(state.close)
	pcm := synthesize(cueSpecs[CueUIClick])

	if allocations := testing.AllocsPerRun(1000, func() {
		if status := state.play(pcm); status != audioPlayReady {
			panic("无设备播放路径失败")
		}
		state.finish(state.lastSlot())
	}); allocations != 0 {
		t.Fatalf("实际 PCM/状态播放路径分配 %v 次，want 0", allocations)
	}
}
