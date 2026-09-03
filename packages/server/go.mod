module github.com/channing771/mornlea/packages/server

go 1.26.0

require (
	github.com/channing771/mornlea v0.1.0-m4q
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

// contracts 与 shared 是兄弟 Go 模块（contracts 的 go:embed 必须与 JSON 契约
// 同目录；shared 承载双侧共享的领域包）；server 是服务端域模块，生产代码
// 只允许 require 这两个兄弟单元，经本地相对路径 replace 引用，保证 GOWORK=off
// 的单模块世界仍可构建。对根模块的 require 仅供本模块测试编译：server 的
// Memory/TCP 集成测试以 internal/client mirror 驱动会话（仅测试导入边，无
// 生产依赖）；客户端域切割后此边必须随测试重构消除。
replace github.com/channing771/mornlea/packages/contracts => ../contracts

replace github.com/channing771/mornlea/packages/shared => ../shared

replace github.com/channing771/mornlea => ../..
