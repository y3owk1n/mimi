package service

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
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

// installedCapturedLogs describes the two console streams the installed plist
// captures: where it tells launchd to write each, and how large each has grown.
//
// The paths come from the plist on disk rather than from settings.log_file,
// because the plist is what launchd loaded. A log_file changed since the last
// install has moved the config's answer without moving the service's, and a
// size reported for a file nothing is writing is worse than no size at all.
//
// Every way of not finding them ends the same way: an unreadable plist, one
// that is not a file mimi could have written — home-manager's is a symlink into
// the Nix store — and one whose keys this cannot make sense of all leave both
// paths empty, and the status simply says nothing about them. It is
// the same bargain the rest of the status makes — degrade the detail, keep the
// answer.
func installedCapturedLogs() (CapturedLog, CapturedLog) {
	installed, err := readInstalledPlist(plistPath())
	if err != nil {
		return CapturedLog{}, CapturedLog{}
	}

	return describeCapturedLog(plistString(installed.content, "StandardOutPath")),
		describeCapturedLog(plistString(installed.content, "StandardErrorPath"))
}

// describeCapturedLog sizes one captured stream. A path that is not there is
// reported as absent rather than as an error: launchd creates the file when it
// first spawns the daemon, so a missing one is a service that has never run.
func describeCapturedLog(path string) CapturedLog {
	if path == "" {
		return CapturedLog{}
	}

	info, err := os.Stat(path)
	if err != nil {
		return CapturedLog{Path: path}
	}

	return CapturedLog{Path: path, Size: info.Size(), Present: true}
}

// plistString reads the value of the first key of this name in a plist mimi
// rendered, at whatever depth it sits, and answers "" for anything it cannot
// find that way.
//
// It is deliberately the smallest reader that works on this one file's shape,
// rather than a plist parser: the file it reads is the one renderPlist writes,
// pinned byte for byte by a golden test. A key whose next element is not a
// <string> — or that another key gets in front of — reads as absent, so a plist
// of some other shape costs the detail rather than inventing one.
//
// What it does not skimp on is the escaping, because renderPlist escapes every
// path it writes: a directory named with XML markup sits in the file as markup,
// and handing that straight back would name a file that does not exist and
// report it missing. The value is resolved back to the path it stands for.
func plistString(content, key string) string {
	_, after, found := strings.Cut(content, "<key>"+key+"</key>")
	if !found {
		return ""
	}

	between, after, found := strings.Cut(after, "<string>")
	if !found || strings.Contains(between, "<key>") {
		return ""
	}

	value, _, found := strings.Cut(after, "</string>")
	if !found {
		return ""
	}

	return unescapeXMLText(value)
}

// unescapeXMLText resolves the text of a <string> back to the value it stands
// for, undoing escapeXMLText. It is encoding/xml doing it, so that the two
// sides cannot drift: whatever the escaper learns to write, this reads.
//
// A value it cannot resolve comes back exactly as it was read. That is a plist
// mimi did not write — home-manager's, or a hand-edited one — and this reader's
// bargain everywhere else is to degrade the detail rather than invent one, so
// the text on disk is a better answer than a partially decoded path.
func unescapeXMLText(value string) string {
	var (
		decoder = xml.NewDecoder(strings.NewReader("<v>" + value + "</v>"))
		decoded strings.Builder
	)

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return decoded.String()
		}

		if err != nil {
			return value
		}

		chars, isText := token.(xml.CharData)
		if isText {
			decoded.Write(chars)
		}
	}
}
