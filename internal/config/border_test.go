package config //nolint:testpackage // reads the validated config directly

import (
	"strings"
	"testing"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

const (
	borderedConfig = "[border]\nenabled = true\n"
	widthMessage   = "border.width must be between 1 and 32"
)

func TestLoad_BorderDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeConfig(t, borderedConfig))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if !cfg.Border.Enabled {
		t.Fatal("border.enabled = false, want true")
	}

	if got, want := cfg.Border.Width, defaultBorderWidth; got != want {
		t.Errorf("border.width = %v, want %v", got, want)
	}

	if got, want := cfg.Border.CornerRadius(), float64(FollowWindowRadius); got != want {
		t.Errorf("border.radius = %v, want %v (follow the window)", got, want)
	}

	if got, want := cfg.Border.ActiveColor, defaultBorderActiveColor; got != want {
		t.Errorf("border.active_color = %q, want %q", got, want)
	}

	if got, want := cfg.Border.InactiveColor, defaultBorderInactiveColor; got != want {
		t.Errorf("border.inactive_color = %q, want %q", got, want)
	}

	// Off is the default.
	cfg, err = Load(writeConfig(t, ""))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.Border.Enabled {
		t.Fatal("border.enabled defaults to true, want false")
	}

	// A radius of 0 is square corners, not following the window.
	cfg, err = Load(writeConfig(t, borderedConfig+"radius = 0\n"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if got := cfg.Border.CornerRadius(); got != 0 {
		t.Errorf("border.radius = %v, want 0", got)
	}
}

func TestLoad_RejectsABorderItCannotDraw(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		extra    string
		fragment string
	}{
		"thin":     {"width = 0.5\n", widthMessage},
		"wide":     {"width = 33\n", widthMessage},
		"radius":   {"radius = -1\n", "border.radius must be >= 0"},
		"active":   {"active_color = \"red\"\n", "border.active_color must be #rrggbb or #rrggbbaa"},
		"inactive": {"inactive_color = \"#12345\"\n", "border.inactive_color must be #rrggbb or #rrggbbaa"},
		"not hex":  {"active_color = \"#gggggg\"\n", "border.active_color must be #rrggbb or #rrggbbaa"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := Load(writeConfig(t, borderedConfig+testCase.extra))
			if !derrors.IsCode(err, derrors.CodeInvalidConfig) {
				t.Fatalf("Load() error = %v, want CodeInvalidConfig", err)
			}

			if !strings.Contains(err.Error(), testCase.fragment) {
				t.Errorf("error %q does not mention %q", err.Error(), testCase.fragment)
			}
		})
	}
}

func TestLoad_ChecksADisabledBorderToo(t *testing.T) {
	t.Parallel()

	_, err := Load(writeConfig(t, "[border]\nenabled = false\nwidth = 99\n"))
	if !derrors.IsCode(err, derrors.CodeInvalidConfig) ||
		!strings.Contains(err.Error(), widthMessage) {
		t.Fatalf("Load() error = %v, want the width message", err)
	}
}

func TestParseColor(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]Color{
		"#ff0000":   {Red: 1, Alpha: 1},
		"00ff00":    {Green: 1, Alpha: 1},
		"#0000ff80": {Blue: 1, Alpha: 128.0 / 255},
		"#FFFFFF":   {Red: 1, Green: 1, Blue: 1, Alpha: 1},
	} {
		got, err := ParseColor(text)
		if err != nil {
			t.Errorf("ParseColor(%q) error = %v", text, err)

			continue
		}

		if got != want {
			t.Errorf("ParseColor(%q) = %+v, want %+v", text, got, want)
		}
	}
}
