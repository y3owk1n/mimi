//go:build integration

//nolint:testpackage // execLauncher is unexported; the seam under test is intentionally internal.
package service

import (
	"context"
	"testing"
)

// TestExecLauncher_List_ReportsAnAbsentJob is the other half of
// [TestExecLauncher_List_ReportsALaunchctlThatCouldNotRun], and it takes the
// real launchctl: only launchd can produce the exit status that means "no such
// job", which this must read as an answer rather than as a failure to ask.
func TestExecLauncher_List_ReportsAnAbsentJob(t *testing.T) {
	// A label no launchd domain can be holding, rather than mimi's own: the
	// machine running this may well have mimi installed.
	loaded, err := execLauncher{}.list(
		context.Background(),
		Label+".test-label-that-is-not-installed",
	)
	if err != nil {
		t.Fatalf("list() = %v, want a launchctl that ran and said no", err)
	}

	if loaded {
		t.Error("list() reported a label nothing could have loaded as loaded")
	}
}
