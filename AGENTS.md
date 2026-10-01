# mcp-kit

Build one typed Go tool definition into CLI and MCP surfaces. Read `TASKS.md`
before changing the tree and work one unchecked slice at a time.

The handler stays surface-neutral. The product owns `main`; this module gives
it registry, CLI-runner, and MCP-server components. Core code is MIT. License
gating and signing stay outside this repository.

Run `go test ./...`, `go vet ./...`, and `go build ./...` before opening a PR.
Use a branch and PR for all work after the repository's initial commit.

## Kontrollrom work

This repository is connected to Kontrollrom (project `agentverk`). Before
taking, resuming, delegating, reviewing or finishing work here, read the
installed `kontrollrom-work` skill; use `kontrollrom-cli` for command details.
Before building, running the full suite or reviewing delegated work, read the
installed `dispatch` skill's `references/review-flow.md`. If they are missing,
run the bootstrap recipe in `Kjopstad-IT/kontrollrom` `docs/cli-distribution.md`
("Bootstrap without an installed CLI"). Canonical sources are in
`Kjopstad-IT/hq` (`system/skills/kontrollrom-work/`, `.claude/skills/dispatch/`).
