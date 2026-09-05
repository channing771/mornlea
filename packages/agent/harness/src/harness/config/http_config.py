"""伙伴 Agent 服务 HTTP 绑定配置：仅限 loopback，单 worker。"""

from __future__ import annotations

from ipaddress import IPv4Address, IPv6Address, ip_address
from typing import Literal

from pydantic import Field, StrictInt, StrictStr, field_validator

from .app_config import _StrictFrozenModel, _validate_env_name, _validate_nonempty_text


class HTTPConfig(_StrictFrozenModel):
    bind: IPv4Address | IPv6Address
    port: StrictInt = Field(ge=1, le=65535)
    workers: Literal[1]
    bearer_token_env: StrictStr

    @field_validator("bind", mode="before")
    @classmethod
    def validate_bind(cls, value: object) -> IPv4Address | IPv6Address:
        if type(value) is not str:
            raise ValueError("http.bind must be a loopback IP literal")
        text = _validate_nonempty_text(value, field="http.bind")
        try:
            parsed = ip_address(text)
        except ValueError as error:
            raise ValueError("http.bind must be a loopback IP literal") from error
        if not parsed.is_loopback:
            raise ValueError("http.bind must be a loopback IP literal")
        return parsed

    @field_validator("workers", mode="before")
    @classmethod
    def validate_workers(cls, value: object) -> int:
        if type(value) is not int or value != 1:
            raise ValueError("http.workers must be the integer 1")
        return value

    @field_validator("bearer_token_env")
    @classmethod
    def validate_bearer_token_env(cls, value: str) -> str:
        return _validate_env_name(value, field="http.bearer_token_env")
