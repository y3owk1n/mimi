package config //nolint:testpackage // reads the validated config directly

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

const ruledTiling = "[tiling]\nenabled = true\nlayout = \"cat\"\n"

func TestLoad_TilingRules(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeConfig(t, ruledTiling+
		"[[tiling.rules]]\napp = \"Finder\"\nmanage = false\n"+
		"[[tiling.rules]]\nbundle_id = \"com.apple.*\"\ntitle = \"^Settings$\"\nmanage = true\n"+
		"[[tiling.rules]]\nnarrower_than = 400\nshorter_than = 300\nmanage = false\n"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	rules, err := CompileRules(cfg.Tiling.Rules)
	if err != nil {
		t.Fatalf("CompileRules() error = %v, want nil", err)
	}

	finder := RuleWindow{
		App:      "Finder",
		BundleID: "com.apple.finder",
		Title:    "Downloads",
		Width:    900,
		Height:   600,
	}
	settings := finder
	settings.Title = "Settings"
	mail := RuleWindow{
		App:      "Mail",
		BundleID: "com.apple.mail",
		Title:    "Inbox",
		Width:    900,
		Height:   600,
	}
	popup := mail
	popup.Title, popup.Width, popup.Height = "Popup", 300, 200
	sidebar := mail
	sidebar.Title, sidebar.Width = "Sidebar", 300

	for name, testCase := range map[string]struct {
		win  RuleWindow
		want bool
	}{
		"finder window":      {finder, false},
		"finder settings":    {settings, true},
		"mail":               {mail, true},
		"small on both axes": {popup, false},
		"narrow but tall":    {sidebar, true},
	} {
		if got := Managed(rules, testCase.win); got != testCase.want {
			t.Errorf("%s: Managed() = %v, want %v", name, got, testCase.want)
		}
	}
}

func TestLoad_RejectsATilingRuleItCannotApply(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		rule     string
		fragment string
	}{
		"no manage":     {"app = \"Finder\"\n", "tiling.rules[0]: manage is required"},
		"names nothing": {"manage = false\n", "tiling.rules[0]: names no window"},
		"negative size": {"narrower_than = -1\nmanage = false\n", "tiling.rules[0]: narrower_than and shorter_than must be >= 0"},
		"bad title":     {"title = \"(\"\nmanage = false\n", "tiling.rules[0]: title"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := Load(writeConfig(t, ruledTiling+"[[tiling.rules]]\n"+testCase.rule))
			if !derrors.IsCode(err, derrors.CodeInvalidConfig) {
				t.Fatalf("Load() error = %v, want CodeInvalidConfig", err)
			}

			if !strings.Contains(err.Error(), testCase.fragment) {
				t.Errorf("error %q does not mention %q", err.Error(), testCase.fragment)
			}
		})
	}
}

func TestLoad_TilingLayouts(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeConfig(t, ruledTiling+
		"[[tiling.layouts]]\ndisplay = 2\nlayout = \"bsp\"\n"+
		"[[tiling.layouts]]\nspace = 3\nlayout = \"monocle\"\n"+
		"[[tiling.layouts]]\ndisplay = 2\nspace = 3\nlayout = \"~/columns\"\n"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	for name, testCase := range map[string]struct {
		display, space int
		want           string
	}{
		"unmatched":       {1, 1, "cat"},
		"display":         {2, 1, "bsp"},
		"space":           {1, 3, "monocle"},
		"both, last wins": {2, 3, filepath.Join(os.Getenv("HOME"), "columns")},
	} {
		if got := cfg.Tiling.LayoutFor(testCase.display, testCase.space); got != testCase.want {
			t.Errorf("%s: LayoutFor() = %q, want %q", name, got, testCase.want)
		}
	}
}

func TestLoad_RejectsATilingLayoutTargetItCannotApply(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		entry    string
		fragment string
	}{
		"no layout":     {"display = 1\n", "tiling.layouts[0]: layout is required"},
		"names nothing": {"layout = \"cat\"\n", "tiling.layouts[0]: names no display or space"},
		"negative":      {"display = -1\nlayout = \"cat\"\n", "tiling.layouts[0]: display and space must be >= 1"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := Load(writeConfig(t, ruledTiling+"[[tiling.layouts]]\n"+testCase.entry))
			if !derrors.IsCode(err, derrors.CodeInvalidConfig) {
				t.Fatalf("Load() error = %v, want CodeInvalidConfig", err)
			}

			if !strings.Contains(err.Error(), testCase.fragment) {
				t.Errorf("error %q does not mention %q", err.Error(), testCase.fragment)
			}
		})
	}
}

func TestLoad_TilingEnabledNeedsADefaultOrATargetLayout(t *testing.T) {
	t.Parallel()

	_, err := Load(
		writeConfig(
			t,
			"[tiling]\nenabled = true\n[[tiling.layouts]]\ndisplay = 2\nlayout = \"bsp\"\n",
		),
	)
	if err != nil {
		t.Fatalf("Load() with a target and no default error = %v, want nil", err)
	}
}
