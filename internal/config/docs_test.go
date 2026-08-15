package config //nolint:testpackage // pins HookKinds against the documented tables

// docs/CONFIGURATION.md lists every hook kind in three tables, one per group.
// That is a fourth hand-maintained enumeration of the same twelve kinds, and
// the only one a user actually reads -- a kind missing from it is a feature
// nobody can find, and a kind listed there but absent from the code is a
// promise mimi does not keep.
//
// The compiler cannot check Markdown, so this test reads the document as a
// fixture, the same trick internal/native/eventkinds_test.go uses on
// eventkinds.h. It checks membership and grouping; the prose descriptions in
// the second column are left to human judgement.

import (
	"os"
	"regexp"
	"testing"
)

// hookTableHeadings maps each "### ..." heading in the Hooks section of
// docs/CONFIGURATION.md onto the group its table documents.
var hookTableHeadings = map[string]HookGroup{
	"Application Lifecycle":                  GroupApp,
	"Window events (requires Accessibility)": GroupWindow,
	"Workspace events":                       GroupWorkspace,
}

var (
	headingRe  = regexp.MustCompile(`(?m)^### (.+)$`)
	hookRowRe  = regexp.MustCompile("(?m)^\\| `(on_[a-z_]+)` \\|")
	hooksSecRe = regexp.MustCompile(`(?ms)^## Hooks$.*?^## `)
)

// documentedHookKinds parses the Hooks section of docs/CONFIGURATION.md into a
// map of TOML key -> the group whose table it appeared under.
func documentedHookKinds(t *testing.T) map[string]HookGroup {
	t.Helper()

	// go test runs with the package directory as the working directory.
	data, err := os.ReadFile("../../docs/CONFIGURATION.md")
	if err != nil {
		t.Fatalf("reading docs/CONFIGURATION.md: %v", err)
	}

	section := hooksSecRe.FindString(string(data))
	if section == "" {
		t.Fatal("docs/CONFIGURATION.md: found no '## Hooks' section; this parser is stale")
	}

	documented := make(map[string]HookGroup)

	headings := headingRe.FindAllStringSubmatchIndex(section, -1)
	if len(headings) == 0 {
		t.Fatal(
			"docs/CONFIGURATION.md: the Hooks section has no '###' subheadings; this parser is stale",
		)
	}

	for idx, heading := range headings {
		title := section[heading[2]:heading[3]]

		group, wanted := hookTableHeadings[title]
		if !wanted {
			continue
		}

		end := len(section)
		if idx+1 < len(headings) {
			end = headings[idx+1][0]
		}

		for _, row := range hookRowRe.FindAllStringSubmatch(section[heading[1]:end], -1) {
			if previous, dup := documented[row[1]]; dup {
				t.Errorf("%s is documented twice (groups %v and %v)", row[1], previous, group)
			}

			documented[row[1]] = group
		}
	}

	return documented
}

func TestHookKinds_MatchTheDocumentedTables(t *testing.T) {
	t.Parallel()

	documented := documentedHookKinds(t)

	if len(documented) == 0 {
		t.Fatal("parsed no hook rows out of docs/CONFIGURATION.md; this parser is stale")
	}

	for _, kind := range HookKinds {
		group, ok := documented[kind.TOMLKey]
		if !ok {
			t.Errorf("%s is a hook kind but is not listed in docs/CONFIGURATION.md", kind.TOMLKey)

			continue
		}

		if group != kind.Group {
			t.Errorf(
				"%s is documented under the %v table but the code puts it in %v",
				kind.TOMLKey, group, kind.Group,
			)
		}
	}

	for key := range documented {
		found := false

		for _, kind := range HookKinds {
			if kind.TOMLKey == key {
				found = true

				break
			}
		}

		if !found {
			t.Errorf("docs/CONFIGURATION.md documents %s, which is not a hook kind", key)
		}
	}
}
