module github.com/channing771/mornlea/packages/server

go 1.26.0

require (
	github.com/channing771/mornlea/packages/client v0.0.0-00010101000000-000000000000
	github.com/channing771/mornlea/packages/contracts v0.0.0-00010101000000-000000000000
	github.com/channing771/mornlea/packages/shared v0.0.0-00010101000000-000000000000
	github.com/go-gl/mathgl v1.2.0
	github.com/gofrs/flock v0.13.0
	github.com/klauspost/compress v1.19.1
	github.com/modelcontextprotocol/go-sdk v1.7.0
)

require (
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

// contracts、shared 与 client 是兄弟 Go 模块（contracts 的 go:embed 必须与 JSON
// 契约同目录；shared 承载双侧共享的领域包）；server 是服务端域模块，兄弟 require
// 集恰为 {contracts, shared, client}，由 internal/archcheck 的单元边界表强制。其中
// client 是测试专用豁免边：server 的 Memory/TCP 集成测试以客户端镜像驱动会话
// （Go 的 require 无法按测试限定，只能模块层放行）；生产代码禁 import client 由
// archcheck 源码守卫强制。各模块经本地相对路径 replace 引用，保证 GOWORK=off
// 的单模块世界仍可构建。
replace github.com/channing771/mornlea/packages/contracts => ../contracts

replace github.com/channing771/mornlea/packages/shared => ../shared

replace github.com/channing771/mornlea/packages/client => ../client
