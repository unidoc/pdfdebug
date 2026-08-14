// Package clitool unit tests. The installer targets the macOS "Install
// 'pdfdebug' Command in PATH" action, installs ONLY into ~/.local/bin (never a
// Homebrew prefix), and offers an "Add it for me" shell-profile edit
// when ~/.local/bin is not on $PATH.
//
// Scope: all install business logic is pure Go and OS-filesystem, so every AC
// except the native-menu wiring and the unsigned-build quarantine smoke is
// covered here at UNIT level. There is NO E2E (the macOS-native menu item
// cannot be reached by Playwright; main.go is source-grep-guarded and verified
// manually); only the LABEL string is unit-asserted here via the exported
// MenuItemLabel constant that main.go consumes.
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
// under test are macOS-specific. The install feature itself is gated to
// darwin, so exercising it elsewhere is not meaningful.
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
// The menu item label is EXACTLY "Install 'pdfdebug' Command in PATH..."
// (trailing ellipsis per Apple HIG: "opens a dialog"). main.go must consume this
// exported constant so the visible label and the macOS-only gating have a single
// source of truth. The menu gating itself (runtime.GOOS == "darwin") and the
// FindByLabel/GetSubmenu wiring live in source-grep-guarded main.go and are
// verified manually.
// ---------------------------------------------------------------------------

func TestMenuItemLabelExactString(t *testing.T) {
	want := "Install 'pdfdebug' Command in PATH..."
	if MenuItemLabel != want {
		t.Errorf("MenuItemLabel = %q, want %q (trailing ellipsis, not \"Install Command Line Tools\")", MenuItemLabel, want)
	}
}

// ---------------------------------------------------------------------------
// resolveBundleCLI derives the CLI path from a Contents/MacOS/<bin>
// running-executable location and returns the sibling
// Contents/Resources/pdfdebug, NOT a hardcoded /Applications path.
// ---------------------------------------------------------------------------

func TestResolveBundleCLIReturnsResourcesPath(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, wantCLI := fakeBundle(t, tmp, "UniDoc PDF Debugger")

	got, err := resolveBundleCLI(macOSBin)
	if err != nil {
		t.Fatalf("resolveBundleCLI(%q) returned error: %v", macOSBin, err)
	}
	if got != wantCLI {
		t.Errorf("resolveBundleCLI = %q, want %q (sibling Contents/Resources/pdfdebug)", got, wantCLI)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("resolveBundleCLI returned non-absolute path %q", got)
	}
	if !strings.HasSuffix(got, filepath.Join("Contents", "Resources", "pdfdebug")) {
		t.Errorf("resolveBundleCLI %q must end in Contents/Resources/pdfdebug", got)
	}
}

// ---------------------------------------------------------------------------
// a non-.app layout (a `go run`/dev binary that is NOT inside
// <X>.app/Contents/MacOS/) is rejected with a clear error rather than linking a
// bogus path.
// ---------------------------------------------------------------------------

func TestResolveBundleCLIRejectsNonAppLayout(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	devBin := filepath.Join(tmp, "pdfdebug-gui-dev")
	if err := os.WriteFile(devBin, []byte("dev"), 0o755); err != nil {
		t.Fatalf("write dev bin: %v", err)
	}
	if _, err := resolveBundleCLI(devBin); err == nil {
		t.Errorf("resolveBundleCLI(%q) must error for a non-.app (dev/go run) layout (\"run from the installed app\")", devBin)
	}
}

