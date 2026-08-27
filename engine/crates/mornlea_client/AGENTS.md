# Rust 图形客户端

## 所有权

- 本 crate 持有 Darwin 窗口、事件采集、GPU 资源与 pass、shader、egui 布局和 client ABI 出口。
- Darwin 本地提示音的 `AudioQueue` 实现在 Go `internal/audio`，不属于本 crate；本 crate 只呈现设置 UI 中的音量值。
- Linux 专用服务端不得依赖本 crate；非 Darwin workspace 构建保持空平台实现，不引入窗口或 GPU 运行时。

## Client ABI

- ABI 变化同批更新 `engine/include/mornlea_client.h`、本 crate FFI、`internal/client` bridge、版本和跨语言测试。
- FFI 先校验 handle、线程、pointer、length、layout 和输出容量，panic 转稳定状态码，失败不写部分结果。

## 渲染与无头路径

- GPU pass 顺序、资源池上限、instance/frame 布局和 overflow 都是渲染契约；修改时以代码和测试为真相，不在指南复制场景数或容量数字。
- 预热后热路径不得动态创建每帧资源。容量不足应显式失败或按既有整批语义跳过，不能截断成表面成功。
- offscreen capture/benchmark 不创建交互窗口、不聚焦或捕获光标；它们也不得间接触发 Go 侧音频设备。

## 定点验证与入口

- 测试：`cd engine && cargo test -p mornlea_client --locked`。
- 当前文档入口：`openspec/specs/rust-client-window/spec.md`、`openspec/specs/rust-client-render-cutover/spec.md`。
