from __future__ import annotations

import ast
import importlib
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

import pytest

SOURCE_ROOT = Path(__file__).resolve().parents[1] / "src"
PACKAGE_ROOT = SOURCE_ROOT / "mornlea_companion_agent"
PACKAGE_NAME = "mornlea_companion_agent"
KNOWN_LAYERS = {"root", "cli", "config", "domain", "harness", "adapters", "storage", "app"}
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
    "sqlalchemy",
    "sqlite3",
    "starlette",
    "uvicorn",
}
DYNAMIC_SEAM_LAYERS = {"cli", "app"}
LOCAL_ALLOWED = {
    "root": set(),
    "domain": {"domain"},
    "config": set(),
    "harness": {"domain", "harness"},
    "adapters": {"domain", "adapters"},
    "storage": {"domain", "storage"},
    "app": {"config", "domain", "harness", "adapters", "storage", "app"},
    "cli": {"root", "config", "app", "cli"},
}
THIRD_PARTY_ALLOWED = {
    "root": set(),
    "cli": set(),
    "config": {"idna", "pydantic", "yaml"},
    "domain": {"pydantic"},
    "harness": {"langchain_core", "langgraph", "pydantic"},
    "adapters": {
        "httpx",
        "langchain_core",
        "langchain_mcp_adapters",
        "langchain_openai",
        "mcp",
        "pydantic",
    },
    "storage": {"aiosqlite", "pydantic"},
    "app": {"fastapi", "pydantic", "starlette", "uvicorn"},
}


@dataclass(frozen=True)
class ImportEdge:
    source: str
    target: str
    line: int


def _module_name(path: Path, package_root: Path = PACKAGE_ROOT) -> str:
    relative = path.relative_to(package_root).with_suffix("")
    parts = relative.parts
    if parts[-1] == "__init__":
        parts = parts[:-1]
    return ".".join((PACKAGE_NAME, *parts)) if parts else PACKAGE_NAME


def _resolve_from(source: str, level: int, module: str | None, *, is_package: bool) -> str:
    source_parts = source.split(".")
    package_parts = source_parts if is_package else source_parts[:-1]
    if level == 0:
        return module or ""
    keep = len(package_parts) - (level - 1)
    if keep < 1:
        return "<invalid-relative-import>"
    base = package_parts[:keep]
    return ".".join((*base, *(module.split(".") if module else ())))


def _dynamic_surface_errors(tree: ast.AST, source: str) -> list[str]:
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


def _scan(package_root: Path = PACKAGE_ROOT) -> tuple[list[ImportEdge], list[str]]:
    edges: list[ImportEdge] = []
    errors: list[str] = []
    for path in sorted(package_root.rglob("*.py")):
        source = _module_name(path, package_root)
        try:
            tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
        except (SyntaxError, UnicodeError) as error:
            errors.append(f"{path}:{getattr(error, 'lineno', 0)}: {error}")
            continue
        is_package = path.name == "__init__.py"
        errors.extend(_dynamic_surface_errors(tree, source))
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                edges.extend(ImportEdge(source, alias.name, node.lineno) for alias in node.names)
            elif isinstance(node, ast.ImportFrom):
                target = _resolve_from(source, node.level, node.module, is_package=is_package)
                edges.append(ImportEdge(source, target, node.lineno))
    return edges, errors


def _layer(module: str) -> str:
    if module == PACKAGE_NAME:
        return "root"
    if not module.startswith(f"{PACKAGE_NAME}."):
        return "external"
    first = module.split(".", 2)[1]
    if first in {"__main__", "cli"}:
        return "cli"
    if first in KNOWN_LAYERS:
        return first
    return f"unknown:{first}"