// ---------------------------------------------------------------------------
// When a directory is BOTH user-writable AND on $PATH, InstallCLI symlinks
// the bundled CLI into it without prompting and reports Installed.
// os.Readlink resolves to the bundle CLI.
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

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, InstallDir: binDir})
	if err != nil {
		t.Fatalf("InstallCLI returned error: %v", err)
	}
	inst, ok := res.(Installed)
	if !ok {
		t.Fatalf("InstallCLI result = %T, want Installed (writable+on-PATH dir is the success path)", res)
	}
	wantLink := filepath.Join(binDir, "pdfdebug")
	if inst.Path != wantLink {
		t.Errorf("Installed.Path = %q, want %q", inst.Path, wantLink)
	}
	target, err := os.Readlink(wantLink)
	if err != nil {
		t.Fatalf("Readlink(%q): %v -- InstallCLI must create a symlink", wantLink, err)
	}
	if target != wantCLI {
		t.Errorf("symlink target = %q, want %q (link points at bundle CLI)", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// InstallCLI run from a non-.app (dev) binary returns NotInBundle
// rather than linking a derived-but-bogus path.
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

	res, err := InstallCLI(Options{ExecutablePath: devBin, InstallDir: binDir})
	if err != nil {
		t.Fatalf("InstallCLI returned a hard error instead of a typed NotInBundle result: %v", err)
	}
	if _, ok := res.(NotInBundle); !ok {
		t.Errorf("InstallCLI result = %T, want NotInBundle for a dev (non-.app) binary", res)
	}
}

// ---------------------------------------------------------------------------
// When ~/.local/bin is not on $PATH, InstallCLI returns NeedsPathHelp
// carrying the exact `export PATH="...:$PATH"` line for the install
// directory; it does NOT shell out as root and does NOT silently fail.
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

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, InstallDir: writableOffPath})
	if err != nil {
		t.Fatalf("InstallCLI returned error instead of NeedsPathHelp: %v", err)
	}
	help, ok := res.(NeedsPathHelp)
	if !ok {
		t.Fatalf("InstallCLI result = %T, want NeedsPathHelp", res)
	}
	if help.Dir == "" {
		t.Errorf("NeedsPathHelp.Dir is empty; must name a writable directory to link into")
	}
	if !strings.Contains(help.ExportLine, "export PATH=") {
		t.Errorf("NeedsPathHelp.ExportLine = %q, must contain an `export PATH=...` shell-profile line", help.ExportLine)
	}
	if !strings.Contains(help.ExportLine, help.Dir) {
		t.Errorf("NeedsPathHelp.ExportLine = %q must reference the target dir %q", help.ExportLine, help.Dir)
	}
}

// ---------------------------------------------------------------------------
// When ~/.local/bin does not exist yet, InstallCLI creates it (0o755) and links
// into it, then falls to NeedsPathHelp guidance. We model this via InstallDir
// set to a not-yet-existing dir; the link must be created there.
// ---------------------------------------------------------------------------

func TestInstallCLICreatesMissingLocalBin(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, wantCLI := fakeBundle(t, tmp, "UniDoc PDF Debugger")

	missingDir := filepath.Join(tmp, "home", ".local", "bin") // does not exist yet
	// Not on PATH, so this is the create-and-guide path.
	t.Setenv("PATH", filepath.Join(tmp, "unrelated"))

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, InstallDir: missingDir})
	if err != nil {
		t.Fatalf("InstallCLI returned error: %v", err)
	}
	if _, ok := res.(NeedsPathHelp); !ok {
		t.Fatalf("InstallCLI result = %T, want NeedsPathHelp (created dir + add-to-PATH guidance)", res)
	}
	info, err := os.Stat(missingDir)
	if err != nil {
		t.Fatalf("fallback dir %q was not created: %v (MkdirAll 0o755)", missingDir, err)
	}
	if !info.IsDir() {
		t.Fatalf("fallback path %q is not a directory", missingDir)
	}
	link := filepath.Join(missingDir, "pdfdebug")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink(%q): %v -- InstallCLI must link into the created fallback dir", link, err)
	}
	if target != wantCLI {
		t.Errorf("symlink target = %q, want %q", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// idempotency. Running InstallCLI twice leaves the link unchanged and
// errors on neither run (the second run sees OUR symlink already pointing at
// the current bundle CLI and no-ops).
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
	opts := Options{ExecutablePath: macOSBin, InstallDir: binDir}

	if _, err := InstallCLI(opts); err != nil {
		t.Fatalf("first InstallCLI: %v", err)
	}
	res, err := InstallCLI(opts)
	if err != nil {
		t.Fatalf("second InstallCLI returned error (must be idempotent no-op): %v", err)
	}
	if _, ok := res.(Installed); !ok {
		t.Errorf("second InstallCLI result = %T, want Installed (idempotent no-op)", res)
	}
	link := filepath.Join(binDir, "pdfdebug")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink after second install: %v", err)
	}
	if target != wantCLI {
		t.Errorf("link target after idempotent re-install = %q, want %q (unchanged)", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// OUR stale/old-bundle link self-heals. A pre-existing symlink whose
// target matches the `.../Contents/Resources/pdfdebug` shape but points at a
// DIFFERENT/old .app (the user reinstalled) is silently re-pointed to the freshly
// resolved target -- NOT treated as foreign.
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

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, InstallDir: binDir})
	if err != nil {
		t.Fatalf("InstallCLI over a stale-ours link returned error: %v (must silently re-point)", err)
	}
	if _, ok := res.(Installed); !ok {
		t.Errorf("InstallCLI over a stale-ours link result = %T, want Installed (silently re-pointed)", res)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink after re-point: %v", err)
	}
	if target != currentCLI {
		t.Errorf("link target after re-point = %q, want %q (re-pointed to current bundle)", target, currentCLI)
	}
}

