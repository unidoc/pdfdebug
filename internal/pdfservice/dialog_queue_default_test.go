//go:build !mcp

package pdfservice

import (
	"reflect"
	"strings"
	"testing"
)

func TestDialogQueueIsAbsentFromTheDefaultBuild(t *testing.T) {
	if _, ok := reflect.TypeOf(&PDFService{}).MethodByName("QueueDialogPaths"); ok {
		t.Error("QueueDialogPaths exists in a build without the mcp tag; it must only exist in mcp-tagged files")
	}
}

func TestDialogQueueTakeReturnsNothingInTheDefaultBuild(t *testing.T) {
	if got := takeQueuedDialogPaths(); got != nil {
		t.Errorf("takeQueuedDialogPaths() = %v in the default build, want nil", got)
	}
}

func TestDialogQueueOpenFileDialogUnchangedInTheDefaultBuild(t *testing.T) {
	s := &PDFService{}
	_, err := s.OpenFileDialog()
	if err == nil || !strings.Contains(err.Error(), "app not initialized") {
		t.Errorf("OpenFileDialog with no app: err = %v, want %q", err, "app not initialized")
	}
}
