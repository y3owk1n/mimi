package config

import (
	"regexp"
	"strings"
)

// Filter is one compiled pattern filter: empty when nothing was set, and
// inverted when the pattern was negated. Hooks and tiling rules match the
// same way, so both compile to this.
type Filter struct {
	re      *regexp.Regexp
	negated bool
}

// Matches reports whether the filter admits value. An unset filter, or the
// catch-all "*", admits everything; negated, the catch-all admits nothing.
func (f Filter) Matches(value string) bool {
	if f.re == nil {
		return !f.negated
	}

	return f.re.MatchString(value) != f.negated
}

// CompileGlobFilter compiles a glob pattern, where "*" is the only wildcard,
// reading the negation prefix off it first. An empty pattern is no filter.
func CompileGlobFilter(pattern string) (Filter, error) {
	return compileFilter(pattern, compileGlob)
}

// CompileRegexpFilter compiles a regular expression, reading the negation
// prefix off it first. An empty pattern is no filter.
func CompileRegexpFilter(pattern string) (Filter, error) {
	return compileFilter(pattern, regexp.Compile)
}

func compileFilter(pattern string, compile func(string) (*regexp.Regexp, error)) (Filter, error) {
	if pattern == "" {
		return Filter{}, nil
	}

	rest, negated := SplitNegation(pattern)

	re, err := compile(rest)
	if err != nil {
		return Filter{}, err
	}

	return Filter{re: re, negated: negated}, nil
}

// compileGlob converts a glob-style pattern (with `*` wildcards) to an
// anchored *regexp.Regexp. Returns nil and a nil error for empty input or
// the catch-all "*", so callers can use a single `if re != nil` check.
func compileGlob(pattern string) (*regexp.Regexp, error) {
	if pattern == "" || pattern == "*" {
		return nil, nil //nolint:nilnil // intentional: signals "no filter"
	}

	quoted := regexp.QuoteMeta(pattern)
	// QuoteMeta escapes `*` to `\*`; convert it back to the regex wildcard.
	body := strings.ReplaceAll(quoted, `\*`, ".*")

	return regexp.Compile("^" + body + "$")
}
