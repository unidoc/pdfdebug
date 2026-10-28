package recentfiles_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/adrg/xdg"

	"unidoc-pdf-debugger/internal/recentfiles"
)

func newList(t *testing.T) (*recentfiles.List, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recent.json")
	return recentfiles.Load(path), path
}

func pdfPath(dir, name string) string {
	return filepath.Join(dir, name)
}

func assertPaths(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("paths = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paths = %q, want %q", got, want)
		}
	}
}

func TestMaxEntriesIsTen(t *testing.T) {
	if recentfiles.MaxEntries != 10 {
		t.Fatalf("MaxEntries = %d, want 10", recentfiles.MaxEntries)
	}
}

func TestAddKeepsMostRecentFirst(t *testing.T) {
	l, _ := newList(t)
	dir := t.TempDir()
	a, b, c := pdfPath(dir, "a.pdf"), pdfPath(dir, "b.pdf"), pdfPath(dir, "c.pdf")
	l.Add(a)
	l.Add(b)
	l.Add(c)
	assertPaths(t, l.Paths(), []string{c, b, a})
}

func TestAddExistingPathMovesItToTheTop(t *testing.T) {
	l, _ := newList(t)
	dir := t.TempDir()
	a, b, c := pdfPath(dir, "a.pdf"), pdfPath(dir, "b.pdf"), pdfPath(dir, "c.pdf")
	l.Add(a)
	l.Add(b)
	l.Add(c)
	l.Add(a)
	assertPaths(t, l.Paths(), []string{a, c, b})
}

func TestAddDedupesByCleanedAbsolutePath(t *testing.T) {
	l, _ := newList(t)
	dir := t.TempDir()
	clean := pdfPath(dir, "a.pdf")
	l.Add(filepath.Join(dir, "sub", "..", "a.pdf"))
	l.Add(clean)
	l.Add(filepath.Join(dir, ".", "a.pdf"))
	assertPaths(t, l.Paths(), []string{clean})
}

func TestAddStoresRelativePathsAsAbsolute(t *testing.T) {
	l, _ := newList(t)
	t.Chdir(t.TempDir())
	l.Add("rel.pdf")
	got := l.Paths()
	if len(got) != 1 {
		t.Fatalf("paths = %q, want one entry", got)
	}
	if !filepath.IsAbs(got[0]) || filepath.Base(got[0]) != "rel.pdf" {
		t.Fatalf("relative path stored as %q, want an absolute path ending in rel.pdf", got[0])
	}
}

func TestAddCapsTheListAtMaxEntries(t *testing.T) {
	l, _ := newList(t)
	dir := t.TempDir()
	var added []string
	for i := 0; i < recentfiles.MaxEntries+2; i++ {
		p := pdfPath(dir, fmt.Sprintf("f%02d.pdf", i))
		added = append(added, p)
		l.Add(p)
	}
	got := l.Paths()
	if len(got) != recentfiles.MaxEntries {
		t.Fatalf("len(paths) = %d, want %d", len(got), recentfiles.MaxEntries)
	}
	if got[0] != added[len(added)-1] {
		t.Fatalf("first entry = %q, want the newest %q", got[0], added[len(added)-1])
	}
	for _, dropped := range added[:2] {
		for _, p := range got {
			if p == dropped {
				t.Fatalf("oldest entry %q still listed after the cap: %q", dropped, got)
			}
		}
	}
}

func TestRemoveDropsOnlyThatEntry(t *testing.T) {
	l, _ := newList(t)
	dir := t.TempDir()
	a, b, c := pdfPath(dir, "a.pdf"), pdfPath(dir, "b.pdf"), pdfPath(dir, "c.pdf")
	l.Add(a)
	l.Add(b)
	l.Add(c)
	l.Remove(b)
	assertPaths(t, l.Paths(), []string{c, a})
}

func TestRemoveUnknownPathIsANoop(t *testing.T) {
	l, _ := newList(t)
	dir := t.TempDir()
	a := pdfPath(dir, "a.pdf")
	l.Add(a)
	l.Remove(pdfPath(dir, "missing.pdf"))
	assertPaths(t, l.Paths(), []string{a})
}

func TestClearEmptiesTheList(t *testing.T) {
	l, _ := newList(t)
	dir := t.TempDir()
	l.Add(pdfPath(dir, "a.pdf"))
	l.Add(pdfPath(dir, "b.pdf"))
	l.Clear()
	if got := l.Paths(); len(got) != 0 {
		t.Fatalf("paths after Clear = %q, want none", got)
	}
}

func TestPathsReturnsACopy(t *testing.T) {
	l, _ := newList(t)
	dir := t.TempDir()
	a := pdfPath(dir, "a.pdf")
	l.Add(a)
	got := l.Paths()
	got[0] = "changed"
	assertPaths(t, l.Paths(), []string{a})
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	l, path := newList(t)
	dir := t.TempDir()
	a, b := pdfPath(dir, "a.pdf"), pdfPath(dir, "b.pdf")
	l.Add(a)
	l.Add(b)
	if err := l.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	assertPaths(t, recentfiles.Load(path).Paths(), []string{b, a})
}

