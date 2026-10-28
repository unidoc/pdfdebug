package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"unidoc-pdf-debugger/internal/filewatch"
)

// eventProbe is the data type used to probe whether a name is registered.
type eventProbe struct{}

// registeredDataType reports the data type a custom event was registered with.
// RegisterEvent panics on a duplicate name and names the existing type, so a
// second registration attempt reveals it.
func registeredDataType(t *testing.T, name string) (typ string, registered bool) {
	t.Helper()
	const marker = "already registered with data type "
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		msg := fmt.Sprint(r)
		i := strings.Index(msg, marker)
		if i < 0 {
			t.Fatalf("registering %q panicked for another reason: %v", name, r)
		}
		typ, registered = msg[i+len(marker):], true
	}()
	application.RegisterEvent[eventProbe](name)
	return "", false
}

// Events emitted with no data, from Go or from the frontend.
var payloadlessEvents = []string{
	"document:batch-complete",
	"document:close-active",
	"document:reload-active",
	"document:compare-active",
	"document:batch-cancel",
	"navigate:back",
	"navigate:forward",
	"navigate:goToPage",
	"palette:open",
	"tab:next",
	"tab:prev",
	"update:check-requested",
	"splash:dismiss",
	"splash:dismissed",
	"splash:timeout",
}

// Events that carry a payload, with the exact type their emitters send. Wails
// cancels an emit whose data type differs from the registered one.
var payloadEvents = map[string]string{
	"document:load-start":      reflect.TypeFor[map[string]any]().String(),
	"document:opened":          reflect.TypeFor[map[string]any]().String(),
	"document:error":           reflect.TypeFor[map[string]any]().String(),
	"document:disk-status":     reflect.TypeFor[filewatch.Event]().String(),
	"document:batch-start":     reflect.TypeFor[map[string]any]().String(),
	"update:download-progress": reflect.TypeFor[map[string]any]().String(),
	"navigate:state-changed":   reflect.TypeFor[map[string]any]().String(),
}

func TestPayloadlessEventsAreRegisteredAsVoid(t *testing.T) {
	for _, name := range payloadlessEvents {
		t.Run(name, func(t *testing.T) {
			typ, ok := registeredDataType(t, name)
			if !ok {
				t.Fatalf("event %q is not registered with application.RegisterEvent", name)
			}
			if typ != "application.Void" {
				t.Fatalf("event %q registered with %s, want application.Void", name, typ)
			}
		})
	}
}

func TestPayloadEventsAreRegisteredWithTheEmittedDataType(t *testing.T) {
	for name, want := range payloadEvents {
		t.Run(name, func(t *testing.T) {
			typ, ok := registeredDataType(t, name)
			if !ok {
				t.Fatalf("event %q is not registered with application.RegisterEvent", name)
			}
			if typ != want {
				t.Fatalf("event %q registered with %s, want %s (the type its emitters send)", name, typ, want)
			}
		})
	}
}
