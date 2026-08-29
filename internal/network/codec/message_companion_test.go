package codec

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"github.com/channing771/mornlea/internal/network/protocol"
	"math"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
)

func TestChatEventTaskLifecycleCombinationsValidateAndRoundTrip(t *testing.T) {
	// 任务事件必须携带完整伙伴身份与原始指令；TaskFailed 额外要求固定失败原因枚举，
	// QueueFull 与 NotFollowing 拒绝事件携带与 Accepted 相同的身份与指令。
	valid := []protocol.ChatEvent{
		taskChatEvent(2, protocol.ChatEventTaskStarted, protocol.ChatRejectNone),
		taskChatEvent(3, protocol.ChatEventTaskProgress, protocol.ChatRejectNone),
		taskChatEvent(4, protocol.ChatEventTaskCompleted, protocol.ChatRejectNone),
		taskChatEvent(5, protocol.ChatEventTaskTimedOut, protocol.ChatRejectNone),
		taskChatEvent(6, protocol.ChatEventTaskFailed, protocol.ChatRejectReason(protocol.TaskFailPlannerUnavailable)),
		taskChatEvent(7, protocol.ChatEventTaskFailed, protocol.ChatRejectReason(protocol.TaskFailInvalidPlan)),
		taskChatEvent(8, protocol.ChatEventTaskFailed, protocol.ChatRejectReason(protocol.TaskFailPathUnreachable)),
		taskChatEvent(9, protocol.ChatEventTaskFailed, protocol.ChatRejectReason(protocol.TaskFailWorldChanged)),
		taskChatEvent(10, protocol.ChatEventTaskFailed, protocol.ChatRejectReason(protocol.TaskFailInventoryFull)),
		taskChatEvent(11, protocol.ChatEventRejected, protocol.ChatRejectQueueFull),
		taskChatEvent(12, protocol.ChatEventRejected, protocol.ChatRejectNotFollowing),
		taskChatEvent(13, protocol.ChatEventTaskStopped, protocol.ChatRejectNone),
	}
	for _, event := range valid {
		if err := event.Validate(); err != nil {
			t.Fatalf("合法任务事件 %d/%d 被拒绝: %v", event.Kind, event.RejectReason, err)
		}
		packetID, payload, err := encodeServerControlPayload(protocol.StatePlay, event)
		if err != nil || packetID != 16 {
			t.Fatalf("任务事件编码 = (%d,%d,%v)", packetID, len(payload), err)
		}
		decoded, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload)
		if err != nil || !reflect.DeepEqual(decoded, event) {
			t.Fatalf("任务事件往返 = %#v, %v，想要 %#v", decoded, err, event)
		}
	}
}

func TestChatEventTaskLifecycleCombinationsAreRejectedAtomically(t *testing.T) {
	// 任一字段不合要求即整体拒绝：不存在"部分应用"的组合。
	invalid := []interface{ Validate() error }{
		// 非 TaskFailed 的任务 kind 携带任何 reason 都非法。
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStarted, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectInvalidFormat }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskProgress, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectQueueFull }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskCompleted, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectReason(protocol.TaskFailInvalidPlan) }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskTimedOut, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectReason(protocol.TaskFailWorldChanged) }),
		// 任务事件缺少伙伴身份。
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStarted, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.CompanionID = companion.ID{} }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskFailed, protocol.ChatRejectReason(protocol.TaskFailPlannerUnavailable)), func(e *protocol.ChatEvent) { e.CompanionID = companion.ID{} }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskProgress, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.CompanionName = "" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskCompleted, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.CompanionName = " A" }),
		// 任务事件缺少合法原始指令。
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStarted, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.Command = "" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskTimedOut, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.Command = " x" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskFailed, protocol.ChatRejectReason(protocol.TaskFailPathUnreachable)), func(e *protocol.ChatEvent) { e.Command = "x " }),
		// TaskFailed 原因必须落在 16..20 固定枚举内。
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskFailed, protocol.ChatRejectNone), func(e *protocol.ChatEvent) {}),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskFailed, protocol.ChatRejectInvalidFormat), func(e *protocol.ChatEvent) {}),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskFailed, protocol.ChatRejectUnknownCompanion), func(e *protocol.ChatEvent) {}),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskFailed, protocol.ChatRejectQueueFull), func(e *protocol.ChatEvent) {}),
		protocol.ChatEvent{EventID: 1, PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: "Chen",
			CompanionID: testCompanionID(1), CompanionName: "A", Kind: protocol.ChatEventTaskFailed,
			RejectReason: protocol.ChatRejectReason(15), Command: "x"},
		protocol.ChatEvent{EventID: 1, PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: "Chen",
			CompanionID: testCompanionID(1), CompanionName: "A", Kind: protocol.ChatEventTaskFailed,
			RejectReason: protocol.ChatRejectReason(21), Command: "x"},
		// TaskStopped 是任务事件：reason 必须为 None，且携带完整伙伴身份与原始指令。
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStopped, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectInvalidFormat }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStopped, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectQueueFull }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStopped, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectNotFollowing }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStopped, protocol.ChatRejectNone), func(e *protocol.ChatEvent) {
			e.RejectReason = protocol.ChatRejectReason(protocol.TaskFailInventoryFull)
		}),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStopped, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.CompanionID = companion.ID{} }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStopped, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.CompanionName = "" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStopped, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.Command = "" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStopped, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.Command = " x" }),
		// QueueFull 与 NotFollowing 拒绝都必须携带完整伙伴身份与合法指令。
		protocol.ChatEvent{EventID: 1, PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: "Chen",
			Kind: protocol.ChatEventRejected, RejectReason: protocol.ChatRejectQueueFull},
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventRejected, protocol.ChatRejectQueueFull), func(e *protocol.ChatEvent) { e.CompanionID = companion.ID{} }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventRejected, protocol.ChatRejectQueueFull), func(e *protocol.ChatEvent) { e.CompanionName = "" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventRejected, protocol.ChatRejectQueueFull), func(e *protocol.ChatEvent) { e.Command = "" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventRejected, protocol.ChatRejectQueueFull), func(e *protocol.ChatEvent) { e.Command = " x" }),
		protocol.ChatEvent{EventID: 1, PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: "Chen",
			Kind: protocol.ChatEventRejected, RejectReason: protocol.ChatRejectNotFollowing},
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventRejected, protocol.ChatRejectNotFollowing), func(e *protocol.ChatEvent) { e.CompanionID = companion.ID{} }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventRejected, protocol.ChatRejectNotFollowing), func(e *protocol.ChatEvent) { e.CompanionName = "" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventRejected, protocol.ChatRejectNotFollowing), func(e *protocol.ChatEvent) { e.Command = "" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventRejected, protocol.ChatRejectNotFollowing), func(e *protocol.ChatEvent) { e.Command = " x" }),
		// 其余 protocol.RejectReason 值（含预留 3）在 Rejected 上仍非法。
		protocol.ChatEvent{EventID: 1, PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: "Chen",
			Kind: protocol.ChatEventRejected, RejectReason: protocol.ChatRejectReason(3)},
		protocol.ChatEvent{EventID: 1, PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: "Chen",
			Kind: protocol.ChatEventRejected, RejectReason: protocol.ChatRejectReason(protocol.TaskFailInvalidPlan)},
		// 任务失败原因不得出现在非 TaskFailed kind 上（含 v18 新增的 TaskStopped）。
		mutateTaskEvent(validAcceptedChatEvent(), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectReason(protocol.TaskFailInvalidPlan) }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStopped, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectReason(protocol.TaskFailWorldChanged) }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventRejected, protocol.ChatRejectInvalidFormat), func(e *protocol.ChatEvent) {
			e.RejectReason = protocol.ChatRejectReason(protocol.TaskFailWorldChanged)
			e.CompanionID = testCompanionID(1)
			e.Command = "x"
		}),
		// 未知 kind 保持非法。
		protocol.ChatEvent{EventID: 1, PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: "Chen",
			CompanionID: testCompanionID(1), CompanionName: "A", Kind: protocol.ChatEventKind(10),
			RejectReason: protocol.ChatRejectNone, Command: "x"},
	}
	for _, message := range invalid {
		if err := message.Validate(); err == nil {
			t.Fatalf("非法任务组合被接受: %+v", message)
		}
		// 编码同样拒绝：非法组合不得进入 wire。
		if event, ok := message.(protocol.ChatEvent); ok {
			if _, _, err := encodeServerControlPayload(protocol.StatePlay, event); err == nil {
				t.Fatalf("非法任务组合被编码: %+v", event)
			}
		}
	}
}

