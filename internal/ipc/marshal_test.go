//nolint:testpackage // exercises marshalCommand, which is unexported by design
package ipc

import (
	"reflect"
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
)

// TestMarshalCommand_FocusWindow pins the flag spellings action.Execute's
// string parser expects, since this is the only place that builds them now
// that the CLI hands over a typed action.Command instead.
func TestMarshalCommand_FocusWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args action.FocusWindowArgs
		want []string
	}{
		{name: "nothing set", args: action.FocusWindowArgs{}, want: []string{}},
		{
			name: "backward",
			args: action.FocusWindowArgs{Backward: true},
			want: []string{"--backward"},
		},
		{name: "up", args: action.FocusWindowArgs{Direction: "up"}, want: []string{"--up"}},
		{name: "down", args: action.FocusWindowArgs{Direction: "down"}, want: []string{"--down"}},
		{name: "left", args: action.FocusWindowArgs{Direction: "left"}, want: []string{"--left"}},
		{
			name: "right",
			args: action.FocusWindowArgs{Direction: "right"},
			want: []string{"--right"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			name, args := marshalCommand(action.Command{
				Name:        action.NameFocusWindow,
				FocusWindow: testCase.args,
			})

			if name != string(action.NameFocusWindow) {
				t.Errorf("name = %q, want %q", name, action.NameFocusWindow)
			}

			if !reflect.DeepEqual(args, testCase.want) {
				t.Errorf("args = %v, want %v", args, testCase.want)
			}
		})
	}
}

func TestMarshalCommand_SpaceArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arg  action.SpaceArg
		want []string
	}{
		{name: "absolute index", arg: action.SpaceArg{Index: 3}, want: []string{"3"}},
		{name: "next", arg: action.SpaceArg{Direction: 1}, want: []string{"next"}},
		{name: "prev", arg: action.SpaceArg{Direction: -1}, want: []string{"prev"}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, args := marshalCommand(action.Command{Name: action.NameSpace, Space: testCase.arg})

			if !reflect.DeepEqual(args, testCase.want) {
				t.Errorf("args = %v, want %v", args, testCase.want)
			}
		})
	}
}

func TestMarshalCommand_MoveWindowToSpaceUsesItsOwnField(t *testing.T) {
	t.Parallel()

	_, args := marshalCommand(action.Command{
		Name:              action.NameMoveWindowToSpace,
		MoveWindowToSpace: action.SpaceArg{Index: 5},
	})

	want := []string{"5"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}
}

// TestMarshalCommand_ResizeWindow pins the Changed()-style behavior: a flag
// the CLI never set is omitted from the wire args entirely, including a
// width or height explicitly given as zero — mirroring what
// cobraCmd.Flags().Changed(...) reports, not whether the value is > 0.
func TestMarshalCommand_ResizeWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args action.ResizeWindowArgs
		want []string
	}{
		{name: "nothing set", args: action.ResizeWindowArgs{}, want: []string{}},
		{
			name: "a preset",
			args: action.ResizeWindowArgs{Preset: "left-half"},
			want: []string{"left-half"},
		},
		{
			name: "width explicitly zero is still forwarded",
			args: action.ResizeWindowArgs{Width: 0, WidthSet: true},
			want: []string{"--width", "0"},
		},
		{
			name: "height explicitly zero is still forwarded",
			args: action.ResizeWindowArgs{Height: 0, HeightSet: true},
			want: []string{"--height", "0"},
		},
		{
			name: "width-percent explicitly zero is still forwarded",
			args: action.ResizeWindowArgs{WidthPercent: 0, WidthPercentSet: true},
			want: []string{"--width-percent", "0"},
		},
		{
			name: "height-percent explicitly zero is still forwarded",
			args: action.ResizeWindowArgs{HeightPercent: 0, HeightPercentSet: true},
			want: []string{"--height-percent", "0"},
		},
		{
			name: "width not set is omitted",
			args: action.ResizeWindowArgs{Width: 0, WidthSet: false},
			want: []string{},
		},
		{
			name: "everything set",
			args: action.ResizeWindowArgs{
				Width: 800, WidthSet: true,
				Height: 600, HeightSet: true,
				X: 10, XSet: true,
				Y: 20, YSet: true,
				Anchor: "cc", AnchorSet: true,
				UseMargin: true,
			},
			want: []string{
				"--width", "800",
				"--height", "600",
				"--x", "10",
				"--y", "20",
				"--anchor", "cc",
				"--margin",
			},
		},
		{
			name: "no-margin",
			args: action.ResizeWindowArgs{NoMargin: true},
			want: []string{"--no-margin"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, args := marshalCommand(action.Command{
				Name:         action.NameResizeWindow,
				ResizeWindow: testCase.args,
			})

			if !reflect.DeepEqual(args, testCase.want) {
				t.Errorf("args = %v, want %v", args, testCase.want)
			}
		})
	}
}
