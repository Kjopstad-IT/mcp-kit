package mcpkit

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewServer projects every registered tool onto an MCP server and returns the
// underlying SDK server so products can add SDK-native features when needed.
func NewServer(
	registry *Registry,
	implementation *mcp.Implementation,
	options *mcp.ServerOptions,
) (*mcp.Server, error) {
	if registry == nil {
		return nil, fmt.Errorf("mcp-kit: nil registry")
	}
	if implementation == nil || implementation.Name == "" || implementation.Version == "" {
		return nil, fmt.Errorf("mcp-kit: implementation name and version are required")
	}
	if options != nil && options.KeepAlive != 0 {
		return nil, fmt.Errorf("mcp-kit: ServerOptions.KeepAlive uses the removed ping method under protocol 2026-07-28")
	}
	server := mcp.NewServer(implementation, options)
	for _, tool := range registry.snapshot() {
		tool.addTo(server)
	}
	return server, nil
}
