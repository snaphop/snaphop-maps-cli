package cli

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/snaphop/snaphop-maps-cli/internal/mcp"
)

const about = `snaphop-maps publishes interactive web maps with plain-text markers at a stable link and as
embeddable HTML, through SnapHop Maps (https://maps.snaphop.ai). It is built for AI agents: each
command is one tool of the service's MCP server, standard output is one JSON document, and a
failure is one JSON error on standard error with an exit status saying what kind it was.`

const start = `Start:
  snaphop-maps register-agent --name "Your agent"   # once: opens an account and keeps its key
  snaphop-maps create-map --name "Coffee in Lisbon" \
    --markers '[{"position": [-9.1427, 38.7107], "title": "A Brasileira"}]'
  # published.page in the answer is the link to give a person.

Positions are [longitude, latitude]: longitude first. Text is plain, never HTML or Markdown.
The account is deleted, and its maps taken down, after limits.inactivityDays days without a
request; get-map returns a map's whole draft, to keep what you need. Replace the key with
replace-key before its expiresAt. ` + "`snaphop-maps schema`" + ` describes all of this as JSON.`

// overview is the program's help.
func overview() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n%s\n\nCommands:\n", about, start)
	for _, cmd := range commands() {
		fmt.Fprintf(&b, "  %-17s %s\n", cmd.Name, cmd.Summary)
	}
	b.WriteString("\nGlobal flags, taken before or after the command:\n")
	writeFlags(&b, globalFlags)
	b.WriteString("\nEnvironment:\n")
	for _, variable := range environment {
		fmt.Fprintf(&b, "  %s\n      %s\n", variable.Name, variable.Description)
	}
	b.WriteString("\nExit statuses:\n")
	for _, status := range exitStatuses {
		fmt.Fprintf(&b, "  %d  %s\n", status.Code, status.Meaning)
	}
	b.WriteString("\nRun `snaphop-maps help COMMAND` for one command.\n")
	return b.String()
}

// commandHelp is one command's help.
func commandHelp(cmd *command) string {
	var b strings.Builder
	line := "snaphop-maps " + cmd.Name
	if cmd.Positional != "" {
		line += " [" + strings.ToUpper(cmd.Positional) + "]"
	}
	fmt.Fprintf(&b, "Usage: %s [flags]\n\n%s\n", line, cmd.Description)
	if cmd.Tool != "" {
		fmt.Fprintf(&b, "\nMCP tool: %s\n", cmd.Tool)
	}
	if len(cmd.Flags) > 0 {
		b.WriteString("\nFlags:\n")
		writeFlags(&b, cmd.Flags)
	}
	fmt.Fprintf(&b, "\nExample:\n  %s\n\nGlobal flags: --url, --api-key, --credentials, --timeout, --pretty. See `snaphop-maps help`.\n", cmd.Example)
	return b.String()
}

func writeFlags(b *strings.Builder, flags []flagSpec) {
	for _, spec := range flags {
		name := "--" + spec.Name
		if spec.Kind != kindLocal && spec.Kind != kindBool {
			name += " " + string(spec.Kind)
		}
		if spec.Required {
			name += " (required)"
		}
		fmt.Fprintf(b, "  %s\n      %s\n", name, spec.Description)
	}
}

func runHelp(inv *invocation) int {
	if len(inv.positional) == 0 {
		fmt.Fprint(inv.env.Stdout, overview())
		return exitOK
	}
	cmd := lookup(inv.positional[0])
	if cmd == nil {
		return inv.usage("UNKNOWN_COMMAND", fmt.Sprintf("There is no command %q.", inv.positional[0]), "Run `snaphop-maps help` for every command.")
	}
	fmt.Fprint(inv.env.Stdout, commandHelp(cmd))
	return exitOK
}

func runSchema(inv *invocation) int {
	document, _ := json.Marshal(map[string]any{
		"name":            "snaphop-maps",
		"version":         inv.env.Version,
		"description":     strings.Join(strings.Fields(about), " "),
		"protocolVersion": mcp.ProtocolVersion,
		"defaultUrl":      DefaultURL,
		"commands":        commands(),
		"globalFlags":     globalFlags,
		"environment":     environment,
		"exitStatuses":    exitStatuses,
		"skillClients":    skillClients,
		"output": map[string]string{
			"stdout":  "On success, one JSON document: the tool's structured result, exactly as the service gave it.",
			"stderr":  `On failure, {"error": {"code", "message", "hint", ...}}; a warning is {"warning": {...}}. One JSON document per line.`,
			"secrets": "register-agent and replace-key print the new apiKey on standard output and keep it in the credentials file; no other output holds a key.",
		},
	})
	inv.write(inv.env.Stdout, document)
	return exitOK
}

func runVersion(inv *invocation) int {
	document, _ := json.Marshal(map[string]string{
		"name":            "snaphop-maps",
		"version":         inv.env.Version,
		"protocolVersion": mcp.ProtocolVersion,
	})
	inv.write(inv.env.Stdout, document)
	return exitOK
}

// Version is the program's version: the one set at build time, or else the module version that
// `go install` recorded, or else "dev".
func Version(stamped string, buildInfo func() (*debug.BuildInfo, bool)) string {
	if stamped != "" && stamped != "dev" {
		return stamped
	}
	if info, ok := buildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
