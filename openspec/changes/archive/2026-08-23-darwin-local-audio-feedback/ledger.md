# darwin-local-audio-feedback 执行 ledger

| Task | Implementer | Commit | Spec review | Quality review | 验证证据 | Ruling |
|---|---|---|---|---|---|---|
| 1 OpenSpec 与配置 | `/root/audio_task1_impl` | `607bc61` + `b493b95` | PASS | PASS | config race + strict OpenSpec | 1 轮修复后通过 |
| 2 Darwin 音频后端 | `/root/audio_task2_impl` | `0a282df` + `b439efd` | PASS | PASS | audio race + archcheck | 1 轮修复后通过 |
| 3 客户端生命周期 | `/root/audio_task3_impl` | `3367d4d` | PASS | PASS | cmd focused race + archcheck | 通过 |
| 4 确认 cue 接线 | `/root/audio_task4_impl` | `75c6817` + `c40429e` + `1f2cae1` | FAIL | FAIL | cmd race + full vet/OpenSpec | 两轮评审证实无 sequence 成功边界无法满足绝对契约 |
| 4 fix 3 v26 成功确认 | `/root/audio_task4_impl_v3` | `dd3bc2f` + `44bb8fe`（重放后） | FAIL | FAIL | network/sim/server/cmd focused race + Memory/TCP parity + 四包组合 race + archcheck/vet/OpenSpec | v26 链路通过；round 3 复审发现累计 Task 4 有三条 P1 |
| 4 fix 4 状态与规格闭环 | `/root/audio_task4_impl_v3` | `4b97306` + `98d09f0` + `3f7e7cb` | PASS | PASS | focused audio race + 四包组合 race + archcheck/vet/gofmt/OpenSpec/diff/cmp | round 4 独立复审无 findings；旧目标失效、同 tick 双 cue、UI click delta spec 与 v26 链路均闭合 |
| 5 自动收尾与外部验收边界 | `/root/audio_task5_impl` | Task 5 证据提交 | 待整分支终审 | 待整分支终审 | Rust release/fmt/clippy/209 tests；focused/full Go race；vet/gofmt；59 项 OpenSpec；Linux/amd64 cross-compile 与专服依赖图；范围审计 | Darwin 字面 `GOOS=linux go test` 因 ELF `exec format error` 无法原生执行；PR Linux runner、四 cue/0.25/0 人工试听与 push/PR 均保持 pending |
| 整分支终审 fix 1 最长 PCM | `/root/audio_whole_fix1_impl` | 本轮修复提交 | 待复审 | 待复审 | `CueEatingComplete` 无设备 RED/GREEN + audio race 多轮 + Rust release + gofmt/diff | 仅同步测试 helper 容量至最长 cue；生产 AudioQueue buffer 与行为不变 |

## 五路归档收尾

- 已验证远端事实：PR #69 合入 `f039377ab2e6f8248d971610eb265242f1a226dc`（`Merge pull request #69 from channing771/codex/darwin-local-audio-feedback`）；最终 GitHub Actions [run `32641082086`](https://github.com/channing771/mornlea/actions/runs/32641082086) 的 8/8 logical job/child 均成功，整分支 SPEC/QUALITY/Remote 终审 PASS。PR 的 Linux runner 已原生执行 `GOOS=linux go test ./internal/audio` 与既有 `linux-server` bundle job，因此对应 Task 5 项据实完成。
- 真机扬声器的采掘、放置、进食、受伤四个 cue，以及 `audioVolume=0.25` 和 `audioVolume=0` 的试听仍无人工作站人工验收，故 tasks 保持未勾选；change 保持 active，等待控制会话归档。
