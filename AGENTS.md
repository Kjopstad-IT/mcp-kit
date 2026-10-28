# mcp-kit

Build one typed Go tool definition into CLI and MCP surfaces. Read `TASKS.md`
before changing the tree and work one unchecked slice at a time.

The handler stays surface-neutral. The product owns `main`; this module gives
it registry, CLI-runner, and MCP-server components. Core code is MIT. License
gating and signing stay outside this repository.

Run `go test ./...`, `go vet ./...`, and `go build ./...` before opening a PR.
Use a branch and PR for all work after the repository's initial commit.
