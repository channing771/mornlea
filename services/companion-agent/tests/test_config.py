from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Any

import httpx
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


def _write_documented_config(path: Path) -> Path:
    repository_root = Path(__file__).resolve().parents[3]
    documentation = (repository_root / "docs/notes/configuration.md").read_text(encoding="utf-8")
    section_marker = "Python 服务读取另一份 strict v1 YAML"
    _, marker, section = documentation.partition(section_marker)
    assert marker == section_marker
    _, fence, fenced = section.partition("```yaml\n")
    assert fence == "```yaml\n"
    example, closing_fence, _ = fenced.partition("```\n")
    assert closing_fence == "```\n"
    path.write_text(example, encoding="utf-8")
    return path


def _replace(path: Path, old: str, new: str) -> Path:
    path.write_text(path.read_text(encoding="utf-8").replace(old, new), encoding="utf-8")
    return path


def test_load_config_is_strict_frozen_and_resolves_sqlite_path(tmp_path: Path) -> None:
    config_path = _write_documented_config(tmp_path / "agent.yaml")

    config = load_config(config_path)

    assert config.config_version == "v1"
    assert str(config.http.bind) == "127.0.0.1"
    assert config.http.port == 8080
    assert config.http.workers == 1
    assert config.storage.sqlite_path == (tmp_path / "companion-agent.sqlite3").resolve()
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


def test_sqlite_path_symlink_loop_becomes_redacted_config_error(tmp_path: Path) -> None:
    loop = tmp_path / "loop.sqlite3"
    try:
        loop.symlink_to(loop.name)
    except (NotImplementedError, OSError) as error:
        pytest.skip(f"symlink unavailable: {error}")
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"),
        "state/companion.sqlite3",
        loop.name,
    )
    with pytest.raises(ConfigError, match="storage.sqlite_path") as captured:
        load_config(config_path)
    assert "symlink" not in str(captured.value).lower()


def test_config_path_symlink_loop_is_a_controlled_cli_error(tmp_path: Path) -> None:
    loop = tmp_path / "agent.yaml"
    try:
        loop.symlink_to(loop.name)
    except (NotImplementedError, OSError) as error:
        pytest.skip(f"symlink unavailable: {error}")

    with pytest.raises(ConfigError, match="unable to load configuration"):
        load_config(loop)

    completed = subprocess.run(
        [sys.executable, "-m", "mornlea_companion_agent", "serve", "--config", os.fspath(loop)],
        capture_output=True,
        text=True,
        check=False,
    )
    assert completed.returncode == 2
    assert "unable to load configuration" in completed.stderr
    assert "Traceback" not in completed.stderr


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
    "url",
    [
        "https://bad host.example/v1",
        "https://bad..example/v1",
        "https://.example.test/v1",
        "https://bad_label.example/v1",
        "https://-bad.example/v1",
        "https://bad-.example/v1",
        "https://xn--.example/v1",
        f"https://{'a' * 64}.example/v1",
        "https://example.test\\evil/v1",
        "http://exa\u200bmple.com/v1",
        "http://a\u200cb.com/v1",
        "http://a\u200db.com/v1",
        "http://\ufeffexample.com/v1",
        "http://example\u2060.com/v1",
        "http://ｅｘａｍｐｌｅ.com/v1",
        "http://💩.com/v1",
        "http://[v1.foo]/v1",
        "http://0127.0.0.1/v1",
        "http://999.1.1.1/v1",
    ],
)
def test_provider_base_url_rejects_httpx_unsendable_authorities(tmp_path: Path, url: str) -> None:
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"),
        "https://models.example.test/v1",
        json.dumps(url, ensure_ascii=False),
    )
    with pytest.raises(ConfigError, match="provider.base_url"):
        load_config(config_path)


@pytest.mark.parametrize(
    "url",
    [
        "http://exa\u200bmple.com/v1",
        "http://a\u200cb.com/v1",
        "http://a\u200db.com/v1",
        "http://\ufeffexample.com/v1",
        "http://example\u2060.com/v1",
        "http://ｅｘａｍｐｌｅ.com/v1",
        "http://💩.com/v1",
        "http://[v1.foo]/v1",
        "http://0127.0.0.1/v1",
        "http://999.1.1.1/v1",
    ],
)
def test_provider_host_probes_are_rejected_by_httpx(url: str) -> None:
    with pytest.raises(httpx.InvalidURL):
        httpx.URL(url)


@pytest.mark.parametrize(
    "url",
    [
        "https://a\u034fb.example/v1",
        "https://a\u0660b.example/v1",
    ],
)
def test_provider_base_url_matches_httpx_idna2008_rejections(tmp_path: Path, url: str) -> None:
    with pytest.raises(httpx.InvalidURL):
        httpx.URL(url)
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"),
        "https://models.example.test/v1",
        json.dumps(url, ensure_ascii=False),
    )
    with pytest.raises(ConfigError, match="provider.base_url"):
        load_config(config_path)


@pytest.mark.parametrize("host", ["xn--", "xn--a.example", "xn--0.pt"])
def test_provider_base_url_rejects_malformed_a_labels(tmp_path: Path, host: str) -> None:
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"),
        "https://models.example.test/v1",
        f"https://{host}/v1",
    )
    with pytest.raises(ConfigError, match="provider.base_url"):
        load_config(config_path)


@pytest.mark.parametrize(
    "url",
    [
        "https://models.example.test/v1",
        "http://127.0.0.1:11434/openai/v1",
        "https://例子.测试/v1",
        "https://क्‍ष.com/v1",
        "https://example.com./v1",
        "http://[::1]:11434/v1",
    ],
)
def test_provider_base_url_accepts_openai_compatible_paths(tmp_path: Path, url: str) -> None:
    config_path = _replace(
        _write_config(tmp_path / "agent.yaml"), "https://models.example.test/v1", url
    )
    assert httpx.URL(url)
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


@pytest.mark.parametrize("secret_env", ["MORNLEA_AGENT_TOKEN", "MORNLEA_PROVIDER_KEY"])
@pytest.mark.parametrize(
    "secret", ["令牌", "café", "token token", "token\tvalue", "token\x00value"]
)
def test_resolved_secrets_are_ascii_header_safe_and_redacted(
    tmp_path: Path, secret_env: str, secret: str
) -> None:
    config = load_config(_write_config(tmp_path / "agent.yaml"))
    environment = {
        "MORNLEA_AGENT_TOKEN": "agent-token",
        "MORNLEA_PROVIDER_KEY": "provider-token",
    }
    environment[secret_env] = secret
    with pytest.raises(ConfigError) as captured:
        resolve_secrets(config, environment)
    assert secret_env in str(captured.value)
    assert secret not in str(captured.value)


def test_resolved_secrets_can_be_encoded_as_httpx_authorization_headers(tmp_path: Path) -> None:
    config = load_config(_write_config(tmp_path / "agent.yaml"))
    secrets = resolve_secrets(
        config,
        {
            "MORNLEA_AGENT_TOKEN": "agent-token_123.~+/=",
            "MORNLEA_PROVIDER_KEY": "provider-token_456.~+/=",
        },
    )
    for secret in (secrets.http_bearer_token, secrets.provider_api_key):
        headers = httpx.Headers({"Authorization": f"Bearer {secret.get_secret_value()}"})
        assert headers.raw[0][1].isascii()


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
