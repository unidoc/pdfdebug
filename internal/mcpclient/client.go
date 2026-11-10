// Package mcpclient is a minimal client for the Wails MCP server over the
// streamable HTTP transport, using plain JSON responses.
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

// protocolVersion is the MCP revision sent in initialize.
const protocolVersion = "2025-06-18"

// Client sends JSON-RPC requests to one MCP endpoint. It is safe for
// concurrent use.
type Client struct {
	endpoint string
	http     *http.Client
	nextID   atomic.Int64
}

// Tool is one entry of a tools/list response.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Result is the outcome of a tools/call. IsError is true when the tool
// itself reported a failure; Text joins every text content item.
type Result struct {
	Text    string
	IsError bool
}

// RPCError is a JSON-RPC error object returned by the server.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error formats the JSON-RPC error code and message.
func (e *RPCError) Error() string {
	return fmt.Sprintf("mcp: json-rpc error %d: %s", e.Code, e.Message)
}

// New returns a client for the MCP endpoint URL, for example
// http://127.0.0.1:9099/mcp.
func New(endpoint string) *Client {
	return &Client{endpoint: endpoint, http: &http.Client{}}
}

// Initialize performs the MCP initialize handshake and sends the
// initialized notification.
func (c *Client) Initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "pdfdebug-mcpclient", "version": "1.0.0"},
	}
	if _, err := c.request(ctx, "initialize", params); err != nil {
		return err
	}
	return c.notify(ctx, "notifications/initialized")
}

// ListTools returns the server's tools.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := c.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mcp: decode tools/list result: %w", err)
	}
	return out.Tools, nil
}

// CallTool calls the named tool with args. A tool-reported failure comes
// back as Result.IsError with a nil error.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (Result, error) {
	if args == nil {
		args = map[string]any{}
	}
	raw, err := c.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return Result{}, err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Result{}, fmt.Errorf("mcp: decode tools/call result: %w", err)
	}
	var texts []string
	for _, item := range out.Content {
		if item.Type == "text" {
			texts = append(texts, item.Text)
		}
	}
	return Result{Text: strings.Join(texts, "\n"), IsError: out.IsError}, nil
}

// request sends one JSON-RPC request and returns its raw result.
func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	body, err := c.post(ctx, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *RPCError       `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("mcp: decode %s response: %w", method, err)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return nil, fmt.Errorf("mcp: %s response has neither result nor error", method)
	}
	return resp.Result, nil
}

// notify sends a JSON-RPC notification, which has no id and no response body.
func (c *Client) notify(ctx context.Context, method string) error {
	_, err := c.post(ctx, map[string]any{"jsonrpc": "2.0", "method": method})
	return err
}

func (c *Client) post(ctx context.Context, msg map[string]any) ([]byte, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: %s: %w", msg["method"], err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mcp: read %s response: %w", msg["method"], err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("mcp: %s: HTTP %d: %s", msg["method"], resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
