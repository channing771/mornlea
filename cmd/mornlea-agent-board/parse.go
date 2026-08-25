// Package main 实现「Mornlea Agent 执行看板」本地 Web 应用。
//
// 本包是 cmd/mornlea-agent-board 的 main 包，只依赖 Go 标准库：服务端已改为
// 从 web/agent-board/dist 目录读盘服务前端产物，对外提供 /（看板）、/assets/*（静态产物）
// 与 /api/status（JSON 状态）三个接口。它聚合本机开发环境里「执行中的 AI 进程、接力链、
// 功能规划表任务、worktree 活动、OpenSpec change 进度、待确认卡片、PR/CI、
// 日志时间线」等数据。所有采集均 best-effort：任一环节失败只写入响应
// errors 字段，绝不拖垮整个页面，绝不返回 5xx。
//
// parse.go 只收纯解析函数与 JSON 输出结构定义（不触网、不执行命令、不读
// 系统目录），便于在 parse_test.go 里做确定性单测；系统交互都在 collect.go。
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Status 是 /api/status 的完整响应结构。
//
// 各切片在采集失败时保持空（而非 null），前端可安全遍历；唯 PRs 在 gh 不可用
// 时会被设为 nil 以输出 JSON null，前端据此展示降级说明。Errors 按采集小节
// 分 key（agents/chains/tasks/worktrees/confirm/prs/logs），内容是给运维看的中文说明。
type Status struct {
	// GeneratedAt 为本轮采集完成时间（RFC3339，UTC）。
	GeneratedAt string `json:"generatedAt"`
	// Root 为发现的仓库根目录（含 module 头的 go.mod 所在目录）。
	Root string `json:"root"`
	// Agents 为执行中的 AI 进程（claude/codex/run-agent/relay/feishu-listener/pr-finalize）。
	Agents []AgentStatus `json:"agents"`
	// Chains 为 ~/.mornlea/loop.guard* 描述的接力链。
	Chains []ChainStatus `json:"chains"`
	// Tasks 为 docs/feature-backlog.md 解析出的功能规划任务。
	Tasks []BacklogTask `json:"tasks"`
	// Worktrees 为 git worktree list 的结果（含主 checkout，标记 isMain）。
	Worktrees []WorktreeStatus `json:"worktrees"`
	// Confirm 为 ~/.mornlea/confirm 下的待确认/已回复卡片。
	Confirm []ConfirmCard `json:"confirm"`
	// PRs 为 gh pr list --state open 解析出的打开 PR；gh 不可用时为 null。
	PRs []PRStatus `json:"prs"`
	// Logs 为各类日志的末尾片段（key 为文件名，value 为行列表）。
	Logs map[string][]string `json:"logs"`
	// Errors 为各小节的采集错误说明（best-effort，永不 5xx）。
	Errors map[string]string `json:"errors"`
}

// AgentStatus 描述一个执行中的 AI 进程。
type AgentStatus struct {
	// Tool 为工具名（claude|codex|其他）。
	Tool string `json:"tool"`
	// Role 为角色（planner|implementer|listener|integrate|其他）。
	Role string `json:"role"`
	// PID / PPID 为进程与父进程号（以字符串保留）。
	PID  string `json:"pid"`
	PPID string `json:"ppid"`
	// Uptime 为运行时长（解析 etime 的规范字符串，如 "1h2m3s"；解析失败回退原始 etime）。
	Uptime string `json:"uptime"`
	// CWD 为该进程当前工作目录（lsof 尽力取，失败为空字符串）。
	CWD string `json:"cwd"`
	// Cmd 为截断到合理长度的命令摘要。
	Cmd string `json:"cmd"`
}

// ChainStatus 描述一条接力链（~/.mornlea/loop.guard*）。
type ChainStatus struct {
	// ID 为链 id（loop.guard → 主链；loop.guard.codex → codex）。
	ID string `json:"id"`
	// Pid 为守卫文件中登记的 pid（原样展示，可能为 0）。
	Pid string `json:"pid"`
	// Alive 为「kill -0 存活探测」结果。
	Alive bool `json:"alive"`
	// Stale 表示 pid 为 0 或已死（守卫失效/链条空闲）。
	Stale bool `json:"stale"`
	// GuardFile 为守卫文件名（如 loop.guard.codex）。
	GuardFile string `json:"guardFile"`
	// Mtime 为守卫文件修改时间（RFC3339，UTC）。
	Mtime string `json:"mtime"`
	// Note 为 UI 注记（说明「pid 可能为会话临时 shell 的已知缺陷」）。
	Note string `json:"note"`
}

