package server

import "github.com/channing771/mornlea/packages/server/server/persistence"

// PersistenceStatus 保留根包的现有状态类型兼容面。
type PersistenceStatus = persistence.Status

// PersistenceStatus 返回当前存档积压、背压和最近完成状态的值副本。
func (server *Server) PersistenceStatus() PersistenceStatus {
	server.stepMu.Lock()
	defer server.stepMu.Unlock()
	return server.world.Status()
}
