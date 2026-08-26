#!/usr/bin/env python3
import importlib.util
import pathlib
import sys
import tempfile
import unittest
from unittest import mock

SCRIPT = pathlib.Path(__file__).with_name("refresh-discussion.py")
SPEC = importlib.util.spec_from_file_location("refresh_discussion", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def row(task_id, status):
    return {"id": task_id, "name": "功能", "status": status, "who": "—", "pr": ""}


class RefreshDiscussionTest(unittest.TestCase):
    def test_current_statuses_render_expected_headers_shapes_and_rows_once(self):
        groups = [
            ("就绪", "🟢", True),
            ("已认领", "📋", False),
            ("开发中", "🟡", False),
            ("待集成", "⏳", False),
            ("排队", "🧭", True),
            ("设计候选", "🧩", True),
            ("已完成", "✅", False),
            ("已取消", "⚪", True),
        ]
        rows = [
            {"id": f"B-{i:02d}", "name": "功能", "status": status, "who": f"worker-{i}", "pr": f"PR #{i}"}
            for i, (status, _, _) in enumerate(groups, 1)
        ]
        body = MODULE.build_body(rows)

        for item, (status, icon, compact) in zip(rows, groups):
            self.assertEqual(body.count(f"| {item['id']} |"), 1)
            self.assertEqual(body.count(f"## {icon} {status}（1）"), 1)
            if compact:
                self.assertIn(f"| {item['id']} | 功能 | {item['pr']} |", body)
            else:
                self.assertIn(f"| {item['id']} | 功能 | {item['who']} | {item['pr']} |", body)
        self.assertNotIn("🟢 未认领", body)

    def test_retired_or_empty_status_fails_closed(self):
        for status in ["未认领", "", "评审中"]:
            with self.subTest(status=status):
                with self.assertRaisesRegex(ValueError, "未知任务状态"):
                    MODULE.build_body([row("A-01", status)])

    def test_parse_rows_accepts_injected_a_and_b_to_f_layouts(self):
        source = """\
| A-01 | A 功能 | 来源 | 开发中 | alice @ branch | PR #12 已合并；其他 |
| B-01 | B 功能 | 描述 | 影响 | 就绪 | bob @ branch | PR #34 待处理；其他 |
"""
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "backlog.md"
            path.write_text(source, encoding="utf-8")
            rows = MODULE.parse_rows(path)

        self.assertEqual(
            rows,
            [
                {"id": "A-01", "name": "A 功能", "status": "开发中", "who": "alice", "pr": "PR #12 已合并"},
                {"id": "B-01", "name": "B 功能", "status": "就绪", "who": "bob", "pr": "PR #34 待处理"},
            ],
        )

    def test_parse_rows_default_path_uses_backlog(self):
        ids = {item["id"] for item in MODULE.parse_rows()}
        self.assertIn("A-01", ids)
        self.assertIn("F-03", ids)

    def test_main_rejects_unknown_status_before_update(self):
        with (
            mock.patch.object(MODULE, "parse_rows", return_value=[row("A-01", "评审中")]),
            mock.patch.object(MODULE, "update_discussion") as update_discussion,
            mock.patch.object(sys, "argv", [str(SCRIPT), "--update"]),
        ):
            with self.assertRaisesRegex(SystemExit, "Discussion 正文生成失败: 未知任务状态") as exit_error:
                MODULE.main()

        self.assertNotEqual(exit_error.exception.code, 0)
        update_discussion.assert_not_called()


if __name__ == "__main__":
    unittest.main()
