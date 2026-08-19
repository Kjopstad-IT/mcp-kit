package main

import (
	"context"
	"fmt"
	"os"

	kit "github.com/Kjopstad-IT/mcp-kit"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type greetIn struct {
	Name string `json:"name" cli:"positional" jsonschema:"who to greet"`
	Loud bool   `json:"loud,omitempty" cli:"loud" jsonschema:"shout the greeting"`
}

type greetOut struct {
	Message string `json:"message"`
}

func main() {
	registry := kit.NewRegistry()
	err := kit.Register(
		registry,
		kit.Tool{Name: "greet", Title: "Greet someone", Description: "Greet someone"},
		func(_ context.Context, input greetIn) (greetOut, error) {
			message := "Hello, " + input.Name
			if input.Loud {
				message = "HELLO, " + input.Name
			}
			return greetOut{Message: message}, nil
		},
		kit.Renderer[greetOut]{Text: func(output greetOut) (string, error) {
			return output.Message, nil
		}},
	)
	if err != nil {
		fail(err)
	}
	if len(os.Args) < 2 {
		fail(fmt.Errorf("usage: greet run <tool> | greet serve"))
	}

	switch os.Args[1] {
	case "run":
		err = kit.Run(context.Background(), registry, os.Args[2:], os.Stdout, os.Stderr)
	case "serve":
		var server *mcp.Server
		server, err = kit.NewServer(
			registry,
			&mcp.Implementation{Name: "greet", Version: "0.1.0"},
			nil,
		)
		if err == nil {
			err = server.Run(context.Background(), &mcp.StdioTransport{})
		}
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
