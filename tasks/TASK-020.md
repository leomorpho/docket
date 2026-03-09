# TASK-020: `docket help-json` — machine-readable manifest

## Status
`[ ]` not started

## Depends on
TASK-019

## What this is
`docket help-json` outputs a machine-readable JSON manifest of all commands, their flags,
expected input/output shapes, and usage examples. This is the primary discovery surface
for LLMs that call docket as a shell tool (without MCP).

## Why
When an LLM is told "you can use the `docket` CLI", it needs to know what commands exist
and how to call them. Embedding `docket help-json` output once in the system prompt is
cheaper and more reliable than an MCP tool description.

## Command signature
```
docket help-json
```

No flags. Always outputs JSON. Always exits 0.

## Output format

```json
{
  "binary": "docket",
  "version": "0.1.0",
  "description": "git-native ticket system for AI-assisted development",
  "global_flags": {
    "--format": {"type": "string", "values": ["human", "json"], "default": "human"},
    "--repo": {"type": "string", "default": "current working directory"}
  },
  "commands": [
    {
      "name": "create",
      "synopsis": "docket create --title <string> [flags]",
      "description": "Create a new ticket in backlog",
      "flags": {
        "--title": {"type": "string", "required": true},
        "--desc": {"type": "string", "description": "use - for stdin"},
        "--priority": {"type": "int", "default": 10},
        "--labels": {"type": "csv"},
        "--state": {"type": "string", "values": ["backlog","todo","in-progress","in-review","done"]}
      },
      "output": {
        "human": "Created TKT-001: <title>",
        "json": {"id": "string", "seq": "int", "title": "string", "state": "string"}
      },
      "examples": [
        "docket create --title 'Add auth middleware' --priority 1 --labels feature",
        "echo 'Long description' | docket create --title 'Fix bug' --desc -"
      ]
    }
  ],
  "environment": {
    "DOCKET_ACTOR": "Set actor identity, e.g. 'agent:claude-sonnet-4-6'. Falls back to git config user.name."
  },
  "conventions": {
    "ticket_id_format": "TKT-NNN (e.g. TKT-001, TKT-042)",
    "commit_trailer": "Add 'Ticket: TKT-NNN' to commit messages to link work",
    "inline_annotation": "Add '// [TKT-NNN] reason' in source code for explicit markers",
    "actor_format": "'human:name' or 'agent:model-id'"
  }
}
```

Include ALL commands from TASK-007 through TASK-021.

## File to create
`cmd/helpjson.go`

The command can build this manifest by iterating cobra's command tree (each command
has `Use`, `Short`, and flags already defined) and encoding them. The `examples` and
`output` shapes must be hardcoded per command — cobra doesn't know those.

## Acceptance criteria
- [ ] `docket help-json` exits 0 and outputs valid JSON
- [ ] Output includes all commands implemented so far
- [ ] Each command entry has `synopsis`, `flags`, `examples`, and `output` shapes
- [ ] `conventions` section explains commit trailers and inline annotations
- [ ] `environment` section lists `DOCKET_ACTOR`
- [ ] `docket help-json | jq .commands[].name` lists all command names

## Notes for LLM
- Keep this command simple — no fancy generation needed, mostly hardcoded schema
- This output will be embedded in LLM system prompts, so make descriptions concise and accurate
- Version should come from a `var Version = "0.1.0"` in `cmd/root.go` or a `version.go` file
