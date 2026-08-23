# Darwin 本地音效反馈实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Darwin 图形客户端增加四个原创、固定、代码生成的本地 PCM 提示音：有效 UI 操作、权威采掘完成、权威进食完成和权威受伤；支持 `audioVolume` 总音量与 0 静音。

**Architecture:** 新包 `internal/audio` 用纯 Go 整数算法一次生成四段 PCM，并用 macOS AudioToolbox `AudioQueue` 的小型 C bridge 管理一个长生命周期输出队列和 8 个预分配 buffer。`cmd/mornlea` 只保存一对播放/关闭函数和有界确认状态；放置 cue 只消费协议 v26 新增的 session-local `PlaceBlockSucceeded` 序号应答，其余触发来源是已确认消息或有效 UI 行为；capture/benchmark/Linux/server 不创建音频设备。

**Tech Stack:** Go 1.26、cgo、macOS AudioToolbox/AudioQueue、`internal/config` schema v1、现有 Darwin application lifecycle、OpenSpec。

**Spec:** `docs/superpowers/specs/2026-08-22-five-way-parallel-wave-design.md` §6、§3、§9。

## Global Constraints

- [ ] 从五路共享 main 基线创建 `codex/darwin-local-audio-feedback` 独立 worktree/change；并行的玩家近战已占用协议 v25，本分支必须堆叠在其上以 v26 交付。除最小的 network/sim/server 放置成功确认链外，产品文件不得触及 HUD、CI 或伙伴台词测试领地。新包只额外登记 `internal/archcheck/dependency_test.go` 的空内部依赖。
- [ ] 不提交音频二进制，不新增第三方依赖、存档字段、ABI/benchmark 版本或配置 schema 版本；协议 v26 只新增 Play S→C ID 20 `PlaceBlockSucceeded{Sequence u64}`，ID 21 保持未分配。`audioVolume` 是 schema v1 的可选顶层字段，缺失默认 0.7，范围 0..1，0 即静音。
- [ ] 稳定测试不打开真实设备；通过纯 PCM 测试和注入 `playCue func(audio.Cue)` 验证触发。只有最终人工验收可启动交互式客户端。
- [ ] 设备创建/播放失败只告警一次并让当前 player 永久静音；队列忙时丢弃该次 cue，不视为设备失败；不得阻塞权威 tick、网络接收或渲染。
- [ ] 每任务全新 implementer、独立 SPEC/QUALITY reviewer、最多 5 轮追加修复，证据入 ledger。

---

## Task 1: 建立 OpenSpec 和配置契约

**Files:**
- Create: `openspec/changes/darwin-local-audio-feedback/.openspec.yaml`
- Create: `openspec/changes/darwin-local-audio-feedback/proposal.md`
- Create: `openspec/changes/darwin-local-audio-feedback/design.md`
- Create: `openspec/changes/darwin-local-audio-feedback/tasks.md`
- Create: `openspec/changes/darwin-local-audio-feedback/ledger.md`
- Create: `openspec/changes/darwin-local-audio-feedback/specs/local-audio-feedback/spec.md`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] spec 覆盖四种 cue 的确认边界、无声路径、总音量、设备失败降级和无头/非 Darwin 零初始化；design 固定 AudioQueue、8 buffer、22050 Hz mono int16、无每次播放分配。
- [ ] 先写配置红测：缺失为 0.7；0、0.25、1 原样加载并 Save/Load；-0.01、1.01、`null`、字符串全部硬错误；未知字段告警不把 `audioVolume` 当未知；`CurrentVersion==1`。
- [ ] 运行红测：

```bash
make rust
go test ./internal/config -run 'TestAudioVolume' -count=1
```

- [ ] 在 `Config` 加 `AudioVolume float32`（JSON key 为 `audioVolume`），`Defaults` 设 0.7；在 `decodeConfig` 用 `*float32` 解码以拒绝 `null`，显式校验闭区间；加入 `warnUnknownTopLevel` 白名单，不进入数值 `Fields()` 或调试面板。
- [ ] 实现形状：

