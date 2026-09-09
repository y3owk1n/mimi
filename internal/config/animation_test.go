package config //nolint:testpackage // reads the validated config directly

import (
	"strings"
	"testing"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

const animatedTiling = "[tiling]\nenabled = true\nlayout = \"cat\"\n[tiling.animation]\nenabled = true\n"

func TestLoad_TilingAnimationDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeConfig(t, animatedTiling))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if !cfg.Tiling.Animation.Enabled {
		t.Fatal("tiling.animation.enabled = false, want true")
	}

	if got, want := cfg.Tiling.Animation.DurationMS, defaultTilingAnimationMS; got != want {
		t.Errorf("tiling.animation.duration_ms = %d, want %d", got, want)
	}

	if got, want := cfg.Tiling.Animation.Easing, defaultTilingAnimationEasing; got != want {
		t.Errorf("tiling.animation.easing = %q, want %q", got, want)
	}

	// Off is the default, whatever [tiling] says.
	cfg, err = Load(writeConfig(t, "[tiling]\nenabled = true\nlayout = \"cat\"\n"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.Tiling.Animation.Enabled {
		t.Fatal("tiling.animation.enabled defaults to true, want false")
	}
}

func TestLoad_RejectsATilingAnimationItCannotRun(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		extra    string
		fragment string
	}{
		"negative duration": {"duration_ms = -1\n", "tiling.animation.duration_ms must be between 1 and 1000"},
		"too long":          {"duration_ms = 1001\n", "tiling.animation.duration_ms must be between 1 and 1000"},
		"unknown easing":    {"easing = \"bounce\"\n", "tiling.animation.easing must be one of linear, ease-in, ease-out, ease-in-out"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := Load(writeConfig(t, animatedTiling+testCase.extra))
			if !derrors.IsCode(err, derrors.CodeInvalidConfig) {
				t.Fatalf("Load() error = %v, want CodeInvalidConfig", err)
			}

			if !strings.Contains(err.Error(), testCase.fragment) {
				t.Errorf("error %q does not mention %q", err.Error(), testCase.fragment)
			}
		})
	}
}

func TestLoad_TilingLayoutMode(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeConfig(t, "[tiling]\nenabled = true\nlayout = \"cat\"\n"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.Tiling.LayoutMode != LayoutModeOneshot {
		t.Fatalf(
			"tiling.layout_mode = %q, want %q by default",
			cfg.Tiling.LayoutMode,
			LayoutModeOneshot,
		)
	}

	cfg, err = Load(
		writeConfig(t, "[tiling]\nenabled = true\nlayout = \"cat\"\nlayout_mode = \"resident\"\n"),
	)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.Tiling.LayoutMode != LayoutModeResident {
		t.Fatalf("tiling.layout_mode = %q, want %q", cfg.Tiling.LayoutMode, LayoutModeResident)
	}

	_, err = Load(
		writeConfig(t, "[tiling]\nenabled = true\nlayout = \"cat\"\nlayout_mode = \"daemon\"\n"),
	)
	if err == nil ||
		!strings.Contains(err.Error(), "tiling.layout_mode must be oneshot or resident") {
		t.Fatalf("Load() error = %v, want the layout_mode message", err)
	}
}
