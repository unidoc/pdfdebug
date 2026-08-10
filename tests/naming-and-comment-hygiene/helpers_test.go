// Package naming_and_comment_hygiene_test holds the executable gates for the
// repo-wide naming and comment hygiene rules recorded in CLAUDE.md:
//
//  1. no scenario IDs, priority tags, risk IDs or acceptance-criterion
//     citations in source text, test names or assertion messages;
//  2. no story or epic numbers in directory, file or module names;
//  3. no test lost while those names and titles are being rewritten.
//
// Each gate reports the offending paths and a count, because the tree carries
// thousands of sites and "expected 0, got 3298" is not actionable.
//
// Run: cd tests/naming-and-comment-hygiene && go test -count=1 ./...
package naming_and_comment_hygiene_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// thisSuite is the repo-relative directory of these gates. It is excluded from
// the test-function inventory so the gates do not inflate the baseline they
// measure, and it is deliberately NOT excluded from the residue scan: the gates
// express every banned shape as a bracketed character class, which no banned
// pattern matches, so they are scanned like any other file.
const thisSuite = "tests/naming-and-comment-hygiene"

// scanSkipPrefixes are the directory prefixes outside the residue scan:
// generated output, vendored dependencies, build artifacts, and the docs tree
// (which belongs to the separate docs repo and is not governed by these rules).
var scanSkipPrefixes = []string{
	"bin/",
	"build/",
	"docs/",
	"frontend/bindings/",
	"frontend/dist/",
	"frontend/node_modules/",
	"node_modules/",
}

// scanSkipPaths are individual files outside the residue scan. CLAUDE.md is the
// document that states the rules, so it has to quote the banned shapes in order
// to ban them; scanning it would make the rules unstateable.
var scanSkipPaths = map[string]bool{
	"CLAUDE.md": true,
}

// projectRoot walks upward from the test working directory to the repo root,
// identified by the root go.mod's module line.
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
			t.Fatalf("could not find project root (no go.mod declaring module unidoc-pdf-debugger)")
		}
		dir = parent
	}
}

// trackedFiles returns every tracked repo-relative path, forward-slashed.
// The rules govern what is committed, so git's index is the authority rather
// than a directory walk, which would pick up local build output and caches.
func trackedFiles(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files in %s: %v: %s", root, err, stderr.String())
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, filepath.ToSlash(p))
		}
	}
	if len(paths) == 0 {
		t.Fatalf("git ls-files returned no paths in %s", root)
	}
	sort.Strings(paths)
	return paths
}

// scanFiles returns the tracked paths inside the residue scan's scope.
func scanFiles(t *testing.T, root string) []string {
	t.Helper()
	var kept []string
	for _, p := range trackedFiles(t, root) {
		if scanSkipPaths[p] {
			continue
		}
		skip := false
		for _, prefix := range scanSkipPrefixes {
			if strings.HasPrefix(p, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			kept = append(kept, p)
		}
	}
	return kept
}

// hit is one regex match: where it is and what matched.
type hit struct {
	path string
	line int
	text string
}

// scanTree returns every match of re across the in-scope tracked files.
// Binary files are skipped by NUL sniffing, the same discriminator grep -I uses.
func scanTree(t *testing.T, root string, re *regexp.Regexp) []hit {
	t.Helper()
	var hits []hit
	for _, rel := range scanFiles(t, root) {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range re.FindAllString(line, -1) {
				hits = append(hits, hit{path: rel, line: i + 1, text: m})
			}
		}
	}
	return hits
}

// reportHits renders a failure body: the total, the per-file breakdown, and
// concrete file:line examples. A gate that only says "3 hits remain" is
// useless to whoever has to clear them.
func reportHits(hits []hit, maxFiles, maxSamples int) string {
	counts := map[string]int{}
	for _, h := range hits {
		counts[h.path]++
	}
	files := make([]string, 0, len(counts))
	for p := range counts {
		files = append(files, p)
	}
	sort.Slice(files, func(i, j int) bool {
		if counts[files[i]] != counts[files[j]] {
			return counts[files[i]] > counts[files[j]]
		}
		return files[i] < files[j]
	})

	var b strings.Builder
	fmt.Fprintf(&b, "%d occurrence(s) in %d file(s).\n\nby file:\n", len(hits), len(files))
	for i, p := range files {
		if i == maxFiles {
			fmt.Fprintf(&b, "  ... and %d more file(s)\n", len(files)-maxFiles)
			break
		}
		fmt.Fprintf(&b, "  %6d  %s\n", counts[p], p)
	}
	b.WriteString("\nexamples:\n")
	for i, h := range hits {
		if i == maxSamples {
			fmt.Fprintf(&b, "  ... and %d more occurrence(s)\n", len(hits)-maxSamples)
			break
		}
		fmt.Fprintf(&b, "  %s:%d: %s\n", h.path, h.line, h.text)
	}
	return b.String()
}

// reportPaths renders a sorted path list with a count.
func reportPaths(paths []string) string {
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	var b strings.Builder
	fmt.Fprintf(&b, "%d:\n", len(sorted))
	for _, p := range sorted {
		fmt.Fprintf(&b, "  %s\n", p)
	}
	return b.String()
}
