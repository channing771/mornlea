"""工作区 Python 分层边界门禁。

依赖方向只有一条：`app` 组合 `harness`/`extension_api`，`harness` 内层只
向下依赖 `domain`，`extension_api` 只做 `harness.domain` 薄转调。`domain`
与工厂不得触碰网关栈；内部导入不得使用旧 `mornlea` 前缀。
"""

from __future__ import annotations

import ast
import importlib
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

import pytest

AGENT_ROOT = Path(__file__).resolve().parents[1]
SRC_ROOTS = {
    "harness": AGENT_ROOT / "harness" / "src",
    "extension_api": AGENT_ROOT / "extension-api" / "src",
    "app": AGENT_ROOT / "app" / "src",
}
KNOWN_AREAS = {
    "harness": frozenset(
        {
            "agents",
            "config",
            "domain",
            "extensions",
            "models",
            "persistence",
            "runtime",
            "sandbox",
            "skills",
            "store",
            "subagents",
            "tools",
        }
    ),
    "extension_api": frozenset({"contracts"}),
    "app": frozenset({"gateway"}),
}
HEAVY_IMPORTS = {
    "aiosqlite",
    "fastapi",
    "httpx",
    "langchain",
    "langchain_core",
    "langchain_mcp_adapters",
    "langchain_openai",
    "langgraph",
    "mcp",
    "sqlite3",
    "starlette",
    "uvicorn",
}
# 动态导入面只允许出现在组合缝合层：`app` 命令行懒加载网关，
# `harness.agents` 包根懒导出工厂，其余内层一律禁止。
DYNAMIC_SEAM_LAYERS = {"app", "harness.agents"}
LOCAL_ALLOWED = {
    "harness.root": set(),
    "harness.domain": {"harness.domain"},
    "harness.config": {"harness.config"},
    "harness.persistence": {"harness.persistence"},
    "harness.runtime": {"harness.domain", "harness.runtime"},
    "harness.store": {"harness.domain", "harness.persistence", "harness.store"},
    "harness.models": {"harness.domain", "harness.models", "harness.tools"},
    "harness.tools": {"harness.domain", "harness.tools"},
    "harness.agents": {"harness.agents", "harness.domain"},
    "harness.extensions": {"harness.extensions"},
    "harness.sandbox": {"harness.sandbox"},
    "harness.skills": {"harness.skills"},
    "harness.subagents": {"harness.subagents"},
    "extension_api": {"extension_api", "harness.domain"},
    "app": {"app", "extension_api"},
}
THIRD_PARTY_ALLOWED = {
    "harness.root": set(),
    "harness.domain": {"pydantic"},
    "harness.config": {"idna", "pydantic", "yaml"},
    "harness.persistence": set(),
    "harness.runtime": {"pydantic"},
    "harness.store": {"aiosqlite", "pydantic"},
    "harness.models": {"httpx", "langchain_core", "langchain_openai", "pydantic"},
    "harness.tools": {"httpx", "mcp", "pydantic"},
    "harness.agents": {"langgraph", "pydantic"},
    "harness.extensions": set(),
    "harness.sandbox": set(),
    "harness.skills": set(),
    "harness.subagents": set(),
    "extension_api": {"pydantic"},
    "app": {"fastapi", "pydantic", "starlette", "uvicorn"},
}


@dataclass(frozen=True)
class ImportEdge:
    source: str
    target: str
    line: int


def _module_name(path: Path, src_root: Path) -> str:
    """由源码路径推导点分模块名，相对各分发包的 `src` 目录。"""

    relative = path.relative_to(src_root).with_suffix("")
    parts = relative.parts
    if parts[-1] == "__init__":
        parts = parts[:-1]
    return ".".join(parts)


def _resolve_from(source: str, level: int, module: str | None, *, is_package: bool) -> str:
    """解析相对导入的目标模块名；非法层级返回哨兵。"""

    source_parts = source.split(".")
    package_parts = source_parts if is_package else source_parts[:-1]
    if level == 0:
        return module or ""
    keep = len(package_parts) - (level - 1)
    if keep < 1:
        return "<invalid-relative-import>"
    base = package_parts[:keep]
    return ".".join((*base, *(module.split(".") if module else ())))


