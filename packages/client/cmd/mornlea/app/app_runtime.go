//go:build darwin

package app

// app_runtime.go: adoption seam between the legacy `Application` session fields
// and `packages/client/runtime`. The app keeps owning every presentation,
// window, audio, and lifecycle object; the runtime owns protocol message
// routing, prediction advance, and mesh scheduling, applied in place to the
// app's current mirror, predictor, and mesher objects.

import (
	"fmt"

	"github.com/channing771/mornlea/packages/client/client"
	clientruntime "github.com/channing771/mornlea/packages/client/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// sessionAdoption records the exact host objects one adopted runtime was built
// from. Test and capture assembly legitimately replace mirror, predictor,
// mesher, receiver, and endpoint objects at any time, so every use re-checks
// pointer identity and re-adopts on mismatch instead of keeping a stale runtime
// draining into detached objects.
type sessionAdoption struct {
	receiver      *client.Receiver
	clientAddress network.ClientEndpoint
	mirror        *client.Mirror
	predictor     *client.Predictor
	mesher        *client.Mesher
	inventory     *client.InventoryMirror
	crafting      *client.CraftingMirror
	chest         *client.ChestMirror
	furnace       *client.FurnaceMirror
	chatEvents    *client.ChatEvents
	itemDrops     *client.ItemDrops
	remotePlayers *client.RemotePlayers
	companions    *client.Companions
	hostiles      *client.Hostiles
	passives      *client.Passives
	projectiles   *client.Projectiles
}

func (a *Application) captureSessionAdoption() sessionAdoption {
	return sessionAdoption{
		receiver:      a.receiver,
		clientAddress: a.clientEndpoint,
		mirror:        a.mirror,
		predictor:     a.predictor,
		mesher:        a.mesher,
		inventory:     &a.inventory,
		crafting:      &a.crafting,
		chest:         &a.chest,
		furnace:       &a.furnace,
		chatEvents:    a.chatEvents,
		itemDrops:     a.itemDrops,
		remotePlayers: a.remotePlayers,
		companions:    a.companions,
		hostiles:      a.hostiles,
		passives:      a.passives,
		projectiles:   a.projectiles,
	}
}

func (adoption sessionAdoption) matches(current sessionAdoption) bool {
	return adoption == current
}

// ensureSessionRuntime returns a runtime adopted from the current session
// fields, creating or re-creating it when the previous adoption no longer
// matches. Adoption is frame-thread only; the returned runtime owns no
// resources, so replacing it leaks nothing.
func (a *Application) ensureSessionRuntime() *clientruntime.Runtime {
	if a.sessionRuntime != nil {
		if a.sessionAdoption.matches(a.captureSessionAdoption()) {
			return a.sessionRuntime
		}
		a.discardSessionRuntime()
	}
	adoption := a.captureSessionAdoption()
	if adoption.mirror == nil || adoption.predictor == nil {
		return nil
	}
	session := clientruntime.AdoptedSession{
		World:         adoption.mirror,
		Predictor:     adoption.predictor,
		Inventory:     adoption.inventory,
		Crafting:      adoption.crafting,
		Chest:         adoption.chest,
		Furnace:       adoption.furnace,
		Chat:          adoption.chatEvents,
		ItemDrops:     adoption.itemDrops,
		RemotePlayers: adoption.remotePlayers,
		Companions:    adoption.companions,
		Hostiles:      adoption.hostiles,
		Passives:      adoption.passives,
		Projectiles:   adoption.projectiles,
		// The app-maintained baselines seed the runtime's stale-state guard and
		// protocol sequence space so a re-adoption continues both without a
		// reset, matching the legacy app fields that survive session resets.
		PlayerTick: a.serverTick,
		Sequence:   a.sequence,
	}
	if adoption.receiver != nil {
		session.Receiver = adoption.receiver
	}
	if adoption.clientAddress != nil {
		session.Sender = adoption.clientAddress
	}
	runtimeSession, err := clientruntime.Adopt(session)
	if err != nil {
		// Adoption fails only when the core session objects are missing, which
		// the guards above already exclude; treat the impossible case as no
		// session rather than panicking inside the frame path.
		return nil
	}
	if adoption.mesher != nil {
		// The fresh runtime has no meshing configured yet; adopting the host
		// mesher gives the runtime the ready queue, per-section revisions, and
		// epoch identity while the app keeps worker-pool ownership and close.
		// Ready capacity bounds payload-bearing upsert publication only:
		// payload-less forget drops are capacity-exempt, so warp-scale forget
		// bursts never terminate the session or delay fresh uploads.
		if err := runtimeSession.AdoptMeshing(adoption.mesher, clientruntime.MaxMeshReadyCapacity); err != nil {
			return nil
		}
	}
	a.sessionRuntime = runtimeSession
	a.sessionAdoption = adoption
	return runtimeSession
}

// advanceSessionMeshes drives the adopted runtime's mesh pipeline for one game
// frame: one bounded scheduling/drain pass feeds the runtime-owned ready queue,
// and the drained section results (quads plus connectivity) feed the existing
// scheduler upload path. Drops carry no quads and sink as scheduler section
// drops, matching the legacy forget handling. Mesh pipeline failures are
// terminal (bounded publication would otherwise lose a section), so they close
// the client session and surface as a frame error.
func (a *Application) advanceSessionMeshes(workMax int) error {
	session := a.ensureSessionRuntime()
	if session == nil {
		return nil
	}
	if err := session.AdvanceMeshes(workMax); err != nil {
		a.CloseClientSession(err)
		return fmt.Errorf("推进会话网格化: %w", err)
	}
	var err error
	a.meshSections, _, err = session.DrainMeshedSections(a.meshSections[:0], workMax)
	if err != nil {
		a.CloseClientSession(err)
		return fmt.Errorf("排空会话网格结果: %w", err)
	}
	for _, section := range a.meshSections {
		if section.Dimension != core.Overworld {
			continue
		}
		// Legacy drain registered connectivity for every mesher result, empty
		// meshes included; world-forget removals carry none and leave earlier
		// registrations untouched.
		if section.ConnectivityKnown {
			a.scheduler.SetConnectivity(section.Pos, section.Conn)
		}
		if section.Upsert {
			a.scheduler.QueueSection(section.Pos, section.Quads)
		} else {
			a.scheduler.QueueSection(section.Pos, nil)
		}
	}
	return nil
}

// discardSessionRuntime drops the adopted runtime at a session boundary. The
// next frame-path use re-adopts from the then-current fields.
func (a *Application) discardSessionRuntime() {
	a.sessionRuntime = nil
	a.sessionAdoption = sessionAdoption{}
}
