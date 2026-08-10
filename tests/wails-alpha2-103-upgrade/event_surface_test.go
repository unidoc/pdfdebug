// AC 8, 11: live-event-surface preservation across the bump, plus the
// pre-flight `native:*` invariant.
//
// alpha2.103 reorganized the event system (the documented breaking change is the
// native:* -> common:* deprecation, mobile-only). Our events are all custom
// namespaces; this suite pins that the Go-emit / JS-consume event names survive
// the bump and that no native:* prefix is introduced.
//
// These read main.go the same way the GRANDFATHERED 10-3 event assertions do
// (Test_10_3_STRUCT_030/032/060). They are NOT new content-grep tests of a
// guarded file in the source-grep-guard sense: the guard forbids NEW greps of
// main.go/MainLayout.tsx/EmptyState.tsx; the 10-3 main.go event greps are
// grandfathered and 12-3 reuses that exact established pattern for the same
// upgrade-audit purpose.
package story_12_3_wails_alpha2_103_upgrade_test

import (
	"fmt"
	"strings"
	"testing"
)

// goEmittedEvents are the events main.go emits to the frontend via
// app.Event.Emit. A bump that forces a call-site edit (e.g. an events.Common.*
// constant rename in the alpha2 reorg) and drops one of these fails loud here.
var goEmittedEvents = []string{
	"document:opened",
	"document:error",
	"document:batch-start",
	"document:batch-complete",
	"navigate:back",
	"navigate:forward",
	"navigate:goToPage",
	"palette:open",
	"tab:next",
	"tab:prev",
	"splash:dismiss",
	"splash:dismissed",
	"splash:timeout",
}

// Test_12_3_INTG_060_GoEventEmitNamesPreserved [P0] AC8, AC11: every Go-emitted
// event name still appears in main.go after the bump.
func Test_12_3_INTG_060_GoEventEmitNamesPreserved(t *testing.T) {
	src := readSource(t, "main.go")
	for _, name := range goEmittedEvents {
		if !strings.Contains(src, `"`+name+`"`) {
			t.Errorf("[P0] 12.3-INTG-060: main.go must still emit event %q -- the alpha2 event reorg must not drop a custom-namespace emit (AC8/AC11)", name)
		}
	}
}

// jsConsumedEvents are the events the frontend subscribes to via Events.On.
// The common:Window* names are emitted by the Wails runtime itself; an alpha2
// runtime-side rename would silently break window geometry persistence.
var jsConsumedEvents = []string{
	"document:opened",
	"document:error",
	"document:warning",
	"document:batch-start",
	"document:batch-complete",
	"navigate:back",
	"navigate:forward",
	"navigate:goToPage",
	"palette:open",
	"tab:next",
	"tab:prev",
	"splash:dismissed",
	"common:WindowDidMove",
	"common:WindowDidResize",
}

// Test_12_3_INTG_061_JsEventOnNamesPreserved [P0] AC8, AC11: every event the
// frontend listens for still appears as an Events.On('<name>', ...) SUBSCRIPTION
// call under frontend/src (non-test files only). Matching the call form
// (Events.On('name' / Events.On("name") rather than a bare quoted literal --
// same pattern INTG-062 uses -- avoids the brittle whole-tree-grep failure mode
// (project memory project_struct_grep_tests_brittle.md): a bare literal can
// false-pass on a match in a comment or unrelated string. Every consumed event
// is verified subscribed via this call form (App.jsx, main.jsx, TabBar.tsx).
func Test_12_3_INTG_061_JsEventOnNamesPreserved(t *testing.T) {
	src := loadFrontendSrcConcat(t)
	for _, name := range jsConsumedEvents {
		needle1 := fmt.Sprintf(`Events.On('%s'`, name)
		needle2 := fmt.Sprintf(`Events.On("%s"`, name)
		if !strings.Contains(src, needle1) && !strings.Contains(src, needle2) {
			t.Errorf("[P0] 12.3-INTG-061: frontend/src must still Events.On(%q, ...) -- an alpha2 runtime rename of this event silently breaks the consumer (AC8/AC11)", name)
		}
	}
}

// Test_12_3_INTG_062_JsToGoEventContract [P1] AC11: the inbound
// document:batch-cancel contract holds on both sides -- frontend Events.Emit and
// a main.go app.Event.On listener.
func Test_12_3_INTG_062_JsToGoEventContract(t *testing.T) {
	const name = "document:batch-cancel"
	mainSrc := readSource(t, "main.go")
	frontSrc := loadFrontendSrcConcat(t)

	jsNeedle1 := fmt.Sprintf(`Events.Emit('%s'`, name)
	jsNeedle2 := fmt.Sprintf(`Events.Emit("%s"`, name)
	if !strings.Contains(frontSrc, jsNeedle1) && !strings.Contains(frontSrc, jsNeedle2) {
		t.Errorf("[P1] 12.3-INTG-062: frontend must still Events.Emit(%q, ...) (AC11)", name)
	}
	goNeedle := fmt.Sprintf(`app.Event.On("%s"`, name)
	if !strings.Contains(mainSrc, goNeedle) {
		t.Errorf("[P1] 12.3-INTG-062: main.go must still app.Event.On(%q, ...) (AC11: JS->Go contract)", name)
	}
}

// Test_12_3_INTG_063_NoNativeEventPrefix [P1] AC8 (pre-flight invariant): the
// bump must not introduce the deprecated `native:` event prefix anywhere in
// main.go or frontend/src. The story's pre-flight verified ZERO native:* usage;
// this pins that the alpha2 upgrade does not regress that (the one documented
// alpha2.103 breaking change does not reach us).
func Test_12_3_INTG_063_NoNativeEventPrefix(t *testing.T) {
	for _, lit := range []string{`"native:`, `'native:`} {
		if scanRepoFor(t, lit) {
			t.Errorf("[P1] 12.3-INTG-063: found a %s event literal in main.go or frontend/src -- alpha2.103 deprecated the native:* prefix (-> common:*); the pre-flight verified we use ZERO native:* events, do not introduce one (AC8)", lit)
		}
	}
}