func TestChatEventTaskDecoderRejectsInvalidKindReasonCombinations(t *testing.T) {
	// 从合法 TaskStarted wire 出发做单字节突变，解码必须整体拒绝。
	started := taskChatEvent(1, protocol.ChatEventTaskStarted, protocol.ChatRejectNone)
	_, eventWire, err := encodeServerControlPayload(protocol.StatePlay, started)
	if err != nil {
		t.Fatal(err)
	}
	kindOffset := chatEventKindOffset(started)
	for _, mutation := range []struct {
		name   string
		offset int
		value  byte
	}{
		{"unknown kind", kindOffset, 10},
		{"task kind with rejection reason", kindOffset + 1, byte(protocol.ChatRejectInvalidFormat)},
		{"task kind with queue full reason", kindOffset + 1, byte(protocol.ChatRejectQueueFull)},
		{"task kind with task fail reason", kindOffset + 1, byte(protocol.TaskFailPlannerUnavailable)},
	} {
		payload := append([]byte(nil), eventWire...)
		payload[mutation.offset] = mutation.value
		if packet, err := decodeServerControlPayload(protocol.StatePlay, 16, payload); err == nil || packet != nil {
			t.Fatalf("%s wire 解码为 %#v, %v", mutation.name, packet, err)
		}
	}

	// TaskStopped wire 的 reason 槽位只接受 None：携带拒绝原因或失败原因都必须整体拒绝，
	// 未知 kind 10 也保持非法。
	stopped := taskChatEvent(1, protocol.ChatEventTaskStopped, protocol.ChatRejectNone)
	_, stoppedWire, err := encodeServerControlPayload(protocol.StatePlay, stopped)
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []byte{byte(protocol.ChatRejectNotFollowing), byte(protocol.ChatRejectQueueFull), byte(protocol.TaskFailInventoryFull), byte(21)} {
		payload := append([]byte(nil), stoppedWire...)
		payload[kindOffset+1] = reason
		if packet, err := decodeServerControlPayload(protocol.StatePlay, 16, payload); err == nil || packet != nil {
			t.Fatalf("TaskStopped reason %d wire 解码为 %#v, %v", reason, packet, err)
		}
	}
	unknownKind := append([]byte(nil), stoppedWire...)
	unknownKind[kindOffset] = 10
	if packet, err := decodeServerControlPayload(protocol.StatePlay, 16, unknownKind); err == nil || packet != nil {
		t.Fatalf("未知 kind 10 wire 解码为 %#v, %v", packet, err)
	}

	// TaskFailed 的 reason 槽位只接受 16..20；15/21 与拒绝原因 4 都必须被拒。
	failed := taskChatEvent(2, protocol.ChatEventTaskFailed, protocol.ChatRejectReason(protocol.TaskFailPlannerUnavailable))
	_, failedWire, err := encodeServerControlPayload(protocol.StatePlay, failed)
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []byte{0, 4, 15, 21} {
		payload := append([]byte(nil), failedWire...)
		payload[kindOffset+1] = reason
		if packet, err := decodeServerControlPayload(protocol.StatePlay, 16, payload); err == nil || packet != nil {
			t.Fatalf("TaskFailed reason %d wire 解码为 %#v, %v", reason, packet, err)
		}
	}
	for _, reason := range []byte{16, 17, 18, 19, 20} {
		payload := append([]byte(nil), failedWire...)
		payload[kindOffset+1] = reason
		packet, err := decodeServerControlPayload(protocol.StatePlay, 16, payload)
		if err != nil {
			t.Fatalf("TaskFailed reason %d 合法 wire 被拒: %v", reason, err)
		}
		event, ok := packet.(protocol.ChatEvent)
		if !ok || event.RejectReason != protocol.ChatRejectReason(reason) {
			t.Fatalf("TaskFailed reason %d 往返 = %#v", reason, packet)
		}
	}

	// QueueFull 与 NotFollowing 拒绝缺少伙伴身份的 wire 必须被拒。
	queueFull := taskChatEvent(3, protocol.ChatEventRejected, protocol.ChatRejectQueueFull)
	_, queueFullWire, err := encodeServerControlPayload(protocol.StatePlay, queueFull)
	if err != nil {
		t.Fatal(err)
	}
	queueFullWire = append([]byte(nil), queueFullWire...)
	clear(queueFullWire[8+16+1+len(queueFull.PlayerName) : 8+16+1+len(queueFull.PlayerName)+16])
	if packet, err := decodeServerControlPayload(protocol.StatePlay, 16, queueFullWire); err == nil || packet != nil {
		t.Fatalf("缺少伙伴身份的 QueueFull wire 解码为 %#v, %v", packet, err)
	}
	notFollowing := taskChatEvent(4, protocol.ChatEventRejected, protocol.ChatRejectNotFollowing)
	_, notFollowingWire, err := encodeServerControlPayload(protocol.StatePlay, notFollowing)
	if err != nil {
		t.Fatal(err)
	}
	notFollowingWire = append([]byte(nil), notFollowingWire...)
	clear(notFollowingWire[8+16+1+len(notFollowing.PlayerName) : 8+16+1+len(notFollowing.PlayerName)+16])
	if packet, err := decodeServerControlPayload(protocol.StatePlay, 16, notFollowingWire); err == nil || packet != nil {
		t.Fatalf("缺少伙伴身份的 NotFollowing wire 解码为 %#v, %v", packet, err)
	}
}

