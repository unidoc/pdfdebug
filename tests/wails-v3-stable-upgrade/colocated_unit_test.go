package wails_v3_stable_upgrade_test

import "testing"

func TestRecentFilesListUnitTests(t *testing.T) {
	out, err := runGoTest(t, "-race", "-count=1", "./internal/recentfiles/")
	if err != nil {
		t.Fatalf("internal/recentfiles unit tests failed:\n%s", out)
	}
}

func TestEmbeddedSaveAndRecordRecentServiceUnitTests(t *testing.T) {
	out, err := runGoTest(t, "-race", "-count=1",
		"-run", "SaveEmbeddedFile|BackendEmbeddedSave|RecordRecentFile|OpenFileDoesNotRecord",
		"./internal/pdfservice/")
	if err != nil {
		t.Fatalf("internal/pdfservice embedded-save and recent-file tests failed:\n%s", out)
	}
}

func TestEventRegistrationUnitTests(t *testing.T) {
	out, err := runGoTest(t, "-count=1",
		"-run", "EventsAreRegistered", ".")
	if err != nil {
		t.Fatalf("main package event registration tests failed:\n%s", out)
	}
}
