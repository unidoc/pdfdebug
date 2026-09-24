package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"unidoc-pdf-debugger/internal/updatecheck"
)

const (
	// refreshTimeout bounds a CLI refresh end to end: one releases page.
	refreshTimeout = 1500 * time.Millisecond
	// rearmAfter is the cooldown before a notice already shown for a version
	// is shown one final time.
	rearmAfter = 7 * 24 * time.Hour
	// releasesURL is where the notice sends the user.
	releasesURL = "https://github.com/unidoc/pdfdebug/releases"
)

// optOutVars suppress the notice and every refresh when present with any value.
var optOutVars = []string{"PDFDEBUG_NO_UPDATE_CHECK", "NO_UPDATE_NOTIFIER"}

// noticeEnv carries every input the update notice depends on, so tests can
// drive each branch without a real terminal, clock, cache or network.
type noticeEnv struct {
	version   string
	lookupEnv func(string) (string, bool)
	stdoutTTY bool
	stderrTTY bool
	// width is the stderr terminal width in columns; 0 means unknown.
	width  int
	now    func() time.Time
	stdout io.Writer
	stderr io.Writer
	// cache is nil when the cache path could not be resolved.
	cache  *updatecheck.Cache
	latest func(context.Context) (string, error)
}

// newNoticeEnv builds the production environment. It resolves the cache path
// and queries the terminals but reads and writes no file.
func newNoticeEnv() *noticeEnv {
	outTTY, _ := terminalInfo(os.Stdout)
	errTTY, width := terminalInfo(os.Stderr)
	var cache *updatecheck.Cache
	if path, err := updatecheck.DefaultPath(); err == nil {
		cache, _ = updatecheck.Open(path)
	}
	return &noticeEnv{
		version:   version,
		lookupEnv: os.LookupEnv,
		stdoutTTY: outTTY,
		stderrTTY: errTTY,
		width:     width,
		now:       time.Now,
		stdout:    os.Stdout,
		stderr:    os.Stderr,
		cache:     cache,
		latest:    updatecheck.New().LatestStable,
	}
}

