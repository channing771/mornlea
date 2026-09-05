"""智能体装配包根（懒导出，避免包根导入拉起 graph 依赖）。"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from harness.agents.companion import DialogueHarness, PlannerHarness

__all__ = ["DialogueHarness", "PlannerHarness"]


def __getattr__(name: str) -> Any:
    """透过伙伴工厂包按需导出，保持包根导入轻量。"""

    if name not in __all__:
        raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
    from harness.agents import companion as companion_package

    return getattr(companion_package, name)
