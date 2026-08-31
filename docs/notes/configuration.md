# 配置文件与调试面板

`mornlea` 与 `mornlea-server` 启动时读取同一份 JSON 配置文件，本文承接 README「配置文件与调试面板」一节，汇总配置的加载与校验行为、设置页三项客户端设置、运行参数分组，以及 `--dev` 调试面板的交互语义。

## 配置文件加载

默认路径为 `os.UserConfigDir()/mornlea/config.json`（与 `profile.json` 同目录），可用 `--config <path>` 覆盖。文件不存在时全部使用编译默认值，**不会自动创建文件**；字段缺失取默认值；`physics`/`sim`/`render` 的数值调参越界时按该字段合法区间钳制并 `slog.Warn`；未知字段（顶层、分组内与伙伴条目内）告警后忽略。

以下情形会导致加载失败：JSON 语法错误；不认识的 `version`；`audioVolume` 非数值或超出 `0..1`；`windowSize` 类型错误或不在三个预设内；`texturePackPath` 非字符串、超过 1024 个 UTF-8 字节或包含 CR/LF；`fluidEnabled` 类型错误；`physics`/`sim`/`render` 字段不是合法数值。`logging` 的未知等级与 `render` 的 LOD 非法值按告警回落默认值，不使加载失败。

显式 `--config` 只读取指定路径；只有默认路径的新文件缺失时才会从旧 `minecraft-go` 目录迁移配置，迁移规则见[改名迁移说明](mornlea-migration.md)。本地材质覆盖的配置与目录格式见[材质包说明](../texture-packs.md)。

## 客户端设置

普通本地客户端从 egui 主菜单进入「设置」页；进入设置页时世界存储、内置权威服务端与登录均不装配，设置页期间的游戏输入不生效。设置页只提供三项：总音量、材质包目录和窗口大小。窗口大小是固定的 16:9 预设，只能选择 `640x360`、`960x540` 或 `1280x720`；`windowSize` 缺失时默认使用 `1280x720`。对应的最小配置示例如下：

```json
{
  "version": 1,
  "audioVolume": 0.7,
  "texturePackPath": "packs/my-pack",
  "windowSize": "1280x720"
}
```

`audioVolume` 的范围是 `0..1`，设置页以 `0..100%` 滑块呈现。设置页显示并保存 `texturePackPath` 原文；该值必须是单行、最多 1024 个 UTF-8 字节，相对路径按配置文件所在目录解析，空串表示内嵌默认材质。保存成功后，音量和窗口大小立即应用于当前进程；发生变化且非空的材质包候选会在写入前完整校验，但当前 atlas 不会热替换，保存的材质包从下次启动起加载。rename 提交点之前的保存失败会保留页面草稿、旧配置与旧运行态；rename 之后若父目录持久性同步异常，设置页会明确提示“已保存但持久性同步异常”，并按磁盘新值应用音量和窗口。保存只覆盖这三项，配置中的其余字段保持磁盘原值；显式点击「保存」时即使三项都是默认值也会创建配置文件。

## 运行参数分组

除三项客户端设置外，配置还包含四组运行参数、一个可选 AI 组和顶层 `fluidEnabled`：

| 分组 | 内容 |
| --- | --- |
| `logging` | 全局日志等级 `default` 与按模块覆盖的 `modules`（键为包路径末段，如 `render`、`storage`），等级为 `debug`/`info`/`warn`/`error` |
| `physics` | 重力、行走/跳跃速度、加减速度、终端下落速度、视线高度、水中运动参数与疾跑速度倍率等运动常量 |
| `sim` | 交互距离、生命回复与溺水/饥饿结算间隔、掉落物寿命与拾取延迟、出生半径、熔炉冶炼/燃烧 tick、流体推进预算、随机 tick 与作物生长、进食时长等权威模拟参数 |
| `render` | `viewDistance`（重启生效，仅配置文件可改）、`fovDegrees`、`mouseSensitivity`，以及远环 LOD 的 `lodEnabled`/`lodFarMultiplier`/`lodStep` |
| `ai` | loopback `agentService.endpoint/apiKeyEnv`、任务超时，以及 `0..4` 个带 canonical UUIDv4 `id`、唯一 `name` 和可选 `persona` 的伙伴定义；provider URL/model/key 只属于独立 Python 服务；列表缺失或为空时 AI 关闭 |

`fluidEnabled` 默认 `true`，控制新生成世界是否注水；benchmark 固定不注水，capture 固定使用编译默认值，两条自动化路径都不随本机配置漂移。

**`mouseSensitivity` 是无量纲倍率**，默认 `1`，区间 `[0.1, 5]`；实际弧度/像素系数是代码内基线常量 `baseMouseSensitivity = 0.002`（`cmd/mornlea/interactive.go`），运行时灵敏度 = 该基线 × 配置里的倍率。

## 伙伴 Agent 双进程配置

启用伙伴时，Go 配置只保存 Agent 服务地址和 HTTP credential 的环境变量名；不再接受 Go direct-model 的 `ai.endpoint`、`ai.model`、`ai.apiKeyEnv` 生产语义。非空伙伴列表出现这些旧字段会拒绝启动并提示迁移；伙伴列表为空时旧字段只告警后忽略。`ai.agentService.endpoint` 必须是 loopback IP 字面量的 `http` URL，不接受 `localhost`、其他 hostname、userinfo、query、fragment、远程地址或 redirect。`apiKeyEnv` 指向的值必须非空，且只作为 Go→Agent Bearer credential：

