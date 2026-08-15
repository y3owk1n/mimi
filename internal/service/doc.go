// Package service manages the mimi launchd service: rendering its plist,
// installing and removing it, and driving launchctl to load, start, stop and
// query it.
//
// Every launchctl invocation runs through the launcher a [Service] holds,
// which is what lets install/uninstall/start/stop/status be exercised
// without a real launchctl underneath them. renderPlist sits beside that seam
// as a pure function, so the exact bytes mimi writes to disk are covered
// without touching the filesystem either.
package service
