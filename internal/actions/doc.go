// Package actions implements the macOS user-facing actions that
// mimi exposes: FocusWindow (cycle focus through windows on the
// active space), Space (switch to a Mission Control space by
// index), and MoveWindowToSpace (send the frontmost window to a
// Mission Control space by index).
//
// The handlers in this package are intentionally macOS-only;
// mimi is a macOS-only project.
package actions
