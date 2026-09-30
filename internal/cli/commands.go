package cli

// The command table. Every tool of SnapHop Maps' MCP server is one command here, named as the
// tool with hyphens, whose flags are the tool's arguments; the service still decides what it
// accepts. Help, the schema command and argument parsing are all read from this table, so they
// cannot drift apart.

type kind string

const (
	kindString kind = "string"  // sent as a JSON string
	kindJSON   kind = "json"    // parsed as JSON, inline, @file or - for standard input
	kindBool   kind = "boolean" // sent only when given, so the service's default applies
	kindLocal  kind = "switch"  // read by this program, never sent

	kindInteger  kind = "integer"  // sent as a JSON number
	kindDuration kind = "duration" // a Go duration, such as 90s
)

type flagSpec struct {
	Name        string `json:"name"`
	Argument    string `json:"argument,omitempty"`
	Kind        kind   `json:"type"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description"`
}

type command struct {
	Name        string     `json:"name"`
	Tool        string     `json:"tool,omitempty"`
	Summary     string     `json:"summary"`
	Description string     `json:"description"`
	Positional  string     `json:"positional,omitempty"`
	Flags       []flagSpec `json:"flags"`
	NeedsKey    bool       `json:"needsApiKey"`
	ReadOnly    bool       `json:"readOnly"`
	Destructive bool       `json:"destructive"`
	Example     string     `json:"example"`
	// run is a command that is no tool call.
	run func(*invocation) int
}

var (
	argsFlag = flagSpec{
		Name: "args", Kind: kindJSON,
		Description: "The tool's whole argument object as JSON, inline, @file or - for standard input. Flags override its fields.",
	}
	noSaveFlag = flagSpec{
		Name: "no-save", Kind: kindLocal,
		Description: "Do not keep the key the answer carries in the credentials file. The answer on standard output is then its only copy.",
	}
	overwriteFlag = flagSpec{
		Name: "overwrite", Kind: kindLocal,
		Description: "Replace an account already kept for this service. Its key is lost unless you hold it elsewhere.",
	}
	yesFlag = flagSpec{
		Name: "yes", Kind: kindLocal,
		Description: "Confirm an action that cannot be undone.",
	}
	idFlag = flagSpec{
		Name: "id", Argument: "id", Kind: kindString, Required: true,
		Description: "The map's id, as create-map or list-maps returned it. May be given as the first positional argument instead.",
	}
	publishCreateFlag = flagSpec{
		Name: "publish", Argument: "publish", Kind: kindBool,
		Description: "Publish the map once created. Defaults to true; --publish=false keeps it a draft.",
	}
	publishUpdateFlag = flagSpec{
		Name: "publish", Argument: "publish", Kind: kindBool,
		Description: "Publish the change. Defaults to true; --publish=false only saves the draft.",
	}
)

func definitionFlags(nameRequired bool) []flagSpec {
	return []flagSpec{
		{Name: "name", Argument: "name", Kind: kindString, Required: nameRequired,
			Description: "The map's name, 1 to 120 characters of plain text."},
		{Name: "style", Argument: "style", Kind: kindString,
			Description: "The basemap style: liberty (the default), bright, positron, dark or fiord. `snaphop-maps tools` lists what the service accepts."},
		{Name: "markers", Argument: "markers", Kind: kindJSON,
			Description: `The whole list of markers as a JSON array, inline, @file or -. Each is {"position": [longitude, latitude], "title": "1 to 120 characters", "description": "at most 1000 characters", "color": "#rrggbb"}; position and title are required. Longitude comes first. Text is plain, never HTML or Markdown.`},
		{Name: "view", Argument: "view", Kind: kindJSON,
			Description: `The opening view as a JSON object: {"center": [longitude, latitude], "zoom": 0-22, "minZoom", "maxZoom", "bounds": [west, south, east, north]}. Leave it out to open on the markers.`},
		{Name: "controls", Argument: "controls", Kind: kindJSON,
			Description: `The viewer's controls as a JSON object: {"navigation": true, "scale": true, "cooperativeGestures": false}.`},
	}
}

