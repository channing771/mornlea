//go:build darwin

package client

import (
	"encoding/json"
	"fmt"
)

// UIGameAction 是带视图身份的语义操作；不携带像素坐标。槽位操作额外携带
// 按键类型（Button，left/right）与 Shift 修饰位（Shift），供分堆与快捷搬运
// 分支分派；数量与落位一律由服务端推导。
type UIGameAction struct {
	Token  uint64 `json:"token"`
	Op     string `json:"op"`
	Area   string `json:"area,omitempty"`
	Index  int    `json:"index,omitempty"`
	Button string `json:"button,omitempty"`
	Shift  bool   `json:"shift,omitempty"`
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
	limit := -1
	switch action.Op {
	case "close", "capture", "inventory", "character", "take-output":
	case "hotbar":
		limit = 8
	case "recipe":
		limit = 9
	case "slot":
		required = append(required, "area", "button", "shift")
		if err := json.Unmarshal(fields["area"], &action.Area); err != nil {
			return UIEvent{}, err
		}
		switch action.Area {
		case "inventory":
			limit = 35
		case "crafting":
			limit = 8
		case "chest":
			limit = 26
		case "furnace":
			limit = 2
		default:
			return UIEvent{}, fmt.Errorf("非法槽位区域")
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
	default:
		return UIEvent{}, fmt.Errorf("非法游戏操作")
	}
	if limit >= 0 {
		required = append(required, "index")
		var index *int
		if err := json.Unmarshal(fields["index"], &index); err != nil || index == nil || *index < 0 || *index > limit {
			return UIEvent{}, fmt.Errorf("非法语义索引")
		}
		action.Index = *index
	}
	if err := requireExactKeys(fields, required); err != nil {
		return UIEvent{}, err
	}
	return UIEvent{Kind: UIEventGameAction, GameAction: action}, nil
}
