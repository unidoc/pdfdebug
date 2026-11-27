//go:build !mcp

package main

import "testing"

func TestInstanceIdentityDefaultBuildUsesTheProductIdentity(t *testing.T) {
	if instanceID != appID {
		t.Errorf("instanceID = %q, want appID %q", instanceID, appID)
	}
	if windowTitle != appName {
		t.Errorf("windowTitle = %q, want appName %q", windowTitle, appName)
	}
}
