package updatecheck

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testChecker(srv *httptest.Server) *Checker {
	return &Checker{BaseURL: srv.URL, Client: srv.Client()}
}

func releasesJSON(t *testing.T, releases []githubRelease) string {
	t.Helper()
	b, err := json.Marshal(releases)
	if err != nil {
		t.Fatalf("marshal releases: %v", err)
	}
	return string(b)
}

func TestCheckSkipsDevBuildWithoutNetwork(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UpdateAvailable {
		t.Fatal("dev build must not report an update")
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("dev build must make no HTTP call, got %d", got)
	}
}

func TestCheckReportsNewerRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept header = %q", got)
		}
		_, _ = fmt.Fprint(w, releasesJSON(t, []githubRelease{
			{TagName: "v1.3.0", Name: "1.3.0", Body: "notes", HTMLURL: "https://x/1.3.0", Assets: []Asset{
				{Name: "unidoc-pdf-debugger-1.3.0-linux-amd64.tar.gz", URL: "https://x/gui"},
				{Name: "SHA256SUMS.txt", URL: "https://x/sums"},
			}},
		}))
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "v1.2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.UpdateAvailable {
		t.Fatal("expected an update")
	}
	if res.LatestVersion != "v1.3.0" {
		t.Fatalf("LatestVersion = %q", res.LatestVersion)
	}
	if res.SumsURL != "https://x/sums" {
		t.Fatalf("SumsURL = %q", res.SumsURL)
	}
}

func TestCheckEqualVersionNoUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, releasesJSON(t, []githubRelease{{TagName: "v1.2.0"}}))
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "v1.2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UpdateAvailable {
		t.Fatal("equal version must not report an update")
	}
}

func TestCheckInstalledAheadNoUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, releasesJSON(t, []githubRelease{{TagName: "v1.1.0"}}))
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "v1.2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UpdateAvailable {
		t.Fatal("newer installed build must not report an update")
	}
}

func TestCheckExcludesPrereleaseAndDraft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, releasesJSON(t, []githubRelease{
			{TagName: "v2.0.0-rc1", Prerelease: true},
			{TagName: "v1.9.0", Draft: true},
			{TagName: "v1.8.0-rc.1"},
			{TagName: "v1.5.0"},
		}))
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "v1.2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.UpdateAvailable || res.LatestVersion != "v1.5.0" {
		t.Fatalf("expected v1.5.0, got available=%v latest=%q", res.UpdateAvailable, res.LatestVersion)
	}
	if len(res.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(res.Releases))
	}
}

func TestCheckMalformedTagIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, releasesJSON(t, []githubRelease{
			{TagName: "nightly"},
			{TagName: "v1.4.0"},
		}))
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "v1.2.0")
	if err != nil {
		t.Fatalf("malformed tag must not error the whole check: %v", err)
	}
	if res.LatestVersion != "v1.4.0" {
		t.Fatalf("LatestVersion = %q", res.LatestVersion)
	}
}

func TestCheckBodyVerbatim(t *testing.T) {
	body := "## Changes\n\n- fixed a `thing`\n- <script>alert(1)</script>\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, releasesJSON(t, []githubRelease{{TagName: "v1.3.0", Body: body}}))
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "v1.2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Releases[0].Body != body {
		t.Fatalf("body not verbatim:\n got %q\nwant %q", res.Releases[0].Body, body)
	}
}

func TestCheckTagWithoutVPrefixCompares(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, releasesJSON(t, []githubRelease{{TagName: "1.3.0"}}))
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "1.2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.UpdateAvailable {
		t.Fatal("unprefixed tag should still compare as newer")
	}
}

func TestCheckOrdersNewestFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, releasesJSON(t, []githubRelease{
			{TagName: "v1.3.0"},
			{TagName: "v1.5.0"},
			{TagName: "v1.4.0"},
		}))
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "v1.2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := []string{res.Releases[0].TagName, res.Releases[1].TagName, res.Releases[2].TagName}
	want := []string{"v1.5.0", "v1.4.0", "v1.3.0"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestCheckPaginationCapsAtFivePages(t *testing.T) {
	var pages atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := pages.Add(1)
		// Every page is full of newer entries and always links a next page, so
		// only the maxPages cap can stop the walk.
		var batch []githubRelease
		for i := range perPage {
			batch = append(batch, githubRelease{TagName: fmt.Sprintf("v9.%d.%d", n, i)})
		}
		w.Header().Set("Link", fmt.Sprintf(`<http://%s/next?page=%d>; rel="next"`, r.Host, n+1))
		_, _ = fmt.Fprint(w, releasesJSON(t, batch))
	}))
	defer srv.Close()

	_, err := testChecker(srv).Check(t.Context(), "v1.2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := pages.Load(); got != maxPages {
		t.Fatalf("fetched %d pages, want cap of %d", got, maxPages)
	}
}

func TestCheckHTTPErrorReturnsNoUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "v1.2.0")
	if err == nil {
		t.Fatal("expected an error on HTTP 500")
	}
	if res.UpdateAvailable {
		t.Fatal("HTTP 500 must not report an update")
	}
}

func TestCheckRateLimitedReturnsNoUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "v1.2.0")
	if err == nil {
		t.Fatal("expected an error on 403 rate-limit")
	}
	if res.UpdateAvailable {
		t.Fatal("rate-limited check must not report an update")
	}
}

func TestAssetForResolvesPerPlatform(t *testing.T) {
	assets := []Asset{
		{Name: "unidoc-pdf-debugger-1.3.0-darwin-arm64.dmg", URL: "u-dmg"},
		{Name: "unidoc-pdf-debugger-1.3.0-windows-amd64.zip", URL: "u-win"},
		{Name: "unidoc-pdf-debugger-1.3.0-linux-amd64.tar.gz", URL: "u-linux"},
		{Name: "pdfdebug-cli-1.3.0-windows-amd64.zip", URL: "cli-win"},
		{Name: "SHA256SUMS.txt", URL: "sums"},
	}
	cases := []struct {
		goos, goarch string
		wantName     string
		wantURL      string
	}{
		{"darwin", "arm64", "unidoc-pdf-debugger-1.3.0-darwin-arm64.dmg", "u-dmg"},
		{"windows", "amd64", "unidoc-pdf-debugger-1.3.0-windows-amd64.zip", "u-win"},
		{"linux", "amd64", "unidoc-pdf-debugger-1.3.0-linux-amd64.tar.gz", "u-linux"},
		{"darwin", "amd64", "", ""},
		{"windows", "arm64", "", ""},
	}
	for _, c := range cases {
		name, url := assetFor(c.goos, c.goarch, assets)
		if name != c.wantName || url != c.wantURL {
			t.Errorf("assetFor(%s,%s) = (%q,%q), want (%q,%q)", c.goos, c.goarch, name, url, c.wantName, c.wantURL)
		}
	}
}

func TestAssetForRejectsCLIArchiveOnSuffixCollision(t *testing.T) {
	// The GUI zip is absent; only the CLI zip shares the windows-amd64.zip suffix.
	assets := []Asset{{Name: "pdfdebug-cli-1.3.0-windows-amd64.zip", URL: "cli-win"}}
	name, url := assetFor("windows", "amd64", assets)
	if name != "" || url != "" {
		t.Fatalf("CLI archive must not match a GUI update, got (%q,%q)", name, url)
	}
}

func writeSums(t *testing.T, entries map[string]string) string {
	t.Helper()
	var b strings.Builder
	for name, sum := range entries {
		_, _ = fmt.Fprintf(&b, "%s  %s\n", sum, name)
	}
	return b.String()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestDownloadAndVerifyMovesOnMatch(t *testing.T) {
	payload := []byte("the release binary bytes")
	name := "unidoc-pdf-debugger-1.3.0-linux-amd64.tar.gz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sums") {
			_, _ = fmt.Fprint(w, writeSums(t, map[string]string{name: sha256Hex(payload)}))
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := t.TempDir()
	saved, err := testChecker(srv).DownloadAndVerify(t.Context(), srv.URL+"/asset", name, srv.URL+"/sums", dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved != filepath.Join(dest, name) {
		t.Fatalf("saved path = %q", saved)
	}
	got, err := os.ReadFile(saved)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("saved file mismatch: %v", err)
	}
}

func TestDownloadAndVerifyDeletesOnMismatch(t *testing.T) {
	payload := []byte("the real bytes")
	name := "unidoc-pdf-debugger-1.3.0-linux-amd64.tar.gz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sums") {
			_, _ = fmt.Fprint(w, writeSums(t, map[string]string{name: sha256Hex([]byte("different"))}))
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := t.TempDir()
	_, err := testChecker(srv).DownloadAndVerify(t.Context(), srv.URL+"/asset", name, srv.URL+"/sums", dest)
	if !errors.Is(err, ErrChecksumVerify) {
		t.Fatalf("expected ErrChecksumVerify, got %v", err)
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Fatalf("dest must be empty after a mismatch, has %d entries", len(entries))
	}
}

func TestDownloadAndVerifyMissingSumsURL(t *testing.T) {
	dest := t.TempDir()
	_, err := New().DownloadAndVerify(t.Context(), "https://x/asset", "a.tar.gz", "", dest)
	if !errors.Is(err, ErrChecksumMissing) {
		t.Fatalf("expected ErrChecksumMissing, got %v", err)
	}
}

func TestDownloadAndVerifyFilenameNotInSums(t *testing.T) {
	name := "unidoc-pdf-debugger-1.3.0-linux-amd64.tar.gz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sums") {
			_, _ = fmt.Fprint(w, writeSums(t, map[string]string{"some-other-file.zip": sha256Hex([]byte("x"))}))
			return
		}
		_, _ = w.Write([]byte("bytes"))
	}))
	defer srv.Close()

	dest := t.TempDir()
	_, err := testChecker(srv).DownloadAndVerify(t.Context(), srv.URL+"/asset", name, srv.URL+"/sums", dest)
	if !errors.Is(err, ErrChecksumMissing) {
		t.Fatalf("expected ErrChecksumMissing, got %v", err)
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Fatalf("dest must be empty, has %d entries", len(entries))
	}
}

