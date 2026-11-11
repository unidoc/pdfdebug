//go:build mcp

package pdfservice

import "sync"

// dialogQueue holds paths that the next OpenFileDialog call returns instead
// of showing the native picker. It is package-level so value copies of
// PDFService share it.
var dialogQueue struct {
	mu    sync.Mutex
	paths []string
}

// QueueDialogPaths sets the paths the next OpenFileDialog call returns before
// clearing the queue. It replaces paths an earlier call queued and no dialog
// used, since File > Open does not consume them. An empty list changes
// nothing. Exists only in builds with the mcp tag, for GUI automation.
func (s *PDFService) QueueDialogPaths(paths []string) {
	if len(paths) == 0 {
		return
	}
	dialogQueue.mu.Lock()
	defer dialogQueue.mu.Unlock()
	dialogQueue.paths = append([]string(nil), paths...)
}

// takeQueuedDialogPaths returns the queued paths and empties the queue.
func takeQueuedDialogPaths() []string {
	dialogQueue.mu.Lock()
	defer dialogQueue.mu.Unlock()
	paths := dialogQueue.paths
	dialogQueue.paths = nil
	return paths
}
