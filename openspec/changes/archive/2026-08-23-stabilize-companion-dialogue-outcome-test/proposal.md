## Why

三个 `companion dialogue` outcome 测试在 CI 偶发失败，现有固定 tick 假设把 HTTP worker 的异步完成误当成同步边界，无法稳定验证既有过时结果语义。

## What Changes

- 仅稳定三项测试对台词 outcome 到达的等待边界。
- 复用既有测试同步模式，不改变产品路径或已有其他 `releaseRequests()` 用途。

## Non-Goals

- 不改变服务端、协议、存档 schema、ABI、benchmark/capture、台词模型行为或生产并发语义。

## Impact

- 仅影响 `internal/server` 的测试与测试辅助同步；不需要 delta spec，见 `.openspec.yaml` 的 `skip_specs: true`。