func TestChatEventCompanionSpeechCombinationsValidateAndRoundTrip(t *testing.T) {
	// v19 追加的 CompanionSpeech 是 protocol.ChatEvent 中唯一允许携带模型生成文本的 kind：
	// 必须携带合法 event ID、完整玩家与伙伴身份、1..256 bytes 合法台词与 None reason，
	// 且不得复述玩家指令。256-byte 单字节与多字节混合台词都是合法上界。
	multibyteMax := strings.Repeat("台", 85) + "a"
	if len(multibyteMax) != 256 {
		t.Fatalf("多字节台词夹具 = %d bytes，想要 256", len(multibyteMax))
	}
	valid := []protocol.ChatEvent{
		companionSpeechEvent(1, "开始干活。"),
		companionSpeechEvent(2, strings.Repeat("x", 256)),
		companionSpeechEvent(3, multibyteMax),
	}
	for _, event := range valid {
		if err := event.Validate(); err != nil {
			t.Fatalf("合法台词事件 %d 被拒绝: %v", event.EventID, err)
		}
		packetID, payload, err := encodeServerControlPayload(protocol.StatePlay, event)
		if err != nil || packetID != 16 {
			t.Fatalf("台词事件编码 = (%d,%d,%v)", packetID, len(payload), err)
		}
		decoded, err := decodeServerControlPayload(protocol.StatePlay, packetID, payload)
		if err != nil || !reflect.DeepEqual(decoded, event) {
			t.Fatalf("台词事件往返 = %#v, %v，想要 %#v", decoded, err, event)
		}
	}
}

func TestChatEventCompanionSpeechCombinationsAreRejectedAtomically(t *testing.T) {
	// 任一字段不满足 Speech 的组合要求即整体拒绝，且不得进入 wire。
	invalid := []interface{ Validate() error }{
		// 台词长度与文本纪律：空台词、257-byte、首尾空白（含全角空格与不换行空格）。
		companionSpeechEvent(1, ""),
		companionSpeechEvent(1, strings.Repeat("x", 257)),
		companionSpeechEvent(1, " 台词"),
		companionSpeechEvent(1, "台词 "),
		companionSpeechEvent(1, "\u3000台词"),
		companionSpeechEvent(1, "台词\u00a0"),
		// 台词不得包含 NUL 或任何 Unicode control。
		companionSpeechEvent(1, "台\x00词"),
		companionSpeechEvent(1, "台\n词"),
		// 台词必须是有效 UTF-8。
		companionSpeechEvent(1, string([]byte{0xff})),
		// reason 必须为 None：拒绝原因与任务失败原因都不允许出现在台词事件上。
		mutateTaskEvent(companionSpeechEvent(1, "台词"), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectInvalidFormat }),
		mutateTaskEvent(companionSpeechEvent(1, "台词"), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectQueueFull }),
		mutateTaskEvent(companionSpeechEvent(1, "台词"), func(e *protocol.ChatEvent) { e.RejectReason = protocol.ChatRejectNotFollowing }),
		mutateTaskEvent(companionSpeechEvent(1, "台词"), func(e *protocol.ChatEvent) {
			e.RejectReason = protocol.ChatRejectReason(protocol.TaskFailInventoryFull)
		}),
		// 台词事件不得携带玩家指令字段：它只表达台词，不复述触发指令。
		mutateTaskEvent(companionSpeechEvent(1, "台词"), func(e *protocol.ChatEvent) { e.Command = "x" }),
		// 伙伴身份必须完整。
		mutateTaskEvent(companionSpeechEvent(1, "台词"), func(e *protocol.ChatEvent) { e.CompanionID = companion.ID{} }),
		mutateTaskEvent(companionSpeechEvent(1, "台词"), func(e *protocol.ChatEvent) { e.CompanionName = "" }),
		mutateTaskEvent(companionSpeechEvent(1, "台词"), func(e *protocol.ChatEvent) { e.CompanionName = " A" }),
		// 台词字段只属于 Speech kind：事实与拒绝 kind 携带台词都必须整条拒绝。
		mutateTaskEvent(validAcceptedChatEvent(), func(e *protocol.ChatEvent) { e.Speech = "台词" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStarted, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.Speech = "台词" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskFailed, protocol.ChatRejectReason(protocol.TaskFailPlannerUnavailable)), func(e *protocol.ChatEvent) { e.Speech = "台词" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventTaskStopped, protocol.ChatRejectNone), func(e *protocol.ChatEvent) { e.Speech = "台词" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventRejected, protocol.ChatRejectInvalidFormat), func(e *protocol.ChatEvent) { e.Speech = "台词" }),
		mutateTaskEvent(taskChatEvent(1, protocol.ChatEventRejected, protocol.ChatRejectQueueFull), func(e *protocol.ChatEvent) { e.Speech = "台词" }),
	}
	for _, message := range invalid {
		if err := message.Validate(); err == nil {
			t.Fatalf("非法台词组合被接受: %+v", message)
		}
		if event, ok := message.(protocol.ChatEvent); ok {
			if _, _, err := encodeServerControlPayload(protocol.StatePlay, event); err == nil {
				t.Fatalf("非法台词组合被编码: %+v", event)
			}
		}
	}
}

func TestChatEventCompanionSpeechDecoderRejectsInvalidWire(t *testing.T) {
	// 从合法台词 wire 出发做定向突变：未知 kind、非 None reason 与台词文本纪律
	// 违例都必须在解码层整体拒绝，不得产生部分应用的事件。
	event := companionSpeechEvent(1, "台词")
	_, wire, err := encodeServerControlPayload(protocol.StatePlay, event)
	if err != nil {
		t.Fatal(err)
	}
	kindOffset := chatEventKindOffset(event)
	for _, mutation := range []struct {
		name   string
		mutate func([]byte)
	}{
		{"unknown kind", func(p []byte) { p[kindOffset] = 10 }},
		{"reject reason", func(p []byte) { p[kindOffset+1] = byte(protocol.ChatRejectQueueFull) }},
		{"fail reason", func(p []byte) { p[kindOffset+1] = byte(protocol.TaskFailInventoryFull) }},
		{"speech NUL", func(p []byte) { p[kindOffset+3] = 0 }},
		{"speech control", func(p []byte) { p[kindOffset+3] = '\n' }},
		{"leading space", func(p []byte) { p[kindOffset+3] = ' ' }},
		{"trailing space", func(p []byte) { p[kindOffset+8] = ' ' }},
	} {
		payload := append([]byte(nil), wire...)
		mutation.mutate(payload)
		if packet, err := decodeServerControlPayload(protocol.StatePlay, 16, payload); err == nil || packet != nil {
			t.Fatalf("%s wire 解码为 %#v, %v", mutation.name, packet, err)
		}
	}

	// 编码器无法产出非法台词，wire 层的长度边界需要手工构造：
	// 空台词与 257/300-byte 台词槽位都必须被解码器拒绝。
	for _, speech := range []string{"", strings.Repeat("x", 257), strings.Repeat("x", 300)} {
		var encoder byteEncoder
		encoder.u64(1)
		encoder.data = append(encoder.data, event.PlayerID[:]...)
		encoder.string(event.PlayerName, 128)
		encoder.data = append(encoder.data, event.CompanionID[:]...)
		encoder.string(event.CompanionName, 128)
		encoder.u8(uint8(protocol.ChatEventCompanionSpeech))
		encoder.u8(uint8(protocol.ChatRejectNone))
		encoder.string(speech, 1024)
		if packet, err := decodeServerControlPayload(protocol.StatePlay, 16, encoder.data); err == nil || packet != nil {
			t.Fatalf("%d-byte 台词 wire 解码为 %#v, %v", len(speech), packet, err)
		}
	}
}

