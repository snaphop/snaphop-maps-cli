// Package cli is the snaphop-maps command line: SnapHop Maps' MCP tools as commands for AI
// agents. Every command writes one JSON document to standard output on success, and one JSON
// error to standard error otherwise, with an exit status that says which kind of failure it was.
package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

// errInputTooLarge is an input over maxInputBytes.
var errInputTooLarge = errors.New("larger than 8 MiB")

// minRedacted is the shortest key taken out of standard error. No real key is shorter, and a shorter
// value would also match ordinary text.
const minRedacted = 8

// keyShape is what a SnapHop key looks like, sh_agent_ and 43 more characters, or enough of one to
// matter: a key this run was never told of, such as one typed where a command's name goes, is still
// taken out of standard error.
var keyShape = regexp.MustCompile(`sh_[a-z]+_[A-Za-z0-9_-]{8,}`)

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
	// choice is the key this run sends, where it came from, and the account kept for the service.
	choice keyChoice
	// sent is the key this run sends, taken out of anything it writes to standard error; header says
	// whether it went as the Authorization header.
	sent   string
	header bool
	// found is the credentials file and the account kept in it for the service, once read.
	found *found
}

// found is the credentials file a run uses, the account kept in it for the service, and why either
// could not be had.
type found struct {
	store credentials.Store
	kept  *credentials.Account
	err   error
}

