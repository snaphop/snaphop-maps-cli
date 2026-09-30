// Package mcp is a client for SnapHop Maps' MCP server: MCP's Streamable HTTP transport, spoken
// statelessly with JSON responses, as the server speaks it. Every call is one JSON-RPC request in
// one POST, answered with one JSON body; there is no session and no server-sent events.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

// ProtocolVersion is the MCP revision the client speaks, the server's own default.
const ProtocolVersion = "2025-06-18"

// MaxResponseBytes bounds how much of an answer is read.
const MaxResponseBytes = 8 << 20

// Client calls one MCP endpoint.
type Client struct {
	// Endpoint is the server's MCP address, such as https://maps.snaphop.ai/mcp.
	Endpoint string
	// HTTP sends the requests. It should follow no redirects.
	HTTP *http.Client
	// UserAgent names the client to the server.
	UserAgent string
}

// ToolResult is a tool's answer: the structured content, and whether the tool refused.
type ToolResult struct {
	Structured json.RawMessage
	IsError    bool
}

// Error is a failure of the exchange itself, as opposed to a tool's refusal. Unless OutcomeKnown
// says otherwise, the request may or may not have been carried out.
type Error struct {
	// Code names the failure: TRANSPORT, TIMEOUT, HTTP_<status>, a JSON-RPC error, or the code the
	// server put in an HTTP error body.
	Code    string `json:"code"`
	Message string `json:"message"`
	// Status is the HTTP status, when an answer arrived.
	Status int `json:"status,omitempty"`
	// RetryAfter is the answer's Retry-After header, when it had one.
	RetryAfter string `json:"retryAfter,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// OutcomeKnown is true when the request was not carried out: it was never sent, or the server
// refused it with an HTTP error below 500 or a JSON-RPC error. It is false when it may have been
// carried out unseen: no answer, an answer from a gateway or a failing server, a JSON-RPC internal
// error, or a successful answer this client could not read.
func (e *Error) OutcomeKnown() bool {
	switch {
	case e.Code == "TIMEOUT" || e.Code == "TRANSPORT":
		return false
	case e.Status == http.StatusOK:
		return strings.HasPrefix(e.Code, "JSONRPC_") && e.Code != "JSONRPC_-32603"
	default:
		return e.Status < 500
	}
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type reply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Call sends one JSON-RPC request and returns its result. apiKey, when not empty, is sent as an
// Authorization bearer header.
func (c *Client) Call(ctx context.Context, method string, params any, apiKey string) (json.RawMessage, *Error) {
	body, err := json.Marshal(request{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return nil, &Error{Code: "INVALID_ARGUMENTS", Message: err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, &Error{Code: "INVALID_URL", Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	req.Header.Set("User-Agent", c.UserAgent)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &Error{Code: "TIMEOUT", Message: "No answer arrived in time; the request may still have been carried out."}
		}
		return nil, &Error{Code: "TRANSPORT", Message: err.Error()}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, &Error{Code: "TRANSPORT", Message: "The answer was cut off: " + err.Error(), Status: resp.StatusCode}
	}
	if len(data) > MaxResponseBytes {
		return nil, &Error{Code: "RESPONSE_TOO_LARGE", Message: "The answer exceeded 8 MiB.", Status: resp.StatusCode}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, httpError(resp, data)
	}
	var answer reply
	if err := json.Unmarshal(data, &answer); err != nil || answer.JSONRPC != "2.0" {
		return nil, &Error{Code: "INVALID_RESPONSE", Message: "The server's answer is not a JSON-RPC reply: " + excerpt(data), Status: resp.StatusCode}
	}
	if answer.Error != nil {
		return nil, &Error{Code: fmt.Sprintf("JSONRPC_%d", answer.Error.Code), Message: answer.Error.Message, Status: resp.StatusCode}
	}
	if string(answer.ID) != "1" || len(answer.Result) == 0 {
		return nil, &Error{Code: "INVALID_RESPONSE", Message: "The server's reply has no result for this request.", Status: resp.StatusCode}
	}
	return answer.Result, nil
}

// CallTool calls one tool with its arguments.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any, apiKey string) (ToolResult, *Error) {
	result, failure := c.Call(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments}, apiKey)
	if failure != nil {
		return ToolResult{}, failure
	}
	var decoded struct {
		Structured json.RawMessage `json:"structuredContent"`
		IsError    bool            `json:"isError"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil {
		return ToolResult{}, &Error{Code: "INVALID_RESPONSE", Message: "The tool's result is malformed: " + err.Error(), Status: http.StatusOK}
	}
	structured := decoded.Structured
	if len(structured) == 0 || string(structured) == "null" {
		// A server that sends only text: its text is the structured content, when it is JSON.
		for _, part := range decoded.Content {
			if part.Type == "text" && json.Valid([]byte(part.Text)) {
				structured = json.RawMessage(part.Text)
				break
			}
		}
	}
	if len(structured) == 0 || string(structured) == "null" {
		return ToolResult{}, &Error{Code: "INVALID_RESPONSE", Message: "The tool's result carries no structured content.", Status: http.StatusOK}
	}
	return ToolResult{Structured: structured, IsError: decoded.IsError}, nil
}

// httpError describes an answer other than 200: the server's own error body when it sent one,
// and otherwise the status. An edge's bot challenge is named, since an agent can do nothing about
// it but report it.
func httpError(resp *http.Response, data []byte) *Error {
	failure := &Error{
		Code:       fmt.Sprintf("HTTP_%d", resp.StatusCode),
		Message:    http.StatusText(resp.StatusCode),
		Status:     resp.StatusCode,
		RetryAfter: resp.Header.Get("Retry-After"),
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	switch {
	case strings.EqualFold(resp.Header.Get("cf-mitigated"), "challenge"):
		failure.Code = "EDGE_CHALLENGE"
		failure.Message = "The edge in front of the service challenged this request as automated traffic, so it never reached the service."
	case json.Unmarshal(data, &body) == nil && body.Error.Code != "":
		failure.Code = body.Error.Code
		failure.Message = body.Error.Message
	case len(bytes.TrimSpace(data)) > 0:
		failure.Message += ": " + excerpt(data)
	}
	return failure
}

// excerpt is the start of a body, on one line, for an error message.
func excerpt(data []byte) string {
	text := strings.Join(strings.Fields(string(data)), " ")
	if len(text) > 200 {
		text = text[:200]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
		text += "..."
	}
	return text
}
