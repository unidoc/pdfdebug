package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"unidoc-pdf-debugger/internal/filewatch"
)

// Custom event names shared by the Go side and the frontend. Each one is
// registered below, so the binding generator emits a typed constant for it in
// frontend/bindings/events and Wails checks the data type on every emit.
const (
	eventDocumentLoadStart     = "document:load-start"
	eventDocumentOpened        = "document:opened"
	eventDocumentError         = "document:error"
	eventDocumentDiskStatus    = "document:disk-status"
	eventDocumentBatchStart    = "document:batch-start"
	eventDocumentBatchComplete = "document:batch-complete"
	eventDocumentBatchCancel   = "document:batch-cancel"
	eventDocumentCloseActive   = "document:close-active"
	eventDocumentReloadActive  = "document:reload-active"
	eventDocumentCompareActive = "document:compare-active"
	eventNavigateBack          = "navigate:back"
	eventNavigateForward       = "navigate:forward"
	eventNavigateGoToPage      = "navigate:goToPage"
	eventNavigateStateChanged  = "navigate:state-changed"
	eventPaletteOpen           = "palette:open"
	eventRowMenuAction         = "row-menu:action"
	eventTabNext               = "tab:next"
	eventTabPrev               = "tab:prev"
	eventUpdateCheckRequested  = "update:check-requested"
	eventSplashDismiss         = "splash:dismiss"
	eventSplashDismissed       = "splash:dismissed"
	eventSplashTimeout         = "splash:timeout"
)

// init registers every app event with the data type its emitters send: Void for
// events sent with nil, the exact Go type otherwise. The registration calls use
// constant names so the binding generator can discover them.
func init() {
	application.RegisterEvent[map[string]any](eventDocumentLoadStart)
	application.RegisterEvent[map[string]any](eventDocumentOpened)
	application.RegisterEvent[map[string]any](eventDocumentError)
	application.RegisterEvent[filewatch.Event](eventDocumentDiskStatus)
	application.RegisterEvent[map[string]any](eventDocumentBatchStart)
	application.RegisterEvent[application.Void](eventDocumentBatchComplete)
	application.RegisterEvent[application.Void](eventDocumentBatchCancel)
	application.RegisterEvent[application.Void](eventDocumentCloseActive)
	application.RegisterEvent[application.Void](eventDocumentReloadActive)
	application.RegisterEvent[application.Void](eventDocumentCompareActive)
	application.RegisterEvent[application.Void](eventNavigateBack)
	application.RegisterEvent[application.Void](eventNavigateForward)
	application.RegisterEvent[application.Void](eventNavigateGoToPage)
	application.RegisterEvent[map[string]any](eventNavigateStateChanged)
	application.RegisterEvent[application.Void](eventPaletteOpen)
	application.RegisterEvent[RowMenuAction](eventRowMenuAction)
	application.RegisterEvent[application.Void](eventTabNext)
	application.RegisterEvent[application.Void](eventTabPrev)
	application.RegisterEvent[application.Void](eventUpdateCheckRequested)
	application.RegisterEvent[application.Void](eventSplashDismiss)
	application.RegisterEvent[application.Void](eventSplashDismissed)
	application.RegisterEvent[application.Void](eventSplashTimeout)
}
