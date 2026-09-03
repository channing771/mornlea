//go:build darwin

package app

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	networktcp "github.com/channing771/mornlea/internal/network/tcp"
	"github.com/channing771/mornlea/internal/render"
)

func TestApplicationCelestialParametersKeepLastAcceptedWorldTime(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	newest := network.PlayerState{
		ServerTick: 2, WorldTimeTicks: 6000, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
	}
	sendInteractiveServerMessage(t, serverEndpoint, newest)
	app.DrainServerMessages(1)
	want := render.DayNightAt(newest.WorldTimeTicks, 0)
	if got := render.DayNightAt(app.worldTimeTicks, app.dayPhaseOffset); got != want {
		t.Fatalf("接受新状态后的天体参数 = %+v，想要 %+v", got, want)
	}
	if got := want.SunDirection; math.Abs(float64(got[1]-1)) > 1e-5 || math.Abs(float64(got[0])) > 1e-5 || math.Abs(float64(got[2])) > 1e-5 {
		t.Fatalf("接受新状态后的太阳方向 = %v，想要正午天顶", got)
	}

	for _, stale := range []network.PlayerState{
		{ServerTick: 1, WorldTimeTicks: 18000, Dimension: core.Overworld, Position: newest.Position, OnGround: true, Ready: true},
		{ServerTick: 2, WorldTimeTicks: 12000, Dimension: core.Overworld, Position: newest.Position, OnGround: true, Ready: true},
	} {
		sendInteractiveServerMessage(t, serverEndpoint, stale)
		app.DrainServerMessages(1)
		if app.worldTimeTicks != newest.WorldTimeTicks {
			t.Fatalf("旧或重复状态将世界时间改为 %d，想要 %d", app.worldTimeTicks, newest.WorldTimeTicks)
		}
		if got := render.DayNightAt(app.worldTimeTicks, app.dayPhaseOffset); got != want {
			t.Fatalf("旧或重复状态改变天体参数 = %+v，想要 %+v", got, want)
		}
	}
}

// TestApplicationCelestialParametersFollowDayPhaseOffset 锁定显示相位偏移的
// 客户端消费链：偏移随权威 `PlayerState` 一起被接受（同样的「只认更新 tick」
// 纪律），昼夜呈现由 `(WorldTimeTicks + DayPhaseOffset)` 决定——offset 变更随
// 下一份权威状态立即切换，旧或重复状态既不回退世界时间也不回退偏移。
func TestApplicationCelestialParametersFollowDayPhaseOffset(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	newest := network.PlayerState{
		ServerTick: 2, WorldTimeTicks: 18000, DayPhaseOffset: 6000, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
	}
	sendInteractiveServerMessage(t, serverEndpoint, newest)
	app.DrainServerMessages(1)
	// 18000 的实际相位加偏移 6000 回绕到周期起点（白昼），而同一绝对时间在
	// 零偏移下是午夜：天空与光照随下一份权威状态切换生效。
	want := render.DayNightAt(newest.WorldTimeTicks, newest.DayPhaseOffset)
	if got := render.DayNightAt(app.worldTimeTicks, app.dayPhaseOffset); got != want {
		t.Fatalf("接受偏移状态后的天体参数 = %+v，想要 %+v", got, want)
	}
	if want != render.DayNightAt(0, 0) {
		t.Fatalf("夹具无效：18000+6000 应呈现周期起点，得到 %+v", want)
	}
	if app.dayPhaseOffset != newest.DayPhaseOffset {
		t.Fatalf("接受的偏移 = %d，想要 %d", app.dayPhaseOffset, newest.DayPhaseOffset)
	}

	for _, stale := range []network.PlayerState{
		{ServerTick: 1, WorldTimeTicks: 18000, DayPhaseOffset: 0, Dimension: core.Overworld, Position: newest.Position, OnGround: true, Ready: true},
		{ServerTick: 2, WorldTimeTicks: 18000, DayPhaseOffset: 23999, Dimension: core.Overworld, Position: newest.Position, OnGround: true, Ready: true},
	} {
		sendInteractiveServerMessage(t, serverEndpoint, stale)
		app.DrainServerMessages(1)
		if app.dayPhaseOffset != newest.DayPhaseOffset || app.worldTimeTicks != newest.WorldTimeTicks {
			t.Fatalf("旧或重复状态将偏移/时间改为 (%d,%d)，想要 (%d,%d)",
				app.dayPhaseOffset, app.worldTimeTicks, newest.DayPhaseOffset, newest.WorldTimeTicks)
		}
		if got := render.DayNightAt(app.worldTimeTicks, app.dayPhaseOffset); got != want {
			t.Fatalf("旧或重复状态改变天体参数 = %+v，想要 %+v", got, want)
		}
	}

	// 更新的权威状态携带新偏移：呈现立即切换到新偏移下的相位。
	newer := network.PlayerState{
		ServerTick: 3, WorldTimeTicks: 18001, DayPhaseOffset: 0, Dimension: core.Overworld,
		Position: newest.Position, OnGround: true, Ready: true,
	}
	sendInteractiveServerMessage(t, serverEndpoint, newer)
	app.DrainServerMessages(1)
	want = render.DayNightAt(newer.WorldTimeTicks, newer.DayPhaseOffset)
	if got := render.DayNightAt(app.worldTimeTicks, app.dayPhaseOffset); got != want {
		t.Fatalf("切换后的天体参数 = %+v，想要 %+v", got, want)
	}
	if app.dayPhaseOffset != 0 || app.worldTimeTicks != 18001 {
		t.Fatalf("新状态未生效: offset=%d ticks=%d", app.dayPhaseOffset, app.worldTimeTicks)
	}
}

