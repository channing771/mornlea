# rust-client-render-cutover Delta

## ADDED Requirements

### Requirement: client ABI v17 新增 viewmodel TLV 段

client C header、Rust 导出、Go bridge 与身份文档的 ABI 版本常数 MUST 同步为 17；无参数 identity export `mornlea_client_abi_version()` MUST 始终报告 17。v17 surface MUST 完整保留 v16 的全部 versioned exports 与语义，只叠加 frame viewmodel TLV 段（tag 取下一个空闲值，段载荷为定长 viewmodel 实例流）。全部 v17 versioned exports MUST 在其他 validation 或状态改变前拒绝包括 16 在内的每一个其他版本；版本错误 MUST 在 handle、pointer 或 renderer 状态改变前返回 `ABI_VERSION`。Go `ClientABIVersion()`、header 常量与三端一致性测试 MUST 同步为 17。

#### Scenario: v17 动态库拒绝 ABI v16

- GIVEN 集成 client ABI v17 的动态库及其全部接受 ABI version 参数的 exports
- WHEN 调用方对每个 export 传入 16
- THEN 每个调用 MUST 在其他 validation 或状态改变前返回 ABI_VERSION

#### Scenario: 三端 ABI 常量同步为 17

- GIVEN header 常量、Rust ffi 常量、Go bridge 常量与身份文档
- WHEN 执行三端一致性测试
- THEN 四处 MUST 同为 17，否则测试 MUST 失败

#### Scenario: 无 viewmodel 段的帧与 v16 逐字节一致

- GIVEN 相同的 RenderFrame 输入且 viewmodel 段为空
- WHEN 分别按 v16 与 v17 编码 frame
- THEN 除 layout 版本字段与 ABI 常量外，frame 字节 MUST 逐字节一致（既有 golden 的根基）

#### Scenario: 错误 ABI 优先于 viewmodel 内容检查

- GIVEN ABI version 不为 17 且 viewmodel 段内容也非法
- WHEN 调用任一接受 ABI version 参数的 client export
- THEN 调用返回 ABI_VERSION
- AND 不读取 viewmodel 输入或改变 renderer 状态