```go
if raw, ok := lookupCaseInsensitive(top, "audioVolume"); ok {
	var volume *float32
	if err := json.Unmarshal(raw, &volume); err != nil || volume == nil {
		return Config{}, errors.New("config: 解析 audioVolume 字段: 必须是 0..1 数值")
	}
	if *volume < 0 || *volume > 1 {
		return Config{}, fmt.Errorf("config: audioVolume 超出 0..1: %v", *volume)
	}
	cfg.AudioVolume = *volume
}
```

- [ ] 格式化、验证 OpenSpec 并提交：

```bash
gofmt -w internal/config/config.go internal/config/config_test.go
go test ./internal/config -race -count=1
openspec validate darwin-local-audio-feedback --strict --no-interactive
git diff --check
git add openspec/changes/darwin-local-audio-feedback internal/config
git commit -m "feat: add local audio volume config"
```

- [ ] 完成 task 双裁决与 ledger。

## Task 2: 生成确定 PCM 并封装一个 AudioQueue player

**Files:**
- Create: `internal/audio/cue.go`
- Create: `internal/audio/cue_test.go`
- Create: `internal/audio/player_darwin.go`
- Create: `internal/audio/player_darwin.c`
- Create: `internal/audio/player_darwin_test.go`
- Modify: `internal/archcheck/dependency_test.go`

- [ ] 先在 `cue_test.go` 写红测，锁定 `CueUIClick`、`CueMiningComplete`、`CueEatingComplete`、`CueDamage` 四枚稳定枚举；PCM 均为 22050 Hz mono int16、非空、长度固定、样本非全零、峰值不溢出，并把 little-endian 样本字节的 SHA-256 固定为：click `6abfada26f17733f85a1da11648c8471a7609b96e1df428d4776d77640ebe011`、mining `0fd329cb57015968b435da587887563713d4795a4ad33545ee051e46a8134d25`、eating `f1a63c03b7bd7668aa2035787e7904d41b3347c93fb3f848b36d368e9ca83005`、damage `2e844e35047ef374ae72f40e1858fa7a86abbc832ec17cf05071665ab8fbd213`。
- [ ] 固定四段长度和整数参数：click 772 samples/1200→900 Hz，mining 2646/180→90 Hz，eating 3087/440→660 Hz，damage 2205/95→60 Hz；幅度分别 7000/10000/8000/12000。
- [ ] 运行红测：

```bash
go test ./internal/audio -run 'TestCuePCM' -count=1
```

- [ ] `cue.go` 提供中文 package comment 和全部导出项 GoDoc；用一个整数 phase accumulator 和线性衰减 envelope 生成所有 cue，不调用 `math.Sin`：

```go
func synthesize(spec cueSpec) []int16 {
	pcm := make([]int16, spec.samples)
	var phase uint32
	for i := range pcm {
		frequency := spec.startHz + (spec.endHz-spec.startHz)*i/max(1, spec.samples-1)
		phase += uint32(frequency * 65536 / sampleRate)
		sign := int32(-1)
		if phase&0x8000 != 0 {
			sign = 1
		}
		envelope := int32(spec.samples - i)
		pcm[i] = int16(sign * int32(spec.amplitude) * envelope / int32(spec.samples))
	}
	return pcm
}
```

