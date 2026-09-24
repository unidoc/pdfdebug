package update_notice_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// releasedVersion is the build version for binaries that are allowed to check.
// Every seeded record advertises newerVersion, so a notice is pending for them.
const (
	releasedVersion = "0.4.0"
	newerVersion    = "v0.5.0"
)

type cliCase struct {
	name string
	args []string
	code int
}

func pipedCases(t *testing.T) []cliCase {
	minimal := testdataPDF(t, "minimal.pdf")
	return []cliCase{
		{"dump tree", []string{"dump", "tree", minimal}, 0},
		{"dump tree json", []string{"dump", "tree", "--json", minimal}, 0},
		{"dump bytes", []string{"dump", "bytes", minimal}, 0},
		{"validate clean", []string{"validate", "--profile", "pdfua-1-structural", testdataPDF(t, "tagged.pdf")}, 0},
		{"diff identical", []string{"diff", minimal, minimal}, 0},
		{"help", []string{"--help"}, 0},
		{"unknown command", []string{"no-such-command"}, 1},
	}
}

// With both streams piped, a pending update changes nothing: stdout, stderr and
// the exit code match the opted-out run byte for byte, the record is not
// rewritten, no shown-state file appears, and nothing leaves the machine.
func TestPipedRunsWithUpdatePendingMatchTheOptedOutRun(t *testing.T) {
	bin := buildCLI(t, releasedVersion)
	for _, c := range pipedCases(t) {
		t.Run(c.name, func(t *testing.T) {
			tw := newTripwire(t)

			xdg := t.TempDir()
			seeded := seedSnapshot(t, xdg, time.Now().Add(-time.Hour), newerVersion)
			out, errOut, code := runCLI(t, bin, merge(tw.env(), map[string]string{"XDG_CACHE_HOME": xdg}), c.args...)

			optXDG := t.TempDir()
			seedSnapshot(t, optXDG, time.Now().Add(-time.Hour), newerVersion)
			optOut, optErr, optCode := runCLI(t, bin, merge(tw.env(), map[string]string{
				"XDG_CACHE_HOME":           optXDG,
				"PDFDEBUG_NO_UPDATE_CHECK": "1",
			}), c.args...)

			if code != c.code || optCode != c.code {
				t.Errorf("exit code = %d (opted out %d), want %d", code, optCode, c.code)
			}
			if !bytes.Equal(out, optOut) {
				t.Errorf("stdout differs from the opted-out run\nwith notice pending:\n%s\nopted out:\n%s", out, optOut)
			}
			if !bytes.Equal(errOut, optErr) {
				t.Errorf("stderr differs from the opted-out run\nwith notice pending:\n%s\nopted out:\n%s", errOut, optErr)
			}
			assertNoNotice(t, c.name, errOut)
			assertNoNotice(t, c.name+" stdout", out)

			after, err := os.ReadFile(snapshotPath(xdg))
			if err != nil {
				t.Fatalf("read record: %v", err)
			}
			if !bytes.Equal(after, seeded) {
				t.Errorf("a piped run rewrote the record\nbefore %s\nafter  %s", seeded, after)
			}
			if _, err := os.Stat(shownStatePath(xdg)); !os.IsNotExist(err) {
				t.Errorf("a piped run created the shown-state file (stat err = %v)", err)
			}
			if n := tw.count(); n != 0 {
				t.Errorf("piped run made %d outbound requests, want 0", n)
			}
		})
	}
}

// Machine-format stdout stays parseable with a notice pending.
func TestJSONOutputStaysParseableWithUpdatePending(t *testing.T) {
	bin := buildCLI(t, releasedVersion)
	tw := newTripwire(t)
	xdg := t.TempDir()
	seedSnapshot(t, xdg, time.Now().Add(-time.Hour), newerVersion)
	out, errOut, code := runCLI(t, bin, merge(tw.env(), map[string]string{"XDG_CACHE_HOME": xdg}), "dump", "tree", "--json", testdataPDF(t, "minimal.pdf"))
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	var v any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Errorf("--json stdout does not parse with a notice pending: %v\n%s", err, out)
	}
	assertNoNotice(t, "dump tree --json", errOut)
	if n := tw.count(); n != 0 {
		t.Errorf("%d outbound requests, want 0", n)
	}
}

