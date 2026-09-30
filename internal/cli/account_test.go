package cli

import (
	"embed"
	"errors"
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
		s := newService(t, func(request) answer { return tool(registered("shk_only_copy")) })
		h := newHarness(t, s)
		blocker := h.file("blocker", "")
		h.vars["SNAPHOP_MAPS_CREDENTIALS"] = filepath.Join(blocker, "credentials.json")
		// Unix cannot read through a file and refuses before sending anything; Windows reads the path
		// as missing, registers, and cannot keep the key. Either way no key is lost unseen.
		o := h.run("register-agent", "--name", "A")
		if o.code == exitNotSaved {
			o.failure(t, exitNotSaved, "CREDENTIALS_NOT_SAVED")
			if decode(t, o.stdout)["apiKey"] != "shk_only_copy" {
				t.Fatalf("stdout = %s", o.stdout)
			}
			return
		}
		o.failure(t, exitFailure, "CREDENTIALS_UNREADABLE")
		if len(s.received()) != 0 {
			t.Fatal("registered although the key could not be kept")
		}
	})
	t.Run("the file cannot be written", func(t *testing.T) {
		t.Parallel()
		s := newService(t, func(request) answer { return tool(registered("shk_only_copy")) })
		h := newHarness(t, s)
		// A name the file system takes, but not with the temporary file's suffix added.
		h.vars["SNAPHOP_MAPS_CREDENTIALS"] = filepath.Join(h.dir, strings.Repeat("c", 240))
		o := h.run("register-agent", "--name", "A")
		p := o.failure(t, exitNotSaved, "CREDENTIALS_NOT_SAVED")
		if decode(t, o.stdout)["apiKey"] != "shk_only_copy" || !strings.Contains(p.Hint, "only copy") {
			t.Fatalf("stdout %s, error %+v", o.stdout, p)
		}
	})
	t.Run("the answer has no key", func(t *testing.T) {
		t.Parallel()
		s := newService(t, func(request) answer { return tool(map[string]any{"agentId": "a1"}) })
		h := newHarness(t, s)
		h.run("register-agent", "--name", "A").failure(t, exitNotSaved, "CREDENTIALS_NOT_SAVED")
		if _, ok := h.kept(); ok {
			t.Fatal("an account without a key was kept")
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
	if len(out["exitStatuses"].([]any)) != 7 || len(out["environment"].([]any)) != 3 || len(out["globalFlags"].([]any)) != 5 {
		t.Fatalf("schema = %v", out)
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
