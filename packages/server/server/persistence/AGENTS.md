# Persistence 子包指南

## 所有权

`persistence` 单独持有世界、玩家、伙伴、夜行者和被动牛的存档加载、观察、异步保存、重试、flush 与 worker 生命周期。根 `server` 只持有 Host、Server、登录、会话、权威 tick、发布和关服编排。根 `server` 保留 `PersistenceStatus` 和 `ErrPlayerPersistenceBackpressure` 的兼容 re-export 与委派；子包持有其状态计算和背压哨兵实现。

## 依赖边界

生产代码只能依赖实际需要的低层领域和存储包；不得导入根 `packages/server/server`，也不得访问 Host、Server、session 或其私有状态。依赖方向以 `packages/audit` 的规则为准。

## 并发与 I/O

worker 只能接收已克隆的不可变存档载荷，并且不得读取实时模拟或会话状态。成功发送到其他 goroutine 后，载荷及其切片必须视为不可变。权威 tick 只能执行已有的有界、非阻塞调度；磁盘 I/O 只能在 worker 中执行，不能发生在 tick 路径。
