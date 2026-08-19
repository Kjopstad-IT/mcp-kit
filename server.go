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
	if err := registry.AddTo(server); err != nil {
		return nil, err
	}
	return server, nil
}

// AddTo projects every registered tool onto an existing SDK server. The first
// projection seals the registry so a later registration cannot be omitted from
// an already-mounted server. A sealed registry may be projected onto more than
// one server.
func (registry *Registry) AddTo(server *mcp.Server) error {
	if registry == nil {
		return fmt.Errorf("mcp-kit: nil registry")
	}
	if server == nil {
		return fmt.Errorf("mcp-kit: nil MCP server")
	}
	for _, tool := range registry.projectionSnapshot() {
		tool.addTo(server)
	}
	return nil
}
