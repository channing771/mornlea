package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// liveCollector 是真实的采集器实现：通过 ps/lsof/git/gh/读文件等本机命令采集数据。
//
// 所有小节都 best-effort：任一环节失败只把中文说明写入 Status.Errors 对应 key，
// 绝不 500、绝不拖垮整页。其 root 在构造时注入；now 可在测试中替换。
type liveCollector struct {
	// root 为仓库根目录（含 module 头的 go.mod 所在目录）。
	root string
	// now 返回当前时间；nil 时退化为 time.Now（便于测试注入固定时间）。
	now func() time.Time
	// prsMu 保护 prsCache；prsCache 缓存 gh pr list 结果 60 秒，避免每次刷新都拉起 gh。
	prsMu    sync.Mutex
	prsCache *prsCacheEntry
}

// prsCacheEntry 是一次 gh pr list 的缓存条目。
type prsCacheEntry struct {
	at     time.Time
	prs    []PRStatus
	errMsg string
}

// 编译期断言 liveCollector 满足 Collector 接口。
var _ Collector = (*liveCollector)(nil)

// nowTime 返回当前时间（优先使用注入的 now，否则 time.Now）。
func (c *liveCollector) nowTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// Collect 聚合所有小节并返回完整 Status。PRs 在 gh 不可用时保持 nil（JSON null），
// 其余切片保证非 nil 空切片，供前端安全遍历。
func (c *liveCollector) Collect(ctx context.Context) Status {
	st := Status{
		GeneratedAt: c.nowTime().UTC().Format(time.RFC3339),
		Root:        c.root,
		Agents:      []AgentStatus{},
		Chains:      []ChainStatus{},
		Tasks:       []BacklogTask{},
		Worktrees:   []WorktreeStatus{},
		Confirm:     []ConfirmCard{},
		Logs:        map[string][]string{},
		Errors:      map[string]string{},
	}
	st.Agents = c.collectAgents(ctx, st.Errors)
	st.Chains = c.collectChains(ctx, st.Errors)
	st.Tasks = c.collectTasks(st.Errors)
	st.Worktrees = c.collectWorktrees(ctx, st.Errors)
	st.Confirm = c.collectConfirm(st.Errors)
	prs, prsErr := c.collectPRs(ctx)
	st.PRs = prs
	if prsErr != "" {
		st.Errors["prs"] = prsErr
	}
	st.Logs = c.collectLogs(st.Errors)
	return st
}

// runWithTimeout 在 ctx 上叠加 timeout 执行命令，返回 stdout 与错误。
//
// stdout 与 stderr 分开捕获：命令失败（非超时）时错误信息附上已捕获 stderr 尾部的截断
// 片段（≤ maxStderrTail 字符），便于定位失败原因；超时则只返回带超时说明的错误，保持现状。
func runWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if cctx.Err() == context.DeadlineExceeded {
		return stdout.Bytes(), fmt.Errorf("%s 超时（%s)", name, timeout)
	}
	if err != nil {
		// 优先附 stderr 尾部；若为空则回退 stdout，尽力保留已捕获的输出。
		tail := stderrTail(stderr.String(), maxStderrTail)
		if tail == "" {
			tail = stderrTail(stdout.String(), maxStderrTail)
		}
		if tail != "" {
			return stdout.Bytes(), fmt.Errorf("%s 失败：%v（stderr 尾部：%s）", name, err, tail)
		}
		return stdout.Bytes(), fmt.Errorf("%s 失败：%v", name, err)
	}
	return stdout.Bytes(), nil
}

// maxStderrTail 是 runWithTimeout 错误信息中附带的 stderr 尾部截断上限。
const maxStderrTail = 200

// stderrTail 返回字符串尾部的截断片段（≤ max 个字符，按 rune 计数），去除首尾空白。
// 超长时以「…」开头，便于识别「这是被截断的片段」。
func stderrTail(s string, max int) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}
	r := []rune(t)
	if len(r) > max {
		return "…" + string(r[len(r)-(max-1):])
	}
	return t
}

// -----------------------------------------
// 小节 1：执行中的 AI 进程
// -----------------------------------------

