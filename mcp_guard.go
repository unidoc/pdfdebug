//go:build production && mcp

package main

// The mcp tag compiles the Wails MCP automation server in. A production build
// must never carry it, so this file refers to an undefined identifier and the
// build fails with its name.
var _ = mcpTagMustNotBeUsedInProductionBuilds
