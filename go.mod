module github.com/channing771/mornlea

go 1.26.0

require (
	github.com/channing771/mornlea/packages/contracts v0.0.0-00010101000000-000000000000
	github.com/channing771/mornlea/packages/shared v0.0.0-00010101000000-000000000000
	github.com/go-gl/mathgl v1.2.0
	github.com/gofrs/flock v0.13.0
	github.com/klauspost/compress v1.19.1
	github.com/modelcontextprotocol/go-sdk v1.7.0
	golang.org/x/image v0.44.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

// contracts 与 shared 是独立 Go 模块（contracts 的 go:embed 必须与 JSON 契约
// 同目录；shared 承载双侧共享的领域包）；经本地 replace 引用，保证 GOWORK=off
// 的单模块世界仍可构建。
replace github.com/channing771/mornlea/packages/contracts => ./packages/contracts

replace github.com/channing771/mornlea/packages/shared => ./packages/shared
