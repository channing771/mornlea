package network

import (
	"encoding/hex"
	"errors"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

func TestCombatHitWireIsFixedTenBytes(t *testing.T) {
	hit := CombatHit{ServerTick: 0x0102030405060708, Damage: 6, TargetKind: core.CombatTargetHostile}
	packetID, payload, err := encodeServerControlPayload(StatePlay, hit)
	if err != nil || packetID != 25 || hex.EncodeToString(payload) != "08070605040302010602" {
		t.Fatalf("CombatHit id=%d payload=%x err=%v", packetID, payload, err)
	}
	decoded, err := decodeServerControlPayload(StatePlay, packetID, payload)
	if err != nil || !reflect.DeepEqual(decoded, hit) {
		t.Fatalf("CombatHit round=%#v err=%v", decoded, err)
	}
	if len(payload) != 10 {
		t.Fatalf("CombatHit payload len=%d want 10", len(payload))
	}
	for length := 0; length < len(payload); length++ {
		if _, err := decodeServerControlPayload(StatePlay, packetID, payload[:length]); err == nil {
			t.Fatalf("CombatHit 截断到 %d bytes 仍被接受", length)
		}
	}
	if _, err := decodeServerControlPayload(StatePlay, packetID, append(payload, 0)); err == nil {
		t.Fatal("CombatHit 带尾随字节仍被接受")
	}
	if _, _, err := encodeServerControlPayload(StateLogin, hit); err == nil {
		t.Fatal("CombatHit 在 Login state 仍可编码")
	}
	if _, err := decodeServerControlPayload(StateLogin, 25, payload); err == nil {
		t.Fatal("CombatHit 在 Login state 仍可解码")
	}
}

func TestCombatHitRegistryIsFrozen(t *testing.T) {
	packetID, ok := serverPacketID(StatePlay, CombatHit{})
	if !ok || packetID != 25 {
		t.Fatalf("CombatHit ID=(%d,%v)，想要 (25,true)", packetID, ok)
	}
	registered, ok := serverPacketForID(StatePlay, 25)
	if !ok {
		t.Fatal("Play S→C ID 25 未注册")
	}
	if _, ok := registered.(CombatHit); !ok {
		t.Fatalf("Play S→C ID 25=%T，想要 CombatHit", registered)
	}
	if _, ok := serverPacketForID(StatePlay, 26); ok {
		t.Fatal("Play S→C ID 26 必须保持未分配")
	}
	if _, err := decodeServerControlPayload(StatePlay, 26, nil); !errors.Is(err, errUnknownPacketID) {
		t.Fatalf("Play S→C ID 26 解码错误=%v，想要 %v", err, errUnknownPacketID)
	}
	if ProtocolVersion != 32 {
		t.Fatalf("协议版本 = %d，想要 32", ProtocolVersion)
	}
}

func TestCombatHitValidateRejectsInvalidValues(t *testing.T) {
	valid := CombatHit{ServerTick: 1, Damage: 6, TargetKind: core.CombatTargetHostile}
	if err := valid.Validate(); err != nil {
		t.Fatalf("合法 CombatHit 被拒绝: %v", err)
	}
	if err := ValidateServerPacket(StatePlay, valid); err != nil {
		t.Fatalf("合法 CombatHit ValidateServerPacket: %v", err)
	}
	invalid := []CombatHit{
		{ServerTick: 0, Damage: 6, TargetKind: core.CombatTargetHostile},
		{ServerTick: 1, Damage: 0, TargetKind: core.CombatTargetHostile},
		{ServerTick: 1, Damage: 21, TargetKind: core.CombatTargetHostile},
		{ServerTick: 1, Damage: 6, TargetKind: core.CombatTargetKind(0)},
		{ServerTick: 1, Damage: 6, TargetKind: core.CombatTargetKind(3)},
	}
	for _, hit := range invalid {
		if err := hit.Validate(); err == nil {
			t.Fatalf("非法 CombatHit %+v 通过 Validate", hit)
		}
		if err := ValidateServerPacket(StatePlay, hit); err == nil {
			t.Fatalf("非法 CombatHit %+v 通过 ValidateServerPacket", hit)
		}
		if _, _, err := encodeServerControlPayload(StatePlay, hit); err == nil {
			t.Fatalf("非法 CombatHit %+v 被编码", hit)
		}
	}
	// 合法边界值
	for _, hit := range []CombatHit{
		{ServerTick: 1, Damage: 1, TargetKind: core.CombatTargetPlayer},
		{ServerTick: 1, Damage: 20, TargetKind: core.CombatTargetHostile},
		{ServerTick: 1, Damage: 6, TargetKind: core.CombatTargetPlayer},
	} {
		if err := hit.Validate(); err != nil {
			t.Fatalf("边界合法 CombatHit %+v 被拒绝: %v", hit, err)
		}
		packetID, payload, err := encodeServerControlPayload(StatePlay, hit)
		if err != nil {
			t.Fatalf("边界合法 CombatHit %+v 编码失败: %v", hit, err)
		}
		decoded, err := decodeServerControlPayload(StatePlay, packetID, payload)
		if err != nil || !reflect.DeepEqual(decoded, hit) {
			t.Fatalf("边界合法 CombatHit %+v 往返=%#v err=%v", hit, decoded, err)
		}
	}
}

func TestCombatHitDecodeRejectsInvalidWire(t *testing.T) {
	valid := CombatHit{ServerTick: 0x0102030405060708, Damage: 6, TargetKind: core.CombatTargetHostile}
	_, payload, err := encodeServerControlPayload(StatePlay, valid)
	if err != nil {
		t.Fatal(err)
	}
	// 截断已在固定长度测试覆盖；这里覆盖值域突变和尾随。
	mutations := []struct {
		name   string
		mutate func([]byte)
	}{
		{"tick zero", func(p []byte) {
			for i := 0; i < 8; i++ {
				p[i] = 0
			}
		}},
		{"damage 0", func(p []byte) { p[8] = 0 }},
		{"damage 21", func(p []byte) { p[8] = 21 }},
		{"kind 0", func(p []byte) { p[9] = 0 }},
		{"kind 3", func(p []byte) { p[9] = 3 }},
	}
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			corrupted := append([]byte(nil), payload...)
			m.mutate(corrupted)
			if packet, err := decodeServerControlPayload(StatePlay, 25, corrupted); err == nil {
				t.Fatalf("%s wire 解码为 %#v，想要拒绝", m.name, packet)
			}
		})
	}
	// 任意尾随已测，这里再补不同长度尾随。
	for _, extra := range [][]byte{{0xFF}, {0x00, 0x01}, {0x01, 0x02, 0x03}} {
		payloadWithTail := append(append([]byte(nil), payload...), extra...)
		if _, err := decodeServerControlPayload(StatePlay, 25, payloadWithTail); err == nil {
			t.Fatalf("CombatHit 尾随 %x 仍被接受", extra)
		}
	}
	if _, err := decodeServerControlPayload(StatePlay, 25, nil); err == nil {
		t.Fatal("CombatHit 空 payload 被接受")
	}
}
