package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/y3owk1n/mimi/internal/config"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/service"
)

// Status is how a check came out.
type Status int

const (
	// Pass is a check that found nothing wrong.
	Pass Status = iota
	// Warn is a check that found something the user may want, but nothing
	// that stops mimi working.
	Warn
	// Fail is a check that found why something does not work.
	Fail
	// Skip is a check with nothing to check: no daemon to reach, no service
	// installed.
	Skip
)

func (s Status) String() string {
	switch s {
	case Pass:
		return "ok"
	case Warn:
		return "warn"
	case Fail:
		return "FAIL"
	case Skip:
		return "skip"
	default:
		return "?"
	}
}

// Check is one line of the report: what was checked, how it came out, what
// was found, and for anything but a pass, what to do about it.
type Check struct {
	Name   string
	Status Status
	Detail string
	Fix    string
}

// The names of the checks, as the report prints them.
const (
	checkConfig        = "config"
	checkAccessibility = "accessibility"
	checkDaemon        = "daemon"
	checkSocket        = "socket"
	checkDaemonBuild   = "daemon build"
	checkService       = "service"
	checkHookCommands  = "hook commands"
	checkLogFile       = "log file"
	checkSpaceSwitch   = "space switch"
	checkLayout        = "layout"
)

// noConfig is what a check that needs the config says when it did not load.
const noConfig = "no config"

// Facts is everything Assess judges, gathered by the command.
type Facts struct {
	ConfigPath string
	Config     *config.Config
	// ConfigErr is what config.Load returned.
	ConfigErr error

	Accessibility bool

	// PIDPath and SocketPath are the runtime paths the config names, expanded.
	PIDPath    string
	SocketPath string
	// PID is the pid file's contents. PIDFound reports whether there was one.
	PID      int
	PIDFound bool
	// Alive reports whether a process with PID answers a signal.
	Alive         bool
	SocketPresent bool

	// CLIVersion is this build, and DaemonVersion the one the daemon
	// reported over the socket. ProbeErr is why it reported none: the
	// daemon refused the request as another build, or did not know it.
	CLIVersion    string
	DaemonVersion string
	ProbeErr      error

	Service service.Status
	// ForeignAgent is the launchd job running the daemon when it is not
	// mimi's own service, "" when there is none.
	ForeignAgent string
	// MissingCommands is what MissingHookCommands found for the PATH the
	// installed service runs hooks with.
	MissingCommands []string

	// LogFile is settings.log_file, "" when unset. LogDirWritable reports
	// whether mimi can write in its directory.
	LogFile        string
	LogDirWritable bool

	// Layouts is every layout command the config names, run once each on
	// the sample input.
	Layouts []LayoutRun

	// SwipeAugmented is which dock swipe encoding a space switch is sent
	// with, and SwipeOverride the MIMI_FORCE_DOCK_SWIPE_AUGMENTATION value
	// forcing it, "" when unset.
	SwipeAugmented bool
	SwipeOverride  string
}

// Assess judges the facts, one check per line of docs/TROUBLESHOOTING.md a
// program can walk for the user.
func Assess(facts Facts) []Check {
	checks := []Check{
		configCheck(facts),
		accessibilityCheck(facts),
		daemonCheck(facts),
		socketCheck(facts),
		daemonBuildCheck(facts),
		serviceCheck(facts),
		hookCommandsCheck(facts),
		logCheck(facts),
	}

	checks = append(checks, layoutChecks(facts)...)

	return append(checks, swipeCheck(facts))
}

func configCheck(facts Facts) Check {
	check := Check{Name: checkConfig, Detail: facts.ConfigPath}

	switch {
	case facts.ConfigErr != nil:
		check.Status = Fail
		check.Detail = facts.ConfigErr.Error()
		check.Fix = "fix the config, then run mimi config validate"
	case len(facts.Config.UnknownHookKeys) > 0:
		check.Status = Warn
		check.Detail = "unknown hook kinds: " + strings.Join(facts.Config.UnknownHookKeys, ", ")
		check.Fix = "run mimi config validate for the recognized kinds"
	}

	return check
}

func accessibilityCheck(facts Facts) Check {
	if facts.Accessibility {
		return Check{Name: checkAccessibility, Detail: "granted"}
	}

	return Check{
		Name:   checkAccessibility,
		Status: Fail,
		Detail: "not granted",
		Fix:    "System Settings > Privacy & Security > Accessibility, enable the mimi binary you run, then restart it",
	}
}

