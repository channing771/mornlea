//go:build darwin

package render

import (
	"bytes"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestEncodeBreakBurstInstancesMatchesDropEncoding 钉住 burst 字节流与 avatar
// pass 的实例契约一致:新掉落物首帧恰 8 实例、每实例 96 字节;同输入逐帧字节一致。
func TestEncodeBreakBurstInstancesMatchesDropEncoding(t *testing.T) {
	encoder := &InstanceEncoder{}
	drops := []ItemDrop{breakTestDrop(3, core.BlockPos{X: 0, Y: 3, Z: 0}, core.ItemDirt)}
	first := encoder.EncodeBreakBurstInstances(nil, 10, drops)
	if len(first) != 8*avatarInstanceBytes {
		t.Fatalf("burst 流 = %d 字节，想要 %d", len(first), 8*avatarInstanceBytes)
	}
	repeat := encoder.EncodeBreakBurstInstances(nil, 10, drops)
	if !bytes.Equal(first, repeat) {
		t.Fatal("同一 tick 两次 burst 编码字节不一致")
	}
	if later := encoder.EncodeBreakBurstInstances(nil, 11, drops); bytes.Equal(first, later) {
		t.Fatal("tick 推进后 burst 编码字节未变化")
	}
}

// TestEncodeBreakBurstInstancesStaysWithinInstanceCap 钉住编码上限:17 个全活
// burst 只输出 64 实例,不做无界工作。
func TestEncodeBreakBurstInstancesStaysWithinInstanceCap(t *testing.T) {
	encoder := &InstanceEncoder{}
	var drops []ItemDrop
	for tick := uint64(0); tick < 17; tick++ {
		drops = append(drops, breakTestDrop(uint8(tick),
			core.BlockPos{X: int32(tick), Y: 3, Z: 0}, core.ItemDirt))
		encoder.EncodeBreakBurstInstances(nil, tick, drops)
	}
	stream := encoder.EncodeBreakBurstInstances(nil, 16, drops)
	if len(stream) != 64*avatarInstanceBytes {
		t.Fatalf("burst 流 = %d 字节，想要上限 %d", len(stream), 64*avatarInstanceBytes)
	}
}
