// Package updatecheck discovers newer GitHub releases of the app and performs a
// checksum-verified assisted download of the correct asset for the running
// platform. It follows the same isolation playbook as internal/splash and
// internal/pendingopen: pure Go with zero Wails dependency, so discovery,
// SemVer filtering, platform-asset resolution, and download verification are all
// unit-testable without a runtime. The Wails-coupled bits (reveal-in-file
// manager, open-in-browser) live in the service adapter, not here.
//
// The GitHub base URL and HTTP client are injectable on the Checker so tests can
// point at an httptest.Server; nothing here ever hard-codes a live network call.
package updatecheck

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	// defaultBaseURL is the GitHub REST API host. Overridable on Checker.BaseURL.
	defaultBaseURL = "https://api.github.com"
	// releasesPath is the releases-list endpoint for the distribution repo.
	releasesPath = "/repos/unidoc/pdfdebug/releases"
	// perPage is the releases page size requested from GitHub.
	perPage = 30
	// maxPages bounds pagination so a pathological "many versions behind" client
	// cannot walk the entire release history.
	maxPages = 5
	// requestTimeout bounds each HTTP request.
	requestTimeout = 5 * time.Second
	// devVersion is the untagged local-build sentinel; the check is skipped for it.
	devVersion = "dev"
	// sumsName is the checksum manifest asset published with each release.
	sumsName = "SHA256SUMS.txt"
	// guiAssetPrefix is the GUI archive name prefix. The CLI archives share the
	// platform suffix (e.g. -windows-amd64.zip), so matching on the suffix alone
	// would pick the wrong binary; the prefix disambiguates.
	guiAssetPrefix = "unidoc-pdf-debugger-"
	// cliAssetPrefix marks the CLI archives, which are never a GUI update target.
	cliAssetPrefix = "pdfdebug-cli-"
)

// ErrChecksumVerify is returned when a downloaded asset's SHA256 does not match
// the published checksum. The download is deleted and nothing is moved.
//
// The frontend classifies the failure by matching the substring "checksum
// verification" in the rejected-promise message (UpdateNotifier.tsx
// CHECKSUM_VERIFY_MARKER); keep that phrase if this message is reworded.
var ErrChecksumVerify = errors.New("update download failed checksum verification")

// ErrChecksumMissing is returned when the release publishes no SHA256SUMS.txt or
// the asset filename is absent from it, so the download cannot be verified.
//
// The frontend keys the release-page fallback off the substring "checksum is
// unavailable" (UpdateNotifier.tsx CHECKSUM_MISSING_MARKER).
var ErrChecksumMissing = errors.New("update download checksum is unavailable")

// Asset is one release asset from the GitHub API.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// ReleaseEntry is one release surfaced to the frontend, newest first. Body is
// the markdown release notes passed through verbatim (sanitized only at render).
type ReleaseEntry struct {
	TagName     string `json:"tagName"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"htmlUrl"`
	PublishedAt string `json:"publishedAt"`
}

// Result is the cumulative check outcome returned to the frontend.
type Result struct {
	UpdateAvailable  bool           `json:"updateAvailable"`
	InstalledVersion string         `json:"installedVersion"`
	LatestVersion    string         `json:"latestVersion"`
	Releases         []ReleaseEntry `json:"releases"`
	DownloadURL      string         `json:"downloadUrl"`
	DownloadName     string         `json:"downloadName"`
	SumsURL          string         `json:"sumsUrl"`
	// LatestStable is the highest stable tag seen on the pages Check walked,
	// newer than installedVersion or not, in canonical form; "" when none was
	// seen. It feeds the app's cache record and is not sent to the frontend.
	LatestStable string `json:"-"`
}

// githubRelease is the subset of the GitHub release JSON the check consumes.
type githubRelease struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	HTMLURL     string  `json:"html_url"`
	PublishedAt string  `json:"published_at"`
	Prerelease  bool    `json:"prerelease"`
	Draft       bool    `json:"draft"`
	Assets      []Asset `json:"assets"`
}

// Progress reports the state of a verified download. Phase is one of
// PhaseDownloading, PhaseVerifying, PhaseSaving. During PhaseDownloading,
// Received and Total (bytes) drive a progress bar; Total is 0 when the server
// sends no Content-Length. The later phases carry no byte counts.
type Progress struct {
	Phase    string
	Received int64
	Total    int64
}

