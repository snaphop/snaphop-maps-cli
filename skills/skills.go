// Package skills holds the Agent Skill (https://agentskills.io) that teaches an AI agent to use the
// SnapHop Maps CLI. The same files are read by every client that follows the standard; the CLI
// embeds them so that `snaphop-maps skill` always matches the binary that prints it.
package skills

import "embed"

// Name is the skill's name, which is also the directory that holds it.
const Name = "snaphop-maps"

// FS holds the skill's directory, Name, and everything in it.
//
//go:embed snaphop-maps
var FS embed.FS
