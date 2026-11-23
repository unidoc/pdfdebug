package boot_smoke_test

import "testing"

// The launch context must outlast the probes, the shutdown wait and the hang
// report; otherwise it kills an app that hangs on quit first and the test
// reads the kill as a clean signalled exit.
func TestProcessDeadlineOutlastsTheHangReport(t *testing.T) {
	need := probeBudget + shutdownWait() + hangReportWait
	if got := processDeadline(); got <= need {
		t.Errorf("processDeadline() = %s, want more than probes + shutdown wait + hang report = %s", got, need)
	}
}
