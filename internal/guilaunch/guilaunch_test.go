package guilaunch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/mcpclient"
)

func TestParseMCPURLReadsTheUncolouredURLAttribute(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
		ok   bool
	}{
		{"tint line written to a pipe", `10:42AM INF MCP server started. Connect MCP clients using the streamable HTTP transport. url=http://127.0.0.1:54321/mcp`, "http://127.0.0.1:54321/mcp", true},
		{"url attribute followed by another attribute", `10:42AM INF MCP server started. Connect MCP clients using the streamable HTTP transport. url=http://127.0.0.1:9099/mcp extra=1`, "http://127.0.0.1:9099/mcp", true},
		{"unrelated log line", `10:42AM INF Build Info: Wails=v3.0.0`, "", false},
		{"url attribute on a line that is not the MCP start line", `10:42AM INF Asset request url=http://wails.localhost/index.html`, "", false},
		{"empty line", ``, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ParseMCPURL(c.line)
			if ok != c.ok || got != c.want {
				t.Errorf("ParseMCPURL(%q) = %q, %v; want %q, %v", c.line, got, ok, c.want, c.ok)
			}
		})
	}
}

func envValue(env []string, key string) (string, int) {
	val, n := "", 0
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			val = v
			n++
		}
	}
	return val, n
}

func TestEnvPointsXDGDirsUnderTheGivenRoot(t *testing.T) {
	root := t.TempDir()
	base := []string{
		"PATH=/usr/bin",
		"XDG_CONFIG_HOME=/home/owner/.config",
		"XDG_CACHE_HOME=relative/cache",
		"WAILS_MCP_HIDE_CURSOR=1",
		"WAILS_MCP_HOST=0.0.0.0",
		"WAILS_MCP_TOKEN=secret",
	}
	env := Env(base, root, false)
	for key, sub := range map[string]string{
		"XDG_CONFIG_HOME": "config",
		"XDG_CACHE_HOME":  "cache",
		"XDG_DATA_HOME":   "data",
	} {
		val, n := envValue(env, key)
		if n != 1 {
			t.Errorf("%s appears %d times, want once", key, n)
		}
		if want := filepath.Join(root, sub); val != want {
			t.Errorf("%s = %q, want %q", key, val, want)
		}
	}
	if v, _ := envValue(env, "PATH"); v != "/usr/bin" {
		t.Errorf("PATH = %q, want the parent's value kept", v)
	}
	if v, _ := envValue(env, "WAILS_MCP_PORT"); v != "0" {
		t.Errorf("WAILS_MCP_PORT = %q, want 0 (free port)", v)
	}
	for _, key := range []string{"WAILS_MCP_HIDE_CURSOR", "WAILS_MCP_HOST", "WAILS_MCP_TOKEN"} {
		if _, n := envValue(env, key); n != 0 {
			t.Errorf("%s is inherited; want it dropped", key)
		}
	}
	if v, n := envValue(Env(base, root, true), "WAILS_MCP_HIDE_CURSOR"); v != "1" || n != 1 {
		t.Errorf("WAILS_MCP_HIDE_CURSOR with hideCursor = %q (%d entries), want one entry with 1", v, n)
	}
}

func TestFindRepoRootMatchesTheModuleLine(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module unidoc-pdf-debugger\r\n\r\ngo 1.27.3"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "tests", "gui-mcp")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// A nested module with a longer name sharing the prefix is not the root.
	if err := os.WriteFile(filepath.Join(sub, "go.mod"), []byte("module unidoc-pdf-debugger-tests\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := FindRepoRoot(sub); err != nil || got != root {
		t.Errorf("FindRepoRoot = %q, %v; want %q", got, err, root)
	}
	if _, err := FindRepoRoot(t.TempDir()); err == nil {
		t.Error("FindRepoRoot outside the repo: want an error")
	}
}

func TestMainWindowRenderedQueriesTheAppShellOnTheMainWindow(t *testing.T) {
	var args map[string]any
	reply := `{"count":0}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		_ = json.Unmarshal(raw, &req)
		args = req.Params.Arguments
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
			"content": []map[string]any{{"type": "text", "text": reply}},
		}})
	}))
	defer srv.Close()
	c := mcpclient.New(srv.URL + "/mcp")
	if MainWindowRendered(context.Background(), c) {
		t.Error("MainWindowRendered with no match = true")
	}
	if args["selector"] != ReadySelector || args["window"] != MainWindow {
		t.Errorf("dom_query arguments = %v, want ReadySelector on the main window", args)
	}
	reply = `{"count":1}`
	if !MainWindowRendered(context.Background(), c) {
		t.Error("MainWindowRendered with a match = false")
	}
}
