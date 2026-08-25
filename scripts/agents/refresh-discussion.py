#!/usr/bin/env python3
# 从 docs/feature-backlog.md 重新生成 Discussion #71 的「状态列表」正文并推送。
# 用法: scripts/agents/refresh-discussion.py [--update]
#   --update   生成后直接更新讨论（需 gh auth）；缺省只打印正文供检查。
# 规范: 讨论是仓库表的镜像——按状态分组、未认领仅列 ID+名称、完整明细以仓库为准。
import json, os, re, subprocess, sys

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
BACKLOG = os.path.join(ROOT, "docs", "feature-backlog.md")
DISCUSSION_ID = "D_kwDOToJS8M4Aou6G"

def parse_rows():
    rows = []
    with open(BACKLOG, encoding="utf-8") as f:
        for line in f:
            m = re.match(r"^\|\s*([A-F]-\d{2})\s*\|", line)
            if not m:
                continue
            cells = [c.strip() for c in line.split("|")[1:-1]]
            rid = cells[0]
            is_a = rid.startswith("A-")
            st_idx = 3 if is_a else 4
            who_idx = 4 if is_a else 5
            status = cells[st_idx] if len(cells) > st_idx else ""
            who = cells[who_idx] if len(cells) > who_idx else ""
            who = re.sub(r"@.*", "", who).strip() or "—"
            note = cells[-1] if len(cells) > st_idx + 1 else ""
            pm = re.search(r"PR #\d+[^；|]*", note)
            pr = pm.group(0).strip() if pm else ""
            rows.append({"id": rid, "name": cells[1] if len(cells) > 1 else "", "status": status, "who": who, "pr": pr})
    return rows

def build_body(rows):
    groups = [
        ("已完成", "✅", lambda s: "完成" in s),
        ("开发中", "🟡", lambda s: "开发" in s),
        ("已认领（未开工）", "📋", lambda s: s == "已认领"),
        ("待集成", "⏳", lambda s: "待集成" in s),
        ("未认领", "🟢", lambda s: s == "未认领"),
    ]
    out = [
        "> **单一真相源**：仓库 [`docs/feature-backlog.md`](../../blob/main/docs/feature-backlog.md)（完整来源/依赖/版本影响都在仓库）。本讨论只维护**按状态分组的列表**，由规划者每轮刷新（`scripts/agents/refresh-discussion.py --update`）；两次刷新间的状态变化以评论为准（每条评论对应一次变更）。",
        "",
        "## 认领规则（简版）",
        "1. 选一行 🟢 未认领且依赖已满足 → 改仓库表该行状态/认领人并提交 → 从此负责到底；",
        "2. 开发流程唯一说明：repo `docs/development-process.md`（OpenSpec change → subagent-driven-development → 双评审 → 门禁 → PR+CI 全绿 → merge → 回填）；",
        "3. 每个任务先经过 brainstorming 内容确认（推送你的飞书，回复后自动续跑）；",
        "4. 认领/完成/放弃等状态变化必须在讨论追加一条结构化评论（模板见 docs/agents/implementer.md）。",
        "",
    ]
    for label, icon, pred in groups:
        lst = [r for r in rows if pred(r["status"])]
        if not lst:
            continue
        out.append("## " + icon + " " + label + "（" + str(len(lst)) + "）")
        if label == "未认领":
            out.append("| ID | 功能 | 备注 |")
            out.append("|---|---|---|")
            for r in lst:
                out.append("| " + r["id"] + " | " + r["name"] + " | " + (r["pr"] or "—") + " |")
        else:
            out.append("| ID | 功能 | 认领者 | 备注 |")
            out.append("|---|---|---|---|")
            for r in lst:
                out.append("| " + r["id"] + " | " + r["name"] + " | " + r["who"] + " | " + (r["pr"] or "—") + " |")
        out.append("")
    return "\n".join(out).rstrip() + "\n"

def update_discussion(body):
    query = "mutation($b:String!){ updateDiscussion(input:{discussionId:\"" + DISCUSSION_ID + "\", body:$b}){ discussion { number url } } }"
    payload = json.dumps({"query": query, "variables": {"b": body}})
    p = subprocess.run(["gh", "api", "graphql", "--input", "-"], input=payload, capture_output=True, text=True)
    if p.returncode != 0:
        raise SystemExit("gh 调用失败: " + p.stderr)
    print(p.stdout)

def main():
    rows = parse_rows()
    body = build_body(rows)
    if "--update" in sys.argv:
        update_discussion(body)
        print("已刷新讨论（" + str(len(rows)) + " 行）")
    else:
        print(body)

if __name__ == "__main__":
    main()