func TestDownloadAndVerifySumsFetchFails(t *testing.T) {
	name := "unidoc-pdf-debugger-1.3.0-linux-amd64.tar.gz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sums") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("bytes"))
	}))
	defer srv.Close()

	dest := t.TempDir()
	_, err := testChecker(srv).DownloadAndVerify(t.Context(), srv.URL+"/asset", name, srv.URL+"/sums", dest)
	if !errors.Is(err, ErrChecksumMissing) {
		t.Fatalf("expected ErrChecksumMissing, got %v", err)
	}
}

func TestDownloadAndVerifyReportsProgressPhases(t *testing.T) {
	payload := []byte(strings.Repeat("x", 1500*1024)) // spans the 512KB emit throttle
	name := "unidoc-pdf-debugger-1.4.0-linux-amd64.tar.gz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sums") {
			_, _ = fmt.Fprint(w, writeSums(t, map[string]string{name: sha256Hex(payload)}))
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	var phases []string
	var maxReceived, total int64
	c := &Checker{
		BaseURL: srv.URL,
		Client:  srv.Client(),
		OnProgress: func(p Progress) {
			if len(phases) == 0 || phases[len(phases)-1] != p.Phase {
				phases = append(phases, p.Phase)
			}
			if p.Received > maxReceived {
				maxReceived = p.Received
			}
			if p.Total > total {
				total = p.Total
			}
		},
	}
	dest := t.TempDir()
	if _, err := c.DownloadAndVerify(t.Context(), srv.URL+"/asset", name, srv.URL+"/sums", dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{PhaseDownloading, PhaseVerifying, PhaseSaving}
	if len(phases) != len(want) {
		t.Fatalf("phases = %v, want %v", phases, want)
	}
	for i := range want {
		if phases[i] != want[i] {
			t.Fatalf("phase[%d] = %q, want %q", i, phases[i], want[i])
		}
	}
	if maxReceived != int64(len(payload)) {
		t.Fatalf("final received = %d, want %d", maxReceived, len(payload))
	}
	if total != int64(len(payload)) {
		t.Fatalf("total = %d, want %d (Content-Length)", total, len(payload))
	}
}

func TestDownloadUsesDownloadClientNotCheckClient(t *testing.T) {
	payload := []byte("release payload that outlives the 5s check bound")
	name := "unidoc-pdf-debugger-1.3.0-linux-amd64.tar.gz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sums") {
			_, _ = fmt.Fprint(w, writeSums(t, map[string]string{name: sha256Hex(payload)}))
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	// The check client is set to an unusable timeout; the download must not use it.
	c := &Checker{BaseURL: srv.URL, Client: &http.Client{Timeout: time.Nanosecond}, DownloadClient: srv.Client()}
	dest := t.TempDir()
	saved, err := c.DownloadAndVerify(t.Context(), srv.URL+"/asset", name, srv.URL+"/sums", dest)
	if err != nil {
		t.Fatalf("download must use the download client, not the 5s check client: %v", err)
	}
	if got, err := os.ReadFile(saved); err != nil || string(got) != string(payload) {
		t.Fatalf("saved payload mismatch: %v", err)
	}
}

func TestCheckWalksPastAnOlderOnlyPage(t *testing.T) {
	// GitHub orders by creation time, not SemVer: page one is only older releases
	// (a recent hotfix on an old branch), page two carries the newer release.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "page2") {
			_, _ = fmt.Fprint(w, releasesJSON(t, []githubRelease{{TagName: "v1.5.0"}}))
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<http://%s/releases?page2=1>; rel="next"`, r.Host))
		_, _ = fmt.Fprint(w, releasesJSON(t, []githubRelease{{TagName: "v1.1.0"}, {TagName: "v1.0.5"}}))
	}))
	defer srv.Close()

	res, err := testChecker(srv).Check(t.Context(), "v1.2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.UpdateAvailable || res.LatestVersion != "v1.5.0" {
		t.Fatalf("expected v1.5.0 found on page two, got available=%v latest=%q", res.UpdateAvailable, res.LatestVersion)
	}
}

