package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/snaphop/snaphop-maps-cli/internal/credentials"
)

// TestEveryToolIsACommand holds the command table to the MCP server's tools/list, kept in
// testdata/tools.json: every tool is a command, every argument but apiKey is a flag, and every
// flag that is sent is an argument the tool takes. Refresh the file with `snaphop-maps tools`
// when the service's tools change.
func TestEveryToolIsACommand(t *testing.T) {
	t.Parallel()
	var listed struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			} `json:"inputSchema"`
			Annotations struct {
				ReadOnly    bool `json:"readOnlyHint"`
				Destructive bool `json:"destructiveHint"`
			} `json:"annotations"`
		} `json:"tools"`
	}
	data, err := testdata.ReadFile("testdata/tools.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) == 0 {
		t.Fatal("testdata/tools.json lists no tools")
	}
	byTool := map[string]*command{}
	for _, cmd := range commands() {
		if cmd.Tool != "" {
			byTool[cmd.Tool] = cmd
		}
	}
	for _, listedTool := range listed.Tools {
		cmd := byTool[listedTool.Name]
		if cmd == nil {
			t.Errorf("tool %s has no command", listedTool.Name)
			continue
		}
		delete(byTool, listedTool.Name)
		if cmd.Name != strings.ReplaceAll(listedTool.Name, "_", "-") {
			t.Errorf("tool %s is command %s", listedTool.Name, cmd.Name)
		}
		_, takesKey := listedTool.InputSchema.Properties["apiKey"]
		if cmd.NeedsKey != takesKey {
			t.Errorf("%s needs a key: %v, the tool takes one: %v", cmd.Name, cmd.NeedsKey, takesKey)
		}
		if cmd.ReadOnly != listedTool.Annotations.ReadOnly || cmd.Destructive != listedTool.Annotations.Destructive {
			t.Errorf("%s is read-only %v and destructive %v; the tool says %+v", cmd.Name, cmd.ReadOnly, cmd.Destructive, listedTool.Annotations)
		}
		flags := map[string]flagSpec{}
		for _, spec := range cmd.Flags {
			if spec.Argument == "" {
				continue
			}
			flags[spec.Argument] = spec
			property, known := listedTool.InputSchema.Properties[spec.Argument]
			if !known {
				t.Errorf("%s --%s sends %s, which %s does not take", cmd.Name, spec.Name, spec.Argument, listedTool.Name)
				continue
			}
			// The flag sends the JSON type the argument is.
			var declared struct {
				Type any `json:"type"`
			}
			_ = json.Unmarshal(property, &declared)
			sends := map[kind][]string{kindString: {"string"}, kindInteger: {"integer"}, kindBool: {"boolean"}, kindJSON: {"array", "object"}}[spec.Kind]
			if !typeAmong(declared.Type, sends) {
				t.Errorf("%s --%s is a %s flag; %s's %s is %v", cmd.Name, spec.Name, spec.Kind, listedTool.Name, spec.Argument, declared.Type)
			}
		}
		for property := range listedTool.InputSchema.Properties {
			if _, ok := flags[property]; !ok && property != "apiKey" {
				t.Errorf("%s has no flag for %s's argument %s", cmd.Name, listedTool.Name, property)
			}
		}
		var required []string
		for _, spec := range flags {
			if spec.Required {
				required = append(required, spec.Argument)
			}
		}
		sort.Strings(required)
		want := append([]string{}, listedTool.InputSchema.Required...)
		sort.Strings(want)
		if !reflect.DeepEqual(required, want) && !(len(required) == 0 && len(want) == 0) {
			t.Errorf("%s requires %v; %s requires %v", cmd.Name, required, listedTool.Name, want)
		}
	}
	for name := range byTool {
		t.Errorf("command for %s, which the service does not list", name)
	}
}

// typeAmong says whether a JSON Schema type, one name or a list of them, includes one of names.
func typeAmong(declared any, names []string) bool {
	var types []any
	switch value := declared.(type) {
	case string:
		types = []any{value}
	case []any:
		types = value
	}
	for _, t := range types {
		for _, name := range names {
			if t == name {
				return true
			}
		}
	}
	return false
}

