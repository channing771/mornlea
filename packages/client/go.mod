module github.com/channing771/mornlea/packages/client

go 1.26.0

require (
	github.com/channing771/mornlea/packages/server v0.0.0-00010101000000-000000000000
	github.com/channing771/mornlea/packages/shared v0.0.0-00010101000000-000000000000
	github.com/go-gl/mathgl v1.2.0
	golang.org/x/image v0.44.0
)

require (
	github.com/channing771/mornlea/packages/contracts v0.0.0-00010101000000-000000000000 // indirect
	github.com/gofrs/flock v0.13.0 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/modelcontextprotocol/go-sdk v1.7.0 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

// shared 与 server 是兄弟 Go 模块（shared 承载双侧共享的领域包；server 承载
// 服务端权威域）。client 是客户端域模块；对 server 与 contracts 的 require 仅由
// cmd/mornlea 应用入口经 server 传递消费（contracts 的 go:embed 必须与 JSON
// 契约同目录）——普通本地与 benchmark 模式在进程内装配本地权威 Host（Memory
// transport 登录边界不变）。client 域库包（client/render/mesh/lod/audio/assets）
// MUST NOT import server，由 packages/audit 源码守卫强制；允许 require 边由
// 单元边界表强制。各兄弟经本地相对路径 replace 引用，保证 GOWORK=off 的单模块
// 世界仍可构建。
replace github.com/channing771/mornlea/packages/contracts => ../contracts

replace github.com/channing771/mornlea/packages/shared => ../shared

replace github.com/channing771/mornlea/packages/server => ../server
