//go:build mcp

package main

import "testing"

func TestInstanceIdentityMCPBuildIsASeparateInstance(t *testing.T) {
	if want := "com.unidoc.unidoc-pdf-debugger.mcp"; instanceID != want {
		t.Errorf("instanceID = %q, want %q", instanceID, want)
	}
	if instanceID == appID {
		t.Error("instanceID equals appID, so the agent build would share the production single-instance lock")
	}
	if want := "UniDoc PDF Debugger [agent]"; windowTitle != want {
		t.Errorf("windowTitle = %q, want %q", windowTitle, want)
	}
}

func TestInstanceIdentityMCPBuildKeepsTheProductConstants(t *testing.T) {
	if appID != "com.unidoc.unidoc-pdf-debugger" {
		t.Errorf("appID = %q; the mcp build must not change the constant the .desktop file matches", appID)
	}
	if appName != "UniDoc PDF Debugger" {
		t.Errorf("appName = %q; the mcp build must not change the menu label constant", appName)
	}
}
