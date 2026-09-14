// Package place sends a window where a [[tiling.rules]] entry says it goes
// when the window is created: to a space, to a display, or both, with focus
// following unless the rule says not to. It acts once, at creation, so a
// window the user later drags elsewhere stays where they put it.
package place