// A stale or absent record does not trigger a refresh when the streams are piped.
func TestPipedRunsDoNotRefreshAStaleOrAbsentRecord(t *testing.T) {
	bin := buildCLI(t, releasedVersion)
	args := []string{"dump", "tree", testdataPDF(t, "minimal.pdf")}

	t.Run("stale record", func(t *testing.T) {
		tw := newTripwire(t)
		xdg := t.TempDir()
		seeded := seedSnapshot(t, xdg, time.Now().Add(-48*time.Hour), newerVersion)
		_, errOut, code := runCLI(t, bin, merge(tw.env(), map[string]string{"XDG_CACHE_HOME": xdg}), args...)
		if code != 0 {
			t.Fatalf("exit %d, stderr: %s", code, errOut)
		}
		assertNoNotice(t, "stale record", errOut)
		after, err := os.ReadFile(snapshotPath(xdg))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(after, seeded) {
			t.Errorf("a piped run wrote an attempt record\nbefore %s\nafter  %s", seeded, after)
		}
		if n := tw.count(); n != 0 {
			t.Errorf("piped run refreshed a stale record: %d outbound requests", n)
		}
	})

	t.Run("absent record", func(t *testing.T) {
		tw := newTripwire(t)
		xdg := t.TempDir()
		_, errOut, code := runCLI(t, bin, merge(tw.env(), map[string]string{"XDG_CACHE_HOME": xdg}), args...)
		if code != 0 {
			t.Fatalf("exit %d, stderr: %s", code, errOut)
		}
		assertNoNotice(t, "absent record", errOut)
		assertDirEmpty(t, xdg)
		if n := tw.count(); n != 0 {
			t.Errorf("piped run refreshed an absent record: %d outbound requests", n)
		}
	})
}

// validate and diff keep their three-way exit codes with a notice pending.
func TestGateExitCodesKeepTheirMeaningWithUpdatePending(t *testing.T) {
	bin := buildCLI(t, releasedVersion)
	minimal := testdataPDF(t, "minimal.pdf")
	missing := filepath.Join(t.TempDir(), "missing.pdf")
	cases := []cliCase{
		{"validate no errors", []string{"validate", "--profile", "pdfua-1-structural", testdataPDF(t, "tagged.pdf")}, 0},
		{"validate errors found", []string{"validate", testdataPDF(t, "non-embedded-font.pdf")}, 1},
		{"validate operational error", []string{"validate", missing}, 2},
		{"diff identical", []string{"diff", minimal, minimal}, 0},
		{"diff differ", []string{"diff", minimal, testdataPDF(t, "multipage.pdf")}, 1},
		{"diff operational error", []string{"diff", minimal, testdataPDF(t, "malformed.pdf")}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tw := newTripwire(t)
			xdg := t.TempDir()
			seedSnapshot(t, xdg, time.Now().Add(-time.Hour), newerVersion)
			_, errOut, code := runCLI(t, bin, merge(tw.env(), map[string]string{"XDG_CACHE_HOME": xdg}), c.args...)
			if code != c.code {
				t.Errorf("exit code = %d, want %d\nstderr: %s", code, c.code, errOut)
			}
			assertNoNotice(t, c.name, errOut)
			if n := tw.count(); n != 0 {
				t.Errorf("%d outbound requests, want 0", n)
			}
		})
	}
}

// A dev build and a build whose version is not SemVer never create a cache file.
func TestUncheckableBuildsTouchNoCache(t *testing.T) {
	minimal := testdataPDF(t, "minimal.pdf")
	for _, v := range []string{"", "abc"} {
		label := v
		if label == "" {
			label = "dev"
		}
		t.Run(label, func(t *testing.T) {
			bin := buildCLI(t, v)
			tw := newTripwire(t)
			xdg := t.TempDir()
			for _, args := range [][]string{
				{"dump", "tree", minimal},
				{"validate", "--profile", "pdfua-1-structural", testdataPDF(t, "tagged.pdf")},
				{"--help"},
			} {
				_, errOut, _ := runCLI(t, bin, merge(tw.env(), map[string]string{"XDG_CACHE_HOME": xdg}), args...)
				assertNoNotice(t, strings.Join(args, " "), errOut)
			}
			assertDirEmpty(t, xdg)
			if n := tw.count(); n != 0 {
				t.Errorf("%d outbound requests, want 0", n)
			}
		})
	}
}

