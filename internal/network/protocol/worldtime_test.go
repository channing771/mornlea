package protocol

import "testing"

// TestProtocolVersionPinned 钉住当前协议版本号：升版必须是有意识的改动，漏改
// 版本号会让新旧两端对同一 payload 做出不同的形状解读。命名不编码具体版本
// （升版只改断言值），与文件内「版本无关命名」的既有约定一致。
func TestProtocolVersionPinned(t *testing.T) {
	if ProtocolVersion != 31 {
		t.Fatalf("协议版本=%d，想要 31", ProtocolVersion)
	}
}
