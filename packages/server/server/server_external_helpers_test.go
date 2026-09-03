package server_test

// server_external_helpers_test.go：server_test 外部包共享的关服清理助手。

import (
	"context"
	"testing"

	"github.com/channing771/mornlea/packages/server/server"
)

func shutdownExternalServerForTest(t *testing.T, running *server.Server) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	if err := running.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown cleanup error=%v", err)
	}
}
