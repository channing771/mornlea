"""Mornlea 伙伴 Agent 服务命令行入口。"""

from __future__ import annotations

import argparse
from collections.abc import Sequence
from importlib import import_module
from typing import TYPE_CHECKING, Protocol, cast

from harness import __version__

if TYPE_CHECKING:
    from harness.config import AgentConfig, ResolvedSecrets


class ServeRunner(Protocol):
    def __call__(self, config: AgentConfig, secrets: ResolvedSecrets) -> int: ...


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="mornlea-companion-agent",
        description="Mornlea companion Agent service",
    )
    parser.add_argument("--version", action="version", version=f"%(prog)s {__version__}")
    commands = parser.add_subparsers(dest="command", required=True)
    serve = commands.add_parser("serve", help="启动单 worker Agent 服务")
    serve.add_argument("--config", required=True, help="严格 v1 YAML 配置路径")
    return parser


def _load_serve_runner() -> ServeRunner:
    """延迟导入 app，避免帮助和版本命令初始化服务依赖。"""

    try:
        module = import_module("app.gateway.app")
    except ImportError as error:
        raise RuntimeError("Agent HTTP application is not installed yet") from error
    runner = vars(module).get("serve")
    if not callable(runner):
        raise RuntimeError("Agent HTTP application does not expose serve")
    return cast(ServeRunner, runner)


def main(argv: Sequence[str] | None = None, *, serve_runner: ServeRunner | None = None) -> int:
    parser = build_parser()
    try:
        arguments = parser.parse_args(argv)
    except SystemExit as error:
        if error.code == 0:
            return 0
        raise
    if arguments.command == "serve":
        from harness.config import (
            ConfigError,
            load_config,
            resolve_config_path,
            resolve_secrets,
        )

        try:
            config = load_config(resolve_config_path(arguments.config))
            secrets = resolve_secrets(config)
        except ConfigError as error:
            parser.error(str(error))
        runner = serve_runner if serve_runner is not None else _load_serve_runner()
        return runner(config, secrets)
    parser.error("unknown command")
    return 2


def entrypoint() -> None:
    raise SystemExit(main())


__all__ = ["ServeRunner", "build_parser", "entrypoint", "main"]
