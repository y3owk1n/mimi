package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

func TestResizeRequestFromArgs_CycleNeedsACyclingPresetAndNoSizeFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    action.ResizeWindowArgs
		wantErr bool
	}{
		{
			name: "left-half cycles",
			args: action.ResizeWindowArgs{Preset: presetLeftHalf, Cycle: true},
		},
		{
			name: "right-half cycles",
			args: action.ResizeWindowArgs{Preset: presetRightHalf, Cycle: true},
		},
		{
			name: "margins may still be asked for",
			args: action.ResizeWindowArgs{Preset: presetLeftHalf, Cycle: true, NoMargin: true},
		},
		{name: "no preset", args: action.ResizeWindowArgs{Cycle: true}, wantErr: true},
		{
			name:    "fill does not cycle",
			args:    action.ResizeWindowArgs{Preset: presetFill, Cycle: true},
			wantErr: true,
		},
		{
			name: "a width flag",
			args: action.ResizeWindowArgs{
				Preset:   "left-half",
				Cycle:    true,
				Width:    800,
				WidthSet: true,
			},
			wantErr: true,
		},
		{
			name: "an anchor flag",
			args: action.ResizeWindowArgs{
				Preset:    "left-half",
				Cycle:     true,
				Anchor:    "tl",
				AnchorSet: true,
			},
			wantErr: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			req, err := action.ResizeRequestFromArgs(testCase.args)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("ResizeRequestFromArgs() error = nil, want invalid input")
				}

				if !derrors.IsCode(err, derrors.CodeInvalidInput) {
					t.Fatalf("error = %v, want invalid input", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ResizeRequestFromArgs() error = %v, want nil", err)
			}

			if !req.Cycle {
				t.Fatal("Cycle = false on the request, want it carried")
			}
		})
	}
}
