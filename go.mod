module github.com/channing771/mornlea

go 1.26.0

require (
	github.com/channing771/mornlea/packages/client v0.0.0-00010101000000-000000000000
	github.com/channing771/mornlea/packages/shared v0.0.0-00010101000000-000000000000
	github.com/go-gl/mathgl v1.2.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	golang.org/x/image v0.44.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

// contracts、shared 与 client 是独立 Go 模块（contracts 的 go:embed 必须与
// JSON 契约同目录；shared 承载双侧共享的领域包；client 承载客户端域——根模块
// 在 tools 切割前的过渡期仍消费其呈现侧包）；server 已不被根模块直接 require
// （服务端入口随 S5 迁入 packages/server）。各模块经本地 replace 引用，保证
// GOWORK=off 的单模块世界仍可构建。
replace github.com/channing771/mornlea/packages/contracts => ./packages/contracts

replace github.com/channing771/mornlea/packages/shared => ./packages/shared

replace github.com/channing771/mornlea/packages/client => ./packages/client
