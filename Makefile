.DEFAULT_GOAL := help

GO := go
CARGO ?= rustup run 1.97.1 cargo
RUST_DIR := engine
RUST_DYLIB := $(RUST_DIR)/target/release/libmornlea_engine.dylib
RUST_SO := $(RUST_DIR)/target/release/libmornlea_engine.so
APP := ./cmd/mornlea
BINARY := bin/mornlea
SERVER := ./cmd/mornlea-server
SERVER_BINARY := bin/mornlea-server
MORNLEA_DYLIB := bin/libmornlea_engine.dylib
MORNLEA_SO := bin/libmornlea_engine.so
PIXEL_PERFECTION_NOTICE_DIR := internal/assets/packs/pixel_perfection
PIXEL_PERFECTION_NOTICE_DEST := bin/third-party/pixel-perfection
ARGS ?=

.PHONY: help run build build-linux-server test test-race test-race-short test-multiplayer bench-multiplayer archcheck fmt clean visual-check visual-update rust rust-check dev-check agent-planner agent-implementer agent-gates agent-dashboard

run test test-multiplayer bench-multiplayer visual-check visual-update: rust
build: rust
test-race: rust

help:
	@printf '%s\n' \
		'常用命令：' \
		'  make run              运行游戏，可通过 ARGS 传递参数' \
		'  make build            构建 bin/mornlea、bin/mornlea-server 与同目录 Rust dylib' \
		'  make build-linux-server 构建 Linux amd64 专服与同目录 Rust .so' \
		'  make test             运行全部测试' \
		'  make test-race        使用 race detector 运行全部测试' \
		'  make test-race-short   race detector 快速冒烟(与 `-short` 同跳过重型测试)' \
		'  make dev-check        迭代期快检:gofmt/vet/短测试与 Rust 静态检查' \
		'  make test-multiplayer 运行 M3C 八玩家与 v6 报告测试' \
		'  make bench-multiplayer 运行三组 M3C 多人微基准' \
		'  make archcheck        验证依赖闭包与无图形服务端边界' \
		'  make rust             构建固定版本的 Rust cdylib' \
		'  make rust-check       运行 Rust 格式、clippy 与单测' \
		'  make fmt              格式化全部 Rust 与 Go 源码' \
		'  make visual-check     跑视觉场景并与 golden 基线比对' \
		'  make visual-update    重新生成 golden 基线（VISUAL_OUT 覆盖输出目录）' \
		'  make clean            删除 bin 目录' \
		'  make agent-planner    手动运行规划者工作者(docs/agents/planner.md)' \
		'  make agent-implementer 手动运行实现者工作者(docs/agents/implementer.md)' \
		'  make agent-gates      运行标准门禁汇总(scripts/agents/gates.sh)' \
		'  make agent-dashboard   本地执行状态看板(cmd/mornlea-agent-board)' \
		'  make help             显示此帮助'

run:
	$(GO) run $(APP) $(ARGS)

rust:
	cd $(RUST_DIR) && $(CARGO) build --locked --release

rust-check:
	cd $(RUST_DIR) && $(CARGO) fmt --check
	cd $(RUST_DIR) && $(CARGO) clippy --workspace --all-targets -- -D warnings
	cd $(RUST_DIR) && $(CARGO) test --workspace --locked

build:
	@mkdir -p $(dir $(BINARY))
	$(GO) build -ldflags='-extldflags=-Wl,-rpath,@loader_path' -o $(BINARY) $(APP)
	$(GO) build -ldflags='-extldflags=-Wl,-rpath,@loader_path' -o $(SERVER_BINARY) $(SERVER)
	cp $(RUST_DYLIB) $(MORNLEA_DYLIB)
	@mkdir -p $(PIXEL_PERFECTION_NOTICE_DEST)
	cp $(PIXEL_PERFECTION_NOTICE_DIR)/ATTRIBUTION.md $(PIXEL_PERFECTION_NOTICE_DEST)/ATTRIBUTION.md
	cp $(PIXEL_PERFECTION_NOTICE_DIR)/LICENSE.txt $(PIXEL_PERFECTION_NOTICE_DEST)/LICENSE.txt
	cp $(PIXEL_PERFECTION_NOTICE_DIR)/PROVENANCE.json $(PIXEL_PERFECTION_NOTICE_DEST)/PROVENANCE.json

build-linux-server: rust
	@mkdir -p $(dir $(SERVER_BINARY))
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build \
		-ldflags='-extldflags=-Wl,-rpath,$$ORIGIN' \
		-o $(SERVER_BINARY) $(SERVER)
	cp $(RUST_SO) $(MORNLEA_SO)

test:
	$(GO) test ./...

test-race:
	$(GO) test ./... -race

# test-race-short:迭代期 race 冒烟——跳过与 `-short` 相同的重型测试,
# 速度比 `test-race` 快一个数量级;提交前仍跑全量 `test-race`。
test-race-short:
	$(GO) test ./... -race -short

test-multiplayer:
	$(GO) test ./internal/client ./internal/server ./cmd/mornlea ./cmd/perfcheck \
		-run 'Test(PerfReportV6|ScenarioV6|PerfcheckV6|PerfcheckV5SameScenario|PerformanceThresholds|InterestObserver|HostStats|BenchmarkServerEpoch|BenchmarkServerMeasuredWindow)' -count=1

bench-multiplayer:
	$(GO) test ./internal/network ./internal/server ./internal/render -run '^$$' \
		-bench '(RemotePlayerStateCodec|EightPlayerInterest|RemoteAvatarNameTag)' -benchmem -count=3

archcheck:
	$(GO) test ./internal/archcheck -count=1
	test -z "$$($(GO) list -deps ./cmd/mornlea-server | rg 'internal/(client|mesh|render|gfx)|glfw|webgpu|x/image/font')"

# dev-check:迭代期快检——gofmt 检查、vet、全仓短测试(重型测试经 `-short` 跳过)
# 与 Rust fmt/clippy/单测。完整门禁(test/test-race/visual-check/rust-check)
# 仍留给 CI 与提交前,短模式不做任何正确性放宽。
dev-check:
	@unformatted=$$(gofmt -l .); if [ -n "$$unformatted" ]; then echo "gofmt 需要格式化: $$unformatted"; exit 1; fi
	$(GO) vet ./...
	$(GO) test ./... -short
	cd $(RUST_DIR) && $(CARGO) fmt --check
	cd $(RUST_DIR) && $(CARGO) clippy --workspace --all-targets -- -D warnings
	cd $(RUST_DIR) && $(CARGO) test --workspace --locked

fmt:
	cd $(RUST_DIR) && $(CARGO) fmt
	find . -type f -name '*.go' \
		-not -path './vendor/*' \
		-not -path './.worktrees/*' \
		-exec gofmt -w {} +

visual-check:
	$(GO) run $(APP) --capture $(or $(VISUAL_OUT),build/visual)

visual-update:
	$(GO) run $(APP) --capture $(or $(VISUAL_OUT),build/visual) --update-golden

clean:
	rm -rf bin

# agent-*:工作者入口——角色卡见 docs/agents/,开发流程见 docs/development-process.md。
# AGENT_TOOL=claude|codex(默认 claude);AGENT_EXTRA_ARGS 透传 CLI。
agent-planner:
	./scripts/agents/run-agent.sh planner

agent-implementer:
	./scripts/agents/run-agent.sh implementer

agent-gates:
	./scripts/agents/gates.sh

agent-dashboard:
	go run ./cmd/mornlea-agent-board
