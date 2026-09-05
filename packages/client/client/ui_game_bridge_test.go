//go:build darwin

package client

import "testing"

func TestGameActionBridgeStrictValidation(t *testing.T) {
	for _, event := range []string{
		`{"type":"game-action","token":1,"op":"slot","area":"inventory","index":35}`,
		`{"type":"game-action","token":1,"op":"close"}`,
		`{"type":"game-action","token":1,"op":"hotbar","index":8}`,
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
	} {
		if _, err := DecodeUIEventBatch([]byte(`{"v":1,"events":[` + event + `]}`)); err == nil {
			t.Errorf("非法事件被接受: %s", event)
		}
	}
}
