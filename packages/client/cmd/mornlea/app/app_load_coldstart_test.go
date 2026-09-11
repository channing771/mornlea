//go:build darwin

package app

import (
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
)

type coldstartFakeApp struct{ viewDistance int }

func (f coldstartFakeApp) Frame(_, _ int, _ time.Duration) (bool, error) {
	return false, nil
}
func (f coldstartFakeApp) Window() Window { return nil }
func (f coldstartFakeApp) LoadedChunks() map[core.ChunkPos]struct{} {
	return map[core.ChunkPos]struct{}{}
}
func (f coldstartFakeApp) Mesher() *client.Mesher { return nil }
func (f coldstartFakeApp) Scheduler() *render.SectionScheduler {
	return nil
}
func (f coldstartFakeApp) Render() config.Render {
	return config.Render{ViewDistance: f.viewDistance}
}

func TestLoadedChunkTargetFormulaLocked(t *testing.T) {
	t.Parallel()
	if got := LoadedChunkTarget(coldstartFakeApp{viewDistance: 0}); got != 9 {
		t.Fatalf("VD=0 got %d, want 9", got)
	}
	if got := LoadedChunkTarget(coldstartFakeApp{viewDistance: 1}); got != 25 {
		t.Fatalf("VD=1 got %d, want 25", got)
	}
}
