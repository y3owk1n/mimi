package config //nolint:testpackage // uses the writeConfig helper beside decode_test.go

import (
	"strings"
	"testing"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

func TestParseSpaceFilter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		filter      string
		wantIndex   string
		wantNegated bool
		wantErr     bool
	}{
		{filter: "2", wantIndex: "2"},
		{filter: "!2", wantIndex: "2", wantNegated: true},
		{filter: " 07 ", wantIndex: "7"},
		{filter: "0", wantErr: true},
		{filter: "!", wantErr: true},
		{filter: "next", wantErr: true},
		{filter: "", wantErr: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.filter, func(t *testing.T) {
			t.Parallel()

			index, negated, err := ParseSpaceFilter(testCase.filter)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("ParseSpaceFilter(%q) error = nil, want an error", testCase.filter)
				}

				if !derrors.IsCode(err, derrors.CodeInvalidConfig) {
					t.Fatalf("error = %v, want invalid config", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseSpaceFilter(%q) error = %v, want nil", testCase.filter, err)
			}

			if index != testCase.wantIndex || negated != testCase.wantNegated {
				t.Fatalf(
					"ParseSpaceFilter(%q) = %q, %v, want %q, %v",
					testCase.filter, index, negated, testCase.wantIndex, testCase.wantNegated,
				)
			}
		})
	}
}

// TestLoad_SpaceFilterIsAcceptedAsANumberOrAString pins the two spellings a
// space filter takes in TOML: a bare number, and a string when negated.
func TestLoad_SpaceFilterIsAcceptedAsANumberOrAString(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"number":         "[hooks]\non_workspace_changed = [{ run = \"true\", space = 2 }]\n",
		"string":         "[hooks]\non_workspace_changed = [{ run = \"true\", space = \"2\" }]\n",
		"negated string": "[hooks]\non_workspace_changed = [{ run = \"true\", space = \"!2\" }]\n",
	}
	want := map[string]string{"number": "2", "string": "2", "negated string": "!2"}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Load(writeConfig(t, src))
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}

			if got := cfg.Hooks.WorkspaceChanged[0].Space; got != want[name] {
				t.Fatalf("Space = %q, want %q", got, want[name])
			}
		})
	}
}

// TestLoad_RejectsFiltersThatCannotMean: a space filter on a hook whose
// events carry no space, a space that is not one, and a filter that is only
// the negation prefix are all reported by the key the user typed.
func TestLoad_RejectsFiltersThatCannotMean(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		src  string
		want string
	}{
		"space on a window hook": {
			src:  "[hooks]\non_window_focus = [{ run = \"true\", space = 2 }]\n",
			want: "hooks.on_window_focus[0]: space applies to workspace hooks only",
		},
		"space zero": {
			src:  "[hooks]\non_workspace_changed = [{ run = \"true\", space = 0 }]\n",
			want: "hooks.on_workspace_changed[0]: space must be a 1-based space number",
		},
		"space that is a word": {
			src:  "[hooks]\non_workspace_changed = [{ run = \"true\", space = \"next\" }]\n",
			want: "hooks.on_workspace_changed[0]: space must be a 1-based space number",
		},
		"space of another type": {
			src:  "[hooks]\non_workspace_changed = [{ run = \"true\", space = true }]\n",
			want: "hooks.on_workspace_changed[0]: space must be a number or a string",
		},
		"app filter that is only a !": {
			src:  "[hooks]\non_app_activate = [{ run = \"true\", app = \"!\" }]\n",
			want: "hooks.on_app_activate[0]: app filter is only a !",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := Load(writeConfig(t, testCase.src))
			if err == nil {
				t.Fatal("Load() error = nil, want a rejection")
			}

			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Load() error = %v, want it to contain %q", err, testCase.want)
			}
		})
	}
}
