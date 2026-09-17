from __future__ import annotations

import argparse
import ast
import io
import re
import tokenize
from collections.abc import Sequence
from pathlib import Path

EXCLUDED_PARTS = frozenset({".godot", ".venv", "__pycache__", "addons"})
CJK_PATTERN = re.compile(r"[\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff]")
NETWORK_MODULES = frozenset(
    {"aiohttp", "ftplib", "http", "requests", "socket", "urllib", "websockets"}
)
NETWORK_GODOT_CLASSES = frozenset(
    {"HTTPRequest", "HTTPClient", "PacketPeer", "StreamPeerTCP", "WebSocketPeer"}
)
NATIVE_ABI_MODULES = frozenset({"_ctypes", "cffi", "ctypes"})
INSTALLER_MODULES = frozenset({"ensurepip", "pip", "uv"})
PROCESS_MODULES = frozenset({"subprocess"})

# Client-core wire magics from the frozen ABI header: the wire-text tags and
# their little-endian 32-bit words. Production Godot scripts must never carry
# them; the protocol stays owned by Go and Rust.
WIRE_MAGIC_TEXTS = frozenset({"MCI1", "MCC1", "MCN1", "MCS1", "MCW1", "MCF1", "MCM1"})
WIRE_MAGIC_WORDS = frozenset(
    {0x3149434D, 0x3143434D, 0x314E434D, 0x3153434D, 0x3157434D, 0x3146434D, 0x314D434D}
)

# Bridge session and pull methods belong to feature data-plane ownership; the
# host binds and forwards the bridge object but never invokes these itself.
HOST_GAMEPLAY_BRIDGE_METHODS = frozenset(
    {
        "session_create",
        "session_connect",
        "session_poll",
        "session_submit",
        "session_step",
        "session_close",
        "pull_world",
        "pull_frame",
        "pull_status",
        "pull_identity",
    }
)

# Concrete feature identities and entry trees declared by the production
# catalog; a host file naming one has stopped being catalog driven.
HOST_FEATURE_PATH_PREFIXES = ("res://features/", "res://platform/")
HOST_FEATURE_IDS = frozenset(
    {
        "session",
        "player_view",
        "world",
        "actors",
        "ui",
        "platform.desktop.lifecycle",
        "platform.desktop.input",
        "platform.desktop.audio",
    }
)

# Script scopes that carry stronger discipline than the general boundary.
PRODUCTION_EXEMPT_PREFIXES = ("tests/", "typing/")
HOST_SCRIPT_PREFIXES = ("app/host/", "app/bootstrap/")


class Finding:
    def __init__(self, path: str, line: int, rule: str, detail: str) -> None:
        self.path = path
        self.line = line
        self.rule = rule
        self.detail = detail

    def sort_key(self) -> tuple[str, int, str, str]:
        return (self.path, self.line, self.rule, self.detail)


def _matches_prefix(module: str, prefix: str) -> bool:
    return module == prefix or module.startswith(prefix + ".")


def _module_rule(module: str) -> tuple[str, str] | None:
    if _matches_prefix(module, "packages.agent"):
        return ("forbidden-agent-import", module)
    if any(
        _matches_prefix(module, prefix)
        for prefix in (
            "mornlea_client_core",
            "mornlea_engine",
            "packages.shared.nativeabi",
        )
    ):
        return ("forbidden-native-abi", module)
    root = module.split(".", 1)[0]
    if root in NATIVE_ABI_MODULES:
        return ("forbidden-native-abi", module)
    if root in NETWORK_MODULES or any(
        part in NETWORK_GODOT_CLASSES for part in module.split(".")
    ):
        return ("forbidden-network", module)
    if root in INSTALLER_MODULES:
        return ("forbidden-runtime-installer", module)
    if root in PROCESS_MODULES:
        return ("forbidden-process-execution", module)
    return None