def _boundary_errors(package_root: Path = PACKAGE_ROOT) -> list[str]:
    edges, errors = _scan(package_root)
    for edge in edges:
        source_layer = _layer(edge.source)
        target_layer = _layer(edge.target)
        if source_layer.startswith("unknown:"):
            errors.append(f"{edge.source}: unknown source layer")
            continue
        if target_layer.startswith("unknown:"):
            errors.append(f"{edge.source}:{edge.line}: unknown target layer {edge.target}")
            continue
        target_root = edge.target.split(".", 1)[0]
        if target_layer == "external":
            if (
                target_root not in sys.stdlib_module_names
                and target_root not in THIRD_PARTY_ALLOWED[source_layer]
            ):
                errors.append(
                    f"{edge.source}:{edge.line}: forbidden external dependency "
                    f"{source_layer}->{edge.target}"
                )
            continue
        if target_layer not in LOCAL_ALLOWED[source_layer]:
            errors.append(f"{edge.source}:{edge.line}: forbidden {source_layer}->{target_layer}")
    for path in sorted(package_root.iterdir()):
        if path.name in {"__init__.py", "__main__.py", "__pycache__"}:
            continue
        name = path.stem if path.is_file() else path.name
        if name not in {"cli", "config", "domain", "harness", "adapters", "storage", "app"}:
            errors.append(f"{path}: unknown package layer")
    return errors


def test_package_import_boundaries_are_acyclic_and_known() -> None:
    assert _boundary_errors() == []


def test_scanner_resolves_relative_imports_and_rejects_forbidden_edges(tmp_path: Path) -> None:
    package = tmp_path / PACKAGE_NAME
    (package / "domain").mkdir(parents=True)
    (package / "domain/__init__.py").write_text(
        "from ..config import AgentConfig\n", encoding="utf-8"
    )
    (package / "config.py").write_text("AgentConfig = object\n", encoding="utf-8")
    errors = _boundary_errors(package)
    assert any("forbidden domain->config" in error for error in errors)


def test_scanner_fails_closed_on_syntax_errors_and_unknown_layers(tmp_path: Path) -> None:
    package = tmp_path / PACKAGE_NAME
    package.mkdir()
    (package / "mystery.py").write_text("if (\n", encoding="utf-8")
    errors = _boundary_errors(package)
    assert any("SyntaxError" in error or "was never closed" in error for error in errors)
    assert any("unknown package layer" in error for error in errors)


def test_scanner_rejects_dynamic_boundary_bypasses(tmp_path: Path) -> None:
    package = tmp_path / PACKAGE_NAME
    (package / "domain").mkdir(parents=True)
    (package / "domain/__init__.py").write_text(
        'from importlib import import_module\nimport_module("fastapi")\n',
        encoding="utf-8",
    )
    errors = _boundary_errors(package)
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
    package = tmp_path / PACKAGE_NAME
    (package / "domain").mkdir(parents=True)
    (package / "domain/__init__.py").write_text(source, encoding="utf-8")
    errors = _boundary_errors(package)
    assert any("dynamic import surface" in error for error in errors), errors


@pytest.mark.parametrize(
    ("layer", "source"),
    [
        (
            "domain",
            "from functools import partial\n"
            "from importlib import import_module\n"
            'load = partial(import_module, "fastapi")\n'
            "load()\n",
        ),
        ("config", 'load = (__import__,)[0]\nload("fastapi")\n'),
        ("harness", "def load(loader=__import__):\n    return loader('fastapi')\n"),
    ],
)
def test_protected_layers_forbid_dynamic_import_surfaces(
    tmp_path: Path, layer: str, source: str
) -> None:
    package = tmp_path / PACKAGE_NAME
    target = package / (f"{layer}.py" if layer == "config" else f"{layer}/__init__.py")
    target.parent.mkdir(parents=True)
    target.write_text(source, encoding="utf-8")
    errors = _boundary_errors(package)
    assert any("dynamic import surface" in error for error in errors), errors


