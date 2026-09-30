// Package cli is the snaphop-maps command line: SnapHop Maps' MCP tools as commands for AI
// agents. Every command writes one JSON document to standard output on success, and one JSON
// error to standard error otherwise, with an exit status that says which kind of failure it was.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/snaphop/snaphop-maps-cli/internal/credentials"
	"github.com/snaphop/snaphop-maps-cli/internal/mcp"
)

// DefaultURL is the SnapHop Maps service commands go to unless told otherwise.
const DefaultURL = "https://maps.snaphop.ai"

// DefaultTimeout is how long a command waits for an answer: a publication takes 5 to 10 seconds,
// and the service's guide asks for at least 30.
const DefaultTimeout = 60 * time.Second

// expiryWarning is how long before a kept key expires that every command says so.
const expiryWarning = 7 * 24 * time.Hour

// maxInputBytes bounds a JSON argument read from a file or standard input.
const maxInputBytes = 8 << 20

// Env is everything a run reads from and writes to, so that tests can supply their own.
type Env struct {
	Args      []string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	Getenv    func(string) string
	ConfigDir func() (string, error)
	HomeDir   func() (string, error)
	// Dir is the working directory relative paths are read against; empty means the process's.
	Dir string
	// HTTP sends requests; nil means a client that follows no redirects.
	HTTP    *http.Client
	Now     func() time.Time
	Version string
}

// Problem is a failure reported to the caller as {"error": Problem}.
type Problem struct {
	Code         string          `json:"code"`
	Message      string          `json:"message"`
	Fields       json.RawMessage `json:"fields,omitempty"`
	Hint         string          `json:"hint,omitempty"`
	Status       int             `json:"status,omitempty"`
	RetryAfter   string          `json:"retryAfter,omitempty"`
	OutcomeKnown *bool           `json:"outcomeKnown,omitempty"`
	Detail       json.RawMessage `json:"detail,omitempty"`
}

type invocation struct {
	env        Env
	ctx        context.Context
	cmd        *command
	fs         *flag.FlagSet
	positional []string
	set        map[string]bool
	strings    map[string]*string
	switches   map[string]*bool
	pretty     bool
	service    string
	timeout    time.Duration
	stdinUsed  bool
}

// Run runs one command line and returns its exit status.
func Run(ctx context.Context, env Env) int {
	if env.HTTP == nil {
		env.HTTP = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	name, rest := splitCommand(env.Args)
	if name == "" {
		fmt.Fprint(env.Stdout, overview())
		return exitOK
	}
	cmd := lookup(name)
	if cmd == nil {
		return usage(env, "UNKNOWN_COMMAND", fmt.Sprintf("There is no command %q.", name),
			"Run `snaphop-maps schema` for every command as JSON, or `snaphop-maps help`.")
	}
	inv := &invocation{env: env, ctx: ctx, cmd: cmd}
	if code, ok := inv.parse(rest); !ok {
		return code
	}
	if cmd.run != nil {
		return cmd.run(inv)
	}
	return runTool(inv)
}

// splitCommand finds the command's name, which global flags may come before, and returns the
// rest of the line without it.
func splitCommand(args []string) (string, []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			return arg, append(append([]string{}, args[:i]...), args[i+1:]...)
		}
		name := strings.TrimLeft(arg, "-")
		for _, spec := range globalFlags {
			if spec.Name == name && spec.Kind != kindLocal {
				i++ // its value
			}
		}
	}
	return "", nil
}

// lookup finds a command by its name, or by the name of the tool it calls.
func lookup(name string) *command {
	for _, cmd := range commands() {
		if cmd.Name == name || (cmd.Tool != "" && cmd.Tool == name) {
			return cmd
		}
	}
	return nil
}

