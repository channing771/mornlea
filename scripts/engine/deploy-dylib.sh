#!/usr/bin/env bash
# 把 cargo 共享 target 目录构建出的 engine/client cdylib 部署到规范 release
# 目录，并完成 macOS 专属的 install name 改写与重签名。Makefile 的 rust
# target 是唯一调用方，CI 经 make rust 间接受益；规范 release 目录同时是
# cgo -L/-rpath、bin/ 拷贝与 CI artifact 打包的取数处。
# 运行前提：cargo build 已完成，当前目录是仓库根。
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "用法：$0 <cargo_release_dir> <release_dir>" >&2
  exit 2
fi

cargo_release_dir=$1
release_dir=$2

mkdir -p "$release_dir"
# 四份产物按需拷贝：macOS（.dylib）与 Linux（.so）不会同时存在，单个平台
# 缺另一平台的库不是错误；拷贝本身失败仍会因 set -e 直接中断部署。
for f in \
  libmornlea_engine.dylib \
  libmornlea_engine.so \
  libmornlea_client.dylib \
  libmornlea_client.so; do
  if [ -f "$cargo_release_dir/$f" ]; then
    cp -f "$cargo_release_dir/$f" "$release_dir/$f"
  fi
done

# macOS：把 install name 改写为 @rpath。cargo 默认嵌入共享目标目录的绝对
# 路径，CI 的 race/integration job 是下载 artifact 到规范 release 目录后
# 运行，绝对路径对不上会让 dyld 直接拒载（表现为测试包秒挂）。@rpath 由
# cgo 的 -Wl,-rpath 解析到规范路径，dylib 因此与构建位置无关。install
# name 改写会让既有签名失效，故逐个重签 ad-hoc 签名补回。
if [ "$(uname)" = "Darwin" ]; then
  for f in libmornlea_engine.dylib libmornlea_client.dylib; do
    if [ -f "$release_dir/$f" ]; then
      install_name_tool -id @rpath/"$f" "$release_dir/$f"
      codesign --force --sign - "$release_dir/$f"
    fi
  done
fi
