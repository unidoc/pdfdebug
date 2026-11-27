// Package file_change_detection_test runs the colocated tests for reload from
// disk and on-disk change detection: the stale-read guard in pdfcore and the
// CLI, the on-disk status check, Windows busy-file handling, and the watcher's
// settle, cadence and isolation rules.
//
// Each test here names the colocated tests it needs and fails when any of
// them is missing, skipped or failing, so a deleted or renamed colocated test
// cannot pass silently. This suite is in the Windows CI subset: file identity
// (rename-over) and sharing violations behave differently there.
//
// Run: cd tests/file-change-detection && go test -v -count=1 ./...
package file_change_detection_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// projectRoot walks up to the go.mod of module unidoc-pdf-debugger.
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
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger)")
		}
		dir = parent
	}
}

// raceAvailable reports whether the toolchain can build with -race here.
func raceAvailable(t *testing.T, root string) bool {
	t.Helper()
	cmd := exec.Command("go", "env", "CGO_ENABLED")
	cmd.Dir = root
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "1"
}

// runColocated runs the named tests in pkg in one go test invocation and
// requires a PASS line for every one of them.
func runColocated(t *testing.T, pkg string, race bool, names ...string) {
	t.Helper()
	root := projectRoot(t)
	args := []string{"test", "-v", "-count=1", "-run", "^(" + strings.Join(names, "|") + ")$"}
	if race && raceAvailable(t, root) {
		args = append(args, "-race")
	}
	args = append(args, pkg)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	out := string(output)
	if err != nil {
		t.Fatalf("go %s failed:\n%s", strings.Join(args, " "), out)
	}
	for _, name := range names {
		if !strings.Contains(out, "--- PASS: "+name+" ") {
			t.Errorf("colocated test %s in %s did not run and pass", name, pkg)
		}
	}
	if t.Failed() {
		t.Logf("output:\n%s", out)
	}
}

// Plain Text reads refuse bytes from a file that was overwritten, deleted or
// renamed over since Open or during the read, cache nothing they refused, and still serve a
// result verified before the change.
func TestPlainTextReadRefusesChangedFile(t *testing.T) {
	runColocated(t, "./internal/pdfcore/", true,
		"TestFileChangeSentinelsCarryFrontendMarkers",
		"TestFileChangePlainTextRefusesOverwrittenFileAndCachesNothing",
		"TestFileChangePlainTextRefusesDeletedFile",
		"TestFileChangePlainTextRefusesRenameOver",
		"TestFileChangePlainTextReturnsCachedResultAfterLaterChange",
		"TestFileChangePlainTextRefusesChangeDuringRead",
	)
}

// Signature decomposition on a changed file keeps every signature's parsed
// details and notes the hole as not checked instead of reporting a mismatch;
// an unchanged file reports as before.
func TestSignaturesNoteChangedFile(t *testing.T) {
	runColocated(t, "./internal/pdfcore/", true,
		"TestFileChangeSignaturesNoteChangedFile",
		"TestFileChangeSignaturesNoteDeletedFile",
		"TestFileChangeSignaturesNoteChangeDuringWalk",
		"TestFileChangeSignaturesIgnoreChangeOnUnsignedFile",
		"TestFileChangeSignaturesUnchangedFileReportsAsBefore",
	)
}

// dump bytes (raw and --json) on a file rewritten after the CLI parsed it
// exits 2 with the file-changed message on stderr; dump signatures prints the
// hole check as not checked.
func TestCLIReportsChangedFileAsViewError(t *testing.T) {
	runColocated(t, "./cmd/cli/", false,
		"TestCLIFileChangedBytesJSON",
		"TestCLIFileChangedBytesRaw",
		"TestCLIFileChangedSignatures",
		"TestCLIFileChangedHookUnsetLeavesRunsUnaffected",
	)
}

// The inspector lists its open tabs and reports current, changed or missing
// against the Open fingerprint; the bound GetDiskStatus does the same and
// records its result in the watcher as the tab's last reported status.
func TestDiskStatusAgainstOpenFingerprint(t *testing.T) {
	runColocated(t, "./internal/pdfcore/", false,
		"TestTabIDsListsOpenDocuments",
		"TestDiskStatusCurrentChangedMissing",
		"TestDiskStatusDetectsRenameOver",
		"TestFileChangeSizeOnlyChangeIsDetected",
		"TestCopyDocumentFileGuardsTheRawBytes",
		"TestDiskStatusUnknownTab",
		"TestDiskStatusJSONShape",
	)
	runColocated(t, "./internal/pdfservice/", true,
		"TestGetDiskStatusReportsCurrentThenChanged",
		"TestGetDiskStatusUnknownTab",
		"TestGetDiskStatusResultIsRecordedInTheWatcher",
		"TestWatcherPollsOnlyWatchedTabs",
	)
}

// A Windows sharing or lock violation is a busy file, not a malformed PDF;
// off Windows nothing is busy.
func TestBusyFileIsNotAParseFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		runColocated(t, "./internal/pdfcore/", false,
			"TestFileBusyRecognisesWindowsErrnos",
			"TestStatWithIdentityCapturesTheFileIDAtStatTime",
			"TestFileBusyOpenOfExclusivelyLockedFile",
			"TestFileBusyPlainTextReadOfExclusivelyLockedFile",
			"TestFileBusySignaturesHoleReadOfLockedFile",
			"TestFileBusyCopyOfLockedFile",
		)
		return
	}
	runColocated(t, "./internal/pdfcore/", false, "TestFileBusyIsFalseOffWindows")
}

// A change is reported once, after two identical checks; a file still being
// written, a stat error and a never-changed tab emit nothing.
func TestWatcherSettleRules(t *testing.T) {
	runColocated(t, "./internal/filewatch/", true,
		"TestWatcherSettleReportsChangeAfterTwoIdenticalChecks",
		"TestWatcherSettleReportsFileStillBeingWritten",
		"TestWatcherSettleReportsMissingAfterTwoChecks",
		"TestWatcherStatErrorResetsSettleStreak",
		"TestWatcherRecordedStatusLetsReturnToCurrentEmit",
		"TestWatcherRecordedStatusRestartsSettleStreak",
		"TestWatcherEventJSONShape",
		"TestWatcherNewTabStartsAtBaselineWithoutEvent",
	)
}

// Ticks every 2s focused and 10s unfocused, checks every tab at once on
// focus, takes the focus value from the caller, and checks background tabs.
func TestWatcherCadenceAndFocus(t *testing.T) {
	runColocated(t, "./internal/filewatch/", true,
		"TestWatcherCadenceFollowsFocus",
		"TestWatcherFocusTakesCallerValueRegardlessOfOrder",
		"TestWatcherMergedFocusGainStillChecks",
		"TestWatcherChecksBackgroundTabsAndNeverEmitsForUnchangedOnes",
	)
}

// A hung stat stalls only its own tab, a closed tab's late result is dropped,
// and Stop does not wait for a hung stat.
func TestWatcherIsolation(t *testing.T) {
	runColocated(t, "./internal/filewatch/", true,
		"TestWatcherBlockedTabDoesNotDelayOthers",
		"TestWatcherDiscardsLateResultForClosedTab",
		"TestWatcherStopReturnsWhileStatIsBlocked",
	)
}
