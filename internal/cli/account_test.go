package cli

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/snaphop/snaphop-maps-cli/internal/credentials"
)

//go:embed testdata
var testdata embed.FS

var errNoHome = errors.New("$HOME is not defined")

func registered(apiKey string) map[string]any {
	return map[string]any{
		"agentId": "a1", "workspaceId": "w1", "apiKey": apiKey, "keyId": "k1",
		"scopes": []any{"EDIT_MAPS", "PUBLISH_MAPS", "READ_MAPS"}, "expiresAt": "2026-10-30T12:00:00Z",
		"limits": map[string]any{"mapsPerWorkspace": 25, "markersPerMap": 500, "inactivityDays": 7},
	}
}

func replaced(apiKey string) map[string]any {
	return map[string]any{"apiKey": apiKey, "keyId": "k2", "scopes": []any{"READ_MAPS"}, "expiresAt": "2026-11-29T12:00:00Z"}
}

func TestRegisterAgentKeepsTheKey(t *testing.T) {
	t.Parallel()
	s := newService(t, func(request) answer { return tool(registered("shk_new")) })
	h := newHarness(t, s)
	h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_never_sent"
	out := h.run("register-agent", "--name", "Trip planner", "--workspace-name", "Trips").success(t)
	if out["apiKey"] != "shk_new" {
		t.Fatalf("stdout = %v", out)
	}
	r := s.only()
	if r.Tool != "register_agent" || r.Auth != "" || r.Arguments["name"] != "Trip planner" || r.Arguments["workspaceName"] != "Trips" {
		t.Fatalf("request = %+v", r)
	}
	account, ok := h.kept()
	want := credentials.Account{APIKey: "shk_new", KeyID: "k1", ExpiresAt: "2026-10-30T12:00:00Z",
		Scopes: []string{"EDIT_MAPS", "PUBLISH_MAPS", "READ_MAPS"}, AgentID: "a1", WorkspaceID: "w1", SavedAt: "2026-09-30T12:00:00Z"}
	if !ok || strings.Join(account.Scopes, ",") != strings.Join(want.Scopes, ",") || account.APIKey != want.APIKey ||
		account.AgentID != want.AgentID || account.WorkspaceID != want.WorkspaceID || account.SavedAt != want.SavedAt ||
		account.KeyID != want.KeyID || account.ExpiresAt != want.ExpiresAt {
		t.Fatalf("kept %+v, want %+v", account, want)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(h.store().Path)
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("credentials mode %v", info.Mode().Perm())
		}
	}
}

func TestRegisterAgentWillNotLoseAKeptAccount(t *testing.T) {
	t.Parallel()
	s := newService(t, func(request) answer { return tool(registered("shk_second")) })
	h := newHarness(t, s)
	h.keep(credentials.Account{APIKey: "shk_first"})
	p := h.run("register-agent", "--name", "A").failure(t, exitUsage, "ACCOUNT_ALREADY_KEPT")
	if !strings.Contains(p.Hint, "--overwrite") || len(s.received()) != 0 {
		t.Fatalf("error %+v after %d requests", p, len(s.received()))
	}
	h.run("register-agent", "--name", "A", "--no-save").success(t)
	if account, _ := h.kept(); account.APIKey != "shk_first" {
		t.Fatalf("--no-save replaced the kept key with %s", account.APIKey)
	}
	h.run("call", "register_agent", "--args", `{"name": "A"}`, "--overwrite").success(t)
	if account, _ := h.kept(); account.APIKey != "shk_second" {
		t.Fatalf("--overwrite kept %s", account.APIKey)
	}
}

