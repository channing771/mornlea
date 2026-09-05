# drop-scatter-fixed-size Ledger

基线 SHA：`6d4774ef`（分支 `codex/cream-experience`，干净起步）。

## Task 1：全尺寸散列 TDD（子代理开发轮）

- Red：先改测试（四档期望改 1）见红——4 堆 scale=0.75298804，想要 1。
- 实现：`packages/client/render/drop_scatter.go` 删除密度缩放（scale 恒 1，`bob` 取全幅，层距按全尺寸），删 `dropScatterRadius/DiameterFill`；测试更新四档期望/越界分档/中心距离下界。
- 冲突与裁决：32 堆 6×6 档边角中心 0.875 + 薄片半宽 0.25 + 最大抖动 0.006 = 探出下界 0.119，原定 0.1 上界不可行——change owner 裁决放宽至 0.14（推导上界 0.131 加裕量），已同步回 proposal/design/delta spec。
- Green：`go test ./packages/client/render -race -count=1` ok；`go test ./packages/audit -count=1` ok。

## Task 2：基线与门禁（子代理执行轮）

- 重出 `testdata/visual-golden/motion/drop-scatter.gif`、`drop-density.gif`（`--motion-scene`，80/160 帧，EXIT 0）；目检：4 堆块内全尺寸可辨、32 堆双层高桩轻微交叠件件可辨；`avatar-walk.gif`/`break-burst.gif`/`avatar-detail.png` 经无关性验证未动；motion/ 不进比对，无测试 golden 需同步。
- `gofmt` 干净；`make test-race` EXIT 0；`make dev-check` EXIT 0；`openspec validate --all --strict --no-interactive` 97/97。

## 独立审查（子代理审查轮）

- SPEC pass / QUALITY pass：scale 恒 1 与 delta 一一对应，骨架保留，0.131 推导成立，cream 原 MUST 修改自洽，旧常量零残留，count=1 新旧等价。
- 3 个非阻塞 polish 已收：design/tasks 的残留“0.1”字样同步为 0.14；删死 helper `scatterAxisGap`；补中心距/层抬升最坏算式注释； focused race 重绿。

## Ruling

Task 1/2 可关闭；不推送、不合并。
