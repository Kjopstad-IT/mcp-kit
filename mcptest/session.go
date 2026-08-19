// Package mcptest starts black-box MCP servers and connects a typed SDK client
// to their stdio wire surface.
package mcptest

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProtocolVersion is the MCP revision required by mcp-kit.
const ProtocolVersion = "2026-07-28"

// Options configures a black-box command session.
type Options struct {
	Implementation    *mcp.Implementation
	ClientOptions     *mcp.ClientOptions
	ExpectedProtocol  string
	TerminateDuration time.Duration
}

// StartCommand starts command, performs MCP initialization over stdio, and
// verifies the negotiated protocol. The caller must close the returned session.
func StartCommand(ctx context.Context, command *exec.Cmd, options Options) (*mcp.ClientSession, error) {
	if ctx == nil {
		return nil, fmt.Errorf("mcptest: nil context")
	}
	if command == nil {
		return nil, fmt.Errorf("mcptest: nil command")
	}
	if options.Implementation == nil || options.Implementation.Name == "" || options.Implementation.Version == "" {
		return nil, fmt.Errorf("mcptest: client implementation name and version are required")
	}
	expected := options.ExpectedProtocol
	if expected == "" {
		expected = ProtocolVersion
	}
	client := mcp.NewClient(options.Implementation, options.ClientOptions)
	session, err := client.Connect(ctx, &mcp.CommandTransport{
		Command:           command,
		TerminateDuration: options.TerminateDuration,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("mcptest: connect command: %w", err)
	}
	initialized := session.InitializeResult()
	if initialized == nil || initialized.ProtocolVersion != expected {
		got := ""
		if initialized != nil {
			got = initialized.ProtocolVersion
		}
		closeErr := session.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("mcptest: negotiated protocol %q, want %q; close command: %v", got, expected, closeErr)
		}
		return nil, fmt.Errorf("mcptest: negotiated protocol %q, want %q", got, expected)
	}
	return session, nil
}