def _layer(module: str) -> str:
    """把点分模块名折叠到门禁层；未知首段标为外部。"""

    head, _, rest = module.partition(".")
    if head == "harness":
        area, _, _ = rest.partition(".")
        return f"harness.{area}" if area else "harness.root"
    if head in {"extension_api", "app"}:
        return head
    return "external"


def _dynamic_surface_errors(tree: ast.AST, source: str) -> list[str]:
    """非缝合层的动态导入面一律拒绝，缝合层放行懒导出写法。"""

    source_layer = _layer(source)
    if source_layer in DYNAMIC_SEAM_LAYERS:
        return []
    errors: list[str] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                if alias.name.split(".", 1)[0] in {"builtins", "importlib"}:
                    errors.append(
                        f"{source}:{node.lineno}: dynamic import surface forbidden "
                        f"in {source_layer}"
                    )
        elif isinstance(node, ast.ImportFrom):
            if (node.module or "").split(".", 1)[0] in {"builtins", "importlib"}:
                errors.append(
                    f"{source}:{node.lineno}: dynamic import surface forbidden in {source_layer}"
                )
        elif (
            isinstance(node, ast.Name)
            and isinstance(node.ctx, ast.Load)
            and node.id == "__import__"
        ):
            errors.append(
                f"{source}:{node.lineno}: dynamic import surface forbidden in {source_layer}"
            )
    return errors


def _scan(
    src_roots: dict[str, Path] = SRC_ROOTS,
) -> tuple[list[ImportEdge], list[str]]:
    """扫描全部源码根，返回导入边与解析错误。"""

    edges: list[ImportEdge] = []
    errors: list[str] = []
    for _dist, src_root in sorted(src_roots.items()):
        for path in sorted(src_root.rglob("*.py")):
            source = _module_name(path, src_root)
            try:
                tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
            except (SyntaxError, UnicodeError) as error:
                errors.append(f"{path}:{getattr(error, 'lineno', 0)}: {error}")
                continue
            is_package = path.name == "__init__.py"
            errors.extend(_dynamic_surface_errors(tree, source))
            for node in ast.walk(tree):
                if isinstance(node, ast.Import):
                    edges.extend(
                        ImportEdge(source, alias.name, node.lineno) for alias in node.names
                    )
                elif isinstance(node, ast.ImportFrom):
                    target = _resolve_from(source, node.level, node.module, is_package=is_package)
                    edges.append(ImportEdge(source, target, node.lineno))
    return edges, errors


def _boundary_errors(src_roots: dict[str, Path] = SRC_ROOTS) -> list[str]:
    """校验层间方向、第三方白名单与已知区域，返回违规描述。"""

    edges, errors = _scan(src_roots)
    for edge in edges:
        source_layer = _layer(edge.source)
        target_layer = _layer(edge.target)
        if source_layer == "external":
            errors.append(f"{edge.source}: unknown source package")
            continue
        if source_layer not in LOCAL_ALLOWED:
            errors.append(f"{edge.source}: unknown source layer")
            continue
        if target_layer == "external":
            target_root = edge.target.split(".", 1)[0]
            if (
                target_root not in sys.stdlib_module_names
                and target_root not in THIRD_PARTY_ALLOWED[source_layer]
            ):
                errors.append(
                    f"{edge.source}:{edge.line}: forbidden external dependency "
                    f"{source_layer}->{edge.target}"
                )
            continue
        if target_layer.startswith("harness.") and source_layer == "app":
            continue
        if target_layer not in LOCAL_ALLOWED[source_layer]:
            errors.append(f"{edge.source}:{edge.line}: forbidden {source_layer}->{target_layer}")
    for dist, src_root in sorted(src_roots.items()):
        package_root = src_root / dist
        if not package_root.is_dir():
            errors.append(f"{src_root}: missing package {dist}")
            continue
        for path in sorted(package_root.iterdir()):
            if path.name in {"__init__.py", "__pycache__"}:
                continue
            name = path.stem if path.is_file() else path.name
            if name not in KNOWN_AREAS[dist]:
                errors.append(f"{path}: unknown package layer")
    return errors


def _write_fake_tree(root: Path, files: dict[str, str]) -> dict[str, Path]:
    """在临时目录搭出 `harness|extension_api|app/src` 三段式假树。"""

    roots: dict[str, Path] = {}
    for dist, src in (
        ("harness", root / "harness" / "src"),
        ("extension_api", root / "extension-api" / "src"),
        ("app", root / "app" / "src"),
    ):
        roots[dist] = src
        (src / dist).mkdir(parents=True, exist_ok=True)
        (src / dist / "__init__.py").write_text("", encoding="utf-8")
    for relative, content in files.items():
        target = root / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")
    return roots