// parse reads the command's flags, which may come before or after its positional argument.
func (inv *invocation) parse(args []string) (int, bool) {
	fs := flag.NewFlagSet(inv.cmd.Name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	inv.fs = fs
	inv.strings = map[string]*string{}
	inv.switches = map[string]*bool{}
	for _, spec := range append(append([]flagSpec{}, globalFlags...), inv.cmd.Flags...) {
		switch spec.Kind {
		case kindLocal, kindBool:
			inv.switches[spec.Name] = fs.Bool(spec.Name, false, spec.Description)
		default:
			inv.strings[spec.Name] = fs.String(spec.Name, "", spec.Description)
		}
	}
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				fmt.Fprint(inv.env.Stdout, commandHelp(inv.cmd))
				return exitOK, false
			}
			return usage(inv.env, "INVALID_FLAG", err.Error(), "Run `snaphop-maps help "+inv.cmd.Name+"` for its flags."), false
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		inv.positional = append(inv.positional, args[0])
		args = args[1:]
	}
	inv.set = map[string]bool{}
	fs.Visit(func(f *flag.Flag) { inv.set[f.Name] = true })
	inv.pretty = *inv.switches["pretty"]
	if len(inv.positional) > 1 || (len(inv.positional) == 1 && inv.cmd.Positional == "") {
		return inv.usage("UNEXPECTED_ARGUMENT", fmt.Sprintf("%s takes no argument %q.", inv.cmd.Name, inv.positional[len(inv.positional)-1]),
			"Run `snaphop-maps help "+inv.cmd.Name+"`."), false
	}
	service, err := serviceURL(inv.first(*inv.strings["url"], inv.env.Getenv("SNAPHOP_MAPS_URL"), DefaultURL))
	if err != nil {
		return inv.usage("INVALID_URL", err.Error(), "Give --url as https://host, such as "+DefaultURL+"."), false
	}
	inv.service = service
	inv.timeout = DefaultTimeout
	if raw := *inv.strings["timeout"]; raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil || timeout <= 0 {
			return inv.usage("INVALID_FLAG", fmt.Sprintf("--timeout %q is not a positive duration.", raw), "Give it as 60s or 2m."), false
		}
		inv.timeout = timeout
	}
	return exitOK, true
}

func (inv *invocation) first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// serviceURL checks a service address and gives it one spelling: scheme and host in lower case and
// no trailing slash, so that a kept key is found again and sent to no other service.
func serviceURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("--url %q is not a URL", raw)
	}
	scheme := strings.ToLower(u.Scheme)
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (scheme != "https" && scheme != "http") {
		return "", fmt.Errorf("--url %q is not an http(s) address of a service, without credentials, query or fragment", raw)
	}
	if scheme == "http" && !loopback(u.Hostname()) {
		return "", fmt.Errorf("--url %q is plain http to another machine, which would send the API key unencrypted", raw)
	}
	return scheme + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.EscapedPath(), "/"), nil
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// arguments builds the tool's arguments: --args first, then every flag given, then the positional.
func (inv *invocation) arguments() (map[string]any, int, bool) {
	arguments := map[string]any{}
	if inv.set["args"] {
		value, code, ok := inv.readJSON("args", *inv.strings["args"])
		if !ok {
			return nil, code, false
		}
		object, isObject := value.(map[string]any)
		if !isObject {
			code := inv.usage("INVALID_JSON", "--args is not a JSON object.", `Give the tool's arguments as {"name": value, ...}.`)
			return nil, code, false
		}
		arguments = object
	}
	for _, spec := range inv.cmd.Flags {
		if spec.Argument == "" || !inv.set[spec.Name] {
			continue
		}
		switch spec.Kind {
		case kindBool:
			arguments[spec.Argument] = *inv.switches[spec.Name]
		case kindJSON:
			value, code, ok := inv.readJSON(spec.Name, *inv.strings[spec.Name])
			if !ok {
				return nil, code, false
			}
			arguments[spec.Argument] = value
		case kindInteger:
			raw := *inv.strings[spec.Name]
			number, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, inv.usage("INVALID_FLAG", fmt.Sprintf("--%s %q is not a whole number.", spec.Name, raw), "Give it as digits, such as 2."), false
			}
			arguments[spec.Argument] = number
		default:
			arguments[spec.Argument] = *inv.strings[spec.Name]
		}
	}
	for _, spec := range inv.cmd.Flags {
		if len(inv.positional) == 1 && spec.Argument == inv.cmd.Positional {
			arguments[spec.Argument] = inv.positional[0]
		}
	}
	for _, spec := range inv.cmd.Flags {
		if _, given := arguments[spec.Argument]; spec.Required && !given {
			return nil, inv.usage("MISSING_ARGUMENT", fmt.Sprintf("%s needs --%s.", inv.cmd.Name, spec.Name), "Run `snaphop-maps help "+inv.cmd.Name+"`."), false
		}
	}
	return arguments, exitOK, true
}