- [ ] `player_darwin.c` 用 `AudioQueueNewOutput` 建一个队列，创建时以 `AudioQueueSetParameter(kAudioQueueParam_Volume, volume)` 设置一次总音量，并用 `AudioQueueAllocateBuffer` 预分配 8 个最大 cue 长度的 buffer；callback 用 `pthread_mutex_t` 把完成 buffer 标为空闲。`mornlea_audio_play` 只找空 buffer、copy、enqueue，首次成功 enqueue 后 start；全部忙返回 BUSY，OSStatus 错误返回 FAILURE。
- [ ] `player_darwin.go` 用 `#cgo LDFLAGS: -framework AudioToolbox -framework CoreFoundation`；`NewPlayer(volume)` 一次合成四段 PCM并创建 queue，0 音量直接静音；`Play` 不分配，BUSY 丢弃，FAILURE 用 `sync.Once` 告警并禁用；`Close` 幂等释放。
- [ ] 在 archcheck 白名单增加 `"internal/audio": {}`；该包不得直接依赖其他 internal 包。
- [ ] 不创建单实现 interface/factory；公开 API 只有：

```go
type Cue uint8

const (
	CueUIClick Cue = iota
	CueMiningComplete
	CueEatingComplete
	CueDamage
)

func NewPlayer(volume float32) *Player
func (player *Player) Play(cue Cue)
func (player *Player) Close()
```

- [ ] 测试不调用 `NewPlayer`：`player_darwin_test.go` 只测试非法 cue no-op 和不需要设备的状态 helper；C queue 由最终人工验收覆盖。
- [ ] 格式化、复绿并提交：

```bash
gofmt -w internal/audio/cue.go internal/audio/cue_test.go internal/audio/player_darwin.go internal/audio/player_darwin_test.go
go test ./internal/audio -race -count=1
go vet ./internal/audio
go test ./internal/archcheck -count=1
git diff --check
git add internal/audio internal/archcheck/dependency_test.go
git commit -m "feat: add bounded Darwin audio player"
```

- [ ] 完成 task 双裁决与 ledger。

## Task 3: 接线客户端生命周期且无头路径零初始化

**Files:**
- Modify: `cmd/mornlea/app.go`
- Modify: `cmd/mornlea/app_dependencies.go`
- Modify: `cmd/mornlea/app_startup.go`
- Modify: `cmd/mornlea/app_lifecycle.go`
- Modify: `cmd/mornlea/main.go`
- Modify: `cmd/mornlea/run_test.go`
- Modify: `cmd/mornlea/app_startup_test.go`

- [ ] 先写红测：`runWithDependencies` 把有效配置音量下传；普通窗口模式恰调用一次 audio constructor；benchmark 和 capture 调用 0 次；`Close` 幂等关闭 player。
- [ ] 在 `applicationOptions` 加 `AudioVolume float32`；`main.go` 从 `effective.AudioVolume` 赋值。
- [ ] 在 `applicationDependencies` 加一个返回函数对的依赖，而不是 interface 或测试专用 concrete hook：

```go
newAudioPlayer func(float32) (play func(audio.Cue), close func())
```

生产 defaults 在闭包内只调用一次 `audio.NewPlayer`，返回 `player.Play`/`player.Close`；测试可直接返回 recorder 与关闭计数函数。依赖缺失代表禁用，只有 `headless == false` 且函数非 nil 时创建。

- [ ] `application` 只保存 `playCue func(audio.Cue)` 与 `closeAudio func()`；`releaseOwnedResources` 在 window/renderer 前调用一次 `closeAudio`。不得暴露 `audio.Player` 内部状态来迁就测试。
- [ ] 不在 `cmd/mornlea-server`、capture control app、benchmark 或 Linux build 中引用/初始化设备。
- [ ] 格式化、复绿并提交：

```bash
gofmt -w cmd/mornlea/app.go cmd/mornlea/app_dependencies.go cmd/mornlea/app_startup.go cmd/mornlea/app_lifecycle.go cmd/mornlea/main.go cmd/mornlea/run_test.go cmd/mornlea/app_startup_test.go
go test ./cmd/mornlea -run 'Test.*(Audio|Capture|Benchmark|ApplicationClose)' -race -count=1
go test ./internal/archcheck -count=1
git diff --check
git commit -am "feat: wire audio into interactive Darwin client"
```

