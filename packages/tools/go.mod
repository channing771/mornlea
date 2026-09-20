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

// tools is the development-tools module: perfcheck (performance report
// comparison), agent-board (worker dashboard; web/ is its frontend),
// gfxspike (Rust renderer terrain spike), composite_grass_side (texture
// compositing), and runtime-oracle (offline contract inventory and replay).
// Legal require directions are shared/server/client/contracts. perfcheck and
// gfxspike compose client mirror and render packages (the client module's
// server require is transitive); runtime-oracle stays a stdlib-only leaf.
// packages/audit enforces the unit-boundary table. Sibling modules are
// referenced by local relative replace so GOWORK=off single-module builds
// still work.
replace github.com/channing771/mornlea/packages/contracts => ../contracts

replace github.com/channing771/mornlea/packages/shared => ../shared

replace github.com/channing771/mornlea/packages/server => ../server

replace github.com/channing771/mornlea/packages/client => ../client
