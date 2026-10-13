package update_check_test

import "testing"

// harnessSrc drives the update-check public API end to end from inside the main
// module: a stubbed GitHub releases list, platform-asset resolution, and a
// checksum-verified download into a temp directory. It exercises the exact call
// sequence the update service uses.
const harnessSrc = `package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"unidoc-pdf-debugger/internal/updatecheck"
)

func TestCheckThenVerifiedDownload(t *testing.T) {
	payload := []byte("release-binary-payload")
	sum := sha256.Sum256(payload)
	sumHex := hex.EncodeToString(sum[:])

	guiNames := []string{
		"unidoc-pdf-debugger-1.5.0-darwin-arm64.dmg",
		"unidoc-pdf-debugger-1.5.0-windows-amd64.zip",
		"unidoc-pdf-debugger-1.5.0-linux-amd64.tar.gz",
	}

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/unidoc/pdfdebug/releases":
			var assets []map[string]any
			for _, n := range guiNames {
				assets = append(assets, map[string]any{"name": n, "browser_download_url": srv.URL + "/dl/" + n})
			}
			assets = append(assets,
				map[string]any{"name": "pdfdebug-cli-1.5.0-windows-amd64.zip", "browser_download_url": srv.URL + "/dl/cli"},
				map[string]any{"name": "SHA256SUMS.txt", "browser_download_url": srv.URL + "/dl/SHA256SUMS.txt"},
			)
			rel := []map[string]any{{
				"tag_name": "v1.5.0", "name": "1.5.0", "body": "notes", "html_url": "https://x/1.5.0",
				"published_at": "2026-09-01T00:00:00Z", "prerelease": false, "draft": false, "assets": assets,
			}}
			_ = json.NewEncoder(w).Encode(rel)
		case "/dl/SHA256SUMS.txt":
			for _, n := range guiNames {
				fmt.Fprintf(w, "%s  %s\n", sumHex, n)
			}
		default:
			_, _ = w.Write(payload)
		}
	}))
	defer srv.Close()

	checker := &updatecheck.Checker{BaseURL: srv.URL, Client: srv.Client()}
	res, err := checker.Check(context.Background(), "v1.0.0")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !res.UpdateAvailable {
		t.Fatal("expected an update to be available")
	}
	if res.DownloadURL == "" || res.DownloadName == "" {
		t.Fatalf("no platform asset resolved: %+v", res)
	}
	if res.SumsURL == "" {
		t.Fatal("no checksum manifest resolved")
	}

	dest := t.TempDir()
	saved, err := checker.DownloadAndVerify(context.Background(), res.DownloadURL, res.DownloadName, res.SumsURL, dest)
	if err != nil {
		t.Fatalf("download and verify: %v", err)
	}
	got, err := os.ReadFile(saved)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("saved payload mismatch: %v", err)
	}
	if filepath.Dir(saved) != dest {
		t.Fatalf("verified file saved outside the destination: %s", saved)
	}
}
`

// TestUpdateCheckDiscoveryAndVerifiedDownload runs the harness inside the main
// module and asserts the full check-then-verified-download flow passes.
func TestUpdateCheckDiscoveryAndVerifiedDownload(t *testing.T) {
	out, err := runHarness(t, map[string]string{"harness_test.go": harnessSrc})
	if err != nil {
		t.Fatalf("update-check acceptance harness failed: %v\n%s", err, out)
	}
}
