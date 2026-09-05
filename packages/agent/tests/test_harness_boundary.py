"""`harness` 与扩展契约的网关栈隔离门禁。

`harness/**` 下任何文件都不得导入 `app`、`fastapi` 或 `uvicorn`；
组合网关的职责只属于 `app/`。扫描覆盖 `harness/src` 与
`extension-api/src`：扩展契约包同样走薄转调定位，一并禁网关栈。
"""

from __future__ import annotations

import ast
from pathlib import Path

AGENT_ROOT = Path(__file__).resolve().parents[1]
# 扩展契约包与 `harness` 同门禁，因其薄转调定位同样禁网关栈。
SCAN_ROOTS = (
    AGENT_ROOT / "harness" / "src",
    AGENT_ROOT / "extension-api" / "src",
)
FORBIDDEN_ROOTS = frozenset({"app", "fastapi", "uvicorn"})
DYNAMIC_LOADERS = frozenset({"import_module", "__import__"})


def _module_name(path: Path, src_root: Path) -> str:
    """由源码路径推导点分模块名，仅用于错误定位。"""

    relative = path.relative_to(src_root).with_suffix("")
    parts = relative.parts
    if parts[-1] == "__init__":
        parts = parts[:-1]
    return ".".join(parts)


def _forbidden_root(dotted: str) -> str | None:
    """点分名的首段命中网关栈时返回该首段，否则返回空。"""

    root = dotted.split(".", 1)[0]
    return root if root in FORBIDDEN_ROOTS else None


def _resolve_from(module: str, level: int, target: str | None, *, is_package: bool) -> str:
    """解析相对导入的目标模块名；非法层级返回哨兵。"""

    if level == 0:
        return target or ""
    package_parts = module.split(".") if is_package else module.split(".")[:-1]
    keep = len(package_parts) - (level - 1)
    if keep < 1:
        return "<invalid-relative-import>"
    base = package_parts[:keep]
    return ".".join((*base, *(target.split(".") if target else ())))


def _loader_name(func: ast.AST) -> str:
    """取出调用节点的函数名，支持别名与属性两种写法。"""

    if isinstance(func, ast.Name):
        return func.id
    if isinstance(func, ast.Attribute):
        return func.attr
    return ""


def _boundary_errors() -> list[str]:
    """扫描全部根目录，返回网关栈违规描述（为空表示通过）。"""

    errors: list[str] = []
    for src_root in SCAN_ROOTS:
        for path in sorted(src_root.rglob("*.py")):
            module = _module_name(path, src_root)
            try:
                tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
            except (SyntaxError, UnicodeError) as error:
                errors.append(f"{path}:{getattr(error, 'lineno', 0)}: {error}")
                continue
            is_package = path.name == "__init__.py"
            for node in ast.walk(tree):
                if isinstance(node, ast.Import):
                    for alias in node.names:
                        hit = _forbidden_root(alias.name)
                        if hit is not None:
                            errors.append(f"{path}:{node.lineno}: {module} 禁止导入网关栈 {hit}")
                elif isinstance(node, ast.ImportFrom):
                    target = _resolve_from(module, node.level, node.module, is_package=is_package)
                    hit = _forbidden_root(target) if target else None
                    if hit is not None:
                        errors.append(f"{path}:{node.lineno}: {module} 禁止导入网关栈 {hit}")
                elif isinstance(node, ast.Call):
                    if _loader_name(node.func) not in DYNAMIC_LOADERS:
                        continue
                    for arg in node.args:
                        if isinstance(arg, ast.Constant) and isinstance(arg.value, str):
                            hit = _forbidden_root(arg.value)
                            if hit is not None:
                                errors.append(
                                    f"{path}:{node.lineno}: {module} 禁止动态加载网关栈 {hit}"
                                )
    return errors


def test_harness_never_imports_gateway_stack() -> None:
    assert _boundary_errors() == []
