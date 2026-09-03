//go:build darwin

package devcapture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// portFileData 是端口发现文件的内容：外部调用方（agent、脚本）靠它在不解析
// 日志的情况下找到「哪个进程在哪个端口提供捕获服务」。
type portFileData struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	StartedAt time.Time `json:"started_at"`
}

// DefaultPortFilePath 返回端口发现文件的默认路径 `~/.mornlea/dev-capture.json`。
func DefaultPortFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("devcapture: 定位用户主目录失败: %w", err)
	}
	return filepath.Join(home, ".mornlea", "dev-capture.json"), nil
}

// writePortFile 写入发现文件（自动补建 `.mornlea` 目录层）。经 os.WriteFile
// 全量落盘：调用方读到的是完整 JSON，不会见到半截。
func writePortFile(path string, data portFileData) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("devcapture: 创建目录 %s 失败: %w", dir, err)
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("devcapture: 编码端口发现文件失败: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("devcapture: 写入端口发现文件 %s 失败: %w", path, err)
	}
	return nil
}

// readPortFile 回读发现文件（测试与人工排障使用；生产路径只写不读）。
func readPortFile(path string) (portFileData, error) {
	var data portFileData
	raw, err := os.ReadFile(path)
	if err != nil {
		return data, err
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, fmt.Errorf("devcapture: 解析端口发现文件 %s 失败: %w", path, err)
	}
	return data, nil
}

// removePortFile 清除发现文件。文件不存在（上次异常退出后已被清理等）视为
// 已完成——清理必须是幂等操作，重复信号不应报错。
func removePortFile(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}