func TestANewKeyThatCannotBeKeptIsStillPrinted(t *testing.T) {
	t.Parallel()
	t.Run("the file is in the way", func(t *testing.T) {
		t.Parallel()
		s := newService(t, never(t))
		h := newHarness(t, s)
		blocker := h.file("blocker", "")
		h.vars["SNAPHOP_MAPS_CREDENTIALS"] = filepath.Join(blocker, "credentials.json")
		// Unix cannot read through a file; Windows reads the path as missing and cannot create it. Either
		// way nothing is sent, so no key is issued that could not be kept.
		o := h.run("register-agent", "--name", "A")
		if o.code != exitFailure || !strings.Contains(o.stderr, "CREDENTIALS_UN") || o.stdout != "" {
			t.Fatalf("exit %d, stdout %s, stderr %s", o.code, o.stdout, o.stderr)
		}
	})
	t.Run("the file cannot be written", func(t *testing.T) {
		t.Parallel()
		s := newService(t, never(t))
		h := newHarness(t, s)
		// A name the file system takes, but not with the temporary file's suffix added.
		h.vars["SNAPHOP_MAPS_CREDENTIALS"] = filepath.Join(h.dir, strings.Repeat("c", 240))
		for _, args := range [][]string{{"register-agent", "--name", "A"}, {"replace-key", "--api-key", "shk_old"}} {
			p := h.run(args...).failure(t, exitFailure, "CREDENTIALS_UNWRITABLE")
			if !strings.Contains(p.Message, "Nothing was sent") || !strings.Contains(p.Hint, "--no-save") {
				t.Fatalf("error %+v", p)
			}
		}
	})
	t.Run("the answer has no key", func(t *testing.T) {
		t.Parallel()
		s := newService(t, func(request) answer { return tool(map[string]any{"agentId": "a1"}) })
		h := newHarness(t, s)
		// There is no key on standard output to give the user: the answer is one this program cannot read.
		for args, hint := range map[string]string{"register-agent --name A": "may have been opened", "replace-key --api-key shk_old": "same key"} {
			o := h.run(strings.Fields(args)...)
			p := o.failure(t, exitTransport, "INVALID_RESPONSE")
			if !strings.Contains(p.Message, "no apiKey") || !strings.Contains(p.Hint, hint) || p.OutcomeKnown == nil || *p.OutcomeKnown ||
				decode(t, o.stdout)["agentId"] != "a1" {
				t.Fatalf("%s: error %+v, stdout %s", args, p, o.stdout)
			}
		}
		if _, ok := h.kept(); ok {
			t.Fatal("an account without a key was kept")
		}
		h.stdout = failingWriter{}
		if p := h.run("register-agent", "--name", "A").failure(t, exitUnwritten, "OUTPUT_FAILED"); strings.Contains(p.Hint, "is kept") {
			t.Fatalf("error %+v", p)
		}
	})
}

func TestReplaceKeyReplacesTheKeyUsed(t *testing.T) {
	t.Parallel()
	s := newService(t, func(request) answer { return tool(replaced("shk_new")) })
	h := newHarness(t, s)
	h.keep(credentials.Account{APIKey: "shk_old", KeyID: "k1", AgentID: "a1", WorkspaceID: "w1"})
	if out := h.run("replace-key").success(t); out["apiKey"] != "shk_new" {
		t.Fatalf("stdout = %v", out)
	}
	if r := s.only(); r.Auth != "Bearer shk_old" || r.Tool != "replace_key" {
		t.Fatalf("request = %+v", r)
	}
	account, _ := h.kept()
	if account.APIKey != "shk_new" || account.KeyID != "k2" || account.ExpiresAt != "2026-11-29T12:00:00Z" ||
		account.AgentID != "a1" || account.WorkspaceID != "w1" || account.SavedAt == "" {
		t.Fatalf("kept %+v", account)
	}
}

func TestReplaceKeyNeverOverwritesAnotherAccount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		env   string
		args  []string
		kept  string
		exit  int
		after string
	}{
		{name: "another account is kept", env: "shk_other", kept: "shk_mine", exit: exitNotSaved, after: "shk_mine"},
		{name: "the same key from the environment", env: "shk_mine", kept: "shk_mine", exit: exitOK, after: "shk_new"},
		{name: "the same key as an argument", args: []string{"--args", `{"apiKey": "shk_mine"}`}, kept: "shk_mine", exit: exitOK, after: "shk_new"},
		{name: "nothing kept yet", env: "shk_other", exit: exitOK, after: "shk_new"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, func(request) answer { return tool(replaced("shk_new")) })
			h := newHarness(t, s)
			h.vars["SNAPHOP_MAPS_API_KEY"] = tc.env
			if tc.kept != "" {
				h.keep(credentials.Account{APIKey: tc.kept})
			}
			o := h.run(append([]string{"replace-key"}, tc.args...)...)
			if o.code != tc.exit || decode(t, o.stdout)["apiKey"] != "shk_new" {
				t.Fatalf("exit %d, stdout %s, stderr %s", o.code, o.stdout, o.stderr)
			}
			if account, _ := h.kept(); account.APIKey != tc.after {
				t.Fatalf("kept %s, want %s", account.APIKey, tc.after)
			}
		})
	}
}

