"""伙伴智能体工厂包根（懒导出，避免包根导入拉起 graph 依赖）。"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from harness.agents.companion.dialogue_factory import DialogueHarness
    from harness.agents.companion.planner_factory import PlannerHarness

__all__ = ["DialogueHarness", "PlannerHarness"]

_FACTORY_MODULES = {
    "DialogueHarness": ".dialogue_factory",
    "PlannerHarness": ".planner_factory",
}


def __getattr__(name: str) -> Any:
    """按需导入工厂模块，保持包根导入轻量。"""

    from importlib import import_module

    try:
        module_name = _FACTORY_MODULES[name]
    except KeyError:
        raise AttributeError(f"module {__name__!r} has no attribute {name!r}") from None
    return getattr(import_module(module_name, __name__), name)