func TestSaveAfterClearPersistsAnEmptyList(t *testing.T) {
	l, path := newList(t)
	l.Add(pdfPath(t.TempDir(), "a.pdf"))
	if err := l.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	l.Clear()
	if err := l.Save(); err != nil {
		t.Fatalf("Save after Clear: %v", err)
	}
	if got := recentfiles.Load(path).Paths(); len(got) != 0 {
		t.Fatalf("reloaded paths = %q, want none", got)
	}
}

func TestSaveCreatesTheParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pdfdebug", "nested", "recent.json")
	l := recentfiles.Load(path)
	a := pdfPath(t.TempDir(), "a.pdf")
	l.Add(a)
	if err := l.Save(); err != nil {
		t.Fatalf("Save into a missing directory: %v", err)
	}
	assertPaths(t, recentfiles.Load(path).Paths(), []string{a})
}

func TestSaveLeavesNoTempFileBehind(t *testing.T) {
	l, path := newList(t)
	l.Add(pdfPath(t.TempDir(), "a.pdf"))
	if err := l.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := l.Save(); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory holds %q after Save, want only %q", names, filepath.Base(path))
	}
}

func TestLoadMissingFileIsAnEmptyList(t *testing.T) {
	l := recentfiles.Load(filepath.Join(t.TempDir(), "absent", "recent.json"))
	if l == nil {
		t.Fatal("Load returned nil for a missing file")
	}
	if got := l.Paths(); len(got) != 0 {
		t.Fatalf("paths = %q, want none", got)
	}
}

