package mcpclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/mcpclient"
)

// rpcRequest is the JSON-RPC 2.0 request shape the fake server decodes.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// fakeServer answers like the Wails MCP server: plain JSON responses over
// POST, 202 with no body for notifications.
type fakeServer struct {
	mu       sync.Mutex
	requests []rpcRequest
	headers  []http.Header
	handle   func(req rpcRequest) (status int, body any)
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.headers = append(f.headers, r.Header.Clone())
	f.mu.Unlock()

	if len(req.ID) == 0 || strings.HasPrefix(req.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	status, body := f.handle(req)
	if body == nil {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fakeServer) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.requests {
		out = append(out, r.Method)
	}
	return out
}

func result(id json.RawMessage, v any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": v}
}

func textResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}

// wailsLike answers initialize, tools/list and tools/call the way Wails v3.0.0 does.
func wailsLike(call func(name string, args map[string]any) map[string]any) func(rpcRequest) (int, any) {
	return func(req rpcRequest) (int, any) {
		switch req.Method {
		case "initialize":
			return http.StatusOK, result(req.ID, map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
				"serverInfo":      map[string]any{"name": "wails-mcp", "version": "1.0.0"},
			})
		case "tools/list":
			return http.StatusOK, result(req.ID, map[string]any{"tools": []map[string]any{
				{
					"name":        "dom_query",
					"description": "Find elements by CSS selector.",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"selector": map[string]any{"type": "string"}, "window": map[string]any{"type": "string"}},
						"required":   []string{"selector"},
					},
				},
				{
					"name":        "app_info",
					"description": "Get information about the running application.",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
				},
			}})
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			return http.StatusOK, result(req.ID, call(p.Name, p.Arguments))
		}
		return http.StatusOK, map[string]any{
			"jsonrpc": "2.0", "id": req.ID,
			"error": map[string]any{"code": -32601, "message": "method not found: " + req.Method},
		}
	}
}