// --version on a dev build keeps stdout to the version line and says on stderr,
// in one line, that update checks are skipped for development builds.
func TestVersionOnDevBuildSaysChecksAreSkipped(t *testing.T) {
	bin := buildCLI(t, "")
	for _, flag := range []string{"--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			tw := newTripwire(t)
			xdg := t.TempDir()
			out, errOut, code := runCLI(t, bin, merge(tw.env(), map[string]string{"XDG_CACHE_HOME": xdg}), flag)
			if code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
			if string(out) != "pdfdebug version dev\n" {
				t.Errorf("stdout = %q, want exactly the version line", out)
			}
			lines := stderrLines(errOut)
			if len(lines) != 1 {
				t.Fatalf("stderr has %d lines, want exactly one saying checks are skipped for dev builds:\n%s", len(lines), errOut)
			}
			if !strings.Contains(strings.ToLower(lines[0]), "dev") {
				t.Errorf("stderr line %q does not mention development builds", lines[0])
			}
			assertNoNotice(t, flag, errOut)
			assertASCII(t, flag+" stderr", errOut)
			assertDirEmpty(t, xdg)
			if n := tw.count(); n != 0 {
				t.Errorf("%d outbound requests, want 0", n)
			}
		})
	}
}

// --version with either opt-out variable present, whatever its value, makes no
// request, writes no cache, keeps stdout to the version line, and says on
// stderr, in one line, that the check is disabled. It shows no box even when a
// fresh record advertises a newer version.
func TestVersionWithOptOutSaysTheCheckIsDisabled(t *testing.T) {
	bin := buildCLI(t, releasedVersion)
	optOuts := []map[string]string{
		{"PDFDEBUG_NO_UPDATE_CHECK": "1"},
		{"PDFDEBUG_NO_UPDATE_CHECK": ""},
		{"PDFDEBUG_NO_UPDATE_CHECK": "false"},
		{"NO_UPDATE_NOTIFIER": "1"},
		{"NO_UPDATE_NOTIFIER": "0"},
	}
	for _, flag := range []string{"--version", "-v"} {
		for _, opt := range optOuts {
			var label string
			for k, v := range opt {
				label = flag + " " + k + "=" + v
			}
			for _, seeded := range []bool{false, true} {
				name := label + " empty cache"
				if seeded {
					name = label + " fresh record"
				}
				t.Run(name, func(t *testing.T) {
					tw := newTripwire(t)
					xdg := t.TempDir()
					var before []byte
					if seeded {
						before = seedSnapshot(t, xdg, time.Now().Add(-time.Hour), newerVersion)
					}
					out, errOut, code := runCLI(t, bin, merge(tw.env(), opt, map[string]string{"XDG_CACHE_HOME": xdg}), flag)
					if code != 0 {
						t.Errorf("exit code = %d, want 0", code)
					}
					if string(out) != "pdfdebug version "+releasedVersion+"\n" {
						t.Errorf("stdout = %q, want exactly the version line", out)
					}
					lines := stderrLines(errOut)
					if len(lines) != 1 {
						t.Errorf("stderr has %d lines, want exactly one saying the check is disabled:\n%s", len(lines), errOut)
					}
					assertNoNotice(t, name, errOut)
					assertASCII(t, name+" stderr", errOut)
					if n := tw.count(); n != 0 {
						t.Errorf("%d outbound requests with the opt-out set, want 0", n)
					}
					if !seeded {
						assertDirEmpty(t, xdg)
						return
					}
					after, err := os.ReadFile(snapshotPath(xdg))
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(after, before) {
						t.Errorf("--version with the opt-out rewrote the record\nbefore %s\nafter  %s", before, after)
					}
					if _, err := os.Stat(shownStatePath(xdg)); !os.IsNotExist(err) {
						t.Errorf("--version with the opt-out created the shown-state file (stat err = %v)", err)
					}
				})
			}
		}
	}
}
