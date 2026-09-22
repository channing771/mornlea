//go:build darwin

package client

import "testing"

func TestGameActionBridgeStrictValidation(t *testing.T) {
	for _, event := range []string{
		`{"type":"game-action","token":1,"op":"slot","area":"inventory","index":35,"button":"left","shift":false}`,
		`{"type":"game-action","token":1,"op":"slot","area":"furnace","index":2,"button":"right","shift":true}`,
		`{"type":"game-action","token":1,"op":"close"}`,
		`{"type":"game-action","token":1,"op":"hotbar","index":8}`,
		`{"type":"game-action","token":1,"op":"drop","area":"chest","index":26}`,
		`{"type":"game-action","token":1,"op":"dragMove","fromArea":"inventory","fromIndex":35,"toArea":"crafting","toIndex":8}`,
	} {
		if _, err := DecodeUIEventBatch([]byte(`{"v":1,"events":[` + event + `]}`)); err != nil {
			t.Errorf("合法语义事件被拒绝: %v", err)
		}
	}
	for _, event := range []string{
		`{"type":"game-action","token":0,"op":"close"}`,
		`{"type":"game-action","token":1,"op":"hotbar","index":null}`,
		`{"type":"game-action","token":1,"op":"slot","area":"inventory","index":36}`,
		`{"type":"game-action","token":1,"op":"slot","area":"output","index":0}`,
		`{"type":"game-action","token":1,"op":"close","index":0}`,
		`{"type":"game-action","token":1,"op":"hotbar","index":9}`,
		`{"type":"game-action","token":1,"op":"slot","area":"crafting","index":0,"extra":true}`,
		// 旧字段集（缺按键语义）被新 schema 拒绝属预期，前端与 Go 同批发布。
		`{"type":"game-action","token":1,"op":"slot","area":"inventory","index":0}`,
		`{"type":"game-action","token":1,"op":"slot","area":"inventory","index":0,"button":"left"}`,
		`{"type":"game-action","token":1,"op":"slot","area":"inventory","index":0,"shift":false}`,
		`{"type":"game-action","token":1,"op":"slot","area":"inventory","index":0,"button":"middle","shift":false}`,
		`{"type":"game-action","token":1,"op":"slot","area":"inventory","index":0,"button":"left","shift":"false"}`,
		`{"type":"game-action","token":1,"op":"slot","area":"inventory","index":0,"button":"left","shift":null}`,
		// Drops reject the whole event for missing, out-of-range, or unknown slot addresses.
		`{"type":"game-action","token":1,"op":"drop"}`,
		`{"type":"game-action","token":1,"op":"drop","area":"inventory"}`,
		`{"type":"game-action","token":1,"op":"drop","area":"inventory","index":36}`,
		`{"type":"game-action","token":1,"op":"drop","area":"output","index":0}`,
		`{"type":"game-action","token":1,"op":"drop","area":"inventory","index":0,"button":"left"}`,
		// Drag moves reject the whole event when either endpoint is missing or out of range, or fields are extra.
		`{"type":"game-action","token":1,"op":"dragMove","fromArea":"inventory","fromIndex":0,"toArea":"inventory"}`,
		`{"type":"game-action","token":1,"op":"dragMove","fromArea":"inventory","fromIndex":36,"toArea":"inventory","toIndex":0}`,
		`{"type":"game-action","token":1,"op":"dragMove","fromArea":"inventory","fromIndex":0,"toArea":"chest","toIndex":27}`,
		`{"type":"game-action","token":1,"op":"dragMove","fromArea":"output","fromIndex":0,"toArea":"inventory","toIndex":0}`,
		`{"type":"game-action","token":1,"op":"dragMove","fromArea":"inventory","fromIndex":0,"toArea":"inventory","toIndex":1,"shift":false}`,
	} {
		if _, err := DecodeUIEventBatch([]byte(`{"v":1,"events":[` + event + `]}`)); err == nil {
			t.Errorf("非法事件被接受: %s", event)
		}
	}
}

// TestGameActionSlotPointerFieldsLandOnStruct 钉住按键语义字段的解码落位，
// 供 handleGameAction 的分堆/快捷搬运分支消费。
func TestGameActionSlotPointerFieldsLandOnStruct(t *testing.T) {
	events, err := DecodeUIEventBatch([]byte(`{"v":1,"events":[{"type":"game-action","token":7,"op":"slot","area":"chest","index":3,"button":"right","shift":true}]}`))
	if err != nil {
		t.Fatalf("解码携带按键语义的槽位事件: %v", err)
	}
	if len(events) != 1 || events[0].Kind != UIEventGameAction {
		t.Fatalf("事件形状: %#v", events)
	}
	action := events[0].GameAction
	if action.Button != "right" || !action.Shift {
		t.Fatalf("按键语义字段落位: %#v", action)
	}
}

// `TestGameActionDragAndDropFieldsLandOnStruct` verifies decoding for both drag
// operations: `dragMove` fills both endpoints and `drop` fills its single slot
// address for the `handleGameAction` drag branches.
func TestGameActionDragAndDropFieldsLandOnStruct(t *testing.T) {
	events, err := DecodeUIEventBatch([]byte(`{"v":1,"events":[` +
		`{"type":"game-action","token":9,"op":"dragMove","fromArea":"chest","fromIndex":3,"toArea":"inventory","toIndex":10},` +
		`{"type":"game-action","token":9,"op":"drop","area":"crafting","index":4}]}`))
	if err != nil {
		t.Fatalf("解码拖拽事件: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("事件数量: %#v", events)
	}
	drag := events[0].GameAction
	if drag.Op != "dragMove" || drag.FromArea != "chest" || drag.FromIndex != 3 ||
		drag.ToArea != "inventory" || drag.ToIndex != 10 {
		t.Fatalf("拖拽落槽字段落位: %#v", drag)
	}
	drop := events[1].GameAction
	if drop.Op != "drop" || drop.Area != "crafting" || drop.Index != 4 {
		t.Fatalf("拖出丢弃字段落位: %#v", drop)
	}
}
