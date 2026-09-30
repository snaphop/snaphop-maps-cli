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
			if spec.Argument != "" {
				flags[spec.Argument] = spec
				if _, known := listedTool.InputSchema.Properties[spec.Argument]; !known {
					t.Errorf("%s --%s sends %s, which %s does not take", cmd.Name, spec.Name, spec.Argument, listedTool.Name)
				}
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
		"HTTPS://Maps.SnapHop.AI/":    "https://maps.snaphop.ai",
		"https://maps.snaphop.ai":     "https://maps.snaphop.ai",
		"https://example.com/maps/":   "https://example.com/maps",
		"http://localhost:8080":       "http://localhost:8080",
		"http://LOCALHOST:8080":       "http://localhost:8080",
		"http://127.0.0.1:8080/":      "http://127.0.0.1:8080",
		"http://[::1]:8080":           "http://[::1]:8080",
		"https://maps.snaphop.ai:443": "https://maps.snaphop.ai:443",
	}
	for raw, want := range cases {
		if got, err := serviceURL(raw); err != nil || got != want {
			t.Errorf("serviceURL(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
}

func TestTheServiceRefusal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		structured any
		code       string
		hint       bool
	}{
		{"known code", map[string]any{"error": map[string]any{"code": "MAP_INVALID", "message": "The map is invalid",
			"fields": map[string]any{"markers[0].position": "is [longitude, latitude]"}}}, "MAP_INVALID", true},
		{"unknown code", map[string]any{"error": map[string]any{"code": "SOMETHING_NEW", "message": "New"}}, "SOMETHING_NEW", false},
		{"no error object", map[string]any{"why": "unknown"}, "REFUSED", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, func(request) answer { return refusal(tc.structured) })
			h := newHarness(t, s)
			h.vars["SNAPHOP_MAPS_API_KEY"] = "k"
			o := h.run("create-map", "--name", "N")
			p := o.failure(t, exitRefused, tc.code)
			if (p.Hint != "") != tc.hint {
				t.Fatalf("hint = %q", p.Hint)
			}
			if o.stdout != "" {
				t.Fatalf("stdout = %q", o.stdout)
			}
			if tc.code == "MAP_INVALID" && !strings.Contains(string(p.Fields), "markers[0].position") {
				t.Fatalf("fields = %s", p.Fields)
			}
			if tc.code == "REFUSED" && !strings.Contains(string(p.Detail), "unknown") {
				t.Fatalf("detail = %s", p.Detail)
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
		{"not json-rpc", answer{status: 200, body: "<html>"}, []string{"tools"}, "INVALID_RESPONSE", true, "--url"},
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
