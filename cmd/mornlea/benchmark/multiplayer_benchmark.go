//go:build darwin

package benchmark

import (
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render"
)

const (
	fixedBenchmarkFrameDuration = 50 * time.Millisecond
	benchmarkOutboxLimit        = 512
	benchmarkLatencyCapacity    = 131_072
)

type multiplayerClientProbe struct {
	app      BenchmarkApplication
	scenario application.MultiplayerBenchmarkScenario
	codec    *network.Codec
	roster   *client.RemotePlayers

	encode       *client.LatencyRecorder
	decode       *client.LatencyRecorder
	rosterApply  *client.LatencyRecorder
	interpolate  *client.LatencyRecorder
	renderTiming *application.MultiplayerRenderTiming
	gpuComplete  *client.LatencyRecorder
	now          func() time.Time
	tick         uint64
}

func newMultiplayerClientProbe(app BenchmarkApplication) (*multiplayerClientProbe, error) {
	codec, err := network.NewCodec()
	if err != nil {
		return nil, err
	}
	probe := &multiplayerClientProbe{
		app:          app,
		scenario:     application.NewMultiplayerBenchmarkScenario(),
		codec:        codec,
		roster:       client.NewRemotePlayers(),
		encode:       client.NewLatencyRecorder(benchmarkLatencyCapacity),
		decode:       client.NewLatencyRecorder(benchmarkLatencyCapacity),
		rosterApply:  client.NewLatencyRecorder(benchmarkLatencyCapacity),
		interpolate:  client.NewLatencyRecorder(benchmarkLatencyCapacity),
		renderTiming: application.NewMultiplayerRenderTiming(benchmarkLatencyCapacity),
		gpuComplete:  client.NewLatencyRecorder(client.ScenarioV12GPUCompletionSamples),
		now:          time.Now,
		tick:         1,
	}
	for _, spawn := range probe.scenario.Spawns {
		if err := probe.roster.Apply(spawn); err != nil {
			probe.Close()
			return nil, err
		}
		if err := app.RemotePlayers().Apply(spawn); err != nil {
			probe.Close()
			return nil, err
		}
	}
	app.SetMultiplayerRenderTiming(probe.renderTiming)
	app.SetMultiplayerRenderNow(time.Now)
	return probe, nil
}

func (probe *multiplayerClientProbe) Close() {
	if probe != nil && probe.app != nil && probe.app.MultiplayerRenderTiming() == probe.renderTiming {
		probe.app.SetMultiplayerRenderTiming(nil)
		probe.app.SetMultiplayerRenderNow(nil)
		probe.app = nil
	}
	if probe != nil && probe.codec != nil {
		probe.codec.Close()
		probe.codec = nil
	}
}

func (probe *multiplayerClientProbe) sampleFrame(app BenchmarkApplication, tick uint64) error {
	batch := probe.statesNearCamera(tick, app.Camera().Pos)
	started := time.Now()
	packetID, payload, err := probe.codec.EncodeServer(network.StatePlay, batch)
	probe.encode.Add(time.Since(started))
	if err != nil {
		return fmt.Errorf("编码远端状态: %w", err)
	}
	started = time.Now()
	decoded, err := probe.codec.DecodeServer(network.StatePlay, packetID, payload)
	probe.decode.Add(time.Since(started))
	if err != nil {
		return fmt.Errorf("解码远端状态: %w", err)
	}
	message, ok := decoded.(network.ServerMessage)
	if !ok {
		return fmt.Errorf("解码远端状态得到非 ServerMessage: %T", decoded)
	}
	started = time.Now()
	if err := app.RemotePlayers().Apply(message); err != nil {
		return fmt.Errorf("应用远端 roster: %w", err)
	}
	probe.rosterApply.Add(time.Since(started))
	if err := probe.roster.Apply(message); err != nil {
		return fmt.Errorf("应用 GPU 样本 roster: %w", err)
	}
	started = time.Now()
	probe.roster.Advance(fixedBenchmarkFrameDuration)
	probe.interpolate.Add(time.Since(started))
	return nil
}

func (probe *multiplayerClientProbe) statesNearCamera(tick uint64, camera mgl32.Vec3) network.RemotePlayerStates {
	batch := probe.scenario.States(tick)
	for index := range batch.Players {
		base := probe.scenario.Spawns[index].Position
		batch.Players[index].Position = batch.Players[index].Position.Sub(base).Add(camera).Add(
			mgl32.Vec3{0, -1.6, -6},
		)
	}
	return batch
}

func benchmarkBillboardCamera(app BenchmarkApplication) render.BillboardCamera {
	right := mgl32.Vec3{
		float32(math.Cos(float64(app.Camera().Yaw))), 0,
		-float32(math.Sin(float64(app.Camera().Yaw))),
	}
	return render.BillboardCamera{
		ViewProj: app.Camera().ViewProj(), Right: right,
		Up: right.Cross(app.Camera().Forward()).Normalize(),
	}
}

// gpuCompletionChunks 是一个样本拆成的 command buffer 数量。
const gpuCompletionChunks = client.ScenarioV12GPUCompletionBatch /
	client.ScenarioV12GPUCompletionChunk

func (probe *multiplayerClientProbe) measureGPUCompletion(app BenchmarkApplication) error {
	avatars, tags := application.RemoteRenderPresentations(probe.roster.Presentations())
	// 切换到 Rust 渲染器后,一个样本是一批完整 RenderFrame(含提交与完成)
	// 的总耗时摊到批次数;Poll 的固定节拍在样本内只出现一次,被摊薄到可忽略。
	for range client.ScenarioV12GPUCompletionSamples {
		if err := app.NameTagRenderer().Prepare(tags, app.Scheduler().UploadBudget()); err != nil {
			return err
		}
		viewProj := app.Camera().ViewProj()
		avatarStream := (&render.InstanceEncoder{}).EncodeAvatarInstances(nil, avatars)
		billboard := benchmarkBillboardCamera(app)
		backgrounds, glyphs := app.NameTagRenderer().FrameStreams()
		frame := client.RenderFrame{
			ViewProj:        viewProj,
			ViewProjInv:     viewProj.Inv(),
			Pos:             app.Camera().Pos,
			Daylight:        1,
			SkyColor:        [4]float32{0, 0, 0, 1},
			AvatarInstances: avatarStream,
			NameTagSegment: client.EncodeQuadSegment(
				render.EncodeBillboardCameraBytes(nil, billboard), backgrounds, glyphs, 64,
			),
		}
		started := probe.now()
		for range client.ScenarioV12GPUCompletionBatch {
			app.Renderer().RenderFrame(frame)
		}
		probe.gpuComplete.Add(probe.now().Sub(started) / client.ScenarioV12GPUCompletionBatch)
		// 每个样本都回收:ru_maxrss 是进程生命周期的历史峰值,必须阻止
		// 采样过程中的对象累积。
		runtime.GC()
	}
	return nil
}

func (probe *multiplayerClientProbe) Summary() client.MultiplayerSummary {
	avatarSubmit, nameTagSubmit := probe.renderTiming.Summaries()
	return client.MultiplayerSummary{
		RemoteStateEncode:      probe.encode.Summary(),
		RemoteStateDecode:      probe.decode.Summary(),
		RosterApply:            probe.rosterApply.Summary(),
		Interpolation:          probe.interpolate.Summary(),
		AvatarSubmit:           avatarSubmit,
		NameTagSubmit:          nameTagSubmit,
		RemoteGPUComplete:      probe.gpuComplete.Summary(),
		RemoteGPUCompleteBatch: client.ScenarioV12GPUCompletionBatch,
	}
}
