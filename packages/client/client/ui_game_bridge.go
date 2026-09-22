//go:build darwin

package client

import (
	"encoding/json"
	"fmt"
)

// UIGameAction 是带视图身份的语义操作；不携带像素坐标。槽位操作额外携带
// 按键类型（Button，left/right）与 Shift 修饰位（Shift），供分堆与快捷搬运
// branch dispatch. A `dragMove` carries source and destination semantic slot
// references, while `drop` reuses `Area` and `Index`; the server derives counts and placement.
type UIGameAction struct {
	Token     uint64 `json:"token"`
	Op        string `json:"op"`
	Area      string `json:"area,omitempty"`
	Index     int    `json:"index,omitempty"`
	Button    string `json:"button,omitempty"`
	Shift     bool   `json:"shift,omitempty"`
	FromArea  string `json:"fromArea,omitempty"`
	FromIndex int    `json:"fromIndex,omitempty"`
	ToArea    string `json:"toArea,omitempty"`
	ToIndex   int    `json:"toIndex,omitempty"`
}

// `gameAreaIndexLimit` returns the inclusive upper index bound for one semantic
// slot area, or false for an unknown area. Slot clicks, drag moves, and drops
// share this area-to-bound table.
func gameAreaIndexLimit(area string) (int, bool) {
	switch area {
	case "inventory":
		return 35, true
	case "crafting":
		return 8, true
	case "chest":
		return 26, true
	case "furnace":
		return 2, true
	default:
		return 0, false
	}
}

// `decodeSlotAddress` strictly decodes one `(area,index)` JSON pair for both
// single-ended `slot` and `drop` addressing and double-ended `dragMove`
// addressing. Unknown areas and out-of-range indices reject the whole event.
func decodeSlotAddress(
	fields map[string]json.RawMessage,
	areaField string,
	indexField string,
	area *string,
	index *int,
) error {
	if err := json.Unmarshal(fields[areaField], area); err != nil {
		return err
	}
	limit, ok := gameAreaIndexLimit(*area)
	if !ok {
		return fmt.Errorf("非法槽位区域")
	}
	var decoded *int
	if err := json.Unmarshal(fields[indexField], &decoded); err != nil || decoded == nil || *decoded < 0 || *decoded > limit {
		return fmt.Errorf("非法语义索引")
	}
	*index = *decoded
	return nil
}

func decodeGameActionEvent(fields map[string]json.RawMessage) (UIEvent, error) {
	var action UIGameAction
	if err := json.Unmarshal(fields["token"], &action.Token); err != nil || action.Token == 0 || action.Token > 9007199254740991 {
		return UIEvent{}, fmt.Errorf("非法视图 token")
	}
	if err := json.Unmarshal(fields["op"], &action.Op); err != nil {
		return UIEvent{}, err
	}
	required := []string{"type", "token", "op"}
	switch action.Op {
	case "close", "capture", "inventory", "character", "take-output":
	case "hotbar":
		required = append(required, "index")
		var index *int
		if err := json.Unmarshal(fields["index"], &index); err != nil || index == nil || *index < 0 || *index > 8 {
			return UIEvent{}, fmt.Errorf("非法语义索引")
		}
		action.Index = *index
	case "recipe":
		required = append(required, "index")
		var index *int
		if err := json.Unmarshal(fields["index"], &index); err != nil || index == nil || *index < 0 || *index > 9 {
			return UIEvent{}, fmt.Errorf("非法语义索引")
		}
		action.Index = *index
	case "slot":
		required = append(required, "area", "index", "button", "shift")
		if err := decodeSlotAddress(fields, "area", "index", &action.Area, &action.Index); err != nil {
			return UIEvent{}, err
		}
		if err := json.Unmarshal(fields["button"], &action.Button); err != nil {
			return UIEvent{}, err
		}
		if action.Button != "left" && action.Button != "right" {
			return UIEvent{}, fmt.Errorf("非法槽位按键类型")
		}
		var shift *bool
		if err := json.Unmarshal(fields["shift"], &shift); err != nil || shift == nil {
			return UIEvent{}, fmt.Errorf("非法槽位修饰位")
		}
		action.Shift = *shift
	case "drop":
		// A whole-stack drop uses the same single-ended bounds as a slot click and has no button fields.
		required = append(required, "area", "index")
		if err := decodeSlotAddress(fields, "area", "index", &action.Area, &action.Index); err != nil {
			return UIEvent{}, err
		}
	case "dragMove":
		// A drag move validates source and destination independently and permits cross-area moves.
		required = append(required, "fromArea", "fromIndex", "toArea", "toIndex")
		if err := decodeSlotAddress(fields, "fromArea", "fromIndex", &action.FromArea, &action.FromIndex); err != nil {
			return UIEvent{}, err
		}
		if err := decodeSlotAddress(fields, "toArea", "toIndex", &action.ToArea, &action.ToIndex); err != nil {
			return UIEvent{}, err
		}
	default:
		return UIEvent{}, fmt.Errorf("非法游戏操作")
	}
	if err := requireExactKeys(fields, required); err != nil {
		return UIEvent{}, err
	}
	return UIEvent{Kind: UIEventGameAction, GameAction: action}, nil
}
