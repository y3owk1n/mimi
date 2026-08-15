//nolint:testpackage // parseJobReport is unexported; the seam under test is intentionally internal.
package service

import "testing"

// runningPrint is `launchctl print gui/501/com.y3owk1n.mimi` for a healthy
// daemon, trimmed to the lines around the ones that are read. It is real
// output, kept verbatim down to the tabs, because the parser's whole job is to
// survive the shape launchctl actually prints.
const runningPrint = `gui/501/com.y3owk1n.mimi = {
	active count = 1
	path = /Users/test/Library/LaunchAgents/com.y3owk1n.mimi.plist
	type = LaunchAgent
	state = running

	program = /usr/local/bin/mimi
	runs = 1
	pid = 1478
	immediate reason = speculative
	last exit code = (never exited)
}`

// respawningPrint is the same command against the daemon this whole feature
// exists for: one that crashes at startup and that KeepAlive schedules again
// ten seconds later, forever. It is loaded, it has no pid, and the only thing
// that says so is the exit status.
const respawningPrint = `gui/501/com.y3owk1n.mimi = {
	active count = 0
	path = /Users/test/Library/LaunchAgents/com.y3owk1n.mimi.plist
	type = LaunchAgent
	state = spawn scheduled

	program = /usr/local/bin/mimi
	runs = 17169
	last exit code = 1
}`

func TestParseJobReport(t *testing.T) {
	tests := []struct {
		name               string
		output             string
		wantPID            OptionalInt
		wantLastExitStatus OptionalInt
	}{
		{
			name:    "a running job reports its pid",
			output:  runningPrint,
			wantPID: OptionalInt{Value: 1478, Known: true},
			// "(never exited)" is not a number, and pretending it is zero
			// would report a clean exit that never happened.
			wantLastExitStatus: OptionalInt{},
		},
		{
			name:               "a respawning job reports how it last exited",
			output:             respawningPrint,
			wantLastExitStatus: OptionalInt{Value: 1, Known: true},
		},
		{
			name:               "an exit status launchd has a name for keeps its number",
			output:             "\tlast exit code = 78: EX_CONFIG\n",
			wantLastExitStatus: OptionalInt{Value: 78, Known: true},
		},
		{
			name:               "a clean exit is told apart from no exit at all",
			output:             "\tlast exit code = 0\n",
			wantLastExitStatus: OptionalInt{Value: 0, Known: true},
		},
		{
			// launchd prints a name and no number when the job was killed by
			// a signal rather than exiting.
			name:   "an exit with a reason and no code reports neither",
			output: "\tstate = not running\n\tlast exit reason = JETSAM_REASON_MEMORY_IDLE_EXIT\n",
		},
		{
			name:   "output in a shape this parser does not know reports nothing",
			output: "some future launchctl saying something else entirely",
		},
		{
			// launchd prints a last exit code for a running job too — there it
			// is how the job exited the previous time round, not a reason it
			// is down. Only the pid says a job is up, so a job launchd calls
			// running with no pid this could read is unreadable output, and
			// reporting its exit status would assert it is down when launchd
			// just said it is not.
			name:   "a running job with an unreadable pid reports nothing",
			output: "\tstate = running\n\tpid = ?\n\tlast exit code = 1\n",
		},
		{
			name:               "a running job that exited before still reports its pid",
			output:             "\tstate = running\n\tpid = 1478\n\tlast exit code = 1\n",
			wantPID:            OptionalInt{Value: 1478, Known: true},
			wantLastExitStatus: OptionalInt{Value: 1, Known: true},
		},
		{
			name:   "empty output reports nothing",
			output: "",
		},
		{
			// The nested blocks launchctl prints after the job's own fields
			// carry keys of their own; none may overwrite what was read.
			name: "a nested block does not overwrite the job's own fields",
			output: "\tpid = 1478\n\tpid-local endpoints = {\n" +
				"\t\t\"com.apple.axserver\" = {\n\t\t\tpid = 99\n\t\t}\n\t}\n",
			wantPID: OptionalInt{Value: 1478, Known: true},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := parseJobReport(testCase.output)

			if got.pid != testCase.wantPID {
				t.Errorf("parseJobReport() pid = %+v, want %+v", got.pid, testCase.wantPID)
			}

			if got.lastExitStatus != testCase.wantLastExitStatus {
				t.Errorf(
					"parseJobReport() lastExitStatus = %+v, want %+v",
					got.lastExitStatus,
					testCase.wantLastExitStatus,
				)
			}
		})
	}
}
