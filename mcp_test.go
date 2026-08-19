package mcpkit_test

import (
	"context"
	"encoding/json"
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
	schemaJSON, err := json.Marshal(listed.Tools[0].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "name" {
		t.Fatalf("required schema fields = %v, want [name]", schema.Required)
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

func TestServerUsesExactMCPTextAndKeepsStructuredContent(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "greet"}, greet, kit.Renderer[greetOut]{
		Text:    func(out greetOut) (string, error) { return out.Message, nil },
		MCPText: func(out greetOut) (string, error) { return "wire:" + out.Message, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	server, err := kit.NewServer(r, &mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
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
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "greet", Arguments: map[string]any{"name": "Ada"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %#v, want one block", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || text.Text != "wire:Hello, Ada" {
		t.Fatalf("content = %#v, want exact MCP text", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["message"] != "Hello, Ada" {
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