// agentMarkers 是命令中需要匹配的 AI 进程关键词。
var agentMarkers = []string{"claude", "codex", "run-agent.sh", "relay.sh", "feishu-listener.js", "pr-finalize.sh"}

// collectAgents 通过 ps 抓取匹配的 AI 进程，并尽力解析 cwd。
func (c *liveCollector) collectAgents(ctx context.Context, errs map[string]string) []AgentStatus {
	out, err := runWithTimeout(ctx, 3*time.Second, "ps", "-axo", "pid=,ppid=,etime=,command=")
	if err != nil {
		errs["agents"] = fmt.Sprintf("ps 采集失败（best-effort）：%v", err)
		return []AgentStatus{}
	}
	agents := []AgentStatus{}
	recs := []psRecord{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !containsAny(line, agentMarkers) {
			continue
		}
		rec, ok := parsePSLine(line)
		if !ok {
			continue
		}
		recs = append(recs, rec)
		agents = append(agents, AgentStatus{
			Tool:   classifyTool(rec.Cmd),
			Role:   classifyRole(rec.Cmd),
			PID:    rec.PID,
			PPID:   rec.PPID,
			Uptime: formatUptime(rec),
			Cmd:    truncate(rec.Cmd, 160),
		})
	}
	// 每个匹配 agent 的 cwd 采集（lsof，各自 ≤2s 超时）改为并行 goroutine，
	// 使最坏耗时从 N×2s 降到 ~2s；每个 goroutine 只写自己下标的 CWD，无数据竞争。
	var wg sync.WaitGroup
	for i := range agents {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			agents[i].CWD = c.agentCWD(ctx, recs[i].PID)
		}(i)
	}
	wg.Wait()
	return agents
}

// classifyTool 从命令摘要判定工具名。
func classifyTool(cmd string) string {
	if strings.Contains(cmd, "claude") {
		return "claude"
	}
	if strings.Contains(cmd, "codex") {
		return "codex"
	}
	return "其他"
}

// classifyRole 从命令摘要判定角色。
//
// 优先级：pr-finalize → integrate；feishu-listener → listener；
// run-agent.sh planner 或 planner-prompt.md → planner；余下 implementer 线索 → implementer。
func classifyRole(cmd string) string {
	switch {
	case strings.Contains(cmd, "pr-finalize"):
		return "integrate"
	case strings.Contains(cmd, "feishu-listener"):
		return "listener"
	case strings.Contains(cmd, "run-agent.sh planner"), strings.Contains(cmd, "planner-prompt.md"):
		return "planner"
	case strings.Contains(cmd, "run-agent.sh implementer"), strings.Contains(cmd, "implementer.md"):
		return "implementer"
	default:
		return "其他"
	}
}

// formatUptime 把 psRecord 转成规范运行时长字符串；etime 解析失败时回退原始 etime。
func formatUptime(rec psRecord) string {
	if rec.Dur > 0 {
		return rec.Dur.Truncate(time.Second).String()
	}
	return rec.Etime
}

// agentCWD 用 lsof 尽力取进程当前工作目录；失败返回空串，不阻塞。
func (c *liveCollector) agentCWD(ctx context.Context, pid string) string {
	out, err := runWithTimeout(ctx, 2*time.Second, "lsof", "-a", "-d", "cwd", "-p", pid, "-Fn")
	if err != nil {
		return ""
	}
	// lsof -Fn 输出若干行，每行以字段前缀字母开头；cwd 的路径行以 n 开头。
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
}

// -----------------------------------------
// 小节 2：接力链
// -----------------------------------------

// chainGuardNote 说明「pid 可能为会话临时 shell」的已知缺陷，UI 据此展示注记。
const chainGuardNote = "pid 可能为会话临时 shell 的已知缺陷"

