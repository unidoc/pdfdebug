package update_notice_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSharedCacheRecordContract runs the cache harness: path resolution without
// side effects, load-as-absent for every broken record, atomic private writes,
// canonical versions, freshness around the TTL, and per-consumer notice policy.
func TestSharedCacheRecordContract(t *testing.T) {
	tw := newTripwire(t)
	xdg := t.TempDir()
	out, err := runHarness(t, "cache", merge(tw.env(), map[string]string{"XDG_CACHE_HOME": xdg}), "")
	if err != nil {
		t.Fatalf("shared cache harness failed: %v\n%s", err, out)
	}
	if n := tw.count(); n != 0 {
		t.Errorf("cache harness made %d outbound requests, want 0", n)
	}
}

// TestBoundedSingleReleasePageRefresh runs the refresh harness against a local
// httptest server: one request, stable tags only, and the caller's deadline.
func TestBoundedSingleReleasePageRefresh(t *testing.T) {
	tw := newTripwire(t)
	out, err := runHarness(t, "refresh", tw.env(), "")
	if err != nil {
		t.Fatalf("bounded refresh harness failed: %v\n%s", err, out)
	}
	if n := tw.count(); n != 0 {
		t.Errorf("refresh harness made %d requests through the proxy, want 0", n)
	}
}

// TestGUIServiceUsesTheSharedCache runs each service scenario in its own
// subprocess with a fresh cache base and tripwire, then checks whether a live
// request was attempted.
func TestGUIServiceUsesTheSharedCache(t *testing.T) {
	cases := []struct {
		test         string
		wantRequest  bool
		anyRequest   bool
		unusableBase bool
	}{
		{test: "TestServiceStartupFreshAndCurrentMakesNoRequest", wantRequest: false},
		{test: "TestServiceStartupFreshButNewerGoesLive", wantRequest: true},
		{test: "TestServiceStartupStaleGoesLiveAndKeepsLastKnownGood", wantRequest: true},
		{test: "TestServiceExplicitCheckStaysLiveAndLeavesTheRecordOnFailure", wantRequest: true},
		{test: "TestServiceDevBuildTouchesNothing", wantRequest: false},
		{test: "TestServiceUnusableCacheLeavesChecksWorking", anyRequest: true, unusableBase: true},
	}
	for _, c := range cases {
		t.Run(c.test, func(t *testing.T) {
			tw := newTripwire(t)
			base := t.TempDir()
			if c.unusableBase {
				base = filepath.Join(base, "not-a-directory")
				if err := os.WriteFile(base, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			out, err := runHarness(t, "service", merge(tw.env(), map[string]string{"XDG_CACHE_HOME": base}), "^"+c.test+"$")
			if err != nil {
				t.Fatalf("service harness failed: %v\n%s", err, out)
			}
			if c.anyRequest {
				return
			}
			n := tw.count()
			if c.wantRequest && n == 0 {
				t.Errorf("expected a live check to be attempted, tripwire saw no request")
			}
			if !c.wantRequest && n != 0 {
				t.Errorf("expected no outbound request, tripwire saw %d", n)
			}
		})
	}
}

// TestTripwireSeesLiveChecks proves the zero-request assertions elsewhere mean
// something: the production checker's live request reaches the tripwire and is
// refused there instead of leaving the machine.
func TestTripwireSeesLiveChecks(t *testing.T) {
	tw := newTripwire(t)
	out, err := runHarness(t, "tripwire", tw.env(), "")
	if err != nil {
		t.Fatalf("tripwire harness failed: %v\n%s", err, out)
	}
	if tw.count() == 0 {
		t.Error("a live check did not go through the proxy, so the tripwire cannot guard this suite")
	}
}