func TestAvailableNameAvoidsCollisions(t *testing.T) {
	dir := t.TempDir()
	mustName := func(name string) string {
		got, err := availableName(dir, name)
		if err != nil {
			t.Fatalf("availableName(%q): %v", name, err)
		}
		return got
	}
	// Free name is returned unchanged.
	if got := mustName("unidoc-pdf-debugger-1.4.0-darwin-arm64.dmg"); got != "unidoc-pdf-debugger-1.4.0-darwin-arm64.dmg" {
		t.Fatalf("free name changed: %q", got)
	}
	// Occupied name gets a bracketed counter before the extension.
	touch := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	touch("app.dmg")
	if got := mustName("app.dmg"); got != "app (1).dmg" {
		t.Fatalf("first collision = %q, want app (1).dmg", got)
	}
	touch("app (1).dmg")
	if got := mustName("app.dmg"); got != "app (2).dmg" {
		t.Fatalf("second collision = %q, want app (2).dmg", got)
	}
	// .tar.gz stays intact rather than splitting at the last dot.
	touch("pkg-1.4.0-linux-amd64.tar.gz")
	if got := mustName("pkg-1.4.0-linux-amd64.tar.gz"); got != "pkg-1.4.0-linux-amd64 (1).tar.gz" {
		t.Fatalf("tar.gz collision = %q", got)
	}
}

func TestDownloadAndVerifySanitizesAssetName(t *testing.T) {
	payload := []byte("payload")
	// The sums manifest keys off the base name, matching what a real release ships.
	base := "evil.dmg"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sums") {
			_, _ = fmt.Fprint(w, writeSums(t, map[string]string{base: sha256Hex(payload)}))
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := t.TempDir()
	saved, err := testChecker(srv).DownloadAndVerify(t.Context(), srv.URL+"/asset", "../../../"+base, srv.URL+"/sums", dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved != filepath.Join(dest, base) {
		t.Fatalf("traversal not neutralized: saved = %q, want inside %q", saved, dest)
	}
}

func TestDownloadAndVerifyDoesNotOverwriteExisting(t *testing.T) {
	payload := []byte("the new verified bytes")
	name := "unidoc-pdf-debugger-1.4.0-darwin-arm64.dmg"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sums") {
			_, _ = fmt.Fprint(w, writeSums(t, map[string]string{name: sha256Hex(payload)}))
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := t.TempDir()
	existing := filepath.Join(dest, name)
	if err := os.WriteFile(existing, []byte("previous download"), 0o644); err != nil {
		t.Fatal(err)
	}

	saved, err := testChecker(srv).DownloadAndVerify(t.Context(), srv.URL+"/asset", name, srv.URL+"/sums", dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved != filepath.Join(dest, "unidoc-pdf-debugger-1.4.0-darwin-arm64 (1).dmg") {
		t.Fatalf("expected a non-colliding name, got %q", saved)
	}
	if got, _ := os.ReadFile(existing); string(got) != "previous download" {
		t.Fatalf("the existing file must be left untouched, got %q", got)
	}
	if got, _ := os.ReadFile(saved); string(got) != string(payload) {
		t.Fatalf("saved file content mismatch: %q", got)
	}
}

func TestMoveFileCopyFallback(t *testing.T) {
	// Force rename to fail so the copy+remove fallback is exercised on every
	// platform (a same-volume temp dir would otherwise let rename succeed).
	orig := renameFunc
	renameFunc = func(string, string) error { return errors.New("simulated cross-device rename") }
	t.Cleanup(func() { renameFunc = orig })

	src := filepath.Join(t.TempDir(), "src.bin")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "dst.bin")
	if err := moveFile(src, dst); err != nil {
		t.Fatalf("moveFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "payload" {
		t.Fatalf("dst content mismatch: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("src should be gone after move")
	}
}
