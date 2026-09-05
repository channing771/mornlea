"""伙伴 Agent 服务应用级配置：组装模型、密钥解析与配置文件寻址。"""

from __future__ import annotations

import os
import re
import unicodedata
from collections.abc import Mapping
from pathlib import Path
from typing import Literal

import yaml
from pydantic import (
    BaseModel,
    ConfigDict,
    SecretStr,
    ValidationError,
)

_ENV_NAME = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
_URI_SCHEME = re.compile(r"^[A-Za-z][A-Za-z0-9+.-]*:")
_DNS_LABEL = re.compile(r"^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$")

_CONFIG_ENV_VAR = "MORNLEA_AGENT_CONFIG"


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


# 子模块只依赖本文件上半部分共享基类与校验函数；
# 为避免循环导入，子模块导入刻意放在共享定义之后。
from .http_config import HTTPConfig  # noqa: E402
from .limits_config import LimitConfig  # noqa: E402
from .memory_config import StorageConfig  # noqa: E402
from .model_config import ProviderConfig  # noqa: E402


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


def resolve_config_path(explicit: str | os.PathLike[str] | None = None) -> Path:
    """按显式参数、`MORNLEA_AGENT_CONFIG`、当前目录默认值逐级确定配置文件路径。"""

    if explicit is not None:
        return Path(explicit)
    configured = os.environ.get(_CONFIG_ENV_VAR)
    if configured:
        return Path(configured)
    return Path("config.yaml")


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
