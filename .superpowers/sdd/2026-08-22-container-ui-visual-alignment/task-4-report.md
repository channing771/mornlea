# Task 4 report

- RED：目标 capture 测试因 15 项表、缺少两个容器场景而失败。
- GREEN：`b98ba7145f65b82acd6ceb0e9535827e79384ded` 注册 17 项、两套确认镜像及两张新 golden。
- Visual update：`make visual-update VISUAL_OUT=build/visual-container-ui-update`；正式 golden 为 17 张，diagnostic control 两张不计数。
- Visual check：`make visual-check VISUAL_OUT=build/visual-container-ui-check` 通过并输出 17 张正式图。
- 人工审查：逐张审查 17 图；三容器的标题/凹槽/来源、36/39/63 格、10 配方与火焰/箭头清晰，末三水/LOD 场景无漂移。
- 验证：聚焦、HUD race、archcheck、vet、strict OpenSpec、gofmt、diff check 通过。
- 风险：完整 `go test ./cmd/mornlea -race -count=1` 与遗留同命令并行时长时间未结束，已终止本次 PID，未声称通过；需 reviewer/后续重跑。
