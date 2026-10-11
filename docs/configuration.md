# Configuration

Use `snaphop-maps schema` for every command and flag, and `snaphop-maps help COMMAND` for one command.
The authoritative global flag definitions are in [`internal/cli/commands.go`](../internal/cli/commands.go).

| Setting | Default and behavior |
| --- | --- |
| `--url`, `SNAPHOP_MAPS_URL` | `https://maps.snaphop.ai`; the flag selects the service for this invocation |
| `--api-key`, `SNAPHOP_MAPS_API_KEY` | Explicit flag, then environment, then the key kept for the selected service |
| `--credentials`, `SNAPHOP_MAPS_CREDENTIALS` | `snaphop-maps/credentials.json` under the user's configuration directory |
| `--timeout` | `60s`, a positive Go duration |
| `--pretty` | Off; indent stdout JSON when set; stderr stays one JSON document per line |

An environment key is bound to the environment's service (`SNAPHOP_MAPS_URL`, or the default), not to a service
selected only by `--url`. A kept key is bound to its stored service address. A replaced environment key uses its
kept successor with `ENVIRONMENT_KEY_REPLACED`. See [SECURITY.md](../SECURITY.md) before changing these rules.
HTTPS is required except for loopback HTTP; redirects are refused.

`--args` accepts an object inline, from `@file`, or from `-` for stdin; explicit flags override its fields.
JSON input from files and stdin and network responses are bounded at 8 MiB. Relative input, credential, project
and output paths resolve against the invocation's working directory. `--no-save` leaves a newly issued key only
in the answer; `--overwrite` permits replacing a kept account. Use either deliberately.

Offline `help`, `version`, `schema` and `skill` need neither a valid URL nor a timeout. `credentials` inspects
configuration without revealing a key. Installation-specific limits and map defaults come from the service's
`guide`, `tools` and `get-installation`, not local configuration.

For development use the local plane with `--url http://localhost:8084` and a separate credentials file. Never use
production registration or publication to verify a change.