// collectChains 扫描 ~/.mornlea/loop.guard*，解析链 id、pid 与存活探测。
func (c *liveCollector) collectChains(ctx context.Context, errs map[string]string) []ChainStatus {
	home, err := os.UserHomeDir()
	if err != nil {
		errs["chains"] = fmt.Sprintf("无法获取用户目录：%v", err)
		return []ChainStatus{}
	}
	dir := filepath.Join(home, ".mornlea")
	entries, err := os.ReadDir(dir)
	if err != nil {
		errs["chains"] = fmt.Sprintf("无法读取 %s：%v", dir, err)
		return []ChainStatus{}
	}
	chains := []ChainStatus{}
	for _, e := range entries {
		name := e.Name()
		if !isGuardFile(name) {
			continue
		}
		path := filepath.Join(dir, name)
		info, _ := e.Info()
		mtime := ""
		if info != nil {
			mtime = info.ModTime().UTC().Format(time.RFC3339)
		}
		status := ChainStatus{
			ID:        chainID(name),
			GuardFile: name,
			Mtime:     mtime,
			Note:      chainGuardNote,
		}
		if data, err := os.ReadFile(path); err == nil {
			status.Pid = strings.TrimSpace(string(data))
		} else {
			status.Pid = ""
		}
		pid := parsePID(status.Pid)
		status.Alive = pidAlive(ctx, pid)
		status.Stale = pid == 0 || !status.Alive
		chains = append(chains, status)
	}
	return chains
}

// chainID 从守卫文件名推导链 id：loop.guard → 主链；loop.guard.codex → codex。
func chainID(name string) string {
	base := strings.TrimPrefix(name, "loop.guard")
	if base == "" {
		return "主链"
	}
	return strings.TrimPrefix(base, ".")
}

// isGuardFile 判断文件名是否是一个接力守卫文件（loop.guard*），并排除以 .bak 结尾的备份，
// 与 confirm 扫描的排除策略保持一致，避免把备份文件当作活跃守卫读取。
func isGuardFile(name string) bool {
	return strings.HasPrefix(name, "loop.guard") && !strings.HasSuffix(name, ".bak")
}

// parsePID 把字符串 pid 转成 int；空或非法返回 0。
func parsePID(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// pidAlive 用 kill -0 探测进程是否存活；pid<=0 视为不存活。
func pidAlive(ctx context.Context, pid int) bool {
	if pid <= 0 {
		return false
	}
	return exec.CommandContext(ctx, "kill", "-0", strconv.Itoa(pid)).Run() == nil
}

// -----------------------------------------
// 小节 3：任务状态
// -----------------------------------------

// collectTasks 解析 docs/feature-backlog.md 的表格行。
func (c *liveCollector) collectTasks(errs map[string]string) []BacklogTask {
	path := filepath.Join(c.root, "docs", "feature-backlog.md")
	data, err := os.ReadFile(path)
	if err != nil {
		errs["tasks"] = fmt.Sprintf("读取 %s 失败：%v", path, err)
		return []BacklogTask{}
	}
	tasks := []BacklogTask{}
	var skipped []int
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if task, ok := parseBacklogRow(line); ok {
			tasks = append(tasks, task)
		} else if isBacklogTaskPrefix(line) {
			// 形似任务行（首格为编号）但列数异常被跳过：记录行号，替换静默丢行。
			skipped = append(skipped, i+1)
		}
	}
	if len(skipped) > 0 {
		errs["tasks"] = fmt.Sprintf("%d 行任务表行因列数异常被跳过：第 %s 行", len(skipped), lineNumbersSuffix(skipped))
	}
	return tasks
}

// isBacklogTaskPrefix 判断一行是否形似任务表行：首格为任务编号。
func isBacklogTaskPrefix(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return false
	}
	cell := line[1:]
	if i := strings.Index(cell, "|"); i >= 0 {
		cell = cell[:i]
	}
	return isTaskID(strings.TrimSpace(cell))
}

// isTaskID 判断字符串是否为任务编号，即字母 A 至 F、连字符和两位数字。
func isTaskID(s string) bool {
	if len(s) != 4 || s[1] != '-' {
		return false
	}
	if s[0] < 'A' || s[0] > 'F' {
		return false
	}
	return s[2] >= '0' && s[2] <= '9' && s[3] >= '0' && s[3] <= '9'
}

