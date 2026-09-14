package config

import (
	"fmt"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// TilingRule is one [[tiling.rules]] entry. It names windows by application
// name and bundle identifier as globs and by title as a regular expression,
// each optionally negated with "!", or by size, and says whether the layout
// manages them. Every condition set has to hold. A window matching a rule
// with manage = false never reaches the layout. The engine reads rules in
// order and the last one that matches decides, so a narrow rule written
// after a broad one takes a window back.
type TilingRule struct {
	App      string `json:"app"      toml:"app"`
	BundleID string `json:"bundleId" toml:"bundle_id"`
	Title    string `json:"title"    toml:"title"`
	// NarrowerThan and ShorterThan match a window whose width or height,
	// in points, is under the number. 0 is unset.
	NarrowerThan float64 `json:"narrowerThan" toml:"narrower_than"`
	ShorterThan  float64 `json:"shorterThan"  toml:"shorter_than"`
	Manage       *bool   `json:"manage"       toml:"manage"`
}

// RuleWindow is what a rule is matched against: one window's application
// name, bundle identifier, title and size in points.
type RuleWindow struct {
	App      string
	BundleID string
	Title    string
	Width    float64
	Height   float64
}

// Rule is a TilingRule compiled for matching.
type Rule struct {
	app          Filter
	bundle       Filter
	title        Filter
	narrowerThan float64
	shorterThan  float64
	manage       bool
}

// Matches reports whether the rule names the window.
func (r Rule) Matches(win RuleWindow) bool {
	return r.app.Matches(win.App) && r.bundle.Matches(win.BundleID) && r.title.Matches(win.Title) &&
		(r.narrowerThan == 0 || win.Width < r.narrowerThan) &&
		(r.shorterThan == 0 || win.Height < r.shorterThan)
}

// CompileRule compiles one rule, reporting what is wrong with it when it
// cannot be.
func CompileRule(rule TilingRule) (Rule, error) {
	if rule.App == "" && rule.BundleID == "" && rule.Title == "" &&
		rule.NarrowerThan == 0 && rule.ShorterThan == 0 {
		return Rule{}, derrors.New(
			derrors.CodeInvalidConfig,
			"names no window: set app, bundle_id, title, narrower_than or shorter_than",
		)
	}

	if rule.NarrowerThan < 0 || rule.ShorterThan < 0 {
		return Rule{}, derrors.New(
			derrors.CodeInvalidConfig,
			"narrower_than and shorter_than must be >= 0",
		)
	}

	if rule.Manage == nil {
		return Rule{}, derrors.New(derrors.CodeInvalidConfig, "manage is required")
	}

	compiled := Rule{
		narrowerThan: rule.NarrowerThan,
		shorterThan:  rule.ShorterThan,
		manage:       *rule.Manage,
	}

	var err error

	compiled.app, err = CompileGlobFilter(rule.App)
	if err != nil {
		return Rule{}, derrors.Wrapf(err, derrors.CodeInvalidConfig, "app")
	}

	compiled.bundle, err = CompileGlobFilter(rule.BundleID)
	if err != nil {
		return Rule{}, derrors.Wrapf(err, derrors.CodeInvalidConfig, "bundle_id")
	}

	compiled.title, err = CompileRegexpFilter(rule.Title)
	if err != nil {
		return Rule{}, derrors.Wrapf(err, derrors.CodeInvalidConfig, "title")
	}

	return compiled, nil
}

// CompileRules compiles every rule, in order.
func CompileRules(rules []TilingRule) ([]Rule, error) {
	compiled := make([]Rule, 0, len(rules))

	for index, rule := range rules {
		one, err := CompileRule(rule)
		if err != nil {
			return nil, derrors.Wrapf(err, derrors.CodeInvalidConfig, "tiling.rules[%d]", index)
		}

		compiled = append(compiled, one)
	}

	return compiled, nil
}

// Managed reports whether the rules let the layout manage a window: true
// when no rule names it, else what the last rule naming it says.
func Managed(rules []Rule, win RuleWindow) bool {
	managed := true

	for _, rule := range rules {
		if rule.Matches(win) {
			managed = rule.manage
		}
	}

	return managed
}

// validateRules holds every [[tiling.rules]] entry to what CompileRules
// needs, one line per broken rule.
func validateRules(tiling TilingConfig) []string {
	var errs []string

	for index, rule := range tiling.Rules {
		_, err := CompileRule(rule)
		if err != nil {
			errs = append(errs, fmt.Sprintf("tiling.rules[%d]: %s", index, derrors.Message(err)))
		}
	}

	return errs
}