// TestCommandsCallTheirTools runs each tool's command and checks the one request it sends.
func TestCommandsCallTheirTools(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		args      []string
		tool      string
		arguments string
	}{
		{"create", []string{"create-map", "--name", "Coffee", "--style", "positron",
			"--markers", `[{"position": [-9.1427, 38.7107], "title": "A Brasileira"}]`,
			"--view", `{"center": [-9.1427, 38.7107], "zoom": 15}`,
			"--controls", `{"scale": false}`, "--publish=false"},
			"create_map", `{"controls":{"scale":false},"markers":[{"position":[-9.1427,38.7107],"title":"A Brasileira"}],"name":"Coffee","publish":false,"style":"positron","view":{"center":[-9.1427,38.7107],"zoom":15}}`},
		{"create keeps the service's default", []string{"create-map", "--name", "Coffee"}, "create_map", `{"name":"Coffee"}`},
		{"list", []string{"list-maps"}, "list_maps", `{}`},
		{"get by position", []string{"get-map", "m1"}, "get_map", `{"id":"m1"}`},
		{"get by flag", []string{"get-map", "--id", "m1"}, "get_map", `{"id":"m1"}`},
		{"flags after the id", []string{"get-map", "m1", "--pretty"}, "get_map", `{"id":"m1"}`},
		{"update", []string{"update-map", "m1", "--args", `{"name": "Old", "style": "dark"}`, "--name", "New", "--view", "null", "--publish"},
			"update_map", `{"id":"m1","name":"New","publish":true,"style":"dark","view":null}`},
		{"publish", []string{"publish-map", "m1"}, "publish_map", `{"id":"m1"}`},
		{"account", []string{"get-account"}, "get_account", `{}`},
		{"installation", []string{"get-installation"}, "get_installation", `{}`},
		{"releases", []string{"list-releases", "m1"}, "list_releases", `{"id":"m1"}`},
		{"rollback", []string{"rollback-map", "m1", "--release", "2"}, "rollback_map", `{"id":"m1","release":2}`},
		{"rollback expecting a release", []string{"rollback-map", "--id", "m1", "--release", "1", "--expected-active", "3"},
			"rollback_map", `{"expectedActive":3,"id":"m1","release":1}`},
		{"activity", []string{"list-activity"}, "list_activity", `{}`},
		{"withdraw", []string{"withdraw-map", "m1", "--yes"}, "withdraw_map", `{"id":"m1"}`},
		{"replace", []string{"replace-key", "--no-save"}, "replace_key", `{}`},
		{"call", []string{"call", "get-map", "--args", `{"id": "m1"}`}, "get_map", `{"id":"m1"}`},
		{"a tool's own name", []string{"list_maps"}, "list_maps", `{}`},
		{"global flags first", []string{"--pretty", "--timeout", "5s", "list-maps"}, "list_maps", `{}`},
		{"numbers kept exactly", []string{"call", "get_map", "--args", `{"id": "m1", "n": 12345678901234567890}`}, "get_map", `{"id":"m1","n":12345678901234567890}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, func(request) answer { return tool(map[string]any{"ok": true}) })
			h := newHarness(t, s)
			h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_env"
			o := h.run(tc.args...)
			if got := o.success(t); got["ok"] != true {
				t.Fatalf("stdout = %s", o.stdout)
			}
			if o.stderr != "" {
				t.Fatalf("stderr = %s", o.stderr)
			}
			r := s.only()
			if r.Method != "tools/call" || r.Tool != tc.tool || r.Raw != tc.arguments {
				t.Fatalf("sent %s %s %s, want %s %s", r.Method, r.Tool, r.Raw, tc.tool, tc.arguments)
			}
			if r.Auth != "Bearer shk_env" {
				t.Fatalf("Authorization = %q", r.Auth)
			}
		})
	}
}

func TestOutputIsOneLineOrIndented(t *testing.T) {
	t.Parallel()
	s := newService(t, func(request) answer { return tool(map[string]any{"maps": []any{}}) })
	h := newHarness(t, s)
	h.vars["SNAPHOP_MAPS_API_KEY"] = "k"
	if o := h.run("list-maps"); o.stdout != "{\"maps\":[]}\n" {
		t.Fatalf("stdout = %q", o.stdout)
	}
	if o := h.run("list-maps", "--pretty"); o.stdout != "{\n  \"maps\": []\n}\n" {
		t.Fatalf("stdout = %q", o.stdout)
	}
}

func TestJSONArgumentsFromFilesAndStandardInput(t *testing.T) {
	t.Parallel()
	s := newService(t, func(request) answer { return tool(map[string]any{"id": "m1"}) })
	h := newHarness(t, s)
	h.vars["SNAPHOP_MAPS_API_KEY"] = "k"
	h.stdin = strings.NewReader(`[{"position": [1, 2], "title": "From stdin"}]`)
	path := h.file("view.json", `{"center": [1, 2], "zoom": 3}`)
	h.run("create-map", "--name", "N", "--markers", "-", "--view", "@"+path).success(t)
	sent, _ := json.Marshal(s.only().Arguments)
	if string(sent) != `{"markers":[{"position":[1,2],"title":"From stdin"}],"name":"N","view":{"center":[1,2],"zoom":3}}` {
		t.Fatalf("sent %s", sent)
	}
}

func TestLocalRefusalsSendNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		args  []string
		exit  int
		code  string
		stdin io.Reader
	}{
		{"unknown command", []string{"frobnicate"}, exitUsage, "UNKNOWN_COMMAND", nil},
		{"unknown flag", []string{"list-maps", "--frobnicate"}, exitUsage, "INVALID_FLAG", nil},
		{"argument to a command without one", []string{"list-maps", "extra"}, exitUsage, "UNEXPECTED_ARGUMENT", nil},
		{"two ids", []string{"get-map", "m1", "m2"}, exitUsage, "UNEXPECTED_ARGUMENT", nil},
		{"missing id", []string{"get-map"}, exitUsage, "MISSING_ARGUMENT", nil},
		{"missing name", []string{"create-map"}, exitUsage, "MISSING_ARGUMENT", nil},
		{"missing tool", []string{"call"}, exitUsage, "MISSING_ARGUMENT", nil},
		{"args not an object", []string{"call", "list_maps", "--args", "[1]"}, exitUsage, "INVALID_JSON", nil},
		{"args not json", []string{"list-maps", "--args", "{"}, exitUsage, "INVALID_JSON", nil},
		{"two json values", []string{"create-map", "--name", "N", "--markers", "[] []"}, exitUsage, "INVALID_JSON", nil},
		{"markers not json", []string{"create-map", "--name", "N", "--markers", "[{"}, exitUsage, "INVALID_JSON", nil},
		{"missing file", []string{"create-map", "--name", "N", "--markers", "@/no/such/file.json"}, exitUsage, "INPUT_UNREADABLE", nil},
		{"file is a directory", []string{"create-map", "--name", "N", "--markers", "@."}, exitUsage, "INPUT_UNREADABLE", nil},
		{"stdin too large", []string{"create-map", "--name", "N", "--markers", "-"}, exitUsage, "INPUT_TOO_LARGE", strings.NewReader(strings.Repeat(" ", maxInputBytes+1))},
		{"stdin twice", []string{"create-map", "--name", "N", "--markers", "-", "--view", "-"}, exitUsage, "INVALID_FLAG", strings.NewReader("[]")},
		{"stdin unreadable", []string{"create-map", "--name", "N", "--markers", "-"}, exitFailure, "INPUT_UNREADABLE", failingReader{}},
		{"withdraw unconfirmed", []string{"withdraw-map", "m1"}, exitUsage, "CONFIRMATION_REQUIRED", nil},
		{"release not a number", []string{"rollback-map", "m1", "--release", "two"}, exitUsage, "INVALID_FLAG", nil},
		{"release missing", []string{"rollback-map", "m1"}, exitUsage, "MISSING_ARGUMENT", nil},
		{"withdraw unconfirmed by call", []string{"call", "withdraw_map", "--args", `{"id": "m1"}`}, exitUsage, "CONFIRMATION_REQUIRED", nil},
		{"not a url", []string{"list-maps", "--url", "http://[::1"}, exitUsage, "INVALID_URL", nil},
		{"no host", []string{"list-maps", "--url", "https://"}, exitUsage, "INVALID_URL", nil},
		{"not http", []string{"list-maps", "--url", "ftp://maps.snaphop.ai"}, exitUsage, "INVALID_URL", nil},
		{"credentials in url", []string{"list-maps", "--url", "https://user:pw@maps.snaphop.ai"}, exitUsage, "INVALID_URL", nil},
		{"query in url", []string{"list-maps", "--url", "https://maps.snaphop.ai/?x=1"}, exitUsage, "INVALID_URL", nil},
		{"fragment in url", []string{"list-maps", "--url", "https://maps.snaphop.ai/#x"}, exitUsage, "INVALID_URL", nil},
		{"plain http elsewhere", []string{"list-maps", "--url", "http://maps.snaphop.ai"}, exitUsage, "INVALID_URL", nil},
		{"bad timeout", []string{"list-maps", "--timeout", "soon"}, exitUsage, "INVALID_FLAG", nil},
		{"negative timeout", []string{"list-maps", "--timeout", "-1s"}, exitUsage, "INVALID_FLAG", nil},
		{"no key", []string{"list-maps"}, exitUsage, "API_KEY_REQUIRED", nil},
		{"help for nothing", []string{"help", "frobnicate"}, exitUsage, "UNKNOWN_COMMAND", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, never(t))
			h := newHarness(t, s)
			if tc.stdin != nil {
				h.stdin = tc.stdin
			}
			p := h.run(tc.args...).failure(t, tc.exit, tc.code)
			if p.Message == "" || p.Hint == "" {
				t.Fatalf("error without message or hint: %+v", p)
			}
		})
	}
}

func TestServiceAddresses(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"HTTPS://Maps.SnapHop.AI/":     "https://maps.snaphop.ai",
		"https://maps.snaphop.ai":      "https://maps.snaphop.ai",
		"https://example.com/maps/":    "https://example.com/maps",
		"http://localhost:8080":        "http://localhost:8080",
		"http://LOCALHOST:8080":        "http://localhost:8080",
		"http://127.0.0.1:8080/":       "http://127.0.0.1:8080",
		"http://[::1]:8080":            "http://[::1]:8080",
		"https://maps.snaphop.ai:443":  "https://maps.snaphop.ai",
		"https://Maps.SnapHop.ai:443/": "https://maps.snaphop.ai",
		"http://[::1]:80":              "http://[::1]",
		"https://example.com:8443":     "https://example.com:8443",
		"http://localhost:443":         "http://localhost:443",
	}
	for raw, want := range cases {
		if got, err := serviceURL(raw, "--url"); err != nil || got != want {
			t.Errorf("serviceURL(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
}

// TestTheServiceRefusal: a refusal reaches standard error as the service gave it, every field
// included, with this program's next step for its code, or else the service's own.
func TestTheServiceRefusal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		answer answer
		code   string
		hint   string
		stderr string
	}{
		{"known code", refusal(map[string]any{"error": map[string]any{"code": "MAP_INVALID", "message": "The map is invalid", "hint": "the service's",
			"fields": map[string]any{"markers[0].position": "is [longitude, latitude]"}}}), "MAP_INVALID", "[longitude, latitude]", `"markers[0].position"`},
		{"unknown code", refusal(map[string]any{"error": map[string]any{"code": "SOMETHING_NEW", "message": "New"}}), "SOMETHING_NEW", "", `"message":"New"`},
		{"unknown code with the service's hint", refusal(map[string]any{"error": map[string]any{"code": "SOMETHING_NEW", "message": "New", "hint": "Do Y", "limit": 5}}),
			"SOMETHING_NEW", "Do Y", `"limit":5`},
		{"a code that is not a string", refusal(map[string]any{"error": map[string]any{"code": 5}}), "REFUSED", "", `"detail":{"error":{"code":5}}`},
		{"no error object", refusal(map[string]any{"why": "unknown"}), "REFUSED", "", `"detail":{"why":"unknown"}`},
		{"words alone", result(map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": "Bad id"}}}), "REFUSED", "", `"detail":"Bad id"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, func(request) answer { return tc.answer })
			h := newHarness(t, s)
			h.vars["SNAPHOP_MAPS_API_KEY"] = "k"
			o := h.run("create-map", "--name", "N")
			p := o.failure(t, exitRefused, tc.code)
			if (p.Hint == "") != (tc.hint == "") || !strings.Contains(p.Hint, tc.hint) {
				t.Fatalf("hint = %q, want %q", p.Hint, tc.hint)
			}
			if o.stdout != "" || !strings.Contains(o.stderr, tc.stderr) {
				t.Fatalf("stdout = %q, stderr = %s, want %s", o.stdout, o.stderr, tc.stderr)
			}
		})
	}
}

