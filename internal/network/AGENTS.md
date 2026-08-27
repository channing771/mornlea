# 协议与传输

## 信任边界

- 所有线上 packet 在编码前和解码后都要经过 state、类型、枚举、数值及长度上限校验；未知 ID、尾随字节和非规范编码必须拒绝。
- 发送成功后的消息及其 slice 视为不可变。需要继续修改时先复制，不得依赖 Memory transport 恰好传递 Go 对象引用。

## 传输一致性

- Memory 与 TCP 必须受同一 packet/codec 契约约束，并复用登录和上层模拟路径；不得为本地连接建立绕过校验、登录或权威模拟的特权路径。
- 根包 `internal/network` 持有 packet、codec、登录状态机、共享 stream 接口和 Memory transport；`internal/network/tcp` 持有 TCP listener、dial、stream 实现，且仍只负责 transport。两者都不决定业务结果，登录状态转换集中在既有 `Login*` 路径。
- TCP 面向可信局域网，现有协议没有认证或加密。不要把它描述为公网安全，也不要用这一既有边界作为降低输入校验的理由。

## 协议演进

升级协议时同步检查 packet 定义、registry ID、双向 codec、Validate、登录版本拒绝、Memory/TCP 一致性、fuzz 与 golden。保留既有 ID 和布局的兼容结论必须由测试证明，不能只改版本常量。

## 定点验证与入口

- 测试：`go test ./internal/network/... -race -count=1`。
- 当前文档入口：`docs/notes/lan-server.md`。
