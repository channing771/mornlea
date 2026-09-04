//go:build darwin

package app

// menu-vista：主菜单与设置页相位的世界全景背景。
//
// 全景是一条不装配世界的渲染层演示管线：固定种子调用 worldgen 纯函数生成
// 菜单相机周围有限区块 → 经 `server.BuildChunkSnapshot`（与权威发布同一份
// 编码出口）进入全景专属客户端镜像 → 既有 mesher 烘焙 → 既有
// `SectionScheduler` 上传 → 既有地形/天空/光照 pass 渲染；远环地平线复用
// 既有 `lod.Scheduler` 壳带。它不打开世界存储、不启动本地权威服务端、
// 不登录（结构性守卫：`TestMenuVistaDoesNotAssembleWorld`）。
//
// 与游戏世界完全隔离：全景拥有独立的 mirror/mesher/scheduler/远环调度器，
// 且世界放在远离出生点的固定锚点上——游戏相位的任何调度、丢弃与可见列表
// 都不会触碰全景内容，反之全景也绝不改写游戏世界的呈现状态（capture 的
// 世界场景 golden 因此逐像素不受影响）。
//
// 确定性：相机自转角是整数 tick 的纯函数（`menuVistaYawAt`），世界内容是
// 种子的纯函数；进入菜单相位把 tick 归零，同种子逐帧一致（spec
// webview-menu-ui「全景背景确定性」）。capture 场景经 `SetMenuVistaTick`
// 在收敛后钉住 tick，golden 因此与机器速度无关。