func TestARefusedPublicationKeepsTheDraft(t *testing.T) {
	t.Parallel()
	s := newService(t, func(r request) answer {
		if r.Tool == "list_maps" {
			return tool(map[string]any{"maps": []any{}, "publicationError": nil})
		}
		return tool(map[string]any{"id": "m1", "published": nil, "publicationError": map[string]any{"code": "TOO_MANY_REQUESTS", "message": "Slow down"}})
	})
	h := newHarness(t, s)
	h.vars["SNAPHOP_MAPS_API_KEY"] = "k"
	o := h.run("update-map", "m1", "--name", "N")
	p := o.failure(t, exitUnpublished, "PUBLICATION_REFUSED")
	if decode(t, o.stdout)["id"] != "m1" || !strings.Contains(p.Hint, "publish-map m1") || !strings.Contains(string(p.Detail), "TOO_MANY_REQUESTS") {
		t.Fatalf("stdout %s, error %+v", o.stdout, p)
	}
	h.run("list-maps").success(t)
}

func TestFailedExchangesSayWhetherToRepeat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		answer answer
		args   []string
		code   string
		known  bool
		hint   string
	}{
		{"create may have happened", answer{status: 502, body: "Bad gateway"}, []string{"create-map", "--name", "N"}, "HTTP_502", false, "list-maps"},
		{"publish may have happened", answer{status: 504}, []string{"publish-map", "m1"}, "HTTP_504", false, "unpublishedChanges"},
		{"read is safe", answer{status: 503}, []string{"get-map", "m1"}, "HTTP_503", false, "safe to repeat"},
		{"edge challenge", answer{status: 403, header: http.Header{"Cf-Mitigated": {"challenge"}}}, []string{"list-maps"}, "EDGE_CHALLENGE", true, "will not help"},
		{"rate limited", answer{status: 429, header: http.Header{"Retry-After": {"30"}}}, []string{"list-maps"}, "HTTP_429", true, "after the 30"},
		{"rate limited without a time", answer{status: 429}, []string{"list-maps"}, "HTTP_429", true, "try again later."},
		{"not the service", answer{status: 404, body: "no"}, []string{"list-maps"}, "HTTP_404", true, "--url"},
		{"not json-rpc", answer{status: 200, body: "<html>"}, []string{"tools"}, "INVALID_RESPONSE", false, "--url"},
		// The tool ran, but its answer cannot be read: repeating it could create a second map.
		{"create answered unreadably", result(map[string]any{"content": []any{map[string]any{"type": "text", "text": "Created"}}}),
			[]string{"create-map", "--name", "N"}, "INVALID_RESPONSE", false, "list-maps"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, func(request) answer { return tc.answer })
			h := newHarness(t, s)
			h.vars["SNAPHOP_MAPS_API_KEY"] = "k"
			p := h.run(tc.args...).failure(t, exitTransport, tc.code)
			if p.OutcomeKnown == nil || *p.OutcomeKnown != tc.known || !strings.Contains(p.Hint, tc.hint) {
				t.Fatalf("error = %+v", p)
			}
		})
	}
}