func commands() []*command {
	return []*command{
		{
			Name: "register-agent", Tool: "register_agent",
			Summary: "Open an account; keeps its API key",
			Description: "Registers this agent: an account with a workspace of its own and one API key that creates, " +
				"publishes and withdraws maps there. The answer's apiKey is shown this once and is the account's only " +
				"credential; this command keeps it in the credentials file, readable only by you, for every later " +
				"command. Call it once, not per map. The account is deleted, and its maps taken down, after " +
				"limits.inactivityDays days without a request.",
			Flags: []flagSpec{
				{Name: "name", Argument: "name", Kind: kindString, Required: true,
					Description: "The agent's name, shown in the workspace's activity. 1 to 200 characters."},
				{Name: "workspace-name", Argument: "workspaceName", Kind: kindString,
					Description: "The workspace's name. Defaults to the agent's name."},
				argsFlag, noSaveFlag, overwriteFlag,
			},
			Example: `snaphop-maps register-agent --name "Trip planner"`,
		},
		{
			Name: "get-account", Tool: "get_account", NeedsKey: true, ReadOnly: true,
			Summary:     "Read the account the key belongs to",
			Description: "Returns the account the key belongs to: its workspace, its role and what that role permits.",
			Flags:       []flagSpec{argsFlag},
			Example:     "snaphop-maps get-account",
		},
		{
			Name: "get-installation", Tool: "get_installation", NeedsKey: true, ReadOnly: true,
			Summary: "What applies to every map here",
			Description: "Returns what applies to every map on this installation: the styles a map may choose, the " +
				"basemap a publication would use, where published maps are delivered from, and whether publication is " +
				"configured.",
			Flags:   []flagSpec{argsFlag},
			Example: "snaphop-maps get-installation",
		},
		{
			Name: "create-map", Tool: "create_map", NeedsKey: true,
			Summary: "Create a map and publish it",
			Description: "Creates a map and, unless --publish=false, publishes it. Returns the map's id and, under " +
				"published, its page link -- the address to give a person -- and embed codes. Without a view, the map " +
				"opens on all of its markers. If publication is refused, published is null, publicationError says " +
				"why, the map is kept as a draft, and the exit status is 5.",
			Flags:   append(append(definitionFlags(true), publishCreateFlag), argsFlag),
			Example: `snaphop-maps create-map --name "Coffee in Lisbon" --markers '[{"position": [-9.1427, 38.7107], "title": "A Brasileira"}]'`,
		},
		{
			Name: "list-maps", Tool: "list_maps", NeedsKey: true, ReadOnly: true,
			Summary: "List the workspace's maps",
			Description: "Lists the workspace's maps: each one's id, name, active release, whether its draft has " +
				"unpublished changes, and its marker count. Any request keeps the account from expiring.",
			Flags:   []flagSpec{argsFlag},
			Example: "snaphop-maps list-maps",
		},
		{
			Name: "get-map", Tool: "get_map", NeedsKey: true, ReadOnly: true, Positional: "id",
			Summary: "Read a map",
			Description: "Returns one map: its draft definition, draft version, and, once published, its page link " +
				"and embed codes. The draft is the whole map; create-map --args takes it back as it is.",
			Flags:   []flagSpec{idFlag, argsFlag},
			Example: "snaphop-maps get-map MAP_ID",
		},
		{
			Name: "update-map", Tool: "update_map", NeedsKey: true, Positional: "id",
			Summary: "Change a map and publish it",
			Description: "Replaces the fields given -- name, style, view, controls or the whole list of markers -- in " +
				"the map's draft, keeps the rest, and unless --publish=false publishes it. New markers without a new " +
				"view get a view that shows them. Every embed and the page link show the new release within about a " +
				"minute. If publication is refused, publicationError says why and the exit status is 5.",
			Flags:   append(append([]flagSpec{idFlag}, definitionFlags(false)...), publishUpdateFlag, argsFlag),
			Example: `snaphop-maps update-map MAP_ID --style positron`,
		},
		{
			Name: "publish-map", Tool: "publish_map", NeedsKey: true, Positional: "id",
			Summary: "Publish a map's draft",
			Description: "Publishes the map's current draft as its next release. Every embed and the page link show it " +
				"within about a minute. Publishing takes 5 to 10 seconds.",
			Flags:   []flagSpec{idFlag, argsFlag},
			Example: "snaphop-maps publish-map MAP_ID",
		},
		{
			Name: "list-releases", Tool: "list_releases", NeedsKey: true, ReadOnly: true, Positional: "id",
			Summary: "List a map's releases",
			Description: "Lists every release the map was published as, newest first: each one's number, state, style " +
				"and when it was published and last made live.",
			Flags:   []flagSpec{idFlag, argsFlag},
			Example: "snaphop-maps list-releases MAP_ID",
		},
		{
			Name: "rollback-map", Tool: "rollback_map", NeedsKey: true, Positional: "id",
			Summary: "Make an earlier release live again",
			Description: "Makes an earlier release of the map live again: its page link and every embed show it within " +
				"about a minute. The draft is left as it is. Returns the release now live. It is refused if another " +
				"release went live since the one --expected-active names, which defaults to the one live now.",
			Flags: []flagSpec{
				idFlag,
				{Name: "release", Argument: "release", Kind: kindInteger, Required: true,
					Description: "The number of the release to make live, as list-releases gave it."},
				{Name: "expected-active", Argument: "expectedActive", Kind: kindInteger,
					Description: "The release you expect to be live now; the rollback is refused if another is. Defaults to the one live when it is called."},
				argsFlag,
			},
			Example: "snaphop-maps rollback-map MAP_ID --release 1",
		},
		{
			Name: "withdraw-map", Tool: "withdraw_map", NeedsKey: true, Destructive: true, Positional: "id",
			Summary: "Take a map down for good",
			Description: "Takes a map down for good: its page link and every embed stop showing it, and it leaves the " +
				"workspace. It cannot be undone, so it needs --yes.",
			Flags:   []flagSpec{idFlag, yesFlag, argsFlag},
			Example: "snaphop-maps withdraw-map MAP_ID --yes",
		},
		{
			Name: "list-activity", Tool: "list_activity", NeedsKey: true, ReadOnly: true,
			Summary: "The workspace's recent activity",
			Description: "Lists the workspace's 50 most recent events, newest first: what was done, to what, by whom, " +
				"and when.",
			Flags:   []flagSpec{argsFlag},
			Example: "snaphop-maps list-activity",
		},
		{
			Name: "replace-key", Tool: "replace_key", NeedsKey: true,
			Summary: "Replace the API key before it expires",
			Description: "Issues a new API key with the same scopes and a new expiry, and keeps it in the credentials " +
				"file in place of the key used. Call it before the key expires: the account has no password and no " +
				"other way back in. The key used keeps working until the new one is first used, which stops it, so " +
				"if this answer is lost, run it again with the same key.",
			Flags:   []flagSpec{argsFlag, noSaveFlag},
			Example: "snaphop-maps replace-key",
		},
		{
			Name: "call", Positional: "tool",
			Summary: "Call any tool by name with JSON arguments",
			Description: "Calls one tool of the service by its MCP name with --args as its arguments, and answers as the " +
				"named commands do, keeping a key that register_agent or replace_key returns. For a tool this build " +
				"has no command for yet; `snaphop-maps tools` lists them all.",
			Flags:   []flagSpec{argsFlag, yesFlag, noSaveFlag, overwriteFlag},
			Example: `snaphop-maps call get_map --args '{"id": "MAP_ID"}'`,
			run:     runCall,
		},
		{
			Name: "skill", Positional: "action",
			Summary: "The Agent Skill for this CLI: print, install or pack it",
			Description: "Without an action, prints the Agent Skill (SKILL.md) that teaches an AI agent to use this CLI. " +
				"`skill install --client NAME` copies it where that client looks for skills: claude (Claude Code), codex " +
				"(OpenAI Codex), gemini (Gemini CLI), grok (Grok Build), cursor (Cursor), or agents (the shared " +
				".agents/skills directory). It goes under the home directory, or under --project for one project, and " +
				"replaces any copy already there. `skill pack` writes the skill as a zip to upload to claude.ai, ChatGPT " +
				"or a model API.",
			Flags: []flagSpec{
				{Name: "client", Kind: kindString,
					Description: "For install: claude, codex, gemini, grok, cursor or agents."},
				{Name: "project", Kind: kindString,
					Description: "For install: a project directory to install into instead of the home directory."},
				{Name: "output", Kind: kindString,
					Description: "For pack: the zip file to write. Defaults to " + defaultSkillZip + "."},
			},
			ReadOnly: true,
			Example:  "snaphop-maps skill install --client claude",
			run:      runSkill,
		},
		{
			Name: "tools", ReadOnly: true,
			Summary:     "List the service's tools and their JSON Schemas",
			Description: "Answers the MCP server's tools/list: every tool, its description and the JSON Schema of its arguments, as the service states them now.",
			Example:     "snaphop-maps tools",
			run:         rpcCommand("tools/list"),
		},
		{
			Name: "guide", ReadOnly: true,
			Summary:     "The service's own instructions for agents",
			Description: "Answers the MCP server's initialize: its protocol version, name and the instructions it gives every agent, with this installation's limits.",
			Example:     "snaphop-maps guide",
			run:         rpcCommand("initialize"),
		},
		{
			Name: "ping", ReadOnly: true,
			Summary:     "Check that the service answers",
			Description: "Sends the MCP server a ping. Answers {} when the service is reachable.",
			Example:     "snaphop-maps ping",
			run:         rpcCommand("ping"),
		},
		{
			Name: "credentials", ReadOnly: true,
			Summary: "Show which key would be used, never the key itself",
			Description: "Answers where the credentials file is, which key a command would send and where it comes from " +
				"(flag, environment or file), and the kept account's key id, expiry and ids. The key itself is never shown.",
			Example: "snaphop-maps credentials",
			run:     runCredentials,
		},
		{
			Name: "schema", ReadOnly: true,
			Summary:     "Describe this program as JSON",
			Description: "Answers this program's commands, flags, environment, output and exit statuses as one JSON document, for an agent to read instead of the help.",
			Example:     "snaphop-maps schema",
			run:         runSchema,
		},
		{
			Name: "version", ReadOnly: true,
			Summary:     "This program's version",
			Description: "Answers this program's version and the MCP protocol version it speaks.",
			Example:     "snaphop-maps version",
			run:         runVersion,
		},
		{
			Name: "help", ReadOnly: true, Positional: "command",
			Summary:     "Help for the program or one command",
			Description: "Explains the program, or with a command's name that command, in text.",
			Example:     "snaphop-maps help create-map",
			run:         runHelp,
		},
	}
}

