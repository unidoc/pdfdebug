package pdfservice

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// embeddedBytesInspector returns fixed bytes or a fixed error from
// GetEmbeddedFileBytes and records the address it was asked for.
type embeddedBytesInspector struct {
	pdfcore.Inspector
	data   []byte
	err    error
	panics bool
	tabID  string
	nodeID string
}

func (s *embeddedBytesInspector) GetEmbeddedFileBytes(tabID, nodeID string) ([]byte, error) {
	s.tabID, s.nodeID = tabID, nodeID
	if s.panics {
		var doc *pdfcore.DocumentState
		_ = doc.PDFContext
	}
	return s.data, s.err
}

// recordingChooser stands in for the native save dialog.
type recordingChooser struct {
	path      string
	err       error
	calls     int
	suggested string
}

func (c *recordingChooser) choose(name string) (string, error) {
	c.calls++
	c.suggested = name
	return c.path, c.err
}

func TestSaveEmbeddedFileWritesExactlyTheBytes(t *testing.T) {
	payload := []byte("<?xml version=\"1.0\"?><Invoice>\x00\x01\xff</Invoice>")
	ins := &embeddedBytesInspector{data: payload}
	dest := filepath.Join(t.TempDir(), "out.xml")
	chooser := &recordingChooser{path: dest}

	got, err := saveEmbeddedFile(ins, "tab-1", "obj:0:4", "factur-x.xml", chooser.choose)
	if err != nil {
		t.Fatalf("saveEmbeddedFile: %v", err)
	}
	if got != dest {
		t.Fatalf("returned path = %q, want %q", got, dest)
	}
	if ins.tabID != "tab-1" || ins.nodeID != "obj:0:4" {
		t.Fatalf("read (%q, %q), want (tab-1, obj:0:4)", ins.tabID, ins.nodeID)
	}
	if chooser.calls != 1 || chooser.suggested != "factur-x.xml" {
		t.Fatalf("dialog called %d times with %q, want once with factur-x.xml", chooser.calls, chooser.suggested)
	}
	written, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if !bytes.Equal(written, payload) {
		t.Fatalf("saved bytes = %q, want %q", written, payload)
	}
}

func TestSaveEmbeddedFileCancelWritesNothing(t *testing.T) {
	ins := &embeddedBytesInspector{data: []byte("payload")}
	dir := t.TempDir()
	chooser := &recordingChooser{path: ""}

	got, err := saveEmbeddedFile(ins, "tab-1", "obj:0:4", "a.bin", chooser.choose)
	if err != nil || got != "" {
		t.Fatalf("cancel returned (%q, %v), want (\"\", nil)", got, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("cancel wrote %d files", len(entries))
	}
}

func TestSaveEmbeddedFileReadErrorNeverOpensTheDialog(t *testing.T) {
	cases := map[string]error{
		"over the extraction ceiling": fmt.Errorf("%w: embedded file exceeds the size limit", pdfcore.ErrUnsupportedPDF),
		"unknown node":                fmt.Errorf("%w: obj:0:99", pdfcore.ErrDocumentNotFound),
	}
	for name, readErr := range cases {
		t.Run(name, func(t *testing.T) {
			ins := &embeddedBytesInspector{err: readErr}
			chooser := &recordingChooser{path: filepath.Join(t.TempDir(), "never.bin")}

			got, err := saveEmbeddedFile(ins, "tab-1", "obj:0:99", "a.bin", chooser.choose)
			if !errors.Is(err, readErr) {
				t.Fatalf("err = %v, want %v", err, readErr)
			}
			if got != "" {
				t.Fatalf("path = %q on a read error, want empty", got)
			}
			if chooser.calls != 0 {
				t.Fatalf("dialog opened %d times on a read error", chooser.calls)
			}
		})
	}
}

func TestSaveEmbeddedFileUnknownTabOnARealInspector(t *testing.T) {
	chooser := &recordingChooser{path: filepath.Join(t.TempDir(), "never.bin")}
	_, err := saveEmbeddedFile(pdfcore.NewInspector(), "no-such-tab", "obj:0:4", "a.bin", chooser.choose)
	if err == nil {
		t.Fatal("unknown tab returned no error")
	}
	if chooser.calls != 0 {
		t.Fatalf("dialog opened %d times for an unknown tab", chooser.calls)
	}
}

func TestSaveEmbeddedFileRecoversAnInspectorPanic(t *testing.T) {
	ins := &embeddedBytesInspector{panics: true}
	chooser := &recordingChooser{path: filepath.Join(t.TempDir(), "never.bin")}
	_, err := saveEmbeddedFile(ins, "tab-1", "obj:0:4", "a.bin", chooser.choose)
	if !errors.Is(err, pdfcore.ErrMalformedPDF) {
		t.Fatalf("err = %v, want ErrMalformedPDF from the recovered panic", err)
	}
	if chooser.calls != 0 {
		t.Fatalf("dialog opened %d times after a panic", chooser.calls)
	}
}

func TestSaveEmbeddedFileDialogErrorWritesNothing(t *testing.T) {
	dialogErr := errors.New("dialog failed")
	ins := &embeddedBytesInspector{data: []byte("payload")}
	chooser := &recordingChooser{err: dialogErr}
	got, err := saveEmbeddedFile(ins, "tab-1", "obj:0:4", "a.bin", chooser.choose)
	if !errors.Is(err, dialogErr) || got != "" {
		t.Fatalf("dialog error returned (%q, %v), want (\"\", %v)", got, err, dialogErr)
	}
}

func TestSaveEmbeddedFileWriteErrorIsReturned(t *testing.T) {
	ins := &embeddedBytesInspector{data: []byte("payload")}
	dest := filepath.Join(t.TempDir(), "missing-dir", "out.bin")
	chooser := &recordingChooser{path: dest}
	got, err := saveEmbeddedFile(ins, "tab-1", "obj:0:4", "a.bin", chooser.choose)
	if err == nil {
		t.Fatalf("write into a missing directory returned no error (path %q)", got)
	}
}

func TestPDFServiceExposesBackendEmbeddedSave(t *testing.T) {
	typ := reflect.TypeOf(&PDFService{})
	m, ok := typ.MethodByName("SaveEmbeddedFileToFile")
	if !ok {
		t.Fatal("PDFService has no SaveEmbeddedFileToFile method")
	}
	want := reflect.TypeOf(func(*PDFService, string, string, string) (string, error) { return "", nil })
	if m.Type != want {
		t.Fatalf("SaveEmbeddedFileToFile type = %v, want %v", m.Type, want)
	}
	if _, ok := typ.MethodByName("SaveBytesToFile"); ok {
		t.Fatal("PDFService still has SaveBytesToFile; the embedded save must not take bytes from the frontend")
	}
}

func TestRecordRecentFileWithoutARecorderIsANoop(t *testing.T) {
	svc := &PDFService{}
	svc.RecordRecentFile("/docs/a.pdf")
}

func TestRecordRecentFileCallsTheInjectedRecorder(t *testing.T) {
	svc := NewPDFService(nil)
	var mu sync.Mutex
	var got []string
	svc.SetRecentFileRecorder(func(path string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, path)
	})
	svc.RecordRecentFile("/docs/a.pdf")
	svc.RecordRecentFile("/docs/b.pdf")
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || got[0] != "/docs/a.pdf" || got[1] != "/docs/b.pdf" {
		t.Fatalf("recorder got %q, want [/docs/a.pdf /docs/b.pdf]", got)
	}
}

