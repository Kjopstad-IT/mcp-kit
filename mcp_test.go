package mcpkit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	kit "github.com/Kjopstad-IT/mcp-kit"
	"github.com/google/jsonschema-go/jsonschema"
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

func TestRegistryAddsToolsToAnExistingSDKServer(t *testing.T) {
	r := newRegistry(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "existing", Version: "0.1.0"}, nil)
	if err := r.AddTo(server); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
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
	if len(listed.Tools) != 1 || listed.Tools[0].Name != "greet" {
		t.Fatalf("tools = %+v, want mounted greet tool", listed.Tools)
	}
}

func TestServerRunsTheRegistryMiddlewareChain(t *testing.T) {
	r := kit.NewRegistry()
	err := r.Use(func(_ kit.Tool, next kit.Handler) kit.Handler {
		return func(ctx context.Context, input any) (any, error) {
			output, err := next(ctx, input)
			if err != nil {
				return nil, err
			}
			value := output.(greetOut)
			value.Message = "wrapped:" + value.Message
			return value, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	err = kit.Register(r, kit.Tool{Name: "greet"}, greet, kit.Renderer[greetOut]{
		Text: func(output greetOut) (string, error) { return output.Message, nil },
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
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["message"] != "wrapped:Hello, Ada" {
		t.Fatalf("structured content = %#v", result.StructuredContent)
	}
}

func TestRegistryMiddlewareCanDenyBothSurfacesBeforeTheHandler(t *testing.T) {
	r := kit.NewRegistry()
	err := r.Use(func(_ kit.Tool, _ kit.Handler) kit.Handler {
		return func(context.Context, any) (any, error) {
			return nil, fmt.Errorf("license required")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = kit.Register(r, kit.Tool{Name: "greet"},
		func(context.Context, greetIn) (greetOut, error) {
			called = true
			return greetOut{}, nil
		},
		kit.Renderer[greetOut]{Text: func(output greetOut) (string, error) { return output.Message, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr testWriter
	err = kit.Run(context.Background(), r, []string{"greet", "Ada"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "license required") {
		t.Fatalf("Run error = %v, want middleware denial", err)
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
	if !result.IsError || len(result.Content) != 1 {
		t.Fatalf("result = %+v, want one error content block", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "license required") {
		t.Fatalf("content = %#v, want middleware denial", result.Content)
	}
	if called {
		t.Fatal("handler ran after middleware denial")
	}
}

func TestServerProjectsMCPOnlyNestedHandler(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "nested", MCPOnly: true},
		func(_ context.Context, input nestedInput) (greetOut, error) {
			return greetOut{Message: input.Filter.Owner + ":" + input.Labels["team"]}, nil
		},
		kit.Renderer[greetOut]{},
	)
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
	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 1 || listed.Tools[0].Name != "nested" {
		t.Fatalf("tools = %+v, want MCP-only nested tool", listed.Tools)
	}
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "nested",
		Arguments: map[string]any{
			"filter": map[string]any{"owner": "ada"},
			"labels": map[string]any{"team": "platform"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["message"] != "ada:platform" {
		t.Fatalf("structured content = %#v", result.StructuredContent)
	}
}

func TestServerProjectsComposedMCPOnlyInputSchema(t *testing.T) {
	custom := &jsonschema.Schema{Type: "object", OneOf: []*jsonschema.Schema{
		{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{"text": {Type: "string"}},
			Required:   []string{"text"},
		},
		{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{"count": {Type: "integer"}},
			Required:   []string{"count"},
		},
	}}
	r := kit.NewRegistry()
	var middlewareSchema *jsonschema.Schema
	if err := r.Use(func(spec kit.Tool, next kit.Handler) kit.Handler {
		middlewareSchema = spec.MCPInputSchema
		return next
	}); err != nil {
		t.Fatal(err)
	}
	err := kit.Register(r, kit.Tool{
		Name: "composed", MCPOnly: true, MCPInputSchema: custom,
	}, func(_ context.Context, input map[string]any) (greetOut, error) {
		if text, ok := input["text"].(string); ok {
			return greetOut{Message: text}, nil
		}
		return greetOut{Message: fmt.Sprint(input["count"])}, nil
	}, kit.Renderer[greetOut]{})
	if err != nil {
		t.Fatal(err)
	}
	if middlewareSchema == nil {
		t.Fatal("middleware did not receive the custom input schema")
	}
	// Registration owns the wire schema. Later caller or middleware mutation
	// must not change the contract mounted on a server.
	custom.OneOf = nil
	middlewareSchema.OneOf = nil

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
	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 1 {
		t.Fatalf("tools = %+v, want composed tool", listed.Tools)
	}
	schemaJSON, err := json.Marshal(listed.Tools[0].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		OneOf []json.RawMessage `json:"oneOf"`
	}
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.OneOf) != 2 {
		t.Fatalf("input schema = %s, want two oneOf branches", schemaJSON)
	}
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "composed", Arguments: map[string]any{"text": "Ada"},
	})
	if err != nil {
		t.Fatal(err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["message"] != "Ada" {
		t.Fatalf("structured content = %#v", result.StructuredContent)
	}
	invalid, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "composed", Arguments: map[string]any{"unknown": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !invalid.IsError {
		t.Fatalf("invalid result = %+v, want schema validation error", invalid)
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

func TestServerProjectsNonTextContentAndKeepsStructuredContent(t *testing.T) {
	type imageOutput struct {
		Data string `json:"data"`
	}
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "capture"},
		func(context.Context, struct{}) (imageOutput, error) {
			return imageOutput{Data: "PNG"}, nil
		},
		kit.Renderer[imageOutput]{
			Text: func(imageOutput) (string, error) { return "captured image/png", nil },
			MCPContent: func(out imageOutput) ([]mcp.Content, error) {
				return []mcp.Content{&mcp.ImageContent{Data: []byte(out.Data), MIMEType: "image/png"}}, nil
			},
		},
	)
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
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %#v, want one image", result.Content)
	}
	image, ok := result.Content[0].(*mcp.ImageContent)
	if !ok || string(image.Data) != "PNG" || image.MIMEType != "image/png" {
		t.Fatalf("content = %#v, want image/png PNG", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["data"] != "PNG" {
		t.Fatalf("structured content = %#v", result.StructuredContent)
	}
}

func TestServerProjectsProgressWithRequestToken(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "work"},
		func(ctx context.Context, _ struct{}) (greetOut, error) {
			if err := kit.ReportProgress(ctx, kit.Progress{Message: "indexing", Current: 2, Total: 5}); err != nil {
				return greetOut{}, err
			}
			return greetOut{Message: "done"}, nil
		},
		kit.Renderer[greetOut]{Text: func(out greetOut) (string, error) { return out.Message, nil }},
	)
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
	updates := make(chan *mcp.ProgressNotificationParams, 1)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1.0"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			updates <- req.Params
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	params := &mcp.CallToolParams{Name: "work"}
	params.SetProgressToken("job-7")
	if _, err := clientSession.CallTool(ctx, params); err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-updates:
		if update.ProgressToken != "job-7" || update.Message != "indexing" || update.Progress != 2 || update.Total != 5 {
			t.Fatalf("progress = %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("progress notification was not delivered")
	}
}

func TestServerProjectsEffectsAnnotations(t *testing.T) {
	readOnly := true
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{
		Name: "inspect",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  &readOnly,
		},
	}, func(context.Context, struct{}) (greetOut, error) {
		return greetOut{Message: "ok"}, nil
	}, kit.Renderer[greetOut]{Text: func(out greetOut) (string, error) { return out.Message, nil }})
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
	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	annotations := listed.Tools[0].Annotations
	if annotations == nil || !annotations.ReadOnlyHint || !annotations.IdempotentHint || annotations.OpenWorldHint == nil || !*annotations.OpenWorldHint {
		t.Fatalf("annotations = %+v", annotations)
	}
}

func TestMCPHeaderAnnotationDoesNotChangeCLIProjection(t *testing.T) {
	type headerInput struct {
		Region string `json:"region,omitempty" mcpheader:"Region"`
		Query  string `json:"query" cli:"positional"`
	}
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "search"},
		func(_ context.Context, in headerInput) (greetOut, error) {
			return greetOut{Message: in.Region + ":" + in.Query}, nil
		},
		kit.Renderer[greetOut]{Text: func(out greetOut) (string, error) { return out.Message, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr testWriter
	if err := kit.Run(context.Background(), r, []string{"search", "needle", "--region", "eu"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "eu:needle\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
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
	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	schemaJSON, err := json.Marshal(listed.Tools[0].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schemaJSON), `"x-mcp-header":"Region"`) {
		t.Fatalf("input schema = %s, want x-mcp-header", schemaJSON)
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
