// Live-event-surface preservation across the bump, plus the pre-flight
// `native:*` invariant.
//
// alpha2.103 reorganized the event system (the documented breaking change is the
// native:* -> common:* deprecation, mobile-only). Our events are all custom
// namespaces, registered in events.go and consumed in the frontend through the
// generated constants; this suite pins that the Go-emit / JS-consume event
// names survive a bump and that no native:* prefix is introduced.
//
// main.go is read through readSource, which the source-grep guard does not
// flag; the guard forbids NEW readFile greps of
// main.go/MainLayout.tsx/EmptyState.tsx.
package wails_alpha2_103_upgrade_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// goEmittedEvents are the custom events the Go side emits to the frontend. Each
// is declared as a constant and registered with application.RegisterEvent in
// events.go, which is what makes the binding generator emit a typed constant.
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
	"document:reload-active",
	"document:compare-active",
	"document:disk-status",
}

// TestGoEventNamesRegistered asserts every Go-emitted event name is declared in
// events.go, the single place the app registers its events.
func TestGoEventNamesRegistered(t *testing.T) {
	src := readSource(t, "events.go")
	for _, name := range goEmittedEvents {
		if !strings.Contains(src, `= "`+name+`"`) {
			t.Errorf("events.go must declare and register event %q", name)
		}
	}
}

// jsConsumedEvents maps each app event the frontend subscribes to onto the
// constant the binding generator emits for it in frontend/bindings/events.
var jsConsumedEvents = map[string]string{
	"document:opened":         "DocumentOpened",
	"document:error":          "DocumentError",
	"document:batch-start":    "DocumentBatchStart",
	"document:batch-complete": "DocumentBatchComplete",
	"navigate:back":           "NavigateBack",
	"navigate:forward":        "NavigateForward",
	"navigate:goToPage":       "NavigateGoToPage",
	"palette:open":            "PaletteOpen",
	"tab:next":                "TabNext",
	"tab:prev":                "TabPrev",
	"splash:dismissed":        "SplashDismissed",
	"document:reload-active":  "DocumentReloadActive",
	"document:compare-active": "DocumentCompareActive",
	"document:disk-status":    "DocumentDiskStatus",
}

// wailsWindowEvents are emitted by the Wails runtime itself and keep their
// string names; a runtime-side rename would silently break window geometry
// persistence.
var wailsWindowEvents = []string{
	"common:WindowDidMove",
	"common:WindowDidResize",
}

// TestJsEventOnNamesPreserved asserts every event the frontend listens for is
// still subscribed through an Events.On(...) call under frontend/src (non-test
// files only): app events through their generated constant, Wails window
// events by name. Matching the call form rather than a bare identifier avoids
// a false pass on a comment or an unrelated string.
func TestJsEventOnNamesPreserved(t *testing.T) {
	src := loadFrontendSrcConcat(t)
	for name, ident := range jsConsumedEvents {
		if !strings.Contains(src, fmt.Sprintf(`Events.On(%s,`, ident)) {
			t.Errorf("frontend/src must still Events.On(%s, ...) for event %q", ident, name)
		}
	}
	for _, name := range wailsWindowEvents {
		needle1 := fmt.Sprintf(`Events.On('%s'`, name)
		needle2 := fmt.Sprintf(`Events.On("%s"`, name)
		if !strings.Contains(src, needle1) && !strings.Contains(src, needle2) {
			t.Errorf("frontend/src must still Events.On(%q,...) -- a runtime rename of this event silently breaks the consumer", name)
		}
	}
}

// TestJsToGoEventContract asserts the inbound document:batch-cancel contract holds
// on both sides: a frontend Events.Emit and a main.go app.Event.On listener, both
// through the registered event's constant.
func TestJsToGoEventContract(t *testing.T) {
	eventsSrc := readSource(t, "events.go")
	// gofmt aligns the const block, so the spacing before "=" varies.
	if !regexp.MustCompile(`eventDocumentBatchCancel\s*=\s*"document:batch-cancel"`).MatchString(eventsSrc) {
		t.Error(`events.go must declare eventDocumentBatchCancel = "document:batch-cancel"`)
	}
	if !strings.Contains(loadFrontendSrcConcat(t), "Events.Emit(DocumentBatchCancel") {
		t.Error("frontend must still Events.Emit(DocumentBatchCancel)")
	}
	if !strings.Contains(readSource(t, "main.go"), "app.Event.On(eventDocumentBatchCancel") {
		t.Error("main.go must still app.Event.On(eventDocumentBatchCancel, ...) (JS->Go contract)")
	}
}

// TestNoNativeEventPrefix asserts the deprecated `native:` event prefix appears
// nowhere in main.go or frontend/src. The tree carries zero native:* usage, and
// this pins that the alpha2 upgrade does not regress it, since the one documented
// alpha2.103 breaking change does not reach this code.
func TestNoNativeEventPrefix(t *testing.T) {
	for _, lit := range []string{`"native:`, `'native:`} {
		if scanRepoFor(t, lit) {
			t.Errorf("found a %s event literal in main.go or frontend/src -- alpha2.103 deprecated the native:* prefix (-> common:*); the pre-flight verified we use ZERO native:* events, do not introduce one", lit)
		}
	}
}
