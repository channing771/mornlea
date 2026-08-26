#!/usr/bin/env python3
import importlib.util
import pathlib
import unittest

SCRIPT = pathlib.Path(__file__).with_name("refresh-discussion.py")
SPEC = importlib.util.spec_from_file_location("refresh_discussion", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def row(task_id, status):
    return {"id": task_id, "name": "功能", "status": status, "who": "—", "pr": ""}


class RefreshDiscussionTest(unittest.TestCase):
    def test_current_statuses_each_render_exactly_once(self):
        statuses = ["就绪", "已认领", "开发中", "待集成", "排队", "设计候选", "已完成", "已取消"]
        rows = [row(f"B-{i:02d}", status) for i, status in enumerate(statuses, 1)]
        body = MODULE.build_body(rows)
        for item in rows:
            self.assertEqual(body.count(f"| {item['id']} |"), 1)
        self.assertIn("🟢 就绪", body)
        self.assertNotIn("🟢 未认领", body)

    def test_retired_or_empty_status_fails_closed(self):
        for status in ["未认领", "", "评审中"]:
            with self.subTest(status=status):
                with self.assertRaisesRegex(ValueError, "未知任务状态"):
                    MODULE.build_body([row("A-01", status)])


if __name__ == "__main__":
    unittest.main()