// Download phase identifiers reported through Checker.OnProgress.
const (
	PhaseDownloading = "downloading"
	PhaseVerifying   = "verifying"
	PhaseSaving      = "saving"
)

// progressEmitBytes throttles byte-level download progress so a large asset does
// not fire thousands of callbacks; a callback fires at most once per this many
// received bytes, plus one final callback at completion.
const progressEmitBytes = 512 * 1024

// Checker performs release discovery and verified downloads. The zero value is
// not usable; construct with New and override BaseURL/Client in tests.
//
// Client governs the release-list check and carries the 5s bound. DownloadClient
// governs asset and checksum downloads and deliberately has NO total timeout -
// a release binary can take far longer than 5s to fetch; cancellation is handled
// by the caller's context instead. OnProgress, when set, receives download phase
// and byte updates (nil is a no-op).
type Checker struct {
	BaseURL        string
	Client         *http.Client
	DownloadClient *http.Client
	OnProgress     func(Progress)
	// WaitIfPaused, when set, is called before each read of the asset body. It
	// blocks while the download is paused and returns ctx.Err() if the context is
	// cancelled during the pause (nil = never pauses).
	WaitIfPaused func(ctx context.Context) error
}

// emit reports progress when a callback is registered.
func (c *Checker) emit(p Progress) {
	if c.OnProgress != nil {
		c.OnProgress(p)
	}
}

// New returns a Checker pointed at the live GitHub API. The check client is
// bounded at 5s; the download client has no total timeout (context-cancellable).
func New() *Checker {
	return &Checker{
		BaseURL:        defaultBaseURL,
		Client:         &http.Client{Timeout: requestTimeout},
		DownloadClient: &http.Client{},
	}
}

// downloadHTTPClient returns the client for asset/checksum downloads, defaulting
// to a no-timeout client so a large asset is not aborted by the check's 5s bound.
func (c *Checker) downloadHTTPClient() *http.Client {
	if c.DownloadClient != nil {
		return c.DownloadClient
	}
	return &http.Client{}
}

// Check lists releases, keeps those strictly newer than installedVersion by
// SemVer (excluding drafts, releases flagged prerelease, and tags with a SemVer
// prerelease suffix), and returns them newest-first with
// the resolved download asset for the running platform from the newest release.
//
// It skips the network entirely when installedVersion is the "dev" sentinel. Any
// transport, status, rate-limit, or parse failure resolves to
// Result{UpdateAvailable:false} plus a returned error for logging - never a
// blocking state for the caller.
func (c *Checker) Check(ctx context.Context, installedVersion string) (Result, error) {
	result := Result{InstalledVersion: installedVersion}
	if installedVersion == devVersion {
		return result, nil
	}

	installed := normalizeVersion(installedVersion)
	if !semver.IsValid(installed) {
		return result, fmt.Errorf("installed version %q is not valid SemVer", installedVersion)
	}

	releases, latestStable, err := c.collectNewer(ctx, installed)
	if err != nil {
		return result, err
	}
	result.LatestStable = latestStable
	if len(releases) == 0 {
		return result, nil
	}

	sort.SliceStable(releases, func(i, j int) bool {
		return semver.Compare(normalizeVersion(releases[i].TagName), normalizeVersion(releases[j].TagName)) > 0
	})

	newest := releases[0]
	result.UpdateAvailable = true
	result.LatestVersion = newest.TagName
	result.Releases = make([]ReleaseEntry, len(releases))
	for i, r := range releases {
		result.Releases[i] = ReleaseEntry{
			TagName:     r.TagName,
			Name:        r.Name,
			Body:        r.Body,
			HTMLURL:     r.HTMLURL,
			PublishedAt: r.PublishedAt,
		}
	}
	result.DownloadName, result.DownloadURL = assetFor(goos(), goarch(), newest.Assets)
	result.SumsURL = sumsURLFor(newest.Assets)
	return result, nil
}

