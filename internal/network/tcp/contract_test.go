package tcp_test

import (
	"context"
	"testing"

	"github.com/channing771/mornlea/internal/network"
	networktcp "github.com/channing771/mornlea/internal/network/tcp"
)

func TestTCPConstructorsExposeNetworkInterfaces(t *testing.T) {
	var listen func(string) (network.Listener, error) = networktcp.ListenTCP
	var dial func(context.Context, string) (network.ClientPacketStream, error) = networktcp.DialTCP
	if listen == nil || dial == nil {
		t.Fatal("TCP constructors are nil")
	}
}
