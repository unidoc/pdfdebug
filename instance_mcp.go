//go:build mcp

package main

// init gives the agent build its own single-instance id, GTK application id
// and window title, so it never shares a lock or a window with the
// production app.
func init() {
	instanceID = appID + ".mcp"
	windowTitle = appName + " [agent]"
}
