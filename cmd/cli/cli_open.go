package main

import (
	"os"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// openForCLI creates an Inspector, opens the file under the "cli" tab, and
// emits the non-fatal structural warning. On failure it classifies the error
// via handleOpenError and returns a non-zero exit code; the inspector is nil in
// that case. On success it returns the live inspector (the caller owns Close)
// alongside the document info and a zero exit code.
func openForCLI(filePath string) (ins *pdfcore.Inspector, info *pdfcore.DocumentInfo, exitCode int) {
	ins = pdfcore.NewInspector()

	info, err := ins.Open("cli", filePath)
	if err != nil {
		return nil, nil, handleOpenError(err)
	}

	// Non-fatal warning for structurally damaged but parseable PDFs.
	if info.Error != "" {
		writeJSONWarning(os.Stderr, info.Error)
	}

	return ins, info, 0
}