// checkableVersion reports whether v can be compared against a release: not
// the dev sentinel and valid SemVer with or without a leading "v".
func checkableVersion(v string) bool {
	if v == "dev" {
		return false
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return semver.IsValid(v)
}

// optedOut reports whether either opt-out variable is present.
func (e *noticeEnv) optedOut() bool {
	for _, k := range optOutVars {
		if _, ok := e.lookupEnv(k); ok {
			return true
		}
	}
	return false
}

// eligible reports whether an ordinary command may refresh the cache and show
// the notice. args excludes the program name.
func (e *noticeEnv) eligible(args []string) bool {
	if !checkableVersion(e.version) {
		return false
	}
	if _, ok := e.lookupEnv("CI"); ok {
		return false
	}
	if e.optedOut() || !e.stdoutTTY || !e.stderrTTY {
		return false
	}
	return !machineFormat(args) && e.cache != nil
}

// machineFormat reports whether args select an output a program reads rather
// than a person: --json, --ops or --raw set true, dump bytes and dump
// plaintext, and dump embedded extraction by --ref or --name. A false positive
// only costs a notice, so an unparseable flag value counts as set. Scanning
// stops at a bare "--".
func machineFormat(args []string) bool {
	dump := len(args) >= 2 && args[0] == "dump"
	if dump && (args[1] == "bytes" || args[1] == "plaintext") {
		return true
	}
	embedded := dump && args[1] == "embedded"
	for _, a := range args {
		if a == "--" {
			return false
		}
		name, ok := strings.CutPrefix(a, "-")
		if !ok {
			continue
		}
		name = strings.TrimPrefix(name, "-")
		name, value, hasValue := strings.Cut(name, "=")
		switch name {
		case "json", "ops", "raw":
			if !hasValue {
				return true
			}
			if b, err := strconv.ParseBool(value); err != nil || b {
				return true
			}
		case "ref", "name":
			if embedded {
				return true
			}
		}
	}
	return false
}

// errRefreshPanicked reports a refresh that panicked and was recovered.
var errRefreshPanicked = errors.New("update refresh panicked")

// refreshSnapshot records an attempt, then asks latest for the newest stable
// tag and records that. The attempt record goes first and carries the previous
// latest version, so a failed or slow request still advances checked_at; if
// the attempt cannot be written the request is never made. A success that
// finds no tag keeps the previous latest version. It returns the record as it
// stands afterwards. A panic is recovered and reported as an error.
func refreshSnapshot(ctx context.Context, cache *updatecheck.Cache, latest func(context.Context) (string, error), now time.Time) (snap updatecheck.Snapshot, err error) {
	prev, _ := cache.Load()
	defer func() {
		if recover() != nil {
			snap, err = prev, errRefreshPanicked
		}
	}()
	if err := cache.Store(updatecheck.Snapshot{CheckedAt: now, LatestVersion: prev.LatestVersion}); err != nil {
		return prev, err
	}
	attempt := updatecheck.Snapshot{Schema: 1, CheckedAt: now, LatestVersion: prev.LatestVersion}
	tag, err := latest(ctx)
	if err != nil {
		return attempt, err
	}
	if tag == "" {
		return attempt, nil
	}
	_ = cache.Store(updatecheck.Snapshot{CheckedAt: now, LatestVersion: tag})
	return updatecheck.Snapshot{Schema: 1, CheckedAt: now, LatestVersion: tag}, nil
}

// pendingNotice is the end-of-run notice for one ordinary command, with its
// refresh (if any) running alongside the command.
type pendingNotice struct {
	env    *noticeEnv
	active bool
	snap   updatecheck.Snapshot
	done   chan updatecheck.Snapshot
	ctx    context.Context
	cancel context.CancelFunc
}

// startNotice evaluates the guards and, when the record is stale or absent,
// starts a bounded refresh in the background. args excludes the program name.
func startNotice(env *noticeEnv, args []string) *pendingNotice {
	p := &pendingNotice{env: env}
	if !env.eligible(args) {
		return p
	}
	p.active = true
	p.snap, _ = env.cache.Load()
	if p.snap.Fresh(env.now(), updatecheck.CacheTTL) {
		return p
	}
	p.ctx, p.cancel = context.WithTimeout(context.Background(), refreshTimeout)
	p.done = make(chan updatecheck.Snapshot, 1)
	go func() {
		result := p.snap
		defer func() {
			_ = recover()
			p.done <- result
		}()
		result, _ = refreshSnapshot(p.ctx, env.cache, env.latest, env.now())
	}()
	return p
}

// finish waits for the refresh, bounded by its deadline, then prints the
// notice if it is due. It never panics and never affects the exit code.
func (p *pendingNotice) finish() {
	defer func() { _ = recover() }()
	if !p.active {
		return
	}
	if p.done != nil {
		// A refresh that finished before the deadline wins even when the
		// command outlasted the deadline and both channels are ready.
		select {
		case s := <-p.done:
			p.snap = s
		default:
			select {
			case s := <-p.done:
				p.snap = s
			case <-p.ctx.Done():
			}
		}
		p.cancel()
	}
	latest, ok := p.snap.Notice(p.env.version)
	if !ok || !p.markShown(latest) {
		return
	}
	writeNotice(p.env.stderr, p.env.version, latest, p.env.width, true)
}

// markShown decides whether the notice for latest is due and, if so, records
// it as shown. It reports true only when the record was written, so an
// unwritable cache directory never prints.
func (p *pendingNotice) markShown(latest string) bool {
	now := p.env.now()
	prev, ok := p.env.cache.LoadShown()
	next, due := nextShown(prev, ok, latest, now)
	if !due {
		return false
	}
	return p.env.cache.StoreShown(next) == nil
}

// nextShown applies the once-per-version rule with one re-arm a week after
// the first showing. A first_shown_at in the future counts as just shown.
func nextShown(prev updatecheck.Shown, ok bool, latest string, now time.Time) (updatecheck.Shown, bool) {
	target := "v" + strings.TrimPrefix(latest, "v")
	if !ok || prev.Version != target {
		return updatecheck.Shown{Version: target, FirstShownAt: now}, true
	}
	if prev.Rearmed || prev.FirstShownAt.After(now) || now.Sub(prev.FirstShownAt) < rearmAfter {
		return prev, false
	}
	prev.Rearmed = true
	return prev, true
}

// writeNotice prints the update notice to w: a framed box sized to its longest
// line, or the same two lines unframed when the box is wider than a known
// terminal width. leadingBlank separates it from the command's own output.
func writeNotice(w io.Writer, installed, latest string, width int, leadingBlank bool) {
	lines := []string{
		fmt.Sprintf("Update available: %s -> %s", strings.TrimPrefix(installed, "v"), strings.TrimPrefix(latest, "v")),
		releasesURL,
	}
	var b strings.Builder
	if leadingBlank {
		b.WriteString("\n")
	}
	b.WriteString(noticeBox(lines, width))
	_, _ = io.WriteString(w, b.String())
}

// noticeBox renders lines inside a +-| frame with two spaces of padding each
// side, or as plain lines when the frame would exceed a known width.
func noticeBox(lines []string, width int) string {
	longest := 0
	for _, l := range lines {
		longest = max(longest, len(l))
	}
	inner := longest + 4
	var b strings.Builder
	if width > 0 && inner+2 > width {
		for _, l := range lines {
			b.WriteString(l + "\n")
		}
		return b.String()
	}
	border := "+" + strings.Repeat("-", inner) + "+\n"
	b.WriteString(border)
	for _, l := range lines {
		b.WriteString("|  " + l + strings.Repeat(" ", longest-len(l)) + "  |\n")
	}
	b.WriteString(border)
	return b.String()
}

// runVersion handles --version and -v. Stdout carries only the version line;
// the outcome goes to stderr. A checkable, not opted-out build runs a live
// bounded refresh first and then shows the box, says the build is current, or
// says the check failed. The terminal and CI guards do not apply here.
func runVersion(env *noticeEnv) int {
	line := fmt.Sprintf("pdfdebug version %s\n", env.version)
	switch {
	case !checkableVersion(env.version):
		_, _ = io.WriteString(env.stdout, line)
		_, _ = fmt.Fprintln(env.stderr, "pdfdebug: update checks are skipped for development builds")
		return 0
	case env.optedOut():
		_, _ = io.WriteString(env.stdout, line)
		_, _ = fmt.Fprintln(env.stderr, "pdfdebug: update check disabled (PDFDEBUG_NO_UPDATE_CHECK or NO_UPDATE_NOTIFIER is set)")
		return 0
	}

	checked := false
	var snap updatecheck.Snapshot
	if env.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
		s, err := refreshSnapshot(ctx, env.cache, env.latest, env.now())
		cancel()
		snap, checked = s, err == nil
	}
	if !checked {
		_, _ = io.WriteString(env.stdout, line)
		_, _ = fmt.Fprintln(env.stderr, "pdfdebug: could not check for updates")
		return 0
	}
	if latest, ok := snap.Notice(env.version); ok {
		// The box counts as shown under the same rule as the end-of-run notice:
		// a new version is recorded, a due re-arm is spent, and an earlier
		// showing of the same version keeps its date.
		prev, ok := env.cache.LoadShown()
		if next, due := nextShown(prev, ok, latest, env.now()); due {
			_ = env.cache.StoreShown(next)
		}
		writeNotice(env.stderr, env.version, latest, env.width, false)
		_, _ = io.WriteString(env.stdout, line)
		return 0
	}
	_, _ = io.WriteString(env.stdout, line)
	_, _ = fmt.Fprintln(env.stderr, "pdfdebug: no newer release is available")
	return 0
}