func TestCredentialsNeverShowsTheKey(t *testing.T) {
	t.Parallel()
	s := newService(t, never(t))
	h := newHarness(t, s)
	out := h.run("credentials").success(t)
	if out["keySource"] != "none" || out["account"] != nil || out["url"] != h.service() || out["path"] != h.store().Path {
		t.Fatalf("credentials = %v", out)
	}
	h.keep(credentials.Account{APIKey: "shk_secret", KeyID: "k1", ExpiresAt: "2026-10-01T00:00:00Z"})
	o := h.run("credentials")
	out = o.success(t)
	if out["keySource"] != "file" || out["account"].(map[string]any)["keyId"] != "k1" || strings.Contains(o.stdout, "shk_secret") {
		t.Fatalf("credentials = %s", o.stdout)
	}
	if !strings.Contains(o.stderr, "API_KEY_EXPIRING") {
		t.Fatalf("stderr = %s", o.stderr)
	}
	h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_env"
	o = h.run("credentials")
	out = o.success(t)
	if out["keySource"] != "environment" || out["account"].(map[string]any)["keyId"] != "k1" || strings.Contains(o.stdout, "shk_") {
		t.Fatalf("credentials = %s", o.stdout)
	}
	out = h.run("credentials", "--api-key", "shk_flag", "--url", "http://127.0.0.1:1").success(t)
	if out["keySource"] != "flag" || out["account"] != nil {
		t.Fatalf("credentials = %v", out)
	}
}

func TestHelp(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}, {"--pretty"}} {
		o := h.run(args...)
		if o.code != exitOK || !strings.Contains(o.stdout, "register-agent") || !strings.Contains(o.stdout, "Exit statuses") {
			t.Fatalf("%v: exit %d, %s", args, o.code, o.stdout)
		}
	}
	for _, cmd := range commands() {
		for _, args := range [][]string{{"help", cmd.Name}, {cmd.Name, "--help"}} {
			o := h.run(args...)
			if o.code != exitOK || !strings.Contains(o.stdout, "snaphop-maps "+cmd.Name) || !strings.Contains(o.stdout, cmd.Example) {
				t.Fatalf("%v: exit %d, %s", args, o.code, o.stdout)
			}
			if cmd.Tool != "" && !strings.Contains(o.stdout, "MCP tool: "+cmd.Tool) {
				t.Fatalf("%v does not name its tool: %s", args, o.stdout)
			}
		}
	}
	if o := h.run("help", "get-map"); !strings.Contains(o.stdout, "--id string (required)") || !strings.Contains(o.stdout, "[ID]") {
		t.Fatalf("help get-map = %s", o.stdout)
	}
}

func TestSchemaDescribesEveryCommand(t *testing.T) {
	t.Parallel()
	out := newHarness(t, nil).run("schema").success(t)
	if out["name"] != "snaphop-maps" || out["version"] != "1.2.3" || out["defaultUrl"] != DefaultURL {
		t.Fatalf("schema = %v", out)
	}
	listed := out["commands"].([]any)
	if len(listed) != len(commands()) {
		t.Fatalf("schema lists %d commands, want %d", len(listed), len(commands()))
	}
	var create map[string]any
	for _, entry := range listed {
		if entry.(map[string]any)["name"] == "create-map" {
			create = entry.(map[string]any)
		}
	}
	if create["tool"] != "create_map" || create["needsApiKey"] != true {
		t.Fatalf("create-map = %v", create)
	}
	if len(out["exitStatuses"].([]any)) != 8 || len(out["environment"].([]any)) != 3 || len(out["globalFlags"].([]any)) != 5 {
		t.Fatalf("schema = %v", out)
	}
	for _, entry := range listed {
		cmd := entry.(map[string]any)
		if _, isList := cmd["flags"].([]any); !isList {
			t.Errorf("%s's flags are %v, not a list", cmd["name"], cmd["flags"])
		}
		// A command that writes files or sends a key says so.
		if name := cmd["name"]; (name == "skill" && cmd["readOnly"] != false) || (name == "call" && cmd["needsApiKey"] != true) {
			t.Errorf("%s = %v", name, cmd)
		}
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()
	out := newHarness(t, nil).run("version").success(t)
	if out["version"] != "1.2.3" || out["protocolVersion"] != "2025-06-18" {
		t.Fatalf("version = %v", out)
	}
}

// TestAKeyKeptMeanwhileIsNeverLost has another command keep a key while this one waits for its
// answer, as parallel runs do. What this one keeps is decided against the file as it is by then.
func TestAKeyKeptMeanwhileIsNeverLost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		before    string
		args      []string
		answer    answer
		meanwhile string
		exit      int
		after     string
	}{
		{"a registration while another registers", "", []string{"register-agent", "--name", "A"}, tool(registered("shk_new")), "shk_other", exitNotSaved, "shk_other"},
		{"an overwrite after the account changed", "shk_old", []string{"register-agent", "--name", "A", "--overwrite"}, tool(registered("shk_new")), "shk_other", exitNotSaved, "shk_other"},
		{"an overwrite of the account seen", "shk_old", []string{"register-agent", "--name", "A", "--overwrite"}, tool(registered("shk_new")), "shk_old", exitOK, "shk_new"},
		{"a replacement after another key was kept", "shk_old", []string{"replace-key"}, tool(replaced("shk_new")), "shk_other", exitNotSaved, "shk_other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var h *harness
			s := newService(t, func(request) answer {
				h.keep(credentials.Account{APIKey: tc.meanwhile})
				return tc.answer
			})
			h = newHarness(t, s)
			if tc.before != "" {
				h.keep(credentials.Account{APIKey: tc.before})
			}
			o := h.run(tc.args...)
			if o.code != tc.exit || decode(t, o.stdout)["apiKey"] != "shk_new" {
				t.Fatalf("exit %d, stdout %s, stderr %s", o.code, o.stdout, o.stderr)
			}
			if tc.exit == exitNotSaved && !strings.Contains(o.stderr, "CREDENTIALS_NOT_SAVED") {
				t.Fatalf("stderr = %s", o.stderr)
			}
			if account, _ := h.kept(); account.APIKey != tc.after {
				t.Fatalf("kept %s, want %s", account.APIKey, tc.after)
			}
		})
	}
}

