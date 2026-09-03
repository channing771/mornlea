package main

import (
	"strings"
	"testing"
	"time"
)

// TestParsePSLineBasic 锁定 pid/ppid/etime/command 的常规解析。
func TestParsePSLineBasic(t *testing.T) {
	rec, ok := parsePSLine("1234 5678 01:02:03 /usr/bin/claude --dangerously-skip-permissions")
	if !ok {
		t.Fatalf("常规 ps 行应解析成功")
	}
	if rec.PID != "1234" || rec.PPID != "5678" {
		t.Errorf("PID=%q PPID=%q", rec.PID, rec.PPID)
	}
	if rec.Dur != time.Hour+2*time.Minute+3*time.Second {
		t.Errorf("Dur = %v", rec.Dur)
	}
	if !strings.HasPrefix(rec.Cmd, "/usr/bin/claude") {
		t.Errorf("Cmd = %q", rec.Cmd)
	}
}

// TestParsePSLineDays 锁定带天数前缀的 etime（DD-HH:MM:SS）。
func TestParsePSLineDays(t *testing.T) {
	rec, ok := parsePSLine("100 200 3-01:02:03 /bin/codex x")
	if !ok {
		t.Fatalf("带天数的 ps 行应解析成功")
	}
	want := 3*24*time.Hour + time.Hour + 2*time.Minute + 3*time.Second
	if rec.Dur != want {
		t.Errorf("Dur = %v，想要 %v", rec.Dur, want)
	}
}

// TestParsePSLineShortEtime 锁定仅 MM:SS 形态。
func TestParsePSLineShortEtime(t *testing.T) {
	rec, ok := parsePSLine("100 200 05:06 /bin/relay.sh")
	if !ok {
		t.Fatalf("短 etime 应解析成功")
	}
	if rec.Dur != 5*time.Minute+6*time.Second {
		t.Errorf("Dur = %v", rec.Dur)
	}
}

// TestParsePSLineInvalidEtime 锁定 etime 解析失败不阻塞，仅记 0。
func TestParsePSLineInvalidEtime(t *testing.T) {
	rec, ok := parsePSLine("100 200 not-a-time /bin/x")
	if !ok {
		t.Fatalf("etime 非法但行本身应解析成功")
	}
	if rec.Dur != 0 {
		t.Errorf("Dur = %v，应为 0", rec.Dur)
	}
	if rec.Etime != "not-a-time" {
		t.Errorf("Etime = %q", rec.Etime)
	}
}

// TestParsePSLineTooFewFields 锁定不足三个字段判失败。
func TestParsePSLineTooFewFields(t *testing.T) {
	if _, ok := parsePSLine("1234 5678"); ok {
		t.Errorf("不足三字段应判失败")
	}
}