func daemonCheck(facts Facts) Check {
	switch {
	case !facts.PIDFound:
		return Check{
			Name:   checkDaemon,
			Status: Warn,
			Detail: "not running",
			Fix:    "mimi start, or mimi services install to run it at login",
		}
	case !facts.Alive:
		return Check{
			Name:   checkDaemon,
			Status: Fail,
			Detail: fmt.Sprintf("stale PID file at %s (pid %d is gone)", facts.PIDPath, facts.PID),
			Fix:    "mimi start overwrites it",
		}
	default:
		return Check{Name: checkDaemon, Detail: fmt.Sprintf("running (pid %d)", facts.PID)}
	}
}

func socketCheck(facts Facts) Check {
	running := facts.PIDFound && facts.Alive

	switch {
	case !running:
		return Check{Name: checkSocket, Status: Skip, Detail: "no daemon to reach"}
	case !facts.SocketPresent:
		return Check{
			Name:   checkSocket,
			Status: Fail,
			Detail: "daemon running but no socket at " + facts.SocketPath,
			Fix:    "the CLI and the daemon may read different configs, so run both with the same --config",
		}
	default:
		return Check{Name: checkSocket, Detail: facts.SocketPath}
	}
}

func daemonBuildCheck(facts Facts) Check {
	fix := "restart the daemon so it runs this build: mimi stop && mimi start, or mimi services restart"

	switch {
	case !facts.PIDFound || !facts.Alive || !facts.SocketPresent:
		return Check{Name: checkDaemonBuild, Status: Skip, Detail: "no daemon to ask"}
	case derrors.IsCode(facts.ProbeErr, derrors.CodeInvalidInput):
		return Check{
			Name:   checkDaemonBuild,
			Status: Fail,
			Detail: "another build than this CLI, one that predates the status request",
			Fix:    fix,
		}
	case facts.ProbeErr != nil:
		return Check{
			Name:   checkDaemonBuild,
			Status: Fail,
			Detail: "another build than this CLI, " + derrors.Message(facts.ProbeErr),
			Fix:    fix,
		}
	case facts.DaemonVersion != facts.CLIVersion:
		return Check{
			Name:   checkDaemonBuild,
			Status: Fail,
			Detail: fmt.Sprintf(
				"daemon is %s, this CLI is %s",
				facts.DaemonVersion,
				facts.CLIVersion,
			),
			Fix: fix,
		}
	default:
		return Check{Name: checkDaemonBuild, Detail: facts.DaemonVersion + ", same as this CLI"}
	}
}

func serviceCheck(facts Facts) Check {
	status := facts.Service

	if status.State == service.LoadStateNotLoaded {
		detail := "not installed"
		if facts.ForeignAgent != "" {
			detail += ", the daemon runs under launchd agent " + facts.ForeignAgent
		}

		return Check{Name: checkService, Status: Skip, Detail: detail}
	}

	if status.State == service.LoadStateUnknown {
		return Check{Name: checkService, Status: Skip, Detail: "launchctl could not be asked"}
	}

	if status.PID.Known {
		return Check{
			Name:   checkService,
			Detail: fmt.Sprintf("loaded and running (pid %d)", status.PID.Value),
		}
	}

	check := Check{Name: checkService, Status: Fail, Detail: "loaded but not running"}

	if status.LastExitStatus.Known {
		check.Detail = fmt.Sprintf(
			"loaded but not running, last exit status %d",
			status.LastExitStatus.Value,
		)
	}

	if status.CapturedStderr.Path != "" {
		check.Fix = "read " + status.CapturedStderr.Path
	} else {
		check.Fix = "mimi services status"
	}

	return check
}

func hookCommandsCheck(facts Facts) Check {
	if facts.Config == nil {
		return Check{Name: checkHookCommands, Status: Skip, Detail: noConfig}
	}

	if len(facts.MissingCommands) == 0 {
		return Check{Name: checkHookCommands, Detail: "every command is on the service PATH"}
	}

	return Check{
		Name:   checkHookCommands,
		Status: Warn,
		Detail: "not on the service PATH: " + strings.Join(facts.MissingCommands, ", "),
		Fix:    "set settings.service_path and run mimi services install, or call the command by absolute path",
	}
}

func logCheck(facts Facts) Check {
	switch {
	case facts.Config == nil:
		return Check{Name: checkLogFile, Status: Skip, Detail: noConfig}
	case facts.LogFile == "":
		return Check{
			Name:   checkLogFile,
			Status: Warn,
			Detail: "settings.log_file is unset",
			Fix:    "set it to keep the daemon's log for a bug report",
		}
	case !facts.LogDirWritable:
		return Check{
			Name:   checkLogFile,
			Status: Fail,
			Detail: "cannot write in " + filepath.Dir(facts.LogFile),
			Fix:    "create the directory or point settings.log_file elsewhere",
		}
	default:
		return Check{Name: checkLogFile, Detail: facts.LogFile}
	}
}

