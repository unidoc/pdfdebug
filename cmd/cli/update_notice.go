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

	"unidoc-pdf-debugger/internal/updatecheck"
)

const (
	// refreshTimeout bounds a CLI refresh end to end: one releases page.
	refreshTimeout = 1500 * time.Millisecond
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
	if dir, err := updatecheck.DefaultDir(); err == nil {
		cache, _ = updatecheck.Open(dir, updatecheck.SurfaceCLI)
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
	if _, ok := updatecheck.CheckableVersion(e.version); !ok {
		return false
	}
	if !e.interactive() || e.optedOut() {
		return false
	}
	return !machineFormat(args) && e.cache != nil
}

// interactive reports whether a person is at a terminal: stdout and stderr
// are both terminals and CI is unset.
func (e *noticeEnv) interactive() bool {
	if _, ok := e.lookupEnv("CI"); ok {
		return false
	}
	return e.stdoutTTY && e.stderrTTY
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
// tag and records the answer. If the attempt cannot be written the request is
// never made. It returns the record as it stands afterwards. A panic is
// recovered and reported as an error.
func refreshSnapshot(ctx context.Context, cache *updatecheck.Cache, latest func(context.Context) (string, error), now time.Time) (snap updatecheck.Snapshot, err error) {
	defer func() {
		if recover() != nil {
			err = errRefreshPanicked
		}
	}()
	snap, err = cache.RecordAttempt(now)
	if err != nil {
		return snap, err
	}
	tag, err := latest(ctx)
	if err != nil {
		return snap, err
	}
	snap, _ = cache.RecordSuccess(now, tag)
	return snap, nil
}

// pendingNotice is the end-of-run notice for one ordinary command, with its
// refresh (if any) running alongside the command.
type pendingNotice struct {
	env    *noticeEnv
	active bool
	// snap is the record the notice reads: whichever of the CLI's own record
	// and the desktop app's names the higher latest version.
	snap updatecheck.Snapshot
	peer updatecheck.Snapshot
	done chan updatecheck.Snapshot
	// answered is closed once the server call returns, answer or not.
	answered chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
}

// startNotice evaluates the guards and, when the record is stale or absent,
// starts a bounded refresh in the background. args excludes the program name.
func startNotice(env *noticeEnv, args []string) *pendingNotice {
	p := &pendingNotice{env: env}
	if !env.eligible(args) {
		return p
	}
	p.active = true
	own, _ := env.cache.Load()
	p.peer, _ = env.cache.LoadPeer()
	p.snap = updatecheck.Newer(own, p.peer)
	// A desktop-app check that succeeded within the TTL walked every release
	// page, so it spares the CLI its own refresh.
	now := env.now()
	if own.Fresh(now, updatecheck.CacheTTL) || p.peer.Confirmed(now, updatecheck.CacheTTL) {
		return p
	}
	p.ctx, p.cancel = context.WithTimeout(context.Background(), refreshTimeout)
	p.done = make(chan updatecheck.Snapshot, 1)
	p.answered = make(chan struct{})
	latest := func(ctx context.Context) (string, error) {
		defer close(p.answered)
		return env.latest(ctx)
	}
	go func() {
		result, _ := refreshSnapshot(p.ctx, env.cache, latest, now)
		p.done <- updatecheck.Newer(result, p.peer)
	}()
	return p
}

// finish waits for the refresh, bounded by its deadline, then prints the
// notice when the record names a newer version. It never panics and never
// affects the exit code.
func (p *pendingNotice) finish() {
	defer func() { _ = recover() }()
	if !p.active {
		return
	}
	if p.done != nil {
		// Past the deadline, a refresh whose server call already returned is
		// only writing the record, which takes milliseconds; waiting for it
		// keeps the answer and leaves no temp file behind at exit.
		select {
		case s := <-p.done:
			p.snap = s
		case <-p.ctx.Done():
			select {
			case <-p.answered:
				p.snap = <-p.done
			default:
			}
		}
		p.cancel()
	}
	latest, ok := p.snap.Notice(p.env.version)
	if !ok {
		return
	}
	writeNotice(p.env.stderr, p.env.version, latest, p.env.width, true)
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
// side, or as plain lines when the frame would reach a known width: a row
// that fills the last column wraps early on terminals without deferred wrap.
func noticeBox(lines []string, width int) string {
	longest := 0
	for _, l := range lines {
		longest = max(longest, len(l))
	}
	inner := longest + 4
	var b strings.Builder
	if width > 0 && inner+2 >= width {
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

// runVersion handles --version and -v. Stdout carries only the version line.
// Outside an interactive session (a pipe, a redirect or CI) that is all it
// does: no request, no stderr, no cache write. In a terminal a checkable, not
// opted-out build runs a live bounded refresh, then on stderr shows the box,
// says the build is current, or says the check failed; a failed check still
// shows the box when the desktop app's record confirms a newer release.
func runVersion(env *noticeEnv) int {
	line := fmt.Sprintf("pdfdebug version %s\n", env.version)
	if !env.interactive() {
		_, _ = io.WriteString(env.stdout, line)
		return 0
	}
	_, checkable := updatecheck.CheckableVersion(env.version)
	switch {
	case !checkable:
		_, _ = io.WriteString(env.stdout, line)
		_, _ = fmt.Fprintln(env.stderr, "pdfdebug: update checks are skipped for development builds")
		return 0
	case env.optedOut():
		_, _ = io.WriteString(env.stdout, line)
		_, _ = fmt.Fprintln(env.stderr, "pdfdebug: update check disabled (PDFDEBUG_NO_UPDATE_CHECK or NO_UPDATE_NOTIFIER is set)")
		return 0
	}

	checked := false
	var snap, peer updatecheck.Snapshot
	if env.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
		s, err := refreshSnapshot(ctx, env.cache, env.latest, env.now())
		cancel()
		peer, _ = env.cache.LoadPeer()
		snap, checked = updatecheck.Newer(s, peer), err == nil
	}
	if !checked && peer.Confirmed(env.now(), updatecheck.CacheTTL) {
		snap, checked = peer, true
	}
	if !checked {
		_, _ = io.WriteString(env.stdout, line)
		_, _ = fmt.Fprintln(env.stderr, "pdfdebug: could not check for updates")
		return 0
	}
	if latest, ok := snap.Notice(env.version); ok {
		writeNotice(env.stderr, env.version, latest, env.width, false)
		_, _ = io.WriteString(env.stdout, line)
		return 0
	}
	_, _ = io.WriteString(env.stdout, line)
	_, _ = fmt.Fprintln(env.stderr, "pdfdebug: no newer release is available")
	return 0
}