// ---------------------------------------------------------------------------
// a FOREIGN regular file at the link path is NOT overwritten;
// InstallCLI returns ConfirmOverwrite and leaves the file intact.
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

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, InstallDir: binDir})
	if err != nil {
		t.Fatalf("InstallCLI over a foreign file returned error: %v (should return ConfirmOverwrite, not error)", err)
	}
	if _, ok := res.(ConfirmOverwrite); !ok {
		t.Errorf("InstallCLI over a foreign regular file result = %T, want ConfirmOverwrite", res)
	}
	// The foreign file must be untouched (no overwrite without the flag).
	got, err := os.ReadFile(link)
	if err != nil {
		t.Fatalf("foreign file vanished: %v (must not overwrite)", err)
	}
	if string(got) != string(foreign) {
		t.Errorf("foreign file was modified without confirmation")
	}
}

// ---------------------------------------------------------------------------
// a FOREIGN-shaped symlink (target does NOT match
// `.../Contents/Resources/pdfdebug`) is NOT overwritten; InstallCLI returns
// ConfirmOverwrite and the link is left pointing at its original target.
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

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, InstallDir: binDir})
	if err != nil {
		t.Fatalf("InstallCLI over a foreign-shaped symlink returned error: %v", err)
	}
	if _, ok := res.(ConfirmOverwrite); !ok {
		t.Errorf("InstallCLI over a foreign-shaped symlink result = %T, want ConfirmOverwrite", res)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != foreignTarget {
		t.Errorf("foreign-shaped symlink was re-pointed without confirmation: got %q, want original %q", target, foreignTarget)
	}
}

// ---------------------------------------------------------------------------
// with the explicit overwrite flag (the user confirmed via the
// ConfirmOverwrite dialog), InstallCLI replaces a foreign entry with OUR
// symlink to the current bundle CLI.
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

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, InstallDir: binDir, Overwrite: true})
	if err != nil {
		t.Fatalf("InstallCLI with Overwrite returned error: %v", err)
	}
	if _, ok := res.(Installed); !ok {
		t.Fatalf("InstallCLI with Overwrite result = %T, want Installed (confirmed overwrite)", res)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink after confirmed overwrite: %v -- entry must now be OUR symlink", err)
	}
	if target != wantCLI {
		t.Errorf("link target after confirmed overwrite = %q, want %q", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// The install LOCATION may legitimately contain shell-significant characters
// (space, single quote, `$`). InstallCLI passes paths directly to os.Symlink
// with NO shell, so os.Readlink round-trips the exact byte-for-byte target
// including those characters.
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

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, InstallDir: binDir})
	if err != nil {
		t.Fatalf("InstallCLI on special-char path returned error: %v", err)
	}
	if _, ok := res.(Installed); !ok {
		t.Fatalf("InstallCLI on special-char path result = %T, want Installed", res)
	}
	link := filepath.Join(binDir, "pdfdebug")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink(%q): %v", link, err)
	}
	if target != wantCLI {
		t.Errorf("os.Readlink did NOT byte-round-trip the special-char target.\n got: %q\nwant: %q\n(paths pass directly to os.Symlink, no shell)", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// IsInstalled returns true ONLY when the link path is OUR symlink (target
// matches `.../Contents/Resources/pdfdebug`), whether or not it dangles; false
// for a foreign file, a foreign-shaped symlink, or a missing entry.
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
		t.Errorf("IsInstalled(missing) = true, want false")
	}

	// OUR-shaped symlink (even dangling) -> true.
	ourTarget := filepath.Join(tmp, "Some App.app", "Contents", "Resources", "pdfdebug")
	if err := os.Symlink(ourTarget, link); err != nil {
		t.Fatalf("symlink ours: %v", err)
	}
	if !IsInstalled(link) {
		t.Errorf("IsInstalled(our dangling symlink) = false, want true (shape match, dangling allowed)")
	}
	_ = os.Remove(link)

	// Foreign-shaped symlink -> false.
	if err := os.Symlink(filepath.Join(tmp, "elsewhere", "pdfdebug"), link); err != nil {
		t.Fatalf("symlink foreign: %v", err)
	}
	if IsInstalled(link) {
		t.Errorf("IsInstalled(foreign-shaped symlink) = true, want false")
	}
	_ = os.Remove(link)

	// Regular file -> false.
	if err := os.WriteFile(link, []byte("foreign"), 0o755); err != nil {
		t.Fatalf("write foreign file: %v", err)
	}
	if IsInstalled(link) {
		t.Errorf("IsInstalled(regular file) = true, want false")
	}
}