- [ ] 完成 task 双裁决与 ledger。

## Task 4: 只从有效 UI 与权威确认触发四种 cue

**Files:**
- Create: `cmd/mornlea/app_audio.go`
- Create: `cmd/mornlea/app_audio_test.go`
- Modify: `cmd/mornlea/app_messages.go`
- Modify: `cmd/mornlea/app_input.go`
- Modify: `cmd/mornlea/app_lifecycle.go`

- [ ] 先用 `application{playCue: recorder}` 写 UI 红测：可合成按钮发送成功、首次有效来源选择、同格取消选择、合法第二次移动发送成功各发一个 click；空白、未确认、不可合成按钮、熔炉输出作目标、send 失败均无声。
- [ ] 写 damage 红测：首个 `PlayerState`、同血、治疗、respawn/reset 不响；后续权威 health 下降恰响一次。
- [ ] 写 mining 红测：active `MiningTarget` 已记录后，镜像成功应用的 `BlockChanges` 把该格改为 `core.AirID` 时响一次；其他格、未确认目标、非法 delta、reset 后无声。
- [ ] 写 eating 两种消息顺序红测：`InventoryState` 先/`PlayerState` 先都只有在“选中食物恰减 1 + hunger 增加”成对时响；只有一半、换选中格、非食物减少、reset、会话关闭均无声。
- [ ] 运行红测：

```bash
go test ./cmd/mornlea -run 'TestLocalAudio|TestAudioCue' -count=1
```

- [ ] 在 `app_audio.go` 实现值类型 `localAudioFeedback`，保存 health 基线、最近 active mining target、上一份选中 stack 和两个进食半匹配位；不启动 goroutine、不计 wall clock。
- [ ] 进食 matcher 使用有界窗口：食物减少最多等下一条新 `PlayerState`；hunger 增加最多等下一条 `InventoryState`；任何不匹配的新对应状态、selected 改变、reset 或 close 都清两位。配对后立即播放并清空。
- [ ] `app_messages.go` 只在消息已通过 Validate/镜像 Apply，且 `PlayerState.ServerTick` 新于当前 tick 后调用 matcher。`BlockChanges` 在 `mirror.Apply` 成功后检查。
- [ ] `clickInventorySlot` 只在实际有效动作之后调用 `a.playLocalCue(audio.CueUIClick)`；不要在命中测试前统一播放。
- [ ] `closeClientSession` 与权威 Reset 都调用 `audioFeedback.Reset()`。
- [ ] 格式化、复绿并提交：

```bash
gofmt -w cmd/mornlea/app_audio.go cmd/mornlea/app_audio_test.go cmd/mornlea/app_messages.go cmd/mornlea/app_input.go cmd/mornlea/app_lifecycle.go
go test ./cmd/mornlea -run 'Test(LocalAudio|AudioCue|Inventory|Furnace|Chest|Mining|Damage)' -race -count=1
go test ./cmd/mornlea -race -count=1
git diff --check
git add cmd/mornlea/app_audio.go cmd/mornlea/app_audio_test.go \
  cmd/mornlea/app_messages.go cmd/mornlea/app_input.go cmd/mornlea/app_lifecycle.go
git commit -m "feat: play cues from confirmed client events"
```

- [ ] 完成 task 双裁决与 ledger。

### Task 4 fix round 3: v26 权威放置成功确认

