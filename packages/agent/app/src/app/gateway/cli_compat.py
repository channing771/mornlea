"""旧别名兼容入口：提示弃用后转调新命令。"""

from __future__ import annotations

import sys
from collections.abc import Sequence

from app.gateway.cli import main as _gateway_main


def main(argv: Sequence[str] | None = None) -> int:
    """输出中文弃用提示后按原参数转调新命令，保持退出码不变。"""

    print("警告：`mornlea-companion-agent` 已弃用，请改用 `mornlea-agent`。", file=sys.stderr)
    return _gateway_main(argv)


def entrypoint() -> None:
    raise SystemExit(main())


__all__ = ["entrypoint", "main"]