// ---------------------------------------------------------------------------
// UninstallCLI removes ONLY our symlink (verified by lstat + Readlink shape
// match before removal).
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
		t.Fatalf("UninstallCLI(our symlink) returned error: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("UninstallCLI must remove our symlink; lstat err = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Coverage expansion: the NeedsPathHelp export line is EXACTLY `export
// PATH="<dir>:$PATH"` with the directory DOUBLE-QUOTED, not merely a substring
// containing `export PATH=`. The double-quoting is the special-char invariant
// applied to the guidance: a `~/.local/bin`-style dir whose path contains a space
// (or `$`) would silently break an UNQUOTED export line the user pastes into
// their shell profile. UNIT-008 only asserts the `contains "export PATH="`
// substring, so the exact quoted shape is unpinned.
// ---------------------------------------------------------------------------

func TestInstallCLINeedsPathHelpExportLineIsQuoted(t *testing.T) {
	onlyDarwin(t)
	tmp := t.TempDir()
	macOSBin, _ := fakeBundle(t, tmp, "UniDoc PDF Debugger")

	// Writable dir whose path contains a space and a `$` (the hazard set).
	writableOffPath := filepath.Join(tmp, "My $cope", "local bin")
	if err := os.MkdirAll(writableOffPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// No candidate on PATH -> NeedsPathHelp path.
	t.Setenv("PATH", filepath.Join(tmp, "unrelated"))

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, InstallDir: writableOffPath})
	if err != nil {
		t.Fatalf("InstallCLI returned error instead of NeedsPathHelp: %v", err)
	}
	help, ok := res.(NeedsPathHelp)
	if !ok {
		t.Fatalf("InstallCLI result = %T, want NeedsPathHelp", res)
	}
	// Exact shape: the dir MUST be double-quoted so spaces/$ survive a paste into
	// the shell profile (special-char invariant carried into guidance).
	want := `export PATH="` + writableOffPath + `:$PATH"`
	if help.ExportLine != want {
		t.Errorf("NeedsPathHelp.ExportLine = %q, want %q (dir must be double-quoted for space/$ safety)", help.ExportLine, want)
	}
}

// ---------------------------------------------------------------------------
// Coverage expansion: UninstallCLI is idempotent when the link is ABSENT
// (e.g. the user already uninstalled, or trashed the app and removed the
// dangling link). It returns nil, not an error. The implementation has an
// explicit `absent -> nil` branch (clitool.go) but no test pinned it; the
// foreign-entry test (UNIT-018) and the our-symlink test (UNIT-017) leave this
// branch uncovered.
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
		t.Errorf("UninstallCLI(absent link) = %v, want nil (idempotent no-op when nothing is installed)", err)
	}
}

// ---------------------------------------------------------------------------
// Coverage expansion: the explicit Overwrite flag replaces a
// FOREIGN-SHAPED SYMLINK (not just a regular file) with OUR symlink. UNIT-014
// confirms the overwrite-a-regular-file path; the symlink case exercises a
// structurally different clobber branch (lstat reports a symlink, linkInto
// must os.Remove the symlink before re-creating it), and was only covered for
// the REFUSE case (UNIT-013), never the confirmed-overwrite case.
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

	res, err := InstallCLI(Options{ExecutablePath: macOSBin, InstallDir: binDir, Overwrite: true})
	if err != nil {
		t.Fatalf("InstallCLI with Overwrite over a foreign-shaped symlink returned error: %v", err)
	}
	if _, ok := res.(Installed); !ok {
		t.Fatalf("InstallCLI with Overwrite result = %T, want Installed (confirmed overwrite of a foreign symlink)", res)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink after confirmed overwrite: %v -- entry must now be OUR symlink", err)
	}
	if target != wantCLI {
		t.Errorf("link target after confirmed overwrite = %q, want %q (foreign symlink re-pointed to bundle CLI)", target, wantCLI)
	}
}

// ---------------------------------------------------------------------------
// The PRODUCTION default install dir is exactly ~/.local/bin and is NEVER a
// Homebrew-managed prefix. This pins the core 12.1 contract: we must not squat
// on /opt/homebrew/bin or /usr/local/bin (a future official pdfdebug Homebrew
// formula would collide there at `brew link`).
// ---------------------------------------------------------------------------

