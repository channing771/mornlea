package protocol

import (
	"github.com/channing771/mornlea/packages/shared/core"
)

// DropStack requests dropping the entire stack addressed by a view slot.
// Its address space matches `MoveStackPartial`: `View` selects {0,1,2}, and
// `Slot` is the unified index: inventory 0..35, crafting 0..44, or container
// 0..38|62. Container views require a valid reference; other views require a
// zero `Container`. The server derives the position under the player from
// authoritative state, so the wire carries no coordinates. Empty slots are
// simulation-level rejections; protocol validation covers static domains.
type DropStack struct {
	Sequence  uint64
	Container core.ContainerRef
	View      uint8
	Slot      uint8
}

func (DropStack) clientMessage() {}
func (DropStack) clientPacket()  {}

func (command DropStack) Validate() error {
	// A drop has no destination slot. Passing `Slot` as both endpoints reuses
	// `validateStackSplit` for view, reference, and index validation.
	return validateStackSplit(command.View, command.Container, command.Slot, command.Slot)
}
