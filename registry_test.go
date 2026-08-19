package mcpkit_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	kit "github.com/Kjopstad-IT/mcp-kit"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type greetIn struct {
	Name string `json:"name" cli:"positional" jsonschema:"who to greet"`
	Loud bool   `json:"loud,omitempty" cli:"loud" jsonschema:"shout the greeting"`
}

type greetOut struct {
	Message string `json:"message"`
}

func greet(_ context.Context, in greetIn) (greetOut, error) {
	message := "Hello, " + in.Name
	if in.Loud {
		message = "HELLO, " + in.Name
	}
	return greetOut{Message: message}, nil
}

func newRegistry(t *testing.T) *kit.Registry {
	t.Helper()
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{
		Name:        "greet",
		Title:       "Greet someone",
		Description: "Greet someone",
	}, greet, kit.Renderer[greetOut]{
		Text: func(out greetOut) (string, error) { return out.Message, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRegisterRejectsDuplicateName(t *testing.T) {
	r := newRegistry(t)
	err := kit.Register(r, kit.Tool{Name: "greet"}, greet, kit.Renderer[greetOut]{})
	if err == nil {
		t.Fatal("duplicate registration succeeded")
	}
}

func TestRegisterRejectsInvalidName(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "../greet"}, greet, kit.Renderer[greetOut]{})
	if err == nil {
		t.Fatal("invalid tool name succeeded")
	}
}

func TestRegisterRejectsMissingTextRenderer(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "greet"}, greet, kit.Renderer[greetOut]{})
	if err == nil || !strings.Contains(err.Error(), "text renderer") {
		t.Fatalf("Register error = %v, want text renderer rejection", err)
	}
}