// collectNewer walks the paginated releases list, keeping entries strictly newer
// than installed. It walks until there is no next page or maxPages is reached; it
// does NOT stop at a page of only-older entries, because GitHub orders releases
// by creation time, not SemVer, so a newer release can sit behind an
// out-of-order older hotfix (the same reason results are sorted by SemVer, not
// published_at). The page cap bounds a pathological "many versions behind" walk.
// It also returns the highest stable tag seen on any walked page.
func (c *Checker) collectNewer(ctx context.Context, installed string) ([]githubRelease, string, error) {
	url := fmt.Sprintf("%s%s?per_page=%d", strings.TrimRight(c.BaseURL, "/"), releasesPath, perPage)
	var kept []githubRelease
	latestStable := ""
	for page := 0; page < maxPages && url != ""; page++ {
		batch, next, err := c.fetchPage(ctx, url)
		if err != nil {
			return nil, "", err
		}
		latestStable = highestStable(latestStable, batch)
		for _, r := range batch {
			if r.Prerelease || r.Draft {
				continue
			}
			tag := normalizeVersion(r.TagName)
			// A SemVer prerelease suffix excludes a release even when GitHub's
			// prerelease flag was not set, matching highestStable, so the
			// cached and live answers agree.
			if !semver.IsValid(tag) || semver.Prerelease(tag) != "" {
				continue
			}
			if semver.Compare(tag, installed) > 0 {
				kept = append(kept, r)
			}
		}
		url = next
	}
	return kept, latestStable, nil
}

// fetchPage requests one releases page and returns the decoded entries plus the
// rel="next" URL from the Link header (empty when there is no next page).
func (c *Checker) fetchPage(ctx context.Context, url string) ([]githubRelease, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("github releases request returned status %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, "", fmt.Errorf("decode releases: %w", err)
	}
	return releases, nextPageURL(resp.Header.Get("Link")), nil
}