// BacklogTask 为 docs/feature-backlog.md 的一行功能规划任务。
type BacklogTask struct {
	// ID 为任务编号（如 A-01、F-03）。
	ID string `json:"id"`
	// Feature 为功能名。
	Feature string `json:"feature"`
	// Summary 为简述。
	Summary string `json:"summary"`
	// Status 为规范化状态（未认领|已认领|开发中|待集成|已完成|其他）。
	Status string `json:"status"`
	// StatusRaw 为列中被识别的原始状态文本（未知状态时保留原样，便于排查）。
	StatusRaw string `json:"statusRaw"`
	// Claimant 为认领人（已去掉 @ 与分支名）。
	Claimant string `json:"claimant"`
	// Branch 为分支名（认领人「@ 分支名」中 @ 之后，或反引号内的分支）。
	Branch string `json:"branch"`
	// Note 为来源与备注列原文。
	Note string `json:"note"`
}

// WorktreeStatus 描述一个 git worktree 的活动。
type WorktreeStatus struct {
	// Path 为 worktree 目录。
	Path string `json:"path"`
	// Branch 为分支名（detached 时为空，前端回退显示 HEAD）。
	Branch string `json:"branch"`
	// Head 为 HEAD commit 的 sha。
	Head string `json:"head"`
	// IsMain 表示这是主 checkout（仓库根目录）。
	IsMain bool `json:"isMain"`
	// LastCommit 为最近提交（时间/作者/标题）；取不到时为 nil。
	LastCommit *Commit `json:"lastCommit,omitempty"`
	// DirtyCount 为 status --porcelain 的脏文件数。
	DirtyCount int `json:"dirtyCount"`
	// AheadCount 为领先 main 的提交数；HasAhead 为 false 表示该字段不可信
	//（HEAD 与 main 无共同祖先等情况，客户端应忽略）。
	AheadCount int  `json:"aheadCount"`
	HasAhead   bool `json:"hasAhead"`
	// Changes 为该 worktree 的 openspec/changes/* 进度（在途分支才有）。
	Changes []ChangeStatus `json:"changes"`
	// Error 为该 worktree 采集失败的原因（best-effort，单 worktree 失败不拖累全局）。
	Error string `json:"error,omitempty"`
}

// Commit 为一次提交的展示信息。
type Commit struct {
	Time    string `json:"time"`
	Author  string `json:"author"`
	Subject string `json:"subject"`
}

// ChangeStatus 为一个 OpenSpec change 的进度摘要。
type ChangeStatus struct {
	// Name 为 change 目录名。
	Name string `json:"name"`
	// Done / Total 为 tasks.md 中勾选/总任务数。
	Done  int `json:"done"`
	Total int `json:"total"`
	// LatestLedger 为 ledger.md 最后一条非空内容行（截断 ≤200 字符）。
	LatestLedger string `json:"latestLedger"`
}

// ConfirmCard 为一张确认卡片。
type ConfirmCard struct {
	// ID 为卡片 id（如 E-13-approval）。
	ID string `json:"id"`
	// Kind 为卡片类型（approval|question；请求未提供时默认 approval）。
	Kind string `json:"kind"`
	// Title 为标题。
	Title string `json:"title"`
	// Category 为分类（bounded|architectural 等）。
	Category string `json:"category"`
	// Question 为问题文本。
	Question string `json:"question"`
	// Design 为短设计摘要（原样字段，前端截断展示）。
	Design string `json:"design"`
	// Waiting 表示「有请求无回复」（等待中）。
	Waiting bool `json:"waiting"`
	// WaitSec 为等待时长（距创建时间秒数；创建时间缺失时用文件 mtime 兜底）。
	WaitSec int64 `json:"waitSec"`
	// ReplyAction 为回复动作（approve|edit|reject|answer）；无回复时为空。
	ReplyAction string `json:"replyAction,omitempty"`
	// ReplyText 为回复文本。
	ReplyText string `json:"replyText,omitempty"`
	// Status 为请求 JSON 中的原始 status 字段（pending|answered 等）。
	Status string `json:"status,omitempty"`
	// SupersededBy 表示该请求已被另一 id 取代（如 E-13-approval1 → E-13-approval2）。
	SupersededBy string `json:"supersededBy,omitempty"`
	// CreatedAt 为创建时间（JSON createdAt，RFC3339）。
	CreatedAt string `json:"createdAt"`
}

