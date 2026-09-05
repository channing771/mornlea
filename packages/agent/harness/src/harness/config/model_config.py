"""伙伴 Agent 服务模型供应商配置：地址、模型名与密钥环境变量名。"""

from __future__ import annotations

from ipaddress import ip_address
from urllib.parse import urlsplit

import idna
from pydantic import StrictStr, field_validator

from .app_config import (
    _DNS_LABEL,
    _StrictFrozenModel,
    _validate_env_name,
    _validate_nonempty_text,
)


class ProviderConfig(_StrictFrozenModel):
    base_url: StrictStr
    model: StrictStr
    api_key_env: StrictStr

    @field_validator("base_url")
    @classmethod
    def validate_base_url(cls, value: str) -> str:
        _validate_nonempty_text(value, field="provider.base_url")
        if "\\" in value:
            raise ValueError("provider.base_url must not contain backslash")
        try:
            parsed = urlsplit(value)
            parsed_port = parsed.port
        except ValueError as error:
            raise ValueError("provider.base_url must be a valid HTTP(S) URL") from error
        if parsed.scheme not in {"http", "https"}:
            raise ValueError("provider.base_url must use http or https")
        if parsed.hostname is None or parsed.username is not None or parsed.password is not None:
            raise ValueError("provider.base_url must have a host and no userinfo")
        if any(character.isspace() for character in parsed.netloc):
            raise ValueError("provider.base_url authority must not contain whitespace")
        if parsed_port is not None and not 1 <= parsed_port <= 65535:
            raise ValueError("provider.base_url has an invalid port")
        if "?" in value or "#" in value:
            raise ValueError("provider.base_url must not contain query or fragment")
        if parsed.netloc.startswith("["):
            try:
                ip_address(parsed.hostname)
            except ValueError as error:
                raise ValueError(
                    "provider.base_url bracketed hostname must be a valid IP literal"
                ) from error
        cls.validate_provider_host(parsed.hostname)
        return value

    @classmethod
    def validate_provider_host(cls, host: str) -> None:
        try:
            ip_address(host)
            return
        except ValueError:
            pass
        unicode_labels = host.split(".")
        if len(unicode_labels) == 4 and all(
            label.isascii() and label.isdecimal() for label in unicode_labels
        ):
            raise ValueError("provider.base_url hostname is an invalid IPv4 address")
        try:
            ascii_host = idna.encode(host.lower()).decode("ascii")
        except idna.IDNAError as error:
            raise ValueError("provider.base_url hostname must be valid IDNA2008") from error
        dns_host = ascii_host[:-1] if ascii_host.endswith(".") else ascii_host
        if not dns_host or len(dns_host) > 253:
            raise ValueError("provider.base_url hostname is too long")
        labels = dns_host.split(".")
        if any(_DNS_LABEL.fullmatch(label) is None for label in labels):
            raise ValueError("provider.base_url hostname contains an invalid DNS label")

    @field_validator("model")
    @classmethod
    def validate_model(cls, value: str) -> str:
        return _validate_nonempty_text(value, field="provider.model")

    @field_validator("api_key_env")
    @classmethod
    def validate_api_key_env(cls, value: str) -> str:
        return _validate_env_name(value, field="provider.api_key_env")
