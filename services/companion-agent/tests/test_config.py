from __future__ import annotations

import os
from pathlib import Path
from typing import Any

import pytest
from pydantic import ValidationError

from mornlea_companion_agent import __version__
from mornlea_companion_agent.cli import main
from mornlea_companion_agent.config import (
    AgentConfig,
    ConfigError,
    ResolvedSecrets,
    load_config,
    resolve_secrets,
)


def _write_config(path: Path, overrides: str = "") -> Path:
    path.write_text(
        """
config_version: v1
http:
  bind: 127.0.0.1
  port: 8765
  workers: 1
  bearer_token_env: MORNLEA_AGENT_TOKEN
storage:
  sqlite_path: state/companion.sqlite3
provider:
  base_url: https://models.example.test/v1
  model: test-model
  api_key_env: MORNLEA_PROVIDER_KEY
""".lstrip()
        + overrides,
        encoding="utf-8",
    )
    return path


def _replace(path: Path, old: str, new: str) -> Path:
    path.write_text(path.read_text(encoding="utf-8").replace(old, new), encoding="utf-8")
    return path


def test_load_config_is_strict_frozen_and_resolves_sqlite_path(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path / "agent.yaml")

    config = load_config(config_path)

    assert config.config_version == "v1"
    assert str(config.http.bind) == "127.0.0.1"
    assert config.http.port == 8765
    assert config.http.workers == 1
    assert config.storage.sqlite_path == (tmp_path / "state/companion.sqlite3").resolve()
    assert not config.storage.sqlite_path.exists()
    assert config.limits.model_calls == 3
    assert config.limits.tool_calls == 4
    assert config.limits.timeout_seconds == 30
    with pytest.raises(ValidationError):
        config.http.port = 9000  # type: ignore[misc]


@pytest.mark.parametrize("bind", ["127.0.0.1", "127.12.34.56", "::1"])
def test_bind_accepts_only_loopback_ip_literals(tmp_path: Path, bind: str) -> None:
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"), "bind: 127.0.0.1", f'bind: "{bind}"'
    )
    assert load_config(config_path).http.bind.is_loopback


@pytest.mark.parametrize(
    "bind", ["localhost", "example.test", "0.0.0.0", "192.0.2.1", "::", "fe80::1"]
)
def test_bind_rejects_hostnames_wildcards_and_non_loopback(tmp_path: Path, bind: str) -> None:
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"), "bind: 127.0.0.1", f'bind: "{bind}"'
    )
    with pytest.raises(ConfigError, match="http.bind"):
        load_config(config_path)


@pytest.mark.parametrize("port", [0, 65536, "8765", True, None])
def test_port_rejects_out_of_range_and_coerced_values(tmp_path: Path, port: object) -> None:
    rendered = {"8765": '"8765"', "True": "true", "None": "null"}.get(repr(port), repr(port))
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"), "port: 8765", f"port: {rendered}"
    )
    with pytest.raises(ConfigError, match="http.port"):
        load_config(config_path)


@pytest.mark.parametrize("workers", [0, 2, "1", True])
def test_workers_is_exactly_one_without_coercion(tmp_path: Path, workers: object) -> None:
    rendered = {"'1'": '"1"', "True": "true"}.get(repr(workers), repr(workers))
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"), "workers: 1", f"workers: {rendered}"
    )
    with pytest.raises(ConfigError, match="http.workers"):
        load_config(config_path)


@pytest.mark.parametrize(
    "sqlite_path",
    [
        "",
        " ",
        ":memory:",
        "file:memory.sqlite3",
        "sqlite:///memory.sqlite3",
        "state/",
        "bad\\u0000path",
    ],
)
def test_sqlite_path_rejects_nonpersistent_or_non_file_values(
    tmp_path: Path, sqlite_path: str
) -> None:
    rendered = f'"{sqlite_path}"' if sqlite_path != "state/" else sqlite_path
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"),
        "sqlite_path: state/companion.sqlite3",
        f"sqlite_path: {rendered}",
    )
    with pytest.raises(ConfigError, match="storage.sqlite_path"):
        load_config(config_path)


def test_sqlite_path_rejects_existing_directory_without_creating_anything(tmp_path: Path) -> None:
    database_dir = tmp_path / "database"
    database_dir.mkdir()
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"),
        "state/companion.sqlite3",
        "database",
    )
    with pytest.raises(ConfigError, match="storage.sqlite_path"):
        load_config(config_path)
    assert list(database_dir.iterdir()) == []


@pytest.mark.parametrize(
    ("old", "new", "field"),
    [
        ("model: test-model", 'model: " test-model"', "provider.model"),
        ("model: test-model", 'model: "test\\u0000model"', "provider.model"),
        (
            "bearer_token_env: MORNLEA_AGENT_TOKEN",
            'bearer_token_env: " BAD_ENV"',
            "http.bearer_token_env",
        ),
        ("api_key_env: MORNLEA_PROVIDER_KEY", "api_key_env: 123", "provider.api_key_env"),
    ],
)
def test_text_and_env_names_reject_whitespace_controls_and_coercion(
    tmp_path: Path, old: str, new: str, field: str
) -> None:
    config_path = _replace(_write_config(tmp_path / "agent.yaml"), old, new)
    with pytest.raises(ConfigError, match=field):
        load_config(config_path)


