package runtime

import "github.com/channing771/mornlea/internal/companion"

// EnqueueCompanionAction 把伙伴意图投递进有界 inbox，满员时立即拒绝。
func (engine *Engine) EnqueueCompanionAction(action CompanionAction) bool {
	engine.inboxMu.Lock()
	defer engine.inboxMu.Unlock()
	if len(engine.companionActions) >= companion.MaxActive {
		return false
	}
	engine.companionActions = append(engine.companionActions, action)
	return true
}

func (engine *Engine) takeCompanionActions() []CompanionAction {
	engine.inboxMu.Lock()
	actions := append([]CompanionAction(nil), engine.companionActions...)
	engine.companionActions = engine.companionActions[:0]
	engine.inboxMu.Unlock()
	return actions
}