def test_workspace_import_boundaries_are_known() -> None:
    assert _boundary_errors() == []


def test_workspace_uses_new_import_prefixes() -> None:
    edges, _ = _scan()
    legacy = [edge for edge in edges if edge.target.split(".", 1)[0].startswith("mornlea")]
    assert legacy == []


def test_scanner_resolves_relative_imports_and_rejects_forbidden_edges(tmp_path: Path) -> None:
    roots = _write_fake_tree(
        tmp_path,
        {"harness/src/harness/domain/__init__.py": "from ..config import AgentConfig\n"},
    )
    errors = _boundary_errors(roots)
    assert any("forbidden harness.domain->harness.config" in error for error in errors)


def test_scanner_fails_closed_on_syntax_errors_and_unknown_layers(tmp_path: Path) -> None:
    roots = _write_fake_tree(
        tmp_path,
        {"harness/src/harness/mystery.py": "if (\n"},
    )
    errors = _boundary_errors(roots)
    assert any("SyntaxError" in error or "was never closed" in error for error in errors)
    assert any("unknown package layer" in error for error in errors)


def test_scanner_rejects_dynamic_boundary_bypasses(tmp_path: Path) -> None:
    roots = _write_fake_tree(
        tmp_path,
        {
            "harness/src/harness/domain/__init__.py": (
                'from importlib import import_module\nimport_module("fastapi")\n'
            )
        },
    )
    errors = _boundary_errors(roots)
    assert any("dynamic import surface" in error for error in errors)


@pytest.mark.parametrize(
    "source",
    [
        'from importlib import import_module as load\nload("fastapi")\n',
        'from builtins import __import__ as load\nload("fastapi")\n',
        'import importlib as x\nx.import_module("fastapi")\n',
        'import importlib\ngetattr(importlib, "import_module")("fastapi")\n',
        'import importlib\nload = getattr(importlib, "import_module")\nload("fastapi")\n',
        'import builtins as b\nb.__import__("fastapi")\n',
        'import importlib as il\nalias = il\nalias.import_module("fastapi")\n',
        ('from importlib import import_module\nalias: object = import_module\nalias("fastapi")\n'),
        ('import importlib\nload = getattr(importlib, "import_" + "module")\nload("fastapi")\n'),
        ('import importlib\nload = importlib.__dict__["import_module"]\nload("fastapi")\n'),
    ],
)
def test_scanner_rejects_common_dynamic_import_aliases(tmp_path: Path, source: str) -> None:
    roots = _write_fake_tree(
        tmp_path,
        {"harness/src/harness/domain/__init__.py": source},
    )
    errors = _boundary_errors(roots)
    assert any("dynamic import surface" in error for error in errors), errors


@pytest.mark.parametrize(
    ("layer", "source"),
    [
        (
            "harness.domain",
            "from functools import partial\n"
            "from importlib import import_module\n"
            'load = partial(import_module, "fastapi")\n'
            "load()\n",
        ),
        ("harness.config", 'load = (__import__,)[0]\nload("fastapi")\n'),
        ("harness.store", "def load(loader=__import__):\n    return loader('fastapi')\n"),
    ],
)
def test_protected_layers_forbid_dynamic_import_surfaces(
    tmp_path: Path, layer: str, source: str
) -> None:
    relative = f"harness/src/{layer.replace('.', '/')}/__init__.py"
    roots = _write_fake_tree(tmp_path, {relative: source})
    errors = _boundary_errors(roots)
    assert any("dynamic import surface" in error for error in errors), errors


