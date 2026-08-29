//go:build persistence_contract

package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/server"
	"github.com/channing771/mornlea/internal/server/persistence"
	"github.com/channing771/mornlea/internal/sim"
	"github.com/channing771/mornlea/internal/storage"
)

type worldLifecycle interface {
	Drain() error
	Observe(tick, worldTime uint64)
	Flush(context.Context) error
	ShutdownContextError(ctxErr, persistenceErr error) error
	Close()
	Status() persistence.Status
}

type playersLifecycle interface {
	Prepare(context.Context, core.PlayerID, string, storage.Metadata) (sim.PlayerRestore, error)
	Activate(core.PlayerID, string) error
	Confirm(core.PlayerID)
	Abort(core.PlayerID)
	Deactivate(core.PlayerID)
	Observe(core.PlayerID, string, sim.PlayerSnapshot, uint64, bool) error
	Poll(uint64) error
	Flush(context.Context) error
	Close()
	QueueDepths() (int, int)
}

type companionsLifecycle interface {
	Restore() ([]companion.Body, []storage.StoredCompanionQueue)
	Observe([]companion.Body, []companion.TaskQueueState, []persistence.CompanionSummary)
	Poll(uint64) error
	Flush(context.Context) error
	Close()
}

type hostilesLifecycle interface {
	Restore() []sim.HostileMob
	Observe([]sim.HostileMob)
	Poll(uint64) error
	Flush(context.Context) error
	Close()
}

var (
	_ = persistence.Options{}
	_ = persistence.Status{
		DirtyChunks:       0,
		EstimatedBytes:    0,
		InFlightChunks:    0,
		Backpressured:     false,
		LastSuccess:       time.Time{},
		LastError:         "",
		LastErrorAt:       time.Time{},
		AutosaveDrained:   false,
		MetadataPending:   false,
		MetadataInFlight:  false,
		MetadataLastError: "",
	}
	_ = persistence.CompanionSummary{ID: companion.ID{}, Summary: ""}

	_ int       = persistence.Status{}.DirtyChunks
	_ int64     = persistence.Status{}.EstimatedBytes
	_ int       = persistence.Status{}.InFlightChunks
	_ bool      = persistence.Status{}.Backpressured
	_ time.Time = persistence.Status{}.LastSuccess
	_ string    = persistence.Status{}.LastError
	_ time.Time = persistence.Status{}.LastErrorAt
	_ bool      = persistence.Status{}.AutosaveDrained
	_ bool      = persistence.Status{}.MetadataPending
	_ bool      = persistence.Status{}.MetadataInFlight
	_ string    = persistence.Status{}.MetadataLastError

	_ func(storage.Store, *sim.Engine, persistence.Options) *persistence.World                            = persistence.NewWorld
	_ func(storage.PlayerStore, persistence.Options) *persistence.Players                                 = persistence.NewPlayers
	_ func(storage.CompanionStore, storage.StoredCompanions, persistence.Options) *persistence.Companions = persistence.NewCompanions
	_ func(storage.HostileMobStore, storage.StoredHostileMobs, persistence.Options) *persistence.Hostiles = persistence.NewHostiles

	_ worldLifecycle      = (*persistence.World)(nil)
	_ playersLifecycle    = (*persistence.Players)(nil)
	_ companionsLifecycle = (*persistence.Companions)(nil)
	_ hostilesLifecycle   = (*persistence.Hostiles)(nil)

	_ persistence.Status = server.PersistenceStatus{}
	_ error              = persistence.ErrPlayerBackpressure
	_ error              = server.ErrPlayerPersistenceBackpressure
)

func TestPublicPersistenceContracts(t *testing.T) {
	if server.ErrPlayerPersistenceBackpressure != persistence.ErrPlayerBackpressure {
		t.Fatal("root player backpressure sentinel is not the child sentinel")
	}
}
