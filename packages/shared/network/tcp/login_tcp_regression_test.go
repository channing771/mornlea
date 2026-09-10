package tcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestRawTCPLoginSemanticIdentityFailuresReturnStableReject(t *testing.T) {
	validID := core.PlayerID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}
	tests := []struct {
		name      string
		id        core.PlayerID
		nameValue string
	}{
		{name: "invalid UUID", id: core.PlayerID{}, nameValue: "Chen"},
		{name: "control character", id: validID, nameValue: "Che\nn"},
		{name: "too many runes", id: validID, nameValue: strings.Repeat("界", 33)},
		{name: "too many encoded bytes", id: validID, nameValue: strings.Repeat("a", 129)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, serverDone := openRawTCPLogin(t)
			defer raw.Close()

			writeRawHandshake(t, raw)
			payload := append([]byte(nil), test.id[:]...)
			payload = appendRawString(payload, test.nameValue)
			// v40 起载荷尾部还需 1 字节域内视距，身份非法路径才不会被
			// 截断拒绝抢先命中。
			payload = append(payload, 32)
			if err := network.WriteFrame(raw, 0, payload); err != nil {
				t.Fatalf("write LoginStart: %v", err)
			}
			packetID, reject, err := network.ReadFrame(raw)
			if err != nil {
				t.Fatalf("read LoginReject: %v", err)
			}
			if packetID != 1 || len(reject) < 2 || network.LoginRejectCode(reject[0]) != network.LoginInvalidIdentity {
				t.Fatalf("LoginReject wire = id %d payload %x, want LoginInvalidIdentity", packetID, reject)
			}
			select {
			case err := <-serverDone:
				if err == nil {
					t.Fatal("BeginServerLogin accepted invalid identity")
				}
			case <-time.After(time.Second):
				t.Fatal("BeginServerLogin did not return after rejecting invalid identity")
			}
		})
	}
}

// TestRawTCPLoginViewDistanceOutsideDomainReturnsStableReject 钉死 v40 视距
// 字节的 wire 语义：域外值经真实 TCP 编解码后必须得到
// `LoginProtocolViolation` 的 `LoginReject` 应答——连接以登录拒绝收尾，而非
// 静默建会话，也非无应答的解码层断连。
func TestRawTCPLoginViewDistanceOutsideDomainReturnsStableReject(t *testing.T) {
	for _, tc := range []struct {
		name         string
		viewDistance byte
	}{
		{"zero", 0},
		{"above maximum", 65},
		{"saturated byte", 255},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, serverDone := openRawTCPLogin(t)
			defer raw.Close()

			writeRawHandshake(t, raw)
			id := core.PlayerID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}
			payload := append([]byte(nil), id[:]...)
			payload = appendRawString(payload, "Chen")
			payload = append(payload, tc.viewDistance)
			if err := network.WriteFrame(raw, 0, payload); err != nil {
				t.Fatalf("write LoginStart: %v", err)
			}
			packetID, reject, err := network.ReadFrame(raw)
			if err != nil {
				t.Fatalf("read LoginReject: %v", err)
			}
			if packetID != 1 || len(reject) < 2 || network.LoginRejectCode(reject[0]) != network.LoginProtocolViolation {
				t.Fatalf("LoginReject wire = id %d payload %x, want LoginProtocolViolation", packetID, reject)
			}
			select {
			case err := <-serverDone:
				if err == nil {
					t.Fatal("BeginServerLogin accepted out-of-domain view distance")
				}
			case <-time.After(time.Second):
				t.Fatal("BeginServerLogin did not return after rejecting view distance")
			}
		})
	}
}

func TestRawTCPMalformedLoginStringRemainsProtocolError(t *testing.T) {
	raw, serverDone := openRawTCPLogin(t)
	defer raw.Close()

	writeRawHandshake(t, raw)
	id := core.PlayerID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}
	payload := append([]byte(nil), id[:]...)
	payload = append(payload, 1, 0xff)
	if err := network.WriteFrame(raw, 0, payload); err != nil {
		t.Fatalf("write malformed LoginStart: %v", err)
	}
	if _, _, err := network.ReadFrame(raw); err == nil {
		t.Fatal("malformed UTF-8 received a LoginReject instead of closing as a protocol error")
	} else if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("malformed UTF-8 close error = %v", err)
	}
	select {
	case err := <-serverDone:
		if err == nil || !strings.Contains(err.Error(), "protocol violation") {
			t.Fatalf("BeginServerLogin malformed UTF-8 error = %v, want protocol violation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("BeginServerLogin did not return after malformed UTF-8")
	}
}

func openRawTCPLogin(t *testing.T) (streamConn, <-chan error) {
	t.Helper()
	client, server := openTCPStreamPair(t)
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	raw := client.(*tcpClientStream).stream.conn
	serverDone := make(chan error, 1)
	go func() {
		_, loginErr := network.BeginServerLogin(context.Background(), server, 0)
		serverDone <- loginErr
	}()
	return raw, serverDone
}

func writeRawHandshake(t *testing.T, raw streamConn) {
	t.Helper()
	if err := network.WriteFrame(raw, 0, []byte{byte(network.ProtocolVersion)}); err != nil {
		t.Fatalf("write ClientHello: %v", err)
	}
	packetID, payload, err := network.ReadFrame(raw)
	if err != nil || packetID != 0 || !bytes.Equal(payload, []byte{byte(network.ProtocolVersion)}) {
		t.Fatalf("ServerHello = id %d payload %x err %v", packetID, payload, err)
	}
}

func appendRawString(destination []byte, value string) []byte {
	length := uint32(len(value))
	for length >= 1<<7 {
		destination = append(destination, byte(length)|0x80)
		length >>= 7
	}
	destination = append(destination, byte(length))
	return append(destination, value...)
}
