package persistence

import "github.com/channing771/mornlea/internal/companion"

// CompanionSummary 是伙伴持久化对外暴露的最小摘要值类型。
type CompanionSummary struct {
	ID      companion.ID
	Summary string
}
