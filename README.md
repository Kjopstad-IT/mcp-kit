# mcp-kit

`mcp-kit` is a Go framework for defining a typed tool once and projecting it
into two surfaces:

- `mybin run <tool>` for direct CLI use;
- `mybin serve` for MCP.

The product owns its binary and mounts the components it needs. Handlers do not
receive surface information. CLI output is human text by default and JSON when
requested. The core is MIT; licensing is a separate plug.

Repeated flags populate slice inputs, and flags for pointer scalars preserve
the difference between absent and explicitly empty values. A renderer can also
provide an exact MCP text block while the typed output remains available as
structured content.

The first slice provides `Register`, `Run`, and `NewServer`. The stateless HTTP
spike confirms that go-sdk v1.7.0 does not emit the removed `ping` method when
protocol 2026-07-28 is active. mcp-kit rejects explicit SDK ping keepalive until
a later protocol supplies a compatible liveness mechanism.

See `examples/greet` for a product-owned binary. This repository is under
construction; `TASKS.md` is the current build ledger.
