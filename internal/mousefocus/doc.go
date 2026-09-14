// Package mousefocus moves keyboard focus to the window under the pointer,
// as [mouse] focus_follows_mouse asks. It reads where the pointer is from
// a native tap, finds the window there through the window server, and
// focuses it through the same path focus_window --number takes.
package mousefocus
