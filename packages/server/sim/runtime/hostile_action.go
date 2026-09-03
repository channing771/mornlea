package runtime

const maxHostileActionsPerTick = 64

// EnqueueHostileAction 把夜行者意图投递进有界 inbox，满员时立即拒绝。
func (engine *Engine) EnqueueHostileAction(action HostileAction) bool {
	engine.inboxMu.Lock()
	defer engine.inboxMu.Unlock()
	if len(engine.hostileActions) >= maxHostileActionsPerTick {
		return false
	}
	engine.hostileActions = append(engine.hostileActions, action)
	return true
}

func (engine *Engine) takeHostileActions() []HostileAction {
	engine.inboxMu.Lock()
	actions := append([]HostileAction(nil), engine.hostileActions...)
	engine.hostileActions = engine.hostileActions[:0]
	engine.inboxMu.Unlock()
	return actions
}
