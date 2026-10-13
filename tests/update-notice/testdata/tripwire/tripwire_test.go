// Package harness confirms that a live check from the production checker is
// routed through HTTP(S)_PROXY, so the suite's tripwire both blocks and counts it.
package harness

import (
	"context"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/updatecheck"
)

func TestLiveCheckGoesThroughTheProxy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := updatecheck.New().Check(ctx, "0.4.0"); err == nil {
		t.Fatal("a live check through a refusing proxy must fail")
	}
}
