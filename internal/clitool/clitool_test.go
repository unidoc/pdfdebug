// Package clitool unit tests -- Story 11.2 "macOS: Install 'pdfdebug' Command
// in PATH". These are TDD RED-PHASE tests: they reference symbols
// (resolveBundleCLI, findInstallDir, InstallCLI, UninstallCLI, IsInstalled, the
// result types, and MenuItemLabel) that do NOT yet exist in package clitool, so
// the package will fail to compile until the Dev step implements it. That
// compile failure IS the red state. No t.Skip() sentinels (repo convention:
// red-phase tests ship directly and fail for the right reason).
//
// Scope: all install business logic is pure Go and OS-filesystem, so every AC
// except the native-menu wiring (AC1 glue) and the unsigned-build quarantine
// smoke (AC7) is covered here at UNIT level. No browser/HTTP surface exists for
// this story -- there is NO E2E. The macOS-native menu item cannot be reached
// by Playwright and main.go is source-grep-guarded; the menu wiring is verified
// manually (see the ATDD checklist) and only the LABEL string is unit-asserted
// here via the exported MenuItemLabel constant that main.go must consume.
//
// Trace:
//
//	AC #1 -> TestMenuItemLabelExactString (label only; menu gating is manual glue)
//	AC #2 -> TestResolveBundleCLIReturnsResourcesPath,
//	         TestInstallCLILinksIntoWritableOnPathDir,
//	         TestFindInstallDirPrefersWritableAndOnPath
//	AC #3 -> TestFindInstallDirReturnsNeedsPathWhenNoOnPathDir,
//	         TestInstallCLINeedsPathHelpSurfacesExportLine,
//	         TestInstallCLICreatesMissingLocalBin
//	AC #4 -> TestResolveBundleCLIRejectsNonAppLayout,
//	         TestInstallCLINotInBundleWhenDevBinary,
//	         TestInstallCLIIdempotentWhenOursAndCurrent,
//	         TestInstallCLIRepointsOursButStaleLink,
//	         TestInstallCLIConfirmOverwriteForeignFile,
//	         TestInstallCLIConfirmOverwriteForeignShapedSymlink,
//	         TestInstallCLIOverwriteFlagReplacesForeign
//	AC #5 -> TestInstallCLISpecialCharPathReadlinkRoundTrip
//	         (+ source-guard in clitool_sourceguard_test.go)
//	AC #6 -> TestIsInstalledTrueOnlyForOurSymlink,
//	         TestUninstallCLIRemovesOnlyOurSymlink,
//	         TestUninstallCLIRefusesForeignEntry
//	AC #7 -> manual (unsigned-build quarantine smoke; see ATDD checklist)
//
// Run: cd internal/clitool && go test -count=1 ./...
package clitool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// onlyDarwin skips a test on non-darwin hosts where the bundle/PATH semantics
// under test are macOS-specific. The install feature itself is gated to darwin
// (AC1), so exercising it elsewhere is not meaningful.
func onlyDarwin(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skipf("clitool install is macOS-only (runtime.GOOS=%q); skipping darwin-specific assertion", runtime.GOOS)
	}
}

