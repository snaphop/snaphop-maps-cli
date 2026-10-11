# Command-line design contract

The CLI's interface is designed for agents that execute commands and read JSON. Its design lives in
[`internal/cli/commands.go`](internal/cli/commands.go), rather than a browser stylesheet or component library.
Follow [REQUIREMENTS.md](REQUIREMENTS.md) and [SECURITY.md](SECURITY.md) when changing it.

## Commands and arguments

Use the MCP tool's name with hyphens for a named command and expose its arguments as flags in the command table.
Keep parsing, help, examples and `schema` in step through that table. Required identifiers can be positional where
specified; conflicting identifiers fail instead of choosing one silently. JSON flags accept inline JSON, `@file`
and `-`; explicit flags override fields from `--args`. Write boolean values as `--publish=false`, not as a
separate word. Keep local safety switches out of tool arguments.

## Output and recovery

Service successes are the service's structured JSON result. `--pretty` changes indentation. Local help, version,
schema and credential inspection produce JSON; `skill` prints its Markdown, and install and pack return JSON.
Errors and warnings remain one JSON document per stderr line even with `--pretty`. Keep the existing exit statuses
and code meanings stable; add a hint that gives the agent a concrete next action. Report uncertain outcomes
honestly, including on timeouts and unreadable responses. A saved draft with refused publication remains available
on stdout with exit status 5. A newly issued key that cannot be kept is returned on stdout with exit status 6;
output failure uses exit status 7 and explains whether the key was kept.

## Safety and discoverability

Do not echo keys in help errors, warnings or diagnostics. `credentials` describes the source without revealing
the key. Withdrawal and member removal require `--yes`. Registration replacing an account requires `--overwrite`.
Keep local commands usable offline, even with invalid service configuration.

Update [`skills/snaphop-maps/SKILL.md`](skills/snaphop-maps/SKILL.md) alongside any interface behavior an assistant
relies on. Its examples are executed by tests. Repository review skills under `.agents/skills/` support development
and are separate from that embedded user skill. The CLI has no browser UI or design-system update workflow.