// lineNumbersSuffix 把行号列表拼成「1, 2, 3」形式；过多时截断到前 20 个并附总数说明。
func lineNumbersSuffix(v []int) string {
	if len(v) == 0 {
		return ""
	}
	parts := make([]string, 0, len(v))
	for i, n := range v {
		if i >= 20 {
			parts = append(parts, fmt.Sprintf("…（共 %d 行）", len(v)))
			break
		}
		parts = append(parts, strconv.Itoa(n))
	}
	return strings.Join(parts, ", ")
}

// -----------------------------------------
// 小节 4：Worktree 活动（含 OpenSpec change 进度）
// -----------------------------------------

// collectWorktrees 解析 git worktree list，并并行采集每个 worktree 的活动与 change 进度。
func (c *liveCollector) collectWorktrees(ctx context.Context, errs map[string]string) []WorktreeStatus {
	out, err := runWithTimeout(ctx, 3*time.Second, "git", "-C", c.root, "worktree", "list", "--porcelain")
	if err != nil {
		errs["worktrees"] = fmt.Sprintf("git worktree list 失败：%v", err)
		return []WorktreeStatus{}
	}
	wts := parseWorktreeList(string(out), c.root)
	// 每个 worktree 一个 goroutine，并行做三个 git 命令（各自 ≤3s 超时）。
	var wg sync.WaitGroup
	for i := range wts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			wt := &wts[i]
			// 1) 最近提交（时间/作者/标题）。
			if lout, err := runWithTimeout(ctx, 3*time.Second, "git", "-C", wt.Path, "log", "-1", "--format=%cI%x1f%an%x1f%s"); err == nil {
				wt.LastCommit = parseCommitLine(string(lout))
			} else {
				wt.Error = fmt.Sprintf("git log 失败：%v", err)
			}
			// 2) 脏文件数。
			if sout, err := runWithTimeout(ctx, 3*time.Second, "git", "-C", wt.Path, "status", "--porcelain"); err == nil {
				wt.DirtyCount = countNonEmptyLines(string(sout))
			}
			// 3) 领先 main 提交数（失败可容忍，如 HEAD 与 main 无共同祖先则忽略该字段）。
			if aout, err := runWithTimeout(ctx, 3*time.Second, "git", "-C", wt.Path, "rev-list", "--count", "main..HEAD"); err == nil {
				if n, err2 := strconv.Atoi(strings.TrimSpace(string(aout))); err2 == nil {
					wt.AheadCount = n
					wt.HasAhead = true
				}
			}
			// 4) OpenSpec change 进度，挂在每个 worktree 上（在途分支才有）。
			wt.Changes = collectChanges(wt.Path)
		}(i)
	}
	wg.Wait()
	return wts
}

// parseWorktreeList 解析 git worktree list --porcelain 输出。
//
// 每个 worktree 以 「worktree <路径>」开头，后续可有 HEAD sha、branch refs/heads/<名字>、
// detached 或 bare 行。返回的 Branch 在 detached 时为空，前端回退显示 HEAD。
func parseWorktreeList(out, root string) []WorktreeStatus {
	var wts []WorktreeStatus
	var cur *WorktreeStatus
	flush := func() {
		if cur != nil {
			wts = append(wts, *cur)
			cur = nil
		}
	}
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &WorktreeStatus{Path: strings.TrimPrefix(line, "worktree ")}
			if samePath(cur.Path, root) {
				cur.IsMain = true
			}
		case strings.HasPrefix(line, "HEAD "):
			if cur != nil {
				cur.Head = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
			}
		case strings.HasPrefix(line, "branch refs/heads/"):
			if cur != nil {
				cur.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			}
		}
	}
	flush()
	return wts
}

// samePath 判断两个路径是否指向同一目录（清除尾部分隔符与 . 区别）。
func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

// parseCommitLine 从 git log --format=%cI%x1f%an%x1f%s 输出解析一次提交；不足三段判失败。
func parseCommitLine(s string) *Commit {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\x1f")
	if len(parts) < 3 {
		return nil
	}
	return &Commit{
		Time:    strings.TrimSpace(parts[0]),
		Author:  strings.TrimSpace(parts[1]),
		Subject: strings.TrimSpace(parts[2]),
	}
}