// readJSON reads one JSON value given inline, as @file, or as - for standard input.
func (inv *invocation) readJSON(flagName, raw string) (any, int, bool) {
	data := []byte(raw)
	source := "--" + flagName
	switch {
	case raw == "-":
		if inv.stdinUsed {
			return nil, inv.usage("INVALID_FLAG", "Only one flag can read standard input.", "Give the others inline or as @file."), false
		}
		inv.stdinUsed = true
		read, err := io.ReadAll(io.LimitReader(inv.env.Stdin, maxInputBytes))
		if err != nil {
			return nil, inv.fail("INPUT_UNREADABLE", "Cannot read standard input: "+err.Error(), "Give the JSON inline or as @file."), false
		}
		data, source = read, source+" (standard input)"
	case strings.HasPrefix(raw, "@"):
		read, err := os.ReadFile(raw[1:])
		if err != nil {
			return nil, inv.usage("INPUT_UNREADABLE", fmt.Sprintf("Cannot read %s for --%s: %v", raw[1:], flagName, err), "Give an existing file after @."), false
		}
		data, source = read, source+" ("+raw[1:]+")"
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	err := decoder.Decode(&value)
	if err == nil && decoder.More() {
		err = errors.New("more than one JSON value")
	}
	if err != nil {
		return nil, inv.usage("INVALID_JSON", fmt.Sprintf("%s is not one JSON value: %v", source, err), "Quote JSON for the shell, or pass it as @file."), false
	}
	return value, exitOK, true
}

// store is the credentials file this run uses.
func (inv *invocation) store() (credentials.Store, error) {
	if path := inv.first(*inv.strings["credentials"], inv.env.Getenv("SNAPHOP_MAPS_CREDENTIALS")); path != "" {
		return credentials.Store{Path: path}, nil
	}
	path, err := credentials.DefaultPath(inv.env.ConfigDir)
	return credentials.Store{Path: path}, err
}

// key is the API key to send, where it came from, and the account kept for this service.
func (inv *invocation) key() (string, string, *credentials.Account, error) {
	if value := *inv.strings["api-key"]; value != "" {
		return value, "flag", nil, nil
	}
	if value := inv.env.Getenv("SNAPHOP_MAPS_API_KEY"); value != "" {
		return value, "environment", nil, nil
	}
	store, err := inv.store()
	if err != nil {
		return "", "", nil, err
	}
	account, ok, err := store.Get(inv.service)
	if err != nil || !ok {
		return "", "none", nil, err
	}
	return account.APIKey, "file", &account, nil
}

func (inv *invocation) client() *mcp.Client {
	return &mcp.Client{
		Endpoint:  inv.service + "/mcp",
		HTTP:      inv.env.HTTP,
		UserAgent: "snaphop-maps-cli/" + inv.env.Version + " (+https://github.com/snaphop/snaphop-maps-cli)",
	}
}

func (inv *invocation) deadline() (context.Context, context.CancelFunc) {
	return context.WithTimeout(inv.ctx, inv.timeout)
}

func runTool(inv *invocation) int {
	arguments, code, ok := inv.arguments()
	if !ok {
		return code
	}
	return inv.callTool(inv.cmd.Tool, arguments)
}

func runCall(inv *invocation) int {
	if len(inv.positional) == 0 {
		return inv.usage("MISSING_ARGUMENT", "call needs the tool's name.", "Run `snaphop-maps tools` for their names, such as get_map.")
	}
	arguments, code, ok := inv.arguments()
	if !ok {
		return code
	}
	return inv.callTool(strings.ReplaceAll(inv.positional[0], "-", "_"), arguments)
}

// callTool calls one tool and reports its answer, keeping any key it issues.
func (inv *invocation) callTool(tool string, arguments map[string]any) int {
	if tool == "withdraw_map" && !inv.on("yes") {
		return inv.usage("CONFIRMATION_REQUIRED", "Withdrawing a map takes it down for good and cannot be undone.", "Add --yes to withdraw it.")
	}
	saves := (tool == "register_agent" || tool == "replace_key") && !inv.on("no-save")
	var store credentials.Store
	var kept *credentials.Account
	if saves {
		var err error
		if store, err = inv.store(); err == nil {
			var account credentials.Account
			var found bool
			if account, found, err = store.Get(inv.service); found {
				kept = &account
			}
		}
		if err != nil {
			return inv.fail("CREDENTIALS_UNREADABLE", err.Error()+". Nothing was sent.",
				"Fix or move the file, give another with --credentials, or add --no-save to only print the key.")
		}
		if tool == "register_agent" && kept != nil && !inv.on("overwrite") {
			return inv.usage("ACCOUNT_ALREADY_KEPT",
				"An account for "+inv.service+" is already kept in "+store.Path+", and registering would replace its key.",
				"Use that account: it needs no registration. To open another anyway, add --no-save, or --overwrite to replace the kept one.")
		}
	}
	// A key among the arguments is sent as the service takes it there, and no header goes with it.
	apiKey, used := "", ""
	if given, inArguments := arguments["apiKey"]; inArguments {
		used, _ = given.(string)
	} else if tool != "register_agent" {
		var err error
		var source string
		var account *credentials.Account
		if apiKey, source, account, err = inv.key(); err != nil {
			return inv.fail("CREDENTIALS_UNREADABLE", err.Error()+". Nothing was sent.", "Fix or move the file, or give the key with $SNAPHOP_MAPS_API_KEY.")
		}
		if source == "none" {
			return inv.usage("API_KEY_REQUIRED", "No API key is kept for "+inv.service+" and none was given.",
				"Run `snaphop-maps register-agent --name NAME` once, which keeps one, or set $SNAPHOP_MAPS_API_KEY.")
		}
		inv.warnExpiry(account)
		used = apiKey
	}
	ctx, cancel := inv.deadline()
	defer cancel()
	result, failure := inv.client().CallTool(ctx, tool, arguments, apiKey)
	if failure != nil {
		return inv.transport(failure, tool)
	}
	if result.IsError {
		return inv.refused(result.Structured)
	}
	inv.write(inv.env.Stdout, result.Structured)
	if saves {
		if code := inv.keep(tool, result.Structured, store, kept, used); code != exitOK {
			return code
		}
	}
	var answer struct {
		ID               string          `json:"id"`
		PublicationError json.RawMessage `json:"publicationError"`
	}
	if json.Unmarshal(result.Structured, &answer) == nil && len(answer.PublicationError) > 0 && string(answer.PublicationError) != "null" {
		inv.report(Problem{
			Code:    "PUBLICATION_REFUSED",
			Message: "The map is saved as a draft, but its publication was refused; publicationError says why.",
			Hint:    "Once the cause is fixed, run `snaphop-maps publish-map " + answer.ID + "`.",
			Detail:  answer.PublicationError,
		})
		return exitUnpublished
	}
	return exitOK
}

// keep puts the key an answer carries in the credentials file. A new registration replaces nothing
// it was not told to; a replacement only replaces the key it was made with, since another kept
// account's key would otherwise be lost for good.
func (inv *invocation) keep(tool string, structured json.RawMessage, store credentials.Store, kept *credentials.Account, used string) int {
	var issued credentials.Account
	_ = json.Unmarshal(structured, &issued)
	if issued.APIKey == "" {
		return inv.notSaved("The answer carries no apiKey to keep.")
	}
	account := issued
	if tool == "replace_key" {
		if kept != nil && kept.APIKey != used {
			return inv.notSaved("Another account's key is kept for " + inv.service + " in " + store.Path + ", so the new key was not put in its place.")
		}
		if kept != nil {
			account = *kept
			account.APIKey, account.KeyID, account.ExpiresAt, account.Scopes = issued.APIKey, issued.KeyID, issued.ExpiresAt, issued.Scopes
		}
	}
	account.SavedAt = inv.env.Now().UTC().Format(time.RFC3339)
	if err := store.Put(inv.service, account); err != nil {
		return inv.notSaved(err.Error() + ".")
	}
	return exitOK
}

func (inv *invocation) notSaved(message string) int {
	inv.report(Problem{
		Code:    "CREDENTIALS_NOT_SAVED",
		Message: message,
		Hint:    "The apiKey on standard output is the only copy: keep it somewhere that outlasts this conversation, and pass it with $SNAPHOP_MAPS_API_KEY.",
	})
	return exitNotSaved
}

// warnExpiry says on standard error when a kept key expires within a week, or has expired.
func (inv *invocation) warnExpiry(account *credentials.Account) {
	if account == nil {
		return
	}
	expires, err := time.Parse(time.RFC3339, account.ExpiresAt)
	if err != nil {
		return
	}
	left := expires.Sub(inv.env.Now())
	switch {
	case left <= 0:
		inv.warn("API_KEY_EXPIRED", "The kept API key expired at "+account.ExpiresAt+".",
			"If the service refuses it, register again with `snaphop-maps register-agent --name NAME --overwrite`.")
	case left < expiryWarning:
		inv.warn("API_KEY_EXPIRING", "The kept API key expires at "+account.ExpiresAt+".",
			"Run `snaphop-maps replace-key` before then; the account has no other way back in.")
	}
}

func (inv *invocation) warn(code, message, hint string) {
	document, _ := json.Marshal(map[string]Problem{"warning": {Code: code, Message: message, Hint: hint}})
	inv.write(inv.env.Stderr, document)
}

// transport reports a failed exchange, saying what repeating the request would do.
func (inv *invocation) transport(failure *mcp.Error, tool string) int {
	// The service's security chain refuses a header's unknown, expired or revoked key before any tool
	// sees it: a refusal like the tools' own, and nothing was carried out.
	if failure.Status == http.StatusUnauthorized {
		inv.report(Problem{
			Code:    "API_KEY_INVALID",
			Message: "The service refused the API key: it is unknown, expired or revoked (" + failure.Message + ").",
			Hint:    refusalHints["API_KEY_INVALID"],
			Status:  failure.Status,
		})
		return exitRefused
	}
	known := failure.OutcomeKnown()
	problem := Problem{Code: failure.Code, Message: failure.Message, Status: failure.Status, RetryAfter: failure.RetryAfter, OutcomeKnown: &known}
	switch {
	case !known:
		problem.Hint = repeatHints[tool]
		if problem.Hint == "" {
			problem.Hint = "It is safe to repeat."
		}
	case failure.Code == "EDGE_CHALLENGE":
		problem.Hint = "Repeating will not help. Report it to SnapHop at security@snaphop.com or through the repository's issues."
	case failure.Status == http.StatusTooManyRequests:
		problem.Hint = "Wait and try again later" + retryAfter(failure.RetryAfter) + "."
	default:
		problem.Hint = "Nothing was carried out. Check that --url names a SnapHop Maps service, such as " + DefaultURL + "."
	}
	inv.report(problem)
	return exitTransport
}

func retryAfter(value string) string {
	if value == "" {
		return ""
	}
	return ", after the " + value + " the service asked for"
}

// repeatHints say, for each tool, what to do when its answer never arrived: the service's own
// guide, "When a connection drops".
var repeatHints = map[string]string{
	"register_agent": "The account may have been opened without its key reaching you. Register again; the unreachable account is deleted after its inactivity period.",
	"create_map":     "The map may have been created. Run `snaphop-maps list-maps` and look for it by name before creating it again, or you may have two.",
	"update_map":     "The change may have been saved and published. Run `snaphop-maps get-map ID` and compare its draft with the change before repeating it.",
	"publish_map":    "It may have been published. Run `snaphop-maps get-map ID`: unpublishedChanges false means it was. Repeating publishes another release and counts against the publication limit.",
	"withdraw_map":   "It is safe to repeat: MAP_NOT_FOUND then means the first request withdrew it.",
	"rollback_map":   "It may have been made live. Run `snaphop-maps get-map ID`: if activeRelease is the release you asked for, it was.",
	"replace_key":    "Repeat it with the same key, which still works until a new key is used. Only the new key you go on to use is kept.",
}

// refusalHints say what to do next about a refusal, by the service's code.
var refusalHints = map[string]string{
	"API_KEY_REQUIRED":              "Run `snaphop-maps register-agent --name NAME` once, which keeps the key, or set $SNAPHOP_MAPS_API_KEY.",
	"API_KEY_INVALID":               "If the key was replaced, use the new one: `snaphop-maps credentials` shows which key is kept and where a key comes from. Otherwise it cannot be recovered; register again with `snaphop-maps register-agent --name NAME --overwrite`.",
	"AGENT_KEY_REQUIRED":            "Only an agent's own key can replace itself; this key belongs to a person or a service account.",
	"AGENT_NAME_INVALID":            "Give a name of 1 to 200 characters on one line.",
	"AGENT_REGISTRATION_CLOSED":     "This installation is not registering agents now. Try later, or use an existing account.",
	"AGENT_REGISTRATIONS_EXHAUSTED": "Today's registrations are used up. Try again tomorrow.",
	"TOO_MANY_REQUESTS":             "Wait and try again later.",
	"MAP_INVALID":                   "error.fields names every refused field by its path, such as markers[3].position. Positions are [longitude, latitude].",
	"MAP_NOT_FOUND":                 "Run `snaphop-maps list-maps` for the ids this key can reach.",
	"MAP_LIMIT_REACHED":             "The workspace holds as many maps as it may. Withdraw one it no longer needs with `snaphop-maps withdraw-map ID --yes`.",
	"DRAFT_CHANGED":                 "Read the map again with `snaphop-maps get-map ID` and redo the change.",
	"RELEASE_UNAVAILABLE":           "Run `snaphop-maps list-releases ID` for the releases that can be made live. A map never published has none.",
	"ACTIVE_RELEASE_CHANGED":        "Another release went live since you looked. Run `snaphop-maps list-releases ID` and decide again.",
	"INVALID_ARGUMENTS":             "Run `snaphop-maps tools` for the arguments each tool takes.",
	"UNKNOWN_TOOL":                  "Run `snaphop-maps tools` for the tools the service has.",
}

// refused reports a tool's refusal as the service gave it, with the next step when one is known.
func (inv *invocation) refused(structured json.RawMessage) int {
	var answer struct {
		Error Problem `json:"error"`
	}
	if json.Unmarshal(structured, &answer) != nil || answer.Error.Code == "" {
		answer.Error = Problem{Code: "REFUSED", Message: "The service refused the request.", Detail: structured}
	}
	answer.Error.Hint = refusalHints[answer.Error.Code]
	inv.report(answer.Error)
	return exitRefused
}

// rpcCommand is a command that sends one MCP method with no key and prints its result.
func rpcCommand(method string) func(*invocation) int {
	return func(inv *invocation) int {
		var params any = map[string]any{}
		if method == "initialize" {
			params = map[string]any{
				"protocolVersion": mcp.ProtocolVersion,
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": "snaphop-maps-cli", "version": inv.env.Version},
			}
		}
		ctx, cancel := inv.deadline()
		defer cancel()
		result, failure := inv.client().Call(ctx, method, params, "")
		if failure != nil {
			return inv.transport(failure, "")
		}
		inv.write(inv.env.Stdout, result)
		return exitOK
	}
}

func runCredentials(inv *invocation) int {
	store, err := inv.store()
	if err != nil {
		return inv.fail("CREDENTIALS_UNREADABLE", err.Error()+".", "Give the file with --credentials.")
	}
	_, source, account, err := inv.key()
	if err != nil {
		return inv.fail("CREDENTIALS_UNREADABLE", err.Error()+".", "Fix or move the file, or give another with --credentials.")
	}
	answer := map[string]any{"url": inv.service, "path": store.Path, "keySource": source, "account": nil}
	if account == nil {
		// A key from a flag or the environment: the file may still keep an account for this service.
		kept, found, _ := store.Get(inv.service)
		if found {
			account = &kept
		}
	}
	if account != nil {
		account.APIKey = ""
		answer["account"] = account
		inv.warnExpiry(account)
	}
	document, _ := json.Marshal(answer)
	inv.write(inv.env.Stdout, document)
	return exitOK
}

func (inv *invocation) on(name string) bool {
	value, ok := inv.switches[name]
	return ok && *value
}

// write writes one JSON document on one line, or indented with --pretty.
func (inv *invocation) write(w io.Writer, document []byte) {
	var out bytes.Buffer
	if inv.pretty {
		_ = json.Indent(&out, document, "", "  ")
	} else {
		_ = json.Compact(&out, document)
	}
	out.WriteByte('\n')
	_, _ = w.Write(out.Bytes())
}

func (inv *invocation) report(problem Problem) {
	document, _ := json.Marshal(map[string]Problem{"error": problem})
	inv.write(inv.env.Stderr, document)
}

func (inv *invocation) usage(code, message, hint string) int {
	inv.report(Problem{Code: code, Message: message, Hint: hint})
	return exitUsage
}

func (inv *invocation) fail(code, message, hint string) int {
	inv.report(Problem{Code: code, Message: message, Hint: hint})
	return exitFailure
}

// usage reports a command line that names no command, before there is an invocation.
func usage(env Env, code, message, hint string) int {
	return (&invocation{env: env}).usage(code, message, hint)
}
