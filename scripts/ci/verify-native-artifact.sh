#!/usr/bin/env bash
# 校验 native-macos job 构建并经 artifact 下载回来的两个 cdylib：source SHA、
# manifest 行数与逐条 size/sha256 全部对上才算可信，任何缺失、篡改或行数漂移
# 直接失败。quality/go-race/integration 三个 job 在下载 artifact 之后统一调用
# 本脚本，替代原先在 ci.yml 里逐字重复三份的内联块——三处必须严格同一套
# 校验，重复文本一旦被单独改动就会出现 job 之间信任基准不一致。
# 运行前提：artifact 已解包到 packages/engine/target/release，且当前目录是仓库根。
set -euo pipefail

ENGINE_DYLIB=packages/engine/target/release/libmornlea_engine.dylib
CLIENT_DYLIB=packages/engine/target/release/libmornlea_client.dylib
MANIFEST=packages/engine/target/release/native-artifact-manifest.txt

test -f "$ENGINE_DYLIB"
test -f "$CLIENT_DYLIB"
test "$(cat packages/engine/target/release/native-source-sha.txt)" = "$GITHUB_SHA"
# manifest 恰好 3 行：sha 头行 + 两个 dylib 条目；多一行少一行都算漂移。
test "$(wc -l < "$MANIFEST" | tr -d ' ')" = 3
{
  IFS=' ' read -r kind sha extra
  test "$kind" = sha
  test "$sha" = "$GITHUB_SHA"
  test -z "$extra"
  validate_artifact() {
    expected_path=$1
    IFS=' ' read -r path size digest extra
    test "$path" = "$expected_path"
    test -z "$extra"
    case "$size" in ''|*[!0-9]*) exit 1 ;; esac
    test "$size" = "$(stat -f '%z' "$path")"
    test "$digest" = "$(shasum -a 256 "$path" | awk '{print $1}')"
  }
  validate_artifact "$ENGINE_DYLIB"
  validate_artifact "$CLIENT_DYLIB"
} < "$MANIFEST"
# 校验通过后布置 deps/：cgo 的 -Wl,-rpath 指向该目录，go test 加载到的
# 必须是刚校验过的这份库，而不是本机可能存在的陈旧构建。
mkdir -p packages/engine/target/release/deps
cp "$ENGINE_DYLIB" packages/engine/target/release/deps/
cp "$CLIENT_DYLIB" packages/engine/target/release/deps/