// PRStatus 描述一个打开的 PR。
type PRStatus struct {
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	Branch     string    `json:"branch"`
	MergeState string    `json:"mergeState"`
	Checks     []PRCheck `json:"checks"`
	URL        string    `json:"url"`
}

// PRCheck 描述一个 CI check。
type PRCheck struct {
	Name  string `json:"name"`
	State string `json:"state"` // success|failure|skipped|pending
}

// psRecord 是 ps 输出中匹配到的一个候选进程记录。
type psRecord struct {
	PID   string
	PPID  string
	Etime string
	Dur   time.Duration
	Cmd   string
}

// confirmRequest 是 ~/.mornlea/confirm/<id>.json 的请求字段
// （字段名以 E-13-approval.json 等实际文件为准）。
type confirmRequest struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Category     string `json:"category"`
	Kind         string `json:"kind"`
	Question     string `json:"question"`
	Design       string `json:"design"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
	RepliedAt    string `json:"repliedAt"`
	SupersededBy string `json:"supersededBy"`
}

// confirmReply 是 ~/.mornlea/confirm/<id>.reply.json 的回复字段。
type confirmReply struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	Text      string `json:"text"`
	RepliedAt string `json:"repliedAt"`
}

// -----------------------------------------
// 纯解析函数
// -----------------------------------------

var knownStatus = map[string]bool{
	"未认领": true, "已认领": true, "开发中": true, "待集成": true, "已完成": true,
}

// splitCells 把一行 Markdown 表格拆成单元格切片（去掉首尾竖线、trim 空格）。
func splitCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// isSeparatorCells 判断该行是否是表格分隔行（如 |---|---|）。
func isSeparatorCells(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		if strings.Trim(c, "-:") != "" {
			return false
		}
	}
	return true
}

// normalizeStatus 把状态列文本归一化为已知集合；未知时回退「其他」。
func normalizeStatus(s string) string {
	if knownStatus[s] {
		return s
	}
	return "其他"
}

// splitClaimant 把「认领人」单元格解析成认领人与分支名。
//
// 规则：先按内容中的 @ 分隔（「<agent 标识> @ <分支名>」）；若无 @，则尝试从
// 反引号内提取形如 codex/… 、fix/… 的分支名（如「前批实现线 `codex/…`」）。
// 认领人为空（—、-、空串）时返回 ("", "")。
func splitClaimant(raw string) (claimant, branch string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "—" || raw == "-" {
		return "", ""
	}
	if i := strings.LastIndex(raw, "@"); i >= 0 {
		return strings.TrimSpace(raw[:i]), strings.TrimSpace(raw[i+1:])
	}
	branch = extractBranchFromBackticks(raw)
	raw = strings.ReplaceAll(raw, "`", "")
	return strings.TrimSpace(raw), branch
}

// extractBranchFromBackticks 从反引号中提取看起来像分支名的 token。
func extractBranchFromBackticks(s string) string {
	// 手动扫描成对反引号，避免引入转义复杂度的正则。
	var idx []int
	for i := 0; i < len(s); i++ {
		if s[i] == '`' {
			idx = append(idx, i)
		}
	}
	for i := 0; i+1 < len(idx); i += 2 {
		tok := strings.TrimSpace(s[idx[i]+1 : idx[i+1]])
		if strings.Contains(tok, "/") && isBranchPrefix(tok) {
			return tok
		}
	}
	return ""
}