- [ ] 先写 network codec/registry/golden/fuzz/非法截断载荷红测，钉死 Play S→C ID 20 `PlaceBlockSucceeded{Sequence uint64}` 的 8-byte 小端载荷及 ID 21 未分配；协议升 v26 并拒绝 v25 及更早握手。
- [ ] 先写 sim 红测：每个成功 `CommandPlaceBlock` 产生一个 `(Session, Sequence)` 成功结果，拒绝不产生；同 tick 与跨 tick 连续两个同 slot/item 放置均保留各自序号。
- [ ] 先写 server Memory/TCP 红测：成功应答只发给发起会话，每次成功各一次，失败只发既有 `CommandRejected`，Memory/TCP wire 一致。
- [ ] 先写客户端红测：首次新 sequence 播放一次，重复/旧 sequence、世界增量与库存拼接、拒绝、其他玩家放置全部无声；reset 后新会话可从低 sequence 重新开始。
- [ ] 最小实现：`TickResult` 新增有界 placement success slice，仅权威放置原子成功后追加；server 沿用 local result/outbox 关闭规则发送；客户端删除 delta+inventory pending matcher，仅以 reset 清空的最高已消费 sequence 去重。不新增队列、map、timeout、retry 或通用 command-success 抽象。
- [ ] 同步 AGENTS.md/CLAUDE.md 协议 v26 基线，保持两文件逐字节相同；存档、engine/client ABI 与 benchmark scenario 不变。

### Task 4 fix round 4: 状态失效、同 tick 双 cue 与规格闭环

- [ ] 先把完整音频栈从旧 melee base `56e5d6c` 安全重放到 PR #66 修复 HEAD `82eb03b`，以 `git range-diff` 证明全部音频提交内容等价；不 force-push。
- [ ] 先写采掘 RED：active 目标后收到新鲜 inactive 状态，再由无关增量移除旧目标必须无声；拒绝后的 inactive 路径同样无声；服务端成功顺序“目标增量先、inactive 状态后”仍只播放一次。
- [ ] 先写进食+伤害 RED：`InventoryState` 先确认食物恰减一件，下一条 `PlayerState` 同时 hunger 上升且 health 下降时，两种 cue 各播放一次；使用两个独立布尔位或等价定长、零分配结果，不使用 slice、队列或通用事件总线。
- [ ] 在 active delta spec 明确有效本地 `CueUIClick`：可合成按钮发送成功、首次有效来源选择、同格取消和合法移动发送成功各响一次；空白、未确认/禁用、不可合成、熔炉输出作目标与发送失败无声。保留全部既有 UI 行为。
- [ ] 保留 v26 placement 成功应答、reset/close、采掘/进食/伤害与 UI 的既有回归；运行 focused RED/GREEN、四包组合 race、archcheck、vet、gofmt、59 项严格 OpenSpec、diff/cmp，记录独立评审证据。

## Task 5: 全量验证、人工试听和终审

**Files:**
- Modify: `openspec/changes/darwin-local-audio-feedback/tasks.md`
- Modify: `openspec/changes/darwin-local-audio-feedback/ledger.md`

- [ ] 运行自动门禁：

```bash
make rust
make rust-check
go test ./internal/audio ./internal/config ./cmd/mornlea -race -count=1
go test ./internal/archcheck -count=1
go test ./... -race
go vet ./...
gofmt -l .
openspec validate --all --strict --no-interactive
git diff --check
```

- [ ] 在非 Darwin 验证纯 Go 音频定义不要求 AudioToolbox：`GOOS=linux go test ./internal/audio`。Linux 专服 bundle 由 PR 的现有 `linux-server` CI job 运行 `make build-linux-server`；确认其依赖闭包不含 `internal/audio`。
- [ ] 由用户/验收者显式启动交互客户端，分别试听四个 cue、`audioVolume=0.25` 和 `audioVolume=0`；记录设备、结果和“capture/benchmark 未请求设备”的证据。自动代理不得自行聚焦窗口。
- [ ] 确认无音频资产、新依赖、存档/schema/ABI/scenario 变化；协议唯一变化是 v26 Play S→C ID 20 的 8-byte 放置成功应答；`gofmt -l .` 无输出。
- [ ] 提交 ledger/tasks，生成 committed review package 与 SHA-256，交给全新 reviewer 做整分支 SPEC/QUALITY 终审，修复不超过 5 轮。
- [ ] 正常 push 并创建独立 PR；不自行归档。
