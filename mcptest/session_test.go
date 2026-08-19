package mcptest

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var helperServer = flag.Bool("mcptest-helper-server", false, "run the mcptest stdio helper")

func TestMain(m *testing.M) {
	flag.Parse()
	if !*helperServer {
		os.Exit(m.Run())
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "helper", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestStartCommandNegotiatesPinnedProtocolAndSpeaksTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := StartCommand(ctx, helperCommand(t), Options{
		Implementation: &mcp.Implementation{Name: "mcptest", Version: "0.1.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if got := session.InitializeResult().ProtocolVersion; got != ProtocolVersion {
		t.Fatalf("protocol = %q, want %q", got, ProtocolVersion)
	}
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 1 || listed.Tools[0].Name != "ping" {
		t.Fatalf("tools = %+v, want ping", listed.Tools)
	}
}

func TestStartCommandRejectsUnexpectedProtocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := StartCommand(ctx, helperCommand(t), Options{
		Implementation:   &mcp.Implementation{Name: "mcptest", Version: "0.1.0"},
		ExpectedProtocol: "2099-01-01",
	})
	if err == nil || !strings.Contains(err.Error(), `negotiated protocol "2026-07-28", want "2099-01-01"`) {
		t.Fatalf("StartCommand error = %v", err)
	}
}

func TestStartCommandRejectsMissingIdentity(t *testing.T) {
	_, err := StartCommand(context.Background(), helperCommand(t), Options{})
	if err == nil || !strings.Contains(err.Error(), "implementation") {
		t.Fatalf("StartCommand error = %v, want identity rejection", err)
	}
}

func helperCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	return exec.Command(os.Args[0], "-mcptest-helper-server", "-test.run=^$")
}