// TestChatEventDecoderRejectsInvalidUTF8TextSlot 显式钉住共享文本槽位的无效
// UTF-8 wire 突变（F-4）：Speech 与 Command 共用 wire 上唯一的文本槽位，把
// 合法事件的槽位字节替换为无效 UTF-8 序列（裸 0xFF 0xFE 前导、孤立 0xFF、
// 截断的多字节序列）必须在解码层整体拒绝，错误类别与既有非法文本一致——
// 都来自 string 原语的 errInvalidString（codec_primitives 的 utf8.Valid
// 检查），而不是穿过解码后才被 Validate 拒绝。fuzz 已探索过此类输入，本
// 矩阵把定界用例钉进显式测试并在 FuzzCompanionMessageCodec 补对应 seed。
func TestChatEventDecoderRejectsInvalidUTF8TextSlot(t *testing.T) {
	// 台词槽位（kind=CompanionSpeech）：从合法 wire 出发做定向字节突变。
	speech := companionSpeechEvent(1, "台词")
	_, speechWire, err := encodeServerControlPayload(protocol.StatePlay, speech)
	if err != nil {
		t.Fatal(err)
	}
	kindOffset := chatEventKindOffset(speech)
	// 槽位布局：kind+1 是 reason、kind+2 是 uvarint 长度前缀（6 < 128 占
	// 1 字节）、kind+3 起是 6 字节台词正文（"台词" = E5 8F B0 E8 AF 8D）。
	speechMutations := []struct {
		name   string
		mutate func([]byte)
	}{
		{"台词前两字节换裸 0xFF 0xFE", func(p []byte) { p[kindOffset+3], p[kindOffset+4] = 0xFF, 0xFE }},
		{"台词首字节换孤立 0xFF", func(p []byte) { p[kindOffset+3] = 0xFF }},
		{"台词尾字节截断多字节序列", func(p []byte) {
			// 把 "词"（E8 AF 8D）的最后一个续字节换成 ASCII：lead byte 仍
			// 期待两个续字节，构成截断的多字节序列。
			p[kindOffset+3+5] = 'x'
		}},
	}
	for _, mutation := range speechMutations {
		payload := append([]byte(nil), speechWire...)
		mutation.mutate(payload)
		packet, err := decodeServerControlPayload(protocol.StatePlay, 16, payload)
		if err == nil || packet != nil {
			t.Fatalf("%s wire 解码为 %#v, %v", mutation.name, packet, err)
		}
		if !errors.Is(err, errInvalidString) {
			t.Fatalf("%s 错误类别 = %v，want errInvalidString（与既有非法文本一致）", mutation.name, err)
		}
	}

	// 指令槽位（kind=TaskStarted）：同一文本槽位在非 Speech kind 上承载
	// 玩家指令，无效 UTF-8 同样必须在解码层拒绝。
	started := taskChatEvent(1, protocol.ChatEventTaskStarted, protocol.ChatRejectNone)
	_, startedWire, err := encodeServerControlPayload(protocol.StatePlay, started)
	if err != nil {
		t.Fatal(err)
	}
	startedPayload := append([]byte(nil), startedWire...)
	startedPayload[chatEventKindOffset(started)+3] = 0xFF
	if packet, err := decodeServerControlPayload(protocol.StatePlay, 16, startedPayload); err == nil || packet != nil {
		t.Fatalf("指令字节换 0xFF wire 解码为 %#v, %v", packet, err)
	} else if !errors.Is(err, errInvalidString) {
		t.Fatalf("指令字节换 0xFF 错误类别 = %v，want errInvalidString", err)
	}

	// 编码器无法产出无效 UTF-8 文本（e.string 自身校验并失败），只保留
	// lead 字节的截断序列必须绕过编码器手工拼 wire：台词侧长度前缀如实
	// 写 2、正文只给 "台"（E5 8F B0）的前两个字节。
	for _, raw := range []struct {
		name string
		kind protocol.ChatEventKind
		text []byte
	}{
		{"台词槽位截断 lead 序列", protocol.ChatEventCompanionSpeech, []byte{0xE5, 0x8F}},
		{"台词槽位裸 0xFF 0xFE", protocol.ChatEventCompanionSpeech, []byte{0xFF, 0xFE}},
		{"指令槽位裸 0xFF 0xFE", protocol.ChatEventAccepted, []byte{0xFF, 0xFE}},
	} {
		payload := chatEventWireWithRawTextSlot(raw.kind, raw.text)
		packet, err := decodeServerControlPayload(protocol.StatePlay, 16, payload)
		if err == nil || packet != nil {
			t.Fatalf("%s wire 解码为 %#v, %v", raw.name, packet, err)
		}
		if !errors.Is(err, errInvalidString) {
			t.Fatalf("%s 错误类别 = %v，want errInvalidString", raw.name, err)
		}
	}
}

