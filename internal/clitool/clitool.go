// Package clitool installs (and uninstalls) the bundled `pdfdebug` CLI onto the
// user's PATH on macOS by creating an unprivileged symlink. Story 11.2.
//
// Design invariants (see the story's Security section):
//   - No shell, no root escalation: the symlink is created with os.Symlink and
//     the link directory is chosen from a hardcoded candidate list. This package
//     intentionally imports neither os/exec nor any shell-escalation idiom; a
//     source-guard test enforces that.
//   - The symlink target is DERIVED from os.Executable() + EvalSymlinks, never
//     from user input. The "validation" here is a self-derivation sanity check,
//     not untrusted-input sanitization.
package clitool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MenuItemLabel is the exact macOS app-menu label for the install action. It
// carries a trailing ellipsis per Apple HIG ("opens a dialog"). main.go consumes
// this constant so the visible label has a single source of truth.
const MenuItemLabel = "Install 'pdfdebug' Command in PATH..."

// UninstallMenuItemLabel is the label the menu item flips to once the CLI is
// installed (optional uninstall affordance, AC #6).
const UninstallMenuItemLabel = "Uninstall 'pdfdebug' Command"

// cliName is the basename of both the bundled CLI and the installed symlink.
const cliName = "pdfdebug"

// resourcesCLISuffix is the bundle-relative shape every "ours" symlink target
// (and the derived install target) must end in.
var resourcesCLISuffix = filepath.Join("Contents", "Resources", cliName)

// DefaultCandidateDirs returns the ordered install-dir candidates: user-owned
// Homebrew bins (Apple Silicon, then Intel) and finally ~/.local/bin. Detected
// by os.Stat at install time, never by exec'ing brew.
func DefaultCandidateDirs() []string {
	dirs := []string{"/opt/homebrew/bin", "/usr/local/bin"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	return dirs
}

// DefaultFallbackDir is the create-if-missing target used when no candidate is
// both writable and on $PATH (~/.local/bin).
func DefaultFallbackDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "bin")
	}
	return ""
}

// Options configures InstallCLI. ExecutablePath is the running GUI binary
// (os.Executable() in production). CandidateDirs and FallbackDir default to the
// Default* helpers when empty; tests inject temp dirs. Overwrite is set only
// after the user confirms a ConfirmOverwrite dialog.
type Options struct {
	ExecutablePath string
	CandidateDirs  []string
	FallbackDir    string
	Overwrite      bool
}

// InstallResult is the closed set of outcomes from InstallCLI. Callers
// type-switch on it to drive the appropriate dialog.
type InstallResult interface{ isInstallResult() }

// Installed reports a successful link at Path, on a dir already on $PATH.
type Installed struct {
	// Path is the created symlink (e.g. /opt/homebrew/bin/pdfdebug).
	Path string
}

// NeedsPathHelp reports the CLI was linked into Dir but Dir is not on $PATH;
// ExportLine is the exact shell-profile line the user must add.
type NeedsPathHelp struct {
	Dir        string
	ExportLine string
}

// ConfirmOverwrite reports a foreign entry already occupies LinkPath; the
// handler must ask the user before re-invoking with Options.Overwrite=true.
type ConfirmOverwrite struct {
	LinkPath       string
	ExistingTarget string
}

// NotInBundle reports the running binary is not inside a .app/Contents/MacOS
// layout (e.g. `go run` / dev binary), so there is no bundled CLI to link.
type NotInBundle struct{}

func (Installed) isInstallResult()        {}
func (NeedsPathHelp) isInstallResult()    {}
func (ConfirmOverwrite) isInstallResult() {}
func (NotInBundle) isInstallResult()      {}

// RunningExecutablePath returns the current GUI binary's path resolved through
// any symlinks (os.Executable() + filepath.EvalSymlinks), suitable as
// Options.ExecutablePath in production. main.go calls this so the bundle
// derivation starts from the real on-disk location.
func RunningExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil
	}
	return resolved, nil
}

// resolveBundleCLI derives the bundled CLI path from the running GUI binary's
// path. It confirms execPath sits in <X>.app/Contents/MacOS/ and returns the
// sibling <X>.app/Contents/Resources/pdfdebug. The caller is responsible for
// passing an already-resolved path (production uses os.Executable() +
// filepath.EvalSymlinks via ResolveBundleCLIFromRunning); the returned target
// is computed by walking execPath's own directory components so any
// space/quote/$ in the bundle path round-trips verbatim into the symlink.
// A non-.app (dev/go-run) layout is rejected with an error.
func resolveBundleCLI(execPath string) (string, error) {
	macOSDir := filepath.Dir(execPath) // .../Contents/MacOS
	contentsDir := filepath.Dir(macOSDir)
	if filepath.Base(macOSDir) != "MacOS" || filepath.Base(contentsDir) != "Contents" {
		return "", fmt.Errorf("not running from a macOS .app bundle (%q is not inside Contents/MacOS); run the install from the installed app", execPath)
	}
	appDir := filepath.Dir(contentsDir)
	if !strings.HasSuffix(appDir, ".app") {
		return "", fmt.Errorf("not running from a macOS .app bundle (parent %q does not end in .app); run the install from the installed app", appDir)
	}
	return filepath.Join(contentsDir, "Resources", cliName), nil
}

// sanityCheckTarget asserts the DERIVED target is absolute, NUL-free, and ends
// in Contents/Resources/pdfdebug. This is a self-derivation sanity check (the
// target comes from os.Executable(), not the user), not input sanitization.
func sanityCheckTarget(target string) error {
	if !filepath.IsAbs(target) {
		return fmt.Errorf("derived CLI target %q is not absolute", target)
	}
	if strings.ContainsRune(target, 0) {
		return errors.New("derived CLI target contains a NUL byte")
	}
	if !strings.HasSuffix(target, resourcesCLISuffix) {
		return fmt.Errorf("derived CLI target %q does not end in %q", target, resourcesCLISuffix)
	}
	return nil
}

