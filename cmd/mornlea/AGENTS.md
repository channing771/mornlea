# 图形客户端装配

## 应用边界

- 本目录只装配配置、profile、transport/login、权威 Host、客户端镜像、Rust renderer、菜单、音频和关闭顺序；不要复制服务端玩法规则。
- 普通本地游戏也必须创建 Memory transport 并经过与远程连接相同的登录边界。不得因同进程运行而直接读取或修改权威模拟状态。

## 入口差异

- 普通本地启动先进入菜单，点击进入游戏后才打开存档、启动本地 Host 并登录。
- `-connect` 跳过本地 Host 和菜单延迟装配，经 TCP 连接并登录远程服务端。
- capture 使用确定性的内存世界与离屏 renderer；benchmark 使用固定工作负载和指定 transport 的无头观察者路径。
- capture 与 benchmark 都忽略用户材质覆盖，不创建交互窗口，也不请求音频设备。benchmark 输入、世界和报告身份不得随用户配置漂移。

## 视觉与性能门禁

- golden 只在预期视觉变化已逐图人工确认后更新；普通验证只比较，不自动接受差异。
- 性能阈值与数值只记录；真实 overflow、数据丢失、报告身份或结构不完整以及 I/O 错误仍必须失败。
- 场景顺序以 `captureScenes` 及其顺序测试为准，固定上传容量以布局代码和容量测试为准；不要在本指南复制会漂移的数字。

## 定点验证与入口

- 测试：`go test ./cmd/mornlea -race -count=1`。
- 无窗口视觉：`make visual-check`。
- 当前文档入口：`openspec/specs/visual-verification/spec.md`、`openspec/specs/bounded-benchmark-workload/spec.md`。
