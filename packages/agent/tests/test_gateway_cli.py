"""新网关 CLI 入口测试：只覆盖参数与配置链，不启动服务。"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

import pytest
from harness.config import AgentConfig, ResolvedSecrets

from app.gateway.cli import main

ROOT = Path(__file__).resolve().parents[1]


def _write_config(path: Path) -> Path:
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
""".lstrip(),
        encoding="utf-8",
    )
    return path


def test_serve_passes_loaded_config_and_secrets_to_runner(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    config_path = _write_config(tmp_path / "agent.yaml")
    monkeypatch.setenv("MORNLEA_AGENT_TOKEN", "agent-token")
    monkeypatch.setenv("MORNLEA_PROVIDER_KEY", "provider-token")
    captured: dict[str, object] = {}

    def fake_runner(config: AgentConfig, secrets: ResolvedSecrets) -> int:
        captured["config"] = config
        captured["secrets"] = secrets
        return 0

    assert main(["serve", "--config", str(config_path)], serve_runner=fake_runner) == 0
    config = captured["config"]
    assert isinstance(config, AgentConfig)
    assert config.storage.sqlite_path == (tmp_path / "state/companion.sqlite3").resolve()
    secrets = captured["secrets"]
    assert isinstance(secrets, ResolvedSecrets)
    assert "agent-token" not in repr(secrets)


def test_serve_with_unreadable_config_exits_without_running(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("MORNLEA_AGENT_TOKEN", "agent-token")
    monkeypatch.setenv("MORNLEA_PROVIDER_KEY", "provider-token")

    def forbidden_runner(config: AgentConfig, secrets: ResolvedSecrets) -> int:
        raise AssertionError("runner must not run with an invalid config")

    with pytest.raises(SystemExit) as error:
        main(
            ["serve", "--config", str(tmp_path / "missing.yaml")],
            serve_runner=forbidden_runner,
        )
    assert error.value.code == 2


@pytest.mark.parametrize("arguments", [["--help"], ["--version"]])
def test_help_and_version_do_not_import_app(arguments: list[str]) -> None:
    script = (
        "import sys; "
        "from app.gateway.cli import main; "
        f"code = main({arguments!r}); "
        "assert code == 0, code; "
        "assert 'app.gateway.app' not in sys.modules, 'heavy app module was imported'"
    )
    completed = subprocess.run(
        [sys.executable, "-c", script],
        cwd=ROOT,
        capture_output=True,
        text=True,
        check=False,
    )
    assert completed.returncode == 0, completed.stderr or completed.stdout
