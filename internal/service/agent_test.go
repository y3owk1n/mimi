//nolint:testpackage // agentForPID is unexported; the seam under test is intentionally internal.
package service

import "testing"

func TestAgentForPID_FindsTheLabelHoldingThePID(t *testing.T) {
	listing := "PID\tStatus\tLabel\n4774\t0\torg.nix-community.home.mimi\n-\t0\tcom.apple.other\n"

	if got := agentForPID(listing, 4774); got != "org.nix-community.home.mimi" {
		t.Fatalf("got %q", got)
	}

	if got := agentForPID(listing, 1); got != "" {
		t.Fatalf("a pid no job holds gave %q", got)
	}
}
