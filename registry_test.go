package mcpkit_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	kit "github.com/Kjopstad-IT/mcp-kit"
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

type duplicateFlagsIn struct {
	First  string `cli:"target"`
	Second string `cli:"target"`
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