func TestCompanionMessageGolden(t *testing.T) {
	tests := []struct {
		name    string
		client  protocol.ClientPacket
		server  protocol.ServerPacket
		wantID  uint32
		wantHex string
	}{
		{"ChatCommand", protocol.ChatCommand{Text: "@A x"}, nil, 12, "0440412078"},
		{"ChatEvent", nil, validAcceptedChatEvent(), 16,
			"0100000000000000" + "10000000000040008000000000000009" + "044368656e" +
				"10000000000040008000000000000001" + "0141" + "0100" + "0178"},
		{"ChatEventTaskStarted", nil, taskChatEvent(2, protocol.ChatEventTaskStarted, protocol.ChatRejectNone), 16,
			"0200000000000000" + "10000000000040008000000000000009" + "044368656e" +
				"10000000000040008000000000000001" + "0141" + "0300" + "0178"},
		{"ChatEventTaskProgress", nil, taskChatEvent(3, protocol.ChatEventTaskProgress, protocol.ChatRejectNone), 16,
			"0300000000000000" + "10000000000040008000000000000009" + "044368656e" +
				"10000000000040008000000000000001" + "0141" + "0400" + "0178"},
		{"ChatEventTaskCompleted", nil, taskChatEvent(4, protocol.ChatEventTaskCompleted, protocol.ChatRejectNone), 16,
			"0400000000000000" + "10000000000040008000000000000009" + "044368656e" +
				"10000000000040008000000000000001" + "0141" + "0500" + "0178"},
		{"ChatEventTaskFailed", nil, taskChatEvent(5, protocol.ChatEventTaskFailed, protocol.ChatRejectReason(protocol.TaskFailPlannerUnavailable)), 16,
			"0500000000000000" + "10000000000040008000000000000009" + "044368656e" +
				"10000000000040008000000000000001" + "0141" + "0610" + "0178"},
		{"ChatEventTaskTimedOut", nil, taskChatEvent(6, protocol.ChatEventTaskTimedOut, protocol.ChatRejectNone), 16,
			"0600000000000000" + "10000000000040008000000000000009" + "044368656e" +
				"10000000000040008000000000000001" + "0141" + "0700" + "0178"},
		{"ChatEventQueueFull", nil, taskChatEvent(7, protocol.ChatEventRejected, protocol.ChatRejectQueueFull), 16,
			"0700000000000000" + "10000000000040008000000000000009" + "044368656e" +
				"10000000000040008000000000000001" + "0141" + "0204" + "0178"},
		{"ChatEventNotFollowing", nil, taskChatEvent(14, protocol.ChatEventRejected, protocol.ChatRejectNotFollowing), 16,
			"0e00000000000000" + "10000000000040008000000000000009" + "044368656e" +
				"10000000000040008000000000000001" + "0141" + "0205" + "0178"},
		{"ChatEventTaskStopped", nil, taskChatEvent(13, protocol.ChatEventTaskStopped, protocol.ChatRejectNone), 16,
			"0d00000000000000" + "10000000000040008000000000000009" + "044368656e" +
				"10000000000040008000000000000001" + "0141" + "0800" + "0178"},
		{"ChatEventTaskFailInventoryFull", nil, taskChatEvent(15, protocol.ChatEventTaskFailed, protocol.ChatRejectReason(protocol.TaskFailInventoryFull)), 16,
			"0f00000000000000" + "10000000000040008000000000000009" + "044368656e" +
				"10000000000040008000000000000001" + "0141" + "0614" + "0178"},
		// v19 台词事件：kind 9、reason 0，台词复用既有文本槽位（此处为 6-byte "台词"）。
		{"ChatEventCompanionSpeech", nil, companionSpeechEvent(16, "台词"), 16,
			"1000000000000000" + "10000000000040008000000000000009" + "044368656e" +
				"10000000000040008000000000000001" + "0141" + "0900" + "06e58fb0e8af8d"},
		{"CompanionSpawn", nil, protocol.CompanionSpawn{ID: testCompanionID(1), Name: "A", Tick: 1}, 17,
			"10000000000040008000000000000001" + "0141" + "0100000000000000" +
				"00000000" + "000000000000000000000000" + "0000000000000000"},
		{"CompanionStates", nil, protocol.CompanionStates{Tick: 2, States: []protocol.CompanionState{{ID: testCompanionID(1)}}}, 18,
			"0200000000000000" + "01" + "10000000000040008000000000000001" +
				"00000000" + "000000000000000000000000" + "0000000000000000" + "00"},
		{"CompanionDespawn", nil, protocol.CompanionDespawn{ID: testCompanionID(1)}, 19,
			"10000000000040008000000000000001"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var packetID uint32
			var payload []byte
			var err error
			if test.client != nil {
				packetID, payload, err = encodeClientPacketPayload(protocol.StatePlay, test.client)
			} else {
				packetID, payload, err = encodeServerControlPayload(protocol.StatePlay, test.server)
			}
			if err != nil || packetID != test.wantID || hex.EncodeToString(payload) != test.wantHex {
				t.Fatalf("id=%d payload=%x error=%v，想要 id=%d payload=%s",
					packetID, payload, err, test.wantID, test.wantHex)
			}
		})
	}
}

func TestChatCommandAccepts1024BytesAndRejects1025(t *testing.T) {
	for _, text := range []string{"@A x", strings.Repeat("x", 1024)} {
		packet := protocol.ChatCommand{Text: text}
		if err := packet.Validate(); err != nil {
			t.Fatalf("%d-byte protocol.ChatCommand 被拒绝: %v", len(text), err)
		}
		packetID, payload, err := encodeClientPacketPayload(protocol.StatePlay, packet)
		if err != nil || packetID != 12 {
			t.Fatalf("protocol.ChatCommand 编码 = (%d,%d,%v)", packetID, len(payload), err)
		}
	}
	for _, text := range []string{
		"", strings.Repeat("x", 1025), " x", "x ", "\u3000x", "x\u00a0",
		"x\x00y", "x\ny", string([]byte{0xff}),
	} {
		packet := protocol.ChatCommand{Text: text}
		if err := packet.Validate(); err == nil {
			t.Fatalf("非法 protocol.ChatCommand %q 通过 Validate", text)
		}
		if _, _, err := encodeClientPacketPayload(protocol.StatePlay, packet); err == nil {
			t.Fatalf("非法 protocol.ChatCommand %q 被编码", text)
		}
	}
	for _, payload := range [][]byte{
		{0},
		append([]byte{0x81, 0x08}, make([]byte, 1025)...),
		{1, 0xff},
		{3, 'x', 0, 'y'},
		{3, 'x', '\n', 'y'},
		{2, ' ', 'x'},
		{2, 'x', ' '},
		{4, 0xe3, 0x80, 0x80, 'x'},
		{3, 'x', 0xc2, 0xa0},
	} {
		if packet, err := decodeClientPacketPayload(protocol.StatePlay, 12, payload); err == nil || packet != nil {
			t.Fatalf("非法 protocol.ChatCommand wire 解码为 %#v, %v", packet, err)
		}
	}
}

