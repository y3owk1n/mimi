package service

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

// AgentForPID is the label of the launchd job running pid in the user's
// domain, "" when no job holds it or launchctl could not be asked. It is
// how mimi doctor names a daemon another installer's agent runs, the
// home-manager module say, when mimi's own service is not loaded.
func AgentForPID(ctx context.Context, pid int) string {
	out, err := exec.CommandContext(ctx, "launchctl", "list").Output()
	if err != nil {
		return ""
	}

	return agentForPID(string(out), pid)
}

// agentForPID reads `launchctl list` output, one job per line as pid, last
// exit status, and label, for the label whose pid is pid.
func agentForPID(listing string, pid int) string {
	want := strconv.Itoa(pid)

	for line := range strings.Lines(listing) {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == want {
			return fields[2]
		}
	}

	return ""
}
