# 客户端镜像与桥接

## 状态边界

- 服务端快照和变更是唯一权威输入；`Mirror`、背包、容器和实体状态只保存经验证的客户端镜像。
- 预测只改善本地响应，必须能被较新的权威状态纠正并重放未确认输入；不得用预测结果反向确认玩法结算。
- 不要在本包复制采掘、放置、伤害、掉落或库存结算等服务端玩法规则。客户端只做输入、预测、校验和呈现。

## 原生资源

- client C ABI 的 Go bridge 位于本包。header、Rust 导出、Go 布局和 ABI 版本必须成套更新，固定缓冲区不得绕过长度与 overflow 校验。
- 窗口、renderer、worker 和接收器都要明确关闭；构造中途失败时按已取得资源的逆序释放，重复关闭保持安全。
- Darwin `AudioQueue` 由 `packages/client/audio` 持有，应用层只把确认后的 cue 注入客户端装配；不要把音频设备生命周期混入镜像或预测状态。
- capture 与 benchmark 必须使用离屏 renderer，不创建交互窗口、不捕获光标，也不请求音频设备。
- HUD、背包、容器与配方栏位通过 `UIItemMetadataSource` 共用装配层缓存的名称与
  PNG data URI；`NewUIHudHotbar` 与 `NewUIGameSlot` 只读取元数据，不编码图像。
  单槽 `icon` 上界由导出常量 `UIIconMaxChars` 与桥 schema 同值，桥整体容量由
  `MaxUIEnvelopeBytes` 暴露给最坏载荷测试，不得为图标放宽。

## 定点验证与入口

- 测试：`go test ./packages/client/client -race -count=1`。
- 当前文档入口：`openspec/specs/rust-client-render-cutover/spec.md`。
