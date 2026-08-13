// Package clitool installs (and uninstalls) the bundled `pdfdebug` CLI onto the
// user's PATH on macOS by creating an unprivileged symlink. Story 11.2, revised
// in 12.1 to target only ~/.local/bin.
//
// Design invariants (see the story's Security section):
//   - No shell, no root escalation: the symlink is created with os.Symlink into
//     user-owned ~/.local/bin. This package intentionally imports neither
//     os/exec nor any shell-escalation idiom; a source-guard test enforces that.
//   - The symlink target is DERIVED from os.Executable() + EvalSymlinks, never
//     from user input. The "validation" here is a self-derivation sanity check,
//     not untrusted-input sanitization.
//   - We never write into a package-manager prefix (/opt/homebrew/bin,
//     /usr/local/bin); squatting there would collide with a future official
//     pdfdebug Homebrew formula at `brew link`.
package clitool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// MenuItemLabel is the exact macOS app-menu label for the install action. It
// carries a trailing ellipsis per Apple HIG ("opens a dialog"). main.go consumes
// this constant so the visible label has a single source of truth.
const MenuItemLabel = "Install 'pdfdebug' Command in PATH..."

// UninstallMenuItemLabel is the label the menu item flips to once the CLI is
// installed (optional uninstall affordance).
const UninstallMenuItemLabel = "Uninstall 'pdfdebug' Command"

// cliName is the basename of both the bundled CLI and the installed symlink.
const cliName = "pdfdebug"

// shellProfileMarker tags the PATH block AddDirToShellProfile appends, so a
// re-run can detect its own prior edit and stay idempotent.
const shellProfileMarker = "# Added by UniDoc PDF Debugger"

// resourcesCLISuffix is the bundle-relative shape every "ours" symlink target
// (and the derived install target) must end in.
var resourcesCLISuffix = filepath.Join("Contents", "Resources", cliName)

// ErrUnknownShell is returned by AddDirToShellProfile when $SHELL is empty or
// not one we know how to edit, so the caller falls back to showing the manual
// export line instead of guessing a profile file.
var ErrUnknownShell = errors.New("unrecognized shell; cannot edit a profile automatically")

// DefaultInstallDir returns the install target: user-owned ~/.local/bin. We
// deliberately target ONLY this dir: dropping a symlink into Homebrew's managed
// prefixes (/opt/homebrew/bin, /usr/local/bin) squats on a name a future
// official pdfdebug Homebrew formula would own, making `brew link` fail.
// ~/.local/bin is the XDG per-user bin dir and is never arbitrated by a package
// manager. Returns "" if the home directory cannot be resolved.
func DefaultInstallDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "bin")
	}
	return ""
}

// Options configures InstallCLI. ExecutablePath is the running GUI binary
// (os.Executable() in production). InstallDir defaults to DefaultInstallDir()
// when empty; tests inject a temp dir. Overwrite is set only after the user
// confirms a ConfirmOverwrite dialog.
type Options struct {
	ExecutablePath string
	InstallDir     string
	Overwrite      bool
}

// InstallResult is the closed set of outcomes from InstallCLI. Callers
// type-switch on it to drive the appropriate dialog.
type InstallResult interface{ isInstallResult() }

// Installed reports a successful link at Path, on a dir already on $PATH.
type Installed struct {
	// Path is the created symlink (e.g. ~/.local/bin/pdfdebug).
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
// filepath.EvalSymlinks via RunningExecutablePath); the returned target is
// computed by walking execPath's own directory components so any space/quote/$
// in the bundle path round-trips verbatim into the symlink. A non-.app
// (dev/go-run) layout is rejected with an error.
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
	return slices.Contains(filepath.SplitList(os.Getenv("PATH")), dir)
}

// exportLineFor returns the shell-profile line that prepends dir to $PATH
// (POSIX-shell syntax; zsh/bash).
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

// InstallCLI links the bundled pdfdebug CLI into ~/.local/bin. See the
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

	dir := opts.InstallDir
	if dir == "" {
		dir = DefaultInstallDir()
	}
	if dir == "" {
		return nil, errors.New("could not resolve ~/.local/bin: home directory unavailable")
	}

	// Create the install dir if missing (the common case: ~/.local/bin often
	// does not exist yet). os.Symlink below surfaces any not-writable error.
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create install dir %q: %w", dir, err)
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

	// ~/.local/bin is frequently not on a default macOS PATH; surface the help
	// path (and the auto-edit offer in the UI) when it is missing.
	if !isOnPath(dir) {
		return NeedsPathHelp{Dir: dir, ExportLine: exportLineFor(dir)}, nil
	}
	return Installed{Path: linkPath}, nil
}

// shellProfile returns the rc file to edit and the PATH line in that shell's
// syntax, derived from $SHELL. ok is false for an empty/unrecognized shell or
// an unresolvable home dir, so the caller falls back to manual guidance.
func shellProfile(dir string) (path, line string, ok bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", false
	}
	switch filepath.Base(os.Getenv("SHELL")) {
	case "zsh":
		return filepath.Join(home, ".zshrc"), exportLineFor(dir), true
	case "bash":
		return filepath.Join(home, ".bash_profile"), exportLineFor(dir), true
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish"), fmt.Sprintf("set -gx PATH %q $PATH", dir), true
	default:
		return "", "", false
	}
}

// AddDirToShellProfile appends an attributed PATH line for dir to the user's
// shell profile, idempotently (guarded by shellProfileMarker). It returns the
// profile path written/already-present. It performs plain file I/O only -- no
// shell is invoked. Returns ErrUnknownShell when $SHELL is empty/unrecognized.
func AddDirToShellProfile(dir string) (string, error) {
	path, line, ok := shellProfile(dir)
	if !ok {
		return "", ErrUnknownShell
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if strings.Contains(string(existing), shellProfileMarker) {
		return path, nil // already added on a prior run; idempotent no-op
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	var b strings.Builder
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "\n%s\n%s\n", shellProfileMarker, line)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(b.String()); err != nil {
		return "", err
	}
	return path, nil
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
