package config

import (
	"slices"
	"strconv"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// negation is the prefix that inverts a hook filter: "!Safari" matches every
// application but Safari.
const negation = "!"

// SplitNegation reads a hook filter's negation prefix off the front of it,
// returning the pattern that is left and whether it was negated. It is the
// one place the prefix is read, shared by validation and by the registry
// that matches events against it, so both agree on what "!" means.
func SplitNegation(pattern string) (string, bool) {
	rest, negated := strings.CutPrefix(pattern, negation)

	return rest, negated
}

// ParseSpaceFilter reads a hook's space filter: a 1-based space number,
// optionally negated, as a string so that "!2" has somewhere to live. It
// returns the number in the form workspace events carry it, so the registry
// compares two strings written by the same function, and whether the filter
// is negated.
func ParseSpaceFilter(filter string) (string, bool, error) {
	pattern, negated := SplitNegation(filter)

	index, err := strconv.Atoi(strings.TrimSpace(pattern))
	if err != nil || index < 1 {
		return "", false, derrors.Newf(
			derrors.CodeInvalidConfig,
			"space must be a 1-based space number, optionally prefixed with %s (got %q)",
			negation,
			filter,
		)
	}

	return strconv.Itoa(index), negated, nil
}

// validateFilters holds the rules a hook entry's filters are held to beyond
// what compiles: a filter that is nothing but the negation prefix names
// nothing to negate, and a space filter only means something on a hook whose
// events carry a space.
func validateFilters(kind HookKind, entry HookEntry) []string {
	var errs []string

	for name, filter := range map[string]string{
		"app":       entry.App,
		"bundle_id": entry.BundleID,
		"title":     entry.Title,
	} {
		if filter == negation {
			errs = append(errs, name+" filter is only a "+negation+", with nothing to negate")
		}
	}

	if entry.Space == "" {
		return sortedErrs(errs)
	}

	if kind.Group != GroupWorkspace {
		errs = append(errs, "space applies to workspace hooks only")
	}

	_, _, err := ParseSpaceFilter(entry.Space)
	if err != nil {
		errs = append(errs, derrors.Message(err))
	}

	return sortedErrs(errs)
}

// sortedErrs orders a filter's problems so the same config reports the same
// way on every run; the filters are checked out of a map.
func sortedErrs(errs []string) []string {
	slices.Sort(errs)

	return errs
}
