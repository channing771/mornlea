"""伙伴 Agent 服务调用预算配置：默认值与硬上限。"""

from __future__ import annotations

from pydantic import Field, StrictInt

from .app_config import _StrictFrozenModel


class LimitConfig(_StrictFrozenModel):
    model_calls: StrictInt = Field(default=3, ge=1, le=5)
    tool_calls: StrictInt = Field(default=4, ge=1, le=8)
    timeout_seconds: StrictInt = Field(default=30, ge=1, le=60)