// isBranchPrefix 判断 token 是否以常见分支前缀开头。
func isBranchPrefix(s string) bool {
	for _, p := range []string{"codex/", "fix/", "chore/", "feat/", "refactor/", "docs/", "main"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// parseBacklogRow 解析 feature-backlog.md 的一行表格。
//
// 该表存在六列（A 组）与七列（B..F 组，多一列「版本与契约影响」）两种布局，
// 据此决定状态/认领人所在下标。返回 (任务, 是否为数据行)；表头、分隔行、列数不符、
// 空 id 或占位 id（-）的行返回 (零值, false)。
func parseBacklogRow(line string) (BacklogTask, bool) {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "|") {
		return BacklogTask{}, false
	}
	cells := splitCells(line)
	if len(cells) < 2 {
		return BacklogTask{}, false
	}
	if isSeparatorCells(cells) {
		return BacklogTask{}, false
	}
	// 表头行：首列是 ID（大小写不敏感）。
	if strings.EqualFold(cells[0], "ID") {
		return BacklogTask{}, false
	}
	var idc, featc, sumc, statusc, claimc, notec int
	switch len(cells) {
	case 6:
		idc, featc, sumc, statusc, claimc, notec = 0, 1, 2, 3, 4, 5
	case 7:
		idc, featc, sumc, statusc, claimc, notec = 0, 1, 2, 4, 5, 6
	default:
		return BacklogTask{}, false
	}
	id := strings.TrimSpace(cells[idc])
	if id == "" || id == "-" {
		return BacklogTask{}, false
	}
	rawStatus := strings.TrimSpace(cells[statusc])
	claimant, branch := splitClaimant(cells[claimc])
	return BacklogTask{
		ID:        id,
		Feature:   strings.TrimSpace(cells[featc]),
		Summary:   strings.TrimSpace(cells[sumc]),
		Status:    normalizeStatus(rawStatus),
		StatusRaw: rawStatus,
		Claimant:  claimant,
		Branch:    branch,
		Note:      strings.TrimSpace(cells[notec]),
	}, true
}

// parsePSLine 解析 ps 输出的一行 pid,ppid,etime,command。
//
// 头三列是 pid/ppid/etime（空格分隔），剩余为 command（可能有空格，join 还原）。
func parsePSLine(line string) (psRecord, bool) {
	f := strings.Fields(line)
	if len(f) < 3 {
		return psRecord{}, false
	}
	cmd := strings.Join(f[3:], " ")
	d, err := parseEtime(f[2])
	if err != nil {
		d = 0 // etime 解析失败不阻塞，仅记录原始字符串
	}
	return psRecord{PID: f[0], PPID: f[1], Etime: f[2], Dur: d, Cmd: cmd}, true
}

// parseEtime 解析 ps 的 etime 字段。支持 [[DD-]HH:]MM:SS 三种形态。
func parseEtime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("空的 etime")
	}
	total := time.Duration(0)
	if idx := strings.Index(s, "-"); idx >= 0 {
		days, err := strconv.Atoi(s[:idx])
		if err != nil {
			return 0, err
		}
		total += time.Duration(days) * 24 * time.Hour
		s = s[idx+1:]
	}
	parts := strings.Split(s, ":")
	var h, m, sec int
	switch len(parts) {
	case 1:
		sec, _ = strconv.Atoi(parts[0])
	case 2:
		m, _ = strconv.Atoi(parts[0])
		sec, _ = strconv.Atoi(parts[1])
	case 3:
		h, _ = strconv.Atoi(parts[0])
		m, _ = strconv.Atoi(parts[1])
		sec, _ = strconv.Atoi(parts[2])
	default:
		return 0, fmt.Errorf("无法解析 etime %q", s)
	}
	total += time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second
	return total, nil
}

// countTaskCheckboxes 统计一个 OpenSpec change 的 tasks.md 勾选状态。
//
// 只统计以「- [x]」或「- [X]」（已完成）与「- [ ]」（待办）开头的任务行；
// 其余标题、说明、代码行一律忽略。返回 (已完成数, 任务总数)；没有任何勾选行时
// 返回 (0, 0)，前端据此渲染空进度。
func countTaskCheckboxes(lines []string) (done, total int) {
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(t, "- [x]") || strings.HasPrefix(t, "- [X]"):
			done++
			total++
		case strings.HasPrefix(t, "- [ ]"):
			total++
		}
	}
	return done, total
}

// parseConfirmRequest 解析 ~/.mornlea/confirm/<id>.json 的请求内容。
//
// 字段名以实测的 E-13-approval.json 等文件为准：id/title/category/kind/question/
// design/status/createdAt/repliedAt/supersededBy。请求 JSON 里还可能有
// options/workerTool 等字段，本解析只保留看板需要的字段，未知字段被忽略。
func parseConfirmRequest(data []byte) (confirmRequest, error) {
	var r confirmRequest
	if err := json.Unmarshal(data, &r); err != nil {
		return confirmRequest{}, err
	}
	return r, nil
}

// parseConfirmReply 解析 ~/.mornlea/confirm/<id>.reply.json 的回复内容。
//
// 字段名以实测的 E-13-approval.reply.json 等文件为准：id/action/text/repliedAt。
// 回复 JSON 还可能有 senderOpenId 等字段，这里只保留看板需要的字段。
func parseConfirmReply(data []byte) (confirmReply, error) {
	var r confirmReply
	if err := json.Unmarshal(data, &r); err != nil {
		return confirmReply{}, err
	}
	return r, nil
}