func TestApplicationCelestialParametersMatchMemoryAndTCP(t *testing.T) {
	state := network.PlayerState{
		ServerTick: 1, WorldTimeTicks: 18000, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
	}
	var memory render.DayNight
	for _, transport := range []string{"memory", "tcp"} {
		t.Run(transport, func(t *testing.T) {
			clientEndpoint, serverEndpoint := celestialTestEndpoints(t, transport)
			app := &Application{
				clientEndpoint: clientEndpoint,
				receiver:       client.NewReceiver(clientEndpoint, 8),
				mirror:         client.NewMirror(),
				predictor:      client.NewPredictor(),
				serverCancel:   func() {},
			}
			sendInteractiveServerMessage(t, serverEndpoint, state)
			deadline := time.Now().Add(time.Second)
			for app.serverTick != state.ServerTick && time.Now().Before(deadline) {
				app.DrainServerMessages(1)
				time.Sleep(time.Millisecond)
			}
			if app.serverTick != state.ServerTick {
				t.Fatalf("等待权威世界时间超时: receiver error=%v", app.receiver.Err())
			}
			got := render.DayNightAt(app.worldTimeTicks, app.dayPhaseOffset)
			if math.Abs(float64(got.SunDirection[1]+1)) > 1e-5 || math.Abs(float64(got.SunDirection[0])) > 1e-5 || math.Abs(float64(got.SunDirection[2])) > 1e-5 {
				t.Fatalf("午夜太阳方向 = %v，想要地平线下方", got.SunDirection)
			}
			if transport == "memory" {
				memory = got
				return
			}
			if got != memory {
				t.Fatalf("TCP 天体参数 = %+v，想要与 Memory 相同的 %+v", got, memory)
			}
		})
	}
}

func celestialTestEndpoints(t *testing.T, transport string) (network.ClientEndpoint, network.ServerEndpoint) {
	t.Helper()
	if transport == "memory" {
		clientEndpoint, serverEndpoint := network.NewMemoryPair(8)
		t.Cleanup(func() {
			_ = clientEndpoint.Close()
			_ = serverEndpoint.Close()
		})
		return clientEndpoint, serverEndpoint
	}
	listener, err := networktcp.ListenTCP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	dialed := make(chan struct {
		stream network.ClientPacketStream
		err    error
	}, 1)
	go func() {
		stream, err := networktcp.DialTCP(context.Background(), listener.Addr())
		dialed <- struct {
			stream network.ClientPacketStream
			err    error
		}{stream, err}
	}()
	serverStream, err := listener.Accept(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan struct {
		endpoint network.ServerEndpoint
		err      error
	}, 1)
	go func() {
		pending, err := network.BeginServerLogin(context.Background(), serverStream, 0)
		if err != nil {
			accepted <- struct {
				endpoint network.ServerEndpoint
				err      error
			}{err: err}
			return
		}
		var endpoint network.ServerEndpoint
		err = pending.Accept(context.Background(), func(attached network.ServerEndpoint) error {
			endpoint = attached
			return nil
		})
		accepted <- struct {
			endpoint network.ServerEndpoint
			err      error
		}{endpoint, err}
	}()
	clientStream := <-dialed
	if clientStream.err != nil {
		t.Fatal(clientStream.err)
	}
	clientEndpoint, err := network.LoginClient(context.Background(), clientStream.stream, network.Identity{
		PlayerID: integrationPlayerID(9), DisplayName: "Celestial",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := <-accepted
	if server.err != nil {
		t.Fatal(server.err)
	}
	t.Cleanup(func() {
		_ = clientEndpoint.Close()
		_ = server.endpoint.Close()
	})
	return clientEndpoint, server.endpoint
}

// TestApplicationFreezeWorldTimeBlocksNewerStateWhileFrozen 锁定世界时间冻结:
// capture 在场景 Apply 里钉住世界时间后,收敛帧仍会持续接受权威状态——
// 服务端时间随真实时间前进,最终帧的昼夜参数因此随进程漂移(golden 逐像素
// 门禁在远景带上整片翻色)。冻结开关只拦 `WorldTimeTicks`/`DayPhaseOffset`
// 两个呈现量的接受,其余权威状态照常前进;解冻后恢复「只认更新 tick」纪律。
func TestApplicationFreezeWorldTimeBlocksNewerStateWhileFrozen(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	base := network.PlayerState{
		ServerTick: 1, WorldTimeTicks: 18000, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
	}
	sendInteractiveServerMessage(t, serverEndpoint, base)
	app.DrainServerMessages(1)
	if app.worldTimeTicks != 18000 {
		t.Fatalf("基线世界时间 = %d,想要 18000", app.worldTimeTicks)
	}

	app.SetWorldTimeFrozen(true)
	newer := base
	newer.ServerTick, newer.WorldTimeTicks = 2, 6000
	sendInteractiveServerMessage(t, serverEndpoint, newer)
	app.DrainServerMessages(1)
	if app.worldTimeTicks != 18000 {
		t.Fatalf("冻结期间世界时间被更新状态改为 %d,想要保持 18000", app.worldTimeTicks)
	}

	app.SetWorldTimeFrozen(false)
	newest := newer
	newest.ServerTick, newest.WorldTimeTicks = 3, 12000
	sendInteractiveServerMessage(t, serverEndpoint, newest)
	app.DrainServerMessages(1)
	if app.worldTimeTicks != 12000 {
		t.Fatalf("解冻后更新的世界状态未被接受: %d,想要 12000", app.worldTimeTicks)
	}
}
