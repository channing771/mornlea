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
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                edges.extend(ImportEdge(source, alias.name, node.lineno) for alias in node.names)
            elif isinstance(node, ast.ImportFrom):
                target = _resolve_from(source, node.level, node.module, is_package=is_package)
                edges.append(ImportEdge(source, target, node.lineno))
            elif isinstance(node, ast.Call):
                dynamic_import = (
                    isinstance(node.func, ast.Name)
                    and node.func.id in {"__import__", "import_module"}
                ) or (isinstance(node.func, ast.Attribute) and node.func.attr == "import_module")
                if not dynamic_import:
                    continue
                if (
                    node.args
                    and isinstance(node.args[0], ast.Constant)
                    and isinstance(node.args[0].value, str)
                ):
                    edges.append(ImportEdge(source, node.args[0].value, node.lineno))
                else:
                    edges.append(ImportEdge(source, "<unresolved-dynamic-import>", node.lineno))
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
    allowed_local = {
        "root": set(),
        "domain": {"domain"},
        "config": set(),
        "harness": {"domain", "harness"},
        "adapters": {"domain", "adapters"},
        "storage": {"domain", "storage"},
        "app": {"config", "domain", "harness", "adapters", "storage", "app"},
        "cli": {"root", "config", "app", "cli"},
    }
    for edge in edges:
        source_layer = _layer(edge.source)
        target_layer = _layer(edge.target)
        if source_layer.startswith("unknown:"):
            errors.append(f"{edge.source}: unknown source layer")
            continue
        if target_layer.startswith("unknown:"):
            errors.append(f"{edge.source}:{edge.line}: unknown target layer {edge.target}")
            continue
        if edge.target == "<unresolved-dynamic-import>":
            errors.append(f"{edge.source}:{edge.line}: unresolved dynamic import")
            continue
        target_root = edge.target.split(".", 1)[0]
        if source_layer in {"root", "domain", "config"} and target_root in HEAVY_IMPORTS:
            errors.append(f"{edge.source}:{edge.line}: eager heavy import {edge.target}")
        if target_layer != "external" and target_layer not in allowed_local[source_layer]:
            errors.append(f"{edge.source}:{edge.line}: forbidden {source_layer}->{target_layer}")
    for path in sorted(package_root.iterdir()):
        if path.name.startswith("__"):
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
    assert any("eager heavy import fastapi" in error for error in errors)


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