class BoundaryVisitor(ast.NodeVisitor):
    def __init__(self, path: str, production: bool, host: bool) -> None:
        self.path = path
        self.production = production
        self.host = host
        self.findings: list[Finding] = []

    def _add(self, node: ast.AST, rule: str, detail: str) -> None:
        self.findings.append(
            Finding(self.path, getattr(node, "lineno", 1), rule, detail)
        )

    def _check_module(self, node: ast.AST, module: str) -> None:
        result = _module_rule(module)
        if result is not None:
            self._add(node, result[0], result[1])

    def visit_Import(self, node: ast.Import) -> None:
        for alias in node.names:
            self._check_module(node, alias.name)
        self.generic_visit(node)

    def visit_ImportFrom(self, node: ast.ImportFrom) -> None:
        module = node.module or ""
        self._check_module(node, module)
        if self.production and module == "struct":
            # Importing pack/unpack directly is the same wire-codec surface as
            # calling struct.pack; production scripts never encode binary
            # records, so the import itself is the violation.
            self._add(
                node,
                "forbidden-wire-record-codec",
                f"from struct import {[alias.name for alias in node.names]}",
            )
        for alias in node.names:
            self._check_module(
                node, ".".join(part for part in (module, alias.name) if part)
            )
        self.generic_visit(node)

    def visit_Constant(self, node: ast.Constant) -> None:
        if self.production:
            if isinstance(node.value, str) and any(
                magic in node.value for magic in WIRE_MAGIC_TEXTS
            ):
                self._add(node, "forbidden-protocol-magic", node.value)
            if (
                isinstance(node.value, int)
                and not isinstance(node.value, bool)
                and node.value in WIRE_MAGIC_WORDS
            ):
                self._add(node, "forbidden-protocol-magic", hex(node.value))
        if self.host and isinstance(node.value, str):
            if node.value in HOST_GAMEPLAY_BRIDGE_METHODS:
                self._add(node, "host-gameplay-bridge-call", node.value)
            if node.value.startswith(HOST_FEATURE_PATH_PREFIXES) or node.value in (
                HOST_FEATURE_IDS
            ):
                self._add(node, "host-feature-reference", node.value)
        self.generic_visit(node)

    def visit_Call(self, node: ast.Call) -> None:
        if isinstance(node.func, ast.Name) and node.func.id == "__import__":
            self._add(node, "unrestricted-dynamic-import", "__import__")
        elif isinstance(node.func, ast.Attribute):
            owner = node.func.value
            owner_name = owner.id if isinstance(owner, ast.Name) else ""
            if owner_name == "importlib" and node.func.attr == "import_module":
                self._add(
                    node, "unrestricted-dynamic-import", "importlib.import_module"
                )
            if owner_name == "pkgutil" and node.func.attr == "resolve_name":
                self._add(node, "unrestricted-dynamic-import", "pkgutil.resolve_name")
            if node.func.attr in {"CDLL", "LoadLibrary", "PyDLL", "WinDLL", "dlopen"}:
                self._add(node, "forbidden-native-abi", node.func.attr)
            if owner_name == "os" and (
                node.func.attr in {"popen", "system"}
                or node.func.attr.startswith("exec")
                or node.func.attr.startswith("spawn")
            ):
                self._add(node, "forbidden-process-execution", f"os.{node.func.attr}")
            if (
                self.production
                and owner_name == "struct"
                and node.func.attr in {"pack", "unpack", "Struct"}
            ):
                # Struct objects are included because their pack/unpack methods
                # encode the same binary wire records.
                self._add(
                    node, "forbidden-wire-record-codec", f"struct.{node.func.attr}"
                )
        self.generic_visit(node)


def _source_paths(root: Path) -> list[Path]:
    paths: list[Path] = []
    for path in root.rglob("*"):
        if path.suffix not in {".py", ".pyi"} or not path.is_file():
            continue
        relative = path.relative_to(root)
        if any(part in EXCLUDED_PARTS for part in relative.parts):
            continue
        paths.append(path)
    return sorted(paths)


def _comment_findings(path: str, source: str) -> list[Finding]:
    findings: list[Finding] = []
    try:
        tokens = tokenize.generate_tokens(io.StringIO(source).readline)
        for token in tokens:
            if token.type == tokenize.COMMENT and CJK_PATTERN.search(token.string):
                findings.append(
                    Finding(path, token.start[0], "non-english-comment", token.string)
                )
    except tokenize.TokenError as error:
        line = error.args[1][0] if len(error.args) > 1 else 1
        findings.append(Finding(path, line, "syntax-error", str(error.args[0])))
    return findings


def scan_project(root: Path) -> list[Finding]:
    root = root.resolve()
    findings: list[Finding] = []
    for source_path in _source_paths(root):
        relative = source_path.relative_to(root).as_posix()
        production = not relative.startswith(PRODUCTION_EXEMPT_PREFIXES)
        host = relative.startswith(HOST_SCRIPT_PREFIXES)
        source = source_path.read_text(encoding="utf-8")
        findings.extend(_comment_findings(relative, source))
        try:
            tree = ast.parse(source, filename=relative)
        except SyntaxError as error:
            findings.append(
                Finding(relative, error.lineno or 1, "syntax-error", error.msg)
            )
            continue
        visitor = BoundaryVisitor(relative, production, host)
        visitor.visit(tree)
        findings.extend(visitor.findings)
    return sorted(findings, key=Finding.sort_key)


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--project-root", required=True, type=Path)
    arguments = parser.parse_args(argv)
    findings = scan_project(arguments.project_root)
    for finding in findings:
        print(f"{finding.path}:{finding.line}: {finding.rule}: {finding.detail}")
    return 1 if findings else 0


if __name__ == "__main__":
    raise SystemExit(main())
