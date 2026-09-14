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
	// Manage says whether the layout sees a matching window. It may be
	// left out on a rule that only places windows.
	Manage *bool `json:"manage" toml:"manage"`
	// Space and Display, as the actions count them, are where a window
	// this rule matches goes when it is created. 0 leaves a side unset.
	Space   int `json:"space"   toml:"space"`
	Display int `json:"display" toml:"display"`
	// Follow says whether the window is focused where it lands, and the
	// space brought to the front. It is on unless set to false.
	Follow *bool `json:"follow" toml:"follow"`
}

// Placement is where a rule sends a window when it is created: a space, a
// display, or both, and whether focus goes with it.
type Placement struct {
	Space   int
	Display int
	Follow  bool
}

// Places reports whether the placement sends the window anywhere.
func (p Placement) Places() bool {
	return p.Space != 0 || p.Display != 0
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
	// manage is nil on a rule that says nothing about managing.
	manage    *bool
	placement Placement
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

	if rule.Space < 0 || rule.Display < 0 {
		return Rule{}, derrors.New(derrors.CodeInvalidConfig, "space and display must be >= 1")
	}

	placement := Placement{Space: rule.Space, Display: rule.Display, Follow: true}
	if rule.Follow != nil {
		placement.Follow = *rule.Follow
	}

	if rule.Manage == nil && !placement.Places() {
		return Rule{}, derrors.New(
			derrors.CodeInvalidConfig,
			"manage is required on a rule that sets no space or display",
		)
	}

	compiled := Rule{
		narrowerThan: rule.NarrowerThan,
		shorterThan:  rule.ShorterThan,
		manage:       rule.Manage,
		placement:    placement,
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
		if rule.Matches(win) && rule.manage != nil {
			managed = *rule.manage
		}
	}

	return managed
}

// PlacementFor is where the rules send a window when it is created: what
// the last rule naming it with a space or a display says, and false when
// none does.
func PlacementFor(rules []Rule, win RuleWindow) (Placement, bool) {
	var (
		placement Placement
		found     bool
	)

	for _, rule := range rules {
		if rule.Matches(win) && rule.placement.Places() {
			placement, found = rule.placement, true
		}
	}

	return placement, found
}

// AnyPlaces reports whether any rule sends windows somewhere, which is what
// says the daemon has to watch windows being created.
func AnyPlaces(rules []TilingRule) bool {
	for _, rule := range rules {
		if rule.Space != 0 || rule.Display != 0 {
			return true
		}
	}

	return false
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
