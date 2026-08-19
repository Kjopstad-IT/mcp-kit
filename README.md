# mcp-kit

`mcp-kit` is a Go framework for defining a typed tool once and projecting it
into two surfaces:

- `mybin run <tool>` for direct CLI use;
- `mybin serve` for MCP.

The product owns its binary and mounts the components it needs. Handlers do not
receive surface information. CLI output is human text by default and JSON when
requested. The core is MIT; licensing is a separate plug.

Repeated flags populate scalar-slice inputs, and a final positional slice is
variadic. Flags for pointer scalars preserve the difference between absent and
explicitly empty values. Byte slices and nested container shapes are rejected
because they do not have an unambiguous CLI projection. Non-`omitempty` schema
fields become required CLI flags, and `--json` is reserved for output. A
renderer can also provide an exact MCP text block while the typed output
remains available as structured content.

Long-running handlers call `ReportProgress` without learning their surface.
The CLI writes progress to stderr; MCP sends a progress notification only when
the request supplies a progress token. `Renderer.MCPContent` emits image,
audio, resource-link, or mixed MCP content while `Renderer.Text` remains the
product-owned CLI policy. `--json` always emits the typed output, so the
framework never writes binary files or base64 payloads implicitly.

`Tool.Annotations` projects effects directly into MCP `ToolAnnotations`.
Fields tagged `mcpheader:"Header-Name"` add the MCP-only `x-mcp-header`
annotation to the inferred input schema. The CLI still exposes the ordinary
typed field and does not inherit HTTP-header semantics.

The first slice provides `Register`, `Run`, and `NewServer`. The stateless HTTP
spike confirms that go-sdk v1.7.0 does not emit the removed `ping` method when
protocol 2026-07-28 is active. mcp-kit rejects explicit SDK ping keepalive until
a later protocol supplies a compatible liveness mechanism.

See `examples/greet` for a product-owned binary. This repository is under
construction; `TASKS.md` is the current build ledger.
