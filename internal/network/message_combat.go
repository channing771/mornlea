package network

import (
	"fmt"

	"github.com/channing771/mornlea/internal/core"
)

const combatHitWireBytes = 10

type CombatHit struct {
	ServerTick uint64
	Damage     uint8
	TargetKind core.CombatTargetKind
}

func (CombatHit) serverMessage() {}
func (CombatHit) serverPacket()  {}

func (hit CombatHit) Validate() error {
	if hit.ServerTick == 0 {
		return fmt.Errorf("network: combat hit server tick is zero")
	}
	if hit.Damage == 0 || hit.Damage > core.MaxHealth {
		return fmt.Errorf("network: combat hit damage %d outside 1..%d", hit.Damage, core.MaxHealth)
	}
	if !hit.TargetKind.Valid() {
		return fmt.Errorf("network: combat hit target kind %d is invalid", hit.TargetKind)
	}
	return nil
}
