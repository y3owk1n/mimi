package config

import (
	"reflect"
	"slices"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// reloadability says what a user has to do for a change to a config field to
// take effect: nothing beyond the reload, a restart, or a reinstall of the
// service. It is not only "when does the daemon pick this up", because a field
// the daemon never reads has no answer to that
// (docs/adr/0003-a-setting-the-daemon-never-reads-is-reinstall-only.md).
type reloadability string

const (
	// reloadable marks a field the daemon's reload path re-reads, so a change
	// to it takes effect on the next reload.
	reloadable reloadability = "reloadable"
	// restartOnly marks a restart-only setting: the daemon reads it once at
	// startup and never again, so a change to it takes effect only after the
	// daemon is restarted.
	restartOnly reloadability = "restart-only"
	// reinstallOnly marks a reinstall-only setting: the daemon never reads it,
	// because it describes how the launchd service is set up rather than how
	// the daemon behaves. `mimi services install` renders it into the plist, so
	// a change to it takes effect only when the service is installed again —
	// restarting the daemon runs it against the same plist.
	reinstallOnly reloadability = "reinstall-only"
)

// reloadTagPerField is the reload tag a section carries when its fields are
// classified one at a time rather than together — [settings] and [systray],
// where some fields reload and some do not. It is a tag value rather than the
// absence of a tag so that every part of the config file says outright how it
// is classified, and a section nobody thought about is an error rather than a
// silent recursion.
const reloadTagPerField = "per-field"

// reloadTagKey is the struct tag that classifies a config field.
//
// The classification lives on the config type itself so that it is written
// where the field is, instead of in a list kept somewhere else: the
// hand-maintained list in docs/CONFIGURATION.md was wrong within a day of
// being written, promising that log_format and max_hook_workers take effect
// on reload when neither does.
//
// The tag records what a user must do for a change to the field to take
// effect. Widening what the daemon's reload path re-reads — teaching
// internal/daemon's reloader.Apply to re-read a field it ignores today —
// means moving that field's tag to reloadable in the same change.
const reloadTagKey = "reload"

// reloadTagValues spells out every value reloadTagKey accepts, for the errors
// an unclassified or misclassified field raises. It is one string so that the
// two errors cannot list different sets.
const reloadTagValues = `"reloadable", "restart-only", "reinstall-only", or "per-field"`

// settingField is one classified part of the config file.
type settingField struct {
	// Key is the dotted TOML path a user would write: "settings.log_level"
	// for a leaf field, or a bare section name ("hooks") for a section
	// classified as a whole.
	Key string
	// kind says whether a change to this field is picked up by a
	// reload or needs a restart.
	kind reloadability
	// index locates the field inside a Config value, for FieldByIndex. A
	// section classified as a whole indexes the section itself.
	index []int
}

// settingFields is every classified part of the config file, in the order
// Config declares them — which is the order changed settings are reported in,
// and the order docs/CONFIGURATION.md lists them.
var settingFields = mustClassifyConfig()

// mustClassifyConfig derives the classification from the Config type.
//
// A config field with no reload tag is a programming error, and package init
// is the first moment it can be caught: Go cannot check struct-field
// exhaustiveness at compile time. Panicking here fails every test binary and
// the daemon's own startup rather than letting an unclassified field be
// silently left out of what a reload reports.
func mustClassifyConfig() []settingField {
	fields, err := classifyFields(reflect.TypeFor[Config](), "", nil)
	if err != nil {
		panic(err)
	}

	return fields
}

// classifyFields walks structType and returns one settingField per part of
// the config file it declares, prefixing keys with prefix and field indices
// with index so nested sections report their full path.
func classifyFields(structType reflect.Type, prefix string, index []int) ([]settingField, error) {
	var fields []settingField

	for field := range structType.Fields() {
		key, inFile := field.Tag.Lookup("toml")
		if !inFile || key == "-" {
			// Not part of the config file, so there is nothing for a user to
			// change and nothing to classify. UnknownHookKeys is the one such
			// field today.
			continue
		}

		if prefix != "" {
			key = prefix + "." + key
		}

		fieldIndex := append(slices.Clip(index), field.Index...)

		tag, classified := field.Tag.Lookup(reloadTagKey)

		switch {
		case !classified:
			return nil, derrors.Newf(
				derrors.CodeInternal,
				"config field %q carries no %q tag: classify it as %s",
				key, reloadTagKey, reloadTagValues,
			)
		case tag == reloadTagPerField:
			if field.Type.Kind() != reflect.Struct {
				return nil, derrors.Newf(
					derrors.CodeInternal,
					"config field %q is tagged %q but is not a section",
					key, reloadTagPerField,
				)
			}

			nested, err := classifyFields(field.Type, key, fieldIndex)
			if err != nil {
				return nil, err
			}

			fields = append(fields, nested...)
		default:
			kind, err := parseReloadability(key, tag)
			if err != nil {
				return nil, err
			}

			fields = append(fields, settingField{Key: key, kind: kind, index: fieldIndex})
		}
	}

	return fields, nil
}

func parseReloadability(key, tag string) (reloadability, error) {
	switch kind := reloadability(tag); kind {
	case reloadable, restartOnly, reinstallOnly:
		return kind, nil
	default:
		return "", derrors.Newf(
			derrors.CodeInternal,
			"config field %q has %s tag %q: want %s",
			key, reloadTagKey, tag, reloadTagValues,
		)
	}
}

// RestartOnlyChanges names the restart-only settings whose values differ
// between running and next, as dotted TOML keys in declaration order.
//
// running is the config the daemon actually started with, not the last one it
// reloaded: a restart-only setting only ever takes effect at startup, so the
// question worth answering is "does this differ from what the daemon is
// running", and it stays answered the same way however many reloads happened
// in between. Changing log_level and then putting it back reports nothing,
// because nothing is out of step any more.
func RestartOnlyChanges(running, next *Config) []string {
	return changedFields(running, next, restartOnly)
}

// ReinstallOnlyChanges names the reinstall-only settings whose values differ
// between running and next, as dotted TOML keys in declaration order.
//
// These are the settings a restart would not pick up either: they are rendered
// into the launchd plist by `mimi services install`, and the plist on disk is
// what the service actually runs with. running is the same baseline
// RestartOnlyChanges uses — the config this process started with — which is
// the closest thing the daemon has to "what the installed service was built
// from", since it cannot read the plist and would not know which install
// produced it if it could.
func ReinstallOnlyChanges(running, next *Config) []string {
	return changedFields(running, next, reinstallOnly)
}

// changedFields names the settings of one classification whose values differ
// between running and next, in declaration order.
func changedFields(running, next *Config, kind reloadability) []string {
	if running == nil || next == nil {
		return nil
	}

	runningValue := reflect.ValueOf(*running)
	nextValue := reflect.ValueOf(*next)

	var changed []string

	for _, field := range settingFields {
		if field.kind != kind {
			continue
		}

		runningField := runningValue.FieldByIndex(field.index).Interface()
		if !reflect.DeepEqual(runningField, nextValue.FieldByIndex(field.index).Interface()) {
			changed = append(changed, field.Key)
		}
	}

	return changed
}
