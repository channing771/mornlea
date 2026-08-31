package server

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/companion"
)

type crossLanguageMCPProbeRequest struct {
	Request companion.PlanRequest `json:"request"`
}

type crossLanguageMCPProbeResult struct {
	ProtocolVersion       string   `json:"protocol_version"`
	ImplementationVersion string   `json:"implementation_version"`
	Tools                 []string `json:"tools"`
	Calls                 []string `json:"calls"`
}

func TestMCPAgentCrossLanguageIntegration(t *testing.T) {
	registry, registration := testMCPRegistry(t)
	service, err := newCompanionMCPService(registry)
	if err != nil {
		t.Fatalf("start real MCP service: %v", err)
	}
	t.Cleanup(service.Close)

	request := companion.PlanRequest{
		ContractVersion:  "v1",
		RequestID:        "11111111-1111-4111-8111-111111111111",
		ClientInstanceID: "22222222-2222-4222-8222-222222222222",
		NamespaceID:      testMCPNamespace,
		LeaseID:          "44444444-4444-4444-8444-444444444444",
		RunID:            "55555555-5555-4555-8555-555555555555",
		CompanionID:      "66666666-6666-4666-8666-666666666666",
		Generation:       1,
		SnapshotID:       registration.SnapshotID,
		SnapshotDigest:   registration.Digest,
		DeadlineUnixMS:   time.Now().Add(15 * time.Second).UnixMilli(),
		MCPEndpoint:      service.Endpoint(),
		MCPCapability:    registration.Capability,
		Instruction:      "采一块石头",
	}
	payload, err := json.Marshal(crossLanguageMCPProbeRequest{Request: request})
	if err != nil {
		t.Fatal(err)
	}

	repositoryRoot := crossLanguageRepositoryRoot(t)
	python := os.Getenv("MORNLEA_COMPANION_AGENT_PYTHON")
	if python == "" {
		python = filepath.Join(repositoryRoot, "services", "companion-agent", ".venv", "bin", "python")
	}
	helper := filepath.Join(repositoryRoot, "services", "companion-agent", "tests", "integration", "process.py")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, python, helper, "mcp-probe")
	command.Dir = filepath.Join(repositoryRoot, "services", "companion-agent")
	command.Stdin = bytes.NewReader(payload)
	command.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", "HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=", "NO_PROXY=*")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("real Python MCP probe failed: %v (stderr bytes=%d)", err, stderr.Len())
	}
	if ctx.Err() != nil {
		t.Fatalf("real Python MCP probe timeout: %v", ctx.Err())
	}

	var result crossLanguageMCPProbeResult
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode Python MCP probe result: %v", err)
	}
	wantTools := companion.PlanningToolNames()
	if result.ProtocolVersion != "2025-11-25" || result.ImplementationVersion != "v1" ||
		!reflect.DeepEqual(result.Tools, wantTools) {
		t.Fatalf("unexpected MCP contract summary: protocol=%q implementation=%q tools=%v",
			result.ProtocolVersion, result.ImplementationVersion, result.Tools)
	}
	wantCalls := []string{"get_planning_context", "query_terrain", "validate_plan"}
	if !reflect.DeepEqual(result.Calls, wantCalls) {
		t.Fatalf("tool calls=%v，want %v", result.Calls, wantCalls)
	}
}

func crossLanguageRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve integration test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