func TestARefusedKeyIsARefusal(t *testing.T) {
	t.Parallel()
	s := newService(t, func(request) answer {
		return answer{status: http.StatusUnauthorized, body: `{"error":"Authentication is unavailable"}`}
	})
	h := newHarness(t, s)
	h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_revoked"
	p := h.run("list-maps").failure(t, exitRefused, "API_KEY_INVALID")
	if p.Status != 401 || !strings.Contains(p.Message, "Authentication is unavailable") || !strings.Contains(p.Hint, "credentials") {
		t.Fatalf("error = %+v", p)
	}
}

func TestATimeoutIsReported(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	s := newService(t, func(request) answer { <-release; return answer{status: 200} })
	defer close(release)
	h := newHarness(t, s)
	h.vars["SNAPHOP_MAPS_API_KEY"] = "k"
	p := h.run("replace-key", "--timeout", "50ms").failure(t, exitTransport, "TIMEOUT")
	if !strings.Contains(p.Hint, "same key") {
		t.Fatalf("hint = %q", p.Hint)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	t.Parallel()
	target := newService(t, never(t))
	s := newService(t, func(request) answer {
		return answer{status: http.StatusTemporaryRedirect, header: http.Header{"Location": {target.server.URL + "/mcp"}}}
	})
	h := newHarness(t, s)
	h.vars["SNAPHOP_MAPS_API_KEY"] = "k"
	h.run("list-maps").failure(t, exitTransport, "HTTP_307")
}

func TestRPCCommands(t *testing.T) {
	t.Parallel()
	s := newService(t, func(r request) answer {
		switch r.Method {
		case "initialize":
			return result(map[string]any{"protocolVersion": r.Params["protocolVersion"], "instructions": "SnapHop Maps publishes", "client": r.Params["clientInfo"]})
		case "tools/list":
			return result(map[string]any{"tools": []any{map[string]any{"name": "list_maps"}}})
		default:
			return result(map[string]any{})
		}
	})
	h := newHarness(t, s)
	h.vars["SNAPHOP_MAPS_API_KEY"] = "never sent"
	guide := h.run("guide").success(t)
	if guide["protocolVersion"] != "2025-06-18" || guide["instructions"] != "SnapHop Maps publishes" ||
		guide["client"].(map[string]any)["version"] != "1.2.3" {
		t.Fatalf("guide = %v", guide)
	}
	if tools := h.run("tools").success(t); len(tools["tools"].([]any)) != 1 {
		t.Fatalf("tools = %v", tools)
	}
	if ping := h.run("ping"); ping.stdout != "{}\n" {
		t.Fatalf("ping = %+v", ping)
	}
	for _, r := range s.received() {
		if r.Auth != "" {
			t.Fatalf("%s sent a key", r.Method)
		}
	}
}

func TestKeySources(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		flag  string
		env   string
		kept  string
		send  string
		args  []string
		inArg bool
	}{
		{name: "flag first", flag: "shk_flag", env: "shk_env", kept: "shk_file", send: "Bearer shk_flag"},
		{name: "environment next", env: "shk_env", kept: "shk_file", send: "Bearer shk_env"},
		{name: "file last", kept: "shk_file", send: "Bearer shk_file"},
		{name: "argument alone", kept: "shk_file", send: "", args: []string{"--args", `{"apiKey": "shk_arg"}`}, inArg: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, func(request) answer { return tool(map[string]any{"maps": []any{}}) })
			h := newHarness(t, s)
			h.vars["SNAPHOP_MAPS_API_KEY"] = tc.env
			h.keep(credentials.Account{APIKey: tc.kept})
			args := append([]string{"list-maps"}, tc.args...)
			if tc.flag != "" {
				args = append(args, "--api-key", tc.flag)
			}
			h.run(args...).success(t)
			r := s.only()
			if r.Auth != tc.send || (r.Arguments["apiKey"] == "shk_arg") != tc.inArg {
				t.Fatalf("Authorization %q, arguments %v", r.Auth, r.Arguments)
			}
		})
	}
}

