//go:build darwin

package client

import (
	"encoding/json"
	"fmt"
)

// UIGameAction 是带视图身份的语义操作；不携带像素坐标。槽位操作额外携带
// 按键类型（Button，left/right）与 Shift 修饰位（Shift），供分堆与快捷搬运
// 分支分派；拖拽落槽（dragMove）携带源/目标两个语义槽位引用，拖出面板丢弃
// （drop）复用 Area/Index 寻址；数量与落位一律由服务端推导。
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

// gameAreaIndexLimit 返回一个语义槽位区域的上闭界索引；未知区域返回 false。
// 槽位点击、拖拽落槽与拖出丢弃共用同一张区域→上界表，避免三处各写一份。
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

// decodeSlotAddress 把一对 (area,index) JSON 字段严格解码进目标字段，供单端
// （slot/drop）与双端（dragMove）寻址复用：未知区域与越界索引都整事件拒绝。
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
		// 拖出面板整组丢弃：单端寻址与槽位点击同界，无按键语义字段。
		required = append(required, "area", "index")
		if err := decodeSlotAddress(fields, "area", "index", &action.Area, &action.Index); err != nil {
			return UIEvent{}, err
		}
	case "dragMove":
		// 拖拽落槽：源/目标两端各自独立校验区域与索引域，跨区域合法。
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