// isOnPath reports whether dir is an exact entry in the current $PATH.
func isOnPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == dir {
			return true
		}
	}
	return false
}

// isWritableDir reports whether dir exists, is a directory, and is writable by
// the current user (probed by creating and removing a temp entry, since the
// mode bits alone do not account for ownership/ACLs).
func isWritableDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	probe, err := os.CreateTemp(dir, ".pdfdebug-write-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return true
}

// findInstallDir returns the first candidate that is BOTH writable AND on
// $PATH. If none qualifies it returns the first writable candidate (so the
// caller can still link there) with needsPath=true. If no candidate is even
// writable it returns an empty dir with needsPath=true, signalling the caller
// to use its create-if-missing fallback (~/.local/bin) rather than attempting
// to MkdirAll a non-writable system dir like /opt/homebrew/bin.
func findInstallDir(candidates []string) (dir string, needsPath bool) {
	var firstWritable string
	for _, c := range candidates {
		if isWritableDir(c) {
			if firstWritable == "" {
				firstWritable = c
			}
			if isOnPath(c) {
				return c, false
			}
		}
	}
	if firstWritable != "" {
		return firstWritable, true
	}
	return "", true
}

// exportLineFor returns the shell-profile line that prepends dir to $PATH.
func exportLineFor(dir string) string {
	return fmt.Sprintf(`export PATH="%s:$PATH"`, dir)
}

// classifyEntry inspects an existing link path. Returns one of: "absent",
// "ours" (a symlink whose target matches the .../Contents/Resources/pdfdebug
// shape, dangling allowed), or "foreign" (a regular file, or a symlink whose
// target does not match the shape). existingTarget is the symlink target when
// the entry is a symlink.
func classifyEntry(linkPath string) (kind, existingTarget string) {
	info, err := os.Lstat(linkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "absent", ""
		}
		return "foreign", ""
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "foreign", "" // regular file or dir
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		return "foreign", ""
	}
	if strings.HasSuffix(target, resourcesCLISuffix) {
		return "ours", target
	}
	return "foreign", target
}

// IsInstalled reports whether linkPath is OUR symlink (target matches the
// .../Contents/Resources/pdfdebug shape), whether or not it currently dangles.
// A foreign file, foreign-shaped symlink, or missing entry returns false.
func IsInstalled(linkPath string) bool {
	kind, _ := classifyEntry(linkPath)
	return kind == "ours"
}

// linkInto creates (or replaces ours/stale at) the symlink linkPath -> target.
// It assumes the clobber classification has already authorized the write.
func linkInto(target, linkPath string) error {
	// Remove an existing ours/stale link first; os.Symlink does not overwrite.
	if _, err := os.Lstat(linkPath); err == nil {
		if err := os.Remove(linkPath); err != nil {
			return fmt.Errorf("remove existing link %q: %w", linkPath, err)
		}
	}
	return os.Symlink(target, linkPath)
}

// InstallCLI links the bundled pdfdebug CLI onto the user's PATH. See the
// InstallResult variants for outcomes. It never escalates privileges and never
// invokes a shell.
func InstallCLI(opts Options) (InstallResult, error) {
	target, err := resolveBundleCLI(opts.ExecutablePath)
	if err != nil {
		return NotInBundle{}, nil
	}
	if err := sanityCheckTarget(target); err != nil {
		// A target failing the self-derivation sanity check means we are not in
		// a real bundle layout; surface it as NotInBundle rather than linking.
		return NotInBundle{}, nil
	}

	candidates := opts.CandidateDirs
	if len(candidates) == 0 {
		candidates = DefaultCandidateDirs()
	}
	fallback := opts.FallbackDir
	if fallback == "" {
		fallback = DefaultFallbackDir()
	}

	dir, needsPath := findInstallDir(candidates)

	// If the chosen dir does not exist yet (the ~/.local/bin fallback case),
	// create it 0o755 so we can still link and then guide the user.
	if needsPath {
		if dir == "" {
			dir = fallback
		}
		if dir == "" {
			return nil, errors.New("no writable install directory available and home directory could not be resolved")
		}
		if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create install dir %q: %w", dir, err)
			}
		}
	}

	linkPath := filepath.Join(dir, cliName)

	kind, existingTarget := classifyEntry(linkPath)
	switch kind {
	case "foreign":
		if !opts.Overwrite {
			return ConfirmOverwrite{LinkPath: linkPath, ExistingTarget: existingTarget}, nil
		}
		// Confirmed: fall through to (re)create the link below.
	case "ours":
		// Idempotent if already pointing at the current target; otherwise it is
		// our own stale/old-bundle link -> silently re-point. Both re-create.
	case "absent":
		// Nothing to clobber.
	}

	if err := linkInto(target, linkPath); err != nil {
		return nil, err
	}

	if needsPath {
		return NeedsPathHelp{Dir: dir, ExportLine: exportLineFor(dir)}, nil
	}
	return Installed{Path: linkPath}, nil
}

// UninstallCLI removes OUR symlink at linkPath. It refuses (returns an error)
// for a regular file or a foreign-shaped symlink, never removing a non-ours
// entry.
func UninstallCLI(linkPath string) error {
	kind, _ := classifyEntry(linkPath)
	switch kind {
	case "ours":
		return os.Remove(linkPath)
	case "absent":
		return nil
	default:
		return fmt.Errorf("refusing to remove %q: it is not a pdfdebug symlink created by this app", linkPath)
	}
}