// layoutChecks is one check per layout command, or one skip when the config
// names none.
func layoutChecks(facts Facts) []Check {
	if facts.Config == nil {
		return []Check{{Name: checkLayout, Status: Skip, Detail: noConfig}}
	}

	if len(facts.Layouts) == 0 {
		return []Check{{Name: checkLayout, Status: Skip, Detail: "tiling.layout is not set"}}
	}

	checks := make([]Check, 0, len(facts.Layouts))
	for _, run := range facts.Layouts {
		checks = append(checks, layoutCheck(run))
	}

	return checks
}

func layoutCheck(run LayoutRun) Check {
	switch {
	case run.Err != nil:
		return Check{
			Name:   checkLayout,
			Status: Fail,
			Detail: run.Command + ": " + derrors.Message(run.Err),
			Fix:    "run it by hand with mimi tiling preview --input | " + run.Command,
		}
	case run.Empty:
		return Check{
			Name:   checkLayout,
			Status: Warn,
			Detail: run.Command + ": printed nothing, which the daemon reads as no frames",
			Fix:    "print {\"frames\": [...], \"state\": null} even when there is nothing to move",
		}
	case run.Frames == 0:
		return Check{
			Name:   checkLayout,
			Status: Warn,
			Detail: fmt.Sprintf("%s: no frames for %d windows", run.Command, run.Windows),
			Fix:    "give every window a frame on a preview event",
		}
	default:
		return Check{
			Name: checkLayout,
			Detail: fmt.Sprintf(
				"%s: %d frames for %d windows",
				run.Command,
				run.Frames,
				run.Windows,
			),
		}
	}
}

func swipeCheck(facts Facts) Check {
	detail := "pre-macOS 27 encoding"
	if facts.SwipeAugmented {
		detail = "macOS 27 encoding"
	}

	if facts.SwipeOverride != "" {
		detail += ", forced by MIMI_FORCE_DOCK_SWIPE_AUGMENTATION=" + facts.SwipeOverride
	}

	return Check{Name: checkSpaceSwitch, Detail: detail}
}

// MissingHookCommands is every command a hook's run line starts with that
// the service PATH does not hold, once each, in config order. A run line
// that starts with a path, a shell keyword, a builtin, or a variable
// assignment names no command to look for.
func MissingHookCommands(cfg *config.Config, pathList string) []string {
	dirs := filepath.SplitList(pathList)

	var missing []string

	seen := map[string]bool{}

	for _, kind := range config.HookKinds {
		for _, entry := range *kind.Entries(&cfg.Hooks) {
			name := leadingCommand(entry.Run)
			if name == "" || seen[name] {
				continue
			}

			seen[name] = true

			if !onPath(name, dirs) {
				missing = append(missing, name)
			}
		}
	}

	return missing
}

// shellWords is what a run line may start with without naming a command.
//
//nolint:gochecknoglobals // a fixed table
var shellWords = map[string]bool{
	"if": true, "then": true, "for": true, "while": true, "case": true, "do": true,
	"cd": true, "export": true, "exec": true, "eval": true, "set": true, "test": true,
	"echo": true, "printf": true, "true": true, "false": true, "exit": true, "[": true,
	"{": true, "(": true, ".": true, "source": true,
}

func leadingCommand(run string) string {
	for word := range strings.FieldsSeq(run) {
		if strings.Contains(word, "=") && !strings.HasPrefix(word, "=") {
			continue
		}

		if strings.Contains(word, "/") || shellWords[word] {
			return ""
		}

		return word
	}

	return ""
}

func onPath(name string, dirs []string) bool {
	for _, dir := range dirs {
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return true
		}
	}

	return false
}

// Format renders the checks as the report mimi doctor prints.
func Format(checks []Check) string {
	var out strings.Builder

	for _, check := range checks {
		fmt.Fprintf(&out, "%-5s %-14s %s\n", check.Status, check.Name, check.Detail)

		if check.Fix != "" {
			fmt.Fprintf(&out, "      fix: %s\n", check.Fix)
		}
	}

	return out.String()
}

// Failed reports whether any check failed.
func Failed(checks []Check) bool {
	for _, check := range checks {
		if check.Status == Fail {
			return true
		}
	}

	return false
}