```json
{
  "version": 1,
  "ai": {
    "agentService": {
      "endpoint": "http://127.0.0.1:8080",
      "apiKeyEnv": "MORNLEA_AGENT_TOKEN"
    },
    "taskTimeoutMinutes": 10,
    "companions": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "name": "阿木",
        "persona": "沉稳、简短地回答。"
      }
    ]
  }
}
```

`ai` 缺失、为 `null`，或 `companions` 缺失/为空时不要求 endpoint、credential 或超时。Go 仍会先做 metadata-only existence probe：没有 `companions.ai` 时不 Load/Save/create；已有文件时会加载并把原 active 记录同步退休为 v5 tombstone，随后保持 Agent/MCP 关闭。

Python 服务读取另一份 strict v1 YAML，未知字段、重复 key、多 worker、非持久 SQLite 路径或非法 limit 都会拒绝启动。相对 `sqlite_path` 按该 YAML 所在目录解析；`workers` 固定为 1。`model_calls`、`tool_calls`、`timeout_seconds` 默认分别为 3、4、30，硬上限分别为 5、8、60：

```yaml
config_version: v1
http:
  bind: 127.0.0.1
  port: 8080
  workers: 1
  bearer_token_env: MORNLEA_AGENT_TOKEN
storage:
  sqlite_path: companion-agent.sqlite3
provider:
  base_url: http://127.0.0.1:11434/v1
  model: local-model
  api_key_env: MORNLEA_PROVIDER_KEY
limits:
  model_calls: 3
  tool_calls: 4
  timeout_seconds: 30
```

环境变量只保存 secret 值，配置文件只写变量名；以下占位符必须替换，不能提交真实 secret：

```bash
export MORNLEA_AGENT_TOKEN='replace-with-random-local-token'
export MORNLEA_PROVIDER_KEY='replace-with-provider-key'
cd services/companion-agent
uv sync --locked
uv run mornlea-companion-agent serve --config /absolute/path/to/agent.yaml
```

先启动 Python 单 worker 并确认带认证的 `/readyz` 成功，再启动内置或专用 Go 服务端；两边的 `MORNLEA_AGENT_TOKEN` 值必须一致。Go 不会自动拉起或监护 Python。Agent HTTP 与 Go MCP 都只监听 loopback，不支持远程 MCP、反向代理或 LAN 暴露。`/livez` 只表示进程存活；`/readyz` 失败通常表示配置、provider adapter 或 SQLite 未就绪。Agent、MCP 或 provider 故障不会停止世界 tick：Planner 稳定失败为 `PlannerUnavailable`，Dialogue 跳过；终态 memory 未确认时不广播模型台词。

## 调试面板

`--dev` 只控制游戏内调试面板是否可用，**不控制配置文件是否生效**：配置文件里调过的值无论是否加 `--dev` 都会生效；不加 `--dev` 时只是看不到、也改不了面板。

加 `--dev` 后按 `F3` 切换面板显隐。面板由 Rust `mornlea_client` 的 egui 绘制（`engine/crates/mornlea_client/src/ui.rs`），顶部是只读读数区（帧时、坐标、朝向、权威 tick、世界时刻、已加载区块数与连接模式），参数行按 `physics`/`sim`/`render` 分组展示：每组一个段头行（如 `── physics ──`），数据行使用裸字段名（如 `gravity`，而非 `physics.gravity`）。

参数行交互：方向键在可编辑行之间移动选中，自动跳过只读行；`Enter` 进入文本编辑，再次 `Enter` 确认写回，`Esc` 取消编辑并恢复原值；非法新值被拒绝并保持原值，写回值按字段合法区间钳制。非编辑态按 `Esc` 关闭面板。面板可见期间游戏键盘输入被整体捕获，不产生游戏上行。面板改动只作用于当前进程，不写回配置文件，也不产生任何网络消息。

联机（`--connect`）时 `physics`/`sim` 两组灰显只读（服务端是唯一权威），`render` 组仍可编辑；`viewDistance` 无论是否联机都只读，只能通过配置文件调整并重启生效。

普通本地模式的内置服务端和 `mornlea-server` 都消费同一配置中的 `logging`、`physics`、`sim`、`fluidEnabled` 与可选 `ai`；专用服务端不消费 `render`、`audioVolume`、`texturePackPath` 或 `windowSize`。`--connect` 客户端不会按本机 `ai` 配置创建伙伴，只呈现远端服务端通过协议发布的伙伴。

**联机时本机配置文件里的 `physics`/`sim` 必须与服务端所用的一致**，否则客户端预测会与权威模拟持续分歧（位置回弹）。面板在联机时锁住这两组，但配置文件不受该锁约束——它始终生效。局域网下让 `mornlea` 与 `mornlea-server` 读同一份配置文件即可满足这条要求；`mornlea` 检测到“`--connect` + 这两组偏离默认值”时会打印一条 `slog.Warn` 提醒。
