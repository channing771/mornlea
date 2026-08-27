#!/usr/bin/env bash
# race-changed.sh — 只对「改动包及其反向依赖」跑 race 测试（测试分层纪律的 T1 层）。
#
# 背景：全量 `go test ./... -race -count=1` 实测约 4.5 分钟且 82% 耗时集中在
# cmd/mornlea 与 internal/server 两个包（见 docs/notes/test-quickstart.md）；
# 绝大多数改动只触及叶子包，为它们付全量代价是开发时间的主要浪费之一。
#
# 包集合的构造规则：
#   1. 改动文件集 = `git diff --name-only <base>...HEAD` ∪ 暂存/未暂存改动 ∪
#      未跟踪的 *.go（工作区当前状态全算）；base 默认 origin/main，可用 --base 覆盖。
#   2. 文件 → 包：按 `go list` 的包目录前缀映射；不在任何包内的 .go（如 testdata）跳过。
#   3. 反向依赖闭包：沿 `go list` 的 .Imports 传递扩散；.TestImports/.XTestImports
#      只作一层直接依赖（测试对被测包的依赖），不沿测试边继续传递——否则触碰
#      internal/core 这类底座包时闭包近似全仓，工具就失去意义；残余风险由
#      T3 全量门禁与 CI 兜底。
#   4. 恒含 internal/archcheck（依赖边界/基线版本/文档一致守卫，秒级）。
#
# 用法:
#   scripts/agents/race-changed.sh            # 计算并运行 go test <集合> -race -count=1
#   scripts/agents/race-changed.sh --diff     # 只打印包集合与依据，不运行
#   scripts/agents/race-changed.sh --base v1  # 换比较基线（默认 origin/main）
#
# 退出码：包集合为空（无 Go 改动）时 0；测试失败时透传 go test 的退出码。
set -euo pipefail

BASE="origin/main"
DIFF_ONLY=0
while [ $# -gt 0 ]; do
  case "$1" in
    --diff) DIFF_ONLY=1 ;;
    --base) BASE="$2"; shift ;;
    *) echo "未知参数: $1（支持 --diff / --base <ref>）" >&2; exit 2 ;;
  esac
  shift
done

if ! git rev-parse --verify --quiet "$BASE" >/dev/null; then
  echo "基线 ${BASE} 不存在，回退 HEAD" >&2
  BASE="HEAD"
fi

# 改动文件集：已提交差异 + 暂存 + 未暂存 + 未跟踪，全部过滤出 .go。
changed_files="$( {
  git diff --name-only "${BASE}...HEAD" 2>/dev/null || true
  git diff --name-only
  git diff --cached --name-only
  git ls-files --others --exclude-standard
} | sort -u | grep '\.go$' || true)"

if [ -z "$changed_files" ]; then
  echo "无 Go 改动（相对 ${BASE}），无事可测"
  exit 0
fi

# 文件 → 包：go list 一次给出全部包目录与导入路径。
pkg_map="$(go list -f '{{.Dir}}|{{.ImportPath}}' ./...)"
changed_pkgs="$(printf '%s\n' "$changed_files" | while read -r f; do
  dir="$(dirname "$f")"
  abs="$(pwd)/$dir"
  printf '%s\n' "$pkg_map" | awk -F'|' -v d="$abs" '$1 == d { print $2 }'
done | sort -u)"

if [ -z "$changed_pkgs" ]; then
  echo "改动只涉及包外的 .go 文件（如 testdata），无包可测"
  exit 0
fi

# 反向依赖闭包：生产导入边传递扩散；测试导入边只作一层直接依赖。
imports_graph="$(go list -f '{{.ImportPath}}|{{join .Imports " "}}|{{join .TestImports " "}}|{{join .XTestImports " "}}' ./...)"
closure="$(printf '%s\n' "$changed_pkgs")"
frontier="$(printf '%s\n' "$changed_pkgs")"
# 生产边可传递：迭代到不动点（包数有限，最多迭代包总数次）。
while [ -n "$frontier" ]; do
  next=""
  while read -r p; do
    [ -n "$p" ] || continue
    # 找到把 p 列进 .Imports 的包，且尚未入集合的，作为下一层。
    new_dependents="$(printf '%s\n' "$imports_graph" | awk -F'|' -v p="$p" '{
      n = split($2, a, " ")
      for (i = 1; i <= n; i++) if (a[i] == p) print $1
    }')"
    while read -r d; do
      [ -n "$d" ] || continue
      if ! printf '%s\n' "$closure" | grep -qx "$d"; then
        closure="$(printf '%s\n%s\n' "$closure" "$d")"
        next="$(printf '%s\n%s\n' "$next" "$d")"
      fi
    done <<< "$new_dependents"
  done <<< "$frontier"
  frontier="$(printf '%s\n' "$next" | sort -u | grep -v '^$' || true)"
done
# 测试边只作一层直接依赖：测试文件 import 了改动包的包，其测试二进制需要重编重跑。
test_dependents="$(printf '%s\n' "$changed_pkgs" | while read -r p; do
  [ -n "$p" ] || continue
  printf '%s\n' "$imports_graph" | awk -F'|' -v p="$p" '{
    n = split($3, a, " "); for (i = 1; i <= n; i++) if (a[i] == p) print $1
    n = split($4, a, " "); for (i = 1; i <= n; i++) if (a[i] == p) print $1
  }'
done | sort -u)"
closure="$(printf '%s\n%s\n' "$closure" "$test_dependents" | sort -u | grep -v '^$')"
# 恒含 archcheck：依赖边界、基线版本与文档一致守卫，秒级成本。
closure="$(printf '%s\n%s\n' "$closure" "github.com/channing771/mornlea/internal/archcheck" | sort -u)"

echo "基线 ${BASE}；改动包："
printf '  %s\n' "$changed_pkgs"
echo "反向依赖闭包（含 archcheck）："
printf '  %s\n' "$closure"

if [ "$DIFF_ONLY" = 1 ]; then
  exit 0
fi

heavy="$(printf '%s\n' "$closure" | grep -E '/(cmd/mornlea|internal/server)$' || true)"
if [ -n "$heavy" ]; then
  echo "提示：集合含重型包（${heavy}），预计分钟级；仅迭代验证可改用 --diff 后手动加 -short" >&2
fi

# shellcheck disable=SC2086 —— 包列表需要按空白展开为多个参数（不可加引号：
# 引号会把换行分隔的整个列表折叠成单个 malformed import path 参数）。
go test $closure -race -count=1