// countNonEmptyLines 统计非空行数（用于 status --porcelain 的脏文件计数）。
func countNonEmptyLines(s string) int {
	n := 0
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) != "" {
			n++
		}
	}
	return n
}

// collectChanges 扫描单个 worktree 的 openspec/changes/*/，统计每个 change 的进度。
func collectChanges(wtPath string) []ChangeStatus {
	changesDir := filepath.Join(wtPath, "openspec", "changes")
	entries, err := os.ReadDir(changesDir)
	if err != nil {
		// 在途分支通常才有 openspec/changes；主 checkout/已归档分支可能没有，属正常。
		return []ChangeStatus{}
	}
	changes := []ChangeStatus{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// archive 是归档容器而非单个 change，跳过避免噪声。
		if name == "archive" {
			continue
		}
		chDir := filepath.Join(changesDir, name)
		done, total := 0, 0
		if data, err := os.ReadFile(filepath.Join(chDir, "tasks.md")); err == nil {
			done, total = countTaskCheckboxes(strings.Split(string(data), "\n"))
		}
		latest := ""
		if data, err := os.ReadFile(filepath.Join(chDir, "ledger.md")); err == nil {
			latest = lastNonEmptyLine(string(data))
		}
		changes = append(changes, ChangeStatus{
			Name:         name,
			Done:         done,
			Total:        total,
			LatestLedger: latest,
		})
	}
	return changes
}

// lastNonEmptyLine 返回内容中最后一条非空行（截断 ≤200 字符）。
func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if t != "" {
			return truncate(t, 200)
		}
	}
	return ""
}

// -----------------------------------------
// 小节 5：待确认卡片
// -----------------------------------------

// collectConfirm 扫描 ~/.mornlea/confirm 下的请求/回复对，构建确认卡片。
//
// 命名约定：<id>.json 是请求，<id>.reply.json 是回复；roundN 是
// 多轮修订的命名，会归并到同一基础 id 判断是否有回复。feishu*.json、*.bak*、
// resume-*.log 等非卡片文件一律忽略。
func (c *liveCollector) collectConfirm(errs map[string]string) []ConfirmCard {
	home, err := os.UserHomeDir()
	if err != nil {
		errs["confirm"] = fmt.Sprintf("无法获取用户目录：%v", err)
		return []ConfirmCard{}
	}
	dir := filepath.Join(home, ".mornlea", "confirm")
	entries, err := os.ReadDir(dir)
	if err != nil {
		errs["confirm"] = fmt.Sprintf("无法读取 %s：%v", dir, err)
		return []ConfirmCard{}
	}
	type reqEntry struct {
		req   confirmRequest
		mtime time.Time
		exact bool
	}
	requests := map[string]*reqEntry{}
	replies := map[string]confirmReply{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") ||
			strings.HasPrefix(name, "feishu") ||
			strings.Contains(name, ".bak") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(name, ".reply") {
			if r, err := parseConfirmReply(data); err == nil && r.ID != "" {
				replies[confirmBaseID(name)] = r
			}
			continue
		}
		if r, err := parseConfirmRequest(data); err == nil && r.ID != "" && (r.Kind != "" || r.Title != "") {
			base := confirmBaseID(name)
			info, _ := e.Info()
			var m time.Time
			if info != nil {
				m = info.ModTime()
			}
			exact := !strings.Contains(name, ".round")
			if cur, ok := requests[base]; !ok || (exact && !cur.exact) {
				requests[base] = &reqEntry{req: r, mtime: m, exact: exact}
			}
		}
	}
	now := c.nowTime()
	cards := make([]ConfirmCard, 0, len(requests))
	for base, re := range requests {
		req := re.req
		card := ConfirmCard{
			ID:           req.ID,
			Kind:         req.Kind,
			Title:        req.Title,
			Category:     req.Category,
			Question:     req.Question,
			Design:       req.Design,
			Status:       req.Status,
			SupersededBy: req.SupersededBy,
			CreatedAt:    req.CreatedAt,
		}
		if card.Kind == "" {
			card.Kind = "approval"
		}
		reply, hasReply := replies[base]
		if hasReply {
			card.ReplyAction = reply.Action
			card.ReplyText = reply.Text
		}
		// 等待中 = 有请求无回复，且请求自身未被标记 answered、未被他卡取代。
		card.Waiting = !hasReply && req.Status != "answered" && req.SupersededBy == ""
		// 等待时长：优先创建时间（RFC3339），缺失用文件 mtime 兜底。
		created := parseRFC3339(req.CreatedAt)
		if created.IsZero() {
			created = re.mtime
		}
		if !created.IsZero() && now.After(created) {
			card.WaitSec = int64(now.Sub(created).Seconds())
		}
		cards = append(cards, card)
	}
	sortConfirm(cards)
	return cards
}

