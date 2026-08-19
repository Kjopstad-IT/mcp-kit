package mcpkit

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var toolNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Tool describes one handler projected into CLI and MCP surfaces.
type Tool struct {
	Name        string
	Title       string
	Description string
	Annotations *mcp.ToolAnnotations
}

// Renderer defines the human-readable CLI projection for a typed output.
// JSON output is always derived from the typed value.
type Renderer[Out any] struct {
	Text func(Out) (string, error)
}

// Registry owns a generation of tool definitions.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*registeredTool
	order []string
}

type registeredTool struct {
	spec       Tool
	invoke     func(context.Context, []string) (any, error)
	renderText func(any) (string, error)
	addTo      func(*mcp.Server)
}

// NewRegistry returns an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]*registeredTool)}
}

// Register adds one typed, surface-neutral handler to a registry.
func Register[In, Out any](
	registry *Registry,
	spec Tool,
	handler func(context.Context, In) (Out, error),
	renderer Renderer[Out],
) error {
	if registry == nil {
		return fmt.Errorf("mcp-kit: nil registry")
	}
	if !toolNamePattern.MatchString(spec.Name) {
		return fmt.Errorf("mcp-kit: invalid tool name %q", spec.Name)
	}
	if handler == nil {
		return fmt.Errorf("mcp-kit: tool %q has nil handler", spec.Name)
	}
	if renderer.Text == nil {
		return fmt.Errorf("mcp-kit: tool %q has no text renderer", spec.Name)
	}
	parser, err := buildCLIParser[In]()
	if err != nil {
		return fmt.Errorf("mcp-kit: tool %q: %w", spec.Name, err)
	}

	entry := &registeredTool{spec: spec}
	entry.invoke = func(ctx context.Context, args []string) (any, error) {
		input, err := parser.parse(args)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec.Name, err)
		}
		return handler(ctx, input)
	}
	entry.renderText = func(value any) (string, error) {
		output, ok := value.(Out)
		if !ok {
			return "", fmt.Errorf("mcp-kit: tool %q returned %T", spec.Name, value)
		}
		return renderer.Text(output)
	}
	entry.addTo = func(server *mcp.Server) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        spec.Name,
			Title:       spec.Title,
			Description: spec.Description,
			Annotations: spec.Annotations,
		}, func(ctx context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
			output, err := handler(ctx, input)
			return nil, output, err
		})
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.tools[spec.Name]; exists {
		return fmt.Errorf("mcp-kit: tool %q is already registered", spec.Name)
	}
	registry.tools[spec.Name] = entry
	registry.order = append(registry.order, spec.Name)
	return nil
}

func (registry *Registry) lookup(name string) (*registeredTool, bool) {
	if registry == nil {
		return nil, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	tool, ok := registry.tools[name]
	return tool, ok
}

func (registry *Registry) snapshot() []*registeredTool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	tools := make([]*registeredTool, 0, len(registry.order))
	for _, name := range registry.order {
		tools = append(tools, registry.tools[name])
	}
	return tools
}
