"""伙伴 Agent 服务存储配置：持久 SQLite 文件路径（相对路径按配置文件所在目录解析）。"""

from __future__ import annotations

from pathlib import Path

from pydantic import ValidationInfo, field_validator

from .app_config import _URI_SCHEME, _StrictFrozenModel, _validate_nonempty_text


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
