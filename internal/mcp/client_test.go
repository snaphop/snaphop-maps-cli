package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// answering is a client whose every request is answered with this status, headers and body.
func answering(status int, header http.Header, body string) *Client {
	return &Client{Endpoint: "https://maps.example/mcp", HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		if header == nil {
			header = http.Header{}
		}
		return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
}

func TestCallToolSendsOneStatelessRequest(t *testing.T) {
	t.Parallel()
	var got struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{
			"Content-Type":         "application/json",
			"Accept":               "application/json, text/event-stream",
			"MCP-Protocol-Version": ProtocolVersion,
			"Authorization":        "Bearer shk_key",
			"User-Agent":           "snaphop-maps-cli/test",
			"Origin":               "",
		}
		for header, want := range checks {
			if value := r.Header.Get(header); value != want {
				t.Errorf("%s = %q, want %q", header, value, want)
			}
		}
		if r.Method != http.MethodPost || r.URL.Path != "/mcp" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{}"}],"structuredContent":{"maps":[]}}}`)
	}))
	defer server.Close()
	client := &Client{Endpoint: server.URL + "/mcp", HTTP: server.Client(), UserAgent: "snaphop-maps-cli/test"}
	result, err := client.CallTool(context.Background(), "list_maps", map[string]any{"x": 1}, "shk_key")
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Structured) != `{"maps":[]}` || result.IsError {
		t.Fatalf("result = %s, %v", result.Structured, result.IsError)
	}
	if got.JSONRPC != "2.0" || got.ID != 1 || got.Method != "tools/call" || got.Params.Name != "list_maps" || got.Params.Arguments["x"] != 1.0 {
		t.Fatalf("request = %+v", got)
	}
}

func TestCallSendsNoAuthorizationWithoutAKey(t *testing.T) {
	t.Parallel()
	client := &Client{Endpoint: "https://maps.example/mcp", HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if _, sent := r.Header["Authorization"]; sent {
			t.Error("Authorization sent without a key")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{}}`))}, nil
	})}}
	result, err := client.Call(context.Background(), "ping", map[string]any{}, "")
	if err != nil || string(result) != "{}" {
		t.Fatalf("Call = %s, %v", result, err)
	}
}

func TestCallToolReportsTheToolsRefusal(t *testing.T) {
	t.Parallel()
	client := answering(200, nil, `{"jsonrpc":"2.0","id":1,"result":{"isError":true,"structuredContent":{"error":{"code":"MAP_NOT_FOUND","message":"No map"}}}}`)
	result, err := client.CallTool(context.Background(), "get_map", nil, "k")
	if err != nil || !result.IsError || !strings.Contains(string(result.Structured), "MAP_NOT_FOUND") {
		t.Fatalf("CallTool = %+v, %v", result, err)
	}
}

func TestCallToolReadsTextWhenThereIsNoStructuredContent(t *testing.T) {
	t.Parallel()
	client := answering(200, nil, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"image","text":"{}"},{"type":"text","text":"not json"},{"type":"text","text":"{\"id\":\"m1\"}"}]}}`)
	result, err := client.CallTool(context.Background(), "get_map", nil, "k")
	if err != nil || string(result.Structured) != `{"id":"m1"}` {
		t.Fatalf("CallTool = %s, %v", result.Structured, err)
	}
}