func TestCompanionSpawnAndChatEventStringBoundaries(t *testing.T) {
	id := testCompanionID(1)
	playerID := core.PlayerID(testCompanionID(2))
	maxName := strings.Repeat("𐐀", 32)
	maxCommand := strings.Repeat("x", 1024)
	valid := []interface{ Validate() error }{
		protocol.CompanionSpawn{ID: id, Name: maxName, Dimension: core.Overworld, Pitch: float32(math.Pi / 2)},
		protocol.ChatEvent{EventID: 1, PlayerID: playerID, PlayerName: maxName, CompanionID: id,
			CompanionName: maxName, Kind: protocol.ChatEventAccepted, RejectReason: protocol.ChatRejectNone, Command: maxCommand},
		protocol.ChatEvent{EventID: 2, PlayerID: playerID, PlayerName: "Chen", Kind: protocol.ChatEventRejected,
			RejectReason: protocol.ChatRejectInvalidFormat},
		protocol.ChatEvent{EventID: 3, PlayerID: playerID, PlayerName: "Chen", CompanionName: maxName,
			Kind: protocol.ChatEventRejected, RejectReason: protocol.ChatRejectUnknownCompanion},
		protocol.ChatEvent{EventID: 4, PlayerID: playerID, PlayerName: maxName, CompanionID: id,
			CompanionName: maxName, Kind: protocol.ChatEventTaskFailed,
			RejectReason: protocol.ChatRejectReason(protocol.TaskFailWorldChanged), Command: maxCommand},
		protocol.ChatEvent{EventID: 5, PlayerID: playerID, PlayerName: maxName, CompanionID: id,
			CompanionName: maxName, Kind: protocol.ChatEventRejected,
			RejectReason: protocol.ChatRejectQueueFull, Command: maxCommand},
	}
	for _, message := range valid {
		if err := message.Validate(); err != nil {
			t.Fatalf("合法 %T 被拒绝: %v", message, err)
		}
	}

	tooManyRunes := strings.Repeat("a", 33)
	// 单个合法 UTF-8 rune 最多四字节，因此超过 128 bytes 的合法名称必然也超过 32 rune。
	exact129Bytes := strings.Repeat("𐐀", 32) + "a"
	tooManyBytes := strings.Repeat("𐐀", 33)
	if len(exact129Bytes) != 129 || utf8.RuneCountInString(exact129Bytes) != 33 {
		t.Fatalf("精确边界夹具 = %d bytes/%d runes，想要 129/33",
			len(exact129Bytes), utf8.RuneCountInString(exact129Bytes))
	}
	invalid := []interface{ Validate() error }{
		protocol.CompanionSpawn{ID: id, Name: tooManyRunes, Dimension: core.Overworld},
		protocol.CompanionSpawn{ID: id, Name: exact129Bytes, Dimension: core.Overworld},
		protocol.CompanionSpawn{ID: id, Name: tooManyBytes, Dimension: core.Overworld},
		protocol.CompanionSpawn{ID: id, Name: " A", Dimension: core.Overworld},
		protocol.CompanionSpawn{ID: id, Name: "A\n", Dimension: core.Overworld},
		protocol.CompanionSpawn{ID: id, Name: "A", Dimension: 1},
		protocol.CompanionSpawn{ID: id, Name: "A", Dimension: core.Overworld, Pitch: float32(math.Pi/2) + 0.01},
		protocol.ChatEvent{EventID: 1, PlayerID: playerID, PlayerName: tooManyRunes, Kind: protocol.ChatEventRejected, RejectReason: protocol.ChatRejectInvalidFormat},
		protocol.ChatEvent{EventID: 1, PlayerID: playerID, PlayerName: exact129Bytes, Kind: protocol.ChatEventRejected, RejectReason: protocol.ChatRejectInvalidFormat},
		protocol.ChatEvent{EventID: 1, PlayerID: playerID, PlayerName: tooManyBytes, Kind: protocol.ChatEventRejected, RejectReason: protocol.ChatRejectInvalidFormat},
		protocol.ChatEvent{EventID: 1, PlayerID: playerID, PlayerName: "Chen", CompanionID: id,
			CompanionName: "A", Kind: protocol.ChatEventAccepted, RejectReason: protocol.ChatRejectNone},
		protocol.ChatEvent{EventID: 1, PlayerID: playerID, PlayerName: "Chen", CompanionID: id,
			CompanionName: "A", Kind: protocol.ChatEventRejected, RejectReason: protocol.ChatRejectInvalidFormat},
		protocol.ChatEvent{EventID: 1, PlayerID: playerID, PlayerName: "Chen", CompanionName: "A",
			Kind: protocol.ChatEventRejected, RejectReason: protocol.ChatRejectInvalidFormat},
		protocol.ChatEvent{EventID: 1, PlayerID: playerID, PlayerName: "Chen", CompanionName: "A", Command: "x",
			Kind: protocol.ChatEventRejected, RejectReason: protocol.ChatRejectUnknownCompanion},
		protocol.ChatEvent{EventID: 1, PlayerID: playerID, PlayerName: "Chen", CompanionName: "A",
			Kind: protocol.ChatEventAccepted, RejectReason: protocol.ChatRejectNone, Command: "x"},
		protocol.ChatEvent{EventID: 1, PlayerID: playerID, PlayerName: "Chen", CompanionID: id,
			CompanionName: "A", Kind: protocol.ChatEventAccepted, RejectReason: protocol.ChatRejectInvalidFormat, Command: "x"},
		protocol.ChatEvent{EventID: 1, PlayerID: playerID, PlayerName: "Chen", CompanionID: id,
			CompanionName: "A", Kind: protocol.ChatEventKind(10), RejectReason: protocol.ChatRejectNone, Command: "x"},
	}
	for _, message := range invalid {
		if err := message.Validate(); err == nil {
			t.Fatalf("非法 %T 被接受: %+v", message, message)
		}
	}

	var oversizedNameEvent byteEncoder
	oversizedNameEvent.u64(1)
	oversizedNameEvent.data = append(oversizedNameEvent.data, playerID[:]...)
	oversizedNameEvent.string(exact129Bytes, 129)
	oversizedNameEvent.data = append(oversizedNameEvent.data, make([]byte, len(id))...)
	oversizedNameEvent.string("", 128)
	oversizedNameEvent.u8(uint8(protocol.ChatEventRejected))
	oversizedNameEvent.u8(uint8(protocol.ChatRejectInvalidFormat))
	oversizedNameEvent.string("", 1024)
	if packet, err := decodeServerControlPayload(protocol.StatePlay, 16, oversizedNameEvent.data); err == nil || packet != nil {
		t.Fatalf("129-byte player name wire 解码为 %#v, %v", packet, err)
	}
}

func TestCompanionPitchUsesRadiansInValidateAndDecode(t *testing.T) {
	id := testCompanionID(1)
	halfPi := float32(math.Pi / 2)
	aboveHalfPi := math.Nextafter32(halfPi, float32(math.Inf(1)))
	belowNegativeHalfPi := math.Nextafter32(-halfPi, float32(math.Inf(-1)))

	for _, pitch := range []float32{-halfPi, halfPi} {
		if err := (protocol.CompanionSpawn{ID: id, Name: "A", Dimension: core.Overworld, Pitch: pitch}).Validate(); err != nil {
			t.Fatalf("合法 Spawn pitch %v 被拒绝: %v", pitch, err)
		}
		if err := (protocol.CompanionStates{States: []protocol.CompanionState{{
			ID: id, Dimension: core.Overworld, Pitch: pitch,
		}}}).Validate(); err != nil {
			t.Fatalf("合法 States pitch %v 被拒绝: %v", pitch, err)
		}
	}

	for _, message := range []interface{ Validate() error }{
		protocol.CompanionSpawn{ID: id, Name: "A", Dimension: core.Overworld, Pitch: aboveHalfPi},
		protocol.CompanionSpawn{ID: id, Name: "A", Dimension: core.Overworld, Pitch: belowNegativeHalfPi},
		protocol.CompanionStates{States: []protocol.CompanionState{{ID: id, Dimension: core.Overworld, Pitch: aboveHalfPi}}},
		protocol.CompanionStates{States: []protocol.CompanionState{{ID: id, Dimension: core.Overworld, Pitch: belowNegativeHalfPi}}},
	} {
		if err := message.Validate(); err == nil {
			t.Fatalf("非法 %T pitch 通过 Validate: %+v", message, message)
		}
	}

	_, spawnWire, err := encodeServerControlPayload(protocol.StatePlay, protocol.CompanionSpawn{
		ID: id, Name: "A", Dimension: core.Overworld, Pitch: halfPi,
	})
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(spawnWire[len(spawnWire)-4:], math.Float32bits(aboveHalfPi))
	if packet, err := decodeServerControlPayload(protocol.StatePlay, 17, spawnWire); err == nil || packet != nil {
		t.Fatalf("非法 Spawn pitch raw wire 解码为 %#v, %v", packet, err)
	}

	statesWire := companionStatesWireFixture(id)
	binary.LittleEndian.PutUint32(statesWire[len(statesWire)-5:len(statesWire)-1], math.Float32bits(belowNegativeHalfPi))
	if packet, err := decodeServerControlPayload(protocol.StatePlay, 18, statesWire); err == nil || packet != nil {
		t.Fatalf("非法 States pitch raw wire 解码为 %#v, %v", packet, err)
	}
}

