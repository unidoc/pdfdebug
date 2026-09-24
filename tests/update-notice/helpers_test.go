// Package update_notice_test is the acceptance suite for the shared update
// cache and the CLI update notice.
//
// Two mechanisms, both network-free:
//
//   - In-module harnesses. The cache, the bounded refresh and the GUI service
//     live in internal packages, which a standalone module cannot import. The
//     harness sources under testdata/ (ignored by the go tool) are copied into a
//     throwaway dot-prefixed directory inside the main module and run with
//     `go test` as a subprocess. The subprocess passing is the assertion.
//   - The built pdfdebug binary, run as a subprocess with an explicit child
//     environment: XDG_CACHE_HOME points at a temp dir, CI and both opt-out
//     variables are removed unless a case sets them, and HTTP(S)_PROXY points at
//     a local tripwire that refuses and counts every request. Stdout and stderr
//     are pipes, so every binary run exercises the non-interactive path.
//
// The test process environment is never mutated.
//
// Run: cd tests/update-notice && go test -count=1 ./...
package update_notice_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// projectRoot walks up from cwd to the main module go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(data), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root")
		}
		dir = parent
	}
}

// strippedKeys are removed from every child environment so the host (a CI
// runner, a developer shell) cannot decide the outcome.
var strippedKeys = []string{
	"CI",
	"PDFDEBUG_NO_UPDATE_CHECK",
	"NO_UPDATE_NOTIFIER",
	"XDG_CACHE_HOME",
	"HTTPS_PROXY", "https_proxy",
	"HTTP_PROXY", "http_proxy",
	"ALL_PROXY", "all_proxy",
	"NO_PROXY", "no_proxy",
}

// childEnv returns the current environment minus strippedKeys, plus extra.
func childEnv(extra map[string]string) []string {
	drop := map[string]bool{}
	for _, k := range strippedKeys {
		drop[k] = true
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if drop[k] {
			continue
		}
		if _, overridden := extra[k]; overridden {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// tripwire is a local HTTP proxy that refuses every request and counts them. A
// child pointed at it cannot reach the real network through the default
// transport, and any attempt is visible to the test.
type tripwire struct {
	srv *httptest.Server
	n   atomic.Int64
}

func newTripwire(t *testing.T) *tripwire {
	t.Helper()
	tw := &tripwire{}
	tw.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tw.n.Add(1)
		http.Error(w, "tripwire: outbound request refused", http.StatusForbidden)
	}))
	t.Cleanup(tw.srv.Close)
	return tw
}

func (tw *tripwire) env() map[string]string {
	return map[string]string{"HTTPS_PROXY": tw.srv.URL, "HTTP_PROXY": tw.srv.URL}
}

func (tw *tripwire) count() int64 { return tw.n.Load() }

// merge returns the union of maps; later maps win.
func merge(ms ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range ms {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// runHarness copies testdata/<name>/*.go into a fresh dot-prefixed dir inside
// the main module and runs `go test -count=1 -run <run> .` there with the given
// extra environment. GOPROXY=off keeps the go command itself off the network.
func runHarness(t *testing.T, name string, extra map[string]string, run string) (string, error) {
	t.Helper()
	root := projectRoot(t)

	srcDir := filepath.Join("testdata", name)
	files, err := filepath.Glob(filepath.Join(srcDir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no harness sources in %s: %v", srcDir, err)
	}

	dir, err := os.MkdirTemp(root, ".acc-update-notice-")
	if err != nil {
		t.Fatalf("mkdir harness: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(f)), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	args := []string{"test", "-count=1"}
	if run != "" {
		args = append(args, "-run", run)
	}
	args = append(args, ".")
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = childEnv(merge(map[string]string{"GOPROXY": "off", "GOTOOLCHAIN": "local"}, extra))
	raw, runErr := cmd.CombinedOutput()
	if runErr == nil && strings.Contains(string(raw), "no tests to run") {
		t.Fatalf("harness %s matched no test for -run %q:\n%s", name, run, raw)
	}
	return string(raw), runErr
}

var (
	buildMu   sync.Mutex
	buildBins = map[string]string{}
)

// buildCLI compiles the CLI once per version. An empty version builds without
// -ldflags, which leaves the "dev" sentinel in place.
func buildCLI(t *testing.T, version string) string {
	t.Helper()
	buildMu.Lock()
	defer buildMu.Unlock()
	if bin, ok := buildBins[version]; ok {
		return bin
	}
	root := projectRoot(t)
	name := "pdfdebug"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dir, err := os.MkdirTemp("", "pdfdebug-update-notice-")
	if err != nil {
		t.Fatalf("mkdir build dir: %v", err)
	}
	bin := filepath.Join(dir, name)
	args := []string{"build", "-o", bin}
	if version != "" {
		args = append(args, "-ldflags", "-X main.version="+version)
	}
	args = append(args, "./cmd/cli/")
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build CLI (version %q): %v\n%s", version, err, out)
	}
	buildBins[version] = bin
	return bin
}

// runCLI runs bin with args under childEnv(extra) and returns stdout, stderr and
// the exit code. Both streams are pipes.
func runCLI(t *testing.T, bin string, extra map[string]string, args ...string) (stdout, stderr []byte, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = childEnv(extra)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return out.Bytes(), errb.Bytes(), code
}

// testdataPDF returns the absolute path of a fixture in the main module's testdata.
func testdataPDF(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata", name)
}

// snapshotPath is where the CLI resolves its own record under an XDG cache base.
func snapshotPath(xdg string) string {
	return filepath.Join(xdg, "pdfdebug", "updatecheck-cli.json")
}

// seedSnapshot writes a schema-1 record of a successful check at checkedAt under
// xdg and returns its exact bytes.
func seedSnapshot(t *testing.T, xdg string, checkedAt time.Time, latest string) []byte {
	t.Helper()
	path := snapshotPath(xdg)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	data, err := json.Marshal(map[string]any{
		"schema":         1,
		"checked_at":     checkedAt.UTC().Format(time.RFC3339Nano),
		"succeeded_at":   checkedAt.UTC().Format(time.RFC3339Nano),
		"latest_version": latest,
	})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	return data
}

// assertDirEmpty fails when dir holds any entry: a suppressed or dev run must
// not create the cache directory at all.
func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) > 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected %s to stay empty, found %v", dir, names)
	}
}

// assertNoNotice fails when stderr carries any part of the update notice.
func assertNoNotice(t *testing.T, label string, stderr []byte) {
	t.Helper()
	s := string(stderr)
	for _, marker := range []string{
		"Update available",
		"github.com/unidoc/pdfdebug/releases",
		"+---",
		"could not check for updates",
	} {
		if strings.Contains(s, marker) {
			t.Errorf("%s: stderr carries update-notice text %q:\n%s", label, marker, s)
		}
	}
}
