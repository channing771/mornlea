// Package storagedef 承载世界存储各子域共享的哨兵错误。
//
// `ErrCorrupt`/`ErrFutureVersion` 被 region 格式原语与 chunk/player/companion/
// hostile 四个实体域共同依赖；独立成不依赖任何 internal 包的叶子，让这份公共
// 依赖显式下沉——任一子域包都只依赖本包取哨兵，避免为取哨兵而依赖同侪域包
// （子包互不导入的方向契约）或反向依赖根包（与「根包 → 子包」方向冲突成环）。
// 哨兵经根包 var 别名再导出为既有 `storage.X` 入口，同一错误值保证
// errors.Is 身份与错误消息逐字节不变。
package storagedef

import "errors"

var (
	// ErrCorrupt 表示存档数据损坏：解码失败、校验不符或格式约束被破坏。
	ErrCorrupt = errors.New("storage: corrupt data")
	// ErrFutureVersion 表示存档数据的 envelope/schema 版本高于当前代码支持的
	// 上界，拒绝解码以避免静默丢弃未知字段。
	ErrFutureVersion = errors.New("storage: future version")
)