var linkNextRe = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="next"`)

// nextPageURL extracts the rel="next" URL from a GitHub Link header.
func nextPageURL(link string) string {
	if link == "" {
		return ""
	}
	if m := linkNextRe.FindStringSubmatch(link); m != nil {
		return m[1]
	}
	return ""
}

// normalizeVersion ensures a leading "v" so a tag with or without the prefix
// compares under golang.org/x/mod/semver.
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// assetFor resolves the GUI download asset for a platform. It requires both the
// GUI prefix and the platform suffix and rejects the CLI archives (which share
// the suffix). Returns empty strings when no GUI asset matches the platform.
func assetFor(goosVal, goarchVal string, assets []Asset) (name, url string) {
	suffix, ok := platformSuffix(goosVal, goarchVal)
	if !ok {
		return "", ""
	}
	for _, a := range assets {
		if strings.HasPrefix(a.Name, cliAssetPrefix) {
			continue
		}
		if strings.HasPrefix(a.Name, guiAssetPrefix) && strings.HasSuffix(a.Name, suffix) {
			return a.Name, a.URL
		}
	}
	return "", ""
}

// platformSuffix maps a GOOS/GOARCH to the GUI asset suffix for the three built
// platforms. The second return is false for any other platform.
func platformSuffix(goosVal, goarchVal string) (string, bool) {
	switch {
	case goosVal == "darwin" && goarchVal == "arm64":
		return "-darwin-arm64.dmg", true
	case goosVal == "windows" && goarchVal == "amd64":
		return "-windows-amd64.zip", true
	case goosVal == "linux" && goarchVal == "amd64":
		return "-linux-amd64.tar.gz", true
	default:
		return "", false
	}
}

// sumsURLFor returns the SHA256SUMS.txt asset URL, or empty when absent.
func sumsURLFor(assets []Asset) string {
	for _, a := range assets {
		if a.Name == sumsName {
			return a.URL
		}
	}
	return ""
}

// DownloadAndVerify downloads assetURL to a temp file, verifies its SHA256
// against the published sum in sumsURL, and on match only moves it into destDir
// and returns the saved path. On mismatch it returns ErrChecksumVerify; on a
// missing sums file or absent asset line it returns ErrChecksumMissing. The
// unverified file and the sums manifest never reach destDir.
func (c *Checker) DownloadAndVerify(ctx context.Context, assetURL, assetName, sumsURL, destDir string) (string, error) {
	if sumsURL == "" {
		return "", ErrChecksumMissing
	}
	// assetName is bound to the frontend; reduce it to a bare filename so it can
	// never traverse out of destDir (defence in depth - the value is server-derived).
	assetName = filepath.Base(assetName)

	tmp, err := os.CreateTemp("", "pdfdebug-update-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	moved := false
	defer func() {
		_ = tmp.Close()
		if !moved {
			_ = os.Remove(tmpPath)
		}
	}()

	sum := sha256.New()
	if err := c.downloadTo(ctx, assetURL, io.MultiWriter(tmp, sum)); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	c.emit(Progress{Phase: PhaseVerifying})
	expected, err := c.expectedSum(ctx, sumsURL, assetName)
	if err != nil {
		return "", err
	}
	actual := hex.EncodeToString(sum.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		return "", ErrChecksumVerify
	}

	c.emit(Progress{Phase: PhaseSaving})
	name, err := availableName(destDir, assetName)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, name)
	if !strings.HasPrefix(dest, filepath.Clean(destDir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("resolved download path escapes the destination directory")
	}
	if err := moveFile(tmpPath, dest); err != nil {
		return "", err
	}
	moved = true
	return dest, nil
}

// maxNameCollisions bounds the "base (n).ext" search so a pathological directory
// cannot spin the goroutine forever.
const maxNameCollisions = 1000

// availableName returns name if destDir/name is free, otherwise the first
// "base (n).ext" variant that does not yet exist (browser convention). The known
// compound extension .tar.gz is kept intact ("x (1).tar.gz", not "x.tar (1).gz").
// A stat error other than "not exist" (permission denied, I/O error, dead network
// mount) is returned rather than looped on, so a locked-down Downloads dir fails
// fast instead of hanging.
func availableName(destDir, name string) (string, error) {
	free := func(candidate string) (bool, error) {
		_, err := os.Stat(filepath.Join(destDir, candidate))
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err // err == nil means it exists; a non-nil err is fatal
	}
	if ok, err := free(name); err != nil {
		return "", err
	} else if ok {
		return name, nil
	}
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	if strings.HasSuffix(base, ".tar") {
		ext = ".tar" + ext
		base = base[:len(base)-len(".tar")]
	}
	for n := 1; n <= maxNameCollisions; n++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, n, ext)
		if ok, err := free(candidate); err != nil {
			return "", err
		} else if ok {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no available filename for %q after %d attempts", name, maxNameCollisions)
}

// downloadTo streams a GET response body into w.
func (c *Checker) downloadTo(ctx context.Context, url string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.downloadHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s returned status %d", url, resp.StatusCode)
	}

	total := resp.ContentLength
	var received, lastEmit int64
	c.emit(Progress{Phase: PhaseDownloading, Received: 0, Total: total})
	buf := make([]byte, 32*1024)
	for {
		// Honor a pause request before each read. While paused we stop reading the
		// body, so TCP backpressure halts the transfer until resumed or cancelled.
		if c.WaitIfPaused != nil {
			if err := c.WaitIfPaused(ctx); err != nil {
				return err
			}
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			received += int64(n)
			if received-lastEmit >= progressEmitBytes {
				lastEmit = received
				c.emit(Progress{Phase: PhaseDownloading, Received: received, Total: total})
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	c.emit(Progress{Phase: PhaseDownloading, Received: received, Total: total})
	return nil
}

// expectedSum fetches SHA256SUMS.txt into memory and returns the lowercased hex
// digest for assetName. A non-200 or an absent asset line is ErrChecksumMissing.
func (c *Checker) expectedSum(ctx context.Context, sumsURL, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumsURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.downloadHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", ErrChecksumMissing
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", ErrChecksumMissing
}

// renameFunc is os.Rename, overridable in tests to exercise the copy fallback.
var renameFunc = os.Rename

// moveFile renames src to dst, falling back to copy+remove when the rename fails
// for any reason (the cross-filesystem EXDEV error is not portably classifiable -
// Windows returns ERROR_NOT_SAME_DEVICE, not EXDEV - so any failure retries via
// copy and only a failed copy surfaces an error). dst is expected to be a fresh
// path (see availableName), so no existing file is overwritten.
func moveFile(src, dst string) error {
	if err := renameFunc(src, dst); err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	_ = in.Close()
	_ = os.Remove(src)
	return nil
}
