//go:build darwin

package devcapture

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPortFileRoundTrip 钉住发现文件的写入与读取：内容含 pid、端口与启动
// 时间，且写入前自动补建 `.mornlea` 目录层。
func TestPortFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".mornlea", "dev-capture.json")
	startedAt := time.Unix(1700000000, 0)
	if err := writePortFile(path, portFileData{PID: 4321, Port: 17790, StartedAt: startedAt}); err != nil {
		t.Fatalf("写入端口发现文件失败：%v", err)
	}
	data, err := readPortFile(path)
	if err != nil {
		t.Fatalf("回读端口发现文件失败：%v", err)
	}
	if data.PID != 4321 || data.Port != 17790 {
		t.Errorf("回读内容 = %+v，想要 pid 4321 / port 17790", data)
	}
	if !data.StartedAt.Equal(startedAt) {
		t.Errorf("started_at = %v，想要 %v", data.StartedAt, startedAt)
	}
	removePortFile(path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("删除后文件仍存在：err = %v", err)
	}
}

// TestRemovePortFileToleratesMissing 钉住清理的幂等：文件不存在时删除必须
// 视为成功（异常退出后的二次清理不应报错）。
func TestRemovePortFileToleratesMissing(t *testing.T) {
	removePortFile(filepath.Join(t.TempDir(), "absent.json"))
}

// TestStartWritesPortFileAndStopRemovesIt 钉住生命周期：Start 绑定后写入含
// 实际端口的发现文件，`/status` 报告同一端口，Stop 清除文件且幂等。
func TestStartWritesPortFileAndStopRemovesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev-capture.json")
	s := New(Options{Addr: "127.0.0.1:0", PortFilePath: path})
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start 失败：%v", err)
	}
	data, err := readPortFile(path)
	if err != nil {
		t.Fatalf("Start 后应存在端口发现文件：%v", err)
	}
	boundPort := data.Port
	if data.PID != os.Getpid() {
		t.Errorf("pid = %d，想要 %d", data.PID, os.Getpid())
	}
	if boundPort <= 0 {
		t.Errorf("端口文件应含实际端口，得到 %d", boundPort)
	}

	// 经真实回环地址核验 /status 报告同一端口（这是端口文件语义的一部分）。
	resp, err := http.Get("http://" + addr + "/status")
	if err != nil {
		t.Fatalf("请求 /status 失败：%v", err)
	}
	var report statusReport
	err = json.NewDecoder(resp.Body).Decode(&report)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("反解 /status 失败：%v", err)
	}
	if report.Port != boundPort {
		t.Errorf("/status 端口 = %d，想要 %d", report.Port, boundPort)
	}

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop 失败：%v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Stop 后端口发现文件应被清除：err = %v", err)
	}
	// 二次 Stop 必须幂等（重复信号/重复清理路径）。
	if err := s.Stop(); err != nil {
		t.Errorf("重复 Stop 应为 nil，得到 %v", err)
	}
}

// TestStartCascadesPastBusyPort 钉住端口顺延：默认端口被占时自动 +1 重试，
// 发现文件记录顺延后的实际端口。
func TestStartCascadesPastBusyPort(t *testing.T) {
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占位监听失败：%v", err)
	}
	defer blocker.Close()
	busyPort := blocker.Addr().(*net.TCPAddr).Port

	path := filepath.Join(t.TempDir(), "dev-capture.json")
	s := New(Options{Addr: fmt.Sprintf("127.0.0.1:%d", busyPort), PortFilePath: path})
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start 应顺延端口成功：%v", err)
	}
	defer s.Stop()

	data, err := readPortFile(path)
	if err != nil {
		t.Fatalf("读取端口发现文件失败：%v", err)
	}
	if data.Port <= busyPort {
		t.Fatalf("应顺延到 %d 之后的端口，实际绑定 %d", busyPort, data.Port)
	}
	if data.Port > busyPort+maxBindAttempts {
		t.Errorf("顺延跨度应受尝试次数约束，实际 %d", data.Port)
	}
	if report := statusOf(t, addr); report.Port != data.Port {
		t.Errorf("/status 端口 = %d，想要顺延后的 %d", report.Port, data.Port)
	}
}

// statusOf 经真实 HTTP 拉取 /status（仅端口生命周期测试使用）。
func statusOf(t *testing.T, addr string) statusReport {
	t.Helper()
	resp, err := http.Get("http://" + addr + "/status")
	if err != nil {
		t.Fatalf("请求 /status 失败：%v", err)
	}
	defer resp.Body.Close()
	var report statusReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("反解 /status 失败：%v", err)
	}
	return report
}

// TestStartTwiceFails 钉住重复启动防御：同一 `Service` 二次 Start 必须报错，
// 且失败发生在任何副作用之前——端口发现文件原样保留（仍是首次绑定端口），
// 不得被顺延重试覆写成指向死端口的条目。
func TestStartTwiceFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev-capture.json")
	s := New(Options{Addr: "127.0.0.1:0", PortFilePath: path})
	if _, err := s.Start(); err != nil {
		t.Fatalf("首次 Start 失败：%v", err)
	}
	defer s.Stop()
	before, err := readPortFile(path)
	if err != nil {
		t.Fatalf("读取端口发现文件失败：%v", err)
	}
	if _, err := s.Start(); err == nil {
		t.Fatal("二次 Start 应报错")
	}
	after, err := readPortFile(path)
	if err != nil {
		t.Fatalf("二次 Start 后端口发现文件应原样保留：%v", err)
	}
	if after != before {
		t.Errorf("二次 Start 不得破坏端口发现文件：%+v → %+v", before, after)
	}
}

// TestStartRejectsNonLoopbackHost 钉住回环绑定防线：options 层传入的任何
// 非回环地址都在绑定之前被明确拒绝，且不写端口发现文件——「仅绑定回环」
// 不能只靠默认值约定。
func TestStartRejectsNonLoopbackHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev-capture.json")
	for _, addr := range []string{"0.0.0.0:17799", ":17799", "192.168.1.5:17799"} {
		s := New(Options{Addr: addr, PortFilePath: path})
		if _, err := s.Start(); err == nil {
			_ = s.Stop()
			t.Fatalf("非回环地址 %s 应被拒绝", addr)
		} else if !strings.Contains(err.Error(), "回环") {
			t.Errorf("非回环拒绝 %s 的文案应点名回环约束：%v", addr, err)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("被拒绝的启动不应写端口发现文件：err = %v", err)
	}
}
