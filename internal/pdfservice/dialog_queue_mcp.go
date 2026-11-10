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

// QueueDialogPaths stores paths for the next OpenFileDialog call, which
// returns them and clears the queue. An empty list queues nothing. Exists
// only in builds with the mcp tag, for GUI automation.
func (s *PDFService) QueueDialogPaths(paths []string) {
	if len(paths) == 0 {
		return
	}
	dialogQueue.mu.Lock()
	defer dialogQueue.mu.Unlock()
	dialogQueue.paths = append(dialogQueue.paths, paths...)
}

// takeQueuedDialogPaths returns the queued paths and empties the queue.
func takeQueuedDialogPaths() []string {
	dialogQueue.mu.Lock()
	defer dialogQueue.mu.Unlock()
	paths := dialogQueue.paths
	dialogQueue.paths = nil
	return paths
}