func TestDefaultInstallDirIsLocalBin(t *testing.T) {
	onlyDarwin(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("home dir unresolvable on this host (%v); skipping ~/.local/bin assertion", err)
	}
	got := DefaultInstallDir()
	want := filepath.Join(home, ".local", "bin")
	if got != want {
		t.Errorf("DefaultInstallDir = %q, want %q (install target is user-owned ~/.local/bin)", got, want)
	}
	if got == "/opt/homebrew/bin" || got == "/usr/local/bin" {
		t.Errorf("DefaultInstallDir = %q must NOT be a Homebrew-managed prefix (avoid brew-link collision)", got)
	}
}

// ---------------------------------------------------------------------------
// UninstallCLI refuses to remove a foreign entry (regular file OR
// foreign-shaped symlink); the entry survives.
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
		t.Errorf("UninstallCLI(foreign regular file) returned nil; must refuse and error")
	}
	if _, err := os.Lstat(fileLink); err != nil {
		t.Errorf("UninstallCLI removed a foreign regular file: %v (must never remove a non-ours entry)", err)
	}
	_ = os.Remove(fileLink)

	// Foreign-shaped symlink must NOT be removed.
	if err := os.Symlink(filepath.Join(tmp, "elsewhere", "pdfdebug"), fileLink); err != nil {
		t.Fatalf("symlink foreign: %v", err)
	}
	if err := UninstallCLI(fileLink); err == nil {
		t.Errorf("UninstallCLI(foreign-shaped symlink) returned nil; must refuse and error")
	}
	if _, err := os.Lstat(fileLink); err != nil {
		t.Errorf("UninstallCLI removed a foreign-shaped symlink: %v", err)
	}
}

// ---------------------------------------------------------------------------
// "Add it for me" appends the attributed PATH line to the shell's rc file and is
// idempotent: a second call detects its own marker and does NOT append a
// duplicate block.
// ---------------------------------------------------------------------------

func TestAddDirToShellProfileAppendsAndIsIdempotent(t *testing.T) {
	// shellProfile resolves home via os.UserHomeDir, which reads %USERPROFILE%
	// (not $HOME) on Windows, so the t.Setenv("HOME", ...) override below is a
	// no-op there. The feature is POSIX-only anyway ($SHELL/.zshrc); on Windows
	// $SHELL is unset and AddDirToShellProfile returns ErrUnknownShell.
	if runtime.GOOS == "windows" {
		t.Skip("shell-profile PATH editing is POSIX-only; os.UserHomeDir ignores $HOME on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	dir := filepath.Join(home, ".local", "bin")

	profile, err := AddDirToShellProfile(dir)
	if err != nil {
		t.Fatalf("AddDirToShellProfile returned error: %v", err)
	}
	wantProfile := filepath.Join(home, ".zshrc")
	if profile != wantProfile {
		t.Errorf("profile = %q, want %q (zsh -> ~/.zshrc)", profile, wantProfile)
	}
	data, err := os.ReadFile(profile)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	if !strings.Contains(string(data), shellProfileMarker) {
		t.Errorf("profile missing marker %q", shellProfileMarker)
	}
	if !strings.Contains(string(data), exportLineFor(dir)) {
		t.Errorf("profile missing export line %q", exportLineFor(dir))
	}

	// Second call: must not append a duplicate block.
	if _, err := AddDirToShellProfile(dir); err != nil {
		t.Fatalf("second AddDirToShellProfile returned error: %v", err)
	}
	data2, _ := os.ReadFile(profile)
	if got := strings.Count(string(data2), shellProfileMarker); got != 1 {
		t.Errorf("marker appears %d times after two calls, want 1 (idempotent)", got)
	}
}

// ---------------------------------------------------------------------------
// When $SHELL is unrecognized, AddDirToShellProfile returns ErrUnknownShell (so
// the UI falls back to manual guidance) and edits no file.
// ---------------------------------------------------------------------------

func TestAddDirToShellProfileUnknownShell(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/usr/bin/exoticsh")

	if _, err := AddDirToShellProfile(filepath.Join(home, ".local", "bin")); err != ErrUnknownShell {
		t.Errorf("AddDirToShellProfile with unknown shell = %v, want ErrUnknownShell", err)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Errorf("unknown shell must not create/edit any profile file; found %d entries", len(entries))
	}
}