func TestAKeptKeyIsOnlySentToItsService(t *testing.T) {
	t.Parallel()
	s := newService(t, never(t))
	h := newHarness(t, s)
	h.keep(credentials.Account{APIKey: "shk_production"})
	h.run("list-maps", "--url", "http://localhost:1").failure(t, exitUsage, "API_KEY_REQUIRED")
}

func TestExpiryWarnings(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"2026-09-29T00:00:00Z": "API_KEY_EXPIRED",
		"2026-10-02T00:00:00Z": "API_KEY_EXPIRING",
		"2026-10-30T00:00:00Z": "",
		"not a time":           "",
	}
	for expires, warning := range cases {
		t.Run(expires, func(t *testing.T) {
			t.Parallel()
			s := newService(t, func(request) answer { return tool(map[string]any{"maps": []any{}}) })
			h := newHarness(t, s)
			h.keep(credentials.Account{APIKey: "k", ExpiresAt: expires})
			o := h.run("list-maps")
			o.success(t)
			if warning == "" {
				if o.stderr != "" {
					t.Fatalf("stderr = %s", o.stderr)
				}
				return
			}
			if p := problem(t, strings.TrimSpace(o.stderr), "warning"); p.Code != warning || p.Hint == "" {
				t.Fatalf("warning = %+v", p)
			}
		})
	}
}

func TestUnreadableCredentials(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"list-maps"}, {"register-agent", "--name", "A"}, {"replace-key"}, {"credentials"}} {
		t.Run(args[0], func(t *testing.T) {
			t.Parallel()
			s := newService(t, never(t))
			h := newHarness(t, s)
			h.vars["SNAPHOP_MAPS_CREDENTIALS"] = h.dir // a directory, not a file
			h.run(args...).failure(t, exitFailure, "CREDENTIALS_UNREADABLE")
		})
	}
}

func TestNoConfigurationDirectory(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"list-maps"}, {"register-agent", "--name", "A"}, {"credentials"}} {
		t.Run(args[0], func(t *testing.T) {
			t.Parallel()
			s := newService(t, never(t))
			h := newHarness(t, s)
			delete(h.vars, "SNAPHOP_MAPS_CREDENTIALS")
			h.configDir = func() (string, error) { return "", errNoHome }
			h.run(args...).failure(t, exitFailure, "CREDENTIALS_UNREADABLE")
		})
	}
}

