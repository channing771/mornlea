---
doc_id: godot-client-project
language: zh
counterpart: README.md
revision: 2026-09-16.5
---

# Mornlea Godot 客户端

此目录是桌面客户端试点以及未来经批准全量迁移所共用的稳定 Godot 项目根目录。请在 Godot Project Manager 中直接打开此目录，不要打开仓库根目录。后续迁移阶段继续沿用该位置，避免再次搬迁场景 UID、`res://` 路径、工具链与发布身份。

## 打开项目

项目要求 Godot 4.7.2 Standard。可先核验已钉定的官方制品元数据而不下载文件：

```bash
scripts/godot/fetch.sh --verify-only
```

以下命令在不显示前台窗口的情况下打开并导入项目：

```bash
scripts/godot/godot.sh --headless --path apps/mornlea-godot --editor --quit
```

可以通过 `MORNLEA_GODOT_BIN` 指定 Godot 4.7.2 可执行文件的绝对路径。如果官方 `/Applications/Godot.app` 安装版本与项目钉定版本一致，包装脚本也会直接使用它。

永久主场景仅使用纯 GDScript 和 Godot 内置节点。打开编辑器时，Python 与原生制品都不是前置条件。Bootstrap 会报告缺失的 Python 扩展、内嵌解释器、标准库与项目桥文件，不兼容的 Godot 或扩展描述符、检测到的桌面目标，以及精确的准备命令。此诊断路径既不会导入 Python 功能，也不会连接服务器。

当前 macOS Apple Silicon 运行时与桥接使用以下命令准备：

```bash
scripts/godot/build-python-runtime.sh --verify --offline
scripts/godot/build-extension.sh --target aarch64-apple-darwin --profile debug --verify
```

以下命令不会修改项目工作区，可分别验证两条干净状态诊断路径：

```bash
scripts/godot/openable-smoke.sh --without-native
scripts/godot/openable-smoke.sh --without-python
```

## 资源同步

注册 atlas 的权威来源继续是 `packages/client/assets`，注册的 Noto Sans CJK 字体继续以 `packages/client/render/assets` 为权威来源。使用以下命令物化 Godot 项目内的衍生资源：

```bash
scripts/godot/sync-assets.sh
scripts/godot/sync-assets.sh --check
```

生成目录包含按材质层优先、mip 层次次优先排列的 RGBA8 atlas、注册字体及其 OFL/来源文件、保留的材质许可证记录，以及确定性清单。清单记录源 Git 树、每个输入的校验和、聚合输入校验和、atlas 布局与每个输出的校验和。不得编辑 `assets/generated/` 下的文件，也不得手工添加文件；检查器会拒绝缺失、改写、符号链接及手写内容。

## Python 开发检查

生产 Python 依赖保持为空。内嵌 CPython 运行时只包含已经通过资格验证的 Py4Godot 单元，不安装开发工具。Ruff 与 mypy 仅作为开发依赖，由 `uv.lock` 精确解析；`uv` 可以根据锁文件填充本地且被忽略的 `.venv/`，但不会修改内嵌运行时或向其中安装软件包。`typing/` 下的本地 Py4Godot 存根提供受检查接口，不导入生成的插件代码。

使用以下命令运行格式、静态检查、严格类型检查、边界 mutation tests 与源码策略检查：

```bash
scripts/godot/python-check.sh --locked
```

边界检查会拒绝导入 companion Agent、直接访问原生 ABI 或动态库、从 Python 发起网络操作、运行时安装器、进程执行、不受控动态导入，以及非英文源码注释。

## Python 功能宿主契约

Bootstrap 完成依赖交接后，由 `app/host/app_root.py` 与 `app/host/feature_host.py` 负责目录规划和功能生命周期。`config/feature_catalog.tres` 是显式白名单，它直接列出粗粒度 `feature.tres` 清单，而不会扫描目录。每份清单声明 Host 协议 `1.0`、一个项目内入口场景、稳定依赖、带版本的桥接功能族要求、必需或可选失败语义、预算等级与重置策略。

生命周期顺序是确定的：验证目录、按排序后的依赖顺序实例化、验证 Python 功能、注入唯一的类型化 Godot 桥接服务、按 epoch 激活、重置，并按反序停用。不兼容或启动失败的必需功能会终止装配并释放此前已激活的功能；可选功能会以可观察结果被禁用，其依赖者不能越过该状态静默启动。功能脚本不会导入同级项目模块，也不会动态发现实现；Godot 资源路径承担有界组合，同时隔离解释器继续确保项目目录不进入 `sys.path`。

使用以下命令运行内嵌 Python 契约和增量扩展检查：

```bash
scripts/godot/feature-contract-check.sh
scripts/godot/feature-contract-check.sh --extensibility-probe
```

## 架构边界

Godot 只负责桌面窗口、键盘鼠标采集、呈现、试点 UI 与 Godot 资源生命周期。Go 客户端运行时继续负责协议 v44、镜像、预测、语义帧状态与有界网络处理。网格、光照、碰撞、射线检测和物理由 engine ABI v11 中的数值实现继续负责。权威 Go 服务器仍是世界与玩家真值的唯一所有者。

未来功能通过同一根目录中的粗粒度 `features/`、`platform/desktop/`、`config/` 与唯一 `addons/mornlea_bridge/` 边界扩展。Bootstrap 必须保持与具体功能无关。移动端、Web、主机、触摸、传感器和移动生命周期支持均不属于本项目范围。

编辑器缓存和原生桥接二进制会被忽略；Godot 生成脚本与着色器 `.uid` 伴随文件后应纳入版本控制。
