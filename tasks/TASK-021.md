# TASK-021: MCP server

## Status
`[ ]` not started

## Depends on
TASK-020

## What this is
`docket serve --mcp` starts a JSON-RPC MCP server over stdin/stdout. Thin wrapper over
the same `internal/` library functions the CLI uses. No business logic here — just protocol.

## Why MCP in addition to CLI?
Some LLM environments (e.g. Claude Desktop, certain agent frameworks) call tools via MCP
protocol rather than shell execution. Having both surfaces from one binary means no
duplication of logic and no schema drift between them.

## Command signature
```
docket serve --mcp [--repo <path>]
```

Reads JSON-RPC from stdin, writes responses to stdout. Runs until stdin closes.

## MCP tool definition

The server exposes a single tool named `docket` with an `action` parameter that maps
to CLI subcommands. This keeps the MCP tool description small (important — MCP descriptions
consume tokens on every LLM call).

```json
{
  "name": "docket",
  "description": "git-native ticket system. Run `docket help-json` for full command reference.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "action": {
        "type": "string",
        "description": "CLI subcommand (e.g. 'create', 'list', 'show', 'update', 'comment', 'ac check', 'validate', 'check')"
      },
      "args": {
        "type": "object",
        "description": "Flags as key-value pairs matching the CLI flags (e.g. {\"title\": \"Add auth\", \"priority\": 1})"
      }
    },
    "required": ["action"]
  }
}
```

## Internal routing

`action` maps to the same library functions as the CLI:

```go
func dispatch(action string, args map[string]interface{}, repoRoot string) (interface{}, error) {
    switch action {
    case "create":
        return handleCreate(args, repoRoot)
    case "list":
        return handleList(args, repoRoot)
    // ...
    }
}
```

Each handler calls the same `internal/` functions as the cobra commands — the cobra layer
is NOT invoked (no flag parsing overhead, no stdout capture needed).

## Output
MCP responses are always JSON. Errors returned as MCP error responses with clear messages.

## Files to create
- `internal/mcp/server.go` — JSON-RPC loop, dispatch table
- `internal/mcp/handlers.go` — one handler per action, calling `internal/` functions
- `cmd/serve.go` — cobra command that starts the MCP server

## Acceptance criteria
- [ ] `docket serve --mcp` starts and reads from stdin without error
- [ ] Sending `{"action": "list", "args": {"state": "open"}}` returns ticket list as JSON
- [ ] Sending `{"action": "create", "args": {"title": "Test"}}` creates a ticket
- [ ] Sending unknown action returns a JSON error (not a crash)
- [ ] Server exits cleanly when stdin closes
- [ ] All handlers call the same `internal/` functions as CLI commands (verify by code review — no duplication)
- [ ] `go test ./internal/mcp/...` passes with stdin/stdout mocking

## Notes for LLM
- Use `bufio.Scanner` on `os.Stdin` to read one JSON object per line
- Each line is a complete JSON-RPC request; respond with one JSON line per request
- Do NOT use cobra in the MCP handlers — call library functions directly
- The MCP tool description should remain minimal — point to `docket help-json` for full schema
