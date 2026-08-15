package config //nolint:testpackage // pins the classification against the documented lists

// docs/CONFIGURATION.md tells users which settings a reload picks up and which
// need a restart. That list is the only one most people will ever read, and
// the previous, hand-maintained version of it was wrong within a day of being
// written — it promised that log_format and max_hook_workers take effect on
// reload when the logger is built once at CLI start and the hook worker limit
// sizes a channel the executor never resizes.
//
// The compiler cannot check Markdown, so this test reads the document as a
// fixture — the same trick docs_test.go uses on the hook kind tables — and
// pins both lists against the reload tags on the config type.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// reloadLabels maps the bold label that introduces each list in the Reloading
// section of docs/CONFIGURATION.md onto the classification it documents.
var reloadLabels = map[string]reloadability{
	"Restart-only": restartOnly,
	"Reloadable":   reloadable,
}

var (
	reloadSecRe   = regexp.MustCompile(`(?ms)^## Reloading$.*?^## `)
	reloadLabelRe = regexp.MustCompile(`(?m)^\*\*([A-Za-z-]+)\*\*`)
	reloadItemRe  = regexp.MustCompile("(?m)^- `([^`]+)`")
)

// documentedReloadability parses the Reloading section of
// docs/CONFIGURATION.md into a map of config key -> the classification whose
// list it appeared under.
//
// A whole section is written the way a user writes it in TOML — `[hooks]` —
// while the classification names it `hooks`, so the brackets are stripped
// here rather than making the document read like something nobody types.
func documentedReloadability(t *testing.T) map[string]reloadability {
	t.Helper()

	// go test runs with the package directory as the working directory.
	data, err := os.ReadFile("../../docs/CONFIGURATION.md")
	if err != nil {
		t.Fatalf("reading docs/CONFIGURATION.md: %v", err)
	}

	section := reloadSecRe.FindString(string(data))
	if section == "" {
		t.Fatal("docs/CONFIGURATION.md: found no '## Reloading' section; this parser is stale")
	}

	documented := make(map[string]reloadability)

	labels := reloadLabelRe.FindAllStringSubmatchIndex(section, -1)
	if len(labels) == 0 {
		t.Fatal(
			"docs/CONFIGURATION.md: the Reloading section has no bold lists; this parser is stale",
		)
	}

	for idx, label := range labels {
		reloadability, wanted := reloadLabels[section[label[2]:label[3]]]
		if !wanted {
			continue
		}

		end := len(section)
		if idx+1 < len(labels) {
			end = labels[idx+1][0]
		}

		for _, item := range reloadItemRe.FindAllStringSubmatch(section[label[1]:end], -1) {
			key := strings.Trim(item[1], "[]")

			if previous, dup := documented[key]; dup {
				t.Errorf("%s is listed twice (as %v and %v)", key, previous, reloadability)
			}

			documented[key] = reloadability
		}
	}

	return documented
}

func TestReloadability_MatchesTheDocumentedLists(t *testing.T) {
	t.Parallel()

	documented := documentedReloadability(t)

	if len(documented) == 0 {
		t.Fatal("parsed no settings out of docs/CONFIGURATION.md; this parser is stale")
	}

	classified := make(map[string]reloadability, len(settingFields))

	for _, field := range settingFields {
		classified[field.Key] = field.kind

		switch documentedReloadability, listed := documented[field.Key]; {
		case !listed:
			t.Errorf(
				"%s is a config field but docs/CONFIGURATION.md lists it under neither reload list",
				field.Key,
			)
		case documentedReloadability != field.kind:
			t.Errorf(
				"docs/CONFIGURATION.md documents %s as %v, but the config type classifies it as %v",
				field.Key, documentedReloadability, field.kind,
			)
		}
	}

	for key := range documented {
		if _, ok := classified[key]; !ok {
			t.Errorf("docs/CONFIGURATION.md lists %s, which is not a config field", key)
		}
	}
}