func TestCompanionMessagesHaveFixedMaximumWireLengths(t *testing.T) {
	id := testCompanionID(1)
	maxName := strings.Repeat("𐐀", 32)
	maxCommand := strings.Repeat("x", 1024)
	states := make([]protocol.CompanionState, 4)
	for index := range states {
		states[index] = protocol.CompanionState{ID: testCompanionID(byte(index + 1)), Dimension: core.Overworld}
	}
	tests := []struct {
		name string
		want int
		call func() ([]byte, error)
	}{
		{"ChatCommand", 1026, func() ([]byte, error) {
			_, payload, err := encodeClientPacketPayload(protocol.StatePlay, protocol.ChatCommand{Text: maxCommand})
			return payload, err
		}},
		{"CompanionSpawn", 178, func() ([]byte, error) {
			_, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.CompanionSpawn{ID: id, Name: maxName, Dimension: core.Overworld})
			return payload, err
		}},
		{"CompanionStates", 173, func() ([]byte, error) {
			_, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.CompanionStates{States: states})
			return payload, err
		}},
		{"ChatEvent", 1328, func() ([]byte, error) {
			_, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.ChatEvent{EventID: 1,
				PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: maxName, CompanionID: id,
				CompanionName: maxName, Kind: protocol.ChatEventAccepted, RejectReason: protocol.ChatRejectNone, Command: maxCommand})
			return payload, err
		}},
		// 任务事件复用同一 wire 形状，固定上限仍是 1328 bytes。
		{"ChatEventTaskFailed", 1328, func() ([]byte, error) {
			_, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.ChatEvent{EventID: 1,
				PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: maxName, CompanionID: id,
				CompanionName: maxName, Kind: protocol.ChatEventTaskFailed,
				RejectReason: protocol.ChatRejectReason(protocol.TaskFailPlannerUnavailable), Command: maxCommand})
			return payload, err
		}},
		// v18 新增的 TaskStopped 与 NotFollowing 同样复用既有 wire 形状，不改变上限。
		{"ChatEventTaskStopped", 1328, func() ([]byte, error) {
			_, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.ChatEvent{EventID: 1,
				PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: maxName, CompanionID: id,
				CompanionName: maxName, Kind: protocol.ChatEventTaskStopped,
				RejectReason: protocol.ChatRejectNone, Command: maxCommand})
			return payload, err
		}},
		{"ChatEventNotFollowing", 1328, func() ([]byte, error) {
			_, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.ChatEvent{EventID: 1,
				PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: maxName, CompanionID: id,
				CompanionName: maxName, Kind: protocol.ChatEventRejected,
				RejectReason: protocol.ChatRejectNotFollowing, Command: maxCommand})
			return payload, err
		}},
		// v19 的 CompanionSpeech 复用既有文本槽位：256-byte 台词 + 最长身份的事件
		// 只有 560 bytes，固定上限 1328 bytes 不变。
		{"ChatEventCompanionSpeech", 560, func() ([]byte, error) {
			_, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.ChatEvent{EventID: 1,
				PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: maxName, CompanionID: id,
				CompanionName: maxName, Kind: protocol.ChatEventCompanionSpeech,
				RejectReason: protocol.ChatRejectNone, Speech: strings.Repeat("x", 256)})
			return payload, err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := test.call()
			if err != nil || len(payload) != test.want {
				t.Fatalf("wire length=%d error=%v, 想要 %d", len(payload), err, test.want)
			}
		})
	}
	// 既有指令文本边界不回归：1022/1023/1024-byte 指令对应 1326/1327/1328-byte wire，
	// 1328 仍是 protocol.ChatEvent 的固定上限。
	for _, boundary := range []struct {
		commandBytes int
		wantWire     int
	}{
		{1022, 1326},
		{1023, 1327},
		{1024, 1328},
	} {
		_, payload, err := encodeServerControlPayload(protocol.StatePlay, protocol.ChatEvent{EventID: 1,
			PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: maxName, CompanionID: id,
			CompanionName: maxName, Kind: protocol.ChatEventAccepted, RejectReason: protocol.ChatRejectNone,
			Command: strings.Repeat("x", boundary.commandBytes)})
		if err != nil || len(payload) != boundary.wantWire {
			t.Fatalf("%d-byte 指令事件 wire length=%d error=%v, 想要 %d",
				boundary.commandBytes, len(payload), err, boundary.wantWire)
		}
	}
	if packet, err := decodeClientPacketPayload(protocol.StatePlay, 12, make([]byte, 1027)); err == nil || packet != nil {
		t.Fatalf("超长 protocol.ChatCommand 解码为 %#v, %v", packet, err)
	}
	for id, length := range map[uint32]int{16: 1329, 17: 179, 18: 174, 19: 17} {
		if packet, err := decodeServerControlPayload(protocol.StatePlay, id, make([]byte, length)); err == nil || packet != nil {
			t.Fatalf("超长 server ID %d 解码为 %#v, %v", id, packet, err)
		}
	}
}

func TestCompanionStatesRejectsFiveDuplicateOrUnsortedAtomically(t *testing.T) {
	five := make([]protocol.CompanionState, 5)
	for index := range five {
		five[index] = protocol.CompanionState{ID: testCompanionID(byte(index + 1)), Dimension: core.Overworld}
	}
	if err := (protocol.CompanionStates{States: five}).Validate(); err == nil {
		t.Fatal("五项 states 通过 Validate")
	}
	if _, _, err := encodeServerControlPayload(protocol.StatePlay, protocol.CompanionStates{States: five}); err == nil {
		t.Fatal("五项 states 被编码")
	}
	for _, test := range []struct {
		name string
		ids  []companion.ID
	}{
		{"empty", nil},
		{"five", []companion.ID{testCompanionID(1), testCompanionID(2), testCompanionID(3), testCompanionID(4), testCompanionID(5)}},
		{"duplicate", []companion.ID{testCompanionID(1), testCompanionID(1)}},
		{"unsorted", []companion.ID{testCompanionID(2), testCompanionID(1)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet, err := decodeServerControlPayload(protocol.StatePlay, 18, companionStatesWireFixture(test.ids...))
			if err == nil || packet != nil {
				t.Fatalf("非法 states 解码为 %#v, %v", packet, err)
			}
		})
	}
	declaredHuge := append(make([]byte, 8), 0xff, 0xff, 0xff, 0xff, 0x0f)
	if packet, err := decodeServerControlPayload(protocol.StatePlay, 18, declaredHuge); err == nil || packet != nil {
		t.Fatalf("巨大 count 解码为 %#v, %v", packet, err)
	}
}

