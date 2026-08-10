// Story 12.1 AC5: PDFService.ConsumePendingOpenFiles service-boundary contract.
//
// The drain is exposed over the Wails service boundary as a thin delegation:
//   - SetPendingOpens(q *pendingopen.Queue) injects the queue from main.go.
//   - ConsumePendingOpenFiles() []string delegates to Queue.Drain().
//   - The method nil-guards: returns nil when no queue is wired (for tests).
//
// pdfservice is a thin adapter, so these are integration-level tests that
// exercise the delegation through the real PDFService + real Queue together.
// Harness compiled inside the main module (see helpers_test.go).
package story_12_1_test

import "testing"

// serviceHarnessPreamble imports both the service and the queue. The
// PDFService zero value must support the queue path (AC5: exported setter +
// nil-guarded method); the harness uses &pdfservice.PDFService{} so the test
// does not depend on a Wails app instance.
const serviceHarnessPreamble = `package atdd

import (
	"testing"

	"unidoc-pdf-debugger/internal/pdfservice"
	"unidoc-pdf-debugger/internal/pendingopen"
)
`

// serviceHarnessPreambleNoQueue is the variant for the nil-guard test, which
// exercises an unwired PDFService and so does not import the queue package
// (importing it unused is a build error once the package exists).
const serviceHarnessPreambleNoQueue = `package atdd

import (
	"testing"

	"unidoc-pdf-debugger/internal/pdfservice"
)
`

// Test_12_1_INTG_001_ConsumeDelegatesToDrain [P0] AC5: a wired PDFService
// delegates ConsumePendingOpenFiles to the queue's Drain -- it returns the
// queued cold-path paths, flips the queue ready, and a second call returns
// empty (drain-on-read idempotency survives the service boundary).
func Test_12_1_INTG_001_ConsumeDelegatesToDrain(t *testing.T) {
	out, err := runHarness(t, map[string]string{
		"harness_test.go": serviceHarnessPreamble + `
func TestConsumeDelegates(t *testing.T) {
	var q pendingopen.Queue
	q.Add("/cold-1.pdf")
	q.Add("/cold-2.pdf")

	svc := &pdfservice.PDFService{}
	svc.SetPendingOpens(&q)

	got := svc.ConsumePendingOpenFiles()
	if len(got) != 2 || got[0] != "/cold-1.pdf" || got[1] != "/cold-2.pdf" {
		t.Fatalf("[P0] 12.1-INTG-001: ConsumePendingOpenFiles must return drained paths in order, got %#v", got)
	}
	// Drain-on-read: second call empty, queue stays ready.
	if again := svc.ConsumePendingOpenFiles(); len(again) != 0 {
		t.Fatalf("[P0] 12.1-INTG-001: second consume must be empty (idempotent across the boundary), got %#v", again)
	}
	if got := q.Add("/warm.pdf"); got != true {
		t.Fatalf("[P0] 12.1-INTG-001: queue must be ready after a consume/drain, Add got %v", got)
	}
}
`,
	})
	if err != nil {
		t.Fatalf("[P0] 12.1-INTG-001 red phase (expected until SetPendingOpens/ConsumePendingOpenFiles land):\n%s", out)
	}
}

// Test_12_1_INTG_002_ConsumeNilGuard [P1] AC5: when no queue is wired, the
// method returns nil rather than panicking. The frontend null-guards this
// (Go nil slice marshals to JSON null), so a nil return is the contract for
// the unwired-or-empty case.
func Test_12_1_INTG_002_ConsumeNilGuard(t *testing.T) {
	out, err := runHarness(t, map[string]string{
		"harness_test.go": serviceHarnessPreambleNoQueue + `
func TestConsumeNilGuard(t *testing.T) {
	svc := &pdfservice.PDFService{} // no SetPendingOpens call
	got := svc.ConsumePendingOpenFiles()
	if got != nil {
		t.Fatalf("[P1] 12.1-INTG-002: unwired ConsumePendingOpenFiles must return nil, got %#v", got)
	}
}
`,
	})
	if err != nil {
		t.Fatalf("[P1] 12.1-INTG-002 red phase (expected until the nil-guarded method lands):\n%s", out)
	}
}