// TestANewKeyIsKeptBeforeItIsPrinted is standard output whose reader has gone, as when an agent pipes
// the answer into a command that ends early: the key printed there is lost, so it must be kept first.
func TestANewKeyIsKeptBeforeItIsPrinted(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		kept string
		hint string
	}{
		{"kept", []string{"register-agent", "--name", "A"}, "shk_new", "kept in the credentials file"},
		{"not kept", []string{"register-agent", "--name", "A", "--no-save"}, "", "neither printed nor kept"},
		{"a replacement not kept", []string{"replace-key", "--api-key", "shk_old", "--no-save"}, "", "Repeat it with the same key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, func(request) answer { return tool(registered("shk_new")) })
			h := newHarness(t, s)
			h.stdout = failingWriter{}
			p := h.run(tc.args...).failure(t, exitUnwritten, "OUTPUT_FAILED")
			if !strings.Contains(p.Message, "broken pipe") || !strings.Contains(p.Hint, tc.hint) {
				t.Fatalf("error = %+v", p)
			}
			if account, _ := h.kept(); account.APIKey != tc.kept {
				t.Fatalf("kept %q, want %q", account.APIKey, tc.kept)
			}
		})
	}
}

// TestAnAnswerThatCannotBePrintedIsReported covers every other command's answer.
func TestAnAnswerThatCannotBePrintedIsReported(t *testing.T) {
	t.Parallel()
	cases := []struct {
		args []string
		hint string
	}{
		{[]string{"create-map", "--name", "N"}, "list-maps"},
		{[]string{"get-map", "m1"}, "safe to repeat"},
		{[]string{"ping"}, "Run it again"},
		{[]string{"version"}, "Run it again"},
		{[]string{"help"}, "Run it again"},
		{nil, "Run it again"},
		{[]string{"get-map", "--help"}, "Run it again"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, newService(t, func(request) answer { return tool(map[string]any{"id": "m1"}) }))
			h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_env"
			h.stdout = failingWriter{}
			if p := h.run(tc.args...).failure(t, exitUnwritten, "OUTPUT_FAILED"); !strings.Contains(p.Hint, tc.hint) {
				t.Fatalf("hint = %q, want %q", p.Hint, tc.hint)
			}
		})
	}
}