@pytest.mark.parametrize(
    ("layer", "module"),
    [
        ("harness.domain", "fastapi"),
        ("harness.domain", "uvicorn"),
        ("harness.domain", "harness.config"),
        ("harness.domain", "harness.store"),
        ("harness.config", "fastapi"),
        ("harness.config", "harness.domain"),
        ("harness.store", "httpx"),
        ("harness.store", "langgraph"),
        ("harness.store", "harness.models"),
        ("harness.models", "mcp"),
        ("harness.models", "fastapi"),
        ("harness.models", "harness.store"),
        ("harness.agents", "fastapi"),
        ("harness.agents", "harness.store"),
        ("extension_api", "httpx"),
        ("extension_api", "harness.store"),
        ("extension_api", "app.gateway.app"),
    ],
)
def test_future_layers_reject_out_of_role_dependencies(
    tmp_path: Path, layer: str, module: str
) -> None:
    if layer == "extension_api":
        relative = "extension-api/src/extension_api/contracts.py"
    else:
        relative = f"harness/src/{layer.replace('.', '/')}/__init__.py"
    roots = _write_fake_tree(tmp_path, {relative: f"import {module}\n"})
    errors = _boundary_errors(roots)
    assert any("forbidden" in error for error in errors), errors


@pytest.mark.parametrize(
    ("layer", "module"),
    [
        ("harness.domain", "pydantic"),
        ("harness.domain", "harness.domain.common"),
        ("harness.config", "yaml"),
        ("harness.config", "idna"),
        ("harness.runtime", "harness.domain.memory"),
        ("harness.store", "aiosqlite"),
        ("harness.store", "sqlite3"),
        ("harness.store", "harness.persistence.sqlite_schema"),
        ("harness.models", "langchain_openai"),
        ("harness.models", "harness.tools.response_limit"),
        ("harness.tools", "mcp"),
        ("harness.agents", "langgraph"),
        ("extension_api", "pydantic"),
        ("extension_api", "harness.domain"),
        ("app", "fastapi"),
        ("app", "uvicorn"),
        ("app", "harness.config"),
        ("app", "harness.agents.companion"),
    ],
)
def test_future_layers_allow_role_specific_dependencies(
    tmp_path: Path, layer: str, module: str
) -> None:
    if layer == "extension_api":
        relative = "extension-api/src/extension_api/contracts.py"
    elif layer == "app":
        relative = "app/src/app/gateway/app.py"
    else:
        relative = f"harness/src/{layer.replace('.', '/')}/__init__.py"
    roots = _write_fake_tree(tmp_path, {relative: f"import {module}\n"})
    assert _boundary_errors(roots) == []


def test_unknown_third_party_dependency_is_rejected_by_default(tmp_path: Path) -> None:
    roots = _write_fake_tree(
        tmp_path,
        {"harness/src/harness/domain/__init__.py": "import requests\n"},
    )
    errors = _boundary_errors(roots)
    assert any(
        "forbidden external dependency harness.domain->requests" in error for error in errors
    )


@pytest.mark.parametrize(
    "relative", ["app/src/app/gateway/cli.py", "harness/src/harness/agents/__init__.py"]
)
def test_composition_layers_are_the_only_dynamic_import_seams(
    tmp_path: Path, relative: str
) -> None:
    roots = _write_fake_tree(
        tmp_path,
        {relative: 'from importlib import import_module\nimport_module("harness.domain")\n'},
    )
    assert _boundary_errors(roots) == []


@pytest.mark.parametrize("relative", ["__escape.py", "__escape/__init__.py"])
def test_scanner_rejects_empty_unknown_dunder_layers(tmp_path: Path, relative: str) -> None:
    roots = _write_fake_tree(tmp_path, {f"harness/src/harness/{relative}": ""})
    errors = _boundary_errors(roots)
    assert any("unknown package layer" in error for error in errors), errors


@pytest.mark.parametrize("module", ["harness", "extension_api"])
def test_lightweight_modules_do_not_eager_import_runtime_dependencies(module: str) -> None:
    script = (
        "import importlib, sys; "
        f"importlib.import_module({module!r}); "
        f"forbidden=sorted(set(sys.modules) & {HEAVY_IMPORTS!r}); "
        "raise SystemExit('eager imports: '+','.join(forbidden) if forbidden else 0)"
    )
    completed = subprocess.run(
        [sys.executable, "-c", script],
        cwd=AGENT_ROOT,
        capture_output=True,
        text=True,
        check=False,
    )
    assert completed.returncode == 0, completed.stderr or completed.stdout


def test_package_identity_does_not_contain_legacy_names() -> None:
    package = importlib.import_module("harness")
    assert package.__name__ == "harness"
    assert "mcgo" not in (package.__doc__ or "").lower()
    assert "mcgod" not in (package.__doc__ or "").lower()
