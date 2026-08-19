package mcpkit_test

import (
	"context"
	"strings"
	"testing"

	kit "github.com/Kjopstad-IT/mcp-kit"
)

type testWriter struct{ strings.Builder }

func TestRunRendersHumanText(t *testing.T) {
	r := newRegistry(t)
	var stdout, stderr testWriter
	if err := kit.Run(context.Background(), r, []string{"greet", "Ada", "--loud"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "HELLO, Ada\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunRendersJSON(t *testing.T) {
	r := newRegistry(t)
	var stdout, stderr testWriter
	if err := kit.Run(context.Background(), r, []string{"greet", "Ada", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "{\"message\":\"Hello, Ada\"}\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunProjectsProgressToStderr(t *testing.T) {
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
	var stdout, stderr testWriter
	if err := kit.Run(context.Background(), r, []string{"work"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stderr.String(), "indexing 2/5\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRunJSONDoesNotEscapeHTMLCharacters(t *testing.T) {
	type output struct {
		Text string `json:"text"`
	}
	r := kit.NewRegistry()
	err := kit.Register(r, kit.Tool{Name: "show"},
		func(context.Context, struct{}) (output, error) { return output{Text: "<a&b>"}, nil },
		kit.Renderer[output]{Text: func(out output) (string, error) { return out.Text, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr testWriter
	if err := kit.Run(context.Background(), r, []string{"show", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "{\"text\":\"<a&b>\"}\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunRequiresPositionalInput(t *testing.T) {
	r := newRegistry(t)
	var stdout, stderr testWriter
	err := kit.Run(context.Background(), r, []string{"greet"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("Run error = %v, want missing name", err)
	}
}

func TestRunHonorsEndOfOptions(t *testing.T) {
	r := newRegistry(t)
	var stdout, stderr testWriter
	if err := kit.Run(context.Background(), r, []string{"greet", "--", "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "Hello, --json\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
