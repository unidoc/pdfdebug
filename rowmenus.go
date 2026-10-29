package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Native row context menus. The frontend picks a menu for the clicked row with
// the --custom-contextmenu CSS property and passes a token for that row as the
// menu data. A clicked item sends its action and the token back on
// eventRowMenuAction; the frontend acts on the row the token names.
const (
	rowMenuReveal      = "row-reveal"
	rowMenuTree        = "tree-row"
	rowMenuTreeWithRef = "tree-row-ref"

	rowActionReveal   = "reveal"
	rowActionCopyRef  = "copy-ref"
	rowActionCopyPath = "copy-path"
)

// RowMenuAction is the data of a row menu click: the item's action and the
// menu data the frontend opened the menu with.
type RowMenuAction struct {
	Action string `json:"action"`
	Token  string `json:"token"`
}

type rowMenuItem struct {
	label  string
	action string
}

// rowMenus lists each row menu's items in display order.
var rowMenus = map[string][]rowMenuItem{
	rowMenuReveal: {
		{"Show node in tree", rowActionReveal},
	},
	rowMenuTree: {
		{"Copy path", rowActionCopyPath},
	},
	rowMenuTreeWithRef: {
		{"Copy object reference", rowActionCopyRef},
		{"Copy path", rowActionCopyPath},
	},
}

// newRowMenus builds the row menus; each item click calls emit.
func newRowMenus(emit func(RowMenuAction)) map[string]*application.Menu {
	menus := make(map[string]*application.Menu, len(rowMenus))
	for name, items := range rowMenus {
		menu := application.NewMenu()
		for _, item := range items {
			action := item.action
			menu.Add(item.label).OnClick(func(ctx *application.Context) {
				emit(RowMenuAction{Action: action, Token: ctx.ContextMenuData()})
			})
		}
		menus[name] = menu
	}
	return menus
}

// registerRowMenus registers the row menus with app by name.
func registerRowMenus(app *application.App) {
	for name, menu := range newRowMenus(func(a RowMenuAction) {
		app.Event.Emit(eventRowMenuAction, a)
	}) {
		app.ContextMenu.Add(name, &application.ContextMenu{Menu: menu})
	}
}