func TestTheDefaultConfigurationDirectory(t *testing.T) {
	t.Parallel()
	s := newService(t, func(request) answer { return tool(map[string]any{"maps": []any{}}) })
	h := newHarness(t, s)
	delete(h.vars, "SNAPHOP_MAPS_CREDENTIALS")
	config, _ := h.configDir()
	if err := (credentials.Store{Path: config + "/snaphop-maps/credentials.json"}).Put(h.service(), credentials.Account{APIKey: "shk_default"}); err != nil {
		t.Fatal(err)
	}
	h.run("list-maps").success(t)
	if r := s.only(); r.Auth != "Bearer shk_default" {
		t.Fatalf("Authorization = %q", r.Auth)
	}
}

func TestAFileTooLargeIsRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t, newService(t, never(t)))
	h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_env"
	h.file("markers.json", "[]"+strings.Repeat(" ", maxInputBytes))
	if p := h.run("create-map", "--name", "N", "--markers", "@markers.json").failure(t, exitUsage, "INPUT_TOO_LARGE"); !strings.Contains(p.Message, "markers.json") {
		t.Fatalf("error = %+v", p)
	}
}

func TestAFileIsReadFromTheWorkingDirectory(t *testing.T) {
	t.Parallel()
	s := newService(t, func(request) answer { return tool(map[string]any{"id": "m1"}) })
	h := newHarness(t, s)
	h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_env"
	h.file("markers.json", `[{"position": [1, 2], "title": "From a file"}]`)
	h.run("create-map", "--name", "N", "--markers", "@markers.json").success(t)
	if sent, _ := json.Marshal(s.only().Arguments["markers"]); string(sent) != `[{"position":[1,2],"title":"From a file"}]` {
		t.Fatalf("sent %s", sent)
	}
}

// TestAnEnvironmentKeyOnlyGoesToItsService is ADR 0004: --url alone, such as a prompt could slip into
// an agent's command, cannot send $SNAPHOP_MAPS_API_KEY to another service.
func TestAnEnvironmentKeyOnlyGoesToItsService(t *testing.T) {
	t.Parallel()
	t.Run("--url names another service", func(t *testing.T) {
		t.Parallel()
		home, elsewhere := newService(t, never(t)), newService(t, never(t))
		h := newHarness(t, home)
		h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_environment"
		p := h.run("list-maps", "--url", elsewhere.server.URL).failure(t, exitUsage, "API_KEY_REQUIRED")
		if !strings.Contains(p.Hint, "$SNAPHOP_MAPS_URL") {
			t.Fatalf("hint = %q", p.Hint)
		}
		if out := h.run("credentials", "--url", elsewhere.server.URL).success(t); out["keySource"] != "none" {
			t.Fatalf("credentials = %v", out)
		}
	})
	t.Run("the environment names no service", func(t *testing.T) {
		t.Parallel()
		s := newService(t, never(t))
		h := newHarness(t, s)
		delete(h.vars, "SNAPHOP_MAPS_URL")
		h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_environment"
		h.run("list-maps", "--url", s.server.URL).failure(t, exitUsage, "API_KEY_REQUIRED")
	})
	t.Run("the environment names no valid service", func(t *testing.T) {
		t.Parallel()
		s := newService(t, never(t))
		h := newHarness(t, s)
		h.vars["SNAPHOP_MAPS_URL"] = "ftp://nowhere"
		h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_environment"
		h.run("list-maps", "--url", s.server.URL).failure(t, exitUsage, "API_KEY_REQUIRED")
	})
	t.Run("--url names the environment's service", func(t *testing.T) {
		t.Parallel()
		s := newService(t, func(request) answer { return tool(map[string]any{"maps": []any{}}) })
		h := newHarness(t, s)
		h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_environment"
		h.run("list-maps", "--url", s.server.URL+"/").success(t)
		if r := s.only(); r.Auth != "Bearer shk_environment" {
			t.Fatalf("Authorization = %q", r.Auth)
		}
	})
	t.Run("the key kept for the other service is sent instead", func(t *testing.T) {
		t.Parallel()
		home := newService(t, never(t))
		elsewhere := newService(t, func(request) answer { return tool(map[string]any{"maps": []any{}}) })
		h := newHarness(t, home)
		h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_environment"
		if err := h.store().Put(elsewhere.server.URL, credentials.Account{APIKey: "shk_kept_elsewhere"}); err != nil {
			t.Fatal(err)
		}
		h.run("list-maps", "--url", elsewhere.server.URL).success(t)
		if r := elsewhere.only(); r.Auth != "Bearer shk_kept_elsewhere" {
			t.Fatalf("Authorization = %q", r.Auth)
		}
	})
}

