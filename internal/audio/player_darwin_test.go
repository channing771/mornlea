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
	if allocations := testing.AllocsPerRun(1000, func() {
		player.Play(Cue(cueCount))
	}); allocations != 0 {
		t.Fatalf("非法 cue 路径分配 %v 次，want 0", allocations)
	}
}