// TestAReplacedEnvironmentKeyIsNotSent is replace-key run with $SNAPHOP_MAPS_API_KEY: the file then
// keeps the new key, and the old one, still in the environment, must not keep being sent in its place.
func TestAReplacedEnvironmentKeyIsNotSent(t *testing.T) {
	t.Parallel()
	for _, before := range []string{"", "shk_old"} {
		t.Run("kept before: "+before, func(t *testing.T) {
			t.Parallel()
			s := newService(t, func(r request) answer {
				if r.Tool == "replace_key" {
					return tool(replaced("shk_new"))
				}
				return tool(map[string]any{"maps": []any{}})
			})
			h := newHarness(t, s)
			h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_old"
			if before != "" {
				h.keep(credentials.Account{APIKey: before, AgentID: "a1"})
			}
			h.run("replace-key").success(t)
			if account, _ := h.kept(); account.APIKey != "shk_new" || !account.Replaced(digest("shk_old")) {
				t.Fatalf("kept %+v", account)
			}
			o := h.run("list-maps")
			o.success(t)
			if r := s.received()[1]; r.Auth != "Bearer shk_new" {
				t.Fatalf("Authorization = %q", r.Auth)
			}
			if p := problem(t, strings.TrimSpace(o.stderr), "warning"); p.Code != "ENVIRONMENT_KEY_REPLACED" || !strings.Contains(p.Hint, "Unset") {
				t.Fatalf("warning = %+v", p)
			}
			out := h.run("credentials").success(t)
			if out["keySource"] != "file" || out["sendsKeptKey"] != true {
				t.Fatalf("credentials = %v", out)
			}
		})
	}
}

// TestARefusedKeyNeverLeadsToLosingTheKeptOne: when a key from the environment or a flag is refused
// while another is kept for the service, registering again would replace the kept key.
func TestARefusedKeyNeverLeadsToLosingTheKeptOne(t *testing.T) {
	t.Parallel()
	for name, set := range map[string]func(h *harness) []string{
		"environment": func(h *harness) []string { h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_other"; return nil },
		"flag":        func(*harness) []string { return []string{"--api-key", "shk_other"} },
		"arguments":   func(*harness) []string { return []string{"--args", `{"apiKey": "shk_other"}`} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// The service refuses a header's key with a 401, and an argument's key as a tool's refusal.
			h := newHarness(t, newService(t, func(r request) answer {
				if r.Auth == "" {
					return refusal(map[string]any{"error": map[string]any{"code": "API_KEY_INVALID", "message": "unknown"}})
				}
				return answer{status: http.StatusUnauthorized}
			}))
			h.keep(credentials.Account{APIKey: "shk_kept", ExpiresAt: "2026-09-29T00:00:00Z"})
			o := h.run(append([]string{"list-maps"}, set(h)...)...)
			p := o.failure(t, exitRefused, "API_KEY_INVALID")
			if strings.Contains(p.Hint, "--overwrite") || !strings.Contains(p.Hint, "Leave") || !strings.Contains(p.Hint, h.store().Path) {
				t.Fatalf("hint = %q", p.Hint)
			}
			// The kept key is not the one sent, so its expiry is not this command's to warn of.
			if strings.Contains(o.stderr, "API_KEY_EXPIRED") {
				t.Fatalf("stderr = %s", o.stderr)
			}
			if name == "arguments" {
				return // credentials takes no --args
			}
			if out := h.run(append([]string{"credentials"}, set(h)...)...).success(t); out["sendsKeptKey"] != false || out["account"] == nil {
				t.Fatalf("credentials = %v", out)
			}
		})
	}
}

// TestAnEnvironmentKeyNeedsNoReadableFile: a key given in the environment is sent even when the
// credentials file cannot be read, as before the file was consulted for replaced keys.
func TestAnEnvironmentKeyNeedsNoReadableFile(t *testing.T) {
	t.Parallel()
	s := newService(t, func(request) answer { return tool(map[string]any{"maps": []any{}}) })
	h := newHarness(t, s)
	h.vars["SNAPHOP_MAPS_CREDENTIALS"] = h.dir // a directory, not a file
	h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_env"
	h.run("list-maps").success(t)
	h.run("list-maps", "--api-key", "shk_flag").success(t)
	if requests := s.received(); requests[0].Auth != "Bearer shk_env" || requests[1].Auth != "Bearer shk_flag" {
		t.Fatalf("requests = %+v", requests)
	}
}

