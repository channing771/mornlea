#!/usr/bin/env bash
# 标准门禁汇总：按 AGENTS.md「验证」节执行；任一失败即汇总并退出非零。
# 环境变量: GATES_SKIP_RACE=1 跳过全量 race（迭代期用，最终门禁必须跑）。
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
FAIL=0
STEP=0

step() { STEP=$((STEP+1)); echo; echo "== [${STEP}] $*"; }
run() { local desc="$1"; shift; echo "   --> $*"; if bash -c "$*"; then echo "   [PASS] $desc"; else echo "   [FAIL] $desc"; FAIL=1; fi; }

step "gofmt 检查（应无输出）"
run "gofmt 检查" 'test -z "$(gofmt -l .)"'

step "go vet（逐 workspace 模块）"
run "go vet 全部模块" 'for module in ./packages/contracts ./packages/shared ./packages/server ./packages/client ./packages/tools ./packages/audit; do go vet $module/... || exit 1; done'

step "archcheck（依赖边界 + 基线版本 + 文档一致）"
run "archcheck" 'go test ./packages/audit -count=1'

step "OpenSpec strict 校验"
run "OpenSpec strict" 'openspec validate --all --strict --no-interactive'

step "Rust 构建（固定工具链）"
run "make rust" 'make rust'

if [ "${GATES_SKIP_RACE:-0}" != "1" ]; then
  step "全量 race（逐 workspace 模块）"
  run "go test 全部模块 -race" 'for module in ./packages/contracts ./packages/shared ./packages/server ./packages/client ./packages/tools ./packages/audit; do go test $module/... -race || exit 1; done'
fi

echo
if [ "$FAIL" = 0 ]; then
  echo "全部门禁通过 ✅"
else
  echo "存在失败 ❌（见上，先修复再收尾）"
  exit 1
fi
