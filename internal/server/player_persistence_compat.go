package server

import "github.com/channing771/mornlea/internal/server/persistence"

// ErrPlayerPersistenceBackpressure 保留根包的现有错误哨兵兼容面，恒等于子包的哨兵。
var ErrPlayerPersistenceBackpressure = persistence.ErrPlayerBackpressure