func newServer(t *testing.T, handle func(rpcRequest) (int, any)) (*fakeServer, string) {
	t.Helper()
	f := &fakeServer{handle: handle}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv.URL + "/mcp"
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestInitializeSendsTheInitializeRequest(t *testing.T) {
	f, url := newServer(t, wailsLike(nil))
	c := mcpclient.New(url)
	if err := c.Initialize(ctx(t)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	methods := f.methods()
	if len(methods) == 0 || methods[0] != "initialize" {
		t.Fatalf("first request method = %v, want initialize first", methods)
	}
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(f.requests[0].Params, &params)
	if params.ProtocolVersion == "" {
		t.Errorf("initialize params carry no protocolVersion: %s", f.requests[0].Params)
	}
	if f.requests[0].JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", f.requests[0].JSONRPC)
	}
	if ct := f.headers[0].Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestListToolsReturnsNamesAndInputSchemas(t *testing.T) {
	_, url := newServer(t, wailsLike(nil))
	c := mcpclient.New(url)
	if err := c.Initialize(ctx(t)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	tools, err := c.ListTools(ctx(t))
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2: %+v", len(tools), tools)
	}
	if tools[0].Name != "dom_query" || tools[1].Name != "app_info" {
		t.Errorf("tool names = %q, %q; want dom_query, app_info", tools[0].Name, tools[1].Name)
	}
	if tools[0].Description == "" {
		t.Error("dom_query description is empty")
	}
	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(tools[0].InputSchema, &schema); err != nil {
		t.Fatalf("dom_query InputSchema is not JSON: %v (%s)", err, tools[0].InputSchema)
	}
	if _, ok := schema.Properties["window"]; !ok {
		t.Errorf("dom_query InputSchema lost the window property: %s", tools[0].InputSchema)
	}
}

func TestCallToolSendsNameAndArgumentsAndReturnsText(t *testing.T) {
	var gotName string
	var gotArgs map[string]any
	_, url := newServer(t, wailsLike(func(name string, args map[string]any) map[string]any {
		gotName, gotArgs = name, args
		return textResult(`{"count":1}`, false)
	}))
	c := mcpclient.New(url)
	if err := c.Initialize(ctx(t)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	res, err := c.CallTool(ctx(t), "dom_query", map[string]any{"selector": "#x", "window": "main"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if gotName != "dom_query" || gotArgs["selector"] != "#x" || gotArgs["window"] != "main" {
		t.Errorf("server saw name=%q args=%v", gotName, gotArgs)
	}
	if res.IsError {
		t.Error("IsError = true for a successful call")
	}
	if res.Text != `{"count":1}` {
		t.Errorf("Text = %q, want the tool's text content", res.Text)
	}
}

func TestCallToolJoinsMultipleTextItems(t *testing.T) {
	_, url := newServer(t, wailsLike(func(string, map[string]any) map[string]any {
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": "first"}, {"type": "text", "text": "second"}},
			"isError": false,
		}
	}))
	c := mcpclient.New(url)
	res, err := c.CallTool(ctx(t), "app_info", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !strings.Contains(res.Text, "first") || !strings.Contains(res.Text, "second") {
		t.Errorf("Text = %q, want both text items", res.Text)
	}
}

func TestCallToolReportsAToolErrorAsIsErrorWithoutAGoError(t *testing.T) {
	_, url := newServer(t, wailsLike(func(string, map[string]any) map[string]any {
		return textResult("no element matches selector: #missing", true)
	}))
	c := mcpclient.New(url)
	res, err := c.CallTool(ctx(t), "mouse_click", map[string]any{"selector": "#missing"})
	if err != nil {
		t.Fatalf("CallTool returned a Go error for a tool error: %v", err)
	}
	if !res.IsError {
		t.Error("IsError = false for a tool error")
	}
	if !strings.Contains(res.Text, "no element matches selector") {
		t.Errorf("Text = %q, want the tool's error message", res.Text)
	}
}

func TestHTTPErrorStatusIsAnError(t *testing.T) {
	_, url := newServer(t, func(rpcRequest) (int, any) {
		return http.StatusUnauthorized, nil
	})
	c := mcpclient.New(url)
	_, err := c.CallTool(ctx(t), "app_info", nil)
	if err == nil {
		t.Fatal("CallTool: want an error for HTTP 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q does not name the HTTP status", err)
	}
}

func TestJSONRPCErrorIsReturnedAsRPCError(t *testing.T) {
	_, url := newServer(t, func(req rpcRequest) (int, any) {
		return http.StatusOK, map[string]any{
			"jsonrpc": "2.0", "id": req.ID,
			"error": map[string]any{"code": -32602, "message": "unknown tool: nope"},
		}
	})
	c := mcpclient.New(url)
	_, err := c.CallTool(ctx(t), "nope", nil)
	if err == nil {
		t.Fatal("CallTool: want an error for a JSON-RPC error response")
	}
	var rpcErr *mcpclient.RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error %T %q is not a *mcpclient.RPCError", err, err)
	}
	if rpcErr.Code != -32602 || !strings.Contains(rpcErr.Message, "unknown tool") {
		t.Errorf("RPCError = %+v, want code -32602 and the server message", rpcErr)
	}
}

func TestMalformedResponseBodyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not json"))
	}))
	t.Cleanup(srv.Close)
	c := mcpclient.New(srv.URL + "/mcp")
	if _, err := c.CallTool(ctx(t), "app_info", nil); err == nil {
		t.Fatal("CallTool: want an error for a malformed body")
	}
}

func TestResponseWithoutResultOrErrorIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1}`))
	}))
	t.Cleanup(srv.Close)
	c := mcpclient.New(srv.URL + "/mcp")
	_, err := c.ListTools(ctx(t))
	if err == nil || !strings.Contains(err.Error(), "neither result nor error") {
		t.Fatalf("ListTools error = %v, want a missing-result error", err)
	}
}

func TestContextDeadlineStopsAHungCall(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})
	c := mcpclient.New(srv.URL + "/mcp")
	short, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.CallTool(short, "app_info", nil)
	if err == nil {
		t.Fatal("CallTool: want an error when the context deadline passes")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error %q does not wrap context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("CallTool took %v after a 200ms deadline", elapsed)
	}
}

func TestRequestIDsAreUniquePerRequest(t *testing.T) {
	f, url := newServer(t, wailsLike(func(string, map[string]any) map[string]any {
		return textResult("ok", false)
	}))
	c := mcpclient.New(url)
	if err := c.Initialize(ctx(t)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := c.CallTool(ctx(t), "app_info", nil); err != nil {
			t.Fatalf("CallTool: %v", err)
		}
	}
	seen := map[string]bool{}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if len(r.ID) == 0 {
			continue
		}
		if seen[string(r.ID)] {
			t.Errorf("request id %s reused", r.ID)
		}
		seen[string(r.ID)] = true
	}
}
