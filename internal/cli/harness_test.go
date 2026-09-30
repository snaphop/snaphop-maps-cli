package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/snaphop/snaphop-maps-cli/internal/credentials"
)

// now is the time every test runs at.
var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// request is one JSON-RPC request the fake service received.
type request struct {
	Method    string
	Tool      string
	Arguments map[string]any
	Params    map[string]any
	Auth      string
	// Raw is the arguments exactly as sent.
	Raw string
}

// answer is what the fake service sends back.
type answer struct {
	status int
	header http.Header
	body   string
}

func result(value any) answer {
	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": value})
	return answer{status: 200, body: string(data)}
}

func tool(structured any) answer {
	text, _ := json.Marshal(structured)
	return result(map[string]any{"content": []any{map[string]any{"type": "text", "text": string(text)}}, "structuredContent": structured})
}

func refusal(structured any) answer {
	return result(map[string]any{"isError": true, "structuredContent": structured})
}

// service is a fake SnapHop Maps MCP server.
type service struct {
	t        *testing.T
	server   *httptest.Server
	mu       sync.Mutex
	requests []request
	respond  func(request) answer
}

func newService(t *testing.T, respond func(request) answer) *service {
	t.Helper()
	s := &service{t: t, respond: respond}
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.server.Close)
	return s
}

func (s *service) handle(w http.ResponseWriter, r *http.Request) {
	var message struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	var raw struct {
		Params struct {
			Arguments json.RawMessage `json:"arguments"`
		} `json:"params"`
	}
	body, _ := io.ReadAll(r.Body)
	if r.URL.Path != "/mcp" || json.Unmarshal(body, &message) != nil || json.Unmarshal(body, &raw) != nil {
		s.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}
	req := request{Method: message.Method, Params: message.Params, Auth: r.Header.Get("Authorization"), Raw: string(raw.Params.Arguments)}
	if message.Method == "tools/call" {
		req.Tool, _ = message.Params["name"].(string)
		req.Arguments, _ = message.Params["arguments"].(map[string]any)
	}
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()
	a := s.respond(req)
	for name, values := range a.header {
		w.Header()[name] = values
	}
	w.WriteHeader(a.status)
	io.WriteString(w, a.body)
}

func (s *service) received() []request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]request(nil), s.requests...)
}

func (s *service) only() request {
	s.t.Helper()
	requests := s.received()
	if len(requests) != 1 {
		s.t.Fatalf("service received %d requests, want 1: %+v", len(requests), requests)
	}
	return requests[0]
}

// harness runs the program the way a shell would, against a fake service, in a directory of its own.
type harness struct {
	t         *testing.T
	dir       string
	vars      map[string]string
	stdin     io.Reader
	configDir func() (string, error)
	http      *http.Client
}

func newHarness(t *testing.T, s *service) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{t: t, dir: dir, vars: map[string]string{"SNAPHOP_MAPS_CREDENTIALS": filepath.Join(dir, "credentials.json")}, stdin: strings.NewReader("")}
	if s != nil {
		h.vars["SNAPHOP_MAPS_URL"] = s.server.URL
	}
	h.configDir = func() (string, error) { return filepath.Join(dir, "config"), nil }
	return h
}

type outcome struct {
	code   int
	stdout string
	stderr string
}

func (h *harness) run(args ...string) outcome {
	h.t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), Env{
		Args:      args,
		Stdin:     h.stdin,
		Stdout:    &stdout,
		Stderr:    &stderr,
		Getenv:    func(name string) string { return h.vars[name] },
		ConfigDir: h.configDir,
		HTTP:      h.http,
		Now:       func() time.Time { return now },
		Version:   "1.2.3",
	})
	return outcome{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// service is the address the harness's credentials are kept under.
func (h *harness) service() string { return h.vars["SNAPHOP_MAPS_URL"] }

func (h *harness) store() credentials.Store {
	return credentials.Store{Path: h.vars["SNAPHOP_MAPS_CREDENTIALS"]}
}

func (h *harness) keep(account credentials.Account) {
	h.t.Helper()
	if err := h.store().Put(h.service(), account); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) kept() (credentials.Account, bool) {
	h.t.Helper()
	account, ok, err := h.store().Get(h.service())
	if err != nil {
		h.t.Fatal(err)
	}
	return account, ok
}

// file writes a file in the harness's directory and returns its path.
func (h *harness) file(name, content string) string {
	h.t.Helper()
	path := filepath.Join(h.dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		h.t.Fatal(err)
	}
	return path
}

// problem decodes the error or warning on a line of standard error.
func problem(t *testing.T, line, key string) Problem {
	t.Helper()
	var document map[string]Problem
	if err := json.Unmarshal([]byte(line), &document); err != nil {
		t.Fatalf("stderr line %q: %v", line, err)
	}
	p, ok := document[key]
	if !ok {
		t.Fatalf("stderr line %q has no %s", line, key)
	}
	return p
}

// failure checks an outcome's exit status and the error it reported, and returns the error.
func (o outcome) failure(t *testing.T, code int, errorCode string) Problem {
	t.Helper()
	if o.code != code {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", o.code, code, o.stdout, o.stderr)
	}
	lines := strings.Split(strings.TrimSpace(o.stderr), "\n")
	p := problem(t, lines[len(lines)-1], "error")
	if p.Code != errorCode {
		t.Fatalf("error code %q, want %q: %s", p.Code, errorCode, o.stderr)
	}
	return p
}

// success checks an outcome succeeded and decodes its standard output.
func (o outcome) success(t *testing.T) map[string]any {
	t.Helper()
	if o.code != exitOK {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", o.code, o.stdout, o.stderr)
	}
	return decode(t, o.stdout)
}

func decode(t *testing.T, text string) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(text), &document); err != nil {
		t.Fatalf("stdout %q: %v", text, err)
	}
	return document
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("stdin closed") }

// never is a service that fails a test that reaches it.
func never(t *testing.T) func(request) answer {
	return func(r request) answer {
		t.Errorf("the service was reached: %+v", r)
		return answer{status: 500}
	}
}