func TestOpenFileDoesNotRecordARecentFile(t *testing.T) {
	svc := NewPDFService(nil)
	calls := 0
	svc.SetRecentFileRecorder(func(string) { calls++ })
	info, err := svc.OpenFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer func() { _ = svc.CloseDocument(info.TabID) }()
	if calls != 0 {
		t.Fatalf("OpenFile recorded a recent file %d times; reload and compare open through it", calls)
	}
}

func TestSaveEmbeddedFileSeedsTheDialogWithTheLastPathElement(t *testing.T) {
	cases := map[string]string{
		"../../evil.sh":       "evil.sh",
		`..\..\evil.exe`:      "evil.exe",
		"/etc/passwd":         "passwd",
		"dir/sub/invoice.xml": "invoice.xml",
		"":                    "embedded-file",
		"..":                  "embedded-file",
		"dir/":                "embedded-file",
		"  ":                  "embedded-file",
		"factur-x.xml":        "factur-x.xml",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			ins := &embeddedBytesInspector{data: []byte("payload")}
			chooser := &recordingChooser{path: ""}
			if _, err := saveEmbeddedFile(ins, "tab-1", "obj:0:4", name, chooser.choose); err != nil {
				t.Fatalf("saveEmbeddedFile: %v", err)
			}
			if chooser.suggested != want {
				t.Fatalf("dialog seeded with %q for %q, want %q", chooser.suggested, name, want)
			}
		})
	}
}

func TestSaveEmbeddedFileReplacesAnExistingFileWholly(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.xml")
	if err := os.WriteFile(dest, []byte("a much longer earlier file body"), 0o644); err != nil {
		t.Fatal(err)
	}
	ins := &embeddedBytesInspector{data: []byte("new")}
	if _, err := saveEmbeddedFile(ins, "tab-1", "obj:0:4", "out.xml", (&recordingChooser{path: dest}).choose); err != nil {
		t.Fatalf("saveEmbeddedFile: %v", err)
	}
	written, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != "new" {
		t.Fatalf("saved bytes = %q, want %q", written, "new")
	}
}

func TestSaveEmbeddedFileWritesAnEmptyFileForAnEmptyAttachment(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "empty.bin")
	ins := &embeddedBytesInspector{data: []byte{}}
	got, err := saveEmbeddedFile(ins, "tab-1", "obj:0:4", "empty.bin", (&recordingChooser{path: dest}).choose)
	if err != nil || got != dest {
		t.Fatalf("saveEmbeddedFile = (%q, %v), want (%q, nil)", got, err, dest)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("empty attachment left no file: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("size = %d, want 0", info.Size())
	}
}

func TestSaveEmbeddedFileToFileWithoutAnAppFailsBeforeReading(t *testing.T) {
	ins := &embeddedBytesInspector{data: []byte("payload")}
	svc := &PDFService{inspector: ins}
	got, err := svc.SaveEmbeddedFileToFile("tab-1", "obj:0:4", "a.bin")
	if err == nil || got != "" {
		t.Fatalf("SaveEmbeddedFileToFile without an app = (%q, %v), want an error", got, err)
	}
	if ins.nodeID != "" {
		t.Fatalf("read %q before checking for an app", ins.nodeID)
	}
}
