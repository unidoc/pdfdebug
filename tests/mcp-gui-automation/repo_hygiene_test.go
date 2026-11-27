package mcp_gui_automation_test

import (
	"testing"
	"time"
)

func TestGuictlStateDirectoryIsIgnored(t *testing.T) {
	requireTool(t, "git")
	for _, p := range []string{".guictl/state.json", ".guictl/app.log", ".guictl/config/pdfdebug/recent.json"} {
		if out, err := runInRoot(t, time.Minute, nil, "git", "check-ignore", "-q", p); err != nil {
			t.Errorf("%s is not git-ignored (%v) %s", p, err, out)
		}
	}
}