// TestNoErrorEchoesAKey covers the ways a key could reach standard error: a mistyped flag that makes
// it the command's name, a stray argument, and an error body that repeats the request's header, whole
// or cut short.
func TestNoErrorEchoesAKey(t *testing.T) {
	t.Parallel()
	echo := func(r request) answer {
		return answer{status: 502, body: `{"debug": {"authorization": "` + r.Auth + `", "arguments": ` + r.Raw + `}}`}
	}
	// A key of the service's own shape, which no flag or variable of the run holds.
	unknown := "sh_agent_" + strings.Repeat("Q", 43)
	cases := []struct {
		name     string
		respond  func(request) answer
		env      string
		kept     string
		args     []string
		exit     int
		code     string
		redacted bool
	}{
		{"a mistyped flag", nil, "shk_environment_key", "", []string{"--apikey", "shk_environment_key", "list-maps"}, exitUsage, "INVALID_FLAG", false},
		{"a mistyped flag with an unknown key", nil, "", "", []string{"--apikey=" + unknown, "list-maps"}, exitUsage, "INVALID_FLAG", false},
		{"an unknown key as the command", nil, "", "", []string{unknown}, exitUsage, "UNKNOWN_COMMAND", true},
		{"a stray argument", nil, "", "", []string{"list-maps", "shk_flag_key", "--api-key", "shk_flag_key"}, exitUsage, "UNEXPECTED_ARGUMENT", true},
		{"an unknown key as a stray argument", nil, "", "", []string{"list-maps", unknown}, exitUsage, "UNEXPECTED_ARGUMENT", true},
		{"a header echoed back", echo, "", "shk_kept_key", []string{"list-maps"}, exitTransport, "HTTP_502", true},
		{"an argument echoed back", echo, "", "", []string{"list-maps", "--args", `{"apiKey": "shk_argument_key"}`}, exitTransport, "HTTP_502", true},
		{"a body after a warning", func(request) answer { return answer{status: 404, body: "shk_kept_key"} }, "", "shk_kept_key", []string{"list-maps"}, exitTransport, "HTTP_404", true},
		{"a key where the body is cut", func(r request) answer {
			return answer{status: 502, body: strings.Repeat("x", 180) + " " + strings.TrimPrefix(r.Auth, "Bearer ")}
		}, "", "shk_kept_key_" + strings.Repeat("K", 40), []string{"list-maps"}, exitTransport, "HTTP_502", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			respond := tc.respond
			if respond == nil {
				respond = never(t)
			}
			h := newHarness(t, newService(t, respond))
			h.vars["SNAPHOP_MAPS_API_KEY"] = tc.env
			if tc.kept != "" {
				h.keep(credentials.Account{APIKey: tc.kept, ExpiresAt: "2026-09-29T00:00:00Z"})
			}
			o := h.run(tc.args...)
			o.failure(t, tc.exit, tc.code)
			if strings.Contains(o.stderr, "shk_") || strings.Contains(o.stderr, "sh_agent_") || strings.Contains(o.stderr, "QQQQ") ||
				strings.Contains(o.stderr, "[REDACTED]") != tc.redacted {
				t.Fatalf("stderr = %s", o.stderr)
			}
		})
	}
}

func TestAShortValueIsNotTakenForAKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t, newService(t, func(request) answer { return answer{status: 404, body: "Check the key"} }))
	h.vars["SNAPHOP_MAPS_API_KEY"] = "key"
	if p := h.run("list-maps").failure(t, exitTransport, "HTTP_404"); !strings.Contains(p.Message, "Check the key") {
		t.Fatalf("error = %+v", p)
	}
}

// TestCommandLinesThatWouldSendTheWrongThing are mistakes that once sent something other than what was
// meant: a switch given its value after a space, an id given twice, a JSON value with more after it,
// and apiKey given as something other than a key.
func TestCommandLinesThatWouldSendTheWrongThing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		code string
		says string
	}{
		{"a switch's value after a space", []string{"update-map", "--id", "m1", "--name", "X", "--publish", "false"}, "INVALID_FLAG", "--publish=false"},
		{"a switch's value after the id", []string{"update-map", "m1", "--publish", "FALSE"}, "INVALID_FLAG", "--publish=false"},
		{"a confirmation's value", []string{"withdraw-map", "m1", "--yes", "true"}, "INVALID_FLAG", "--yes=true"},
		{"the id twice", []string{"withdraw-map", "--id", "keep-me", "--yes", "other"}, "CONFLICTING_ARGUMENT", `"keep-me"`},
		{"a stray bracket", []string{"create-map", "--name", "N", "--markers", `[{"position": [1, 2], "title": "T"}]]`}, "INVALID_JSON", "more follows"},
		{"a stray brace", []string{"call", "list_maps", "--args", `{"style": "dark"}}`}, "INVALID_JSON", "more follows"},
		{"apiKey null", []string{"list-maps", "--args", `{"apiKey": null}`}, "INVALID_FLAG", "apiKey"},
		{"apiKey empty", []string{"list-maps", "--args", `{"apiKey": ""}`}, "INVALID_FLAG", "apiKey"},
		{"a command flag before the command", []string{"--name", "N", "create-map"}, "INVALID_FLAG", "--name is not a flag"},
		{"a mistyped flag and no command", []string{"--verison"}, "INVALID_FLAG", "--verison is not a flag"},
		{"an unknown flag and no command", []string{"--pretty", "--json=1"}, "INVALID_FLAG", "--json is not a flag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, newService(t, never(t)))
			h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_env"
			if p := h.run(tc.args...).failure(t, exitUsage, tc.code); !strings.Contains(p.Message+p.Hint, tc.says) {
				t.Fatalf("error = %+v, want %q", p, tc.says)
			}
		})
	}
}