// fakeBundle builds a minimal `<name>.app/Contents/{MacOS,Resources}` tree
// under base, writes an executable stub at Contents/MacOS/<name> (the "running
// GUI binary") and an executable stub at Contents/Resources/pdfdebug (the CLI),
// then returns the path to the MacOS binary and the Resources/pdfdebug path.
func fakeBundle(t *testing.T, base, name string) (macOSBin, resourcesCLI string) {
	t.Helper()
	appDir := filepath.Join(base, name+".app")
	macOSDir := filepath.Join(appDir, "Contents", "MacOS")
	resDir := filepath.Join(appDir, "Contents", "Resources")
	if err := os.MkdirAll(macOSDir, 0o755); err != nil {
		t.Fatalf("mkdir MacOS: %v", err)
	}
	if err := os.MkdirAll(resDir, 0o755); err != nil {
		t.Fatalf("mkdir Resources: %v", err)
	}
	macOSBin = filepath.Join(macOSDir, name)
	resourcesCLI = filepath.Join(resDir, "pdfdebug")
	if err := os.WriteFile(macOSBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write MacOS bin: %v", err)
	}
	if err := os.WriteFile(resourcesCLI, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write Resources cli: %v", err)
	}
	return macOSBin, resourcesCLI
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-001 (P1): AC #1 -- the menu item label is EXACTLY
// "Install 'pdfdebug' Command in PATH..." (trailing ellipsis per Apple HIG:
// "opens a dialog"). main.go must consume this exported constant so the visible
// label and the macOS-only gating have a single source of truth. The menu
// gating itself (runtime.GOOS == "darwin") and the FindByLabel/GetSubmenu wiring
// live in source-grep-guarded main.go and are verified manually.
// ---------------------------------------------------------------------------

func TestMenuItemLabelExactString(t *testing.T) {
	want := "Install 'pdfdebug' Command in PATH..."
	if MenuItemLabel != want {
		t.Errorf("MenuItemLabel = %q, want %q (AC #1: trailing ellipsis, not \"Install Command Line Tools\")", MenuItemLabel, want)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-002 (P0): AC #2/#4(a) -- resolveBundleCLI derives the CLI path from
// a Contents/MacOS/<bin> running-executable location and returns the sibling
// Contents/Resources/pdfdebug, NOT a hardcoded /Applications path.
// ---------------------------------------------------------------------------

func TestResolveBundleCLIReturnsResourcesPath(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, wantCLI := fakeBundle(t, tmp, "UniDoc PDF Debugger")

	got, err := resolveBundleCLI(macOSBin)
	if err != nil {
		t.Fatalf("resolveBundleCLI(%q) returned error: %v (AC #2)", macOSBin, err)
	}
	if got != wantCLI {
		t.Errorf("resolveBundleCLI = %q, want %q (AC #2: sibling Contents/Resources/pdfdebug)", got, wantCLI)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("resolveBundleCLI returned non-absolute path %q (AC #4a)", got)
	}
	if !strings.HasSuffix(got, filepath.Join("Contents", "Resources", "pdfdebug")) {
		t.Errorf("resolveBundleCLI %q must end in Contents/Resources/pdfdebug (AC #4a)", got)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-003 (P0): AC #4(a) -- a non-.app layout (a `go run`/dev binary that
// is NOT inside <X>.app/Contents/MacOS/) is rejected with a clear error rather
// than linking a bogus path.
// ---------------------------------------------------------------------------

func TestResolveBundleCLIRejectsNonAppLayout(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	devBin := filepath.Join(tmp, "pdfdebug-gui-dev")
	if err := os.WriteFile(devBin, []byte("dev"), 0o755); err != nil {
		t.Fatalf("write dev bin: %v", err)
	}
	if _, err := resolveBundleCLI(devBin); err == nil {
		t.Errorf("resolveBundleCLI(%q) must error for a non-.app (dev/go run) layout (AC #4a: \"run from the installed app\")", devBin)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-004 (P0): AC #2 -- when a directory is BOTH user-writable AND on
// $PATH, InstallCLI symlinks the bundled CLI into it without prompting and
// reports Installed. os.Readlink resolves to the bundle CLI.
// ---------------------------------------------------------------------------

func TestInstallCLILinksIntoWritableOnPathDir(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, wantCLI := fakeBundle(t, tmp, "UniDoc PDF Debugger")

	binDir := filepath.Join(tmp, "writable-on-path-bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, CandidateDirs: []string{binDir}})
	if err != nil {
		t.Fatalf("InstallCLI returned error: %v (AC #2)", err)
	}
	inst, ok := res.(Installed)
	if !ok {
		t.Fatalf("InstallCLI result = %T, want Installed (AC #2: writable+on-PATH dir is the success path)", res)
	}
	wantLink := filepath.Join(binDir, "pdfdebug")
	if inst.Path != wantLink {
		t.Errorf("Installed.Path = %q, want %q (AC #2)", inst.Path, wantLink)
	}
	target, err := os.Readlink(wantLink)
	if err != nil {
		t.Fatalf("Readlink(%q): %v -- InstallCLI must create a symlink (AC #2)", wantLink, err)
	}
	if target != wantCLI {
		t.Errorf("symlink target = %q, want %q (AC #2: link points at bundle CLI)", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-005 (P0): AC #4(a) -- InstallCLI run from a non-.app (dev) binary
// returns NotInBundle rather than linking a derived-but-bogus path.
// ---------------------------------------------------------------------------

func TestInstallCLINotInBundleWhenDevBinary(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	devBin := filepath.Join(tmp, "dev-gui")
	if err := os.WriteFile(devBin, []byte("dev"), 0o755); err != nil {
		t.Fatalf("write dev bin: %v", err)
	}
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	res, err := InstallCLI(Options{ExecutablePath: devBin, CandidateDirs: []string{binDir}})
	if err != nil {
		t.Fatalf("InstallCLI returned a hard error instead of a typed NotInBundle result: %v (AC #4a)", err)
	}
	if _, ok := res.(NotInBundle); !ok {
		t.Errorf("InstallCLI result = %T, want NotInBundle for a dev (non-.app) binary (AC #4a)", res)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-006 (P1): AC #2 -- findInstallDir prefers a dir that is BOTH
// writable AND on $PATH over one that is writable-but-not-on-PATH. (A
// writable-not-on-PATH dir would yield a success dialog whose `pdfdebug` call
// fails -- that is the AC3 NeedsPathHelp case, not the success path.)
// ---------------------------------------------------------------------------

func TestFindInstallDirPrefersWritableAndOnPath(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	writableNotOnPath := filepath.Join(tmp, "writable-off-path")
	writableOnPath := filepath.Join(tmp, "writable-on-path")
	for _, d := range []string{writableNotOnPath, writableOnPath} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	// Only writableOnPath is on PATH.
	t.Setenv("PATH", writableOnPath)

	// Order the candidates so the off-PATH dir is checked FIRST; findInstallDir
	// must still pick the on-PATH dir for the success path.
	dir, needsPath := findInstallDir([]string{writableNotOnPath, writableOnPath})
	if needsPath {
		t.Fatalf("findInstallDir signalled needsPath despite an available writable+on-PATH dir (AC #2)")
	}
	if dir != writableOnPath {
		t.Errorf("findInstallDir = %q, want %q (AC #2: writable+on-PATH wins over writable-not-on-PATH)", dir, writableOnPath)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-007 (P0): AC #3 -- when NO candidate dir is both writable and on
// $PATH, findInstallDir returns a fallback dir plus a needsPath signal (it does
// NOT pick a writable-off-PATH dir as a silent success).
// ---------------------------------------------------------------------------

func TestFindInstallDirReturnsNeedsPathWhenNoOnPathDir(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	writableOffPath := filepath.Join(tmp, "writable-off-path")
	if err := os.MkdirAll(writableOffPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// PATH contains a dir that is NOT among the candidates, so none qualify.
	t.Setenv("PATH", filepath.Join(tmp, "unrelated"))

	_, needsPath := findInstallDir([]string{writableOffPath})
	if !needsPath {
		t.Errorf("findInstallDir must signal needsPath when no candidate is writable AND on $PATH (AC #3)")
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-008 (P0): AC #3 -- when no writable+on-PATH dir exists, InstallCLI
// returns NeedsPathHelp carrying the exact `export PATH="...:$PATH"` line for
// the chosen directory; it does NOT shell out as root and does NOT silently
// fail.
// ---------------------------------------------------------------------------

func TestInstallCLINeedsPathHelpSurfacesExportLine(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, _ := fakeBundle(t, tmp, "UniDoc PDF Debugger")

	writableOffPath := filepath.Join(tmp, "writable-off-path")
	if err := os.MkdirAll(writableOffPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// No candidate on PATH.
	t.Setenv("PATH", filepath.Join(tmp, "unrelated"))

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, CandidateDirs: []string{writableOffPath}})
	if err != nil {
		t.Fatalf("InstallCLI returned error instead of NeedsPathHelp: %v (AC #3)", err)
	}
	help, ok := res.(NeedsPathHelp)
	if !ok {
		t.Fatalf("InstallCLI result = %T, want NeedsPathHelp (AC #3)", res)
	}
	if help.Dir == "" {
		t.Errorf("NeedsPathHelp.Dir is empty; must name a writable directory to link into (AC #3)")
	}
	if !strings.Contains(help.ExportLine, "export PATH=") {
		t.Errorf("NeedsPathHelp.ExportLine = %q, must contain an `export PATH=...` shell-profile line (AC #3)", help.ExportLine)
	}
	if !strings.Contains(help.ExportLine, help.Dir) {
		t.Errorf("NeedsPathHelp.ExportLine = %q must reference the target dir %q (AC #3)", help.ExportLine, help.Dir)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-009 (P1): AC #3 -- when the preferred fallback (~/.local/bin-style)
// dir does not exist, InstallCLI MAY create it (0o755) and link into it, then
// fall to NeedsPathHelp guidance. We model the fallback via CandidateDirs whose
// only entry is a not-yet-existing dir; the link must be created there.
// ---------------------------------------------------------------------------

func TestInstallCLICreatesMissingLocalBin(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, wantCLI := fakeBundle(t, tmp, "UniDoc PDF Debugger")

	missingDir := filepath.Join(tmp, "home", ".local", "bin") // does not exist yet
	// Not on PATH, so this is the create-and-guide path.
	t.Setenv("PATH", filepath.Join(tmp, "unrelated"))

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, CandidateDirs: []string{missingDir}, FallbackDir: missingDir})
	if err != nil {
		t.Fatalf("InstallCLI returned error: %v (AC #3)", err)
	}
	if _, ok := res.(NeedsPathHelp); !ok {
		t.Fatalf("InstallCLI result = %T, want NeedsPathHelp (AC #3: created dir + add-to-PATH guidance)", res)
	}
	info, err := os.Stat(missingDir)
	if err != nil {
		t.Fatalf("fallback dir %q was not created: %v (AC #3: MkdirAll 0o755)", missingDir, err)
	}
	if !info.IsDir() {
		t.Fatalf("fallback path %q is not a directory (AC #3)", missingDir)
	}
	link := filepath.Join(missingDir, "pdfdebug")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink(%q): %v -- InstallCLI must link into the created fallback dir (AC #3)", link, err)
	}
	if target != wantCLI {
		t.Errorf("symlink target = %q, want %q (AC #3)", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-019 (P0, regression): AC #3 -- when NO candidate is writable (the
// production clean-machine case: /opt/homebrew/bin and /usr/local/bin absent or
// not user-writable), InstallCLI must fall back to its create-if-missing
// FallbackDir (~/.local/bin) rather than attempting MkdirAll on a non-writable
// system candidate and erroring. Guards the findInstallDir none-writable path.
// ---------------------------------------------------------------------------

func TestInstallCLIFallsBackWhenNoCandidateWritable(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, wantCLI := fakeBundle(t, tmp, "UniDoc PDF Debugger")

	// Two non-existent candidate dirs whose parent is itself non-existent, so
	// they are neither writable nor trivially creatable -- they model the
	// absent/non-user-writable /opt/homebrew/bin and /usr/local/bin. The
	// fallback is a DISTINCT missing dir that IS creatable.
	cand1 := filepath.Join(tmp, "nope1", "bin")
	cand2 := filepath.Join(tmp, "nope2", "bin")
	fallback := filepath.Join(tmp, "home", ".local", "bin")
	t.Setenv("PATH", filepath.Join(tmp, "unrelated"))

	res, err := InstallCLI(Options{
		ExecutablePath: macOSBin,
		CandidateDirs:  []string{cand1, cand2},
		FallbackDir:    fallback,
	})
	if err != nil {
		t.Fatalf("InstallCLI errored instead of using the fallback dir: %v (AC #3: must not MkdirAll a non-writable system candidate)", err)
	}
	if _, ok := res.(NeedsPathHelp); !ok {
		t.Fatalf("InstallCLI result = %T, want NeedsPathHelp (AC #3: fallback create-and-guide)", res)
	}
	link := filepath.Join(fallback, "pdfdebug")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink(%q): %v -- InstallCLI must link into the created fallback dir, not a candidate (AC #3)", link, err)
	}
	if target != wantCLI {
		t.Errorf("symlink target = %q, want %q (AC #3)", target, wantCLI)
	}
	// The non-writable candidates must NOT have been created.
	if _, err := os.Stat(cand1); !os.IsNotExist(err) {
		t.Errorf("candidate %q should not have been created (AC #3)", cand1)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-010 (P0): AC #4(b) -- idempotency. Running InstallCLI twice leaves
// the link unchanged and errors on neither run (the second run sees OUR symlink
// already pointing at the current bundle CLI and no-ops).
// ---------------------------------------------------------------------------

func TestInstallCLIIdempotentWhenOursAndCurrent(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, wantCLI := fakeBundle(t, tmp, "UniDoc PDF Debugger")
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("PATH", binDir)
	opts := Options{ExecutablePath: macOSBin, CandidateDirs: []string{binDir}}

	if _, err := InstallCLI(opts); err != nil {
		t.Fatalf("first InstallCLI: %v", err)
	}
	res, err := InstallCLI(opts)
	if err != nil {
		t.Fatalf("second InstallCLI returned error (must be idempotent no-op): %v (AC #4b)", err)
	}
	if _, ok := res.(Installed); !ok {
		t.Errorf("second InstallCLI result = %T, want Installed (idempotent no-op) (AC #4b)", res)
	}
	link := filepath.Join(binDir, "pdfdebug")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink after second install: %v", err)
	}
	if target != wantCLI {
		t.Errorf("link target after idempotent re-install = %q, want %q (AC #4b: unchanged)", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-011 (P0): AC #4(c) -- OUR stale/old-bundle link self-heals. A
// pre-existing symlink whose target matches the `.../Contents/Resources/pdfdebug`
// shape but points at a DIFFERENT/old .app (the user reinstalled) is silently
// re-pointed to the freshly resolved target -- NOT treated as foreign.
// ---------------------------------------------------------------------------

func TestInstallCLIRepointsOursButStaleLink(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, currentCLI := fakeBundle(t, tmp, "UniDoc PDF Debugger")

	// An OLD bundle's CLI path (matches our shape, different .app). It may even
	// dangle -- the classifier keys on the path shape, not existence.
	oldCLI := filepath.Join(tmp, "old", "UniDoc PDF Debugger.app", "Contents", "Resources", "pdfdebug")

	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(binDir, "pdfdebug")
	if err := os.Symlink(oldCLI, link); err != nil {
		t.Fatalf("pre-create stale link: %v", err)
	}
	t.Setenv("PATH", binDir)

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, CandidateDirs: []string{binDir}})
	if err != nil {
		t.Fatalf("InstallCLI over a stale-ours link returned error: %v (AC #4c: must silently re-point)", err)
	}
	if _, ok := res.(Installed); !ok {
		t.Errorf("InstallCLI over a stale-ours link result = %T, want Installed (silently re-pointed) (AC #4c)", res)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink after re-point: %v", err)
	}
	if target != currentCLI {
		t.Errorf("link target after re-point = %q, want %q (AC #4c: re-pointed to current bundle)", target, currentCLI)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-012 (P0): AC #4(d) -- a FOREIGN regular file at the link path is
// NOT overwritten; InstallCLI returns ConfirmOverwrite and leaves the file
// intact.
// ---------------------------------------------------------------------------

func TestInstallCLIConfirmOverwriteForeignFile(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, _ := fakeBundle(t, tmp, "UniDoc PDF Debugger")
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(binDir, "pdfdebug")
	foreign := []byte("#!/bin/sh\necho not ours\n")
	if err := os.WriteFile(link, foreign, 0o755); err != nil {
		t.Fatalf("pre-create foreign file: %v", err)
	}
	t.Setenv("PATH", binDir)

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, CandidateDirs: []string{binDir}})
	if err != nil {
		t.Fatalf("InstallCLI over a foreign file returned error: %v (AC #4d: should return ConfirmOverwrite, not error)", err)
	}
	if _, ok := res.(ConfirmOverwrite); !ok {
		t.Errorf("InstallCLI over a foreign regular file result = %T, want ConfirmOverwrite (AC #4d)", res)
	}
	// The foreign file must be untouched (no overwrite without the flag).
	got, err := os.ReadFile(link)
	if err != nil {
		t.Fatalf("foreign file vanished: %v (AC #4d: must not overwrite)", err)
	}
	if string(got) != string(foreign) {
		t.Errorf("foreign file was modified without confirmation (AC #4d)")
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-013 (P0): AC #4(d) -- a FOREIGN-shaped symlink (target does NOT
// match `.../Contents/Resources/pdfdebug`) is NOT overwritten; InstallCLI
// returns ConfirmOverwrite and the link is left pointing at its original target.
// ---------------------------------------------------------------------------

func TestInstallCLIConfirmOverwriteForeignShapedSymlink(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, _ := fakeBundle(t, tmp, "UniDoc PDF Debugger")
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	foreignTarget := filepath.Join(tmp, "usr", "local", "bin", "pdfdebug-other")
	link := filepath.Join(binDir, "pdfdebug")
	if err := os.Symlink(foreignTarget, link); err != nil {
		t.Fatalf("pre-create foreign-shaped symlink: %v", err)
	}
	t.Setenv("PATH", binDir)

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, CandidateDirs: []string{binDir}})
	if err != nil {
		t.Fatalf("InstallCLI over a foreign-shaped symlink returned error: %v (AC #4d)", err)
	}
	if _, ok := res.(ConfirmOverwrite); !ok {
		t.Errorf("InstallCLI over a foreign-shaped symlink result = %T, want ConfirmOverwrite (AC #4d)", res)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != foreignTarget {
		t.Errorf("foreign-shaped symlink was re-pointed without confirmation: got %q, want original %q (AC #4d)", target, foreignTarget)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-014 (P1): AC #4(d) -- with the explicit overwrite flag (the user
// confirmed via the ConfirmOverwrite dialog), InstallCLI replaces a foreign
// entry with OUR symlink to the current bundle CLI.
// ---------------------------------------------------------------------------

func TestInstallCLIOverwriteFlagReplacesForeign(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, wantCLI := fakeBundle(t, tmp, "UniDoc PDF Debugger")
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(binDir, "pdfdebug")
	if err := os.WriteFile(link, []byte("foreign"), 0o755); err != nil {
		t.Fatalf("pre-create foreign file: %v", err)
	}
	t.Setenv("PATH", binDir)

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, CandidateDirs: []string{binDir}, Overwrite: true})
	if err != nil {
		t.Fatalf("InstallCLI with Overwrite returned error: %v (AC #4d)", err)
	}
	if _, ok := res.(Installed); !ok {
		t.Fatalf("InstallCLI with Overwrite result = %T, want Installed (AC #4d: confirmed overwrite)", res)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink after confirmed overwrite: %v -- entry must now be OUR symlink", err)
	}
	if target != wantCLI {
		t.Errorf("link target after confirmed overwrite = %q, want %q (AC #4d)", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-015 (P0): AC #5 -- the install LOCATION may legitimately contain
// shell-significant characters (space, single quote, `$`). InstallCLI passes
// paths directly to os.Symlink with NO shell, so os.Readlink round-trips the
// exact byte-for-byte target including those characters.
// ---------------------------------------------------------------------------

func TestInstallCLISpecialCharPathReadlinkRoundTrip(t *testing.T) {
	onlyDarwin(t)
	// Bundle dir whose path contains a space, a single quote, and a `$`.
	tricky := filepath.Join(t.TempDir(), "My $(apps)", "Jose O'Brien")
	if err := os.MkdirAll(tricky, 0o755); err != nil {
		t.Fatalf("mkdir tricky base: %v", err)
	}
	macOSBin, wantCLI := fakeBundle(t, tricky, "UniDoc PDF Debugger")

	binDir := filepath.Join(tricky, "bin dir's $cope")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir tricky binDir: %v", err)
	}
	t.Setenv("PATH", binDir)

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, CandidateDirs: []string{binDir}})
	if err != nil {
		t.Fatalf("InstallCLI on special-char path returned error: %v (AC #5)", err)
	}
	if _, ok := res.(Installed); !ok {
		t.Fatalf("InstallCLI on special-char path result = %T, want Installed (AC #5)", res)
	}
	link := filepath.Join(binDir, "pdfdebug")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink(%q): %v (AC #5)", link, err)
	}
	if target != wantCLI {
		t.Errorf("os.Readlink did NOT byte-round-trip the special-char target.\n got: %q\nwant: %q\n(AC #5: paths pass directly to os.Symlink, no shell)", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-016 (P1): AC #6 -- IsInstalled returns true ONLY when the link path
// is OUR symlink (target matches `.../Contents/Resources/pdfdebug`), whether or
// not it dangles; false for a foreign file, a foreign-shaped symlink, or a
// missing entry.
// ---------------------------------------------------------------------------

func TestIsInstalledTrueOnlyForOurSymlink(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(binDir, "pdfdebug")

	// Missing -> false.
	if IsInstalled(link) {
		t.Errorf("IsInstalled(missing) = true, want false (AC #6)")
	}

	// OUR-shaped symlink (even dangling) -> true.
	ourTarget := filepath.Join(tmp, "Some App.app", "Contents", "Resources", "pdfdebug")
	if err := os.Symlink(ourTarget, link); err != nil {
		t.Fatalf("symlink ours: %v", err)
	}
	if !IsInstalled(link) {
		t.Errorf("IsInstalled(our dangling symlink) = false, want true (AC #6: shape match, dangling allowed)")
	}
	_ = os.Remove(link)

	// Foreign-shaped symlink -> false.
	if err := os.Symlink(filepath.Join(tmp, "elsewhere", "pdfdebug"), link); err != nil {
		t.Fatalf("symlink foreign: %v", err)
	}
	if IsInstalled(link) {
		t.Errorf("IsInstalled(foreign-shaped symlink) = true, want false (AC #6)")
	}
	_ = os.Remove(link)

	// Regular file -> false.
	if err := os.WriteFile(link, []byte("foreign"), 0o755); err != nil {
		t.Fatalf("write foreign file: %v", err)
	}
	if IsInstalled(link) {
		t.Errorf("IsInstalled(regular file) = true, want false (AC #6)")
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-017 (P1): AC #6 -- UninstallCLI removes ONLY our symlink (verified
// by lstat + Readlink shape match before removal).
// ---------------------------------------------------------------------------

func TestUninstallCLIRemovesOnlyOurSymlink(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(binDir, "pdfdebug")
	ourTarget := filepath.Join(tmp, "UniDoc PDF Debugger.app", "Contents", "Resources", "pdfdebug")
	if err := os.Symlink(ourTarget, link); err != nil {
		t.Fatalf("symlink ours: %v", err)
	}

	if err := UninstallCLI(link); err != nil {
		t.Fatalf("UninstallCLI(our symlink) returned error: %v (AC #6)", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("UninstallCLI must remove our symlink; lstat err = %v (AC #6)", err)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-020 (P1, coverage expansion): AC #3/#5 -- the NeedsPathHelp export
// line is EXACTLY `export PATH="<dir>:$PATH"` with the directory DOUBLE-QUOTED,
// not merely a substring containing `export PATH=`. The double-quoting is the
// AC5 special-char invariant applied to the AC3 guidance: a `~/.local/bin`-style
// dir whose path contains a space (or `$`) would silently break an UNQUOTED
// export line the user pastes into their shell profile. UNIT-008 only asserts
// the `contains "export PATH="` substring, so the exact quoted shape is unpinned.
// ---------------------------------------------------------------------------

func TestInstallCLINeedsPathHelpExportLineIsQuoted(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, _ := fakeBundle(t, tmp, "UniDoc PDF Debugger")

	// Writable dir whose path contains a space and a `$` (the AC5 hazard set).
	writableOffPath := filepath.Join(tmp, "My $cope", "local bin")
	if err := os.MkdirAll(writableOffPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// No candidate on PATH -> NeedsPathHelp path.
	t.Setenv("PATH", filepath.Join(tmp, "unrelated"))

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, CandidateDirs: []string{writableOffPath}})
	if err != nil {
		t.Fatalf("InstallCLI returned error instead of NeedsPathHelp: %v (AC #3)", err)
	}
	help, ok := res.(NeedsPathHelp)
	if !ok {
		t.Fatalf("InstallCLI result = %T, want NeedsPathHelp (AC #3)", res)
	}
	// Exact shape: the dir MUST be double-quoted so spaces/$ survive a paste into
	// the shell profile (AC5 special-char invariant carried into AC3 guidance).
	want := `export PATH="` + writableOffPath + `:$PATH"`
	if help.ExportLine != want {
		t.Errorf("NeedsPathHelp.ExportLine = %q, want %q (AC #3/#5: dir must be double-quoted for space/$ safety)", help.ExportLine, want)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-021 (P1, coverage expansion): AC #6 -- UninstallCLI is idempotent
// when the link is ABSENT (e.g. the user already uninstalled, or trashed the
// app and removed the dangling link). It returns nil, not an error. The
// implementation has an explicit `absent -> nil` branch (clitool.go) but no test
// pinned it; the foreign-entry test (UNIT-018) and the our-symlink test
// (UNIT-017) leave this branch uncovered.
// ---------------------------------------------------------------------------

func TestUninstallCLIIdempotentWhenAbsent(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(binDir, "pdfdebug") // never created

	if err := UninstallCLI(link); err != nil {
		t.Errorf("UninstallCLI(absent link) = %v, want nil (AC #6: idempotent no-op when nothing is installed)", err)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-022 (P1, coverage expansion): AC #4(d) -- the explicit Overwrite
// flag replaces a FOREIGN-SHAPED SYMLINK (not just a regular file) with OUR
// symlink. UNIT-014 confirms the overwrite-a-regular-file path; the symlink
// case exercises a structurally different clobber branch (lstat reports a
// symlink, linkInto must os.Remove the symlink before re-creating it), and was
// only covered for the REFUSE case (UNIT-013), never the confirmed-overwrite
// case.
// ---------------------------------------------------------------------------

func TestInstallCLIOverwriteFlagReplacesForeignSymlink(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, wantCLI := fakeBundle(t, tmp, "UniDoc PDF Debugger")
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(binDir, "pdfdebug")
	foreignTarget := filepath.Join(tmp, "usr", "local", "bin", "pdfdebug-other")
	if err := os.Symlink(foreignTarget, link); err != nil {
		t.Fatalf("pre-create foreign-shaped symlink: %v", err)
	}
	t.Setenv("PATH", binDir)

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, CandidateDirs: []string{binDir}, Overwrite: true})
	if err != nil {
		t.Fatalf("InstallCLI with Overwrite over a foreign-shaped symlink returned error: %v (AC #4d)", err)
	}
	if _, ok := res.(Installed); !ok {
		t.Fatalf("InstallCLI with Overwrite result = %T, want Installed (AC #4d: confirmed overwrite of a foreign symlink)", res)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink after confirmed overwrite: %v -- entry must now be OUR symlink", err)
	}
	if target != wantCLI {
		t.Errorf("link target after confirmed overwrite = %q, want %q (AC #4d: foreign symlink re-pointed to bundle CLI)", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-023 (P1, coverage expansion): AC #2/#3 -- the PRODUCTION default
// candidate ordering is load-bearing (the Decision section pins Apple-Silicon
// Homebrew first, then Intel Homebrew, then ~/.local/bin) and feeds the real
// main.go install/IsInstalled scan, yet every InstallCLI test injects
// CandidateDirs and never exercises DefaultCandidateDirs / DefaultFallbackDir.
// A reordering or a dropped candidate would silently change which dir gets the
// symlink with no test failure. This pins the contract without writing to any
// real system dir.
// ---------------------------------------------------------------------------

func TestDefaultCandidateDirsOrderingAndFallback(t *testing.T) {
	onlyDarwin(t)
	dirs := DefaultCandidateDirs()
	if len(dirs) < 2 || dirs[0] != "/opt/homebrew/bin" || dirs[1] != "/usr/local/bin" {
		t.Fatalf("DefaultCandidateDirs() = %v, want [/opt/homebrew/bin /usr/local/bin ...] (AC #2: Apple-Silicon Homebrew first, then Intel)", dirs)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("home dir unresolvable on this host (%v); skipping ~/.local/bin ordering assertion", err)
	}
	wantLocal := filepath.Join(home, ".local", "bin")
	if dirs[len(dirs)-1] != wantLocal {
		t.Errorf("DefaultCandidateDirs() last entry = %q, want %q (AC #2/#3: ~/.local/bin is the final, portable candidate)", dirs[len(dirs)-1], wantLocal)
	}
	if fb := DefaultFallbackDir(); fb != wantLocal {
		t.Errorf("DefaultFallbackDir() = %q, want %q (AC #3: create-if-missing fallback is ~/.local/bin)", fb, wantLocal)
	}
}

// ---------------------------------------------------------------------------
// 11.2-UNIT-018 (P0): AC #6 -- UninstallCLI refuses to remove a foreign entry
// (regular file OR foreign-shaped symlink); the entry survives.
// ---------------------------------------------------------------------------

func TestUninstallCLIRefusesForeignEntry(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Foreign regular file must NOT be removed.
	fileLink := filepath.Join(binDir, "pdfdebug")
	if err := os.WriteFile(fileLink, []byte("foreign"), 0o755); err != nil {
		t.Fatalf("write foreign file: %v", err)
	}
	if err := UninstallCLI(fileLink); err == nil {
		t.Errorf("UninstallCLI(foreign regular file) returned nil; must refuse and error (AC #6)")
	}
	if _, err := os.Lstat(fileLink); err != nil {
		t.Errorf("UninstallCLI removed a foreign regular file: %v (AC #6: must never remove a non-ours entry)", err)
	}
	_ = os.Remove(fileLink)

	// Foreign-shaped symlink must NOT be removed.
	if err := os.Symlink(filepath.Join(tmp, "elsewhere", "pdfdebug"), fileLink); err != nil {
		t.Fatalf("symlink foreign: %v", err)
	}
	if err := UninstallCLI(fileLink); err == nil {
		t.Errorf("UninstallCLI(foreign-shaped symlink) returned nil; must refuse and error (AC #6)")
	}
	if _, err := os.Lstat(fileLink); err != nil {
		t.Errorf("UninstallCLI removed a foreign-shaped symlink: %v (AC #6)", err)
	}
}
