// Package action implements CLI window and space utility commands.
//
// Every action runs on an [Executor], which reaches macOS only through the
// [Desktop] it holds. Windows cross that seam as values rather than native
// references, so the actions' branch logic can be exercised against a fake
// desktop; the adapter in this package is the one place that talks to
// internal/native, and it owns the lifetime of every reference behind a
// [WindowID].
package action
