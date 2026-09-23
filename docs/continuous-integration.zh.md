---
doc_id: continuous-integration
doc_revision: 2026-09-23.5
language: zh-CN
counterpart: continuous-integration.md
---
# 持续集成

必需工作流是 [`.github/workflows/ci.yml`](../.github/workflows/ci.yml)，名称为 `Required CI`。它针对拉取请求及推送到 `main` 的事件验证实际检出的候选 SHA；新候选会取消旧候选尚未完成的运行。在仓库根目录执行 `make ci-preflight` 可在本地复现前期政策检查。这项命令在无需原生产物时检查工具依赖、格式、OpenSpec、Agent Hook 政策、注释语言、六模块包清单和不依赖原生产物的仓库审计。依赖原生库的 Godot 素材同步审计会在 Linux 产物验证后，随完整审计测试集于 `linux-quality` 中运行。缺少必需工具时，入口会失败。

## 必需层级

`preflight`、`frontend`、`rust-quality`、`native-linux` 和 `native-macos` 独立启动。两个原生构建分别产出 Linux 和 macOS 包，清单记录候选 SHA、平台，以及按顺序排列的文件路径、大小和 SHA-256 摘要。每个下游任务下载对应平台的包，并在测试前执行 `ci-verify-linux-artifact` 或 `ci-verify-macos-artifact`。缺失、不匹配或损坏的包会被拒绝；缓存不能证明产物身份，下游也不会在缺包时重新构建。

| 平台 | 验证原生产物后的任务 | 本地入口 |
| --- | --- | --- |
| Linux（`ubuntu-24.04`） | `linux-quality`、`race-server`、`race-rest`、`integration-server` | `make ci-linux-quality`、`make ci-race-server`、`make ci-race-rest`、`make ci-integration-server` |
| macOS（`macos-15`） | `race-client`、`integration-client` | `make ci-race-client`、`make ci-integration-client` |

包清单检查互不重叠的 `client`、`server` 和 `rest` race 分片是否覆盖 `go.work` 六个模块的全部包。macOS client 分片包含 `packages/tools/gfxspike`；server 和 rest 在 Linux 上运行。Linux quality 编译并 vet 其受支持源码集，macOS client 任务覆盖依赖 Darwin 的图形源码。执行完整仓库 audit 的两个 Linux 任务（`linux-quality` 与 `race-rest`）都会安装 ripgrep，并在包清单或测试运行前检查 audit 依赖配置。独立的服务端时序探针仍在 race 测试之外。

本地原生产出和下游入口均需使用同一个候选 `CI_CANDIDATE_SHA`。在各自支持的平台上执行 `make ci-native-linux CI_CANDIDATE_SHA=<sha>` 或 `make ci-native-macos CI_CANDIDATE_SHA=<sha>`；下游目标接收相同变量，并在检查前验证对应清单。可执行契约以 `Makefile` 和 `scripts/ci/` 为准。

`Required CI / merge-gate` 是唯一拟设为分支保护必需状态的结果。只有同一候选的所有必需前置任务成功，它才会成功；失败、跳过、取消或超时均不能授权合并。必需任务失败后，应修复并显式重跑；适用时可使用 GitHub 的失败任务重跑功能。必需校验不会自动重试失败命令，也不会把失败转换为成功。仓库分支保护设置是单独的上线步骤，工作流本身不会配置它。

初始反馈目标是五分钟内得到可处理的 preflight 结果，以及热 runner 上二十分钟的必需关键路径。任务耗时和 runner 身份会记录以便诊断。耗时与性能数值仅供参考；报告不完整、溢出、数据丢失、身份无效或 I/O 错误仍为失败。

## 可选 Godot 验证

[`Godot CI`](../.github/workflows/godot.yml) 针对相关路径运行，也支持手动触发。它不属于 `merge-gate`，但自身失败仍会显示为红色并使可选工作流失败。`godot-static` 在 Linux 上检查项目闭包。成功后，`godot-runtime` 在 macOS 26 arm64 和 Xcode 26.5 上运行：在冷 runner 上预取六个工作区模块的外部 Go 依赖，先构建原生引擎再生成依赖 cgo 的确定性素材，并获取通过校验和验证的 Godot 编辑器。随后它验证嵌入式 Python 运行时，先构建编辑器选用的 debug GDExtension，再构建 release GDExtension。release 校验先在无界面编辑器中打开项目，使全新的 `.godot` 缓存发现扩展，然后运行身份和 bridge-host 探针；直接校验 release 也必须已有 debug 库。Go client core 和离线导出的应用探针继续验证 release 分发产物。Python 工具检查和原有的 100 次无界面生命周期 smoke 也会运行。导出探针通过仓库的解析入口使用已获取的编辑器。只有另行批准的切换变更才能将此工作流纳入合并权限。
