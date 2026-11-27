package main

import (
	"reflect"
	"testing"
)

func TestRowMenusListTheirItemsInOrder(t *testing.T) {
	want := map[string][]string{
		rowMenuReveal:      {"Show node in tree"},
		rowMenuTree:        {"Copy path"},
		rowMenuTreeWithRef: {"Copy object reference", "Copy path"},
	}
	menus := newRowMenus(func(RowMenuAction) {})
	if len(menus) != len(want) {
		t.Fatalf("got %d row menus, want %d", len(menus), len(want))
	}
	for name, labels := range want {
		menu, ok := menus[name]
		if !ok {
			t.Fatalf("row menu %q is missing", name)
		}
		var got []string
		for i := 0; menu.ItemAt(i) != nil; i++ {
			got = append(got, menu.ItemAt(i).Label())
		}
		if !reflect.DeepEqual(got, labels) {
			t.Errorf("row menu %q items = %v, want %v", name, got, labels)
		}
	}
}

func TestRowMenuActionsAreDistinctPerMenu(t *testing.T) {
	for name, items := range rowMenus {
		seen := map[string]bool{}
		for _, item := range items {
			if item.action == "" {
				t.Errorf("row menu %q item %q has no action", name, item.label)
			}
			if seen[item.action] {
				t.Errorf("row menu %q repeats action %q", name, item.action)
			}
			seen[item.action] = true
		}
	}
}