@pytest.mark.parametrize(
    ("layer", "module"),
    [
        ("harness", "fastapi"),
        ("harness", "uvicorn"),
        ("harness", "starlette"),
        ("harness", "mornlea_companion_agent.adapters"),
        ("harness", "mornlea_companion_agent.storage"),
        ("harness", "langchain_openai"),
        ("harness", "mcp"),
        ("adapters", "aiosqlite"),
        ("adapters", "fastapi"),
        ("adapters", "mornlea_companion_agent.storage"),
        ("storage", "httpx"),
        ("storage", "langgraph"),
        ("storage", "mornlea_companion_agent.adapters"),
    ],
)
def test_future_layers_reject_out_of_role_dependencies(
    tmp_path: Path, layer: str, module: str
) -> None:
    package = tmp_path / PACKAGE_NAME
    target = package / f"{layer}/__init__.py"
    target.parent.mkdir(parents=True)
    target.write_text(f"import {module}\n", encoding="utf-8")
    errors = _boundary_errors(package)
    assert any("forbidden" in error for error in errors), errors


@pytest.mark.parametrize(
    ("layer", "module"),
    [
        ("harness", "langgraph"),
        ("harness", "langchain_core"),
        ("harness", "mornlea_companion_agent.domain"),
        ("adapters", "httpx"),
        ("adapters", "langchain_openai"),
        ("adapters", "langchain_mcp_adapters"),
        ("adapters", "mcp"),
        ("storage", "aiosqlite"),
        ("storage", "sqlite3"),
    ],
)
def test_future_layers_allow_role_specific_dependencies(
    tmp_path: Path, layer: str, module: str
) -> None:
    package = tmp_path / PACKAGE_NAME
    target = package / f"{layer}/__init__.py"
    target.parent.mkdir(parents=True)
    target.write_text(f"import {module}\n", encoding="utf-8")
    assert _boundary_errors(package) == []


def test_unknown_third_party_dependency_is_rejected_by_default(tmp_path: Path) -> None:
    package = tmp_path / PACKAGE_NAME
    (package / "harness").mkdir(parents=True)
    (package / "harness/__init__.py").write_text("import requests\n", encoding="utf-8")
    errors = _boundary_errors(package)
    assert any("forbidden external dependency harness->requests" in error for error in errors)


@pytest.mark.parametrize("relative", ["cli.py", "app.py"])
def test_composition_layers_are_the_only_dynamic_import_seams(
    tmp_path: Path, relative: str
) -> None:
    package = tmp_path / PACKAGE_NAME
    package.mkdir(parents=True)
    (package / relative).write_text(
        'from importlib import import_module\nimport_module("mornlea_companion_agent.app")\n',
        encoding="utf-8",
    )
    assert _boundary_errors(package) == []


@pytest.mark.parametrize("relative", ["__escape.py", "__escape/__init__.py"])
def test_scanner_rejects_empty_unknown_dunder_layers(tmp_path: Path, relative: str) -> None:
    package = tmp_path / PACKAGE_NAME
    target = package / relative
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text("", encoding="utf-8")
    errors = _boundary_errors(package)
    assert any("unknown package layer" in error for error in errors), errors


@pytest.mark.parametrize(
    "module",
    ["mornlea_companion_agent", "mornlea_companion_agent.domain", "mornlea_companion_agent.config"],
)
def test_lightweight_modules_do_not_eager_import_runtime_dependencies(module: str) -> None:
    script = (
        "import importlib, sys; "
        f"importlib.import_module({module!r}); "
        f"forbidden=sorted(set(sys.modules) & {HEAVY_IMPORTS!r}); "
        "raise SystemExit('eager imports: '+','.join(forbidden) if forbidden else 0)"
    )
    completed = subprocess.run(
        [sys.executable, "-c", script],
        cwd=SOURCE_ROOT.parent,
        capture_output=True,
        text=True,
        check=False,
    )
    assert completed.returncode == 0, completed.stderr or completed.stdout


def test_package_identity_does_not_contain_legacy_names() -> None:
    package = importlib.import_module(PACKAGE_NAME)
    assert package.__name__ == "mornlea_companion_agent"
    assert "mcgo" not in (package.__doc__ or "").lower()
    assert "mcgod" not in (package.__doc__ or "").lower()
