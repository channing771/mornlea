package main

import (
	"strings"
	"testing"
)

// TestParseBacklogRowHeader 锁定表头行（六列与七列两种布局）一律判为非数据行。
func TestParseBacklogRowHeader(t *testing.T) {
	rows := []string{
		"| ID | 功能 | 简述 | 状态 | 认领人 | 来源与备注 |",
		"| ID | 功能 | 简述 | 版本与契约影响 | 状态 | 认领人 | 来源与备注 |",
	}
	for _, r := range rows {
		if task, ok := parseBacklogRow(r); ok {
			t.Fatalf("表头行 %q 应判为非数据行，得到 %+v", r, task)
		}
	}
}

// TestParseBacklogRowSeparator 锁定分隔行（含列数不同）判为非数据行。
func TestParseBacklogRowSeparator(t *testing.T) {
	rows := []string{
		"|---|---|---|---|---|---|",
		"|---|---|---|---|---|---|---|",
	}
	for _, r := range rows {
		if _, ok := parseBacklogRow(r); ok {
			t.Fatalf("分隔行 %q 应判为非数据行", r)
		}
	}
}

// TestParseBacklogRowSixCol 锁定 A 组六列布局：状态/认领人在第 3/4 列，且从反引号提取分支。
func TestParseBacklogRowSixCol(t *testing.T) {
	row := "| A-01 | 权威格子工作台 | 背包 2×2 配方 | 待集成 | 前批实现线 `codex/authoritative-grid-crafting` | 分支头 a657c1cb |"
	task, ok := parseBacklogRow(row)
	if !ok {
		t.Fatalf("六列数据行应被识别，实际不是")
	}
	if task.ID != "A-01" {
		t.Errorf("ID = %q", task.ID)
	}
	if task.Status != "待集成" {
		t.Errorf("Status = %q", task.Status)
	}
	if task.Branch != "codex/authoritative-grid-crafting" {
		t.Errorf("Branch = %q", task.Branch)
	}
	if task.Claimant == "" {
		t.Errorf("Claimant 不应为空")
	}
	if task.Feature != "权威格子工作台" {
		t.Errorf("Feature = %q", task.Feature)
	}
	if !strings.Contains(task.Note, "a657c1cb") {
		t.Errorf("Note = %q", task.Note)
	}
}

// TestParseBacklogRowSevenCol 锁定 B..F 组七列布局：状态/认领人在第 4/5 列。
func TestParseBacklogRowSevenCol(t *testing.T) {
	row := "| B-01 | 更多食物与作物 | 简述 | 物品编号追加 | 已认领 | claude @ fix/B-01 | hunger 遗留 |"
	task, ok := parseBacklogRow(row)
	if !ok {
		t.Fatalf("七列数据行应被识别，实际不是")
	}
	if task.ID != "B-01" {
		t.Errorf("ID = %q", task.ID)
	}
	if task.Status != "已认领" {
		t.Errorf("Status = %q", task.Status)
	}
	if task.Claimant != "claude" {
		t.Errorf("Claimant = %q", task.Claimant)
	}
	if task.Branch != "fix/B-01" {
		t.Errorf("Branch = %q", task.Branch)
	}
}

// TestParseBacklogRowEmptyClaimant 锁定「—」/「-」/空串认领人解析为空。
func TestParseBacklogRowEmptyClaimant(t *testing.T) {
	for _, claim := range []string{"—", "-", ""} {
		row := "| C-01 | 功能 | 简述 | 未认领 | " + claim + " | 备注 |"
		task, ok := parseBacklogRow(row)
		if !ok {
			t.Fatalf("认领人 %q 的行应被识别", claim)
		}
		if task.Claimant != "" || task.Branch != "" {
			t.Errorf("claim=%q branch=%q 应为空", task.Claimant, task.Branch)
		}
		if task.Status != "未认领" {
			t.Errorf("Status = %q", task.Status)
		}
	}
}

// TestParseBacklogRowUnknownStatus 锁定未知状态归一为「其他」且保留原文。
func TestParseBacklogRowUnknownStatus(t *testing.T) {
	row := "| D-01 | foo | bar | 评审中 | — | note |"
	task, ok := parseBacklogRow(row)
	if !ok {
		t.Fatalf("数据行应被识别")
	}
	if task.Status != "其他" {
		t.Errorf("Status = %q，应为「其他」", task.Status)
	}
	if task.StatusRaw != "评审中" {
		t.Errorf("StatusRaw = %q", task.StatusRaw)
	}
}

// TestParseBacklogRowNonTableAndLegend 锁定非表格行与「状态图例」三列行判为非数据行。
func TestParseBacklogRowNonTableAndLegend(t *testing.T) {
	if _, ok := parseBacklogRow("# 标题行"); ok {
		t.Errorf("# 开头行应判为非数据行")
	}
	if _, ok := parseBacklogRow("普通段落，无竖线"); ok {
		t.Errorf("无竖线行应判为非数据行")
	}
	// 「状态图例」三列布局（状态/含义/谁可继续）列数不符，判为非数据行。
	if _, ok := parseBacklogRow("| 未认领 | 等待认领 | 任意 agent 可认领 |"); ok {
		t.Errorf("三列图例行应判为非数据行")
	}
}