// globalFlags are taken by every command.
var globalFlags = []flagSpec{
	{Name: "url", Kind: kindString,
		Description: "The SnapHop Maps service. Defaults to $SNAPHOP_MAPS_URL, then https://maps.snaphop.ai. Plain http is refused except to this machine."},
	{Name: "api-key", Kind: kindString,
		Description: "The API key to send. Defaults to $SNAPHOP_MAPS_API_KEY when --url is the environment's service, then the key kept for --url in the credentials file. A flag is visible to other processes; prefer the file or the environment."},
	{Name: "credentials", Kind: kindString,
		Description: "The credentials file. Defaults to $SNAPHOP_MAPS_CREDENTIALS, then snaphop-maps/credentials.json in the user's configuration directory."},
	{Name: "timeout", Kind: kindDuration,
		Description: "How long to wait for an answer, as a Go duration. Defaults to 60s; a publication takes 5 to 10 seconds."},
	{Name: "pretty", Kind: kindLocal,
		Description: "Indent the JSON written."},
}

type environmentVariable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var environment = []environmentVariable{
	{"SNAPHOP_MAPS_URL", "The service, when --url is not given."},
	{"SNAPHOP_MAPS_API_KEY", "The API key, when --api-key is not given. It takes precedence over the credentials file, and is only sent to $SNAPHOP_MAPS_URL's service, or the default when that is not set: never to another that --url names."},
	{"SNAPHOP_MAPS_CREDENTIALS", "The credentials file, when --credentials is not given."},
}

type exitStatus struct {
	Code    int    `json:"code"`
	Meaning string `json:"meaning"`
}

const (
	exitOK          = 0
	exitFailure     = 1
	exitUsage       = 2
	exitRefused     = 3
	exitTransport   = 4
	exitUnpublished = 5
	exitNotSaved    = 6
)

var exitStatuses = []exitStatus{
	{exitOK, "Done. Standard output holds the answer."},
	{exitFailure, "This program could not do its own part, such as reading the credentials file. Nothing was sent."},
	{exitUsage, "The command line is wrong or incomplete. Nothing was sent. error.code says what to change."},
	{exitRefused, "The service refused the request and changed nothing. error.code is the service's code and error.hint the next step."},
	{exitTransport, "The exchange failed. Unless error.outcomeKnown is true, the request may have been carried out; error.hint says what to check before repeating it."},
	{exitUnpublished, "The map was saved but its publication was refused. Standard output holds the map, with publicationError."},
	{exitNotSaved, "A new API key was issued but not kept in the credentials file. Standard output holds it, and is its only copy: keep it."},
}
