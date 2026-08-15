//nolint:testpackage // execLauncher is unexported; the seam under test is intentionally internal.
package service

import (
	"context"
	"testing"
)

// TestExecLauncher_List_ReportsALaunchctlThatCouldNotRun covers half of the
// distinction the rest of this package is built on, in the one place it is
// actually drawn: everywhere else a fake launcher supplies the three states
// directly, and here a failed `launchctl list` has to be read as two different
// things. The other half — launchctl running and reporting no such job — needs
// a real launchctl, and lives in the integration tier.
func TestExecLauncher_List_ReportsALaunchctlThatCouldNotRun(t *testing.T) {
	// No PATH, so there is no launchctl to run and nothing ever asks launchd
	// anything. Answering "not loaded" here is the bug this whole change is
	// about.
	t.Setenv("PATH", "")

	loaded, err := execLauncher{}.list(context.Background(), Label)
	if err == nil {
		t.Fatal("list() = nil, want the failure to run launchctl at all")
	}

	if loaded {
		t.Error("list() reported a job loaded on a launchctl that never ran")
	}
}
