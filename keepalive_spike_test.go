package mcpkit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSDKStatelessKeepAliveDoesNotEmitRemovedPingOn20260728(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "spike", Version: "0.1.0"},
		&mcp.ServerOptions{KeepAlive: 20 * time.Millisecond},
	)
	var pings atomic.Int64
	client := mcp.NewClient(
		&mcp.Implementation{Name: "spike-client", Version: "0.1.0"},
		nil,
	)
	client.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "ping" {
				pings.Add(1)
			}
			return next(ctx, method, req)
		}
	})

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	time.Sleep(80 * time.Millisecond)
	if got := pings.Load(); got != 0 {
		t.Fatalf("stateless keepalive emitted removed ping: attempts = %d", got)
	}
}