func TestFailuresOfTheExchange(t *testing.T) {
	t.Parallel()
	longPage := "<html>x" + strings.Repeat("é", 150) + "</html>" // cut inside a character
	cases := map[string]struct {
		client  *Client
		call    func(*Client) *Error
		code    string
		message string
		status  int
		known   bool
	}{
		"server error body": {
			client: answering(400, http.Header{"Retry-After": {"30"}}, `{"error":{"code":"PROTOCOL_VERSION_UNSUPPORTED","message":"This server speaks MCP"}}`),
			code:   "PROTOCOL_VERSION_UNSUPPORTED", message: "This server speaks MCP", status: 400, known: true,
		},
		"edge challenge": {
			client: answering(403, http.Header{"Cf-Mitigated": {"challenge"}}, "<html>Just a moment...</html>"),
			code:   "EDGE_CHALLENGE", message: "challenged this request", status: 403, known: true,
		},
		"html page": {
			client: answering(502, nil, longPage),
			code:   "HTTP_502", message: "Bad Gateway: <html>xé", status: 502, known: false,
		},
		"empty body": {
			client: answering(404, nil, "  "),
			code:   "HTTP_404", message: "Not Found", status: 404, known: true,
		},
		"not json-rpc": {
			client: answering(200, nil, `{"jsonrpc":"1.0"}`),
			code:   "INVALID_RESPONSE", message: "not a JSON-RPC reply", status: 200, known: false,
		},
		"not json": {
			client: answering(200, nil, `<html>`),
			code:   "INVALID_RESPONSE", message: "<html>", status: 200, known: false,
		},
		"json-rpc error": {
			client: answering(200, nil, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"Method not found"}}`),
			code:   "JSONRPC_-32601", message: "Method not found", status: 200, known: true,
		},
		"json-rpc internal error": {
			client: answering(200, nil, `{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"Internal error"}}`),
			code:   "JSONRPC_-32603", message: "Internal error", status: 200, known: false,
		},
		"another request's reply": {
			client: answering(200, nil, `{"jsonrpc":"2.0","id":2,"result":{}}`),
			code:   "INVALID_RESPONSE", message: "no result", status: 200, known: false,
		},
		"too large": {
			client: answering(200, nil, strings.Repeat(" ", MaxResponseBytes+1)),
			code:   "RESPONSE_TOO_LARGE", message: "8 MiB", status: 200, known: false,
		},
		"cut off": {
			client: &Client{Endpoint: "https://maps.example/mcp", HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(io.MultiReader(strings.NewReader("{"), failingReader{}))}, nil
			})}},
			code: "TRANSPORT", message: "cut off", status: 200, known: false,
		},
		"unreachable": {
			client: &Client{Endpoint: "https://maps.example/mcp", HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			})}},
			code: "TRANSPORT", message: "connection refused", known: false,
		},
		"bad endpoint": {
			client: &Client{Endpoint: "https://maps.example/\x7f", HTTP: http.DefaultClient},
			code:   "INVALID_URL", message: "invalid", known: true,
		},
		"unencodable params": {
			client: answering(200, nil, ""),
			call: func(c *Client) *Error {
				_, err := c.Call(context.Background(), "ping", map[string]any{"x": make(chan int)}, "")
				return err
			},
			code: "INVALID_ARGUMENTS", message: "unsupported type", known: true,
		},
		"malformed tool result": {
			client: answering(200, nil, `{"jsonrpc":"2.0","id":1,"result":[]}`),
			code:   "INVALID_RESPONSE", message: "malformed", status: 200, known: false,
		},
		"tool result without content": {
			client: answering(200, nil, `{"jsonrpc":"2.0","id":1,"result":{"structuredContent":null,"content":[{"type":"text","text":"done"}]}}`),
			code:   "INVALID_RESPONSE", message: "no structured content", status: 200, known: false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			call := tc.call
			if call == nil {
				call = func(c *Client) *Error {
					_, err := c.CallTool(context.Background(), "list_maps", map[string]any{}, "k")
					return err
				}
			}
			err := call(tc.client)
			if err == nil {
				t.Fatal("no error")
			}
			if err.Code != tc.code || !strings.Contains(err.Message, tc.message) || err.Status != tc.status || err.OutcomeKnown() != tc.known {
				t.Fatalf("error = %+v (known %v), want %s %q %d known %v", err, err.OutcomeKnown(), tc.code, tc.message, tc.status, tc.known)
			}
			if !strings.HasPrefix(err.Error(), tc.code+": ") {
				t.Fatalf("Error() = %q", err.Error())
			}
		})
	}
}

func TestRetryAfterIsKept(t *testing.T) {
	t.Parallel()
	_, err := answering(429, http.Header{"Retry-After": {"120"}}, "").Call(context.Background(), "ping", nil, "")
	if err == nil || err.Code != "HTTP_429" || err.RetryAfter != "120" {
		t.Fatalf("error = %+v", err)
	}
}

func TestTimeoutSaysTheOutcomeIsUnknown(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	client := &Client{Endpoint: server.URL + "/mcp", HTTP: server.Client()}
	_, err := client.Call(ctx, "ping", nil, "")
	if err == nil || err.Code != "TIMEOUT" || err.OutcomeKnown() {
		t.Fatalf("error = %+v", err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }
