package config //nolint:testpackage // reads the validated config directly

import (
	"strings"
	"testing"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

const ruledTiling = "[tiling]\nenabled = true\nlayout = \"cat\"\n"

func TestLoad_TilingRules(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeConfig(t, ruledTiling+
		"[[tiling.rules]]\napp = \"Finder\"\nmanage = false\n"+
		"[[tiling.rules]]\nbundle_id = \"com.apple.*\"\ntitle = \"^Settings$\"\nmanage = true\n"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	rules, err := CompileRules(cfg.Tiling.Rules)
	if err != nil {
		t.Fatalf("CompileRules() error = %v, want nil", err)
	}

	for _, testCase := range []struct {
		app, bundle, title string
		want               bool
	}{
		{"Finder", "com.apple.finder", "Downloads", false},
		{"Finder", "com.apple.finder", "Settings", true},
		{"Safari", "com.apple.Safari", "Start Page", true},
	} {
		if got := Managed(
			rules,
			testCase.app,
			testCase.bundle,
			testCase.title,
		); got != testCase.want {
			t.Errorf(
				"Managed(%q, %q, %q) = %v, want %v",
				testCase.app,
				testCase.bundle,
				testCase.title,
				got,
				testCase.want,
			)
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
