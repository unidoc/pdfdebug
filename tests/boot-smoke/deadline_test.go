package boot_smoke_test

import (
	"testing"
	"time"
)

// The gdb bound and the SIGQUIT wait fit inside the hang-report budget, so
// the launch context does not kill the app while gdb is attached.
func TestHangReportWaitCoversGdbAndGoStacks(t *testing.T) {
	need := gdbTimeout + gdbKillAfter + goStacksWait
	if hangReportWait <= need {
		t.Errorf("hangReportWait = %s, want more than gdb timeout + kill-after + SIGQUIT wait = %s", hangReportWait, need)
	}
}

// The timeout duration keeps sub-second parts, since timeout reads 0 as no
// limit.
func TestTimeoutSecondsKeepsFractions(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{20 * time.Second, "20"},
		{1500 * time.Millisecond, "1.5"},
		{250 * time.Millisecond, "0.25"},
	} {
		if got := timeoutSeconds(tc.d); got != tc.want {
			t.Errorf("timeoutSeconds(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
