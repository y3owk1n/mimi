package action_test

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
	"github.com/y3owk1n/mimi/internal/geometry"
)

// TestExecutor_DeniedAccessibilityLeavesTheDesktopAlone checks that a revoked
// permission stops every action before it touches anything.
//
// The desktop underneath is rigged to fail every way it can — Mission Control
// open, every read erroring — so that reaching it at all before the permission
// check would come back as some other error. What the user sees is the
// permission, and a desktop still holding its window, its space and its frame.
func TestExecutor_DeniedAccessibilityLeavesTheDesktopAlone(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		run  func(*action.Executor) error
	}{
		{
			name: string(action.NameFocusWindow),
			run: func(e *action.Executor) error {
				return e.ExecuteCommand(focusCommandFor(t, false, false, false, false, false))
			},
		},
		{
			name: string(action.NameFocusWindow) + " directional",
			run: func(e *action.Executor) error {
				return e.ExecuteCommand(focusCommandFor(t, false, false, false, false, true))
			},
		},
		{
			name: string(action.NameSpace),
			run: func(e *action.Executor) error {
				return e.ExecuteCommand(spaceCommandFor(t, action.NameSpace, "3"))
			},
		},
		{
			name: string(action.NameSpace) + " next",
			run: func(e *action.Executor) error {
				return e.ExecuteCommand(spaceCommandFor(t, action.NameSpace, nextKeyword))
			},
		},
		{
			name: string(action.NameMoveWindowToSpace),
			run: func(e *action.Executor) error {
				return e.ExecuteCommand(spaceCommandFor(t, action.NameMoveWindowToSpace, "3"))
			},
		},
		{
			name: string(action.NameResizeWindow),
			run: func(e *action.Executor) error {
				return e.ExecuteCommand(resizeCommandFor(t, action.ResizeWindowArgs{
					Preset: presetFill,
				}))
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desktop := rowOfWindows(3, 0)
			desktop.spaceCount = 3
			desktop.activeSpace = 1
			desktop.windowSpace = 1
			desktop.screen = geometry.Screen{
				Visible:       geometry.Rect{X: 0, Y: 0, W: 1000, H: 1000},
				PrimaryHeight: 1000,
			}
			desktop.accessibilityErr = derrors.New(
				derrors.CodeAccessibilityDenied,
				"Accessibility permission is required",
			)

			denied := derrors.New(derrors.CodeActionFailed, "the desktop was reached too early")
			desktop.missionControlActive = true
			desktop.enumerateErr = denied
			desktop.frontmostErr = denied
			desktop.activeSpaceErr = denied
			desktop.screenErr = denied
			desktop.focusSpaceErr = denied
			desktop.moveErr = denied

			before := desktop.windows[0].frame

			err := testCase.run(action.NewExecutor(desktop))
			if err == nil {
				t.Fatal("ExecuteCommand() error = nil, want an error")
			}

			if !derrors.IsCode(err, derrors.CodeAccessibilityDenied) {
				t.Fatalf("ExecuteCommand() error = %v, want accessibility denied", err)
			}

			wantFocused(t, desktop, 1)

			if desktop.activeSpace != 1 {
				t.Fatalf("active space = %d, want it left on 1", desktop.activeSpace)
			}

			if desktop.windowSpace != 1 {
				t.Fatalf("window space = %d, want it left on 1", desktop.windowSpace)
			}

			if desktop.windows[0].frame != before {
				t.Fatalf(
					"window frame = %+v, want it left at %+v",
					desktop.windows[0].frame,
					before,
				)
			}
		})
	}
}