func TestLoadCorruptFileIsAnEmptyList(t *testing.T) {
	cases := map[string]string{
		"garbage":        "\x00\x01not json at all",
		"truncated json": `["/a.pdf", "/b.p`,
		"empty file":     "",
		"wrong shape":    `{"paths": 42}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "recent.json")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			l := recentfiles.Load(path)
			if l == nil {
				t.Fatal("Load returned nil for a corrupt file")
			}
			if got := l.Paths(); len(got) != 0 {
				t.Fatalf("paths = %q, want none", got)
			}
			a := pdfPath(t.TempDir(), "a.pdf")
			l.Add(a)
			if err := l.Save(); err != nil {
				t.Fatalf("Save over a corrupt file: %v", err)
			}
			assertPaths(t, recentfiles.Load(path).Paths(), []string{a})
		})
	}
}

func TestConcurrentRecordsStayConsistent(t *testing.T) {
	l, _ := newList(t)
	dir := t.TempDir()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				l.Add(pdfPath(dir, fmt.Sprintf("g%d-%d.pdf", g, i%15)))
				_ = l.Paths()
				if i%10 == 0 {
					l.Remove(pdfPath(dir, fmt.Sprintf("g%d-0.pdf", g)))
				}
			}
		}(g)
	}
	wg.Wait()
	got := l.Paths()
	if len(got) > recentfiles.MaxEntries {
		t.Fatalf("len(paths) = %d, over the cap %d", len(got), recentfiles.MaxEntries)
	}
	seen := map[string]bool{}
	for _, p := range got {
		if seen[p] {
			t.Fatalf("duplicate entry %q in %q", p, got)
		}
		seen[p] = true
	}
}

func TestLabelsUseTheBaseNameWhenUnique(t *testing.T) {
	paths := []string{
		filepath.Join("/", "docs", "a.pdf"),
		filepath.Join("/", "other", "b.pdf"),
	}
	got := recentfiles.Labels(paths)
	if len(got) != 2 || got[0] != "a.pdf" || got[1] != "b.pdf" {
		t.Fatalf("labels = %q, want [a.pdf b.pdf]", got)
	}
}

func TestLabelsDisambiguateDuplicateBaseNames(t *testing.T) {
	paths := []string{
		filepath.Join("/", "work", "invoices", "report.pdf"),
		filepath.Join("/", "work", "drafts", "report.pdf"),
		filepath.Join("/", "work", "notes.pdf"),
	}
	got := recentfiles.Labels(paths)
	if len(got) != 3 {
		t.Fatalf("labels = %q, want 3 entries", got)
	}
	if got[0] == got[1] {
		t.Fatalf("duplicate base names got the same label %q", got[0])
	}
	for i, parent := range []string{"invoices", "drafts"} {
		if !strings.Contains(got[i], "report.pdf") || !strings.Contains(got[i], parent) {
			t.Fatalf("label %q for %q must carry the file name and its parent %q", got[i], paths[i], parent)
		}
	}
	if got[2] != "notes.pdf" {
		t.Fatalf("label for a unique base name = %q, want notes.pdf", got[2])
	}
}

func TestDefaultPathIsUnderTheUserConfigDirectory(t *testing.T) {
	want := filepath.Join(xdg.ConfigHome, "pdfdebug", "recent.json")
	if got := recentfiles.DefaultPath(); got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
	if strings.HasPrefix(recentfiles.DefaultPath(), xdg.CacheHome+string(filepath.Separator)) {
		t.Fatalf("DefaultPath() %q is under the cache directory", recentfiles.DefaultPath())
	}
}

func TestLabelsWidenUntilParentsWithTheSameNameDiffer(t *testing.T) {
	paths := []string{
		filepath.Join("/", "proj-a", "out", "report.pdf"),
		filepath.Join("/", "proj-b", "out", "report.pdf"),
	}
	got := recentfiles.Labels(paths)
	if got[0] == got[1] {
		t.Fatalf("labels = %q, want them to differ", got)
	}
	for i, proj := range []string{"proj-a", "proj-b"} {
		if !strings.Contains(got[i], "report.pdf") || !strings.Contains(got[i], proj) || !strings.Contains(got[i], "out") {
			t.Fatalf("label %q for %q must carry the file name, %q and out", got[i], paths[i], proj)
		}
	}
}

func TestConcurrentSavesLeaveTheNewestList(t *testing.T) {
	l, path := newList(t)
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		l.Add(pdfPath(dir, fmt.Sprintf("f%02d.pdf", i)))
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Save(); err != nil {
				t.Errorf("Save: %v", err)
			}
		}()
	}
	wg.Wait()
	if err := l.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	assertPaths(t, recentfiles.Load(path).Paths(), l.Paths())
}

func TestLoadNormalizesAndDedupesStoredEntries(t *testing.T) {
	dir := t.TempDir()
	a := pdfPath(dir, "a.pdf")
	b := pdfPath(dir, "b.pdf")
	stored := []string{a, filepath.Join(dir, "x", "..", "a.pdf"), b, a}
	path := filepath.Join(t.TempDir(), "recent.json")
	data := `["` + strings.Join(stored, `","`) + `"]`
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(data, `\`, `\\`)), 0o644); err != nil {
		t.Fatal(err)
	}
	l := recentfiles.Load(path)
	assertPaths(t, l.Paths(), []string{a, b})
	l.Remove(a)
	assertPaths(t, l.Paths(), []string{b})
}

func TestLoadStoresRelativeEntriesAsAbsoluteSoRemoveMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recent.json")
	if err := os.WriteFile(path, []byte(`["rel.pdf"]`), 0o644); err != nil {
		t.Fatal(err)
	}
	l := recentfiles.Load(path)
	got := l.Paths()
	if len(got) != 1 || !filepath.IsAbs(got[0]) {
		t.Fatalf("paths = %q, want one absolute entry", got)
	}
	l.Remove(got[0])
	if left := l.Paths(); len(left) != 0 {
		t.Fatalf("paths after Remove = %q, want none", left)
	}
}

func TestLabelsTellApartAFileInTheRootFromOneInADirectory(t *testing.T) {
	paths := []string{
		filepath.Join("/", "report.pdf"),
		filepath.Join("/", "work", "report.pdf"),
	}
	got := recentfiles.Labels(paths)
	if got[0] == got[1] {
		t.Fatalf("labels = %q, want them to differ", got)
	}
	if strings.HasSuffix(got[0], " - ") {
		t.Fatalf("label %q for a file in the root has an empty directory part", got[0])
	}
}

func TestLabelsTellApartFilesUnderParentsThatOnlyDifferAtTheRoot(t *testing.T) {
	paths := []string{
		filepath.Join("/", "out", "report.pdf"),
		filepath.Join("/", "work", "out", "report.pdf"),
	}
	got := recentfiles.Labels(paths)
	if got[0] == got[1] {
		t.Fatalf("labels = %q, want them to differ", got)
	}
}

func TestLoadSkipsEmptyEntriesAndKeepsTheFirstMaxEntriesUniquePaths(t *testing.T) {
	dir := t.TempDir()
	var stored, want []string
	stored = append(stored, "")
	for i := 0; i < recentfiles.MaxEntries+3; i++ {
		p := pdfPath(dir, fmt.Sprintf("f%02d.pdf", i))
		stored = append(stored, p, p, "")
		if i < recentfiles.MaxEntries {
			want = append(want, p)
		}
	}
	path := filepath.Join(t.TempDir(), "recent.json")
	data := `["` + strings.Join(stored, `","`) + `"]`
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(data, `\`, `\\`)), 0o644); err != nil {
		t.Fatal(err)
	}
	assertPaths(t, recentfiles.Load(path).Paths(), want)
}

func TestLabelsOfAnEmptyListIsEmpty(t *testing.T) {
	if got := recentfiles.Labels(nil); len(got) != 0 {
		t.Fatalf("labels = %q, want none", got)
	}
}
