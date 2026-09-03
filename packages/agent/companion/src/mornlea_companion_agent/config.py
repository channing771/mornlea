"""伙伴 Agent 服务的严格启动配置。"""

from __future__ import annotations

import os
import re
import unicodedata
from collections.abc import Mapping
from ipaddress import IPv4Address, IPv6Address, ip_address
from pathlib import Path
from typing import Literal
from urllib.parse import urlsplit

import idna
import yaml
from pydantic import (
    BaseModel,
    ConfigDict,
    Field,
    SecretStr,
    StrictInt,
    StrictStr,
    ValidationError,
    ValidationInfo,
    field_validator,
)

_ENV_NAME = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
_URI_SCHEME = re.compile(r"^[A-Za-z][A-Za-z0-9+.-]*:")
_DNS_LABEL = re.compile(r"^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$")


class ConfigError(ValueError):
    """表示配置无效，消息只描述字段而不包含 secret。"""


class _StrictFrozenModel(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)


def _has_control(value: str) -> bool:
    return any(unicodedata.category(character) == "Cc" for character in value)


def _validate_nonempty_text(value: str, *, field: str) -> str:
    if not value or value != value.strip() or _has_control(value):
        raise ValueError(f"{field} must be non-empty text without edge whitespace or controls")
    try:
        value.encode("utf-8")
    except UnicodeEncodeError as error:
        raise ValueError(f"{field} must be valid UTF-8") from error
    return value


def _validate_env_name(value: str, *, field: str) -> str:
    _validate_nonempty_text(value, field=field)
    if _ENV_NAME.fullmatch(value) is None:
        raise ValueError(f"{field} must be a valid environment variable name")
    return value


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


class StorageConfig(_StrictFrozenModel):
    sqlite_path: Path

    @field_validator("sqlite_path", mode="before")
    @classmethod
    def validate_sqlite_path(cls, value: object, info: ValidationInfo) -> Path:
        if type(value) is not str:
            raise ValueError("storage.sqlite_path must be a persistent file path")
        text = _validate_nonempty_text(value, field="storage.sqlite_path")
        if text == ":memory:" or _URI_SCHEME.match(text) or text.endswith(("/", "\\")):
            raise ValueError(
                "storage.sqlite_path must be a persistent file path, not memory, URI, or directory"
            )
        if "\x00" in text:
            raise ValueError("storage.sqlite_path must not contain NUL")
        context = info.context if isinstance(info.context, dict) else {}
        config_dir = context.get("config_dir")
        if not isinstance(config_dir, Path):
            raise ValueError("storage.sqlite_path requires a configuration directory")
        candidate = Path(text)
        if not candidate.is_absolute():
            candidate = config_dir / candidate
        try:
            resolved = candidate.resolve(strict=False)
            if resolved.exists() and not resolved.is_file():
                raise ValueError("storage.sqlite_path must name a file, not a directory")
        except (OSError, RuntimeError) as error:
            raise ValueError(
                "storage.sqlite_path cannot be resolved as a persistent file"
            ) from error
        return resolved


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


class LimitConfig(_StrictFrozenModel):
    model_calls: StrictInt = Field(default=3, ge=1, le=5)
    tool_calls: StrictInt = Field(default=4, ge=1, le=8)
    timeout_seconds: StrictInt = Field(default=30, ge=1, le=60)


class AgentConfig(_StrictFrozenModel):
    config_version: Literal["v1"]
    http: HTTPConfig
    storage: StorageConfig
    provider: ProviderConfig
    limits: LimitConfig = LimitConfig()


class ResolvedSecrets(_StrictFrozenModel):
    http_bearer_token: SecretStr
    provider_api_key: SecretStr


class _UniqueKeyLoader(yaml.SafeLoader):
    pass


def _construct_unique_mapping(
    loader: _UniqueKeyLoader, node: yaml.MappingNode, deep: bool = False
) -> dict[object, object]:
    loader.flatten_mapping(node)
    result: dict[object, object] = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=deep)
        try:
            duplicate = key in result
        except TypeError as error:
            raise yaml.constructor.ConstructorError(
                "while constructing a mapping",
                node.start_mark,
                "found an unhashable key",
                key_node.start_mark,
            ) from error
        if duplicate:
            raise yaml.constructor.ConstructorError(
                "while constructing a mapping",
                node.start_mark,
                f"found duplicate key {key!r}",
                key_node.start_mark,
            )
        result[key] = loader.construct_object(value_node, deep=deep)
    return result


_UniqueKeyLoader.add_constructor(
    yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG,
    _construct_unique_mapping,
)


def _format_validation_fields(error: ValidationError) -> str:
    fields = []
    for item in error.errors(include_url=False, include_context=False, include_input=False):
        fields.append(".".join(str(part) for part in item["loc"]))
    return ", ".join(dict.fromkeys(fields)) or "root"


def load_config(path: str | os.PathLike[str]) -> AgentConfig:
    """加载一个 YAML mapping，并相对配置文件解析持久数据库路径。"""

    try:
        config_path = Path(path).expanduser().resolve(strict=False)
        raw_bytes = config_path.read_bytes()
        document = raw_bytes.decode("utf-8", errors="strict")
        raw = yaml.load(document, Loader=_UniqueKeyLoader)
    except (OSError, RuntimeError, UnicodeError, yaml.YAMLError):
        raise ConfigError("unable to load configuration: invalid path, YAML, or UTF-8") from None
    if type(raw) is not dict:
        raise ConfigError("configuration root must be a single YAML mapping")
    try:
        return AgentConfig.model_validate(raw, context={"config_dir": config_path.parent})
    except ValidationError as error:
        raise ConfigError(
            f"invalid configuration fields: {_format_validation_fields(error)}"
        ) from None


def _secret_from_environment(environment: Mapping[str, str], name: str) -> SecretStr:
    value = environment.get(name)
    if (
        type(value) is not str
        or not value
        or any(character.isspace() for character in value)
        or _has_control(value)
    ):
        raise ConfigError(
            f"environment variable {name} must contain a non-empty header-safe secret"
        )
    try:
        value.encode("ascii", errors="strict")
    except UnicodeEncodeError:
        raise ConfigError(
            f"environment variable {name} must contain an ASCII header-safe secret"
        ) from None
    return SecretStr(value)


def resolve_secrets(
    config: AgentConfig,
    environment: Mapping[str, str] | None = None,
) -> ResolvedSecrets:
    """在结构校验完成后解析 secret，错误与 repr 均不暴露 secret 值。"""

    source = os.environ if environment is None else environment
    return ResolvedSecrets(
        http_bearer_token=_secret_from_environment(source, config.http.bearer_token_env),
        provider_api_key=_secret_from_environment(source, config.provider.api_key_env),
    )


__all__ = [
    "AgentConfig",
    "ConfigError",
    "HTTPConfig",
    "LimitConfig",
    "ProviderConfig",
    "ResolvedSecrets",
    "StorageConfig",
    "load_config",
    "resolve_secrets",
]