// confirmBaseID 把请求/回复文件名归一到基础 id，以便 roundN 变体能够配对。
//
// 规则：去掉 .json，随后反复剥掉末尾的 .reply 与 .round<数字>。
func confirmBaseID(filename string) string {
	s := strings.TrimSuffix(filename, ".json")
	for {
		if rest := strings.TrimSuffix(s, ".reply"); rest != s {
			s = rest
			continue
		}
		if i := strings.LastIndex(s, ".round"); i >= 0 {
			rest := s[i+len(".round"):]
			if rest != "" && allDigits(rest) {
				s = s[:i]
				continue
			}
		}
		break
	}
	return s
}

// allDigits 判断字符串是否全为数字。
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseRFC3339 解析 RFC3339/RFC3339Nano 时间；失败返回零值。
func parseRFC3339(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// sortConfirm 按卡片 id 升序排序，保证 UI 顺序稳定。
func sortConfirm(cards []ConfirmCard) {
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
}

// -----------------------------------------
// 小节 6：PR / CI（尽力而为 + 60s 缓存）
// -----------------------------------------

// prCacheTTL 是 gh 结果缓存的存活时长。
const prCacheTTL = 60 * time.Second

// collectPRs 返回打开的 PR 列表与错误说明；gh 不可用时返回 (nil, 中文说明)。
// 结果缓存 60 秒，避免每次刷新都拉起 gh。
func (c *liveCollector) collectPRs(ctx context.Context) ([]PRStatus, string) {
	c.prsMu.Lock()
	if c.prsCache != nil && c.nowTime().Sub(c.prsCache.at) < prCacheTTL {
		cache := *c.prsCache
		c.prsMu.Unlock()
		return cache.prs, cache.errMsg
	}
	c.prsMu.Unlock()
	// 在锁外执行 gh（可能耗时最长 3s），避免阻塞其它并发请求。
	prs, errMsg := fetchPRs(ctx)
	c.prsMu.Lock()
	c.prsCache = &prsCacheEntry{at: c.nowTime(), prs: prs, errMsg: errMsg}
	c.prsMu.Unlock()
	return prs, errMsg
}

// fetchPRs 执行 gh pr list 并解析为 PRStatus 切片。
func fetchPRs(ctx context.Context) ([]PRStatus, string) {
	out, err := runWithTimeout(ctx, 3*time.Second, "gh", "pr", "list",
		"--state", "open",
		"--json", "number,title,headRefName,mergeStateStatus,statusCheckRollup,url")
	if err != nil {
		return nil, fmt.Sprintf("gh pr list 不可用（%v）。请确认已登录 GitHub CLI：`gh auth status`。", err)
	}
	var raw []struct {
		Number            int    `json:"number"`
		Title             string `json:"title"`
		HeadRefName       string `json:"headRefName"`
		MergeStateStatus  string `json:"mergeStateStatus"`
		StatusCheckRollup []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			State      string `json:"state"`
		} `json:"statusCheckRollup"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Sprintf("解析 gh pr list 结果失败：%v", err)
	}
	prs := make([]PRStatus, 0, len(raw))
	for _, p := range raw {
		pr := PRStatus{
			Number:     p.Number,
			Title:      p.Title,
			Branch:     p.HeadRefName,
			MergeState: p.MergeStateStatus,
			URL:        p.URL,
			Checks:     []PRCheck{},
		}
		for _, chk := range p.StatusCheckRollup {
			pr.Checks = append(pr.Checks, PRCheck{
				Name:  chk.Name,
				State: rollupState(chk.Status, chk.Conclusion, chk.State),
			})
		}
		prs = append(prs, pr)
	}
	return prs, ""
}

// rollupState 把 gh statusCheckRollup 项归一为小写状态（success/failure/skipped/pending…）。
func rollupState(status, conclusion, state string) string {
	if conclusion != "" {
		return strings.ToLower(conclusion)
	}
	if state != "" {
		return strings.ToLower(state)
	}
	if status != "" {
		return strings.ToLower(status)
	}
	return "unknown"
}

// -----------------------------------------
// 小节 7：日志时间线
// -----------------------------------------

// logNames 是需要展示尾部的日志文件名（存在才读）。
var logNames = []string{"mornlea-implementer-loop.log", "mornlea-planner.log", "mornlea-pr-finalize.log"}

// collectLogs 读取各日志的末尾片段（每个 ≤30 行），存在与否与错误都容忍。
func (c *liveCollector) collectLogs(errs map[string]string) map[string][]string {
	logs := map[string][]string{}
	home, err := os.UserHomeDir()
	if err != nil {
		errs["logs"] = fmt.Sprintf("无法获取用户目录：%v", err)
		return logs
	}
	logDir := filepath.Join(home, "Library", "Logs")
	for _, name := range logNames {
		if lines, ok := tailFile(filepath.Join(logDir, name), 30); ok {
			logs[name] = lines
		}
	}
	// 可选：取 ~/.mornlea/confirm 下最新一个 resume-*.log 的尾部。
	if latest := latestResumeLog(filepath.Join(home, ".mornlea", "confirm")); latest != "" {
		if lines, ok := tailFile(latest, 30); ok {
			logs[filepath.Base(latest)] = lines
		}
	}
	return logs
}

// tailFileCappedBytes 是 tailFile 对单个日志文件一次性读入字节数的上限。
//
// 日志（尤其 ~/.mornlea/confirm/resume-*.log）可能暴涨到数 MB，整文件读入会浪费内存；
// 超过该上限时只从文件尾部读取最后一个窗口再取尾行，从而保持「取末尾 maxLines 行」语义。
const tailFileCappedBytes int64 = 8 * 1024 * 1024

// tailFile 读取文件末尾的 maxLines 行，保留原文行；文件不存在或不可读返回 (nil,false)。
//
// 文件超过 tailFileCappedBytes 时，先 Seek 到距末尾该上限的偏移处再读，避免把整个大文件
// 读进内存；此时首行可能被截断成半个行，因此丢弃它，保证取到的都是完整尾部行。
func tailFile(path string, maxLines int) ([]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false
	}
	start := int64(0)
	if info.Size() > tailFileCappedBytes {
		// 只读文件尾部最后一个窗口（上限内），避免整文件读入内存。
		start = info.Size() - tailFileCappedBytes
	}
	window := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(window, start); err != nil {
		return nil, false
	}
	text := strings.ReplaceAll(string(window), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if start > 0 && len(lines) > 0 {
		// 窗口起点落在行中间：丢弃被截断的首行，剩余均为完整行。
		lines = lines[1:]
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	// 去掉文件尾部来自末尾换行符的空行。
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	// 文件为空（或只有换行符）时视为「无内容」，不放进日志区，避免渲染空条目。
	if len(lines) == 0 {
		return nil, false
	}
	return lines, true
}

// latestResumeLog 返回 confirm 目录下 mtime 最新的 resume-*.log 路径；无则空串。
func latestResumeLog(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var latest string
	var latestTime time.Time
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "resume-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = filepath.Join(dir, name)
		}
	}
	return latest
}

// -----------------------------------------
// 通用小工具
// -----------------------------------------

// containsAny 判断 s 是否包含任意一个子串（用于关键词过滤）。
func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// truncate 把字符串截断到最多 max 个字符（按 rune 计数，避免切断 UTF-8），并追加省略号。
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
