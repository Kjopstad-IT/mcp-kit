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

func TestRunRequiresPositionalInput(t *testing.T) {
	r := newRegistry(t)
	var stdout, stderr testWriter
	err := kit.Run(context.Background(), r, []string{"greet"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("Run error = %v, want missing name", err)
	}
}
