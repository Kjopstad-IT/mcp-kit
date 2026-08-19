package mcpkit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	kit "github.com/Kjopstad-IT/mcp-kit"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerProjectsRegisteredHandler(t *testing.T) {
	ctx := context.Background()
	server, err := kit.NewServer(newRegistry(t), &mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 1 || listed.Tools[0].Title != "Greet someone" {
		t.Fatalf("tools = %+v, want titled greet tool", listed.Tools)
	}

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "greet",
		Arguments: map[string]any{"name": "Ada", "loud": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["message"] != "HELLO, Ada" {
		t.Fatalf("structured content = %#v", result.StructuredContent)
	}
}

func TestServerRequiresImplementationIdentity(t *testing.T) {
	_, err := kit.NewServer(newRegistry(t), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "implementation") {
		t.Fatalf("NewServer error = %v, want implementation rejection", err)
	}
}

func TestServerRejectsPingKeepAliveOn20260728(t *testing.T) {
	_, err := kit.NewServer(
		newRegistry(t),
		&mcp.Implementation{Name: "test", Version: "0.1.0"},
		&mcp.ServerOptions{KeepAlive: time.Second},
	)
	if err == nil || !strings.Contains(err.Error(), "KeepAlive") {
		t.Fatalf("NewServer error = %v, want KeepAlive rejection", err)
	}
}