// Run runs one command line and returns its exit status.
func Run(ctx context.Context, env Env) int {
	if env.HTTP == nil {
		env.HTTP = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	name, rest, stray := splitCommand(env.Args)
	if name == "" && stray != "" {
		return usage(env, "INVALID_FLAG", stray+" is not a flag of this program.",
			"Run `snaphop-maps help` for its commands and flags; give a command's own flags after its name.")
	}
	if name == "" {
		return (&invocation{env: env}).text(overview())
	}
	if stray != "" {
		// The value after an unknown flag was taken as the command's name, and may be a key.
		return usage(env, "INVALID_FLAG", stray+" is not a flag that can come before the command.",
			"Only --url, --api-key, --credentials, --timeout and --pretty come before it; give the command's own flags after its name.")
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

// splitCommand finds the command's name, which global flags may come before, and returns the rest of
// the line without it, and the first flag before it that is not a global one.
func splitCommand(args []string) (string, []string, string) {
	stray := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			return arg, append(append([]string{}, args[:i]...), args[i+1:]...), stray
		}
		given, _, hasValue := strings.Cut(arg, "=")
		name := strings.TrimLeft(given, "-")
		known := name == "h" || name == "help"
		for _, spec := range globalFlags {
			if spec.Name == name {
				known = true
				if spec.Kind != kindLocal && !hasValue {
					i++ // its value
				}
			}
		}
		if !known && stray == "" {
			stray = given
		}
	}
	return "", nil, stray
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
	for _, spec := range inv.flags() {
		switch spec.Kind {
		case kindLocal, kindBool:
			inv.switches[spec.Name] = fs.Bool(spec.Name, false, spec.Description)
		default:
			inv.strings[spec.Name] = fs.String(spec.Name, "", spec.Description)
		}
	}
	for {
		before := args
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return inv.text(commandHelp(inv.cmd)), false
			}
			return inv.usage("INVALID_FLAG", err.Error(), "Run `snaphop-maps help "+inv.cmd.Name+"` for its flags."), false
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		// A switch takes its value after =: in `--publish false`, false is a positional argument.
		if value := strings.ToLower(args[0]); value == "true" || value == "false" {
			if name := inv.switchBefore(before, args); name != "" {
				return inv.usage("INVALID_FLAG", fmt.Sprintf("--%s takes its value after =, as --%s=%s.", name, name, value),
					"Write --"+name+"="+value+"; `--"+name+" "+value+"` would take "+value+" as the command's argument."), false
			}
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
	if inv.cmd.offline {
		return exitOK, true
	}
	source, raw := "--url", *inv.strings["url"]
	if raw == "" {
		source, raw = "$SNAPHOP_MAPS_URL", inv.env.Getenv("SNAPHOP_MAPS_URL")
	}
	service, err := serviceURL(inv.first(raw, DefaultURL), source)
	if err != nil {
		return inv.usage("INVALID_URL", err.Error(), "Give "+source+" as https://host, such as "+DefaultURL+"."), false
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

func (inv *invocation) flags() []flagSpec {
	return append(append([]flagSpec{}, globalFlags...), inv.cmd.Flags...)
}

// switchBefore names the switch that was the last flag parsed before rest, when it was given without
// a value. The words parsed are flags, and the values of those that take one: a value that happens to
// be a switch's name is not that switch.
func (inv *invocation) switchBefore(before, rest []string) string {
	parsed := before[:len(before)-len(rest)]
	last := ""
	for i := 0; i < len(parsed); i++ {
		name, _, hasValue := strings.Cut(strings.TrimLeft(parsed[i], "-"), "=")
		last = ""
		for _, spec := range inv.flags() {
			switch {
			case spec.Name != name || hasValue:
			case spec.Kind == kindBool || spec.Kind == kindLocal:
				last = name
			default:
				i++ // its value
			}
		}
	}
	return last
}

func (inv *invocation) first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// path is a path given on the command line or in the environment, read against the working directory.
func (inv *invocation) path(name string) string {
	if name == "" || filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(inv.env.Dir, name)
}

// serviceURL checks a service address and gives it one spelling: scheme and host in lower case,
// without the scheme's own port or a trailing slash, so that a kept key is found again and sent to no
// other service. source names where the address was given.
func serviceURL(raw, source string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s %q is not a URL", source, raw)
	}
	scheme := strings.ToLower(u.Scheme)
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (scheme != "https" && scheme != "http") {
		return "", fmt.Errorf("%s %q is not an http(s) address of a service, without credentials, query or fragment", source, raw)
	}
	if scheme == "http" && !loopback(u.Hostname()) {
		return "", fmt.Errorf("%s %q is plain http to another machine, which would send the API key unencrypted", source, raw)
	}
	host := u.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port := u.Port(); port != "" && !(scheme == "https" && port == "443") && !(scheme == "http" && port == "80") {
		host += ":" + port
	}
	return scheme + "://" + strings.ToLower(host) + strings.TrimRight(u.EscapedPath(), "/"), nil
}

// kept is the credentials file this run uses and the account kept in it for the service, read once
// for the whole run.
func (inv *invocation) kept() found {
	if inv.found == nil {
		store, err := inv.store()
		inv.found = &found{store: store, err: err}
		if err == nil {
			account, ok, err := store.Get(inv.service)
			if ok {
				inv.found.kept = &account
			}
			inv.found.err = err
		}
	}
	return *inv.found
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
		if len(inv.positional) != 1 || spec.Argument != inv.cmd.Positional {
			continue
		}
		if value := *inv.strings[spec.Name]; inv.set[spec.Name] && value != inv.positional[0] {
			return nil, inv.usage("CONFLICTING_ARGUMENT", fmt.Sprintf("%s is given twice, as %q and as --%s %q.", spec.Argument, inv.positional[0], spec.Name, value),
				"Give it once, either first or with --"+spec.Name+"."), false
		}
		arguments[spec.Argument] = inv.positional[0]
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
	var err error
	switch {
	case raw == "-":
		if inv.stdinUsed {
			return nil, inv.usage("INVALID_FLAG", "Only one flag can read standard input.", "Give the others inline or as @file."), false
		}
		inv.stdinUsed = true
		data, err = readLimited(inv.env.Stdin)
		source += " (standard input)"
		if err != nil && !errors.Is(err, errInputTooLarge) {
			return nil, inv.fail("INPUT_UNREADABLE", "Cannot read standard input: "+err.Error(), "Give the JSON inline or as @file."), false
		}
	case strings.HasPrefix(raw, "@"):
		data, err = inv.readFile(raw[1:])
		source += " (" + raw[1:] + ")"
		if err != nil && !errors.Is(err, errInputTooLarge) {
			return nil, inv.usage("INPUT_UNREADABLE", fmt.Sprintf("Cannot read %s for --%s: %v", raw[1:], flagName, err), "Give an existing file after @."), false
		}
	}
	if err != nil {
		return nil, inv.usage("INPUT_TOO_LARGE", source+" is larger than 8 MiB.", "The service takes far less; send fewer markers or shorter text."), false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	err = decoder.Decode(&value)
	if err == nil {
		// Anything after the value, even a stray ] or }, which the decoder would leave unread.
		if _, next := decoder.Token(); next != io.EOF {
			err = errors.New("more follows the first JSON value")
		}
	}
	if err != nil {
		return nil, inv.usage("INVALID_JSON", fmt.Sprintf("%s is not one JSON value: %v", source, err), "Quote JSON for the shell, or pass it as @file."), false
	}
	return value, exitOK, true
}

// readFile reads a file named after @, relative to the working directory, up to maxInputBytes.
func (inv *invocation) readFile(name string) ([]byte, error) {
	file, err := os.Open(inv.path(name))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readLimited(file)
}

// readLimited reads everything, and fails with errInputTooLarge rather than cut an input short.
func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxInputBytes+1))
	if err == nil && len(data) > maxInputBytes {
		err = errInputTooLarge
	}
	return data, err
}

// store is the credentials file this run uses.
func (inv *invocation) store() (credentials.Store, error) {
	if path := inv.first(*inv.strings["credentials"], inv.env.Getenv("SNAPHOP_MAPS_CREDENTIALS")); path != "" {
		return credentials.Store{Path: inv.path(path)}, nil
	}
	path, err := credentials.DefaultPath(inv.env.ConfigDir)
	return credentials.Store{Path: path}, err
}

// keyChoice is the key a command sends, where it comes from, and the account kept for the service.
type keyChoice struct {
	key    string
	source string // flag, environment, file, none, or arguments for a key given in --args
	kept   *credentials.Account
	path   string
	// replaced says the environment's key is one that the kept key replaced, and is not sent.
	replaced bool
}

// sendsKept says whether the key sent is the one kept for the service.
func (c keyChoice) sendsKept() bool { return c.kept != nil && c.kept.APIKey == c.key }

// key chooses the API key to send: --api-key, then $SNAPHOP_MAPS_API_KEY, then the key kept for the
// service. The environment's key is only for the service the environment names (ADR 0004): --url
// alone cannot send it elsewhere. Nor is it sent when the kept key replaced it (ADR 0005): it stops
// working once the new key is used, and sending it meanwhile would leave the new key unused until the
// old one expired.
func (inv *invocation) key() (keyChoice, error) {
	file := inv.kept()
	store, kept, err := file.store, file.kept, file.err
	environment := inv.env.Getenv("SNAPHOP_MAPS_API_KEY")
	forHere := environment != "" && inv.service == inv.environmentService()
	stale := forHere && kept != nil && kept.Replaced(digest(environment))
	switch {
	case *inv.strings["api-key"] != "":
		return keyChoice{key: *inv.strings["api-key"], source: "flag", kept: kept, path: store.Path}, nil
	case forHere && !stale:
		// A key from the environment is sent even when the file cannot be read.
		return keyChoice{key: environment, source: "environment", kept: kept, path: store.Path}, nil
	case err != nil:
		return keyChoice{}, err
	case kept == nil:
		return keyChoice{source: "none", path: store.Path}, nil
	}
	return keyChoice{key: kept.APIKey, source: "file", kept: kept, path: store.Path, replaced: stale}, nil
}

// digest is a key's SHA-256 in hex, which the file keeps of a replaced key instead of the key.
func digest(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// environmentService is the service $SNAPHOP_MAPS_API_KEY is for: $SNAPHOP_MAPS_URL's, or else the
// default. None, when $SNAPHOP_MAPS_URL is no service address.
func (inv *invocation) environmentService() string {
	service, _ := serviceURL(inv.first(inv.env.Getenv("SNAPHOP_MAPS_URL"), DefaultURL), "$SNAPHOP_MAPS_URL")
	return service
}

func (inv *invocation) client() *mcp.Client {
	return &mcp.Client{
		Endpoint:  inv.service + "/mcp",
		HTTP:      inv.env.HTTP,
		UserAgent: "snaphop-maps-cli/" + inv.env.Version + " (+https://github.com/snaphop/snaphop-maps-cli)",
		Redact:    inv.redactText,
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

// callTool calls one tool and reports its answer, keeping any key it issues. A key is kept before it
// is printed, and a command that would issue one first proves it could keep it (ADR 0005): the key is
// shown only once, and printing can fail, or end the process, as keeping it cannot.
func (inv *invocation) callTool(tool string, arguments map[string]any) int {
	if confirm, destructive := confirmations[tool]; destructive && !inv.on("yes") {
		return inv.usage("CONFIRMATION_REQUIRED", confirm.message, confirm.hint)
	}
	issues := tool == "register_agent" || tool == "replace_key"
	saves := issues && !inv.on("no-save")
	var file found
	if saves {
		if file = inv.kept(); file.err != nil {
			return inv.fail("CREDENTIALS_UNREADABLE", file.err.Error()+". Nothing was sent.",
				"Fix or move the file, give another with --credentials, or add --no-save to only print the key.")
		}
		if tool == "register_agent" && file.kept != nil && !inv.on("overwrite") {
			return inv.usage("ACCOUNT_ALREADY_KEPT",
				"An account for "+inv.service+" is already kept in "+file.store.Path+", and registering would replace its key.",
				"Use that account: it needs no registration. To open another anyway, add --no-save, or --overwrite to replace the kept one.")
		}
		if err := file.store.Check(); err != nil {
			return inv.fail("CREDENTIALS_UNWRITABLE", err.Error()+". Nothing was sent: the new key could not have been kept.",
				"Make the file and its directory writable, give another file with --credentials, or add --no-save to only print the key.")
		}
	}
	// A key among the arguments is sent as the service takes it there, and no header goes with it.
	apiKey, used := "", ""
	if given, inArguments := arguments["apiKey"]; inArguments {
		if used, _ = given.(string); used == "" {
			return inv.usage("INVALID_FLAG", "--args gives apiKey, but not as a key.",
				"Give apiKey as the key itself, or leave it out to send the key from the environment or the credentials file.")
		}
		// The account kept for the service, if the file can be read, so that a refusal of this key does
		// not advise registering over it.
		file := inv.kept()
		inv.choice = keyChoice{key: used, source: "arguments", kept: file.kept, path: file.store.Path}
	} else if tool != "register_agent" {
		choice, err := inv.key()
		if err != nil {
			return inv.fail("CREDENTIALS_UNREADABLE", err.Error()+". Nothing was sent.", "Fix or move the file, or give the key with $SNAPHOP_MAPS_API_KEY.")
		}
		inv.choice = choice
		if choice.source == "none" {
			hint := "Run `snaphop-maps register-agent --name NAME` once, which keeps one, or set $SNAPHOP_MAPS_API_KEY."
			if inv.env.Getenv("SNAPHOP_MAPS_API_KEY") != "" {
				hint = "$SNAPHOP_MAPS_API_KEY is only sent to $SNAPHOP_MAPS_URL, or to " + DefaultURL +
					" when that is not set. To use it with " + inv.service + ", set $SNAPHOP_MAPS_URL to that address instead of giving --url."
			}
			return inv.usage("API_KEY_REQUIRED", "No API key is kept for "+inv.service+" and none was given for it.", hint)
		}
		inv.warnChoice()
		apiKey, used = choice.key, choice.key
	}
	inv.sent, inv.header = used, apiKey != ""
	ctx, cancel := inv.deadline()
	defer cancel()
	result, failure := inv.client().CallTool(ctx, tool, arguments, apiKey)
	if failure != nil {
		return inv.transport(failure, tool)
	}
	if result.IsError {
		return inv.refused(result.Structured)
	}
	// An answer that carries no key is not one this program can read: the account may have been
	// opened, or its key replaced, without the key reaching here.
	var issued credentials.Account
	_ = json.Unmarshal(result.Structured, &issued)
	keyless := saves && issued.APIKey == ""
	notKept := ""
	if saves && !keyless {
		notKept = inv.keep(tool, issued, file.store, file.kept, used)
	}
	if err := inv.writeAnswer(result.Structured); err != nil {
		switch {
		case saves && !keyless && notKept == "":
			return inv.unwritten(err, "The new key is kept in the credentials file for every later command; `snaphop-maps credentials` describes it.")
		case issues:
			return inv.unwritten(err, "The new key was neither printed nor kept, and cannot be recovered. "+repeatHint(tool))
		}
		return inv.unwritten(err, "The request was carried out, but its answer is lost. "+repeatHint(tool))
	}
	if keyless {
		return inv.transport(&mcp.Error{Code: "INVALID_RESPONSE", Message: "The service's answer carries no apiKey to keep.", Status: http.StatusOK}, tool)
	}
	if notKept != "" {
		return inv.notSaved(notKept)
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

// keep puts the key an answer issued in the credentials file, and returns why it could not, if it
// could not. It decides against the file as it is when the key is put there, under the file's lock,
// since another command may have kept a key since this one began. A new registration replaces only
// the account it was told to; a replacement only replaces the key it was made with, and remembers
// that key's digest along with those of the keys it replaced, so that none is sent again. Any other
// key would otherwise be lost for good.
func (inv *invocation) keep(tool string, issued credentials.Account, store credentials.Store, kept *credentials.Account, used string) string {
	issued.SavedAt = inv.env.Now().UTC().Format(time.RFC3339)
	err := store.Update(inv.service, func(current credentials.Account, found bool) (credentials.Account, error) {
		switch {
		case tool == "register_agent" && found && (kept == nil || current.APIKey != kept.APIKey):
			return current, errors.New("Another account was kept for " + inv.service + " in " + store.Path + " while this one registered, so its key was not replaced")
		case tool == "replace_key" && found && current.APIKey != used:
			return current, errors.New("Another account's key is kept for " + inv.service + " in " + store.Path + ", so the new key was not put in its place")
		case tool == "replace_key":
			// With nothing kept, the new key replaces only the key used.
			current.Replace(issued, digest(used))
			return current, nil
		}
		return issued, nil
	})
	if err != nil {
		return err.Error() + "."
	}
	return ""
}

func (inv *invocation) notSaved(message string) int {
	inv.report(Problem{
		Code:    "CREDENTIALS_NOT_SAVED",
		Message: message,
		Hint: "The apiKey on standard output is the account's only copy of its key. Give it to the user this once, to keep " +
			"somewhere safe such as a password manager, and send it with $SNAPHOP_MAPS_API_KEY until it is kept.",
	})
	return exitNotSaved
}

// unwritten reports an answer that could not be written to standard output.
func (inv *invocation) unwritten(err error, hint string) int {
	inv.report(Problem{Code: "OUTPUT_FAILED", Message: "Cannot write the answer to standard output: " + err.Error() + ".", Hint: hint})
	return exitUnwritten
}

// warnChoice says on standard error what an agent should know about the key it sends: that the
// environment's key was replaced, or that the kept key it sends expires within a week or has expired.
func (inv *invocation) warnChoice() {
	if inv.choice.replaced {
		inv.warn("ENVIRONMENT_KEY_REPLACED", "$SNAPHOP_MAPS_API_KEY holds a key that replace-key replaced, so the key kept for "+
			inv.service+" in "+inv.choice.path+" was sent instead.", "Unset $SNAPHOP_MAPS_API_KEY: the kept key is sent without it.")
	}
	if !inv.choice.sendsKept() {
		return
	}
	account := inv.choice.kept
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
	inv.emit(document)
}

// secrets are the keys this run knows: the key it sends, and those given by flag or environment.
func (inv *invocation) secrets() []string {
	var keys []string
	for _, key := range []string{inv.sent, inv.lookupString("api-key"), inv.env.Getenv("SNAPHOP_MAPS_API_KEY")} {
		if len(key) >= minRedacted {
			keys = append(keys, key)
		}
	}
	return keys
}

func (inv *invocation) lookupString(name string) string {
	if value := inv.strings[name]; value != nil {
		return *value
	}
	return ""
}

// redact takes every key out of a document for standard error: the keys this run knows, and anything
// shaped like a key. A mistyped command line can echo one back, and so can an error body from
// something between here and the service.
func (inv *invocation) redact(document []byte) []byte {
	for _, key := range inv.secrets() {
		// The key as it appears inside a JSON string.
		quoted, _ := json.Marshal(key)
		document = bytes.ReplaceAll(document, quoted[1:len(quoted)-1], []byte("[REDACTED]"))
	}
	return keyShape.ReplaceAll(document, []byte("[REDACTED]"))
}

// redactText is redact for text not yet in a document, such as an answer's body before it is cut short.
func (inv *invocation) redactText(text string) string {
	for _, key := range inv.secrets() {
		text = strings.ReplaceAll(text, key, "[REDACTED]")
	}
	return keyShape.ReplaceAllString(text, "[REDACTED]")
}

// transport reports a failed exchange, saying what repeating the request would do.
func (inv *invocation) transport(failure *mcp.Error, tool string) int {
	// The service's security chain refuses a header's unknown, expired or revoked key before any tool
	// sees it: a refusal like the tools' own, and nothing was carried out. Without a key sent, a 401
	// comes from something else.
	if failure.Status == http.StatusUnauthorized && inv.header {
		inv.report(Problem{
			Code:    "API_KEY_INVALID",
			Message: "The service refused the API key: it is unknown, expired or revoked (" + failure.Message + ").",
			Hint:    inv.refusalHint("API_KEY_INVALID"),
			Status:  failure.Status,
		})
		return exitRefused
	}
	known := failure.OutcomeKnown()
	problem := Problem{Code: failure.Code, Message: failure.Message, Status: failure.Status, RetryAfter: failure.RetryAfter, OutcomeKnown: &known}
	switch {
	case !known:
		problem.Hint = repeatHint(tool)
		if failure.Code == "INVALID_RESPONSE" {
			problem.Hint += " The answer was not one this program can read: check that --url names a SnapHop Maps service, such as " + DefaultURL + "."
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

// repeatHint says what to do when a tool's answer never arrived or was lost. A tool this build does
// not know may change something, so it is not called safe to repeat.
func repeatHint(tool string) string {
	if hint := repeatHints[tool]; hint != "" {
		return hint
	}
	if cmd := lookup(tool); tool == "" || (cmd != nil && cmd.ReadOnly) {
		return "It is safe to repeat."
	}
	return "It may have been carried out. Unless the tool only reads, check what it does before repeating it."
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

	// The people tools: an agent invites the people it works for by a link it hands them.
	"invite_person":     "The invitation may have been made without its link reaching you. Invite the same address again: that replaces the invitation, and only the new link works.",
	"revoke_invitation": "It is safe to repeat: INVITATION_NOT_FOUND then means the first request revoked it.",
	"remove_member":     "It is safe to repeat: MEMBER_NOT_FOUND then means the first request removed them.",
}

// confirmations are the tools the service calls destructive, which run only with --yes: what each
// would do, and how to confirm it. TestEveryDestructiveCommandNeedsConfirming holds it to the command table.
var confirmations = map[string]struct{ message, hint string }{
	"withdraw_map":  {"Withdrawing a map takes it down for good and cannot be undone.", "Add --yes to withdraw it."},
	"remove_member": {"Removing a person ends their access to the workspace at once; only a new invitation they accept brings them back.", "Add --yes, once the user has agreed, to remove them."},
}

// refusalHints say what to do next about a refusal, by the service's code.
var refusalHints = map[string]string{
	"API_KEY_REQUIRED":              "Run `snaphop-maps register-agent --name NAME` once, which keeps the key, or set $SNAPHOP_MAPS_API_KEY.",
	"API_KEY_INVALID":               "If the key was replaced, use the new one: `snaphop-maps credentials` shows which key is kept and where a key comes from. Otherwise it cannot be recovered; register again with `snaphop-maps register-agent --name NAME --overwrite`.",
	"AGENT_KEY_REQUIRED":            "Only an agent's own key can replace itself or invite and remove people; this key belongs to a person or a service account.",
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
	"INVITATION_INVALID":            "Give --email as the person's address and --role as ADMIN, EDITOR or VIEWER.",
	"ALREADY_A_MEMBER":              "That person is in the workspace already and needs no invitation. Run `snaphop-maps list-members` for their role.",
	"PEOPLE_LIMIT_REACHED":          "The workspace holds as many people as it may, counting invitations not yet accepted. Revoke one with `snaphop-maps revoke-invitation ID`, or, once the user agrees, remove someone with `snaphop-maps remove-member USER_ID --yes`.",
	"INVITATION_NOT_FOUND":          "It may have been accepted, revoked or replaced. Run `snaphop-maps list-invitations` for those still open, and `snaphop-maps list-members` for who joined.",
	"MEMBER_NOT_FOUND":              "Run `snaphop-maps list-members` for the userIds in the workspace. The agent cannot remove itself.",
	"INVALID_ARGUMENTS":             "Run `snaphop-maps tools` for the arguments each tool takes.",
	"UNKNOWN_TOOL":                  "Run `snaphop-maps tools` for the tools the service has.",
}

// refusalHint is the next step after a refusal. When the refused key came from a flag or the
// environment while another is kept for the service, registering again would replace the kept key,
// so the hint points to that key instead.
func (inv *invocation) refusalHint(code string) string {
	if code == "API_KEY_INVALID" && inv.choice.kept != nil && !inv.choice.sendsKept() {
		given := map[string]string{"flag": "--api-key", "environment": "$SNAPHOP_MAPS_API_KEY", "arguments": "the apiKey in --args"}[inv.choice.source]
		return "The key from " + given + " was refused, but another is kept for " + inv.service + " in " + inv.choice.path +
			". Leave " + given + " out to send the kept one. Do not register again: that would replace it."
	}
	return refusalHints[code]
}

// refused reports a tool's refusal as the service gave it, every field included, with this program's
// next step for its code when it has one, and otherwise the service's own.
func (inv *invocation) refused(structured json.RawMessage) int {
	var answer struct {
		Error map[string]json.RawMessage `json:"error"`
	}
	var code string
	if json.Unmarshal(structured, &answer) != nil || json.Unmarshal(answer.Error["code"], &code) != nil || code == "" {
		inv.report(Problem{Code: "REFUSED", Message: "The service refused the request.", Detail: structured})
		return exitRefused
	}
	if hint := inv.refusalHint(code); hint != "" {
		answer.Error["hint"], _ = json.Marshal(hint)
	}
	document, _ := json.Marshal(answer)
	inv.emit(document)
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
		return inv.answer(result)
	}
}

func runCredentials(inv *invocation) int {
	if _, err := inv.store(); err != nil {
		return inv.fail("CREDENTIALS_UNREADABLE", err.Error()+".", "Give the file with --credentials.")
	}
	choice, err := inv.key()
	if err != nil {
		return inv.fail("CREDENTIALS_UNREADABLE", err.Error()+".", "Fix or move the file, or give another with --credentials.")
	}
	inv.choice = choice
	answer := map[string]any{"url": inv.service, "path": choice.path, "keySource": choice.source, "sendsKeptKey": choice.sendsKept(), "account": nil}
	if choice.kept != nil {
		account := *choice.kept
		account.APIKey = ""
		answer["account"] = account
	}
	inv.warnChoice()
	document, _ := json.Marshal(answer)
	return inv.answer(document)
}

func (inv *invocation) on(name string) bool {
	value, ok := inv.switches[name]
	return ok && *value
}

// writeAnswer writes one JSON document to standard output, on one line or indented with --pretty.
func (inv *invocation) writeAnswer(document []byte) error {
	var out bytes.Buffer
	if inv.pretty {
		_ = json.Indent(&out, document, "", "  ")
	} else {
		_ = json.Compact(&out, document)
	}
	out.WriteByte('\n')
	_, err := inv.env.Stdout.Write(out.Bytes())
	return err
}

// answer writes a command's answer, and reports it when standard output cannot take it.
func (inv *invocation) answer(document []byte) int {
	if err := inv.writeAnswer(document); err != nil {
		return inv.unwritten(err, "Run it again with standard output going somewhere that can take it.")
	}
	return exitOK
}

// text writes text, such as help, to standard output.
func (inv *invocation) text(text string) int {
	if _, err := io.WriteString(inv.env.Stdout, text); err != nil {
		return inv.unwritten(err, "Run it again with standard output going somewhere that can take it.")
	}
	return exitOK
}

// emit writes one JSON document to standard error, always on one line, with every key taken out.
func (inv *invocation) emit(document []byte) {
	var out bytes.Buffer
	_ = json.Compact(&out, inv.redact(document))
	out.WriteByte('\n')
	_, _ = inv.env.Stderr.Write(out.Bytes())
}

func (inv *invocation) report(problem Problem) {
	document, _ := json.Marshal(map[string]Problem{"error": problem})
	inv.emit(document)
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
