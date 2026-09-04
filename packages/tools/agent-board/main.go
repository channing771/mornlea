package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	// 默认监听地址：优先 BOARD_ADDR 环境变量，否则回退「127.0.0.1:8787」；
	// --addr flag 再覆盖默认值。
	defaultAddr := os.Getenv("BOARD_ADDR")
	if defaultAddr == "" {
		defaultAddr = "127.0.0.1:8787"
	}
	addr := flag.String("addr", defaultAddr, "看板监听地址（BOARD_ADDR 提供默认值，flag 覆盖之；默认 127.0.0.1:8787）")
	flag.Parse()

	root, err := discoverRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法确定 Mornlea 仓库根目录：%v\n", err)
		os.Exit(1)
	}

	collector := &liveCollector{root: root}
	// distDir 为前端构建产物目录（packages/tools/agent-board/web/dist）；未构建时
	// / 会返回指引页。
	distDir := filepath.Join(root, "packages", "tools", "agent-board", "web", "dist")
	// ReadHeaderTimeout 限制读取请求头的时间，避免慢速/伪造连接长期占用连接，提升健壮性。
	server := &http.Server{
		Addr:              *addr,
		Handler:           newStatusHandlerWithDist(collector, distDir),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// 监听 SIGINT/SIGTERM，收到后优雅关停。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("Mornlea Agent 执行看板已启动：http://%s （仓库根：%s）\n", *addr, root)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		fmt.Println("收到信号，正在优雅关停…")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		fmt.Println("看板已关停")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "服务器错误：%v\n", err)
			os.Exit(1)
		}
	}
}

// findRepoRoot 从 startDir 开始逐级向上查找仓库根：根 go.mod 已随单元化解散，
// 根目录的唯一 Go 清单是 go.work（内容以 `go ` 版本指令开头，且只出现在仓库根）；
// 命中返回 (目录, true)，否则 (空, false)。可被单测在临时目录验证。
func findRepoRoot(startDir string) (string, bool) {
	dir := startDir
	for {
		workspace := filepath.Join(dir, "go.work")
		if data, err := os.ReadFile(workspace); err == nil {
			if strings.HasPrefix(string(data), "go ") {
				return dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// discoverRoot 解析仓库根：先可执行文件目录向上，再 CWD 向上，最后 BOARD_ROOT 环境变量。
func discoverRoot() (string, error) {
	if exe, err := os.Executable(); err == nil {
		if dir, ok := findRepoRoot(filepath.Dir(exe)); ok {
			return dir, nil
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if dir, ok := findRepoRoot(cwd); ok {
			return dir, nil
		}
	}
	if env := os.Getenv("BOARD_ROOT"); env != "" {
		if dir, ok := findRepoRoot(env); ok {
			return dir, nil
		}
		// BOARD_ROOT 已设置但无效：说明具体路径与原因，避免「提示可设置 BOARD_ROOT」与行为矛盾。
		return "", fmt.Errorf("BOARD_ROOT 已设置但无效：%s（该目录不存在或其中没有含 go 工作区清单的 go.work）", env)
	}
	return "", errors.New("无法定位仓库根目录：请在仓库内运行，或用 BOARD_ROOT 指定含 go.work 工作区清单的目录")
}
