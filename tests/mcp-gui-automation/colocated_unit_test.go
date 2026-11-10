package mcp_gui_automation_test

import "testing"

func TestMCPClientUnitTests(t *testing.T) {
	out, err := runGo(t, nil, "test", "-race", "-count=1", "./internal/mcpclient/")
	if err != nil {
		t.Fatalf("internal/mcpclient unit tests failed:\n%s", out)
	}
}

func TestGuictlUnitTests(t *testing.T) {
	out, err := runGo(t, nil, "test", "-race", "-count=1", "./tools/guictl/")
	if err != nil {
		t.Fatalf("tools/guictl unit tests failed:\n%s", out)
	}
}

func TestDialogQueueUnitTestsWithTheMCPTag(t *testing.T) {
	out, err := runGo(t, nil, "test", "-tags", "mcp", "-race", "-count=1", "-run", "DialogQueue", "./internal/pdfservice/")
	if err != nil {
		t.Fatalf("internal/pdfservice dialog queue tests (mcp build) failed:\n%s", out)
	}
}

func TestDialogQueueUnitTestsWithoutTheMCPTag(t *testing.T) {
	out, err := runGo(t, nil, "test", "-race", "-count=1", "-run", "DialogQueue", "./internal/pdfservice/")
	if err != nil {
		t.Fatalf("internal/pdfservice dialog queue tests (default build) failed:\n%s", out)
	}
}

func TestInstanceIdentityUnitTestsWithoutTheMCPTag(t *testing.T) {
	out, err := runGo(t, nil, "test", "-count=1", "-run", "InstanceIdentity", ".")
	if err != nil {
		t.Fatalf("main package instance identity tests (default build) failed:\n%s", out)
	}
}

func TestInstanceIdentityUnitTestsWithTheMCPTag(t *testing.T) {
	out, err := runGo(t, nil, "test", "-tags", "mcp", "-count=1", "-run", "InstanceIdentity|DesktopEntry", ".")
	if err != nil {
		t.Fatalf("main package instance identity tests (mcp build) failed:\n%s", out)
	}
}
