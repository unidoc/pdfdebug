// Package recentfiles keeps the File > Open Recent list: the most recently
// opened PDF paths, newest first, persisted as a small JSON file in the user
// config directory. It has no Wails dependency so it can be tested without a
// runtime.
package recentfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/adrg/xdg"
)

// MaxEntries is the most paths the list keeps.
const MaxEntries = 10

// DefaultPath returns where the list is stored: recent.json under the pdfdebug
// directory in the user config directory.
func DefaultPath() string {
	return filepath.Join(xdg.ConfigHome, "pdfdebug", "recent.json")
}

// List is the recent-files list backed by a JSON file. It is safe for
// concurrent use.
type List struct {
	mu    sync.Mutex
	path  string
	paths []string
	// saveMu serializes Save so the file always ends up with the newest copy.
	saveMu sync.Mutex
}

// Load reads the list stored at path. A missing, unreadable or malformed file
// gives an empty list; Load never fails. Entries are normalized the way Add
// stores them and duplicates are dropped, so Remove matches every entry.
func Load(path string) *List {
	l := &List{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		return l
	}
	var paths []string
	if json.Unmarshal(data, &paths) != nil {
		return l
	}
	seen := map[string]bool{}
	for _, p := range paths {
		if p == "" || len(l.paths) == MaxEntries {
			continue
		}
		p = normalize(p)
		if !seen[p] {
			seen[p] = true
			l.paths = append(l.paths, p)
		}
	}
	return l
}

// normalize returns the cleaned absolute form of path, falling back to the
// cleaned path when it cannot be made absolute.
func normalize(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// without returns paths minus every entry equal to p.
func without(paths []string, p string) []string {
	out := paths[:0]
	for _, q := range paths {
		if q != p {
			out = append(out, q)
		}
	}
	return out
}

// Add puts path at the top of the list, removing an earlier entry for the same
// file and dropping the oldest entry past MaxEntries.
func (l *List) Add(path string) {
	p := normalize(path)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.paths = append([]string{p}, without(l.paths, p)...)
	if len(l.paths) > MaxEntries {
		l.paths = l.paths[:MaxEntries]
	}
}

// Remove drops path from the list. An unknown path is a no-op.
func (l *List) Remove(path string) {
	p := normalize(path)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.paths = without(l.paths, p)
}

// Clear empties the list.
func (l *List) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.paths = nil
}

// Paths returns a copy of the list, most recent first.
func (l *List) Paths() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.paths...)
}

// Save writes the list to its file atomically: a temp file in the same
// directory is renamed over the target. The parent directory is created when
// missing.
func (l *List) Save() error {
	l.saveMu.Lock()
	defer l.saveMu.Unlock()
	l.mu.Lock()
	paths := append([]string{}, l.paths...)
	l.mu.Unlock()
	data, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".recent-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), l.path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Labels returns a menu label per path: the file name when it is unique in the
// list, otherwise the file name followed by as many trailing directories of its
// parent as it takes to tell the entries with that file name apart.
func Labels(paths []string) []string {
	byBase := map[string][]int{}
	for i, p := range paths {
		base := filepath.Base(p)
		byBase[base] = append(byBase[base], i)
	}
	labels := make([]string, len(paths))
	for base, idx := range byBase {
		if len(idx) == 1 {
			labels[idx[0]] = base
			continue
		}
		for depth := 1; ; depth++ {
			seen := map[string]bool{}
			unique, exhausted := true, true
			for _, i := range idx {
				dir, more := parentSuffix(paths[i], depth)
				labels[i] = base + " - " + dir
				unique = unique && !seen[labels[i]]
				seen[labels[i]] = true
				exhausted = exhausted && !more
			}
			if unique || exhausted {
				break
			}
		}
	}
	return labels
}

// parentSuffix returns the last depth directories of path's parent joined with
// the path separator, and whether the parent has more directories above them.
// When the walk reaches the root, the root itself ("/" or a volume such as
// "C:\") leads the suffix, so files in the roots of different volumes differ.
func parentSuffix(path string, depth int) (string, bool) {
	dir := filepath.Dir(path)
	var parts []string
	for len(parts) < depth {
		name := filepath.Base(dir)
		next := filepath.Dir(dir)
		if next == dir {
			parts = append([]string{dir}, parts...)
			break
		}
		parts = append([]string{name}, parts...)
		dir = next
	}
	return filepath.Join(parts...), filepath.Dir(dir) != dir
}
