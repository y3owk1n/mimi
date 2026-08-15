package service

import (
	"strconv"
	"strings"
)

// OptionalInt is a number launchctl either reported or did not. Both numbers a
// status carries need one: the pid is absent whenever the job is not running,
// and an exit status cannot use zero as its "no value" — zero is a clean exit,
// which is exactly the case a caller wants told apart from a crash.
type OptionalInt struct {
	// Value is the number launchctl reported. It means nothing unless Known.
	Value int
	// Known reports whether launchctl gave this number at all.
	Known bool
}

// jobReport is what `launchctl print` says about a loaded job, reduced to the
// two facts that separate a healthy daemon from one launchd keeps respawning.
type jobReport struct {
	// pid is the process id of the running job, unknown when it is not
	// running.
	pid OptionalInt
	// lastExitStatus is how the job last exited, unknown before it ever has.
	lastExitStatus OptionalInt
}

// parseJobReport reads pid and last exit status out of `launchctl print`
// output.
//
// That output is semi-structured text Apple documents nowhere and promises
// nothing about, so every line it does not recognize is skipped and an
// unrecognizable report simply comes back with nothing known. A status that
// says less is the intended failure: the command still answers whether the
// service is loaded, which is all it ever answered before.
func parseJobReport(output string) jobReport {
	var (
		report  jobReport
		running bool
	)

	for line := range strings.SplitSeq(output, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), " = ")
		if !found {
			continue
		}

		// The first occurrence a number could be read from wins. launchctl
		// prints the job's own fields before the nested blocks (endpoints,
		// coalitions) that could repeat a key with an unrelated meaning.
		switch key {
		case "state":
			running = running || value == "running"
		case "pid":
			if !report.pid.Known {
				report.pid = parseLeadingInt(value)
			}
		case "last exit code":
			if !report.lastExitStatus.Known {
				report.lastExitStatus = parseLeadingInt(value)
			}
		}
	}

	// launchd prints a last exit code for a running job too, where it is how
	// the job exited the previous time round rather than a reason it is down.
	// Only the pid distinguishes the two, so a job launchd calls running whose
	// pid could not be read is output this could not parse — and reporting the
	// exit status anyway would tell a user the daemon is down moments after
	// launchd said it is up.
	if running && !report.pid.Known {
		report.lastExitStatus = OptionalInt{}
	}

	return report
}

// parseLeadingInt reads the number a launchctl value starts with. The value is
// not always only a number: an exit status is printed as "78: EX_CONFIG" when
// launchd has a name for it, and as "(never exited)" when there is no number
// there at all.
func parseLeadingInt(value string) OptionalInt {
	number, _, _ := strings.Cut(value, ":")

	parsed, err := strconv.Atoi(strings.TrimSpace(number))
	if err != nil {
		return OptionalInt{}
	}

	return OptionalInt{Value: parsed, Known: true}
}