func TestRelativePathsAreReadAgainstTheWorkingDirectory(t *testing.T) {
	t.Parallel()
	s := newService(t, func(request) answer { return tool(registered("shk_new")) })
	h := newHarness(t, s)
	h.run("register-agent", "--name", "A", "--credentials", "mine.json").success(t)
	if data, err := os.ReadFile(filepath.Join(h.dir, "mine.json")); err != nil || !strings.Contains(string(data), "shk_new") {
		t.Fatalf("mine.json: %v", err)
	}
	h.vars["SNAPHOP_MAPS_CREDENTIALS"] = "env.json"
	h.run("register-agent", "--name", "A").success(t)
	if _, err := os.Stat(filepath.Join(h.dir, "env.json")); err != nil {
		t.Fatal(err)
	}
	out := h.run("skill", "install", "--client", "claude", "--project", "project").success(t)
	if out["path"] != filepath.Join(h.dir, "project", ".claude", "skills", "snaphop-maps") {
		t.Fatalf("install = %v", out)
	}
}

// TestAKeyKeptUnderTheSchemesOwnPortIsFound: addresses once kept their scheme's own port, as in
// --url https://maps.snaphop.ai:443, and now drop it. An account kept that way is still found, and a
// new key for it takes its place under the address now, keeping the account's agent and workspace.
func TestAKeyKeptUnderTheSchemesOwnPortIsFound(t *testing.T) {
	t.Parallel()
	s := newService(t, func(r request) answer {
		if r.Tool == "register_agent" {
			return tool(registered("shk_registered"))
		}
		return tool(replaced("shk_new"))
	})
	h := newHarness(t, s)
	// Every request goes to the fake service, whatever address it is sent to.
	h.http = &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, s.server.Listener.Addr().String())
	}}}
	h.vars["SNAPHOP_MAPS_URL"] = "http://localhost"
	former := credentials.Store{Path: h.store().Path}
	if err := former.Put("http://localhost:80", credentials.Account{APIKey: "shk_old", AgentID: "a0", WorkspaceID: "w0"}); err != nil {
		t.Fatal(err)
	}
	h.run("register-agent", "--name", "A").failure(t, exitUsage, "ACCOUNT_ALREADY_KEPT")
	h.run("replace-key").success(t)
	if r := s.only(); r.Auth != "Bearer shk_old" {
		t.Fatalf("request = %+v", r)
	}
	account, _ := h.kept()
	if account.APIKey != "shk_new" || account.AgentID != "a0" || account.WorkspaceID != "w0" || !account.Replaced(digest("shk_old")) {
		t.Fatalf("kept %+v", account)
	}
	if data, _ := os.ReadFile(h.store().Path); strings.Contains(string(data), "localhost:80") {
		t.Fatalf("the replaced key was left under the former address: %s", data)
	}
	// An overwrite replaces the account kept under the former address too.
	h.vars["SNAPHOP_MAPS_CREDENTIALS"] = filepath.Join(h.dir, "former.json")
	former = h.store()
	if err := former.Put("http://localhost:80", credentials.Account{APIKey: "shk_old"}); err != nil {
		t.Fatal(err)
	}
	h.vars["SNAPHOP_MAPS_URL"] = "http://localhost:80"
	h.run("register-agent", "--name", "A", "--overwrite").success(t)
	if data, _ := os.ReadFile(h.store().Path); strings.Contains(string(data), "localhost:80") || strings.Contains(string(data), "shk_old") {
		t.Fatalf("the overwritten account was left under the former address: %s", data)
	}
}

// TestAnEnvironmentKeyReplacedLongAgoIsNotSent: $SNAPHOP_MAPS_API_KEY holds the first key, and
// replace-key has run twice since. The first key is still recognised as replaced, and never sent.
func TestAnEnvironmentKeyReplacedLongAgoIsNotSent(t *testing.T) {
	t.Parallel()
	next := 0
	s := newService(t, func(r request) answer {
		if r.Tool == "replace_key" {
			next++
			return tool(replaced(fmt.Sprintf("shk_key%d", next)))
		}
		return tool(map[string]any{"maps": []any{}})
	})
	h := newHarness(t, s)
	h.vars["SNAPHOP_MAPS_API_KEY"] = "shk_key0"
	h.keep(credentials.Account{APIKey: "shk_key0", AgentID: "a1"})
	h.run("replace-key").success(t)
	h.run("replace-key").success(t)
	h.run("list-maps").success(t)
	requests := s.received()
	for i, want := range []string{"shk_key0", "shk_key1", "shk_key2"} {
		if requests[i].Auth != "Bearer "+want {
			t.Fatalf("request %d sent %q, want %s", i, requests[i].Auth, want)
		}
	}
	account, _ := h.kept()
	if account.APIKey != "shk_key2" || account.AgentID != "a1" || !account.Replaced(digest("shk_key0")) || !account.Replaced(digest("shk_key1")) {
		t.Fatalf("kept %+v", account)
	}
}
