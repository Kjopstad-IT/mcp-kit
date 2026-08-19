package mcpkit

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var toolNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Tool describes one handler projected into CLI and MCP surfaces. MCPOnly
// keeps a tool off the CLI when its typed input has no unambiguous CLI shape.
type Tool struct {
	Name        string
	Title       string
	Description string
	Annotations *mcp.ToolAnnotations
	MCPOnly     bool
}

// Renderer defines the surface renderers for a typed output. Text is the
// human-readable CLI projection. JSON CLI output and MCP structured content
// are always derived from the typed value. MCPText optionally supplies an
// exact text block. MCPContent supplies image, audio, resource-link, or mixed
// MCP blocks. Set at most one MCP renderer.
type Renderer[Out any] struct {
	Text       func(Out) (string, error)
	MCPText    func(Out) (string, error)
	MCPContent func(Out) ([]mcp.Content, error)
}

// Handler is the type-erased form of a registered typed handler. Middleware
// may inspect or replace input and output values, but should preserve their
// registered Go types unless it deliberately wants type validation to fail.
type Handler func(context.Context, any) (any, error)

// Middleware wraps every subsequently registered handler. The Tool value lets
// a product select policy by tool identity without exposing the active surface
// to the handler. Middleware added first runs outermost.
type Middleware func(Tool, Handler) Handler

// Registry owns a generation of tool definitions.
type Registry struct {
	mu         sync.RWMutex
	tools      map[string]*registeredTool
	order      []string
	middleware []Middleware
	frozen     bool
}

type registeredTool struct {
	spec       Tool
	handle     Handler
	invoke     func(context.Context, []string) (any, error)
	renderText func(any) (string, error)
	addTo      func(*mcp.Server)
}

// NewRegistry returns an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]*registeredTool)}
}

// Use appends registry-wide middleware. Middleware must be installed before
// the first tool registration so every surface receives the same chain.
func (registry *Registry) Use(middleware ...Middleware) error {
	if registry == nil {
		return fmt.Errorf("mcp-kit: nil registry")
	}
	for _, candidate := range middleware {
		if candidate == nil {
			return fmt.Errorf("mcp-kit: nil middleware")
		}
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.frozen {
		return fmt.Errorf("mcp-kit: middleware must be installed before tools are registered")
	}
	registry.middleware = append(registry.middleware, middleware...)
	return nil
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
	if !spec.MCPOnly && renderer.Text == nil {
		return fmt.Errorf("mcp-kit: tool %q has no text renderer", spec.Name)
	}
	if renderer.MCPText != nil && renderer.MCPContent != nil {
		return fmt.Errorf("mcp-kit: tool %q has more than one MCP renderer", spec.Name)
	}
	var parser *cliParser[In]
	if !spec.MCPOnly {
		var err error
		parser, err = buildCLIParser[In]()
		if err != nil {
			return fmt.Errorf("mcp-kit: tool %q: %w", spec.Name, err)
		}
	}
	inputSchema, err := inputSchemaFor[In]()
	if err != nil {
		return fmt.Errorf("mcp-kit: tool %q: %w", spec.Name, err)
	}
	middleware := registry.freezeMiddleware()

	entry := &registeredTool{spec: spec}
	entry.handle = func(ctx context.Context, input any) (any, error) {
		typedInput, ok := input.(In)
		if !ok {
			return nil, fmt.Errorf("mcp-kit: tool %q received %T", spec.Name, input)
		}
		return handler(ctx, typedInput)
	}
	for i := len(middleware) - 1; i >= 0; i-- {
		entry.handle = middleware[i](spec, entry.handle)
		if entry.handle == nil {
			return fmt.Errorf("mcp-kit: tool %q middleware returned a nil handler", spec.Name)
		}
	}
	if !spec.MCPOnly {
		entry.invoke = func(ctx context.Context, args []string) (any, error) {
			input, err := parser.parse(args)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", spec.Name, err)
			}
			return entry.handle(ctx, input)
		}
		entry.renderText = func(value any) (string, error) {
			output, ok := value.(Out)
			if !ok {
				return "", fmt.Errorf("mcp-kit: tool %q returned %T", spec.Name, value)
			}
			return renderer.Text(output)
		}
	}
	entry.addTo = func(server *mcp.Server) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        spec.Name,
			Title:       spec.Title,
			Description: spec.Description,
			Annotations: spec.Annotations,
			InputSchema: inputSchema,
		}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
			var zero Out
			if token := request.Params.GetProgressToken(); token != nil {
				ctx = withProgressReporter(ctx, func(ctx context.Context, progress Progress) error {
					return request.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
						ProgressToken: token,
						Message:       progress.Message,
						Progress:      progress.Current,
						Total:         progress.Total,
					})
				})
			}
			value, err := entry.handle(ctx, input)
			if err != nil {
				return nil, zero, err
			}
			output, ok := value.(Out)
			if !ok {
				return nil, zero, fmt.Errorf("mcp-kit: tool %q returned %T", spec.Name, value)
			}
			if renderer.MCPContent != nil {
				content, renderErr := renderer.MCPContent(output)
				if renderErr != nil {
					return nil, output, renderErr
				}
				return &mcp.CallToolResult{Content: content}, output, nil
			}
			if renderer.MCPText != nil {
				text, renderErr := renderer.MCPText(output)
				if renderErr != nil {
					return nil, output, renderErr
				}
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: text}},
				}, output, nil
			}
			return nil, output, nil
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

func (registry *Registry) freezeMiddleware() []Middleware {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.frozen = true
	return append([]Middleware(nil), registry.middleware...)
}

func (registry *Registry) lookup(name string) (*registeredTool, bool) {
	if registry == nil {
		return nil, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	tool, ok := registry.tools[name]
	return tool, ok && !tool.spec.MCPOnly
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