@pytest.mark.parametrize(
    "url",
    [
        "ftp://models.example.test/v1",
        "https:///v1",
        "https://user@models.example.test/v1",
        "https://models.example.test/v1?debug=1",
        "https://models.example.test/v1#fragment",
        " https://models.example.test/v1",
    ],
)
def test_provider_base_url_rejects_unsafe_shapes(tmp_path: Path, url: str) -> None:
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"),
        "https://models.example.test/v1",
        f'"{url}"',
    )
    with pytest.raises(ConfigError, match="provider.base_url"):
        load_config(config_path)


@pytest.mark.parametrize(
    "url", ["https://models.example.test/v1", "http://127.0.0.1:11434/openai/v1"]
)
def test_provider_base_url_accepts_openai_compatible_paths(tmp_path: Path, url: str) -> None:
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"), "https://models.example.test/v1", url
    )
    assert load_config(config_path).provider.base_url == url


@pytest.mark.parametrize(
    ("name", "value", "maximum"),
    [("model_calls", 5, 5), ("tool_calls", 8, 8), ("timeout_seconds", 60, 60)],
)
def test_limit_hard_boundaries_are_accepted(
    tmp_path: Path, name: str, value: int, maximum: int
) -> None:
    config_path = _write_config(tmp_path / "agent.yaml", f"limits:\n  {name}: {value}\n")
    assert getattr(load_config(config_path).limits, name) == maximum


@pytest.mark.parametrize(
    ("name", "value"),
    [
        ("model_calls", 0),
        ("model_calls", 6),
        ("tool_calls", 0),
        ("tool_calls", 9),
        ("timeout_seconds", 0),
        ("timeout_seconds", 61),
        ("model_calls", '"3"'),
    ],
)
def test_limits_reject_out_of_range_and_coerced_values(
    tmp_path: Path, name: str, value: object
) -> None:
    config_path = _write_config(tmp_path / "agent.yaml", f"limits:\n  {name}: {value}\n")
    with pytest.raises(ConfigError, match=f"limits.{name}"):
        load_config(config_path)


@pytest.mark.parametrize(
    "document",
    [
        "[]\n",
        "null\n",
        "---\nconfig_version: v1\n---\nconfig_version: v1\n",
        "config_version: v1\nunknown: true\n",
        "config_version: 1\n",
    ],
)
def test_loader_requires_one_strict_yaml_mapping(tmp_path: Path, document: str) -> None:
    config_path = tmp_path / "agent.yaml"
    config_path.write_text(document, encoding="utf-8")
    with pytest.raises(ConfigError):
        load_config(config_path)


def test_unknown_nested_field_is_rejected(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path / "agent.yaml", "limits:\n  retry_count: 2\n")
    with pytest.raises(ConfigError, match="limits.retry_count"):
        load_config(config_path)


def test_structure_loading_does_not_resolve_environment_secrets(tmp_path: Path) -> None:
    config = load_config(_write_config(tmp_path / "agent.yaml"))
    assert isinstance(config, AgentConfig)


def test_secret_resolution_is_separate_and_redacted(tmp_path: Path) -> None:
    config = load_config(_write_config(tmp_path / "agent.yaml"))
    secret_value = "do-not-leak-this-value"
    secrets = resolve_secrets(
        config,
        {
            "MORNLEA_AGENT_TOKEN": secret_value,
            "MORNLEA_PROVIDER_KEY": "provider-secret",
        },
    )
    assert isinstance(secrets, ResolvedSecrets)
    assert secrets.http_bearer_token.get_secret_value() == secret_value
    assert secret_value not in repr(secrets)

    for environment in (
        {},
        {"MORNLEA_AGENT_TOKEN": secret_value},
        {"MORNLEA_AGENT_TOKEN": secret_value, "MORNLEA_PROVIDER_KEY": ""},
        {"MORNLEA_AGENT_TOKEN": secret_value, "MORNLEA_PROVIDER_KEY": "two words"},
        {"MORNLEA_AGENT_TOKEN": secret_value, "MORNLEA_PROVIDER_KEY": "bad\nsecret"},
    ):
        with pytest.raises(ConfigError) as error:
            resolve_secrets(config, environment)
        assert secret_value not in str(error.value)


def test_cli_version_and_help_are_lightweight(capsys: pytest.CaptureFixture[str]) -> None:
    assert main(["--version"]) == 0
    assert __version__ in capsys.readouterr().out
    assert main(["--help"]) == 0
    help_output = capsys.readouterr().out
    assert "Mornlea" in help_output
    assert "serve" in help_output


def test_cli_serve_resolves_config_and_secrets_then_calls_injected_runner(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    config_path = _write_config(tmp_path / "agent.yaml")
    monkeypatch.setenv("MORNLEA_AGENT_TOKEN", "agent-token")
    monkeypatch.setenv("MORNLEA_PROVIDER_KEY", "provider-token")
    captured: dict[str, Any] = {}

    def runner(config: AgentConfig, secrets: ResolvedSecrets) -> int:
        captured["config"] = config
        captured["secrets"] = secrets
        return 17

    assert main(["serve", "--config", os.fspath(config_path)], serve_runner=runner) == 17
    assert captured["config"].http.workers == 1
    assert captured["secrets"].http_bearer_token.get_secret_value() == "agent-token"


def test_cli_serve_does_not_accept_reload_workers_or_daemon(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path / "agent.yaml")

    def unused_runner(_config: AgentConfig, _secrets: ResolvedSecrets) -> int:
        return 0

    for argument in ("--reload", "--workers", "--daemon"):
        with pytest.raises(SystemExit):
            main(
                ["serve", "--config", os.fspath(config_path), argument], serve_runner=unused_runner
            )
