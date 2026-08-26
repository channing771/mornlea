#!/usr/bin/env python3
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).with_name("relay.sh")


def run_relay(backlog):
    with tempfile.TemporaryDirectory() as directory:
        root = pathlib.Path(directory)
        relay = root / "scripts" / "agents" / "relay.sh"
        relay.parent.mkdir(parents=True)
        shutil.copy2(SCRIPT, relay)
        docs = root / "docs"
        docs.mkdir()
        (docs / "feature-backlog.md").write_text(backlog, encoding="utf-8")
        home = root / "home"
        home.mkdir()
        env = os.environ.copy()
        env.update(
            HOME=str(home),
            MORNLEA_LOOP_GUARD=str(root / "loop.guard"),
            MORNLEA_LOOP_LOG=str(root / "relay.log"),
        )
        return subprocess.run(["bash", str(relay)], cwd=root, env=env, capture_output=True, text=True)


class RelayTest(unittest.TestCase):
    def test_non_status_ready_cells_do_not_relay(self):
        backlog = """\
| A-01 | 就绪 | 来源 | 排队 | — | 就绪 |
| B-01 | 就绪 | 描述 | 影响 | 设计候选 | — | 就绪 |
"""
        result = run_relay(backlog)

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("规划表已无就绪任务", result.stdout)

    def test_a_and_b_to_f_status_columns_relay(self):
        rows = [
            "| A-01 | 功能 | 来源 | 就绪 | — | 备注 |\n",
            "| B-01 | 功能 | 描述 | 影响 | 就绪 | — | 备注 |\n",
        ]
        for backlog in rows:
            with self.subTest(backlog=backlog):
                result = run_relay(backlog)
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn("run-agent.sh 缺失", result.stdout)

    def test_large_backlog_with_early_ready_row_relays(self):
        ready = "| A-01 | 功能 | 来源 | 就绪 | — | 备注 |\n"
        queued = "| B-99 | 功能 | 描述 | 影响 | 排队 | — | " + ("x" * 1024) + " |\n"
        result = run_relay(ready + queued * 2048)

        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("run-agent.sh 缺失", result.stdout)


if __name__ == "__main__":
    unittest.main()