func TestCompanionDecoderRejectsInvalidIDsEnumsNumbersAndDimensions(t *testing.T) {
	spawn := protocol.CompanionSpawn{ID: testCompanionID(1), Name: "A", Tick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{1, 2, 3}, Yaw: 4, Pitch: 0.5}
	_, spawnWire, err := encodeServerControlPayload(protocol.StatePlay, spawn)
	if err != nil {
		t.Fatal(err)
	}
	mutations := [][]byte{
		append([]byte(nil), spawnWire...),
		append([]byte(nil), spawnWire...),
		append([]byte(nil), spawnWire...),
		append([]byte(nil), spawnWire...),
	}
	clear(mutations[0][:16])
	binary.LittleEndian.PutUint32(mutations[1][26:30], 1)
	binary.LittleEndian.PutUint32(mutations[2][len(spawnWire)-4:], math.Float32bits(91))
	binary.LittleEndian.PutUint32(mutations[3][len(spawnWire)-8:len(spawnWire)-4], 0x7fc00000)
	for index, payload := range mutations {
		if packet, err := decodeServerControlPayload(protocol.StatePlay, 17, payload); err == nil || packet != nil {
			t.Fatalf("spawn mutation %d 解码为 %#v, %v", index, packet, err)
		}
	}

	accepted := validAcceptedChatEvent()
	_, eventWire, err := encodeServerControlPayload(protocol.StatePlay, accepted)
	if err != nil {
		t.Fatal(err)
	}
	kindOffset := chatEventKindOffset(accepted)
	for _, mutation := range []func([]byte){
		func(payload []byte) { payload[kindOffset] = 10 },
		func(payload []byte) { payload[kindOffset+1] = byte(protocol.ChatRejectInvalidFormat) },
		func(payload []byte) { clear(payload[8 : 8+16]) },
	} {
		payload := append([]byte(nil), eventWire...)
		mutation(payload)
		if packet, err := decodeServerControlPayload(protocol.StatePlay, 16, payload); err == nil || packet != nil {
			t.Fatalf("protocol.ChatEvent mutation 解码为 %#v, %v", packet, err)
		}
	}

	stateWire := companionStatesWireFixture(testCompanionID(1))
	stateOffset := 9
	for _, mutation := range []func([]byte){
		func(payload []byte) { clear(payload[stateOffset : stateOffset+16]) },
		func(payload []byte) { binary.LittleEndian.PutUint32(payload[stateOffset+16:stateOffset+20], 1) },
		func(payload []byte) {
			binary.LittleEndian.PutUint32(payload[len(payload)-5:len(payload)-1], math.Float32bits(-91))
		},
		func(payload []byte) { payload[len(payload)-1] = 2 },
	} {
		payload := append([]byte(nil), stateWire...)
		mutation(payload)
		if packet, err := decodeServerControlPayload(protocol.StatePlay, 18, payload); err == nil || packet != nil {
			t.Fatalf("protocol.CompanionStates mutation 解码为 %#v, %v", packet, err)
		}
	}
	if packet, err := decodeServerControlPayload(protocol.StatePlay, 19, make([]byte, 16)); err == nil || packet != nil {
		t.Fatalf("零 protocol.CompanionDespawn ID 解码为 %#v, %v", packet, err)
	}
}

func validAcceptedChatEvent() protocol.ChatEvent {
	return protocol.ChatEvent{
		EventID: 1, PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: "Chen",
		CompanionID: testCompanionID(1), CompanionName: "A",
		Kind: protocol.ChatEventAccepted, RejectReason: protocol.ChatRejectNone, Command: "x",
	}
}

// chatEventKindOffset 返回 protocol.ChatEvent wire 载荷中 kind 字节的偏移。头部布局与
// wire 编码一致：8 bytes 前缀（event ID）+ 16 玩家 ID + 1 名称长度 + 玩家名 +
// 16 伙伴 ID + 1 名称长度 + 伙伴名，随后即是 1 byte kind（再后 1 byte 是
// reason 槽位）。wire 突变测试据此定位 kind 与 reason 字节。
func chatEventKindOffset(event protocol.ChatEvent) int {
	return 8 + 16 + 1 + len(event.PlayerName) + 16 + 1 + len(event.CompanionName)
}

// chatEventWireWithRawTextSlot 手工拼一份文本槽位为任意原始字节的
// protocol.ChatEvent wire：编码器只能写出合法 UTF-8 文本（e.string 自身校验），
// 无效 UTF-8 槽位的定界突变（裸 0xFF 0xFE、截断多字节序列）必须绕过
// 编码器直接落字节。身份头部与 taskCombinationSeed 同一夹具，长度前缀
// 如实写 len(text)。
func chatEventWireWithRawTextSlot(kind protocol.ChatEventKind, text []byte) []byte {
	playerID := core.PlayerID(testCompanionID(9))
	companionID := testCompanionID(1)
	var encoder byteEncoder
	encoder.u64(1)
	encoder.data = append(encoder.data, playerID[:]...)
	encoder.string("Chen", 128)
	encoder.data = append(encoder.data, companionID[:]...)
	encoder.string("A", 128)
	encoder.u8(uint8(kind))
	encoder.u8(uint8(protocol.ChatRejectNone))
	encoder.uvarint(uint32(len(text)))
	encoder.data = append(encoder.data, text...)
	return encoder.data
}

// taskChatEvent 构造一条携带完整伙伴身份与原始指令的任务生命周期事件；
// reason 槽位同时承载拒绝原因与 protocol.TaskFailReason。
func taskChatEvent(id uint64, kind protocol.ChatEventKind, reason protocol.ChatRejectReason) protocol.ChatEvent {
	return protocol.ChatEvent{
		EventID: id, PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: "Chen",
		CompanionID: testCompanionID(1), CompanionName: "A",
		Kind: kind, RejectReason: reason, Command: "x",
	}
}

// companionSpeechEvent 构造一条 v19 伙伴台词事件：携带完整玩家与伙伴身份、
// 合法台词与 None reason，且不复述玩家指令。
func companionSpeechEvent(id uint64, speech string) protocol.ChatEvent {
	return protocol.ChatEvent{
		EventID: id, PlayerID: core.PlayerID(testCompanionID(9)), PlayerName: "Chen",
		CompanionID: testCompanionID(1), CompanionName: "A",
		Kind: protocol.ChatEventCompanionSpeech, RejectReason: protocol.ChatRejectNone, Speech: speech,
	}
}

// mutateTaskEvent 返回应用单字段突变后的任务事件副本，用于原子拒绝用例。
func mutateTaskEvent(event protocol.ChatEvent, mutate func(*protocol.ChatEvent)) protocol.ChatEvent {
	mutate(&event)
	return event
}

func testCompanionID(last byte) companion.ID {
	return companion.ID{0: 0x10, 6: 0x40, 8: 0x80, 15: last}
}

func companionStatesWireFixture(ids ...companion.ID) []byte {
	var encoder byteEncoder
	encoder.u64(1)
	encoder.uvarint(uint32(len(ids)))
	for _, id := range ids {
		encoder.data = append(encoder.data, id[:]...)
		encoder.i32(int32(core.Overworld))
		for range 3 {
			encoder.f32(0)
		}
		encoder.f32(0)
		encoder.f32(0)
		encoder.bool(false)
	}
	return encoder.data
}