import (
	"log/slog"
	"math"
	"runtime"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/lod"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/server/server"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

const (
	// MenuVistaYawPeriodTicks 是相机自转一整圈耗费的整数 tick（渲染帧）。
	// 60fps 下约 48 秒一圈，即「缓慢自转」；导出供 capture 场景在周期内
	// 挑选确定性的钉住时刻。
	MenuVistaYawPeriodTicks = 2880
	// menuVistaSeed 是全景世界的固定种子：与任何存档/登录种子无关，同构建
	// 逐格一致。
	menuVistaSeed int64 = 20260829
	// menuVistaAnchorChunk 是全景世界的固定锚点区块（1024,1024），对应
	// block (16384,*,16384)：离出生点足够远，游戏相机的视距丢弃、远环带与
	// 视锥都永不覆盖它；又在 float32 世界坐标精度充裕的范围内。
	menuVistaAnchorX int32 = 1024
	menuVistaAnchorZ int32 = 1024
	// menuVistaRadiusChunks 是近景网格的区块半径（切比雪夫）：12 chunk =
	// 192 block，覆盖固定相机俯仰下的近景地形带，更远处由远环壳接手。
	menuVistaRadiusChunks = 12
	// menuVistaChunksPerFrame 是每帧生成的区块预算：惰性/后台生成在帧循环
	// 内分摊，不阻塞渲染热路径。
	menuVistaChunksPerFrame = 4
	// menuVistaCameraLift 是相机在地面之上的固定抬升（block）。取值让画面
	// 同容纳天空与地形，且相机保持在远环壳带最高点之下（与 far-horizon 场景
	// 「相机低于壳上界」同一构图纪律，跨壳缝的视线因此被壳体正面遮挡）。
	menuVistaCameraLift = 30
	// menuVistaLowestGroundY 是相机地面的下界：锚点若落在海底（HeightAt
	// 返回海床），相机仍保证在海面之上，不会沉入水下视角。
	menuVistaLowestGroundY = 63
	// menuVistaWorldTimeTicks 是全景固定的世界时间：6000 为正午，日光与
	// 太阳高度取最大值，昼夜呈现不随任何权威时间漂移。
	menuVistaWorldTimeTicks = 6000
	// menuVistaPitch 是固定俯仰：小幅下俯让画面同时容纳天空与地形。
	menuVistaPitch float32 = -0.25
)

// menuVistaSink 是全景对 Rust 渲染器的上传出口抽象：近景区段与远环 tile
// 共用同一个渲染器句柄（`*client.Renderer` 隐式实现），测试注入计数替身。
type menuVistaSink interface {
	render.SectionSink
	lod.TileSink
}

// menuVista 是全景管线的全部状态。只在渲染帧线程构造、推进与释放，无并发
// 访问；生命周期为「进入菜单相位惰性构建 → 世界装配丢弃 → 重进菜单重建」。
type menuVista struct {
	generator    *worldgen.Generator
	mirror       *client.Mirror
	mesher       *client.Mesher
	scheduler    *render.SectionScheduler
	lodScheduler *lod.Scheduler
	// center 是全景近景网格的中心区块（锚点），上传冲刷与丢弃半径的圆心。
	center core.ChunkPos
	// lodCenter/inner/outer 是远环带的播种域：内半径把近景网格覆盖的 tile
	// 排除在外（与游戏远环同一 Ruling 19 语义，壳不会在近景之上戳出地表），
	// 外半径与游戏配置同源，保证全景地平线没入同一道雾。
	lodCenter core.ChunkPos
	lodInner  int
	lodOuter  int
	// cameraPos 是固定相机位置（锚点上空、地面下界之上），逐帧恒定。
	cameraPos mgl32.Vec3
	// queue 是待生成区块的确定性序列（切比雪夫环序：近处先出图）。
	queue []core.ChunkPos
	// nextRevision 是镜像快照的递增 revision（快照校验要求非零）。
	nextRevision uint64
	// lodSeeded 防止远环带重复播种（QueueRing 幂等，这里只省一次全环扫描）。
	lodSeeded bool
	// tick 是自转时钟：每渲染一帧加一，相位重进归零，capture 可钉住。
	tick uint64
}

// menuVistaYawAt 把整数 tick 映射到 [0, 2π) 的自转角。按周期取余后再做
// 纯浮点线性映射，角度因此只由 tick 决定，与墙钟、帧间隔和机器速度无关。
func menuVistaYawAt(tick uint64) float64 {
	phase := float64(tick%MenuVistaYawPeriodTicks) / float64(MenuVistaYawPeriodTicks)
	return phase * 2 * math.Pi
}

// newMenuVista 构造一条完整全景管线：worldgen 纯函数生成器、全景专属
// 镜像/mesher/区段调度器与远环调度器，并推导固定相机位置与确定性生成
// 序列。registry 必须与会话材质一致，全景与游戏画面才共享同一套材质映射。
func newMenuVista(
	sink menuVistaSink,
	registry *assets.Registry,
	renderConfig config.Render,
	fluidEnabled bool,
) (*menuVista, error) {
	if sink == nil {
		return nil, nil
	}
	renderConfig = renderConfig.NormalizeLOD()
	generator := worldgen.New(menuVistaSeed, fluidEnabled)
	center := core.ChunkPos{X: menuVistaAnchorX, Z: menuVistaAnchorZ}
	// 相机地面取锚点中心的最高实心方块：纯单点查询，同种子逐位一致；
	// 海底下界防御保证相机永远在海面与地形之上。
	groundY := generator.HeightAt(
		menuVistaAnchorX*core.SectionSize+core.SectionSize/2,
		menuVistaAnchorZ*core.SectionSize+core.SectionSize/2,
	)
	lift := float32(max(groundY+1, menuVistaLowestGroundY) + menuVistaCameraLift)
	// worker 数与近环 mesher 同式：菜单相位没有权威消息流量，近环 mesher
	// 空闲，全景装配独享同样的 CPU 配额，出图速度与游戏初始加载同量级。
	mesher := client.NewMesher(registry, max(1, runtime.NumCPU()-2))
	lodScheduler, err := lod.NewScheduler(
		sink,
		generator.Header(),
		uint32(renderConfig.LodStep),
		applicationUploadPerFrame,
	)
	if err != nil {
		mesher.Close()
		return nil, err
	}
	vista := &menuVista{
		generator:    generator,
		mirror:       client.NewMirror(),
		mesher:       mesher,
		scheduler:    render.NewSectionScheduler(sink, applicationUploadPerFrame),
		lodScheduler: lodScheduler,
		center:       center,
		lodCenter:    lodTileFromChunk(center),
		lodInner:     LodNearTileRadius(menuVistaRadiusChunks),
		lodOuter:     LodFarTileRadius(renderConfig.ViewDistance, renderConfig.LodFarMultiplier),
		cameraPos: mgl32.Vec3{
			float32(menuVistaAnchorX*core.SectionSize) + core.SectionSize/2 + 0.5,
			lift,
			float32(menuVistaAnchorZ*core.SectionSize) + core.SectionSize/2 + 0.5,
		},
		queue:        menuVistaChunkRing(center, menuVistaRadiusChunks),
		nextRevision: 1,
	}
	return vista, nil
}

// menuVistaChunkRing 返回以 center 为中心、半径 radius（切比雪夫）的区块
// 序列，按环距升序、环内按 X/Z 字典序排列：全景从相机脚下向外逐环出现，
// 序列本身是纯函数，同参数逐次一致。
func menuVistaChunkRing(center core.ChunkPos, radius int) []core.ChunkPos {
	chunks := make([]core.ChunkPos, 0, (2*radius+1)*(2*radius+1))
	for ring := 0; ring <= radius; ring++ {
		for dz := -ring; dz <= ring; dz++ {
			for dx := -ring; dx <= ring; dx++ {
				if max(max(-dx, dx), max(-dz, dz)) != ring {
					continue
				}
				chunks = append(chunks, core.ChunkPos{X: center.X + int32(dx), Z: center.Z + int32(dz)})
			}
		}
	}
	return chunks
}

// pump 推进一帧全景装配：按预算生成区块并装入镜像，调度网格化、冲刷上传，
// 并在首帧播种远环带。全部步骤与游戏世界同一套基建与预算语义，单帧工作量
// 有界（区块生成预算 + mesh 工作预算 + 每帧上传字节预算）。
func (v *menuVista) pump(workMax int) {
	// 每帧推进入口先重置上传预算：`UploadBudget` 的 spent 只由 `BeginFrame`
	// 清零，漏调会把「每帧 4MiB」退化成「整个装配期一次性总额」——当前固定
	// 种子的网格总量恰好低于预算才收敛，未来 worldgen 内容增长时全景将静默
	// 失去上传配额。与游戏世界调度器的每帧预算语义保持一致。
	v.scheduler.BeginFrame()
	for range menuVistaChunksPerFrame {
		if len(v.queue) == 0 {
			break
		}
		position := v.queue[0]
		v.queue = v.queue[1:]
		chunk := v.generator.GenerateChunk(position)
		// 区块 ID 全部来自 worldgen 生产路径，快照校验在此不可达；违约即
		// 编程错误，与远环生成同一 fail-fast 口径。
		snapshot, err := server.BuildChunkSnapshot(core.Overworld, chunk, v.nextRevision)
		if err != nil {
			panic("app: 全景区块快照编码失败（不可达）: " + err.Error())
		}
		v.nextRevision++
		update, err := v.mirror.Apply(snapshot)
		if err != nil {
			panic("app: 全景区块装入镜像失败（不可达）: " + err.Error())
		}
		v.mesher.MarkDirty(update.Dirty...)
	}
	v.mesher.Schedule(v.mirror, workMax)
	for _, result := range v.mesher.Drain(v.mirror, workMax) {
		if result.Dimension != core.Overworld {
			continue
		}
		v.scheduler.SetConnectivity(result.Pos, result.Conn)
		v.scheduler.QueueSection(result.Pos, result.Quads)
	}
	v.scheduler.FlushUploads(v.center)
	if !v.lodSeeded {
		v.lodSeeded = true
		v.lodScheduler.QueueRing(v.lodCenter, v.lodInner, v.lodOuter)
	}
	v.lodScheduler.BeginFrame()
	v.lodScheduler.FlushUploads(v.lodCenter)
}

// pending 返回尚未完成的全景装配工作量：待生成区块、mesher 全部队列、
// 待上传区段与远环在途 tile 之和。归零即全景完整就绪（capture 收敛判据）。
func (v *menuVista) pending() int {
	stats := v.mesher.Stats()
	return len(v.queue) +
		stats.DirtySections + stats.QueuedJobs + stats.InFlightJobs + stats.ReadyResults +
		v.scheduler.PendingUploads() +
		v.lodScheduler.Busy()
}

// pose 以当前 tick 计算本帧相机：位置与俯仰恒定，偏航来自自转脚本，投影
// 参数继承会话相机（FOV/纵横比随窗口与配置走）。
func (v *menuVista) pose(base client.Camera) client.Camera {
	return client.Camera{
		Pos:    v.cameraPos,
		Yaw:    float32(menuVistaYawAt(v.tick)),
		Pitch:  menuVistaPitch,
		FovY:   base.FovY,
		Aspect: base.Aspect,
		Near:   base.Near,
		Far:    base.Far,
	}
}

// release 释放全景占用的 GPU 段、远环 tile 与 worker：先经调度器把已上传
// 资源逐个下沉为渲染器释放调用，再关闭 mesher 与远环 worker。幂等（调用方
// 随即置空引用），Close 之后的重复调度调用为安全空操作。
func (v *menuVista) release() {
	// 中心区块列不会落入 DropOutside(radius=0) 的丢弃域，显式下沉为 drop。
	for y := int32(0); y < core.SectionsPerChunk; y++ {
		v.scheduler.QueueSection(core.SectionPos{X: v.center.X, Y: y, Z: v.center.Z}, nil)
	}
	v.scheduler.DropOutside(v.center, 0)
	// 外半径小于内半径退化为「全释放」（与游戏远环同一防御语义）。
	v.lodScheduler.DropOutside(v.lodCenter, 1, 0)
	v.mesher.Close()
	v.lodScheduler.Close()
}

// menuVistaForFrame 返回本帧应使用的全景（或 nil 表示渲染真实世界）：
// 只有主菜单与设置页相位参与；首次进入惰性构建，相位切换把自转 tick 归零，
// 保证每次进入菜单的第一帧姿态一致。装配失败只降级为暗色天空背景（记
// 警告日志），绝不阻塞菜单可用性。
func (a *Application) menuVistaForFrame() *menuVista {
	if a.menu.phase != MenuPhaseMenu && a.menu.phase != MenuPhaseSettings {
		return nil
	}
	if a.renderer == nil || a.registry == nil {
		return nil
	}
	if a.menuVista == nil {
		vista, err := newMenuVista(
			a.renderer, a.registry, a.render, a.startupOptions.FluidEnabled,
		)
		if err != nil {
			slog.Warn("构建菜单全景失败，背景退化为天空", "error", err)
			return nil
		}
		a.menuVista = vista
		a.menuVistaPhase = a.menu.phase
		return a.menuVista
	}
	if a.menuVistaPhase != a.menu.phase {
		// 主菜单 ⇄ 设置页切换视为重新进入：自转从头开始，两次进入同一
		// 相位的第一帧逐位一致。
		a.menuVistaPhase = a.menu.phase
		a.menuVista.tick = 0
	}
	return a.menuVista
}

// discardMenuVista 丢弃当前全景（世界装配成功或 Application 关闭时调用）：
// GPU 段、远环 tile 与 worker 全部释放，重进菜单相位时按同种子重建出相同
// 画面。幂等。
func (a *Application) discardMenuVista() {
	if a.menuVista == nil {
		return
	}
	a.menuVista.release()
	a.menuVista = nil
}

// SetMenuVistaTick 钉住全景自转时钟（capture 场景在收敛后固定相机时刻，
// 使最终帧姿态与机器速度解耦）；无全景时为空操作。
func (a *Application) SetMenuVistaTick(tick uint64) {
	if a.menuVista != nil {
		a.menuVista.tick = tick
	}
}

// MenuVistaPending 返回全景装配的未完成工作量，归零表示全景完整就绪；
// 无全景（游戏相位或尚未构建）恒为 0，capture 的收敛判据因此对非菜单
// 场景零影响。
func (a *Application) MenuVistaPending() int {
	if a.menuVista == nil {
		return 0
	}
	return a.menuVista.pending()
}