func TestRegistryMiddlewareWrapsCLIHandlerInOrder(t *testing.T) {
	r := kit.NewRegistry()
	var events []string
	outer := func(spec kit.Tool, next kit.Handler) kit.Handler {
		if spec.Name != "greet" {
			t.Fatalf("middleware tool = %q, want greet", spec.Name)
		}
		return func(ctx context.Context, input any) (any, error) {
			events = append(events, "outer before")
			output, err := next(ctx, input)
			events = append(events, "outer after")
			return output, err
		}
	}
	inner := func(_ kit.Tool, next kit.Handler) kit.Handler {
		return func(ctx context.Context, input any) (any, error) {
			events = append(events, "inner before")
			output, err := next(ctx, input)
			events = append(events, "inner after")
			if err == nil {
				value := output.(greetOut)
				value.Message = "wrapped:" + value.Message
				output = value
			}
			return output, err
		}
	}
	if err := r.Use(outer, inner); err != nil {
		t.Fatal(err)
	}
	err := kit.Register(r, kit.Tool{Name: "greet"},
		func(_ context.Context, input greetIn) (greetOut, error) {
			events = append(events, "handler")
			return greet(context.Background(), input)
		},
		kit.Renderer[greetOut]{Text: func(output greetOut) (string, error) { return output.Message, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr testWriter
	if err := kit.Run(context.Background(), r, []string{"greet", "Ada"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "wrapped:Hello, Ada\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := strings.Join(events, ","), "outer before,inner before,handler,inner after,outer after"; got != want {
		t.Fatalf("events = %q, want %q", got, want)
	}
}

func TestRegistryRejectsMiddlewareAfterRegistration(t *testing.T) {
	r := newRegistry(t)
	err := r.Use(func(_ kit.Tool, next kit.Handler) kit.Handler { return next })
	if err == nil || !strings.Contains(err.Error(), "before tools") {
		t.Fatalf("Use error = %v, want ordering rejection", err)
	}
}

func TestRegistryRejectsWrongMiddlewareOutputOnJSONCLI(t *testing.T) {
	r := kit.NewRegistry()
	err := r.Use(func(_ kit.Tool, _ kit.Handler) kit.Handler {
		return func(context.Context, any) (any, error) { return "wrong type", nil }
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
	var stdout, stderr testWriter
	err = kit.Run(context.Background(), r, []string{"greet", "Ada", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "returned string") {
		t.Fatalf("Run error = %v, want output-type rejection", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no wrong-schema JSON", stdout.String())
	}
}

func TestRegistryRejectsNilMiddleware(t *testing.T) {
	err := kit.NewRegistry().Use(nil)
	if err == nil || !strings.Contains(err.Error(), "nil middleware") {
		t.Fatalf("Use error = %v, want nil rejection", err)
	}
}

func TestRegistryRejectsRegistrationAfterProjection(t *testing.T) {
	r := newRegistry(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	if err := r.AddTo(server); err != nil {
		t.Fatal(err)
	}
	err := kit.Register(r, kit.Tool{Name: "late"}, greet, kit.Renderer[greetOut]{
		Text: func(output greetOut) (string, error) { return output.Message, nil },
	})
	if err == nil || !strings.Contains(err.Error(), "already projected") {
		t.Fatalf("Register error = %v, want sealed-registry rejection", err)
	}
}

type duplicateFlagsIn struct {
	First  string `cli:"target"`
	Second string `cli:"target"`
}

type reservedFlagInput struct {
	Format string `json:"format" cli:"json"`
}

func TestRegisterRejectsReservedJSONFlag(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "reserved"},
		func(context.Context, reservedFlagInput) (greetOut, error) { return greetOut{}, nil },
		kit.Renderer[greetOut]{Text: func(greetOut) (string, error) { return "", nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "--json is reserved") {
		t.Fatalf("Register error = %v, want reserved flag rejection", err)
	}
}

type requiredFlagsInput struct {
	Name     string `json:"name"`
	Optional string `json:"optional,omitempty"`
}

func TestRunRequiresNonOmittedSchemaFlags(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "required"},
		func(_ context.Context, input requiredFlagsInput) (greetOut, error) {
			return greetOut{Message: input.Name}, nil
		},
		kit.Renderer[greetOut]{Text: func(output greetOut) (string, error) { return output.Message, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr testWriter
	err = kit.Run(context.Background(), r, []string{"required", "--optional", "ok"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "missing required flag --name") {
		t.Fatalf("Run error = %v, want required flag rejection", err)
	}
	if err := kit.Run(context.Background(), r, []string{"required", "--name", ""}, &stdout, &stderr); err != nil {
		t.Fatalf("explicit empty required flag: %v", err)
	}
}

type collectionInput struct {
	Labels []string `json:"labels,omitempty"`
	Body   *string  `json:"body,omitempty"`
}

type collectionOutput struct {
	Labels []string `json:"labels"`
	Body   *string  `json:"body"`
}

type recursiveValues []recursiveValues

type recursiveInput struct {
	Values recursiveValues `json:"values"`
}

type byteSliceInput struct {
	Data []byte `json:"data"`
}

type nestedInput struct {
	Filter struct {
		Owner string `json:"owner"`
	} `json:"filter"`
	Labels map[string]string `json:"labels,omitempty"`
}

func TestRegisterRejectsNestedCLIFieldsByDefault(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "nested"},
		func(context.Context, nestedInput) (greetOut, error) { return greetOut{}, nil },
		kit.Renderer[greetOut]{Text: func(greetOut) (string, error) { return "", nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported CLI field filter") {
		t.Fatalf("Register error = %v, want nested-field rejection", err)
	}
}

func TestMCPOnlyToolNeedsNoCLITextRendererAndIsHiddenFromRun(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "nested", MCPOnly: true},
		func(context.Context, nestedInput) (greetOut, error) { return greetOut{}, nil },
		kit.Renderer[greetOut]{},
	)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr testWriter
	err = kit.Run(context.Background(), r, []string{"nested"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), `unknown tool "nested"`) {
		t.Fatalf("Run error = %v, want MCP-only tool hidden from CLI", err)
	}
}

func TestRegisterRejectsCustomSchemaOnDualSurfaceTool(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{
		Name: "greet", MCPInputSchema: &jsonschema.Schema{Type: "object"},
	}, greet, kit.Renderer[greetOut]{
		Text: func(output greetOut) (string, error) { return output.Message, nil },
	})
	if err == nil || !strings.Contains(err.Error(), "MCP-only") {
		t.Fatalf("Register error = %v, want dual-surface custom-schema rejection", err)
	}
}

func TestRegisterRejectsInvalidCustomInputSchema(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{
		Name: "invalid", MCPOnly: true,
		MCPInputSchema: &jsonschema.Schema{Type: "object", Types: []string{"object"}},
	}, func(context.Context, map[string]any) (greetOut, error) {
		return greetOut{}, nil
	}, kit.Renderer[greetOut]{})
	if err == nil || !strings.Contains(err.Error(), "custom input schema") {
		t.Fatalf("Register error = %v, want invalid-schema rejection", err)
	}
}

func TestRegisterRejectsCustomInputSchemaWithoutObjectRoot(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{
		Name: "invalid", MCPOnly: true,
		MCPInputSchema: &jsonschema.Schema{OneOf: []*jsonschema.Schema{{Type: "object"}}},
	}, func(context.Context, map[string]any) (greetOut, error) {
		return greetOut{}, nil
	}, kit.Renderer[greetOut]{})
	if err == nil || !strings.Contains(err.Error(), `type "object"`) {
		t.Fatalf("Register error = %v, want object-root rejection", err)
	}
}

func TestRegisterRejectsInvalidHeaderInCustomInputSchema(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{
		Name: "invalid", MCPOnly: true,
		MCPInputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"token": {Type: "string", Extra: map[string]any{"x-mcp-header": "bad header"}},
			},
		},
	}, func(context.Context, map[string]any) (greetOut, error) {
		return greetOut{}, nil
	}, kit.Renderer[greetOut]{})
	if err == nil || !strings.Contains(err.Error(), "x-mcp-header") {
		t.Fatalf("Register error = %v, want header rejection", err)
	}
}

func TestRegisterRejectsInvalidHeaderInComposedCustomInputSchema(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{
		Name: "invalid", MCPOnly: true,
		MCPInputSchema: &jsonschema.Schema{
			Type: "object",
			OneOf: []*jsonschema.Schema{{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"token": {Type: "array", Extra: map[string]any{"x-mcp-header": "Authorization"}},
				},
			}},
		},
	}, func(context.Context, map[string]any) (greetOut, error) {
		return greetOut{}, nil
	}, kit.Renderer[greetOut]{})
	if err == nil || !strings.Contains(err.Error(), "x-mcp-header") {
		t.Fatalf("Register error = %v, want composed-header rejection", err)
	}
}

func TestRegisterRejectsByteSliceCLIField(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "bytes"},
		func(context.Context, byteSliceInput) (greetOut, error) { return greetOut{}, nil },
		kit.Renderer[greetOut]{Text: func(greetOut) (string, error) { return "", nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported CLI field data") {
		t.Fatalf("Register error = %v, want byte-slice rejection", err)
	}
}

type variadicInput struct {
	Prefix string   `json:"prefix" cli:"positional"`
	Values []string `json:"values" cli:"positional"`
}

func TestRunProjectsFinalSliceAsVariadicPositional(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "variadic"},
		func(_ context.Context, input variadicInput) (greetOut, error) {
			if input.Values == nil {
				return greetOut{}, fmt.Errorf("values = nil, want an explicit slice")
			}
			return greetOut{Message: input.Prefix + ":" + strings.Join(input.Values, ",")}, nil
		},
		kit.Renderer[greetOut]{Text: func(output greetOut) (string, error) { return output.Message, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr testWriter
	if err := kit.Run(context.Background(), r, []string{"variadic", "items", "one", "two"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "items:one,two\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	stdout.Reset()
	if err := kit.Run(context.Background(), r, []string{"variadic", "items"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "items:\n"; got != want {
		t.Fatalf("empty variadic stdout = %q, want %q", got, want)
	}
}

type invalidVariadicInput struct {
	Values []string `json:"values" cli:"positional"`
	Suffix string   `json:"suffix" cli:"positional"`
}

func TestRegisterRejectsNonFinalPositionalSlice(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "variadic"},
		func(context.Context, invalidVariadicInput) (greetOut, error) { return greetOut{}, nil },
		kit.Renderer[greetOut]{Text: func(greetOut) (string, error) { return "", nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "positional slice values must be last") {
		t.Fatalf("Register error = %v, want non-final positional slice rejection", err)
	}
}

func TestRegisterRejectsRecursiveCLIField(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "recursive"},
		func(context.Context, recursiveInput) (greetOut, error) { return greetOut{}, nil },
		kit.Renderer[greetOut]{Text: func(greetOut) (string, error) { return "", nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported CLI field values") {
		t.Fatalf("Register error = %v, want recursive field rejection", err)
	}
}

func TestRunProjectsRepeatedSlicesAndOptionalPointers(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "collect"},
		func(_ context.Context, input collectionInput) (collectionOutput, error) {
			if got, want := strings.Join(input.Labels, ","), "bug,urgent"; got != want {
				return collectionOutput{}, fmt.Errorf("labels = %q, want %q", got, want)
			}
			if input.Body == nil || *input.Body != "" {
				return collectionOutput{}, fmt.Errorf("body = %#v, want pointer to empty string", input.Body)
			}
			return collectionOutput(input), nil
		},
		kit.Renderer[collectionOutput]{Text: func(collectionOutput) (string, error) { return "ok", nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr testWriter
	if err := kit.Run(context.Background(), r, []string{
		"collect", "--labels", "bug", "--labels=urgent", "--body", "",
	}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "ok\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRegisterRejectsDuplicateCLIFlags(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "duplicate"},
		func(context.Context, duplicateFlagsIn) (greetOut, error) { return greetOut{}, nil },
		kit.Renderer[greetOut]{Text: func(greetOut) (string, error) { return "", nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "duplicate CLI flag") {
		t.Fatalf("Register error = %v, want duplicate flag rejection", err)
	}
}

type CommonInput struct {
	Loud bool `json:"loud,omitempty" cli:"loud"`
}

type embeddedInput struct {
	*CommonInput
	Name string `json:"name" cli:"positional"`
}

func TestRunProjectsAnonymousEmbeddedFields(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "embedded"},
		func(_ context.Context, input embeddedInput) (greetOut, error) {
			message := "Hello, " + input.Name
			if input.Loud {
				message = "HELLO, " + input.Name
			}
			return greetOut{Message: message}, nil
		},
		kit.Renderer[greetOut]{Text: func(output greetOut) (string, error) { return output.Message, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr testWriter
	if err := kit.Run(context.Background(), r, []string{"embedded", "Ada", "--loud"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "HELLO, Ada\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRendererFailurePropagates(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "greet"}, greet, kit.Renderer[greetOut]{
		Text: func(greetOut) (string, error) { return "", fmt.Errorf("render failed") },
	})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr testWriter
	err = kit.Run(context.Background(), r, []string{"greet", "Ada"}, &stdout, &stderr)
	if err == nil || err.Error() != "render failed" {
		t.Fatalf("Run error = %v, want render failed", err)
	}
}

func TestRegisterRejectsAmbiguousMCPRenderers(t *testing.T) {
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "greet"}, greet, kit.Renderer[greetOut]{
		Text:       func(out greetOut) (string, error) { return out.Message, nil },
		MCPText:    func(out greetOut) (string, error) { return out.Message, nil },
		MCPContent: func(greetOut) ([]mcp.Content, error) { return nil, nil },
	})
	if err == nil || !strings.Contains(err.Error(), "MCP renderer") {
		t.Fatalf("Register error = %v, want ambiguous MCP renderer rejection", err)
	}
}

func TestRegisterRejectsInvalidMCPHeaderName(t *testing.T) {
	type input struct {
		Region string `json:"region" mcpheader:"Bad Header"`
	}
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "greet"},
		func(context.Context, input) (greetOut, error) { return greetOut{}, nil },
		kit.Renderer[greetOut]{Text: func(greetOut) (string, error) { return "", nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "invalid mcpheader") {
		t.Fatalf("Register error = %v, want invalid header rejection", err)
	}
}

func TestRegisterRejectsInvalidMCPHeaderFieldType(t *testing.T) {
	type input struct {
		Weight float64 `json:"weight" mcpheader:"Weight"`
	}
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "greet"},
		func(context.Context, input) (greetOut, error) { return greetOut{}, nil },
		kit.Renderer[greetOut]{Text: func(greetOut) (string, error) { return "", nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "string, integer, or boolean") {
		t.Fatalf("Register error = %v, want header field type rejection", err)
	}
}

func TestRegisterRejectsShadowedEmbeddedMCPHeader(t *testing.T) {
	type Embedded struct {
		Region string `json:"region" cli:"embedded-region" mcpheader:"Region"`
	}
	type input struct {
		Embedded
		Region string `json:"region" cli:"region"`
	}
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "greet"},
		func(context.Context, input) (greetOut, error) { return greetOut{}, nil },
		kit.Renderer[greetOut]{Text: func(greetOut) (string, error) { return "", nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "shadowed or ambiguous") {
		t.Fatalf("Register error = %v, want shadowed header rejection", err)
	}
}