func TestLinesThatMeanWhatTheySay(t *testing.T) {
	t.Parallel()
	cases := []struct {
		args      []string
		arguments string
	}{
		{[]string{"get-map", "m1", "--id", "m1"}, `{"id":"m1"}`},
		{[]string{"update-map", "m1", "--publish=false"}, `{"id":"m1","publish":false}`},
		{[]string{"--url=" + "SERVICE", "--timeout=5s", "list-maps"}, `{}`},
		{[]string{"create-map", "--name", "true"}, `{"name":"true"}`},
		{[]string{"get-map", "true"}, `{"id":"true"}`},
		{[]string{"get-map", "--timeout", "5s", "false"}, `{"id":"false"}`},
		{[]string{"update-map", "--publish=true", "false"}, `{"id":"false","publish":true}`},
		// A value that is a switch's name is a value, not that switch.
		{[]string{"update-map", "--name", "publish", "true"}, `{"id":"true","name":"publish"}`},
		{[]string{"update-map", "--name", "--publish", "true"}, `{"id":"true","name":"--publish"}`},
		{[]string{"get-map", "--api-key", "pretty", "true"}, `{"id":"true"}`},
		{[]string{"get-map", "--pretty=false", "true"}, `{"id":"true"}`},
		{[]string{"--help", "get-map", "m1"}, ""},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()
			s := newService(t, func(request) answer { return tool(map[string]any{"ok": true}) })
			h := newHarness(t, s)
			h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_env"
			args := append([]string{}, tc.args...)
			for i, arg := range args {
				args[i] = strings.ReplaceAll(arg, "SERVICE", s.server.URL)
			}
			o := h.run(args...)
			if tc.arguments == "" {
				if o.code != exitOK || !strings.Contains(o.stdout, "Usage: snaphop-maps get-map") {
					t.Fatalf("exit %d, stdout %s, stderr %s", o.code, o.stdout, o.stderr)
				}
				return
			}
			o.success(t)
			if r := s.only(); r.Raw != tc.arguments {
				t.Fatalf("sent %s, want %s", r.Raw, tc.arguments)
			}
		})
	}
}

// TestLocalCommandsNeedNoService: help, version, schema and skill read neither --url nor --timeout,
// so a service address the environment gets wrong does not stop them.
func TestLocalCommandsNeedNoService(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	h.vars["SNAPHOP_MAPS_URL"] = "http://192.168.1.10:8084"
	for _, args := range [][]string{{"help"}, {"help", "get-map"}, {"version"}, {"schema"}, {"skill"}, {"version", "--timeout", "soon"}} {
		if o := h.run(args...); o.code != exitOK {
			t.Fatalf("%v: exit %d, stderr %s", args, o.code, o.stderr)
		}
	}
	p := h.run("list-maps").failure(t, exitUsage, "INVALID_URL")
	if !strings.Contains(p.Message, "$SNAPHOP_MAPS_URL") || strings.Contains(p.Message, "--url") || !strings.Contains(p.Hint, "$SNAPHOP_MAPS_URL") {
		t.Fatalf("error = %+v", p)
	}
}

// TestStandardErrorIsOneDocumentPerLine, as the schema promises, even with --pretty.
func TestStandardErrorIsOneDocumentPerLine(t *testing.T) {
	t.Parallel()
	h := newHarness(t, newService(t, func(request) answer { return tool(map[string]any{"maps": []any{}}) }))
	h.keep(credentials.Account{APIKey: "shk_kept", ExpiresAt: "2026-10-01T00:00:00Z"})
	o := h.run("list-maps", "--pretty")
	if lines := strings.Split(strings.TrimSpace(o.stderr), "\n"); len(lines) != 1 || !strings.Contains(o.stdout, "\n  ") {
		t.Fatalf("stdout %q, stderr %q", o.stdout, o.stderr)
	}
	o = h.run("get-map", "--pretty")
	if lines := strings.Split(strings.TrimSpace(o.stderr), "\n"); o.code != exitUsage || len(lines) != 1 {
		t.Fatalf("stderr %q", o.stderr)
	}
}

// TestAnUnknownToolIsNotCalledSafeToRepeat: `call` exists for tools newer than this build, which may
// change something.
func TestAnUnknownToolIsNotCalledSafeToRepeat(t *testing.T) {
	t.Parallel()
	h := newHarness(t, newService(t, func(request) answer { return answer{status: 504} }))
	h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_env"
	p := h.run("call", "delete_workspace", "--yes").failure(t, exitTransport, "HTTP_504")
	if strings.Contains(p.Hint, "safe to repeat") || !strings.Contains(p.Hint, "may have been carried out") {
		t.Fatalf("hint = %q", p.Hint)
	}
	if p := h.run("call", "list_maps").failure(t, exitTransport, "HTTP_504"); !strings.Contains(p.Hint, "safe to repeat") {
		t.Fatalf("hint = %q", p.Hint)
	}
}

// TestA401WithoutAKeyIsNotTheKeysRefusal: nothing sent a key, so something else refused.
func TestA401WithoutAKeyIsNotTheKeysRefusal(t *testing.T) {
	t.Parallel()
	h := newHarness(t, newService(t, func(request) answer { return answer{status: http.StatusUnauthorized} }))
	for _, args := range [][]string{{"ping"}, {"register-agent", "--name", "A", "--no-save"}} {
		p := h.run(args...).failure(t, exitTransport, "HTTP_401")
		if p.OutcomeKnown == nil || !*p.OutcomeKnown || strings.Contains(p.Hint, "register") {
			t.Fatalf("%v: error = %+v", args, p)
		}
	}
}
