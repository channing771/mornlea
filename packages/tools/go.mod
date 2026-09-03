module github.com/channing771/mornlea/packages/tools

go 1.26.0

require (
	github.com/channing771/mornlea/packages/client v0.0.0-00010101000000-000000000000
	github.com/channing771/mornlea/packages/shared v0.0.0-00010101000000-000000000000
	github.com/go-gl/mathgl v1.2.0
)

require (
	github.com/klauspost/compress v1.19.1 // indirect
	golang.org/x/image v0.44.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

// tools 是开发工具模块：perfcheck（性能报告比较）、agent-board（AI 工作者执行
// 状态看板，web/ 为其前端）、gfxspike（Rust renderer 地形渲染验证）与
// composite_grass_side（材质合成）。合法 require 方向为 shared/server/client/
// contracts——perfcheck 与 gfxspike 组合消费客户端镜像与渲染侧包（client 模块
// 对 server 的 require 经其传递），由 packages/audit 的单元边界表强制。各兄弟
// 经本地相对路径 replace 引用，保证 GOWORK=off 的单模块世界仍可构建。
replace github.com/channing771/mornlea/packages/contracts => ../contracts

replace github.com/channing771/mornlea/packages/shared => ../shared

replace github.com/channing771/mornlea/packages/server => ../server

replace github.com/channing771/mornlea/packages/client => ../client
