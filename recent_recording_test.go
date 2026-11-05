package main

import (
	"errors"
	"sync"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// recordingOpener is a pdfOpener that also implements recentRecorder. It opens
// every path except failPath and records what RecordRecentFile receives.
type recordingOpener struct {
	failPath string
	rootErr  error
	mu       sync.Mutex
	recorded []string
}

func (o *recordingOpener) OpenFile(path string) (*pdfcore.DocumentInfo, error) {
	if path == o.failPath {
		return nil, errors.New("malformed PDF")
	}
	return &pdfcore.DocumentInfo{TabID: "tab-" + path, FileName: "a.pdf", FilePath: path}, nil
}

func (o *recordingOpener) GetTreeRoot(tabID string) (*pdfcore.TreeNode, error) {
	if o.rootErr != nil {
		return nil, o.rootErr
	}
	return &pdfcore.TreeNode{ID: "root"}, nil
}

func (o *recordingOpener) GetChildren(tabID string, nodeID string) ([]*pdfcore.TreeNode, error) {
	return nil, nil
}

func (o *recordingOpener) CloseDocument(tabID string) error {
	return nil
}

func (o *recordingOpener) RecordRecentFile(path string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.recorded = append(o.recorded, path)
}

func openAndWait(svc pdfOpener, emitter eventEmitter, path string) {
	var wg sync.WaitGroup
	wg.Add(1)
	openFileAndEmitWithWarning(svc, emitter, path, "", &wg)
	wg.Wait()
}

func TestSuccessfulGoOpenRecordsTheRecentFile(t *testing.T) {
	opener := &recordingOpener{}
	emitter := &recordingEmitter{}

	openAndWait(opener, emitter, "/docs/a.pdf")

	if len(opener.recorded) != 1 || opener.recorded[0] != "/docs/a.pdf" {
		t.Fatalf("recorded %q, want [/docs/a.pdf]", opener.recorded)
	}
	events := emitter.snapshot()
	if events[len(events)-1] != eventDocumentOpened {
		t.Fatalf("events = %v, want document:opened last", events)
	}
}

func TestFailedGoOpenDoesNotRecordTheRecentFile(t *testing.T) {
	opener := &recordingOpener{failPath: "/docs/bad.pdf"}
	emitter := &recordingEmitter{}

	openAndWait(opener, emitter, "/docs/bad.pdf")

	if len(opener.recorded) != 0 {
		t.Fatalf("a failed open recorded %q", opener.recorded)
	}
	events := emitter.snapshot()
	if events[len(events)-1] != eventDocumentError {
		t.Fatalf("events = %v, want document:error last", events)
	}
}

func TestGoOpenWhoseTreeRootFailsDoesNotRecordTheRecentFile(t *testing.T) {
	opener := &recordingOpener{rootErr: errors.New("no catalog")}
	emitter := &recordingEmitter{}

	openAndWait(opener, emitter, "/docs/a.pdf")

	if len(opener.recorded) != 0 {
		t.Fatalf("an open whose tree root failed recorded %q", opener.recorded)
	}
	events := emitter.snapshot()
	if events[len(events)-1] != eventDocumentError {
		t.Fatalf("events = %v, want document:error last", events)
	}
}

func TestLiteralMenuLabelDoublesThePlatformMnemonicMarker(t *testing.T) {
	cases := []struct {
		goos, label, want string
	}{
		{"windows", "R&D_plan.pdf", "R&&D_plan.pdf"},
		{"linux", "R&D_plan.pdf", "R&D__plan.pdf"},
		{"darwin", "R&D_plan.pdf", "R&D_plan.pdf"},
		{"linux", "a_b_c.pdf - x_y", "a__b__c.pdf - x__y"},
		{"windows", "plain.pdf", "plain.pdf"},
	}
	for _, c := range cases {
		if got := literalMenuLabel(c.label, c.goos); got != c.want {
			t.Errorf("literalMenuLabel(%q, %q) = %q, want %q", c.label, c.goos, got, c.want)
		}
	}
}
